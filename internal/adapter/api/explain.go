package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gsoultan/cronos/internal/app/explain"
	"github.com/gsoultan/cronos/internal/app/run"
)

// Explaining says in plain language what a rendered view shows.
type Explaining interface {
	Explain(ctx context.Context, v run.View, req explain.Request) (explain.Answer, error)
}

/*
Explain serves POST /v1/reports/{name}/explain.

The report is rendered here, as the person asking, before anything is sent to
a model — through the portal's own checks and the embed handler's own render,
so the rows an explanation reads are the rows this person was served a moment
ago and no others. A request cannot hand this endpoint numbers to explain; it
can only name a report it may already open.

Mounted only where a deployment configured a model. Absent, the route does not
exist and the catalogue says so, which is how the portal knows not to offer it.
*/
type Explain struct {
	reports  *PortalReports
	explains Explaining
	log      *slog.Logger
}

// NewExplain wires the handler over the portal read whose checks it shares.
func NewExplain(p *PortalReports, e Explaining, log *slog.Logger) *Explain {
	return &Explain{reports: p, explains: e, log: log}
}

// explainRequest is the report's own request, sent as it was to render it,
// and what the reader is asking about it.
type explainRequest struct {
	request
	Block    *int   `json:"block,omitempty"`
	Question string `json:"question,omitempty"`
}

func (h *Explain) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		fail(w, http.StatusMethodNotAllowed, "Use POST.")
		return
	}
	pr, name, ok := h.reports.reader(w, r)
	if !ok {
		return
	}
	var in explainRequest
	if r.ContentLength != 0 {
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&in); err != nil {
			fail(w, http.StatusBadRequest, "The request could not be read.")
			return
		}
	}
	view, ok := h.reports.embed.rendered(w, r, pr, name, nil, in.request)
	if !ok {
		return
	}

	answer, err := h.explains.Explain(r.Context(), view, explain.Request{Block: in.Block, Question: in.Question})
	// The length of the question and never its text: a question about a
	// report is often a sentence about somebody's numbers, and the audit
	// sink is not where those should be kept.
	detail := map[string]any{"block": in.Block, "question": len(in.Question)}
	if err != nil {
		detail["reason"] = err.Error()
		audit(r.Context(), h.log, pr, ActionExplain, name, Refused, detail)
		h.refuse(w, name, err)
		return
	}
	audit(r.Context(), h.log, pr, ActionExplain, name, Allowed, detail)
	send(w, http.StatusOK, answer)
}

// refuse maps a failure to what a reader can do about it. The provider's own
// error reaches the log and nowhere else.
func (h *Explain) refuse(w http.ResponseWriter, name string, err error) {
	switch {
	case errors.Is(err, explain.ErrInvalid):
		fail(w, http.StatusBadRequest,
			strings.TrimPrefix(err.Error(), explain.ErrInvalid.Error()+": "))
	case errors.Is(err, explain.ErrDeclined):
		fail(w, http.StatusUnprocessableEntity, "The assistant would not answer that. Try asking it another way.")
	default:
		h.log.Warn("explanation failed", "report", name, "err", err)
		fail(w, http.StatusBadGateway, "No explanation is available right now. Try again in a moment.")
	}
}
