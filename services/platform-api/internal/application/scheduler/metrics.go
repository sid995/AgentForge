package scheduler

import (
	"fmt"
	"net/http"
	"sync"
	"time"
)

type Metrics struct {
	mu                   sync.Mutex
	counters             map[string]uint64
	queueAgeMilliseconds int64
	inFlight             int
}

func NewMetrics() *Metrics { return &Metrics{counters: make(map[string]uint64)} }
func (metrics *Metrics) increment(name string) {
	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	metrics.counters[name]++
}
func (metrics *Metrics) Claimed(result string, count int) {
	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	metrics.counters["claims_"+result]++
	metrics.counters["runs_claimed"] += uint64(max(count, 0))
}
func (metrics *Metrics) QueueAge(age time.Duration) {
	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	metrics.queueAgeMilliseconds = age.Milliseconds()
}
func (metrics *Metrics) LeaseRenewed(result string) { metrics.increment("lease_" + result) }
func (metrics *Metrics) Decided(outcome, code string) {
	metrics.increment("eligibility_" + outcome + "_" + code)
}
func (metrics *Metrics) ObserveSelection(strategy, result string) {
	metrics.increment("selection_" + strategy + "_" + result)
}
func (metrics *Metrics) Processed(result string) { metrics.increment("processed_" + result) }
func (metrics *Metrics) Backpressure()           { metrics.increment("backpressure") }
func (metrics *Metrics) InFlight(delta int) {
	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	metrics.inFlight += delta
}

func (metrics *Metrics) ServeHTTP(writer http.ResponseWriter, _ *http.Request) {
	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	writer.Header().Set("Content-Type", "text/plain; version=0.0.4")
	_, _ = fmt.Fprintln(writer, "# TYPE agentforge_scheduler_events_total counter")
	for key, value := range metrics.counters {
		_, _ = fmt.Fprintf(writer, "agentforge_scheduler_events_total{kind=%q} %d\n", key, value)
	}
	_, _ = fmt.Fprintf(writer, "# TYPE agentforge_scheduler_oldest_claim_age_milliseconds gauge\nagentforge_scheduler_oldest_claim_age_milliseconds %d\n", metrics.queueAgeMilliseconds)
	_, _ = fmt.Fprintf(writer, "# TYPE agentforge_scheduler_in_flight gauge\nagentforge_scheduler_in_flight %d\n", metrics.inFlight)
}

var _ ClaimMetrics = (*Metrics)(nil)
var _ EligibilityMetrics = (*Metrics)(nil)
var _ SelectionObserver = (*Metrics)(nil)
var _ EngineMetrics = (*Metrics)(nil)
