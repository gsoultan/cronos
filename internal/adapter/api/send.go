package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	app "github.com/gsoultan/cronos/internal/app/send"
	"github.com/gsoultan/cronos/internal/core/principal"
)

// Sending delivers one report to people the sender names.
type Sending interface {
	Send(ctx context.Context, req app.Request, pr principal.Principal) (app.Result, error)
}

/*
Send serves the share panel's other half.

The panel has offered to email a report since it was drawn and there was
nothing behind it. The channels have existed all along — schedules deliver
through them every month — so what was missing was this: the decision to send
one now, to addresses somebody typed, rather than on a cron.

Rendered as the sender, which the panel says out loud. Anybody named here
receives the view of the person who sent it; a link is what to send when they
should see their own rows.
*/
type Send struct {
	sends Sending
	auth  Principals
	log   *slog.Logger
	// gate refuses a report the caller may not open. Sending renders the
	// original and mails it, so a grant that does not reach here is a grant
	// with a door beside it.
	gate gate
}

// NewSend wires the handler.
func NewSend(s Sending, a Principals, log *slog.Logger) *Send {
	return &Send{sends: s, auth: a, log: log}
}

// WithGrants refuses to send a report the caller has not been granted.
func (h *Send) WithGrants(g Granting) *Send {
	h.gate = gate{grants: g, log: h.log}
	return h
}

type sendRequest struct {
	Output  string   `json:"output"`
	Via     string   `json:"via"`
	To      []string `json:"to"`
	Subject string   `json:"subject,omitempty"`
	Note    string   `json:"note,omitempty"`
}

// ServeHTTP handles POST /v1/reports/{name}/send.
func (h *Send) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.auth.Principal(r)
	if !ok {
		fail(w, http.StatusUnauthorized, "Not authorised.")
		return
	}
	if r.Method != http.MethodPost {
		fail(w, http.StatusMethodNotAllowed, "Use POST.")
		return
	}

	var in sendRequest
	if err := decodeJSON(w, r, &in); err != nil {
		fail(w, http.StatusBadRequest, "Send a channel, a format and at least one recipient.")
		return
	}

	name := r.PathValue("name")
	// The same question /v1/reports/{name} asks, and the same answer: a report
	// somebody may not open is one they may not mail to eight addresses.
	if allowed, known := h.gate.may(r.Context(), pr, name); !known {
		fail(w, http.StatusServiceUnavailable, "Could not check who may open this report.")
		return
	} else if !allowed {
		audit(r.Context(), h.log, pr, ActionRead, name, Refused,
			map[string]any{"reason": "not granted", "via": "send"})
		fail(w, http.StatusNotFound, "No such report.")
		return
	}
	result, err := h.sends.Send(r.Context(), app.Request{
		Report: name, Output: in.Output, Via: in.Via,
		To: in.To, Subject: in.Subject, Note: in.Note,
	}, pr)

	if err != nil {
		audit(r.Context(), h.log, pr, ActionSend, name, Refused,
			map[string]any{"reason": err.Error(), "via": in.Via})
		switch {
		case errors.Is(err, app.ErrForbidden):
			fail(w, http.StatusForbidden, err.Error())
		case errors.Is(err, app.ErrInvalid):
			fail(w, http.StatusBadRequest, err.Error())
		default:
			// The renderer's own sentence: it is the only thing that knows a
			// field is missing or a typesetter is not installed.
			fail(w, http.StatusUnprocessableEntity, err.Error())
		}
		return
	}

	h.log.Info("report sent", "report", name, "via", in.Via,
		"sent", len(result.Sent), "failed", len(result.Failed), "by", pr.Subject)
	// Every recipient, because a send that reached seven of eight is not a
	// success and not a failure, and the one it missed is the whole message.
	audit(r.Context(), h.log, pr, ActionSend, name, Allowed, map[string]any{
		"via": in.Via, "sent": len(result.Sent), "failed": len(result.Failed),
		"recipients": result.Sent,
	})
	send(w, http.StatusOK, result)
}
