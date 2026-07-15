package metrics

import (
	"sync"
	"time"
)

// HistogramVec is a histogram family with a fixed label set.
type HistogramVec struct {
	desc      Descriptor
	labelKeys []string
	bounds    []int64
	mu        sync.RWMutex
	metrics   map[string]*Histogram
}

// HistogramVec registers or returns an existing labeled histogram family.
func (r *Registry) HistogramVec(desc Descriptor, labelKeys []string, bounds []int64) (*HistogramVec, error) {
	var err error
	desc, err = canonicalizeDescriptor(desc, KindHistogram)
	if err != nil {
		return nil, err
	}
	labelKeys, err = validateLabelKeys(labelKeys)
	if err != nil {
		return nil, err
	}
	if err := validateHistogramLabelKeys(labelKeys); err != nil {
		return nil, err
	}
	if err := validateBounds(bounds); err != nil {
		return nil, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if metric := r.histogramVecs[desc.Name]; metric != nil {
		if sameDescriptor(metric.desc, desc) && sameLabelKeys(metric.labelKeys, labelKeys) && sameBounds(metric.bounds, bounds) {
			return metric, nil
		}
		return nil, conflictingMetricError(desc.Name)
	}
	if err := r.reserveRegistration(desc); err != nil {
		return nil, err
	}

	metric := &HistogramVec{
		desc:      desc,
		labelKeys: append([]string(nil), labelKeys...),
		bounds:    append([]int64(nil), bounds...),
		metrics:   map[string]*Histogram{},
	}
	r.histogramVecs[desc.Name] = metric
	return metric, nil
}

// DurationHistogramVec registers or returns an existing labeled duration histogram family.
func (r *Registry) DurationHistogramVec(desc Descriptor, labelKeys []string, bounds []time.Duration) (*HistogramVec, error) {
	var err error
	desc, err = durationDescriptor(desc)
	if err != nil {
		return nil, err
	}
	return r.HistogramVec(desc, labelKeys, durationBounds(bounds))
}

// MustDurationHistogramVec registers a labeled duration histogram family or panics.
func (r *Registry) MustDurationHistogramVec(desc Descriptor, labelKeys []string, bounds []time.Duration) *HistogramVec {
	metric, err := r.DurationHistogramVec(desc, labelKeys, bounds)
	if err != nil {
		panic(err)
	}
	return metric
}

// MustHistogramVec registers a labeled histogram family or panics.
func (r *Registry) MustHistogramVec(desc Descriptor, labelKeys []string, bounds []int64) *HistogramVec {
	metric, err := r.HistogramVec(desc, labelKeys, bounds)
	if err != nil {
		panic(err)
	}
	return metric
}

// WithLabelValues returns the child histogram for one fixed label value set.
func (v *HistogramVec) WithLabelValues(values ...string) *Histogram {
	if len(values) != len(v.labelKeys) {
		panic("metrics: label value count mismatch")
	}
	key := labelsKey(values)

	v.mu.RLock()
	metric := v.metrics[key]
	v.mu.RUnlock()
	if metric != nil {
		return metric
	}
	if err := validateLabelValues(values); err != nil {
		panic(err)
	}
	return v.loadOrCreate(key, values)
}

// loadOrCreate serializes the first child creation and lets concurrent losers reuse its result.
func (v *HistogramVec) loadOrCreate(key string, values []string) *Histogram {
	v.mu.Lock()
	defer v.mu.Unlock()
	if metric := v.metrics[key]; metric != nil {
		return metric
	}
	metric := newHistogram(v.desc, makeLabels(v.labelKeys, values), v.bounds)
	v.metrics[key] = metric
	return metric
}
