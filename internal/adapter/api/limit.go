package api

import (
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

/*
Limit refuses more than a rate, per caller.

Three routes need one and they need it for different reasons. Sign-in is
password-based and public, so unlimited attempts is unlimited guesses. Opening
a share takes no credential at all — the id is the credential — so an
unbounded rate is an offer to enumerate the id space. And a report render
executes SQL against somebody else's warehouse, where the cost of a request is
not ours to spend.

A token bucket rather than a counter per window. A window resets, and a caller
who learns when it resets gets the whole allowance again on the tick — which
turns a limit of ten a minute into twenty in the same second, twice a minute.

Held in memory, which is the honest scope of it: this process, this instance.
A deployment behind several needs the limit at the edge as well, and the value
of this one is that a single instance cannot be walked through on its own.
*/
type Limit struct {
	// Rate is how many requests a caller regains per second.
	Rate float64
	// Burst is how many it may make at once before the rate applies.
	Burst float64

	mu      sync.Mutex
	buckets map[string]*bucket
	now     func() time.Time
	// swept is when idle buckets were last dropped. A map keyed by caller
	// address grows for as long as callers keep being new, which for a public
	// endpoint is a memory leak with an attacker holding the tap.
	swept time.Time
}

type bucket struct {
	tokens float64
	seen   time.Time
}

// NewLimit returns a limit of rate per second with room for burst at once.
func NewLimit(rate, burst float64) *Limit {
	return &Limit{Rate: rate, Burst: burst, buckets: map[string]*bucket{}, now: time.Now}
}

// Allow reports whether this caller may make another request now.
func (l *Limit) Allow(caller string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.sweep(now)

	b, ok := l.buckets[caller]
	if !ok {
		b = &bucket{tokens: l.Burst, seen: now}
		l.buckets[caller] = b
	}

	// Refill for the time that passed, capped at the burst: a caller who was
	// idle for a day does not get a day's worth of requests at once.
	b.tokens += now.Sub(b.seen).Seconds() * l.Rate
	if b.tokens > l.Burst {
		b.tokens = l.Burst
	}
	b.seen = now

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// Forget returns a caller's full allowance.
//
// What a successful sign-in does: somebody who mistyped twice and then got it
// right is not somebody to keep throttling, and leaving them throttled makes
// the next genuine attempt fail for a reason they cannot see.
func (l *Limit) Forget(caller string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.buckets, caller)
}

// sweep drops buckets nobody has used for long enough to have refilled.
//
// A full bucket is indistinguishable from a caller that has never been seen,
// so forgetting one costs nothing and remembering every address that ever
// arrived costs memory an attacker chooses.
func (l *Limit) sweep(now time.Time) {
	const every = time.Minute
	if now.Sub(l.swept) < every {
		return
	}
	l.swept = now

	idle := time.Duration(l.Burst/max(l.Rate, 0.001)) * time.Second
	for caller, b := range l.buckets {
		if now.Sub(b.seen) > idle {
			delete(l.buckets, caller)
		}
	}
}

/*
Limited wraps a handler with a limit, keyed by who is asking.

By identity where there is one, and by address otherwise.

The address alone was wrong and the load harness proved it in its first run:
eighty of a hundred renders refused at a concurrency of one, because an office
behind one NAT — or anything behind one load balancer without CRONOS_BEHIND_PROXY
— shares a single allowance between everybody in it. That is a limit that
throttles a real team and gets reported as "the reports are broken sometimes".

A token's subject is the right key for a render: an embed token belongs to one
end customer, a portal token to one author, and limiting each of them
separately is the thing the limit was for. Sign-in and share-open have no
identity by construction, so they keep the address.
*/
type Limited struct {
	next  http.Handler
	limit *Limit
	// by names the caller. Nil means the address.
	by func(*http.Request) string
	// message is what a refused caller is told. Per route, because "too many
	// sign-in attempts" and "too many requests" send somebody to different
	// places.
	message string
	// trusted says the caller's address arrives in a header because something
	// in front terminates the connection. Off by default: honouring
	// X-Forwarded-For without a proxy in front means the limit is keyed by a
	// value the caller chooses, which is no limit at all.
	trusted bool
}

// NewLimited wraps next.
func NewLimited(next http.Handler, l *Limit, message string) *Limited {
	return &Limited{next: next, limit: l, message: message}
}

// BehindProxy makes the limit read the forwarded address.
func (h *Limited) BehindProxy(trusted bool) *Limited {
	h.trusted = trusted
	return h
}

// By keys the limit on something other than the address.
func (h *Limited) By(key func(*http.Request) string) *Limited {
	h.by = key
	return h
}

/*
ByBearer keys a limit on the token presented, falling back to the address.

The token rather than anything inside it: reading a subject out means verifying
a signature, and a rate limiter that does public-key work before deciding
whether to do work is one an attacker can spend. Hashing the bearer is enough —
two requests with the same token are the same caller, which is the whole
question — and it costs nothing on the path that refuses.

Hashed rather than kept, because this is a map key that lives in memory for
minutes and a token is a credential.
*/
func ByBearer(r *http.Request) string {
	token := bearer(r)
	if token == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(token))
	return "t:" + hex.EncodeToString(sum[:8])
}

func (h *Limited) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	caller := ""
	if h.by != nil {
		caller = h.by(r)
	}
	if caller == "" {
		caller = callerOf(r, h.trusted)
	}
	if !h.limit.Allow(caller) {
		// Retry-After, because a client that is told to slow down and not told
		// by how much retries immediately.
		w.Header().Set("Retry-After", strconv.Itoa(int(1/max(h.limit.Rate, 0.001))+1))
		fail(w, http.StatusTooManyRequests, h.message)
		return
	}
	h.next.ServeHTTP(w, r)
}

// callerOf is the address a limit is keyed by.
func callerOf(r *http.Request, trusted bool) string {
	if trusted {
		// The leftmost entry is the original client; everything after it was
		// added by a hop. Only read when something in front is known to be
		// setting it, because a caller can send whatever it likes.
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			if comma := strings.IndexByte(forwarded, ','); comma > 0 {
				return strings.TrimSpace(forwarded[:comma])
			}
			return strings.TrimSpace(forwarded)
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
