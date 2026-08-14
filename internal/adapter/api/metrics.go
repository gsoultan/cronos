package api

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

/*
Metrics counts what happened, in the format everything already scrapes.

Prometheus exposition, written by hand and with no dependency. The client
library is a reasonable thing to depend on and this needs four counters and a
histogram — the cost of the dependency is a supply-chain surface and a version
to keep current, and the cost of not having it is this file.

What is counted is what a deployment needs to alert on, and no more:

  - requests, by route and status, which answers "is it up and is it refusing"
  - how long they took, as a histogram, which answers "is it slowing down"
  - runs and deliveries, which answer "did last night work" — the question
    that has no other source, because a burst that failed at 06:00 is over by
    the time anybody looks

Route rather than path. A path carries a report name, and a metric with one
series per report is one series per customer of our customer — which is a
cardinality explosion and, in a shared monitoring system, a list of who they
sell to.
*/
type Metrics struct {
	mu sync.Mutex

	requests map[requestKey2]int64
	// durations are bucketed on the way in. Keeping observations and
	// summarising at scrape time means holding every request served since the
	// last one, which on a busy instance is the memory a burst needs.
	durations map[string]*histogram

	runs       map[string]int64 // by result
	deliveries map[string]int64 // by result
	started    time.Time
	now        func() time.Time

	// schedulers are read at scrape time rather than pushed to. A gauge whose
	// job is to say "nothing has happened for a while" cannot be maintained by
	// the thing that has stopped happening.
	schedulers map[string]SchedulerState
}

/*
SchedulerState is what a scheduler can be asked at scrape time.

Both halves are needed and neither is sufficient. Due says what is armed and
when it fires, which catches a deployment that armed nothing. LastTick says the
loop is still going round, which catches one that armed everything and stopped.
*/
type SchedulerState interface {
	Due() map[string]time.Time
	LastTick() time.Time
}

type requestKey2 struct {
	route  string
	status int
}

// NewMetrics returns an empty registry.
func NewMetrics() *Metrics {
	now := time.Now
	return &Metrics{
		requests:   map[requestKey2]int64{},
		durations:  map[string]*histogram{},
		runs:       map[string]int64{},
		deliveries: map[string]int64{},
		started:    now(),
		now:        now,
		schedulers: map[string]SchedulerState{},
	}
}

// WithClock fixes the clock, so a test can be a stopped scheduler rather than
// wait to become one.
func (m *Metrics) WithClock(now func() time.Time) *Metrics {
	m.now = now
	m.started = now()
	return m
}

/*
WatchScheduler reports on one project's scheduler.

Called only for a scheduler that was actually armed, so the absence of any
watcher is itself the signal that this process schedules nothing — which is a
correct and common state for a replica, and an incident when it is true of
every replica at once.
*/
func (m *Metrics) WatchScheduler(project string, s SchedulerState) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.schedulers[project] = s
}

// Request records one served request.
func (m *Metrics) Request(route string, status int, took time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.requests[requestKey2{route: route, status: status}]++
	h, ok := m.durations[route]
	if !ok {
		h = newHistogram()
		m.durations[route] = h
	}
	h.observe(took.Seconds())
}

// Run records a finished run by its result: delivered, partial or failed.
func (m *Metrics) Run(result string, recipients, delivered int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.runs[result]++
	m.deliveries["delivered"] += int64(delivered)
	// Counted rather than derived at query time: "how many customers did not
	// get theirs" is the alert, and making somebody subtract two series to ask
	// it is how the alert does not get written.
	m.deliveries["failed"] += int64(recipients - delivered)
}

// ServeHTTP writes the exposition format.
func (m *Metrics) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var out strings.Builder

	out.WriteString("# HELP cronos_requests_total Requests served, by route and status.\n")
	out.WriteString("# TYPE cronos_requests_total counter\n")
	for _, k := range sortedRequests(m.requests) {
		fmt.Fprintf(&out, "cronos_requests_total{route=%q,status=\"%d\"} %d\n",
			k.route, k.status, m.requests[k])
	}

	out.WriteString("# HELP cronos_request_duration_seconds How long requests took.\n")
	out.WriteString("# TYPE cronos_request_duration_seconds histogram\n")
	for _, route := range sortedKeys(m.durations) {
		m.durations[route].write(&out, route)
	}

	out.WriteString("# HELP cronos_runs_total Scheduled runs, by result.\n")
	out.WriteString("# TYPE cronos_runs_total counter\n")
	for _, result := range sortedCounts(m.runs) {
		fmt.Fprintf(&out, "cronos_runs_total{result=%q} %d\n", result, m.runs[result])
	}

	out.WriteString("# HELP cronos_deliveries_total Documents delivered, by result.\n")
	out.WriteString("# TYPE cronos_deliveries_total counter\n")
	for _, result := range sortedCounts(m.deliveries) {
		fmt.Fprintf(&out, "cronos_deliveries_total{result=%q} %d\n", result, m.deliveries[result])
	}

	m.writeSchedulers(&out)

	out.WriteString("# HELP cronos_uptime_seconds How long this process has been serving.\n")
	out.WriteString("# TYPE cronos_uptime_seconds gauge\n")
	fmt.Fprintf(&out, "cronos_uptime_seconds %.0f\n", m.now().Sub(m.started).Seconds())

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(out.String()))
}

