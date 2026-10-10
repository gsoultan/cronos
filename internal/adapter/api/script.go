package api

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"strings"
	"time"
)

/*
Script serves the embed element to a host page.

`<script type="module" src="https://reports.acme.com/v1/embed/cronos-embed.js">`
is the whole integration for a page with no build step, and what @cronos/react
loads for one with a build. Served from here, the element is always the build
of the API it talks to.

Unauthenticated, and that is not a fast path around a check: it is code, the
same bytes for everybody, carrying no data and no credential. What it can do
is decided by the token a host gives it, which every data route still checks.

A module script is fetched in CORS mode, so this is answered through the same
allow-list as the API — a page that may not call cronos may not load the
element either, and nothing here is served with a wildcard.
*/
type Script struct {
	plain, packed representation
}

// representation is one encoding of the script, with the ETag that names it.
// Two, because a strong ETag names bytes on the wire, and a cache that took
// the gzip for the plain file would hand a browser something it cannot parse.
type representation struct {
	body []byte
	etag string
}

// NewScript serves body, which is the built cronos-embed.js.
//
// Compressed once, here, rather than per request: the bundle is 125 KB and 38
// KB gzipped, and nothing else in front of cronos can be assumed to do it.
func NewScript(body []byte) *Script {
	var gz bytes.Buffer
	w, _ := gzip.NewWriterLevel(&gz, gzip.BestCompression)
	_, _ = w.Write(body)
	_ = w.Close()
	return &Script{plain: represent(body, ""), packed: represent(gz.Bytes(), "-gz")}
}

func represent(body []byte, suffix string) representation {
	sum := sha256.Sum256(body)
	return representation{body: body, etag: `"` + base64.RawURLEncoding.EncodeToString(sum[:]) + suffix + `"`}
}

func (s *Script) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		fail(w, http.StatusMethodNotAllowed, "Use GET.")
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/javascript; charset=utf-8")
	// A browser that guessed this was something else would run it as that.
	h.Set("X-Content-Type-Options", "nosniff")
	/* Revalidated on every load rather than cached for a time. The point of
	   serving it from here is that the element matches the server, and a
	   max-age would pair an upgraded API with yesterday's element for as long
	   as it said. An unchanged script costs a 304. */
	h.Set("Cache-Control", "no-cache")
	h.Add("Vary", "Accept-Encoding")

	rep := s.plain
	if acceptsGzip(r.Header.Get("Accept-Encoding")) {
		rep = s.packed
		h.Set("Content-Encoding", "gzip")
	}
	h.Set("ETag", rep.etag)
	// ServeContent answers If-None-Match and HEAD from the ETag set above.
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(rep.body))
}

// acceptsGzip reads Accept-Encoding, including the `gzip;q=0` that refuses it.
func acceptsGzip(header string) bool {
	for _, part := range strings.Split(header, ",") {
		name, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		if !strings.EqualFold(strings.TrimSpace(name), "gzip") {
			continue
		}
		q := strings.ReplaceAll(strings.TrimSpace(params), " ", "")
		return q != "q=0" && q != "q=0.0" && q != "q=0.00" && q != "q=0.000"
	}
	return false
}
