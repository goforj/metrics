package metrics

import "testing"

func TestRegistryCounterRegistrationIsIdempotent(t *testing.T) {
	reg := NewRegistry()
	desc := Descriptor{
		Name: "http.requests.total",
		Help: "Total requests.",
		Kind: KindCounter,
	}

	first, err := reg.Counter(desc)
	if err != nil {
		t.Fatalf("first registration failed: %v", err)
	}
	second, err := reg.Counter(desc)
	if err != nil {
		t.Fatalf("second registration failed: %v", err)
	}
	if first != second {
		t.Fatalf("expected same counter instance")
	}
}

func TestRegistryRejectsConflictingRegistration(t *testing.T) {
	reg := NewRegistry()

	_, err := reg.Counter(Descriptor{
		Name: "http.requests.total",
		Help: "Total requests.",
		Kind: KindCounter,
	})
	if err != nil {
		t.Fatalf("counter registration failed: %v", err)
	}

	_, err = reg.Gauge(Descriptor{
		Name: "http.requests.total",
		Help: "Current requests.",
		Kind: KindGauge,
	})
	if err == nil {
		t.Fatalf("expected conflicting registration error")
	}
}

func TestRegistryRejectsConflictingHistogramBounds(t *testing.T) {
	reg := NewRegistry()
	desc := Descriptor{
		Name: "http.request.duration",
		Help: "Request duration.",
		Kind: KindHistogram,
		Unit: UnitSeconds,
	}

	_, err := reg.Histogram(desc, []int64{10, 20})
	if err != nil {
		t.Fatalf("histogram registration failed: %v", err)
	}
	_, err = reg.Histogram(desc, []int64{10, 30})
	if err == nil {
		t.Fatalf("expected conflicting histogram registration error")
	}
}
