package metrics

// Snapshot is a detached, deterministically ordered copy of registry state.
type Snapshot struct {
	// Counters contains scalar counters and instantiated counter-vector children.
	Counters []CounterSnapshot
	// Gauges contains scalar gauges and instantiated gauge-vector children.
	Gauges []GaugeSnapshot
	// Histograms contains scalar histograms and instantiated histogram-vector children.
	Histograms []HistogramSnapshot
}

// CounterSnapshot captures one counter value.
type CounterSnapshot struct {
	// Descriptor identifies the counter family.
	Descriptor Descriptor
	// Labels identifies the vector child, or is empty for a scalar.
	Labels []Label
	// Value is the counter value at collection time.
	Value uint64
}

// GaugeSnapshot captures one gauge value.
type GaugeSnapshot struct {
	// Descriptor identifies the gauge family.
	Descriptor Descriptor
	// Labels identifies the vector child, or is empty for a scalar.
	Labels []Label
	// Value is the gauge value at collection time.
	Value int64
}

// HistogramSnapshot captures one histogram state.
type HistogramSnapshot struct {
	// Descriptor identifies the histogram family.
	Descriptor Descriptor
	// Labels identifies the vector child, or is empty for a scalar.
	Labels []Label
	// Bounds contains inclusive bucket upper bounds in the stored base unit.
	Bounds []int64
	// BucketCounts contains non-cumulative counts plus a final overflow bucket.
	BucketCounts []uint64
	// Count is the number of completed observations included in the snapshot.
	Count uint64
	// Sum is the floating-point sum in the stored base unit and may round large totals.
	Sum float64
}
