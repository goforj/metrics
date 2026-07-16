package metrics

import "testing"

// TestCounterDescriptor verifies counters retain canonical registration metadata.
func TestCounterDescriptor(t *testing.T) {
	reg := NewRegistry()
	desc := Descriptor{Name: "requests", Help: "Requests.", Kind: KindCounter}
	counter := reg.MustCounter(desc)
	if got := counter.Descriptor(); got != desc {
		t.Fatalf("descriptor: got %#v want %#v", got, desc)
	}
}

// TestGaugeDescriptorAndArithmetic verifies signed gauge updates and metadata access.
func TestGaugeDescriptorAndArithmetic(t *testing.T) {
	reg := NewRegistry()
	desc := Descriptor{Name: "jobs.inflight", Help: "In-flight jobs.", Kind: KindGauge}
	gauge := reg.MustGauge(desc)
	gauge.Add(5)
	gauge.Sub(2)
	if got := gauge.Value(); got != 3 {
		t.Fatalf("value: got %d want 3", got)
	}
	if got := gauge.Descriptor(); got != desc {
		t.Fatalf("descriptor: got %#v want %#v", got, desc)
	}
}

// TestHistogramDescriptorAndBoundsCopy verifies histogram metadata cannot be mutated through accessors.
func TestHistogramDescriptorAndBoundsCopy(t *testing.T) {
	reg := NewRegistry()
	desc := Descriptor{Name: "request.duration", Help: "Request duration.", Kind: KindHistogram}
	hist := reg.MustHistogram(desc, []int64{10, 20})
	bounds := hist.Bounds()
	bounds[0] = 999
	if got := hist.Bounds()[0]; got != 10 {
		t.Fatalf("expected bounds copy, got %d", got)
	}
	if got := hist.Descriptor(); got != desc {
		t.Fatalf("descriptor: got %#v want %#v", got, desc)
	}
}
