package metrics

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
)

// Registry collects process-local counters and duration summaries in a shape
// that Prometheus can scrape without pulling in a client library.
type Registry struct {
	mu        sync.Mutex
	counters  map[string]float64
	durations map[string]*durationSummary
}

type durationSummary struct {
	count   float64
	seconds float64
}

func NewRegistry() *Registry {
	return &Registry{
		counters:  make(map[string]float64),
		durations: make(map[string]*durationSummary),
	}
}

var defaultRegistry = NewRegistry()

// Default returns the process-wide registry used by the API and store layers.
func Default() *Registry {
	return defaultRegistry
}

// metricKey renders a metric name with optional labels, keeping output stable
// by sorting label keys.
func metricKey(name string, labels map[string]string) string {
	if len(labels) == 0 {
		return name
	}
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var builder strings.Builder
	builder.WriteString(name)
	builder.WriteString("{")
	for i, key := range keys {
		if i > 0 {
			builder.WriteString(",")
		}
		builder.WriteString(key)
		builder.WriteString("=\"")
		builder.WriteString(strings.ReplaceAll(labels[key], "\"", "'"))
		builder.WriteString("\"")
	}
	builder.WriteString("}")
	return builder.String()
}

func (r *Registry) Inc(name string, labels map[string]string) {
	r.Add(name, labels, 1)
}

func (r *Registry) Add(name string, labels map[string]string, delta float64) {
	if r == nil {
		return
	}
	key := metricKey(name, labels)
	r.mu.Lock()
	r.counters[key] += delta
	r.mu.Unlock()
}

// Observe records a duration sample for a summary metric.
func (r *Registry) Observe(name string, seconds float64) {
	if r == nil {
		return
	}
	r.mu.Lock()
	summary, ok := r.durations[name]
	if !ok {
		summary = &durationSummary{}
		r.durations[name] = summary
	}
	summary.count++
	summary.seconds += seconds
	r.mu.Unlock()
}

// Render writes the registry in Prometheus text exposition format.
func (r *Registry) Render(w io.Writer) {
	if r == nil {
		return
	}
	r.mu.Lock()
	counters := make(map[string]float64, len(r.counters))
	for key, value := range r.counters {
		counters[key] = value
	}
	durations := make(map[string]durationSummary, len(r.durations))
	for key, value := range r.durations {
		durations[key] = *value
	}
	r.mu.Unlock()

	counterKeys := make([]string, 0, len(counters))
	for key := range counters {
		counterKeys = append(counterKeys, key)
	}
	sort.Strings(counterKeys)
	for _, key := range counterKeys {
		fmt.Fprintf(w, "%s %g\n", key, counters[key])
	}

	durationKeys := make([]string, 0, len(durations))
	for key := range durations {
		durationKeys = append(durationKeys, key)
	}
	sort.Strings(durationKeys)
	for _, key := range durationKeys {
		summary := durations[key]
		fmt.Fprintf(w, "%s_count %g\n", key, summary.count)
		fmt.Fprintf(w, "%s_sum %g\n", key, summary.seconds)
	}
}
