package metrics

import (
	"fmt"
	"strconv"
	"strings"
)

// Label is one Prometheus label key/value pair.
type Label struct {
	Key   string
	Value string
}

func validateLabelKeys(keys []string) ([]string, error) {
	if len(keys) == 0 {
		return nil, fmt.Errorf("metrics: at least one label key is required")
	}
	out := make([]string, len(keys))
	seen := make(map[string]struct{}, len(keys))
	for i, key := range keys {
		key = strings.TrimSpace(key)
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

func validateLabelKey(key string) error {
	if key == "" {
		return fmt.Errorf("metrics: label key is required")
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
