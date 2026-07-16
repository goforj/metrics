package metrics

import (
	"strings"
	"testing"
	"time"
)

// TestDescriptorValidationRejectsUnsafeMetadata covers every descriptor field that reaches exposition.
func TestDescriptorValidationRejectsUnsafeMetadata(t *testing.T) {
	tests := []struct {
		name string
		desc Descriptor
		want string
	}{
		{name: "empty name", desc: Descriptor{Help: "Help."}, want: "name is required"},
		{name: "whitespace name", desc: Descriptor{Name: "   ", Help: "Help."}, want: "name is required"},
		{name: "surrounding whitespace", desc: Descriptor{Name: " requests ", Help: "Help."}, want: "surrounding whitespace"},
		{name: "invalid name UTF-8", desc: Descriptor{Name: string([]byte{0xff}), Help: "Help."}, want: "name must be valid UTF-8"},
		{name: "meaningless name", desc: Descriptor{Name: "!!!", Help: "Help."}, want: "has no letters or digits"},
		{name: "empty help", desc: Descriptor{Name: "requests"}, want: "help is required"},
		{name: "whitespace help", desc: Descriptor{Name: "requests", Help: "\t"}, want: "help is required"},
		{name: "invalid help UTF-8", desc: Descriptor{Name: "requests", Help: string([]byte{0xff})}, want: "must be valid UTF-8"},
		{name: "kind mismatch", desc: Descriptor{Name: "requests", Help: "Help.", Kind: KindGauge}, want: "kind mismatch"},
		{name: "unsupported unit", desc: Descriptor{Name: "requests", Help: "Help.", Unit: Unit("widgets")}, want: "unsupported unit"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewRegistry().Counter(test.desc)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error: got %v, want substring %q", err, test.want)
			}
		})
	}
}

// TestRegistryZeroValueSupportsEveryMetricKind verifies lazy initialization is complete, not constructor-dependent.
func TestRegistryZeroValueSupportsEveryMetricKind(t *testing.T) {
	var registry Registry
	registry.MustCounter(Descriptor{Name: "requests", Help: "Requests."}).Inc()
	registry.MustCounterVec(Descriptor{Name: "requests.by_method", Help: "Requests by method."}, []string{"method"}).WithLabelValues("GET").Inc()
	registry.MustGauge(Descriptor{Name: "workers", Help: "Workers."}).Set(2)
	registry.MustGaugeVec(Descriptor{Name: "workers.by_pool", Help: "Workers by pool."}, []string{"pool"}).WithLabelValues("main").Set(1)
	registry.MustHistogram(Descriptor{Name: "sizes", Help: "Sizes."}, []int64{10}).Observe(4)
	registry.MustHistogramVec(Descriptor{Name: "sizes.by_type", Help: "Sizes by type."}, []string{"type"}, []int64{10}).WithLabelValues("small").Observe(4)

	snapshot := registry.Snapshot()
	if len(snapshot.Counters) != 2 || len(snapshot.Gauges) != 2 || len(snapshot.Histograms) != 2 {
		t.Fatalf("snapshot sizes: got counters=%d gauges=%d histograms=%d", len(snapshot.Counters), len(snapshot.Gauges), len(snapshot.Histograms))
	}
}

// TestLabelKeyValidationRejectsAmbiguousSchemas exercises the full legacy label-name contract.
func TestLabelKeyValidationRejectsAmbiguousSchemas(t *testing.T) {
	tests := []struct {
		name string
		keys []string
		want string
	}{
		{name: "missing", keys: nil, want: "at least one"},
		{name: "surrounding whitespace", keys: []string{" method"}, want: "surrounding whitespace"},
		{name: "empty", keys: []string{""}, want: "key is required"},
		{name: "reserved", keys: []string{"__name__"}, want: "reserved"},
		{name: "invalid first rune", keys: []string{"1method"}, want: "invalid label key"},
		{name: "invalid later rune", keys: []string{"http-method"}, want: "invalid label key"},
		{name: "duplicate", keys: []string{"method", "method"}, want: "duplicate"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewRegistry().CounterVec(Descriptor{Name: "requests", Help: "Requests."}, test.keys)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error: got %v, want substring %q", err, test.want)
			}
		})
	}

	if _, err := NewRegistry().HistogramVec(Descriptor{Name: "latency", Help: "Latency."}, []string{"le"}, []int64{1}); err == nil {
		t.Fatal("expected histogram le label rejection")
	}
	if _, err := NewRegistry().CounterVec(Descriptor{Name: "requests", Help: "Requests."}, []string{"Method_2"}); err != nil {
		t.Fatalf("valid label key rejected: %v", err)
	}
}

