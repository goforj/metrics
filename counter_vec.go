package metrics

import "sync"

// CounterVec is a counter family with a fixed label set.
type CounterVec struct {
	desc      Descriptor
	labelKeys []string
	mu        sync.RWMutex
	metrics   map[string]*Counter
}

// CounterVec registers or returns an existing labeled counter family.
func (r *Registry) CounterVec(desc Descriptor, labelKeys []string) (*CounterVec, error) {
	var err error
	desc, err = canonicalizeDescriptor(desc, KindCounter)
	if err != nil {
		return nil, err
	}
	labelKeys, err = validateLabelKeys(labelKeys)
	if err != nil {
		return nil, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if metric := r.counterVecs[desc.Name]; metric != nil {
		if sameDescriptor(metric.desc, desc) && sameLabelKeys(metric.labelKeys, labelKeys) {
			return metric, nil
		}
		return nil, conflictingMetricError(desc.Name)
	}
	if err := r.reserveRegistration(desc); err != nil {
		return nil, err
	}

	metric := &CounterVec{
		desc:      desc,
		labelKeys: append([]string(nil), labelKeys...),
		metrics:   map[string]*Counter{},
	}
	r.counterVecs[desc.Name] = metric
	return metric, nil
}

// MustCounterVec registers a labeled counter family or panics.
func (r *Registry) MustCounterVec(desc Descriptor, labelKeys []string) *CounterVec {
	metric, err := r.CounterVec(desc, labelKeys)
	if err != nil {
		panic(err)
	}
	return metric
}

// WithLabelValues returns the child counter for one fixed label value set.
func (v *CounterVec) WithLabelValues(values ...string) *Counter {
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
func (v *CounterVec) loadOrCreate(key string, values []string) *Counter {
	v.mu.Lock()
	defer v.mu.Unlock()
	if metric := v.metrics[key]; metric != nil {
		return metric
	}
	metric := &Counter{
		desc:   v.desc,
		labels: makeLabels(v.labelKeys, values),
	}
	v.metrics[key] = metric
	return metric
}
