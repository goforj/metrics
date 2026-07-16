package metrics

// Kind identifies how a metric behaves and is exported.
type Kind string

const (
	// KindCounter identifies a monotonically increasing counter.
	KindCounter Kind = "counter"
	// KindGauge identifies a signed point-in-time gauge.
	KindGauge Kind = "gauge"
	// KindHistogram identifies a fixed-bucket histogram.
	KindHistogram Kind = "histogram"
)

// Unit identifies the base unit represented by a metric.
type Unit string

const (
	// UnitNone leaves the metric name and values unitless.
	UnitNone Unit = ""
	// UnitSeconds exports duration values in seconds.
	UnitSeconds Unit = "seconds"
	// UnitBytes exports sizes in bytes.
	UnitBytes Unit = "bytes"
	// UnitItems exports counts of discrete items.
	UnitItems Unit = "items"
)

// Descriptor defines the canonical identity and metadata for a metric.
type Descriptor struct {
	// Name is the internal metric name before Prometheus normalization.
	Name string
	// Help explains what the metric measures.
	Help string
	// Unit identifies the metric's base unit.
	Unit Unit
	// Kind identifies the metric type and may be omitted during registration.
	Kind Kind
}
