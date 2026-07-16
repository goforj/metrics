package metrics

import (
	"fmt"
	"math"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

const (
	histogramHotIndexMask = uint64(1 << 63)
	histogramCountMask    = histogramHotIndexMask - 1
)

// histogramCounts stores one bank of observations so scrapes can flip writers to the other bank.
type histogramCounts struct {
	sumBits atomic.Uint64
	count   atomic.Uint64
	buckets []atomic.Uint64
}

// Histogram records integer observations in fixed, non-cumulative buckets.
type Histogram struct {
	countAndHotIndex atomic.Uint64
	desc             Descriptor
	labels           []Label
	bounds           []int64
	snapshotMu       sync.Mutex
	counts           [2]*histogramCounts
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

	ticket := h.countAndHotIndex.Add(1)
	counts := h.counts[ticket>>63]
	counts.buckets[idx].Add(1)
	atomicAddFloat(&counts.sumBits, float64(v))
	counts.count.Add(1)
}

// ObserveDuration records a duration in nanoseconds for a seconds histogram.
func (h *Histogram) ObserveDuration(d time.Duration) {
	if h.desc.Unit != UnitSeconds {
		panic("metrics: ObserveDuration requires a seconds histogram")
	}
	h.Observe(d.Nanoseconds())
}

// Count returns the number of completed observations across both scrape banks.
func (h *Histogram) Count() uint64 {
	h.snapshotMu.Lock()
	defer h.snapshotMu.Unlock()
	return h.counts[0].count.Load() + h.counts[1].count.Load()
}

// Sum returns the floating-point sum of observations in the stored base unit.
// Large totals retain range but may no longer preserve single-unit precision.
func (h *Histogram) Sum() float64 {
	h.snapshotMu.Lock()
	defer h.snapshotMu.Unlock()
	return loadFloat(&h.counts[0].sumBits) + loadFloat(&h.counts[1].sumBits)
}

// newHistogram creates aligned hot and cold banks for lock-free observations.
func newHistogram(desc Descriptor, labels []Label, bounds []int64) *Histogram {
	histogram := &Histogram{
		desc:   desc,
		labels: append([]Label(nil), labels...),
		bounds: append([]int64(nil), bounds...),
	}
	for i := range histogram.counts {
		histogram.counts[i] = &histogramCounts{
			buckets: make([]atomic.Uint64, len(bounds)+1),
		}
	}
	return histogram
}

// snapshotState flips the hot bank so a finite set of in-flight observations can finish before it is copied.
func (h *Histogram) snapshotState() ([]uint64, uint64, float64) {
	h.snapshotMu.Lock()
	defer h.snapshotMu.Unlock()

	ticket := h.countAndHotIndex.Add(histogramHotIndexMask)
	count := ticket & histogramCountMask
	hot := h.counts[ticket>>63]
	cold := h.counts[(^ticket)>>63]
	for cold.count.Load() != count {
		runtime.Gosched()
	}

	buckets := make([]uint64, len(cold.buckets))
	for i := range cold.buckets {
		buckets[i] = cold.buckets[i].Load()
	}
	sum := loadFloat(&cold.sumBits)
	mergeHistogramCounts(hot, cold)
	return buckets, count, sum
}

// mergeHistogramCounts preserves cumulative state in the new hot bank before the cold bank can be reused.
func mergeHistogramCounts(hot, cold *histogramCounts) {
	hot.count.Add(cold.count.Load())
	cold.count.Store(0)
	atomicAddFloat(&hot.sumBits, loadFloat(&cold.sumBits))
	cold.sumBits.Store(0)
	for i := range hot.buckets {
		hot.buckets[i].Add(cold.buckets[i].Load())
		cold.buckets[i].Store(0)
	}
}

// atomicAddFloat adds to a float64 encoded in an atomic word without serializing observations.
func atomicAddFloat(bits *atomic.Uint64, value float64) {
	for {
		oldBits := bits.Load()
		newBits := math.Float64bits(math.Float64frombits(oldBits) + value)
		if bits.CompareAndSwap(oldBits, newBits) {
			return
		}
	}
}

// loadFloat decodes a float64 stored in an atomic word.
func loadFloat(bits *atomic.Uint64) float64 {
	return math.Float64frombits(bits.Load())
}

// validateBounds rejects bucket layouts that cannot produce monotonic exposition.
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

// durationBounds converts durations to the nanosecond base unit used for atomic storage.
func durationBounds(bounds []time.Duration) []int64 {
	out := make([]int64, len(bounds))
	for i, bound := range bounds {
		out[i] = bound.Nanoseconds()
	}
	return out
}

// DurationBounds converts duration buckets into nanosecond histogram bounds.
//
// Deprecated: use Registry.DurationHistogram, Registry.MustDurationHistogram,
// Registry.DurationHistogramVec, or Registry.MustDurationHistogramVec.
func DurationBounds(bounds []time.Duration) []int64 {
	return durationBounds(bounds)
}
