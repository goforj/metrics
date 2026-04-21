package metrics

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

const prometheusContentType = `text/plain; version=0.0.4; charset=utf-8`

// Handler exposes a Prometheus-compatible scrape endpoint for a registry.
func Handler(reg *Registry) http.Handler {
	if reg == nil {
		reg = NewRegistry()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", prometheusContentType)
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

	for _, metric := range snap.Counters {
		name := prometheusMetricName(metric.Descriptor)
		if _, err := fmt.Fprintf(w, "# HELP %s %s\n", name, escapeHelp(metric.Descriptor.Help)); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "# TYPE %s counter\n", name); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "%s %d\n", name, metric.Value); err != nil {
			return err
		}
	}

	for _, metric := range snap.Gauges {
		name := prometheusMetricName(metric.Descriptor)
		if _, err := fmt.Fprintf(w, "# HELP %s %s\n", name, escapeHelp(metric.Descriptor.Help)); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "# TYPE %s gauge\n", name); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "%s %d\n", name, metric.Value); err != nil {
			return err
		}
	}

	for _, metric := range snap.Histograms {
		name := prometheusMetricName(metric.Descriptor)
		if _, err := fmt.Fprintf(w, "# HELP %s %s\n", name, escapeHelp(metric.Descriptor.Help)); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "# TYPE %s histogram\n", name); err != nil {
			return err
		}
		var cumulative uint64
		for i, count := range metric.BucketCounts {
			cumulative += count
			if i < len(metric.Bounds) {
				if _, err := fmt.Fprintf(w, "%s_bucket{le=%q} %d\n", name, formatBound(metric.Bounds[i], metric.Descriptor.Unit), cumulative); err != nil {
					return err
				}
				continue
			}
			if _, err := fmt.Fprintf(w, "%s_bucket{le=%q} %d\n", name, "+Inf", cumulative); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(w, "%s_sum %s\n", name, formatSum(metric.Sum, metric.Descriptor.Unit)); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "%s_count %d\n", name, metric.Count); err != nil {
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
