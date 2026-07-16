package metrics

import (
	"bytes"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestPrometheusGoldenExpositionFreezesNamesEscapingLabelsAndHistogramShape protects the dashboard-facing wire contract.
func TestPrometheusGoldenExpositionFreezesNamesEscapingLabelsAndHistogramShape(t *testing.T) {
	registry := NewRegistry()
	counter := registry.MustCounterVec(Descriptor{
		Name: "HTTP.Requests.Total",
		Help: "Line one\nline two \\ slash",
	}, []string{"detail"})
	counter.WithLabelValues("slash\\ quote\" line\n tab\t cr\r 世界").Inc()
	registry.MustGauge(Descriptor{
		Name: "jobs.inflight",
		Help: "In-flight jobs.",
	}).Set(-2)
	histogram := registry.MustDurationHistogramVec(Descriptor{
		Name: "HTTP.Request.Duration",
		Help: "HTTP request duration.",
	}, []string{"route"}, []time.Duration{100 * time.Millisecond})
	histogram.WithLabelValues("/users").ObserveDuration(50 * time.Millisecond)
	histogram.WithLabelValues("/users").ObserveDuration(2 * time.Second)

	var output bytes.Buffer
	if err := EncodePrometheus(&output, registry.Snapshot()); err != nil {
		t.Fatalf("encode: %v", err)
	}
	want := "# HELP http_requests_total Line one\\nline two \\\\ slash\n" +
		"# TYPE http_requests_total counter\n" +
		"http_requests_total{detail=\"slash\\\\ quote\\\" line\\n tab\t cr\r 世界\"} 1\n" +
		"# HELP jobs_inflight In-flight jobs.\n" +
		"# TYPE jobs_inflight gauge\n" +
		"jobs_inflight -2\n" +
		"# HELP http_request_duration_seconds HTTP request duration.\n" +
		"# TYPE http_request_duration_seconds histogram\n" +
		"http_request_duration_seconds_bucket{route=\"/users\",le=\"0.1\"} 1\n" +
		"http_request_duration_seconds_bucket{route=\"/users\",le=\"+Inf\"} 2\n" +
		"http_request_duration_seconds_sum{route=\"/users\"} 2.05\n" +
		"http_request_duration_seconds_count{route=\"/users\"} 2\n"
	if output.String() != want {
		t.Fatalf("exposition mismatch:\n--- got ---\n%s--- want ---\n%s", output.String(), want)
	}
}

// TestPrometheusCounterUnitPrecedesTotalSuffix keeps base units in the conventional position.
func TestPrometheusCounterUnitPrecedesTotalSuffix(t *testing.T) {
	tests := []struct {
		name string
		desc Descriptor
		want string
	}{
		{name: "suffix absent", desc: Descriptor{Name: "jobs.processed", Kind: KindCounter, Unit: UnitItems}, want: "jobs_processed_items_total"},
		{name: "total supplied", desc: Descriptor{Name: "jobs.processed.total", Kind: KindCounter, Unit: UnitItems}, want: "jobs_processed_items_total"},
		{name: "unit and total supplied", desc: Descriptor{Name: "jobs.processed.items.total", Kind: KindCounter, Unit: UnitItems}, want: "jobs_processed_items_total"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := prometheusMetricName(test.desc); got != test.want {
				t.Fatalf("name: got %q want %q", got, test.want)
			}
		})
	}
	if got := normalizePrometheusName("!!!"); got != "_" {
		t.Fatalf("empty normalized name: got %q want _", got)
	}
}

