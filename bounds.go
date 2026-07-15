package metrics

import "time"

// defaultDurationBounds spans common in-process and network request latencies.
var defaultDurationBounds = []time.Duration{
	5 * time.Millisecond,
	10 * time.Millisecond,
	25 * time.Millisecond,
	50 * time.Millisecond,
	100 * time.Millisecond,
	250 * time.Millisecond,
	500 * time.Millisecond,
	time.Second,
	2500 * time.Millisecond,
	5 * time.Second,
}

// DefaultDurationBounds returns a copy of the opinionated default duration buckets.
func DefaultDurationBounds() []time.Duration {
	out := make([]time.Duration, len(defaultDurationBounds))
	copy(out, defaultDurationBounds)
	return out
}
