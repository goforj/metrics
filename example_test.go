package metrics_test

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/goforj/metrics"
)

// ExampleRegistry shows scalar metric registration and updates.
func ExampleRegistry() {
	registry := metrics.NewRegistry()
	processed := registry.MustCounter(metrics.Descriptor{
		Name: "jobs.processed",
		Help: "Jobs processed successfully.",
	})
	inflight := registry.MustGauge(metrics.Descriptor{
		Name: "jobs.inflight",
		Help: "Jobs currently being processed.",
	})

	processed.Add(3)
	inflight.Set(5)
	inflight.Sub(1)

	fmt.Println(processed.Value(), inflight.Value())
	// Output:
	// 3 4
}

// ExampleCounterVec shows how fixed labels become Prometheus samples.
func ExampleCounterVec() {
	registry := metrics.NewRegistry()
	requests := registry.MustCounterVec(metrics.Descriptor{
		Name: "http.requests",
		Help: "HTTP requests served.",
	}, []string{"method", "status"})

	requests.WithLabelValues("GET", "200").Add(2)

	var output bytes.Buffer
	if err := metrics.EncodePrometheus(&output, registry.Snapshot()); err != nil {
		panic(err)
	}
	fmt.Print(output.String())
	// Output:
	// # HELP http_requests_total HTTP requests served.
	// # TYPE http_requests_total counter
	// http_requests_total{method="GET",status="200"} 2
}

// ExampleRegistry_MustDurationHistogram shows duration storage and seconds exposition.
func ExampleRegistry_MustDurationHistogram() {
	registry := metrics.NewRegistry()
	duration := registry.MustDurationHistogram(metrics.Descriptor{
		Name: "worker.job.duration",
		Help: "Worker job duration.",
	}, []time.Duration{50 * time.Millisecond, 100 * time.Millisecond})

	duration.ObserveDuration(75 * time.Millisecond)

	var output bytes.Buffer
	if err := metrics.EncodePrometheus(&output, registry.Snapshot()); err != nil {
		panic(err)
	}
	fmt.Print(output.String())
	// Output:
	// # HELP worker_job_duration_seconds Worker job duration.
	// # TYPE worker_job_duration_seconds histogram
	// worker_job_duration_seconds_bucket{le="0.05"} 0
	// worker_job_duration_seconds_bucket{le="0.1"} 1
	// worker_job_duration_seconds_bucket{le="+Inf"} 1
	// worker_job_duration_seconds_sum 0.075
	// worker_job_duration_seconds_count 1
}

// ExampleHandler shows a registry exposed as an HTTP scrape endpoint.
func ExampleHandler() {
	registry := metrics.NewRegistry()
	ready := registry.MustGauge(metrics.Descriptor{
		Name: "service.ready",
		Help: "Whether the service is ready.",
	})
	ready.Set(1)

	request := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	response := httptest.NewRecorder()
	metrics.Handler(registry).ServeHTTP(response, request)

	fmt.Println(response.Code)
	fmt.Println(response.Header().Get("Content-Type"))
	fmt.Print(response.Body.String())
	// Output:
	// 200
	// text/plain; version=0.0.4; charset=utf-8
	// # HELP service_ready Whether the service is ready.
	// # TYPE service_ready gauge
	// service_ready 1
}