// TestVectorLabelValueValidationPanicsAtFirstCreation checks invalid UTF-8 and arity across vector kinds.
func TestVectorLabelValueValidationPanicsAtFirstCreation(t *testing.T) {
	invalid := string([]byte{0xff})
	registry := NewRegistry()
	counter := registry.MustCounterVec(Descriptor{Name: "counter.labels", Help: "Counter labels."}, []string{"key"})
	gauge := registry.MustGaugeVec(Descriptor{Name: "gauge.labels", Help: "Gauge labels."}, []string{"key"})
	histogram := registry.MustHistogramVec(Descriptor{Name: "histogram.labels", Help: "Histogram labels."}, []string{"key"}, []int64{1})

	for name, call := range map[string]func(){
		"counter invalid value":   func() { counter.WithLabelValues(invalid) },
		"gauge invalid value":     func() { gauge.WithLabelValues(invalid) },
		"histogram invalid value": func() { histogram.WithLabelValues(invalid) },
		"counter arity":           func() { counter.WithLabelValues() },
		"gauge arity":             func() { gauge.WithLabelValues() },
		"histogram arity":         func() { histogram.WithLabelValues() },
	} {
		t.Run(name, func(t *testing.T) {
			assertPanics(t, call)
		})
	}
}

// TestNilVectorReceiversPanic keeps required metric handles fail-fast and consistent.
func TestNilVectorReceiversPanic(t *testing.T) {
	var counter *CounterVec
	var gauge *GaugeVec
	var histogram *HistogramVec

	for name, call := range map[string]func(){
		"counter":   func() { counter.WithLabelValues("value") },
		"gauge":     func() { gauge.WithLabelValues("value") },
		"histogram": func() { histogram.WithLabelValues("value") },
	} {
		t.Run(name, func(t *testing.T) {
			assertPanics(t, call)
		})
	}
}

// TestDurationHistogramHelpersEnforceSecondsUnit verifies the safe duration path and its Must variants.
func TestDurationHistogramHelpersEnforceSecondsUnit(t *testing.T) {
	registry := NewRegistry()
	histogram := registry.MustDurationHistogram(
		Descriptor{Name: "request.duration", Help: "Request duration."},
		[]time.Duration{time.Millisecond},
	)
	histogram.ObserveDuration(500 * time.Microsecond)
	if histogram.Descriptor().Unit != UnitSeconds || histogram.Count() != 1 || histogram.Sum() != float64(500*time.Microsecond) {
		t.Fatalf("duration histogram state: descriptor=%#v count=%d sum=%v", histogram.Descriptor(), histogram.Count(), histogram.Sum())
	}

	vector := registry.MustDurationHistogramVec(
		Descriptor{Name: "request.duration.by_route", Help: "Request duration by route."},
		[]string{"route"},
		[]time.Duration{time.Millisecond},
	)
	vector.WithLabelValues("/users").ObserveDuration(time.Millisecond)

	if _, err := NewRegistry().DurationHistogram(Descriptor{Name: "bad.duration", Help: "Bad duration.", Unit: UnitBytes}, nil); err == nil {
		t.Fatal("expected conflicting scalar duration unit error")
	}
	if _, err := NewRegistry().DurationHistogramVec(Descriptor{Name: "bad.duration", Help: "Bad duration.", Unit: UnitItems}, []string{"route"}, nil); err == nil {
		t.Fatal("expected conflicting vector duration unit error")
	}
	assertPanics(t, func() {
		NewRegistry().MustHistogram(Descriptor{Name: "sizes", Help: "Sizes.", Unit: UnitBytes}, nil).ObserveDuration(time.Second)
	})
}

// TestAllMustHelpersPanicOnInvalidRegistration verifies every convenience method preserves registration errors.
func TestAllMustHelpersPanicOnInvalidRegistration(t *testing.T) {
	for name, call := range map[string]func(){
		"counter vector":     func() { NewRegistry().MustCounterVec(Descriptor{}, []string{"key"}) },
		"gauge vector":       func() { NewRegistry().MustGaugeVec(Descriptor{}, []string{"key"}) },
		"histogram vector":   func() { NewRegistry().MustHistogramVec(Descriptor{}, []string{"key"}, nil) },
		"duration histogram": func() { NewRegistry().MustDurationHistogram(Descriptor{}, nil) },
		"duration vector":    func() { NewRegistry().MustDurationHistogramVec(Descriptor{}, []string{"key"}, nil) },
	} {
		t.Run(name, func(t *testing.T) {
			assertPanics(t, call)
		})
	}
}

// TestLabelComparisonDefinesTotalOrder protects deterministic snapshot sorting for all key/value and length cases.
func TestLabelComparisonDefinesTotalOrder(t *testing.T) {
	tests := []struct {
		name  string
		left  []Label
		right []Label
		want  int
	}{
		{name: "equal", left: []Label{{Key: "a", Value: "1"}}, right: []Label{{Key: "a", Value: "1"}}, want: 0},
		{name: "key before", left: []Label{{Key: "a"}}, right: []Label{{Key: "b"}}, want: -1},
		{name: "key after", left: []Label{{Key: "b"}}, right: []Label{{Key: "a"}}, want: 1},
		{name: "value before", left: []Label{{Key: "a", Value: "1"}}, right: []Label{{Key: "a", Value: "2"}}, want: -1},
		{name: "value after", left: []Label{{Key: "a", Value: "2"}}, right: []Label{{Key: "a", Value: "1"}}, want: 1},
		{name: "shorter", left: nil, right: []Label{{Key: "a"}}, want: -1},
		{name: "longer", left: []Label{{Key: "a"}}, right: nil, want: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := compareLabels(test.left, test.right); got != test.want {
				t.Fatalf("comparison: got %d want %d", got, test.want)
			}
		})
	}
}
