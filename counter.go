package metrics

import "sync/atomic"

// Counter is a monotonic increasing metric.
type Counter struct {
	desc  Descriptor
	value atomic.Uint64
}

// Descriptor returns the metric descriptor.
func (c *Counter) Descriptor() Descriptor {
	return c.desc
}

// Inc increments the counter by one.
func (c *Counter) Inc() {
	c.value.Add(1)
}

// Add increments the counter by n.
func (c *Counter) Add(n uint64) {
	c.value.Add(n)
}

// Value returns the current value.
func (c *Counter) Value() uint64 {
	return c.value.Load()
}
