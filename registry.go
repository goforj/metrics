package metrics

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// Registry owns metric registration and snapshot collection.
type Registry struct {
	mu         sync.RWMutex
	counters   map[string]*Counter
	gauges     map[string]*Gauge
	histograms map[string]*Histogram
}

// NewRegistry creates an empty metric registry.
func NewRegistry() *Registry {
	return &Registry{
		counters:   map[string]*Counter{},
		gauges:     map[string]*Gauge{},
		histograms: map[string]*Histogram{},
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
	if r.gauges[desc.Name] != nil || r.histograms[desc.Name] != nil {
		return nil, conflictingMetricError(desc.Name)
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
	if r.counters[desc.Name] != nil || r.histograms[desc.Name] != nil {
		return nil, conflictingMetricError(desc.Name)
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
	if r.counters[desc.Name] != nil || r.gauges[desc.Name] != nil {
		return nil, conflictingMetricError(desc.Name)
	}

	cloned := append([]int64(nil), bounds...)
	metric := &Histogram{
		desc:    desc,
		bounds:  cloned,
		buckets: make([]atomic.Uint64, len(cloned)+1),
	}
	r.histograms[desc.Name] = metric
	return metric, nil
}

// DurationHistogram registers or returns an existing duration histogram.
func (r *Registry) DurationHistogram(desc Descriptor, bounds []time.Duration) (*Histogram, error) {
	desc.Unit = UnitSeconds
	return r.Histogram(desc, durationBounds(bounds))
}

// MustHistogram registers a histogram or panics.
func (r *Registry) MustHistogram(desc Descriptor, bounds []int64) *Histogram {
	metric, err := r.Histogram(desc, bounds)
	if err != nil {
		panic(err)
	}
	return metric
}

// Snapshot collects an immutable copy of the registry state.
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
			Value:      metric.Value(),
		})
	}
	for _, metric := range r.gauges {
		snap.Gauges = append(snap.Gauges, GaugeSnapshot{
			Descriptor: metric.desc,
			Value:      metric.Value(),
		})
	}
	for _, metric := range r.histograms {
		bounds := append([]int64(nil), metric.bounds...)
		buckets := make([]uint64, len(metric.buckets))
		for i := range metric.buckets {
			buckets[i] = metric.buckets[i].Load()
		}
		snap.Histograms = append(snap.Histograms, HistogramSnapshot{
			Descriptor:   metric.desc,
			Bounds:       bounds,
			BucketCounts: buckets,
			Count:        metric.Count(),
			Sum:          metric.Sum(),
		})
	}

	sort.Slice(snap.Counters, func(i, j int) bool {
		return snap.Counters[i].Descriptor.Name < snap.Counters[j].Descriptor.Name
	})
	sort.Slice(snap.Gauges, func(i, j int) bool {
		return snap.Gauges[i].Descriptor.Name < snap.Gauges[j].Descriptor.Name
	})
	sort.Slice(snap.Histograms, func(i, j int) bool {
		return snap.Histograms[i].Descriptor.Name < snap.Histograms[j].Descriptor.Name
	})

	return snap
}

func canonicalizeDescriptor(desc Descriptor, kind Kind) (Descriptor, error) {
	if desc.Name == "" {
		return Descriptor{}, errors.New("metrics: descriptor name is required")
	}
	if desc.Kind == "" {
		desc.Kind = kind
	}
	if desc.Kind != kind {
		return Descriptor{}, fmt.Errorf("metrics: descriptor kind mismatch for %q", desc.Name)
	}
	return desc, nil
}

func sameDescriptor(a, b Descriptor) bool {
	return a.Name == b.Name && a.Help == b.Help && a.Unit == b.Unit && a.Kind == b.Kind
}

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

func conflictingMetricError(name string) error {
	return fmt.Errorf("metrics: conflicting metric registration for %q", name)
}
