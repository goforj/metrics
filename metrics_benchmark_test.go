package metrics

import (
	"bytes"
	"testing"
	"time"
)

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

func BenchmarkHistogramObserveDuration(b *testing.B) {
	reg := NewRegistry()
	hist := reg.MustHistogram(Descriptor{
		Name: "http.request.duration",
		Help: "HTTP request duration.",
		Kind: KindHistogram,
		Unit: UnitSeconds,
	}, DurationBounds(DefaultDurationBounds()))

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hist.ObserveDuration(35 * time.Millisecond)
	}
}

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
	hist := reg.MustHistogram(Descriptor{
		Name: "http.request.duration",
		Help: "HTTP request duration.",
		Kind: KindHistogram,
		Unit: UnitSeconds,
	}, DurationBounds(DefaultDurationBounds()))

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
