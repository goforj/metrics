package metrics

import "testing"

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
