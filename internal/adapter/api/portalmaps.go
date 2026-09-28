package api

import "net/http"

// PortalMaps serves POST /v1/reports/{name}/map: the part of a large map a
// signed-in reader has in view, with the checks a portal read makes.
type PortalMaps struct {
	reports *PortalReports
}

// NewPortalMaps wires the handler over the portal read whose checks it shares.
func NewPortalMaps(p *PortalReports) *PortalMaps { return &PortalMaps{reports: p} }

// ServeHTTP checks the reader as a portal read does, then answers the view.
func (h *PortalMaps) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		fail(w, http.StatusMethodNotAllowed, "Use POST.")
		return
	}
	pr, name, ok := h.reports.reader(w, r)
	if !ok {
		return
	}
	h.reports.embed.view(w, r, pr, name, nil)
}
