package metrics

import (
	"testing"
	"time"
)

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
		t.Fatalf("sum: got %d want 45", got.Sum)
	}
}

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

func TestDefaultDurationBoundsReturnsCopy(t *testing.T) {
	first := DefaultDurationBounds()
	second := DefaultDurationBounds()
	first[0] = time.Hour
	if second[0] == time.Hour {
		t.Fatalf("expected bounds copy")
	}
}

func TestObserveSinceOnNilHistogramIsNoOp(t *testing.T) {
	var hist *Histogram
	hist.ObserveSince(time.Now())
}
