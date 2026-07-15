package metrics

import (
	"sync"
	"testing"
	"time"
)

// TestCounterAndGaugeConcurrentUpdates verifies that primitive updates do not lose writes under contention.
func TestCounterAndGaugeConcurrentUpdates(t *testing.T) {
	const (
		workers     = 32
		updates     = 2_000
		wantUpdates = workers * updates
	)

	registry := NewRegistry()
	counter := registry.MustCounter(Descriptor{Name: "concurrent.counter", Help: "Concurrent counter updates."})
	gauge := registry.MustGauge(Descriptor{Name: "concurrent.gauge", Help: "Concurrent gauge updates."})

	start := make(chan struct{})
	var wait sync.WaitGroup
	wait.Add(workers)
	for range workers {
		go func() {
			defer wait.Done()
			<-start
			for range updates {
				counter.Inc()
				gauge.Add(1)
			}
		}()
	}
	close(start)
	wait.Wait()

	if got := counter.Value(); got != wantUpdates {
		t.Fatalf("counter value: got %d want %d", got, wantUpdates)
	}
	if got := gauge.Value(); got != wantUpdates {
		t.Fatalf("gauge value: got %d want %d", got, wantUpdates)
	}
}

// TestHistogramObserveAndSnapshotRemainCoherent verifies scrape invariants while observations are in flight.
func TestHistogramObserveAndSnapshotRemainCoherent(t *testing.T) {
	const (
		writers      = 8
		observations = 20_000
	)

	registry := NewRegistry()
	histogram := registry.MustHistogram(
		Descriptor{Name: "concurrent.histogram", Help: "Concurrent histogram observations."},
		[]int64{0, 1, 2},
	)

	start := make(chan struct{})
	var wait sync.WaitGroup
	wait.Add(writers)
	for range writers {
		go func() {
			defer wait.Done()
			<-start
			for range observations {
				histogram.Observe(1)
			}
		}()
	}
	close(start)

	for range 16 {
		snapshot := snapshotRegistryWithin(t, registry, 5*time.Second)
		assertHistogramSnapshotCoherent(t, snapshot)
	}
	wait.Wait()

	snapshot := registry.Snapshot()
	assertHistogramSnapshotCoherent(t, snapshot)
	wantCount := uint64(writers * observations)
	if got := snapshot.Histograms[0].Count; got != wantCount {
		t.Fatalf("final histogram count: got %d want %d", got, wantCount)
	}
}

// TestHistogramSnapshotCompletesWithContinuousWriters proves that a scrape cannot be starved by ongoing observations.
func TestHistogramSnapshotCompletesWithContinuousWriters(t *testing.T) {
	const writers = 8

	registry := NewRegistry()
	histogram := registry.MustHistogram(
		Descriptor{Name: "live.histogram", Help: "Continuously updated histogram."},
		[]int64{0, 1, 2},
	)

	stop := make(chan struct{})
	var ready sync.WaitGroup
	var wait sync.WaitGroup
	ready.Add(writers)
	wait.Add(writers)
	for range writers {
		go func() {
			defer wait.Done()
			histogram.Observe(1)
			ready.Done()
			for {
				select {
				case <-stop:
					return
				default:
					histogram.Observe(1)
				}
			}
		}()
	}
	ready.Wait()
	defer func() {
		close(stop)
		wait.Wait()
	}()

	snapshot := snapshotRegistryWithin(t, registry, 5*time.Second)
	assertHistogramSnapshotCoherent(t, snapshot)
}

