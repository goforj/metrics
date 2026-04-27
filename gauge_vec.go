package metrics

import "sync"

// GaugeVec is a gauge family with a fixed label set.
type GaugeVec struct {
	desc      Descriptor
	labelKeys []string
	mu        sync.RWMutex
	metrics   map[string]*Gauge
}

// GaugeVec registers or returns an existing labeled gauge family.
func (r *Registry) GaugeVec(desc Descriptor, labelKeys []string) (*GaugeVec, error) {
	var err error
	desc, err = canonicalizeDescriptor(desc, KindGauge)
	if err != nil {
		return nil, err
	}
	labelKeys, err = validateLabelKeys(labelKeys)
	if err != nil {
		return nil, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if metric := r.gaugeVecs[desc.Name]; metric != nil {
		if sameDescriptor(metric.desc, desc) && sameLabelKeys(metric.labelKeys, labelKeys) {
			return metric, nil
		}
		return nil, conflictingMetricError(desc.Name)
	}
	if r.counters[desc.Name] != nil || r.counterVecs[desc.Name] != nil || r.gauges[desc.Name] != nil || r.histograms[desc.Name] != nil || r.histogramVecs[desc.Name] != nil {
		return nil, conflictingMetricError(desc.Name)
	}

	metric := &GaugeVec{
		desc:      desc,
		labelKeys: append([]string(nil), labelKeys...),
		metrics:   map[string]*Gauge{},
	}
	r.gaugeVecs[desc.Name] = metric
	return metric, nil
}

// MustGaugeVec registers a labeled gauge family or panics.
func (r *Registry) MustGaugeVec(desc Descriptor, labelKeys []string) *GaugeVec {
	metric, err := r.GaugeVec(desc, labelKeys)
	if err != nil {
		panic(err)
	}
	return metric
}

// WithLabelValues returns the child gauge for one fixed label value set.
func (v *GaugeVec) WithLabelValues(values ...string) *Gauge {
	if v == nil {
		return nil
	}
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

	v.mu.Lock()
	defer v.mu.Unlock()
	if metric = v.metrics[key]; metric != nil {
		return metric
	}
	metric = &Gauge{
		desc:   v.desc,
		labels: makeLabels(v.labelKeys, values),
	}
	v.metrics[key] = metric
	return metric
}
