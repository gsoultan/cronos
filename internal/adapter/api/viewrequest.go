package api

import (
	"encoding/json"
	"net/http"
)

/*
viewRequest is a viewer asking for the part of a large map it has in view.

The report's own request — output, parameters, filters, sent again as they were
sent to render it — and where the reader is looking: the map by its place in
the output, the view in world units as minX, minY, maxX, maxY, and the pixels
it is drawn in. Categories are the map's own, echoed back so each view colours
them as the map did.
*/
type viewRequest struct {
	request
	Block      int        `json:"block"`
	View       [4]float64 `json:"view"`
	Width      int        `json:"width"`
	Height     int        `json:"height"`
	Categories []string   `json:"categories,omitempty"`
}

// decodeView reads one, refusing fields it does not know for the reason
// decode does.
func decodeView(r *http.Request) (viewRequest, error) {
	var req viewRequest
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, maxBody))
	dec.DisallowUnknownFields()
	err := dec.Decode(&req)
	return req, err
}
