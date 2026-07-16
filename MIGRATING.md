# Migrating from v0.1.0

The quality pass keeps the small registry model but makes invalid metrics fail during startup instead of producing ambiguous or malformed exposition.

## Descriptors and registration

Descriptors now require non-blank, valid UTF-8 `Name` and `Help` fields. Only the declared `Unit` constants are accepted. `Kind` can still be omitted; if present, it must agree with the registration method.

Exact repeat registrations remain idempotent. Registration now rejects every scalar/vector conflict, normalized Prometheus name collision, and collision with a histogram-derived `_bucket`, `_sum`, or `_count` series.

Review applications that intentionally registered different internal names which normalize to the same output. Give each family a distinct Prometheus identity before upgrading.

## Duration histograms

Prefer the duration-specific constructors:

```go
latency := registry.MustDurationHistogram(metrics.Descriptor{
	Name: "http.request.duration",
	Help: "HTTP request duration.",
}, metrics.DefaultDurationBounds())
```

`DurationHistogram`, `MustDurationHistogram`, `DurationHistogramVec`, and `MustDurationHistogramVec` enforce `UnitSeconds`. `DurationBounds` is deprecated but remains available for source compatibility.

`ObserveDuration` and `ObserveSince` now panic when called on a histogram that does not export seconds. Use `Observe` for raw integer histograms.

## Histogram sums

`Histogram.Sum` and `HistogramSnapshot.Sum` now use `float64` instead of `int64`. This prevents a high-rate duration histogram's cumulative nanosecond sum from wrapping through the much smaller signed-integer range and matches Prometheus's floating-point sample model. Totals above `2^53` can lose single-unit precision; code requiring an exact integer aggregate should maintain that domain value separately. Code that assigns a sum directly to an `int64` must update its type or convert deliberately.

Histogram snapshots now collect coherent bucket, count, and sum state while observations continue. The public bucket representation remains non-cumulative and includes the final overflow bucket.

## Labels and snapshots

Vector label keys now require the Prometheus legacy label grammar, reject duplicates and the reserved `__` prefix, and reserve `le` on histograms. Label values must be valid UTF-8. A wrong `WithLabelValues` arity still panics.

`EncodePrometheus` now validates public snapshots before writing them. Caller-constructed snapshots must be deterministically ordered and have consistent descriptors, label schemas, histogram bounds, bucket totals, and generated series names.

## Nil handling

Required dependencies no longer become empty output or silent no-ops:

- `Handler(nil)` panics during handler construction;
- `EncodePrometheus` returns an error for a nil writer or snapshot;
- methods called through nil metric or vector pointers panic.

Wire a real registry and pass an explicit empty `&metrics.Snapshot{}` when empty exposition is intentional.

## Prometheus series changes

Label values containing backslashes, quotes, or newlines are now escaped exactly once. v0.1.0 double-escaped those values, so affected labels intentionally move to their correct series identity after upgrading.

For counters with units, the unit now precedes the required `_total` suffix. For example, an items counter named `jobs.processed.total` exports as `jobs_processed_items_total`. Review dashboards or recording rules that referenced the previous `jobs_processed_total_items_total` shape.
