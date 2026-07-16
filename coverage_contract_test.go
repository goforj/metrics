package metrics

import "testing"

// TestVectorSlowPathsReuseTheWinningChild verifies the deterministic half of concurrent double-checked creation.
func TestVectorSlowPathsReuseTheWinningChild(t *testing.T) {
	registry := NewRegistry()
	counter := registry.MustCounterVec(Descriptor{Name: "counter.children", Help: "Counter children."}, []string{"key"})
	gauge := registry.MustGaugeVec(Descriptor{Name: "gauge.children", Help: "Gauge children."}, []string{"key"})
	histogram := registry.MustHistogramVec(Descriptor{Name: "histogram.children", Help: "Histogram children."}, []string{"key"}, []int64{1})

	if first := counter.WithLabelValues("value"); counter.loadOrCreate(labelsKey([]string{"value"}), []string{"value"}) != first {
		t.Fatal("counter slow path did not reuse child")
	}
	if first := gauge.WithLabelValues("value"); gauge.loadOrCreate(labelsKey([]string{"value"}), []string{"value"}) != first {
		t.Fatal("gauge slow path did not reuse child")
	}
	if first := histogram.WithLabelValues("value"); histogram.loadOrCreate(labelsKey([]string{"value"}), []string{"value"}) != first {
		t.Fatal("histogram slow path did not reuse child")
	}
}

// TestVectorRegistrationReportsKindSpecificValidationAndConflicts covers errors before shared registry reservation.
func TestVectorRegistrationReportsKindSpecificValidationAndConflicts(t *testing.T) {
	if _, err := NewRegistry().GaugeVec(Descriptor{Name: "gauges", Help: "Gauges."}, []string{"bad-key"}); err == nil {
		t.Fatal("expected gauge vector label validation error")
	}
	if _, err := NewRegistry().HistogramVec(Descriptor{Name: "histograms", Help: "Histograms."}, []string{"bad-key"}, nil); err == nil {
		t.Fatal("expected histogram vector label validation error")
	}
	if _, err := NewRegistry().HistogramVec(Descriptor{Name: "histograms", Help: "Histograms."}, []string{"key"}, []int64{2, 1}); err == nil {
		t.Fatal("expected histogram vector bounds validation error")
	}

	gaugeRegistry := NewRegistry()
	gaugeRegistry.MustGaugeVec(Descriptor{Name: "gauges", Help: "First help."}, []string{"key"})
	if _, err := gaugeRegistry.GaugeVec(Descriptor{Name: "gauges", Help: "Second help."}, []string{"key"}); err == nil {
		t.Fatal("expected gauge vector descriptor conflict")
	}

	histogramRegistry := NewRegistry()
	histogramRegistry.MustHistogramVec(Descriptor{Name: "histograms", Help: "First help."}, []string{"key"}, []int64{1})
	if _, err := histogramRegistry.HistogramVec(Descriptor{Name: "histograms", Help: "Second help."}, []string{"key"}, []int64{1}); err == nil {
		t.Fatal("expected histogram vector descriptor conflict")
	}
}

// TestRegistrationComparisonHelpersRejectLengthAndValueDifferences protects exact idempotency checks.
func TestRegistrationComparisonHelpersRejectLengthAndValueDifferences(t *testing.T) {
	if sameLabelKeys([]string{"a"}, []string{"b"}) {
		t.Fatal("different label keys compared equal")
	}
	if sameBounds([]int64{1}, []int64{1, 2}) {
		t.Fatal("different bound lengths compared equal")
	}

	registry := NewRegistry()
	registry.MustGauge(Descriptor{Name: "workers", Help: "First help."})
	if _, err := registry.Gauge(Descriptor{Name: "workers", Help: "Second help."}); err == nil {
		t.Fatal("expected scalar gauge descriptor conflict")
	}
}

// TestHistogramSnapshotSortsVectorChildren verifies histogram label ordering uses the shared comparator.
func TestHistogramSnapshotSortsVectorChildren(t *testing.T) {
	registry := NewRegistry()
	vector := registry.MustHistogramVec(Descriptor{Name: "latency", Help: "Latency."}, []string{"route"}, []int64{1})
	vector.WithLabelValues("z").Observe(1)
	vector.WithLabelValues("a").Observe(1)

	snapshot := registry.Snapshot()
	if got := snapshot.Histograms[0].Labels[0].Value; got != "a" {
		t.Fatalf("first route: got %q want a", got)
	}
}

// TestRegistrationAndSnapshotInputsAreDetached verifies caller-owned slices cannot mutate live metric identity.
func TestRegistrationAndSnapshotInputsAreDetached(t *testing.T) {
	registry := NewRegistry()
	keys := []string{"route"}
	bounds := []int64{1, 2}
	vector := registry.MustHistogramVec(Descriptor{Name: "latency", Help: "Latency."}, keys, bounds)
	keys[0] = "changed"
	bounds[0] = 99
	vector.WithLabelValues("/ready").Observe(1)

	first := registry.Snapshot()
	first.Histograms[0].Labels[0].Key = "changed"
	first.Histograms[0].Bounds[0] = 99
	first.Histograms[0].BucketCounts[0] = 99
	second := registry.Snapshot()
	if second.Histograms[0].Labels[0].Key != "route" || second.Histograms[0].Bounds[0] != 1 || second.Histograms[0].BucketCounts[0] != 1 {
		t.Fatalf("live state changed through caller slice: %#v", second.Histograms[0])
	}
}
