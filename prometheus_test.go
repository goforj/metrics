package metrics

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestEncodePrometheusCounterAndGauge(t *testing.T) {
	reg := NewRegistry()
	counter := reg.MustCounter(Descriptor{
		Name: "http.requests.total",
		Help: "Total HTTP requests.",
		Kind: KindCounter,
	})
	gauge := reg.MustGauge(Descriptor{
		Name: "jobs.inflight",
		Help: "In-flight jobs.",
		Kind: KindGauge,
	})

	counter.Add(3)
	gauge.Set(2)

	var out bytes.Buffer
	if err := EncodePrometheus(&out, reg.Snapshot()); err != nil {
		t.Fatalf("encode failed: %v", err)
	}

	text := out.String()
	if !strings.Contains(text, "# TYPE http_requests_total counter") {
		t.Fatalf("missing counter type line:\n%s", text)
	}
	if !strings.Contains(text, "http_requests_total 3") {
		t.Fatalf("missing counter value:\n%s", text)
	}
	if !strings.Contains(text, "# TYPE jobs_inflight gauge") {
		t.Fatalf("missing gauge type line:\n%s", text)
	}
	if !strings.Contains(text, "jobs_inflight 2") {
		t.Fatalf("missing gauge value:\n%s", text)
	}
}

func TestEncodePrometheusDurationHistogram(t *testing.T) {
	reg := NewRegistry()
	hist, err := reg.DurationHistogram(Descriptor{
		Name: "http.request.duration",
		Help: "HTTP request duration.",
		Kind: KindHistogram,
	}, []time.Duration{100 * time.Millisecond, 500 * time.Millisecond})
	if err != nil {
		t.Fatalf("duration histogram registration failed: %v", err)
	}

	hist.ObserveDuration(50 * time.Millisecond)
	hist.ObserveDuration(200 * time.Millisecond)
	hist.ObserveDuration(2 * time.Second)

	var out bytes.Buffer
	if err := EncodePrometheus(&out, reg.Snapshot()); err != nil {
		t.Fatalf("encode failed: %v", err)
	}

	text := out.String()
	if !strings.Contains(text, "# TYPE http_request_duration_seconds histogram") {
		t.Fatalf("missing histogram type line:\n%s", text)
	}
	if !strings.Contains(text, `http_request_duration_seconds_bucket{le="0.1"} 1`) {
		t.Fatalf("missing first bucket:\n%s", text)
	}
	if !strings.Contains(text, `http_request_duration_seconds_bucket{le="0.5"} 2`) {
		t.Fatalf("missing second bucket:\n%s", text)
	}
	if !strings.Contains(text, `http_request_duration_seconds_bucket{le="+Inf"} 3`) {
		t.Fatalf("missing +Inf bucket:\n%s", text)
	}
	if !strings.Contains(text, "http_request_duration_seconds_count 3") {
		t.Fatalf("missing count:\n%s", text)
	}
	if !strings.Contains(text, "http_request_duration_seconds_sum 2.25") {
		t.Fatalf("missing sum:\n%s", text)
	}
}

func TestEncodePrometheusEscapesHelpAndNormalizesNames(t *testing.T) {
	reg := NewRegistry()
	counter := reg.MustCounter(Descriptor{
		Name: "HTTP Requests/Total",
		Help: "Line one\nline two \\ slash",
		Kind: KindCounter,
	})
	counter.Inc()

	var out bytes.Buffer
	if err := EncodePrometheus(&out, reg.Snapshot()); err != nil {
		t.Fatalf("encode failed: %v", err)
	}

	text := out.String()
	if !strings.Contains(text, "# HELP http_requests_total Line one\\nline two \\\\ slash") {
		t.Fatalf("missing escaped help:\n%s", text)
	}
	if !strings.Contains(text, "# TYPE http_requests_total counter") {
		t.Fatalf("missing normalized type line:\n%s", text)
	}
}

func TestHandlerSetsPrometheusContentType(t *testing.T) {
	reg := NewRegistry()
	reg.MustCounter(Descriptor{
		Name: "http.requests",
		Help: "HTTP requests.",
		Kind: KindCounter,
	}).Inc()

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()

	Handler(reg).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); got != prometheusContentType {
		t.Fatalf("content-type: got %q want %q", got, prometheusContentType)
	}
	if !strings.Contains(rec.Body.String(), "http_requests_total 1") {
		t.Fatalf("missing body content:\n%s", rec.Body.String())
	}
}

func TestHandlerWithNilRegistryStillServes(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()

	Handler(nil).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want %d", rec.Code, http.StatusOK)
	}
	if body := rec.Body.String(); body != "" {
		t.Fatalf("expected empty body, got %q", body)
	}
}

func TestEncodePrometheusNilSnapshot(t *testing.T) {
	var out bytes.Buffer
	if err := EncodePrometheus(&out, nil); err != nil {
		t.Fatalf("encode failed: %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("expected empty output")
	}
}

func TestEncodePrometheusBytesHistogram(t *testing.T) {
	reg := NewRegistry()
	hist := reg.MustHistogram(Descriptor{
		Name: "cache.value.size",
		Help: "Cache value size.",
		Kind: KindHistogram,
		Unit: UnitBytes,
	}, []int64{128, 512})
	hist.Observe(64)
	hist.Observe(256)

	var out bytes.Buffer
	if err := EncodePrometheus(&out, reg.Snapshot()); err != nil {
		t.Fatalf("encode failed: %v", err)
	}

	text := out.String()
	if !strings.Contains(text, `cache_value_size_bytes_bucket{le="128"} 1`) {
		t.Fatalf("missing bytes bucket:\n%s", text)
	}
	if !strings.Contains(text, "cache_value_size_bytes_sum 320") {
		t.Fatalf("missing bytes sum:\n%s", text)
	}
}

func TestEncodePrometheusWriterError(t *testing.T) {
	reg := NewRegistry()
	reg.MustCounter(Descriptor{
		Name: "http.requests",
		Help: "HTTP requests.",
		Kind: KindCounter,
	}).Inc()

	err := EncodePrometheus(failingWriter{}, reg.Snapshot())
	if err == nil {
		t.Fatalf("expected writer error")
	}
}

func TestNormalizePrometheusNameEdgeCases(t *testing.T) {
	reg := NewRegistry()
	reg.MustGauge(Descriptor{
		Name: "123 !!!",
		Help: "Odd name.",
		Kind: KindGauge,
	}).Set(7)

	var out bytes.Buffer
	if err := EncodePrometheus(&out, reg.Snapshot()); err != nil {
		t.Fatalf("encode failed: %v", err)
	}
	if !strings.Contains(out.String(), "# TYPE _123 gauge") {
		t.Fatalf("missing normalized numeric-leading name:\n%s", out.String())
	}
}

func TestEncodePrometheusAppendsCounterSuffixWhenKindDefaulted(t *testing.T) {
	reg := NewRegistry()
	counter, err := reg.Counter(Descriptor{
		Name: "http.requests",
		Help: "HTTP requests.",
	})
	if err != nil {
		t.Fatalf("counter registration failed: %v", err)
	}
	counter.Inc()

	var out bytes.Buffer
	if err := EncodePrometheus(&out, reg.Snapshot()); err != nil {
		t.Fatalf("encode failed: %v", err)
	}
	if !strings.Contains(out.String(), "# TYPE http_requests_total counter") {
		t.Fatalf("missing counter suffix:\n%s", out.String())
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}
