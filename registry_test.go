package metrics

import "testing"

// TestRegistryCounterRegistrationIsIdempotent verifies exact startup repeats reuse the counter handle.
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

// TestRegistryRejectsConflictingRegistration verifies one internal name cannot change metric kind.
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

// TestRegistryRejectsConflictingHistogramBounds verifies bucket layouts are part of histogram identity.
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

// TestRegistryGaugeRegistrationIsIdempotent verifies exact startup repeats reuse the gauge handle.
func TestRegistryGaugeRegistrationIsIdempotent(t *testing.T) {
	reg := NewRegistry()
	desc := Descriptor{Name: "jobs.inflight", Help: "In-flight jobs.", Kind: KindGauge}
	first, err := reg.Gauge(desc)
	if err != nil {
		t.Fatalf("first registration failed: %v", err)
	}
	second, err := reg.Gauge(desc)
	if err != nil {
		t.Fatalf("second registration failed: %v", err)
	}
	if first != second {
		t.Fatalf("expected same gauge instance")
	}
}

// TestRegistryRejectsConflictingCounterDescriptor verifies help text is stable family metadata.
func TestRegistryRejectsConflictingCounterDescriptor(t *testing.T) {
	reg := NewRegistry()
	if _, err := reg.Counter(Descriptor{Name: "http.requests", Help: "one", Kind: KindCounter}); err != nil {
		t.Fatalf("counter registration failed: %v", err)
	}
	if _, err := reg.Counter(Descriptor{Name: "http.requests", Help: "two", Kind: KindCounter}); err == nil {
		t.Fatalf("expected conflicting counter registration")
	}
}

// TestRegistryRejectsHistogramWhenNameUsedByGauge verifies scalar kinds share the raw-name namespace.
func TestRegistryRejectsHistogramWhenNameUsedByGauge(t *testing.T) {
	reg := NewRegistry()
	if _, err := reg.Gauge(Descriptor{Name: "queue.depth", Help: "Queue depth.", Kind: KindGauge}); err != nil {
		t.Fatalf("gauge registration failed: %v", err)
	}
	if _, err := reg.Histogram(Descriptor{Name: "queue.depth", Help: "Queue depth histogram.", Kind: KindHistogram}, []int64{1}); err == nil {
		t.Fatalf("expected conflicting histogram registration")
	}
}

// TestRegistrySnapshotIsSorted verifies deterministic family ordering across metric kinds.
func TestRegistrySnapshotIsSorted(t *testing.T) {
	reg := NewRegistry()
	reg.MustGauge(Descriptor{Name: "zeta", Help: "zeta", Kind: KindGauge})
	reg.MustCounter(Descriptor{Name: "alpha", Help: "alpha", Kind: KindCounter})
	reg.MustHistogram(Descriptor{Name: "middle", Help: "middle", Kind: KindHistogram}, []int64{1})

	snap := reg.Snapshot()
	if got := snap.Counters[0].Descriptor.Name; got != "alpha" {
		t.Fatalf("counter order: got %q", got)
	}
	if got := snap.Gauges[0].Descriptor.Name; got != "zeta" {
		t.Fatalf("gauge order: got %q", got)
	}
	if got := snap.Histograms[0].Descriptor.Name; got != "middle" {
		t.Fatalf("histogram order: got %q", got)
	}
}

// TestRegistryRejectsMissingName verifies descriptors cannot create anonymous families.
func TestRegistryRejectsMissingName(t *testing.T) {
	reg := NewRegistry()
	if _, err := reg.Counter(Descriptor{Kind: KindCounter}); err == nil {
		t.Fatalf("expected missing name error")
	}
}

// TestRegistryRejectsKindMismatch verifies explicit kinds must agree with registration methods.
func TestRegistryRejectsKindMismatch(t *testing.T) {
	reg := NewRegistry()
	if _, err := reg.Counter(Descriptor{Name: "x", Kind: KindGauge}); err == nil {
		t.Fatalf("expected kind mismatch error")
	}
}

// TestRegistryDefaultsDescriptorKind verifies registration methods own omitted kind metadata.
func TestRegistryDefaultsDescriptorKind(t *testing.T) {
	reg := NewRegistry()
	counter, err := reg.Counter(Descriptor{Name: "http.requests", Help: "HTTP requests."})
	if err != nil {
		t.Fatalf("counter registration failed: %v", err)
	}
	if got := counter.Descriptor().Kind; got != KindCounter {
		t.Fatalf("kind: got %q want %q", got, KindCounter)
	}
}

// TestMustHelpersPanicOnInvalidDescriptor verifies scalar Must helpers fail during startup registration.
func TestMustHelpersPanicOnInvalidDescriptor(t *testing.T) {
	reg := NewRegistry()

	assertPanics(t, func() {
		reg.MustCounter(Descriptor{})
	})
	assertPanics(t, func() {
		reg.MustGauge(Descriptor{})
	})
	assertPanics(t, func() {
		reg.MustHistogram(Descriptor{}, []int64{1})
	})
}

// assertPanics keeps fail-fast contract assertions concise across metric types.
func assertPanics(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatalf("expected panic")
		}
	}()
	fn()
}