// TestEncodePrometheusRejectsMalformedSnapshots validates every public snapshot invariant before any bytes are written.
func TestEncodePrometheusRejectsMalformedSnapshots(t *testing.T) {
	counter := Descriptor{Name: "requests", Help: "Requests.", Kind: KindCounter}
	gauge := Descriptor{Name: "workers", Help: "Workers.", Kind: KindGauge}
	histogram := Descriptor{Name: "latency", Help: "Latency.", Kind: KindHistogram}
	validHistogram := HistogramSnapshot{Descriptor: histogram, Bounds: []int64{1}, BucketCounts: []uint64{0, 0}}

	tests := []struct {
		name     string
		snapshot *Snapshot
		want     string
	}{
		{
			name: "counter order",
			snapshot: &Snapshot{Counters: []CounterSnapshot{
				{Descriptor: Descriptor{Name: "z", Help: "Z.", Kind: KindCounter}},
				{Descriptor: Descriptor{Name: "a", Help: "A.", Kind: KindCounter}},
			}},
			want: "counter snapshot is not sorted",
		},
		{
			name: "gauge order",
			snapshot: &Snapshot{Gauges: []GaugeSnapshot{
				{Descriptor: Descriptor{Name: "z", Help: "Z.", Kind: KindGauge}},
				{Descriptor: Descriptor{Name: "a", Help: "A.", Kind: KindGauge}},
			}},
			want: "gauge snapshot is not sorted",
		},
		{
			name: "histogram order",
			snapshot: &Snapshot{Histograms: []HistogramSnapshot{
				{Descriptor: Descriptor{Name: "z", Help: "Z.", Kind: KindHistogram}, BucketCounts: []uint64{0}},
				{Descriptor: Descriptor{Name: "a", Help: "A.", Kind: KindHistogram}, BucketCounts: []uint64{0}},
			}},
			want: "histogram snapshot is not sorted",
		},
		{name: "counter kind", snapshot: &Snapshot{Counters: []CounterSnapshot{{Descriptor: gauge}}}, want: "want \"counter\""},
		{name: "gauge kind", snapshot: &Snapshot{Gauges: []GaugeSnapshot{{Descriptor: counter}}}, want: "want \"gauge\""},
		{name: "histogram kind", snapshot: &Snapshot{Histograms: []HistogramSnapshot{{Descriptor: counter, BucketCounts: []uint64{0}}}}, want: "want \"histogram\""},
		{name: "invalid descriptor", snapshot: &Snapshot{Counters: []CounterSnapshot{{Descriptor: Descriptor{Name: "requests", Help: "Requests.", Kind: KindCounter, Unit: Unit("bad")}}}}, want: "unsupported unit"},
		{name: "label whitespace", snapshot: &Snapshot{Counters: []CounterSnapshot{{Descriptor: counter, Labels: []Label{{Key: " route", Value: "/"}}}}}, want: "surrounding whitespace"},
		{name: "invalid label key", snapshot: &Snapshot{Counters: []CounterSnapshot{{Descriptor: counter, Labels: []Label{{Key: "route-name", Value: "/"}}}}}, want: "invalid label key"},
		{name: "histogram le label", snapshot: &Snapshot{Histograms: []HistogramSnapshot{{Descriptor: histogram, Labels: []Label{{Key: "le", Value: "1"}}, BucketCounts: []uint64{0}}}}, want: "reserved"},
		{name: "duplicate label", snapshot: &Snapshot{Counters: []CounterSnapshot{{Descriptor: counter, Labels: []Label{{Key: "route"}, {Key: "route"}}}}}, want: "duplicate label"},
		{name: "invalid label value", snapshot: &Snapshot{Counters: []CounterSnapshot{{Descriptor: counter, Labels: []Label{{Key: "route", Value: string([]byte{0xff})}}}}}, want: "valid UTF-8"},
		{name: "unsorted bounds", snapshot: &Snapshot{Histograms: []HistogramSnapshot{{Descriptor: histogram, Bounds: []int64{2, 1}, BucketCounts: []uint64{0, 0, 0}}}}, want: "strictly increasing"},
		{name: "bucket length", snapshot: &Snapshot{Histograms: []HistogramSnapshot{{Descriptor: histogram, Bounds: []int64{1}, BucketCounts: []uint64{0}}}}, want: "has 1 buckets, want 2"},
		{name: "bucket overflow", snapshot: &Snapshot{Histograms: []HistogramSnapshot{{Descriptor: histogram, Bounds: []int64{1}, BucketCounts: []uint64{math.MaxUint64, 1}, Count: 0}}}, want: "overflows"},
		{name: "count mismatch", snapshot: &Snapshot{Histograms: []HistogramSnapshot{{Descriptor: histogram, Bounds: []int64{1}, BucketCounts: []uint64{1, 0}, Count: 2}}}, want: "does not match"},
		{
			name: "inconsistent family labels",
			snapshot: &Snapshot{Counters: []CounterSnapshot{
				{Descriptor: counter, Labels: []Label{{Key: "a", Value: "1"}}},
				{Descriptor: counter, Labels: []Label{{Key: "b", Value: "2"}}},
			}},
			want: "inconsistent snapshot family",
		},
		{
			name: "inconsistent histogram bounds",
			snapshot: &Snapshot{Histograms: []HistogramSnapshot{
				{Descriptor: histogram, Labels: []Label{{Key: "route", Value: "a"}}, Bounds: []int64{1}, BucketCounts: []uint64{0, 0}},
				{Descriptor: histogram, Labels: []Label{{Key: "route", Value: "b"}}, Bounds: []int64{2}, BucketCounts: []uint64{0, 0}},
			}},
			want: "inconsistent snapshot family",
		},
		{
			name: "derived series collision",
			snapshot: &Snapshot{
				Gauges:     []GaugeSnapshot{{Descriptor: Descriptor{Name: "latency.count", Help: "Latency count.", Kind: KindGauge}}},
				Histograms: []HistogramSnapshot{validHistogram},
			},
			want: "conflicting descriptors",
		},
		{
			name: "duplicate sample",
			snapshot: &Snapshot{Counters: []CounterSnapshot{
				{Descriptor: counter, Labels: []Label{{Key: "route", Value: "/"}}},
				{Descriptor: counter, Labels: []Label{{Key: "route", Value: "/"}}},
			}},
			want: "duplicate snapshot sample",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			err := EncodePrometheus(&output, test.snapshot)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error: got %v, want substring %q", err, test.want)
			}
			if output.Len() != 0 {
				t.Fatalf("validation wrote %d bytes before failing", output.Len())
			}
		})
	}
}

