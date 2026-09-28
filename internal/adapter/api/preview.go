package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	codec "github.com/gsoultan/cronos/internal/adapter/codec/yaml"
	"github.com/gsoultan/cronos/internal/app/publish"
	"github.com/gsoultan/cronos/internal/app/run"
	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/principal"
	"github.com/gsoultan/cronos/internal/core/query"
)

// Drafting checks a report as publishing it would, and keeps nothing.
type Drafting interface {
	Draft(ctx context.Context, raw []byte, pr principal.Principal) (definition.Report, error)
}

// previewRequest is a draft, and what to draw it with: the request a
// published report takes, and the report itself as its author would publish
// it.
type previewRequest struct {
	Report string `json:"report"`
	request
}

/*
Preview draws a report nobody has published: the draft in the builder.

The bar is publishing's, because the reach is: somebody who may publish a
report may open it, and a preview is that report without the store in
between. The draft is checked as a publish checks it and refused with the
same sentence, then drawn through the caller's own project and row scope, so
it shows exactly what the published report would show them. Nothing is kept:
a draft has no name anything else could ask for it by, which is also why a
large map drawn here is drawn as it opens and not refined as it is zoomed.
*/
type Preview struct {
	drafts   Drafting
	projects Projects
	auth     Principals
	log      *slog.Logger
}

// NewPreview wires the handler.
func NewPreview(drafts Drafting, projects Projects, a Principals, log *slog.Logger) *Preview {
	return &Preview{drafts: drafts, projects: projects, auth: a, log: log}
}

// mountPreview serves a draft drawn as it would be published, where drafts
// can be checked — on the render allowance, since it is a render.
func mountPreview(mux *http.ServeMux, d Deps, author Principals, limited func(http.Handler) http.Handler) {
	if drafts, ok := d.Publish.(Drafting); ok {
		mux.Handle("/v1/preview", limited(NewPreview(drafts, d.Projects, author, d.Log)))
	}
}

// ServeHTTP handles POST /v1/preview.
func (p *Preview) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	pr, ok := p.auth.Principal(r)
	if !ok {
		fail(w, http.StatusUnauthorized, "Not authorised.")
		return
	}
	if !pr.CanEdit() {
		fail(w, http.StatusForbidden, "You may not draft reports in this project.")
		return
	}
	if r.Method != http.MethodPost {
		fail(w, http.StatusMethodNotAllowed, "Not a method this endpoint takes.")
		return
	}
	var req previewRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxDefinition+maxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		fail(w, http.StatusBadRequest, "The request could not be read.")
		return
	}
	p.draw(w, r, pr, req)
}

// draw checks the draft and renders it in the caller's project.
//
// The project first. Where one project is served, the draft is checked by a
// service that knows only that project's datasets and asks nothing about who
// is asking — so checking before this would tell a caller from anywhere else
// which datasets exist here, one refusal at a time.
func (p *Preview) draw(w http.ResponseWriter, r *http.Request, pr principal.Principal, req previewRequest) {
	project, err := p.projects.Project(r.Context(), pr)
	if err != nil {
		fail(w, http.StatusForbidden, "You may not draft reports in this project.")
		return
	}
	report, err := p.drafts.Draft(r.Context(), []byte(req.Report), pr)
	if err != nil {
		p.refuse(w, err)
		return
	}
	view, err := project.Runner.Render(r.Context(), report, run.Request{
		Output: req.Output, Params: req.Params, Filters: req.Filters,
	}, pr)
	if err != nil {
		audit(r.Context(), p.log, pr, ActionPreview, report.Name, Refused, scopeOf(pr))
		p.refuse(w, err)
		return
	}
	// A preview reads rows as a published report does, and an audit that
	// could not say who saw them in a draft would miss the reads made before
	// anything was published.
	audit(r.Context(), p.log, pr, ActionPreview, report.Name, Allowed, scopeOf(pr))
	send(w, http.StatusOK, view)
}

// refuse maps a failure to a status. A draft's author is who a sentence about
// the draft is for — which field, which layer, which dataset — so those come
// back in full, as publishing returns them. A driver's own errors name tables
// and columns and reach only the log, as they do for a published report.
func (p *Preview) refuse(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, publish.ErrForbidden):
		fail(w, http.StatusForbidden, "You may not draft reports in this project.")
	case errors.Is(err, run.ErrPinned):
		fail(w, http.StatusForbidden, "That filter is fixed for this report.")
	case isCallerError(err):
		fail(w, http.StatusBadRequest, plainly(err))
	case errors.Is(err, codec.ErrDecode), errors.Is(err, publish.ErrUnsupported),
		errors.Is(err, publish.ErrNotFound), errors.Is(err, definition.ErrInvalid),
		errors.Is(err, query.ErrBadTemplate), errors.Is(err, run.ErrNotRenderable):
		fail(w, http.StatusUnprocessableEntity, err.Error())
	default:
		p.log.Error("preview failed", "err", err)
		fail(w, http.StatusInternalServerError, "The draft could not be run.")
	}
}
