package metrics

import (
	"fmt"
	"sync"
	"testing"
)

// registrationCase describes one of the six scalar or vector registration paths.
type registrationCase struct {
	name     string
	register func(*Registry, string) (any, error)
}

// TestRegistrationKindsConflictInBothOrders exhaustively verifies cross-family raw-name ownership.
func TestRegistrationKindsConflictInBothOrders(t *testing.T) {
	cases := registrationCases()
	for firstIndex, first := range cases {
		for secondIndex, second := range cases {
			if firstIndex == secondIndex {
				continue
			}
			t.Run(first.name+"_then_"+second.name, func(t *testing.T) {
				registry := NewRegistry()
				if _, err := first.register(registry, "shared.metric"); err != nil {
					t.Fatalf("register %s: %v", first.name, err)
				}
				if _, err := second.register(registry, "shared.metric"); err == nil {
					t.Fatalf("expected %s to conflict after %s", second.name, first.name)
				}
			})
		}
	}
}

// TestConcurrentRegistrationIsIdempotent verifies that every registration path returns one shared instance under contention.
func TestConcurrentRegistrationIsIdempotent(t *testing.T) {
	const callers = 64

	for _, registration := range registrationCases() {
		t.Run(registration.name, func(t *testing.T) {
			registry := NewRegistry()
			results := make([]any, callers)
			errors := make([]error, callers)
			start := make(chan struct{})

			var wait sync.WaitGroup
			wait.Add(callers)
			for i := range callers {
				go func() {
					defer wait.Done()
					<-start
					results[i], errors[i] = registration.register(registry, "idempotent.metric")
				}()
			}
			close(start)
			wait.Wait()

			first := results[0]
			if first == nil {
				t.Fatalf("first %s registration returned nil", registration.name)
			}
			for i := range callers {
				if errors[i] != nil {
					t.Fatalf("registration %d failed: %v", i, errors[i])
				}
				if results[i] != first {
					t.Fatalf("registration %d returned a different %s instance", i, registration.name)
				}
			}
		})
	}
}

// TestRegistrationRejectsPrometheusSeriesCollisions verifies collisions introduced during name export and histogram expansion.
func TestRegistrationRejectsPrometheusSeriesCollisions(t *testing.T) {
	type collisionRegistration struct {
		name     string
		register func(*Registry) error
	}
	type collisionPair struct {
		name  string
		left  collisionRegistration
		right collisionRegistration
	}

	gauge := func(name string, unit Unit) collisionRegistration {
		return collisionRegistration{
			name: name,
			register: func(registry *Registry) error {
				_, err := registry.Gauge(Descriptor{Name: name, Help: "Collision test gauge.", Unit: unit})
				return err
			},
		}
	}
	counter := func(name string, unit Unit) collisionRegistration {
		return collisionRegistration{
			name: name,
			register: func(registry *Registry) error {
				_, err := registry.Counter(Descriptor{Name: name, Help: "Collision test counter.", Unit: unit})
				return err
			},
		}
	}
	histogram := func(name string) collisionRegistration {
		return collisionRegistration{
			name: name,
			register: func(registry *Registry) error {
				_, err := registry.Histogram(Descriptor{Name: name, Help: "Collision test histogram."}, []int64{1})
				return err
			},
		}
	}

	pairs := []collisionPair{
		{
			name:  "normalized",
			left:  gauge("HTTP Requests", UnitNone),
			right: gauge("http_requests", UnitNone),
		},
		{
			name:  "unit suffix",
			left:  gauge("payload", UnitBytes),
			right: gauge("payload_bytes", UnitNone),
		},
		{
			name:  "counter suffix against gauge",
			left:  counter("requests", UnitNone),
			right: gauge("requests_total", UnitNone),
		},
		{
			name:  "counter suffix against counter",
			left:  counter("requests", UnitNone),
			right: counter("requests.total", UnitNone),
		},
		{
			name:  "counter unit and suffix",
			left:  counter("jobs", UnitItems),
			right: gauge("jobs_items_total", UnitNone),
		},
		{
			name:  "histogram bucket",
			left:  histogram("latency"),
			right: gauge("latency_bucket", UnitNone),
		},
		{
			name:  "histogram sum",
			left:  histogram("latency"),
			right: gauge("latency_sum", UnitNone),
		},
		{
			name:  "histogram count",
			left:  histogram("latency"),
			right: gauge("latency_count", UnitNone),
		},
	}

	for _, pair := range pairs {
		orders := []struct {
			name   string
			first  collisionRegistration
			second collisionRegistration
		}{
			{name: "forward", first: pair.left, second: pair.right},
			{name: "reverse", first: pair.right, second: pair.left},
		}
		for _, order := range orders {
			t.Run(fmt.Sprintf("%s/%s", pair.name, order.name), func(t *testing.T) {
				registry := NewRegistry()
				if err := order.first.register(registry); err != nil {
					t.Fatalf("register %q: %v", order.first.name, err)
				}
				if err := order.second.register(registry); err == nil {
					t.Fatalf("expected %q to conflict after %q", order.second.name, order.first.name)
				}
			})
		}
	}
}

// registrationCases returns the complete registration surface for conflict and idempotency tests.
func registrationCases() []registrationCase {
	return []registrationCase{
		{
			name: "counter",
			register: func(registry *Registry, name string) (any, error) {
				return registry.Counter(Descriptor{Name: name, Help: "Registration test counter."})
			},
		},
		{
			name: "counter_vector",
			register: func(registry *Registry, name string) (any, error) {
				return registry.CounterVec(Descriptor{Name: name, Help: "Registration test counter vector."}, []string{"label"})
			},
		},
		{
			name: "gauge",
			register: func(registry *Registry, name string) (any, error) {
				return registry.Gauge(Descriptor{Name: name, Help: "Registration test gauge."})
			},
		},
		{
			name: "gauge_vector",
			register: func(registry *Registry, name string) (any, error) {
				return registry.GaugeVec(Descriptor{Name: name, Help: "Registration test gauge vector."}, []string{"label"})
			},
		},
		{
			name: "histogram",
			register: func(registry *Registry, name string) (any, error) {
				return registry.Histogram(Descriptor{Name: name, Help: "Registration test histogram."}, []int64{1, 2})
			},
		},
		{
			name: "histogram_vector",
			register: func(registry *Registry, name string) (any, error) {
				return registry.HistogramVec(Descriptor{Name: name, Help: "Registration test histogram vector."}, []string{"label"}, []int64{1, 2})
			},
		},
	}
}
