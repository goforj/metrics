package metrics

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// TestCounterVecRegistrationAndEncoding verifies labeled counter families share metadata and retain distinct values.
func TestCounterVecRegistrationAndEncoding(t *testing.T) {
	reg := NewRegistry()
	vec := reg.MustCounterVec(Descriptor{
		Name: "http.requests.by_route",
		Help: "Requests by route.",
		Kind: KindCounter,
	}, []string{"method", "route", "status"})

	vec.WithLabelValues("GET", "/users/:id", "200").Inc()
	vec.WithLabelValues("GET", "/users/:id", "200").Inc()
	vec.WithLabelValues("POST", "/users", "201").Inc()

	var out bytes.Buffer
	if err := EncodePrometheus(&out, reg.Snapshot()); err != nil {
		t.Fatalf("encode failed: %v", err)
	}

	text := out.String()
	if strings.Count(text, "# TYPE http_requests_by_route_total counter") != 1 {
		t.Fatalf("expected one type line:\n%s", text)
	}
	if !strings.Contains(text, `http_requests_by_route_total{method="GET",route="/users/:id",status="200"} 2`) {
		t.Fatalf("missing GET series:\n%s", text)
	}
	if !strings.Contains(text, `http_requests_by_route_total{method="POST",route="/users",status="201"} 1`) {
		t.Fatalf("missing POST series:\n%s", text)
	}
}

// TestHistogramVecRegistrationAndEncoding verifies duration buckets include fixed labels in exposition.
func TestHistogramVecRegistrationAndEncoding(t *testing.T) {
	reg := NewRegistry()
	vec, err := reg.DurationHistogramVec(Descriptor{
		Name: "http.request.duration.by_route",
		Help: "Request duration by route.",
		Kind: KindHistogram,
	}, []string{"method", "route"}, []time.Duration{100 * time.Millisecond, time.Second})
	if err != nil {
		t.Fatalf("register histogram vec: %v", err)
	}

	vec.WithLabelValues("GET", "/users/:id").ObserveDuration(50 * time.Millisecond)
	vec.WithLabelValues("GET", "/users/:id").ObserveDuration(500 * time.Millisecond)

	var out bytes.Buffer
	if err := EncodePrometheus(&out, reg.Snapshot()); err != nil {
		t.Fatalf("encode failed: %v", err)
	}

	text := out.String()
	if strings.Count(text, "# TYPE http_request_duration_by_route_seconds histogram") != 1 {
		t.Fatalf("expected one type line:\n%s", text)
	}
	if !strings.Contains(text, `http_request_duration_by_route_seconds_bucket{method="GET",route="/users/:id",le="0.1"} 1`) {
		t.Fatalf("missing first bucket:\n%s", text)
	}
	if !strings.Contains(text, `http_request_duration_by_route_seconds_bucket{method="GET",route="/users/:id",le="1"} 2`) {
		t.Fatalf("missing second bucket:\n%s", text)
	}
	if !strings.Contains(text, `http_request_duration_by_route_seconds_count{method="GET",route="/users/:id"} 2`) {
		t.Fatalf("missing count:\n%s", text)
	}
}

// TestGaugeVecRegistrationAndEncoding verifies labeled gauges support signed arithmetic and stable ordering.
func TestGaugeVecRegistrationAndEncoding(t *testing.T) {
	reg := NewRegistry()
	vec := reg.MustGaugeVec(Descriptor{
		Name: "queue.jobs.inflight",
		Help: "In-flight jobs by queue.",
		Kind: KindGauge,
	}, []string{"queue"})

	vec.WithLabelValues("default").Add(2)
	vec.WithLabelValues("default").Sub(1)
	vec.WithLabelValues("critical").Set(3)

	var out bytes.Buffer
	if err := EncodePrometheus(&out, reg.Snapshot()); err != nil {
		t.Fatalf("encode failed: %v", err)
	}

	text := out.String()
	if strings.Count(text, "# TYPE queue_jobs_inflight gauge") != 1 {
		t.Fatalf("expected one type line:\n%s", text)
	}
	if !strings.Contains(text, `queue_jobs_inflight{queue="critical"} 3`) {
		t.Fatalf("missing critical series:\n%s", text)
	}
	if !strings.Contains(text, `queue_jobs_inflight{queue="default"} 1`) {
		t.Fatalf("missing default series:\n%s", text)
	}
}

// TestVectorRegistrationRejectsConflicts verifies vector schemas participate in exact registration identity.
func TestVectorRegistrationRejectsConflicts(t *testing.T) {
	reg := NewRegistry()
	if _, err := reg.CounterVec(Descriptor{
		Name: "http.requests.by_route",
		Help: "Requests by route.",
		Kind: KindCounter,
	}, []string{"method", "route", "status"}); err != nil {
		t.Fatalf("register counter vec: %v", err)
	}
	if _, err := reg.CounterVec(Descriptor{
		Name: "http.requests.by_route",
		Help: "Requests by route.",
		Kind: KindCounter,
	}, []string{"method", "route"}); err == nil {
		t.Fatalf("expected label key conflict")
	}
	if _, err := reg.Counter(Descriptor{
		Name: "http.requests.by_route",
		Help: "Requests by route.",
		Kind: KindCounter,
	}); err == nil {
		t.Fatalf("expected scalar/vector conflict")
	}
	if _, err := reg.GaugeVec(Descriptor{
		Name: "http.requests.by_route",
		Help: "Requests by route.",
		Kind: KindGauge,
	}, []string{"queue"}); err == nil {
		t.Fatalf("expected cross-kind vector conflict")
	}
}
