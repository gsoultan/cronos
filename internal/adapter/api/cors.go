package api

import (
	"net/http"
	"slices"
)

// CORS allows an allow-listed set of host origins to call the API.
//
// An allow-list and never a wildcard. `Access-Control-Allow-Origin: *` on an
// endpoint that reads an Authorization header means any page on the internet
// can make an authenticated request with a token it managed to obtain — and
// embed tokens live in host pages, where they are obtainable.
type CORS struct {
	origins []string
	next    http.Handler
}

// NewCORS wraps next, permitting exactly these origins.
func NewCORS(origins []string, next http.Handler) *CORS {
	return &CORS{origins: origins, next: next}
}

func (c *CORS) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin != "" && slices.Contains(c.origins, origin) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		// Echoing the origin means the response varies by it, and a shared
		// cache that misses this serves one customer's headers to another.
		w.Header().Add("Vary", "Origin")
		w.Header().Set("Access-Control-Allow-Headers", "authorization, content-type")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Max-Age", "600")
	}

	if r.Method == http.MethodOptions {
		// Answered whether or not the origin was allowed: a preflight that
		// 404s is indistinguishable from a server that is down, and the
		// difference matters to whoever is debugging their integration.
		w.WriteHeader(http.StatusNoContent)
		return
	}
	c.next.ServeHTTP(w, r)
}
