package metrics

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// snapshotFamily captures the metadata that every child in one exported family must share.
type snapshotFamily struct {
	descriptor Descriptor
	labelKeys  string
	bounds     string
}

// snapshotValidator tracks cross-sample invariants while validating a caller-visible snapshot.
type snapshotValidator struct {
	families     map[string]snapshotFamily
	seriesOwners map[string]Descriptor
	samples      map[string]struct{}
}

// validatePrometheusSnapshot prevents malformed caller-constructed snapshots from producing ambiguous exposition.
func validatePrometheusSnapshot(snapshot *Snapshot) error {
	validator := snapshotValidator{
		families:     map[string]snapshotFamily{},
		seriesOwners: map[string]Descriptor{},
		samples:      map[string]struct{}{},
	}

	if err := validateSnapshotOrder(snapshot); err != nil {
		return err
	}
	for _, metric := range snapshot.Counters {
		if err := validator.validateMetric(metric.Descriptor, metric.Labels, KindCounter, nil, nil, 0); err != nil {
			return err
		}
	}
	for _, metric := range snapshot.Gauges {
		if err := validator.validateMetric(metric.Descriptor, metric.Labels, KindGauge, nil, nil, 0); err != nil {
			return err
		}
	}
	for _, metric := range snapshot.Histograms {
		if err := validator.validateMetric(metric.Descriptor, metric.Labels, KindHistogram, metric.Bounds, metric.BucketCounts, metric.Count); err != nil {
			return err
		}
	}
	return nil
}

// validateMetric checks one sample and records its family and generated-series ownership.
func (v *snapshotValidator) validateMetric(desc Descriptor, labels []Label, kind Kind, bounds []int64, buckets []uint64, count uint64) error {
	if desc.Kind != kind {
		return fmt.Errorf("metrics: snapshot descriptor %q has kind %q, want %q", desc.Name, desc.Kind, kind)
	}
	if _, err := canonicalizeDescriptor(desc, kind); err != nil {
		return err
	}

	labelKeys, err := validateSnapshotLabels(labels, kind == KindHistogram)
	if err != nil {
		return fmt.Errorf("metrics: snapshot metric %q: %w", desc.Name, err)
	}
	boundsIdentity := ""
	if kind == KindHistogram {
		if err := validateSnapshotHistogram(desc.Name, bounds, buckets, count); err != nil {
			return err
		}
		boundsIdentity = int64SliceKey(bounds)
	}

	base := prometheusMetricName(desc)
	family := snapshotFamily{
		descriptor: desc,
		labelKeys:  labelKeys,
		bounds:     boundsIdentity,
	}
	if previous, exists := v.families[base]; exists && previous != family {
		return fmt.Errorf("metrics: inconsistent snapshot family %q", base)
	}
	v.families[base] = family

	for _, name := range prometheusSeriesNames(desc) {
		if owner, exists := v.seriesOwners[name]; exists && owner != desc {
			return fmt.Errorf("metrics: snapshot Prometheus series %q has conflicting descriptors", name)
		}
		v.seriesOwners[name] = desc
	}

	sample := base + "\x00" + formatLabels(labels)
	if _, exists := v.samples[sample]; exists {
		return fmt.Errorf("metrics: duplicate snapshot sample %q", base)
	}
	v.samples[sample] = struct{}{}
	return nil
}

// validateSnapshotOrder ensures family metadata remains adjacent and output remains deterministic.
func validateSnapshotOrder(snapshot *Snapshot) error {
	for i := 1; i < len(snapshot.Counters); i++ {
		previous := snapshot.Counters[i-1]
		current := snapshot.Counters[i]
		if snapshotMetricAfter(previous.Descriptor, previous.Labels, current.Descriptor, current.Labels) {
			return fmt.Errorf("metrics: counter snapshot is not sorted")
		}
	}
	for i := 1; i < len(snapshot.Gauges); i++ {
		previous := snapshot.Gauges[i-1]
		current := snapshot.Gauges[i]
		if snapshotMetricAfter(previous.Descriptor, previous.Labels, current.Descriptor, current.Labels) {
			return fmt.Errorf("metrics: gauge snapshot is not sorted")
		}
	}
	for i := 1; i < len(snapshot.Histograms); i++ {
		previous := snapshot.Histograms[i-1]
		current := snapshot.Histograms[i]
		if snapshotMetricAfter(previous.Descriptor, previous.Labels, current.Descriptor, current.Labels) {
			return fmt.Errorf("metrics: histogram snapshot is not sorted")
		}
	}
	return nil
}

// snapshotMetricAfter reports whether the left sample should sort after the right sample.
func snapshotMetricAfter(left Descriptor, leftLabels []Label, right Descriptor, rightLabels []Label) bool {
	if left.Name != right.Name {
		return left.Name > right.Name
	}
	return compareLabels(leftLabels, rightLabels) > 0
}

// validateSnapshotLabels returns an identity for a valid, ordered label-key schema.
func validateSnapshotLabels(labels []Label, histogram bool) (string, error) {
	keys := make([]string, len(labels))
	values := make([]string, len(labels))
	seen := make(map[string]struct{}, len(labels))
	for i, label := range labels {
		if label.Key != strings.TrimSpace(label.Key) {
			return "", fmt.Errorf("label key %q has surrounding whitespace", label.Key)
		}
		if err := validateLabelKey(label.Key); err != nil {
			return "", err
		}
		if histogram && label.Key == "le" {
			return "", fmt.Errorf("histogram label key %q is reserved", label.Key)
		}
		if _, exists := seen[label.Key]; exists {
			return "", fmt.Errorf("duplicate label key %q", label.Key)
		}
		seen[label.Key] = struct{}{}
		keys[i] = label.Key
		values[i] = label.Value
	}
	if err := validateLabelValues(values); err != nil {
		return "", err
	}
	return labelsKey(keys), nil
}

// validateSnapshotHistogram checks the public exclusive-bucket representation before encoding it cumulatively.
func validateSnapshotHistogram(name string, bounds []int64, buckets []uint64, count uint64) error {
	if err := validateBounds(bounds); err != nil {
		return fmt.Errorf("metrics: snapshot histogram %q: %w", name, err)
	}
	if len(buckets) != len(bounds)+1 {
		return fmt.Errorf("metrics: snapshot histogram %q has %d buckets, want %d", name, len(buckets), len(bounds)+1)
	}
	var total uint64
	for _, bucket := range buckets {
		if math.MaxUint64-total < bucket {
			return fmt.Errorf("metrics: snapshot histogram %q bucket count overflows", name)
		}
		total += bucket
	}
	if total != count {
		return fmt.Errorf("metrics: snapshot histogram %q bucket total %d does not match count %d", name, total, count)
	}
	return nil
}

// int64SliceKey builds an unambiguous identity for histogram bounds.
func int64SliceKey(values []int64) string {
	var builder strings.Builder
	for _, value := range values {
		encoded := strconv.FormatInt(value, 10)
		builder.WriteString(strconv.Itoa(len(encoded)))
		builder.WriteByte(':')
		builder.WriteString(encoded)
	}
	return builder.String()
}
