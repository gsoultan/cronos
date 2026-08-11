package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gsoultan/cronos/internal/app/run"
	"github.com/gsoultan/cronos/internal/core/principal"
	"github.com/gsoultan/cronos/internal/platform/token"
)

// maxBody caps a request body. A filter set is a few hundred bytes; anything
// larger is a mistake or an attempt to make the server hold it.
const maxBody = 64 << 10

// Embed serves reports to embedded viewers.
type Embed struct {
	reports Reports
	runner  *run.Service
	signer  *token.Signer
	log     *slog.Logger
}

// NewEmbed wires the handler.
func NewEmbed(r Reports, s *run.Service, sg *token.Signer, log *slog.Logger) *Embed {
	return &Embed{reports: r, runner: s, signer: sg, log: log}
}

// ServeHTTP handles POST /v1/embed/reports/{name}.
func (e *Embed) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		fail(w, http.StatusMethodNotAllowed, "Use POST.")
		return
	}
	name := r.PathValue("name")

	// The embed audience only. A portal token belongs to an author and must
	// not be usable as an end customer, whichever direction somebody tries it.
	claims, err := e.signer.Verify(bearer(r), token.Embed)
	if err != nil {
		// One message for every token failure. Distinguishing expired from
		// forged tells an attacker which half of their attempt worked.
		e.log.Info("embed token rejected", "report", name, "err", err)
		fail(w, http.StatusUnauthorized, "This report link is no longer valid.")
		return
	}
	// A token pinned to one report may not open another. The pin is the
	// host's decision; honouring it is what makes it worth making.
	if claims.Report != "" && claims.Report != name {
		e.log.Info("embed token used for another report",
			"pinned", claims.Report, "asked", name)
		fail(w, http.StatusForbidden, "This link does not open that report.")
		return
	}

	e.renderWith(w, r, claims.Principal(), name, claims.Params)
}

// render runs a report for an already-authenticated caller.
//
// Exported to the portal handler, which differs from this one only in how it
// decided who is asking. The compiling, the row scope and the shape of the
// answer are identical, and having two copies of them is how they stop being.
func (e *Embed) render(w http.ResponseWriter, r *http.Request,
	pr principal.Principal, name string) {
	e.renderWith(w, r, pr, name, nil)
}

func (e *Embed) renderWith(w http.ResponseWriter, r *http.Request,
	pr principal.Principal, name string, pinned map[string]any) {

	req, err := decode(r)
	if err != nil {
		fail(w, http.StatusBadRequest, "The request could not be read.")
		return
	}

	report, err := e.reports.Report(r.Context(), name)
	if err != nil {
		e.log.Info("report not found", "report", name, "err", err)
		fail(w, http.StatusNotFound, "No such report.")
		return
	}

	view, err := e.runner.Render(r.Context(), report, run.Request{
		Output:  req.Output,
		Params:  merge(pinned, req.Params),
		Filters: req.Filters,
	}, pr)
	if err != nil {
		e.report(w, name, err)
		return
	}
	send(w, http.StatusOK, view)
}

// merge layers the caller's parameters under the token's.
//
// The token wins, and it wins silently rather than by rejecting the request: a
// parameter fixed by the host is a constraint the end user never agreed to and
// should not be told about. Widening is not something the client can express —
// this is the line that makes that true.
func merge(pinned, sent map[string]any) map[string]any {
	out := make(map[string]any, len(pinned)+len(sent))
	for k, v := range sent {
		out[k] = v
	}
	for k, v := range pinned {
		out[k] = v
	}
	return out
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if after, ok := strings.CutPrefix(h, "Bearer "); ok {
		return strings.TrimSpace(after)
	}
	return ""
}

func decode(r *http.Request) (request, error) {
	var req request
	if r.Body == nil || r.ContentLength == 0 {
		return req, nil
	}
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, maxBody))
	// Unknown fields are refused. A viewer sending `filter` rather than
	// `filters` would otherwise get an unfiltered report and believe it.
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		return request{}, err
	}
	return req, nil
}

// report maps a render failure to a status and a sentence.
//
// A bad argument is the caller's to fix and says what was wrong; anything else
// is ours, and the caller is told only that. The detail reaches the log,
// because a driver error names tables and columns.
func (e *Embed) report(w http.ResponseWriter, name string, err error) {
	var status int
	var msg string

	switch {
	case errors.Is(err, run.ErrPinned):
		status, msg = http.StatusForbidden, "That filter is fixed for this report."
	case errors.Is(err, run.ErrNotRenderable):
		status, msg = http.StatusNotFound, "No such report."
	case isCallerError(err):
		status, msg = http.StatusBadRequest, plainly(err)
	default:
		status, msg = http.StatusInternalServerError, "The report could not be run."
	}

	if status >= 500 {
		e.log.Error("embed render failed", "report", name, "err", err)
	} else {
		e.log.Info("embed request refused", "report", name, "status", status, "err", err)
	}
	fail(w, status, msg)
}
