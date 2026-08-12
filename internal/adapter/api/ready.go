package api

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// Check is one dependency this server cannot serve without.
type Check struct {
	// Name is what appears in the answer, so an operator reading a failed
	// readiness probe knows which thing to go and look at.
	Name string
	// Probe returns nil when the dependency is answering.
	Probe func(ctx context.Context) error
	// Required says a failure means not ready. A dependency that only some
	// requests need — a second warehouse behind one report — makes the answer
	// degraded rather than down, because taking the whole instance out of
	// rotation would fail every other report too.
	Required bool
}

/*
Ready answers whether this process can serve, by asking.

Liveness and readiness are different questions and /v1/health was answering
only the first: it returned ok unconditionally, so a load balancer kept sending
traffic to a process whose warehouse had gone away, and every request it routed
there failed. Liveness says "this process is running, do not restart it";
readiness says "send it work".

The checks are cached briefly. A readiness probe every second from each of
several load balancers, each opening a connection to every warehouse, is a
denial of service this product performs on its own customers' databases.
*/
type Ready struct {
	checks []Check
	log    *slog.Logger

	// every is how long an answer is reused. Short enough that a dependency
	// coming back is noticed within a probe interval or two, long enough that
	// a fleet of probes is one round trip rather than one each.
	every time.Duration

	mu     sync.Mutex
	at     time.Time
	answer readiness
	now    func() time.Time
}

// NewReady wires the handler.
func NewReady(log *slog.Logger, checks ...Check) *Ready {
	return &Ready{checks: checks, log: log, every: 5 * time.Second, now: time.Now}
}

type readiness struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks"`
}

// ServeHTTP handles GET /v1/ready.
func (h *Ready) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	answer := h.check(r.Context())

	status := http.StatusOK
	if answer.Status == "down" {
		// 503 rather than 500: this is a statement about whether to send work
		// here, and a load balancer reads the code, not the body.
		status = http.StatusServiceUnavailable
	}
	w.Header().Set("Cache-Control", "no-store")
	send(w, status, answer)
}

func (h *Ready) check(ctx context.Context) readiness {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.now().Sub(h.at) < h.every && h.answer.Status != "" {
		return h.answer
	}

	// Bounded, because a probe is answering a question about whether to send
	// traffic here and an unbounded one holds the answer open exactly when the
	// dependency is the problem.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()

	answer := readiness{Status: "ok", Checks: map[string]string{}}
	for _, c := range h.checks {
		if err := c.Probe(ctx); err != nil {
			answer.Checks[c.Name] = err.Error()
			if c.Required {
				answer.Status = "down"
			} else if answer.Status == "ok" {
				// Degraded is still served. One warehouse of four being
				// unreachable means three-quarters of the reports work, and
				// taking the instance out of rotation fails those too.
				answer.Status = "degraded"
			}
			continue
		}
		answer.Checks[c.Name] = "ok"
	}

	if answer.Status != "ok" {
		h.log.Warn("not ready", "status", answer.Status, "checks", answer.Checks)
	}
	h.at, h.answer = h.now(), answer
	return answer
}
