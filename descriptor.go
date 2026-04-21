package metrics

// Kind identifies the metric type.
type Kind string

const (
	KindCounter   Kind = "counter"
	KindGauge     Kind = "gauge"
	KindHistogram Kind = "histogram"
)

// Unit identifies the exported semantic unit for a metric.
type Unit string

const (
	UnitNone    Unit = ""
	UnitSeconds Unit = "seconds"
	UnitBytes   Unit = "bytes"
	UnitItems   Unit = "items"
)

// Descriptor defines the canonical identity and metadata for a metric.
type Descriptor struct {
	Name string
	Help string
	Unit Unit
	Kind Kind
}
