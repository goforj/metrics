// Package metrics provides explicit in-memory metric registries and Prometheus
// text exposition.
//
// A Registry owns counters, gauges, and fixed-bucket histograms. The package has
// no global registry: applications create and wire the registry they want to
// expose. NewRegistry is the clearest constructor, and Registry's zero value is
// also ready to use.
//
// Registration is intended for application startup. Repeating an identical
// registration returns the same metric handle, while conflicting descriptors,
// vector labels, histogram bounds, internal names, or normalized Prometheus
// series names return errors. Must registration methods turn those programming
// errors into startup panics.
//
// Metric handles are safe for concurrent use. Scalar handles and vector
// children update without registry or vector locks; WithLabelValues uses a lock
// to find or permanently create a child. Applications should therefore use
// bounded label dimensions and retain frequently updated child handles.
//
// Duration histogram helpers accept time.Duration bounds, store nanoseconds,
// and export seconds. Registry.Snapshot returns detached, deterministically
// ordered state, with coherent buckets and counts for each histogram.
// EncodePrometheus validates snapshots, including caller-constructed snapshots,
// before writing Prometheus text format 0.0.4. Handler exposes the same encoding
// over HTTP and requires a non-nil registry.
package metrics