// TestEncodePrometheusRejectsNilWriter ensures invalid output wiring fails before traversal.
func TestEncodePrometheusRejectsNilWriter(t *testing.T) {
	if err := EncodePrometheus(nil, &Snapshot{}); err == nil {
		t.Fatal("expected missing writer error")
	}
}

// TestEncodePrometheusPropagatesEveryWriteFailure covers metadata and samples for every metric kind.
func TestEncodePrometheusPropagatesEveryWriteFailure(t *testing.T) {
	registry := NewRegistry()
	registry.MustCounter(Descriptor{Name: "requests", Help: "Requests."}).Inc()
	registry.MustGauge(Descriptor{Name: "workers", Help: "Workers."}).Set(1)
	registry.MustHistogram(Descriptor{Name: "sizes", Help: "Sizes."}, []int64{1}).Observe(1)
	snapshot := registry.Snapshot()

	for successfulWrites := 0; successfulWrites < 12; successfulWrites++ {
		writer := &failAfterWriter{remaining: successfulWrites}
		if err := EncodePrometheus(writer, snapshot); err == nil {
			t.Fatalf("successful writes %d: expected writer error", successfulWrites)
		}
	}
	writer := &failAfterWriter{remaining: 12}
	if err := EncodePrometheus(writer, snapshot); err != nil {
		t.Fatalf("fully writable output failed: %v", err)
	}
}

// TestHandlerReportsSnapshotValidationBeforeCommittingResponse verifies buffered HTTP error handling.
func TestHandlerReportsSnapshotValidationBeforeCommittingResponse(t *testing.T) {
	registry := NewRegistry()
	counter := registry.MustCounter(Descriptor{Name: "requests", Help: "Requests."})
	counter.desc.Kind = KindGauge

	request := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	response := httptest.NewRecorder()
	Handler(registry).ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status: got %d want %d", response.Code, http.StatusInternalServerError)
	}
	if !strings.Contains(response.Body.String(), "snapshot descriptor") {
		t.Fatalf("body: got %q", response.Body.String())
	}
}

// failAfterWriter fails after a configured number of successful writes.
type failAfterWriter struct {
	remaining int
}

// Write implements io.Writer for deterministic exporter failure injection.
func (w *failAfterWriter) Write(value []byte) (int, error) {
	if w.remaining == 0 {
		return 0, errors.New("write failed")
	}
	w.remaining--
	return len(value), nil
}