// buckets are the boundaries, in seconds.
//
// Chosen around what matters for this product rather than by decade: an
// interactive report is expected under a second, a heavy one under ten, and
// anything past thirty is a timeout waiting to be reported as a bug.
var buckets = []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30}

type histogram struct {
	counts []int64
	sum    float64
	total  int64
}

func newHistogram() *histogram {
	return &histogram{counts: make([]int64, len(buckets))}
}

func (h *histogram) observe(seconds float64) {
	h.sum += seconds
	h.total++
	for i, upper := range buckets {
		if seconds <= upper {
			h.counts[i]++
		}
	}
}

func (h *histogram) write(out *strings.Builder, route string) {
	for i, upper := range buckets {
		fmt.Fprintf(out, "cronos_request_duration_seconds_bucket{route=%q,le=\"%g\"} %d\n",
			route, upper, h.counts[i])
	}
	fmt.Fprintf(out, "cronos_request_duration_seconds_bucket{route=%q,le=\"+Inf\"} %d\n",
		route, h.total)
	fmt.Fprintf(out, "cronos_request_duration_seconds_sum{route=%q} %g\n", route, h.sum)
	fmt.Fprintf(out, "cronos_request_duration_seconds_count{route=%q} %d\n", route, h.total)
}

func sortedRequests(in map[requestKey2]int64) []requestKey2 {
	out := make([]requestKey2, 0, len(in))
	for k := range in {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].route != out[j].route {
			return out[i].route < out[j].route
		}
		return out[i].status < out[j].status
	})
	return out
}

func sortedKeys(in map[string]*histogram) []string {
	out := make([]string, 0, len(in))
	for k := range in {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedCounts(in map[string]int64) []string {
	out := make([]string, 0, len(in))
	for k := range in {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

/*
writeSchedulers is the answer to "is anybody running the schedules".

Three gauges, because there are three ways for the answer to be no and a
deployment has to be able to tell them apart:

  - nothing armed anywhere: the flag was never set on any replica, so every
    schedule in every project is simply not running. `cronos_scheduler_armed`
    is 0 on every instance, and no counter anywhere is non-zero, because
    nothing has happened at all.

  - armed and stuck: the loop stopped going round. Seconds since the last pass
    grows without bound, and this is the one nothing else can see — the process
    serves every request, answers health and readiness, and quietly does not
    fire. Alert on it above a few times the tick.

  - going round and behind: a schedule is past its firing time and has not run,
    usually because the previous run of the same schedule is still going. Small
    values are ordinary; growing ones are a burst that cannot finish inside its
    own interval.

Labelled by project because a deployment serving three of them wants to know
which one is stuck, and the label is bounded by configuration rather than by
data — three projects is three series, not one per customer.
*/
func (m *Metrics) writeSchedulers(out *strings.Builder) {
	now := m.now()

	out.WriteString("# HELP cronos_scheduler_armed Whether this process runs schedules.\n")
	out.WriteString("# TYPE cronos_scheduler_armed gauge\n")
	armed := 0
	if len(m.schedulers) > 0 {
		armed = 1
	}
	fmt.Fprintf(out, "cronos_scheduler_armed %d\n", armed)

	if len(m.schedulers) == 0 {
		return
	}

	projects := make([]string, 0, len(m.schedulers))
	for p := range m.schedulers {
		projects = append(projects, p)
	}
	sort.Strings(projects)

	out.WriteString("# HELP cronos_schedules_armed Schedules with a next firing, by project.\n")
	out.WriteString("# TYPE cronos_schedules_armed gauge\n")
	for _, p := range projects {
		fmt.Fprintf(out, "cronos_schedules_armed{project=%q} %d\n", p, len(m.schedulers[p].Due()))
	}

	out.WriteString("# HELP cronos_scheduler_seconds_since_tick " +
		"How long since the scheduler loop last completed a pass.\n")
	out.WriteString("# TYPE cronos_scheduler_seconds_since_tick gauge\n")
	for _, p := range projects {
		last := m.schedulers[p].LastTick()
		if last.IsZero() {
			// Never ran. Reported as the uptime rather than as a number since
			// the epoch, which would be true and would also make every
			// dashboard unreadable.
			fmt.Fprintf(out, "cronos_scheduler_seconds_since_tick{project=%q} %.0f\n",
				p, now.Sub(m.started).Seconds())
			continue
		}
		fmt.Fprintf(out, "cronos_scheduler_seconds_since_tick{project=%q} %.0f\n",
			p, now.Sub(last).Seconds())
	}

	out.WriteString("# HELP cronos_schedule_overdue_seconds " +
		"How far past its firing time the most overdue schedule is.\n")
	out.WriteString("# TYPE cronos_schedule_overdue_seconds gauge\n")
	for _, p := range projects {
		fmt.Fprintf(out, "cronos_schedule_overdue_seconds{project=%q} %.0f\n",
			p, overdue(m.schedulers[p].Due(), now))
	}
}

// overdue is how long the most overdue schedule has been waiting, or zero when
// none is. Zero rather than a negative time-until-next: an alert reads more
// clearly as "greater than five minutes" than as "less than minus five".
func overdue(due map[string]time.Time, now time.Time) float64 {
	worst := 0.0
	for _, when := range due {
		if late := now.Sub(when).Seconds(); late > worst {
			worst = late
		}
	}
	return worst
}