// TestConcurrentWithLabelValuesReturnsOneChild verifies double-checked vector insertion for every vector kind.
func TestConcurrentWithLabelValuesReturnsOneChild(t *testing.T) {
	t.Run("counter", func(t *testing.T) {
		registry := NewRegistry()
		vector := registry.MustCounterVec(
			Descriptor{Name: "concurrent.counter.vector", Help: "Concurrent counter vector lookup."},
			[]string{"route"},
		)
		assertConcurrentChildIdentity(t, func() any {
			return vector.WithLabelValues("/ready")
		})
	})

	t.Run("gauge", func(t *testing.T) {
		registry := NewRegistry()
		vector := registry.MustGaugeVec(
			Descriptor{Name: "concurrent.gauge.vector", Help: "Concurrent gauge vector lookup."},
			[]string{"queue"},
		)
		assertConcurrentChildIdentity(t, func() any {
			return vector.WithLabelValues("critical")
		})
	})

	t.Run("histogram", func(t *testing.T) {
		registry := NewRegistry()
		vector := registry.MustHistogramVec(
			Descriptor{Name: "concurrent.histogram.vector", Help: "Concurrent histogram vector lookup."},
			[]string{"operation"},
			[]int64{1, 2},
		)
		assertConcurrentChildIdentity(t, func() any {
			return vector.WithLabelValues("write")
		})
	})
}

// TestPrimitiveHotPathsDoNotAllocate protects the allocation-free update contract used in request paths.
func TestPrimitiveHotPathsDoNotAllocate(t *testing.T) {
	registry := NewRegistry()
	counter := registry.MustCounter(Descriptor{Name: "allocation.counter", Help: "Counter allocation check."})
	gauge := registry.MustGauge(Descriptor{Name: "allocation.gauge", Help: "Gauge allocation check."})
	histogram := registry.MustHistogram(
		Descriptor{Name: "allocation.histogram", Help: "Histogram allocation check."},
		[]int64{1, 2},
	)
	child := registry.MustCounterVec(
		Descriptor{Name: "allocation.counter.vector", Help: "Counter vector allocation check."},
		[]string{"route"},
	).WithLabelValues("/ready")

	tests := []struct {
		name string
		fn   func()
	}{
		{name: "counter increment", fn: counter.Inc},
		{name: "gauge add", fn: func() { gauge.Add(1) }},
		{name: "histogram observe", fn: func() { histogram.Observe(1) }},
		{name: "cached vector child increment", fn: child.Inc},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := testing.AllocsPerRun(1_000, test.fn); got != 0 {
				t.Fatalf("allocations per update: got %v want 0", got)
			}
		})
	}
}

// snapshotRegistryWithin collects a registry snapshot with a deadline so liveness regressions fail promptly.
func snapshotRegistryWithin(t *testing.T, registry *Registry, timeout time.Duration) *Snapshot {
	t.Helper()
	result := make(chan *Snapshot, 1)
	go func() {
		result <- registry.Snapshot()
	}()

	select {
	case snapshot := <-result:
		return snapshot
	case <-time.After(timeout):
		t.Fatalf("registry snapshot did not complete within %s", timeout)
		return nil
	}
}

// assertHistogramSnapshotCoherent checks the count, bucket, and sum relationship required by Prometheus exposition.
func assertHistogramSnapshotCoherent(t *testing.T, snapshot *Snapshot) {
	t.Helper()
	if len(snapshot.Histograms) != 1 {
		t.Fatalf("histogram snapshot count: got %d want 1", len(snapshot.Histograms))
	}
	histogram := snapshot.Histograms[0]
	var bucketTotal uint64
	for _, count := range histogram.BucketCounts {
		bucketTotal += count
	}
	if bucketTotal != histogram.Count {
		t.Fatalf("bucket total %d does not match count %d", bucketTotal, histogram.Count)
	}
	if histogram.Sum != float64(histogram.Count) {
		t.Fatalf("sum %v does not match unit observations counted as %d", histogram.Sum, histogram.Count)
	}
}

// assertConcurrentChildIdentity verifies that every concurrent lookup receives the same child pointer.
func assertConcurrentChildIdentity(t *testing.T, lookup func() any) {
	t.Helper()
	const callers = 64

	results := make([]any, callers)
	start := make(chan struct{})
	var wait sync.WaitGroup
	wait.Add(callers)
	for i := range callers {
		go func() {
			defer wait.Done()
			<-start
			results[i] = lookup()
		}()
	}
	close(start)
	wait.Wait()

	first := results[0]
	if first == nil {
		t.Fatal("first vector child is nil")
	}
	for i, result := range results[1:] {
		if result != first {
			t.Fatalf("lookup %d returned a different child", i+1)
		}
	}
}
