package metrics

import "time"

// ObserveSince records the elapsed time since start.
func (h *Histogram) ObserveSince(start time.Time) {
	if h == nil {
		return
	}
	h.ObserveDuration(time.Since(start))
}
