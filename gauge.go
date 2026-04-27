package metrics

import "sync/atomic"

// Gauge is a signed point-in-time metric.
type Gauge struct {
	desc   Descriptor
	labels []Label
	value  atomic.Int64
}

// Descriptor returns the metric descriptor.
func (g *Gauge) Descriptor() Descriptor {
	return g.desc
}

// Set updates the gauge to an exact value.
func (g *Gauge) Set(v int64) {
	g.value.Store(v)
}

// Add increments the gauge by delta.
func (g *Gauge) Add(delta int64) {
	g.value.Add(delta)
}

// Sub decrements the gauge by delta.
func (g *Gauge) Sub(delta int64) {
	g.value.Add(-delta)
}

// Value returns the current value.
func (g *Gauge) Value() int64 {
	return g.value.Load()
}
