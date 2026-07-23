package handoff

import (
	"fmt"
	"net/http"
	"sync"
)

// RuntimeMetrics exposes bounded handoff and retry/DLQ outcomes.
type RuntimeMetrics struct {
	mu       sync.Mutex
	counters map[string]uint64
}

func NewRuntimeMetrics() *RuntimeMetrics {
	return &RuntimeMetrics{counters: make(map[string]uint64)}
}

func (metrics *RuntimeMetrics) increment(key string) {
	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	metrics.counters[key]++
}

func (metrics *RuntimeMetrics) Processed(result string) { metrics.increment("processed_" + result) }
func (metrics *RuntimeMetrics) Retried(_ string, attempt int) {
	metrics.increment(fmt.Sprintf("retried_%d", attempt))
}
func (metrics *RuntimeMetrics) DeadLettered(code string) { metrics.increment("dead_lettered_" + code) }
func (metrics *RuntimeMetrics) RoutingFailed(stage string) {
	metrics.increment("routing_failed_" + stage)
}

func (metrics *RuntimeMetrics) ServeHTTP(writer http.ResponseWriter, _ *http.Request) {
	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	writer.Header().Set("Content-Type", "text/plain; version=0.0.4")
	_, _ = fmt.Fprintln(writer, "# TYPE agentforge_agentrun_handoff_events_total counter")
	for kind, value := range metrics.counters {
		_, _ = fmt.Fprintf(writer, "agentforge_agentrun_handoff_events_total{kind=%q} %d\n", kind, value)
	}
}
