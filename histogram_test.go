package metrics

import (
	"testing"
	"time"
)

// TestHistogramObserveBuckets verifies inclusive bounds and the exclusive snapshot representation.
func TestHistogramObserveBuckets(t *testing.T) {
	reg := NewRegistry()
	hist, err := reg.Histogram(Descriptor{
		Name: "queue.duration",
		Help: "Queue duration.",
		Kind: KindHistogram,
	}, []int64{10, 20})
	if err != nil {
		t.Fatalf("histogram registration failed: %v", err)
	}

	hist.Observe(5)
	hist.Observe(15)
	hist.Observe(25)

	snap := reg.Snapshot()
	if len(snap.Histograms) != 1 {
		t.Fatalf("expected one histogram, got %d", len(snap.Histograms))
	}

	got := snap.Histograms[0]
	wantBuckets := []uint64{1, 1, 1}
	for i := range wantBuckets {
		if got.BucketCounts[i] != wantBuckets[i] {
			t.Fatalf("bucket %d: got %d want %d", i, got.BucketCounts[i], wantBuckets[i])
		}
	}
	if got.Count != 3 {
		t.Fatalf("count: got %d want 3", got.Count)
	}
	if got.Sum != 45 {
		t.Fatalf("sum: got %v want 45", got.Sum)
	}
}

// TestHistogramRejectsUnsortedBounds verifies cumulative exposition cannot be built from descending bounds.
func TestHistogramRejectsUnsortedBounds(t *testing.T) {
	reg := NewRegistry()
	_, err := reg.Histogram(Descriptor{
		Name: "bad.histogram",
		Help: "Bad histogram.",
		Kind: KindHistogram,
	}, []int64{20, 10})
	if err == nil {
		t.Fatalf("expected validation error")
	}
}

// TestHistogramObserveSince verifies elapsed durations use the seconds histogram storage path.
func TestHistogramObserveSince(t *testing.T) {
	reg := NewRegistry()
	hist := reg.MustHistogram(Descriptor{
		Name: "request.duration",
		Help: "Request duration.",
		Kind: KindHistogram,
		Unit: UnitSeconds,
	}, DurationBounds([]time.Duration{time.Millisecond, 2 * time.Millisecond}))

	hist.ObserveSince(time.Now().Add(-1500 * time.Microsecond))

	snap := reg.Snapshot()
	if got := snap.Histograms[0].Count; got != 1 {
		t.Fatalf("count: got %d want 1", got)
	}
	if got := snap.Histograms[0].BucketCounts[1]; got != 1 {
		t.Fatalf("bucket[1]: got %d want 1", got)
	}
}

// TestDefaultDurationBoundsReturnsCopy verifies callers cannot mutate package defaults.
func TestDefaultDurationBoundsReturnsCopy(t *testing.T) {
	first := DefaultDurationBounds()
	second := DefaultDurationBounds()
	first[0] = time.Hour
	if second[0] == time.Hour {
		t.Fatalf("expected bounds copy")
	}
}

// TestHistogramFloatSumTradesUnitPrecisionForRange documents the Prometheus-aligned sum contract.
func TestHistogramFloatSumTradesUnitPrecisionForRange(t *testing.T) {
	histogram := NewRegistry().MustHistogram(Descriptor{Name: "large.values", Help: "Large values."}, nil)
	histogram.Observe(1 << 53)
	histogram.Observe(1)
	if got := histogram.Sum(); got != float64(1<<53) {
		t.Fatalf("rounded sum: got %v want %v", got, float64(1<<53))
	}
	histogram.Observe(1 << 62)
	histogram.Observe(1 << 62)
	if got := histogram.Sum(); got <= 0 {
		t.Fatalf("large sum wrapped negative: %v", got)
	}
}

// TestObserveSinceOnNilHistogramPanics verifies a missing required metric handle does not disable instrumentation silently.
func TestObserveSinceOnNilHistogramPanics(t *testing.T) {
	var hist *Histogram
	assertPanics(t, func() {
		hist.ObserveSince(time.Now())
	})
}
