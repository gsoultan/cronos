package api

import (
	"errors"
	"net/http"

	"github.com/gsoultan/cronos/internal/app/run"
	"github.com/gsoultan/cronos/internal/core/principal"
)

// EmbedMaps serves POST /v1/embed/reports/{name}/map: the part of a large map
// an embedded reader has in view.
type EmbedMaps struct {
	embed *Embed
}

// NewEmbedMaps wires the handler over the embed handler whose checks it shares.
func NewEmbedMaps(e *Embed) *EmbedMaps { return &EmbedMaps{embed: e} }

// ServeHTTP checks the caller exactly as a render does, then answers the view.
func (h *EmbedMaps) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	claims, name, ok := h.embed.caller(w, r)
	if !ok {
		return
	}
	h.embed.view(w, r, claims.Principal(), name, claims.Params)
}

/*
view answers a viewRequest for an authenticated caller: the same project, the
same report and the same pinned parameters a render resolves, and the map's
view instead of the whole report.

Audited as a read, because it is one — zoomed in far enough, a view is the
places themselves, each with its label — with what was viewed in the detail.
*/
func (e *Embed) view(w http.ResponseWriter, r *http.Request, pr principal.Principal,
	name string, pinned map[string]any) {

	req, err := decodeView(r)
	if err != nil {
		fail(w, http.StatusBadRequest, "The request could not be read.")
		return
	}
	project, err := e.projects.Project(r.Context(), pr)
	if err != nil {
		fail(w, http.StatusNotFound, "No such report.")
		return
	}
	report, err := project.Reports.Report(r.Context(), name)
	if err != nil {
		fail(w, http.StatusNotFound, "No such report.")
		return
	}
	m, err := project.Runner.MapView(r.Context(), report, run.ViewRequest{
		Request: run.Request{Output: req.Output, Params: merge(pinned, req.Params), Filters: req.Filters},
		Block:   req.Block, Overlay: req.Overlay, Width: req.Width, Height: req.Height,
		Categories: req.Categories,
		View:       run.Bounds{MinX: req.View[0], MinY: req.View[1], MaxX: req.View[2], MaxY: req.View[3]},
	}, pr)
	detail := scopeOf(pr)
	detail["view"] = "map"
	if err != nil {
		audit(r.Context(), e.log, pr, ActionRead, name, Refused, detail)
		e.viewFailed(w, name, err)
		return
	}
	audit(r.Context(), e.log, pr, ActionRead, name, Allowed, detail)
	send(w, http.StatusOK, m)
}

// viewFailed says why a view could not be answered: a view of something that
// is not a large map is the caller's mistake, and anything else is the same as
// a render's.
func (e *Embed) viewFailed(w http.ResponseWriter, name string, err error) {
	if errors.Is(err, run.ErrNotAMap) {
		e.log.Info("map view refused", "report", name, "err", err)
		fail(w, http.StatusBadRequest, "That is not a view of a map in this report.")
		return
	}
	e.report(w, name, err)
}
