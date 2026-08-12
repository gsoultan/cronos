package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"
)

// requestKey is where the id lives on a context. Unexported and of its own
// type, so nothing else can collide with it or read it by guessing.
type requestKey struct{}

// RequestID returns the id of the request this context belongs to, if any.
//
// What ties "a customer says their statement was wrong at 06:12" to the lines
// that describe it. Every log line under a request carries it, and it goes
// back in a header so the person reporting the problem can quote it.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestKey{}).(string)
	return id
}

/*
Observed wraps a handler with the three things every request needs and none of
the handlers should have to write.

An id, so a complaint and a log line can be connected. A line per request when
it finishes, at a level that reflects what happened rather than one severity
for everything. And a recover, because without one a nil map in any handler
takes the process down — and this process is often halfway through delivering
five thousand documents, each of which is somebody's invoice.

Wrapping rather than a handler each handler calls: a rule enforced by being
unavoidable is a rule that holds for the handler somebody adds next year.
*/
type Observed struct {
	next http.Handler
	log  *slog.Logger
	// now is injectable so a test can assert on a duration it chose.
	now func() time.Time
}

// NewObserved wraps next.
func NewObserved(next http.Handler, log *slog.Logger) *Observed {
	return &Observed{next: next, log: log, now: time.Now}
}

func (o *Observed) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// An id the caller supplied is kept. A load balancer or a host application
	// that already stamped one is describing the same request, and minting a
	// second would break the chain at exactly the boundary somebody is trying
	// to trace across.
	id := r.Header.Get("X-Request-Id")
	if !plausibleID(id) {
		id = newRequestID()
	}
	w.Header().Set("X-Request-Id", id)

	r = r.WithContext(context.WithValue(r.Context(), requestKey{}, id))
	rec := &recorder{ResponseWriter: w, status: http.StatusOK}
	started := o.now()

	defer func() {
		if cause := recover(); cause != nil {
			o.crashed(rec, r, id, cause, debug.Stack())
		}
		o.done(rec, r, id, o.now().Sub(started))
	}()

	o.next.ServeHTTP(rec, r)
}

// crashed turns a panic into a 500 and a log line with a stack.
//
// The stack goes to the log and never to the caller. A panic message names
// internals — a file path, a struct field, sometimes a query — and the caller
// is somebody else's end customer.
func (o *Observed) crashed(rec *recorder, r *http.Request, id string, cause any, stack []byte) {
	o.log.Error("panic serving request",
		"request", id, "method", r.Method, "path", r.URL.Path,
		"panic", fmt.Sprint(cause), "stack", string(stack))

	// Only if nothing has been written yet. A handler that panicked halfway
	// through a response has already sent a status, and writing a second one
	// produces a log line about superfluous headers rather than a fix.
	if !rec.wrote {
		fail(rec.ResponseWriter, http.StatusInternalServerError,
			"Something went wrong on our side. Quote "+id+" if you report this.")
		rec.status = http.StatusInternalServerError
	}
}

// done writes the one line that describes what happened.
func (o *Observed) done(rec *recorder, r *http.Request, id string, took time.Duration) {
	level := slog.LevelInfo
	switch {
	case rec.status >= 500:
		level = slog.LevelError
	case rec.status >= 400:
		// A refused request is not a server problem, and logging every wrong
		// password at error level is how a real error gets missed.
		level = slog.LevelWarn
	}

	o.log.Log(r.Context(), level, "request",
		"request", id, "method", r.Method, "path", r.URL.Path,
		"status", rec.status, "bytes", rec.bytes, "ms", took.Milliseconds())
}

// recorder remembers what the handler answered, because ResponseWriter will
// not say.
type recorder struct {
	http.ResponseWriter
	status int
	bytes  int
	wrote  bool
}

func (r *recorder) WriteHeader(status int) {
	if r.wrote {
		return
	}
	r.status, r.wrote = status, true
	r.ResponseWriter.WriteHeader(status)
}

func (r *recorder) Write(b []byte) (int, error) {
	r.wrote = true
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

// Flush passes through, so a handler that streams keeps streaming.
func (r *recorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// newRequestID is sixteen random bytes, hex.
//
// Not a UUID: this is read aloud in support tickets and pasted into log
// queries, and a shorter opaque string is easier to do both with. Random
// rather than sequential, because a sequential one tells whoever holds it how
// many requests this deployment has served.
func newRequestID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// A request that could not be named must still be served.
		return "req-unnamed"
	}
	return hex.EncodeToString(b[:])
}

// plausibleID rejects a caller-supplied id that is missing, enormous, or
// carrying anything that would end up in a log line unescaped.
func plausibleID(id string) bool {
	if len(id) == 0 || len(id) > 64 {
		return false
	}
	for _, c := range id {
		ok := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '-' || c == '_'
		if !ok {
			return false
		}
	}
	return true
}
