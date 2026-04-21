package metrics

import (
	"bytes"
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
