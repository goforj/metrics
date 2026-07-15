package metrics

import (
	"bytes"
	"strconv"
	"testing"
	"time"
)

// BenchmarkCounterInc measures the uncontended counter update hot path.
func BenchmarkCounterInc(b *testing.B) {
	reg := NewRegistry()
	counter := reg.MustCounter(Descriptor{
		Name: "http.requests",
		Help: "HTTP requests.",
		Kind: KindCounter,
	})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		counter.Inc()
	}
}

// BenchmarkGaugeAdd measures the uncontended gauge update hot path.
func BenchmarkGaugeAdd(b *testing.B) {
	reg := NewRegistry()
	gauge := reg.MustGauge(Descriptor{
		Name: "jobs.inflight",
		Help: "In-flight jobs.",
		Kind: KindGauge,
	})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		gauge.Add(1)
	}
}

// BenchmarkHistogramObserveDuration measures the uncontended duration histogram update hot path.
func BenchmarkHistogramObserveDuration(b *testing.B) {
	reg := NewRegistry()
	hist := reg.MustDurationHistogram(Descriptor{
		Name: "http.request.duration",
		Help: "HTTP request duration.",
		Kind: KindHistogram,
	}, DefaultDurationBounds())

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hist.ObserveDuration(35 * time.Millisecond)
	}
}

// BenchmarkCounterIncParallel measures counter contention across benchmark workers.
func BenchmarkCounterIncParallel(b *testing.B) {
	reg := NewRegistry()
	counter := reg.MustCounter(Descriptor{
		Name: "parallel.http.requests",
		Help: "Parallel HTTP requests.",
	})

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(parallel *testing.PB) {
		for parallel.Next() {
			counter.Inc()
		}
	})
}

// BenchmarkGaugeAddParallel measures gauge contention across benchmark workers.
func BenchmarkGaugeAddParallel(b *testing.B) {
	reg := NewRegistry()
	gauge := reg.MustGauge(Descriptor{
		Name: "parallel.jobs.inflight",
		Help: "Parallel in-flight jobs.",
	})

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(parallel *testing.PB) {
		for parallel.Next() {
			gauge.Add(1)
		}
	})
}

// BenchmarkHistogramObserveParallel measures histogram contention across benchmark workers.
func BenchmarkHistogramObserveParallel(b *testing.B) {
	reg := NewRegistry()
	histogram := reg.MustHistogram(Descriptor{
		Name: "parallel.payload.size",
		Help: "Parallel payload sizes.",
		Unit: UnitBytes,
	}, []int64{128, 1_024, 8_192})

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(parallel *testing.PB) {
		for parallel.Next() {
			histogram.Observe(512)
		}
	})
}

// BenchmarkCounterVecWithLabelValuesHit measures repeated lookups of an existing vector child.
func BenchmarkCounterVecWithLabelValuesHit(b *testing.B) {
	reg := NewRegistry()
	vector := reg.MustCounterVec(Descriptor{
		Name: "http.requests.by_route",
		Help: "HTTP requests by route.",
	}, []string{"method", "route"})
	vector.WithLabelValues("GET", "/ready")

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		vector.WithLabelValues("GET", "/ready")
	}
}

// BenchmarkCounterVecWithLabelValuesMiss measures creation of distinct vector children with caller-owned values.
func BenchmarkCounterVecWithLabelValuesMiss(b *testing.B) {
	values := make([]string, b.N)
	for i := range values {
		values[i] = strconv.Itoa(i)
	}
	reg := NewRegistry()
	vector := reg.MustCounterVec(Descriptor{
		Name: "jobs.by_id",
		Help: "Jobs by identifier.",
	}, []string{"id"})

	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		vector.WithLabelValues(values[i])
	}
}

// BenchmarkCounterVecCachedChildInc measures the recommended cached-child update path.
func BenchmarkCounterVecCachedChildInc(b *testing.B) {
	reg := NewRegistry()
	child := reg.MustCounterVec(Descriptor{
		Name: "cached.http.requests.by_route",
		Help: "Cached HTTP requests by route.",
	}, []string{"method", "route"}).WithLabelValues("GET", "/ready")

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		child.Inc()
	}
}

// BenchmarkSnapshotAndEncodePrometheus measures a complete scrape of representative primitive families.
func BenchmarkSnapshotAndEncodePrometheus(b *testing.B) {
	reg := NewRegistry()
	counter := reg.MustCounter(Descriptor{
		Name: "http.requests",
		Help: "HTTP requests.",
		Kind: KindCounter,
	})
	gauge := reg.MustGauge(Descriptor{
		Name: "jobs.inflight",
		Help: "In-flight jobs.",
		Kind: KindGauge,
	})
	hist := reg.MustDurationHistogram(Descriptor{
		Name: "http.request.duration",
		Help: "HTTP request duration.",
		Kind: KindHistogram,
	}, DefaultDurationBounds())

	for i := 0; i < 1000; i++ {
		counter.Inc()
		gauge.Set(12)
		hist.ObserveDuration(42 * time.Millisecond)
	}

	var buf bytes.Buffer
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		if err := EncodePrometheus(&buf, reg.Snapshot()); err != nil {
			b.Fatalf("encode failed: %v", err)
		}
	}
}
