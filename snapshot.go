package metrics

// Snapshot is an immutable view of the registry at a point in time.
type Snapshot struct {
	Counters   []CounterSnapshot
	Gauges     []GaugeSnapshot
	Histograms []HistogramSnapshot
}

// CounterSnapshot captures one counter value.
type CounterSnapshot struct {
	Descriptor Descriptor
	Value      uint64
}

// GaugeSnapshot captures one gauge value.
type GaugeSnapshot struct {
	Descriptor Descriptor
	Value      int64
}

// HistogramSnapshot captures one histogram state.
type HistogramSnapshot struct {
	Descriptor   Descriptor
	Bounds       []int64
	BucketCounts []uint64
	Count        uint64
	Sum          int64
}
