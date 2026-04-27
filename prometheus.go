package metrics

import (
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
		reg = NewRegistry()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", PrometheusContentType)
		if err := EncodePrometheus(w, reg.Snapshot()); err != nil {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
}

// EncodePrometheus writes a snapshot in Prometheus text exposition format.
func EncodePrometheus(w io.Writer, snap *Snapshot) error {
	if snap == nil {
		return nil
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

func prometheusMetricName(desc Descriptor) string {
	name := normalizePrometheusName(desc.Name)
	if desc.Unit != UnitNone {
		suffix := "_" + string(desc.Unit)
		if !strings.HasSuffix(name, suffix) {
			name += suffix
		}
	}
	if desc.Kind == KindCounter && !strings.HasSuffix(name, "_total") {
		name += "_total"
	}
	return name
}

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

func escapeHelp(help string) string {
	help = strings.ReplaceAll(help, `\`, `\\`)
	help = strings.ReplaceAll(help, "\n", `\n`)
	return help
}

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
		b.WriteString(strconv.Quote(escapeLabelValue(label.Value)))
	}
	b.WriteByte('}')
	return b.String()
}

func formatLabelsWithExtra(labels []Label, extra Label) string {
	out := make([]Label, 0, len(labels)+1)
	out = append(out, labels...)
	out = append(out, extra)
	return formatLabels(out)
}

func escapeLabelValue(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, "\n", `\n`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return value
}

func formatBound(v int64, unit Unit) string {
	if unit == UnitSeconds {
		return strconv.FormatFloat(float64(v)/1e9, 'f', -1, 64)
	}
	return strconv.FormatInt(v, 10)
}

func formatSum(v int64, unit Unit) string {
	if unit == UnitSeconds {
		return strconv.FormatFloat(float64(v)/1e9, 'f', -1, 64)
	}
	return strconv.FormatInt(v, 10)
}
