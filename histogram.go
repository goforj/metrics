package metrics

import (
	"fmt"
	"sync/atomic"
	"time"
)

// Histogram records fixed-bucket observations.
type Histogram struct {
	desc    Descriptor
	labels  []Label
	bounds  []int64
	buckets []atomic.Uint64
	count   atomic.Uint64
	sum     atomic.Int64
}

// Descriptor returns the metric descriptor.
func (h *Histogram) Descriptor() Descriptor {
	return h.desc
}

// Bounds returns a copy of the histogram bucket bounds.
func (h *Histogram) Bounds() []int64 {
	out := make([]int64, len(h.bounds))
	copy(out, h.bounds)
	return out
}

// Observe records an observation in the histogram's base unit.
func (h *Histogram) Observe(v int64) {
	idx := len(h.bounds)
	for i, bound := range h.bounds {
		if v <= bound {
			idx = i
			break
		}
	}
	h.buckets[idx].Add(1)
	h.count.Add(1)
	h.sum.Add(v)
}

// ObserveDuration records a duration as nanoseconds internally.
func (h *Histogram) ObserveDuration(d time.Duration) {
	h.Observe(d.Nanoseconds())
}

// Count returns the number of observations.
func (h *Histogram) Count() uint64 {
	return h.count.Load()
}

// Sum returns the summed observations in the histogram's base unit.
func (h *Histogram) Sum() int64 {
	return h.sum.Load()
}

func validateBounds(bounds []int64) error {
	var prev int64
	for i, bound := range bounds {
		if i == 0 {
			prev = bound
			continue
		}
		if bound <= prev {
			return fmt.Errorf("metrics: histogram bounds must be strictly increasing")
		}
		prev = bound
	}
	return nil
}

func durationBounds(bounds []time.Duration) []int64 {
	out := make([]int64, len(bounds))
	for i, bound := range bounds {
		out[i] = bound.Nanoseconds()
	}
	return out
}

// DurationBounds converts duration buckets into histogram base-unit bounds.
func DurationBounds(bounds []time.Duration) []int64 {
	return durationBounds(bounds)
}
