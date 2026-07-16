package metrics

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// PrometheusContentType is the standard Prometheus text exposition content type.
const PrometheusContentType = `text/plain; version=0.0.4; charset=utf-8`

// Handler exposes a Prometheus-compatible scrape endpoint for a registry.
func Handler(reg *Registry) http.Handler {
	if reg == nil {
		panic("metrics: registry is required")
	}
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		var body bytes.Buffer
		if err := EncodePrometheus(&body, reg.Snapshot()); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", PrometheusContentType)
		_, _ = w.Write(body.Bytes())
	})
}

// EncodePrometheus writes a snapshot in Prometheus text exposition format.
func EncodePrometheus(w io.Writer, snap *Snapshot) error {
	if w == nil {
		return errors.New("metrics: writer is required")
	}
	if snap == nil {
		return errors.New("metrics: snapshot is required")
	}
	if err := validatePrometheusSnapshot(snap); err != nil {
		return err
	}

	lastCounterName := ""
	for _, metric := range snap.Counters {
		name := prometheusMetricName(metric.Descriptor)
		if name != lastCounterName {
			if _, err := fmt.Fprintf(w, "# HELP %s %s\n", name, escapeHelp(metric.Descriptor.Help)); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(w, "# TYPE %s counter\n", name); err != nil {
				return err
			}
			lastCounterName = name
		}
		if _, err := fmt.Fprintf(w, "%s%s %d\n", name, formatLabels(metric.Labels), metric.Value); err != nil {
			return err
		}
	}

	lastGaugeName := ""
	for _, metric := range snap.Gauges {
		name := prometheusMetricName(metric.Descriptor)
		if name != lastGaugeName {
			if _, err := fmt.Fprintf(w, "# HELP %s %s\n", name, escapeHelp(metric.Descriptor.Help)); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(w, "# TYPE %s gauge\n", name); err != nil {
				return err
			}
			lastGaugeName = name
		}
		if _, err := fmt.Fprintf(w, "%s%s %d\n", name, formatLabels(metric.Labels), metric.Value); err != nil {
			return err
		}
	}

	lastHistogramName := ""
	for _, metric := range snap.Histograms {
		name := prometheusMetricName(metric.Descriptor)
		if name != lastHistogramName {
			if _, err := fmt.Fprintf(w, "# HELP %s %s\n", name, escapeHelp(metric.Descriptor.Help)); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(w, "# TYPE %s histogram\n", name); err != nil {
				return err
			}
			lastHistogramName = name
		}
		var cumulative uint64
		for i, count := range metric.BucketCounts {
			cumulative += count
			if i < len(metric.Bounds) {
				if _, err := fmt.Fprintf(w, "%s_bucket%s %d\n", name, formatLabelsWithExtra(metric.Labels, Label{Key: "le", Value: formatBound(metric.Bounds[i], metric.Descriptor.Unit)}), cumulative); err != nil {
					return err
				}
				continue
			}
			if _, err := fmt.Fprintf(w, "%s_bucket%s %d\n", name, formatLabelsWithExtra(metric.Labels, Label{Key: "le", Value: "+Inf"}), cumulative); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(w, "%s_sum%s %s\n", name, formatLabels(metric.Labels), formatSum(metric.Sum, metric.Descriptor.Unit)); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "%s_count%s %d\n", name, formatLabels(metric.Labels), metric.Count); err != nil {
			return err
		}
	}

	return nil
}

// prometheusMetricName applies base-unit and counter suffixes after legacy name normalization.
func prometheusMetricName(desc Descriptor) string {
	name := normalizePrometheusName(desc.Name)
	if desc.Kind == KindCounter {
		name = strings.TrimSuffix(name, "_total")
	}
	if desc.Unit != UnitNone {
		suffix := "_" + string(desc.Unit)
		if !strings.HasSuffix(name, suffix) {
			name += suffix
		}
	}
	if desc.Kind == KindCounter {
		name += "_total"
	}
	return name
}

// prometheusSeriesNames lists every sample name reserved by a metric family.
func prometheusSeriesNames(desc Descriptor) []string {
	base := prometheusMetricName(desc)
	if desc.Kind != KindHistogram {
		return []string{base}
	}
	return []string{base, base + "_bucket", base + "_sum", base + "_count"}
}

// normalizePrometheusName converts friendly internal names to the legacy Prometheus identifier grammar.
func normalizePrometheusName(name string) string {
	name = strings.ToLower(name)
	var b strings.Builder
	b.Grow(len(name))
	lastUnderscore := false
	for _, r := range name {
		valid := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_'
		if !valid {
			if !lastUnderscore {
				b.WriteByte('_')
				lastUnderscore = true
			}
			continue
		}
		b.WriteRune(r)
		lastUnderscore = r == '_'
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "_"
	}
	if out[0] >= '0' && out[0] <= '9' {
		return "_" + out
	}
	return out
}

// escapeHelp protects the two characters with special meaning in HELP text.
func escapeHelp(help string) string {
	help = strings.ReplaceAll(help, `\`, `\\`)
	help = strings.ReplaceAll(help, "\n", `\n`)
	return help
}

// formatLabels writes a stable label set using Prometheus's exact escaping rules.
func formatLabels(labels []Label) string {
	if len(labels) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteByte('{')
	for i, label := range labels {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(label.Key)
		b.WriteByte('=')
		b.WriteByte('"')
		b.WriteString(escapeLabelValue(label.Value))
		b.WriteByte('"')
	}
	b.WriteByte('}')
	return b.String()
}

// formatLabelsWithExtra adds the exporter-owned histogram bound without mutating snapshot labels.
func formatLabelsWithExtra(labels []Label, extra Label) string {
	out := make([]Label, 0, len(labels)+1)
	out = append(out, labels...)
	out = append(out, extra)
	return formatLabels(out)
}

// escapeLabelValue escapes only backslashes, newlines, and quotes as required by text exposition.
func escapeLabelValue(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, "\n", `\n`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return value
}

// formatBound scales stored nanoseconds to seconds for duration histograms.
func formatBound(v int64, unit Unit) string {
	if unit == UnitSeconds {
		return strconv.FormatFloat(float64(v)/1e9, 'f', -1, 64)
	}
	return strconv.FormatInt(v, 10)
}

// formatSum scales stored nanoseconds to seconds while retaining float64 range.
func formatSum(v float64, unit Unit) string {
	if unit == UnitSeconds {
		return strconv.FormatFloat(v/1e9, 'f', -1, 64)
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}
