package metrics

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Label is one Prometheus label key/value pair.
type Label struct {
	// Key identifies the label dimension.
	Key string
	// Value identifies one value within the label dimension.
	Value string
}

// validateLabelKeys returns trimmed, unique keys that are safe to expose.
func validateLabelKeys(keys []string) ([]string, error) {
	if len(keys) == 0 {
		return nil, fmt.Errorf("metrics: at least one label key is required")
	}
	out := make([]string, len(keys))
	seen := make(map[string]struct{}, len(keys))
	for i, key := range keys {
		if key != strings.TrimSpace(key) {
			return nil, fmt.Errorf("metrics: label key %q has surrounding whitespace", key)
		}
		if err := validateLabelKey(key); err != nil {
			return nil, err
		}
		if _, ok := seen[key]; ok {
			return nil, fmt.Errorf("metrics: duplicate label key %q", key)
		}
		seen[key] = struct{}{}
		out[i] = key
	}
	return out, nil
}

// validateHistogramLabelKeys prevents callers from shadowing the exporter-owned bucket label.
func validateHistogramLabelKeys(keys []string) error {
	for _, key := range keys {
		if key == "le" {
			return fmt.Errorf("metrics: histogram label key %q is reserved", key)
		}
	}
	return nil
}

// validateLabelValues rejects bytes that cannot be represented by the UTF-8 exposition format.
func validateLabelValues(values []string) error {
	for _, value := range values {
		if !utf8.ValidString(value) {
			return fmt.Errorf("metrics: label value must be valid UTF-8")
		}
	}
	return nil
}

// validateLabelKey enforces the Prometheus legacy label-name grammar.
func validateLabelKey(key string) error {
	if key == "" {
		return fmt.Errorf("metrics: label key is required")
	}
	if strings.HasPrefix(key, "__") {
		return fmt.Errorf("metrics: label key %q is reserved", key)
	}
	for i, r := range key {
		if i == 0 {
			if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_') {
				return fmt.Errorf("metrics: invalid label key %q", key)
			}
			continue
		}
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_') {
			return fmt.Errorf("metrics: invalid label key %q", key)
		}
	}
	return nil
}

// sameLabelKeys reports whether two vector registrations describe the same ordered dimensions.
func sameLabelKeys(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// makeLabels pairs a vector's fixed keys with one child metric's values.
func makeLabels(keys, values []string) []Label {
	labels := make([]Label, len(keys))
	for i := range keys {
		labels[i] = Label{
			Key:   keys[i],
			Value: values[i],
		}
	}
	return labels
}

// labelsKey builds an unambiguous map key without constraining label contents.
func labelsKey(values []string) string {
	var b strings.Builder
	for _, value := range values {
		b.WriteString(strconv.Itoa(len(value)))
		b.WriteByte(':')
		b.WriteString(value)
		b.WriteByte('|')
	}
	return b.String()
}

// compareLabels provides deterministic snapshot ordering for labeled metric children.
func compareLabels(a, b []Label) int {
	limit := len(a)
	if len(b) < limit {
		limit = len(b)
	}
	for i := 0; i < limit; i++ {
		if a[i].Key < b[i].Key {
			return -1
		}
		if a[i].Key > b[i].Key {
			return 1
		}
		if a[i].Value < b[i].Value {
			return -1
		}
		if a[i].Value > b[i].Value {
			return 1
		}
	}
	switch {
	case len(a) < len(b):
		return -1
	case len(a) > len(b):
		return 1
	default:
		return 0
	}
}
