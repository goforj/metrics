package metrics

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Registry owns concurrent metric registration and snapshot collection.
// Its zero value is ready to use, though NewRegistry is preferred for clarity.
type Registry struct {
	mu            sync.RWMutex
	counters      map[string]*Counter
	counterVecs   map[string]*CounterVec
	gauges        map[string]*Gauge
	gaugeVecs     map[string]*GaugeVec
	histograms    map[string]*Histogram
	histogramVecs map[string]*HistogramVec
	seriesOwners  map[string]string
}

// NewRegistry creates an empty metric registry.
func NewRegistry() *Registry {
	return &Registry{
		counters:      map[string]*Counter{},
		counterVecs:   map[string]*CounterVec{},
		gauges:        map[string]*Gauge{},
		gaugeVecs:     map[string]*GaugeVec{},
		histograms:    map[string]*Histogram{},
		histogramVecs: map[string]*HistogramVec{},
		seriesOwners:  map[string]string{},
	}
}

// Counter registers or returns an existing counter.
func (r *Registry) Counter(desc Descriptor) (*Counter, error) {
	var err error
	desc, err = canonicalizeDescriptor(desc, KindCounter)
	if err != nil {
		return nil, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if metric := r.counters[desc.Name]; metric != nil {
		if sameDescriptor(metric.desc, desc) {
			return metric, nil
		}
		return nil, conflictingMetricError(desc.Name)
	}
	if err := r.reserveRegistration(desc); err != nil {
		return nil, err
	}

	metric := &Counter{desc: desc}
	r.counters[desc.Name] = metric
	return metric, nil
}

// MustCounter registers a counter or panics.
func (r *Registry) MustCounter(desc Descriptor) *Counter {
	metric, err := r.Counter(desc)
	if err != nil {
		panic(err)
	}
	return metric
}

// Gauge registers or returns an existing gauge.
func (r *Registry) Gauge(desc Descriptor) (*Gauge, error) {
	var err error
	desc, err = canonicalizeDescriptor(desc, KindGauge)
	if err != nil {
		return nil, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if metric := r.gauges[desc.Name]; metric != nil {
		if sameDescriptor(metric.desc, desc) {
			return metric, nil
		}
		return nil, conflictingMetricError(desc.Name)
	}
	if err := r.reserveRegistration(desc); err != nil {
		return nil, err
	}

	metric := &Gauge{desc: desc}
	r.gauges[desc.Name] = metric
	return metric, nil
}

// MustGauge registers a gauge or panics.
func (r *Registry) MustGauge(desc Descriptor) *Gauge {
	metric, err := r.Gauge(desc)
	if err != nil {
		panic(err)
	}
	return metric
}

// Histogram registers or returns an existing histogram.
func (r *Registry) Histogram(desc Descriptor, bounds []int64) (*Histogram, error) {
	var err error
	desc, err = canonicalizeDescriptor(desc, KindHistogram)
	if err != nil {
		return nil, err
	}
	if err := validateBounds(bounds); err != nil {
		return nil, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if metric := r.histograms[desc.Name]; metric != nil {
		if sameDescriptor(metric.desc, desc) && sameBounds(metric.bounds, bounds) {
			return metric, nil
		}
		return nil, conflictingMetricError(desc.Name)
	}
	if err := r.reserveRegistration(desc); err != nil {
		return nil, err
	}

	metric := newHistogram(desc, nil, bounds)
	r.histograms[desc.Name] = metric
	return metric, nil
}

// DurationHistogram registers or returns an existing duration histogram.
func (r *Registry) DurationHistogram(desc Descriptor, bounds []time.Duration) (*Histogram, error) {
	var err error
	desc, err = durationDescriptor(desc)
	if err != nil {
		return nil, err
	}
	return r.Histogram(desc, durationBounds(bounds))
}

// MustDurationHistogram registers a duration histogram or panics.
func (r *Registry) MustDurationHistogram(desc Descriptor, bounds []time.Duration) *Histogram {
	metric, err := r.DurationHistogram(desc, bounds)
	if err != nil {
		panic(err)
	}
	return metric
}

// MustHistogram registers a histogram or panics.
func (r *Registry) MustHistogram(desc Descriptor, bounds []int64) *Histogram {
	metric, err := r.Histogram(desc, bounds)
	if err != nil {
		panic(err)
	}
	return metric
}

// Snapshot collects a detached, deterministically ordered copy of registry state.
func (r *Registry) Snapshot() *Snapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()

	snap := &Snapshot{
		Counters:   make([]CounterSnapshot, 0, len(r.counters)),
		Gauges:     make([]GaugeSnapshot, 0, len(r.gauges)),
		Histograms: make([]HistogramSnapshot, 0, len(r.histograms)),
	}

	for _, metric := range r.counters {
		snap.Counters = append(snap.Counters, CounterSnapshot{
			Descriptor: metric.desc,
			Labels:     append([]Label(nil), metric.labels...),
			Value:      metric.Value(),
		})
	}
	for _, family := range r.counterVecs {
		family.mu.RLock()
		for _, metric := range family.metrics {
			snap.Counters = append(snap.Counters, CounterSnapshot{
				Descriptor: metric.desc,
				Labels:     append([]Label(nil), metric.labels...),
				Value:      metric.Value(),
			})
		}
		family.mu.RUnlock()
	}
	for _, metric := range r.gauges {
		snap.Gauges = append(snap.Gauges, GaugeSnapshot{
			Descriptor: metric.desc,
			Labels:     append([]Label(nil), metric.labels...),
			Value:      metric.Value(),
		})
	}
	for _, family := range r.gaugeVecs {
		family.mu.RLock()
		for _, metric := range family.metrics {
			snap.Gauges = append(snap.Gauges, GaugeSnapshot{
				Descriptor: metric.desc,
				Labels:     append([]Label(nil), metric.labels...),
				Value:      metric.Value(),
			})
		}
		family.mu.RUnlock()
	}
	for _, metric := range r.histograms {
		snap.Histograms = append(snap.Histograms, snapshotHistogram(metric))
	}
	for _, family := range r.histogramVecs {
		family.mu.RLock()
		for _, metric := range family.metrics {
			snap.Histograms = append(snap.Histograms, snapshotHistogram(metric))
		}
		family.mu.RUnlock()
	}

	sort.Slice(snap.Counters, func(i, j int) bool {
		if snap.Counters[i].Descriptor.Name != snap.Counters[j].Descriptor.Name {
			return snap.Counters[i].Descriptor.Name < snap.Counters[j].Descriptor.Name
		}
		return compareLabels(snap.Counters[i].Labels, snap.Counters[j].Labels) < 0
	})
	sort.Slice(snap.Gauges, func(i, j int) bool {
		if snap.Gauges[i].Descriptor.Name != snap.Gauges[j].Descriptor.Name {
			return snap.Gauges[i].Descriptor.Name < snap.Gauges[j].Descriptor.Name
		}
		return compareLabels(snap.Gauges[i].Labels, snap.Gauges[j].Labels) < 0
	})
	sort.Slice(snap.Histograms, func(i, j int) bool {
		if snap.Histograms[i].Descriptor.Name != snap.Histograms[j].Descriptor.Name {
			return snap.Histograms[i].Descriptor.Name < snap.Histograms[j].Descriptor.Name
		}
		return compareLabels(snap.Histograms[i].Labels, snap.Histograms[j].Labels) < 0
	})

	return snap
}

// snapshotHistogram copies one coherent hot/cold histogram generation for exposition.
func snapshotHistogram(metric *Histogram) HistogramSnapshot {
	bounds := append([]int64(nil), metric.bounds...)
	buckets, count, sum := metric.snapshotState()
	return HistogramSnapshot{
		Descriptor:   metric.desc,
		Labels:       append([]Label(nil), metric.labels...),
		Bounds:       bounds,
		BucketCounts: buckets,
		Count:        count,
		Sum:          sum,
	}
}

// durationDescriptor enforces seconds as the only valid exported unit for duration helpers.
func durationDescriptor(desc Descriptor) (Descriptor, error) {
	if desc.Unit != UnitNone && desc.Unit != UnitSeconds {
		return Descriptor{}, fmt.Errorf("metrics: duration histogram %q cannot use unit %q", desc.Name, desc.Unit)
	}
	desc.Unit = UnitSeconds
	return desc, nil
}

// canonicalizeDescriptor fills method-owned metadata and rejects descriptors that cannot be exported safely.
func canonicalizeDescriptor(desc Descriptor, kind Kind) (Descriptor, error) {
	if strings.TrimSpace(desc.Name) == "" {
		return Descriptor{}, errors.New("metrics: descriptor name is required")
	}
	if desc.Name != strings.TrimSpace(desc.Name) {
		return Descriptor{}, fmt.Errorf("metrics: descriptor name %q has surrounding whitespace", desc.Name)
	}
	if !utf8.ValidString(desc.Name) {
		return Descriptor{}, errors.New("metrics: descriptor name must be valid UTF-8")
	}
	if normalizePrometheusName(desc.Name) == "_" {
		return Descriptor{}, fmt.Errorf("metrics: descriptor name %q has no letters or digits", desc.Name)
	}
	if strings.TrimSpace(desc.Help) == "" {
		return Descriptor{}, fmt.Errorf("metrics: descriptor help is required for %q", desc.Name)
	}
	if !utf8.ValidString(desc.Help) {
		return Descriptor{}, fmt.Errorf("metrics: descriptor help for %q must be valid UTF-8", desc.Name)
	}
	if desc.Kind == "" {
		desc.Kind = kind
	}
	if desc.Kind != kind {
		return Descriptor{}, fmt.Errorf("metrics: descriptor kind mismatch for %q", desc.Name)
	}
	switch desc.Unit {
	case UnitNone, UnitSeconds, UnitBytes, UnitItems:
	default:
		return Descriptor{}, fmt.Errorf("metrics: unsupported unit %q for %q", desc.Unit, desc.Name)
	}
	return desc, nil
}

// sameDescriptor reports whether repeat registration is exactly idempotent.
func sameDescriptor(a, b Descriptor) bool {
	return a.Name == b.Name && a.Help == b.Help && a.Unit == b.Unit && a.Kind == b.Kind
}

// sameBounds reports whether repeat histogram registration uses the same buckets.
func sameBounds(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// reserveRegistration rejects raw-name and normalized-series collisions before mutating registry state.
func (r *Registry) reserveRegistration(desc Descriptor) error {
	r.initialize()
	if r.metricNameRegistered(desc.Name) {
		return conflictingMetricError(desc.Name)
	}

	series := prometheusSeriesNames(desc)
	for _, name := range series {
		if owner, exists := r.seriesOwners[name]; exists {
			return fmt.Errorf("metrics: Prometheus series %q for %q conflicts with metric %q", name, desc.Name, owner)
		}
	}
	for _, name := range series {
		r.seriesOwners[name] = desc.Name
	}
	return nil
}

// initialize makes the Registry zero value usable without weakening explicit constructor wiring.
func (r *Registry) initialize() {
	if r.counters == nil {
		r.counters = map[string]*Counter{}
	}
	if r.counterVecs == nil {
		r.counterVecs = map[string]*CounterVec{}
	}
	if r.gauges == nil {
		r.gauges = map[string]*Gauge{}
	}
	if r.gaugeVecs == nil {
		r.gaugeVecs = map[string]*GaugeVec{}
	}
	if r.histograms == nil {
		r.histograms = map[string]*Histogram{}
	}
	if r.histogramVecs == nil {
		r.histogramVecs = map[string]*HistogramVec{}
	}
	if r.seriesOwners == nil {
		r.seriesOwners = map[string]string{}
	}
}

// metricNameRegistered reports whether any scalar or vector already owns an internal name.
func (r *Registry) metricNameRegistered(name string) bool {
	return r.counters[name] != nil ||
		r.counterVecs[name] != nil ||
		r.gauges[name] != nil ||
		r.gaugeVecs[name] != nil ||
		r.histograms[name] != nil ||
		r.histogramVecs[name] != nil
}

// conflictingMetricError describes a non-idempotent registration using an existing internal name.
func conflictingMetricError(name string) error {
	return fmt.Errorf("metrics: conflicting metric registration for %q", name)
}
