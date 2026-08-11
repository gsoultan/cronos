package api

import (
	"log/slog"
	"net/http"

	"github.com/gsoultan/cronos/internal/app/publish"
	"github.com/gsoultan/cronos/internal/app/run"
	"github.com/gsoultan/cronos/internal/platform/token"
)

// Routes builds the HTTP surface.
//
// Every route is behind a token. There is no unauthenticated read of a
// definition, not even its name: a report's existence is information about our
// customer's business.
func Routes(reports Reports, runner *run.Service, signer *token.Signer,
	origins []string, log *slog.Logger) http.Handler {
	return RoutesWith(reports, runner, signer, origins, log, nil, nil, nil, nil)
}

// RoutesWith adds the management API when an admin key is configured.
//
// Absent, the endpoints are not mounted at all rather than mounted and always
// refusing. A read-only server is a legitimate deployment, and an endpoint
// that exists only to say no is an endpoint somebody will spend an afternoon
// probing.
func RoutesWith(reports Reports, runner *run.Service, signer *token.Signer,
	origins []string, log *slog.Logger,
	pub *publish.Service, store publish.Store, admin *AdminKey, runs History) http.Handler {

	embed := NewEmbed(reports, runner, signer, log)
	author := NewAuthor(signer, admin)

	mux := http.NewServeMux()
	mux.Handle("/v1/embed/reports/{name}", embed)
	// The portal's own read. A separate path from the embed one because the
	// two have different callers and different audiences, and the audience
	// check should be the first thing a handler does rather than a branch
	// inside it.
	mux.Handle("/v1/reports/{name}", NewPortalReports(embed, author, log))
	mux.HandleFunc("/v1/health", func(w http.ResponseWriter, _ *http.Request) {
		send(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// Management is open to an author with a portal token or to a pipeline
	// with the shared key. Mounted when either can exist.
	if pub != nil && (author.Enabled() || (admin != nil && admin.Enabled())) {
		defs := NewDefinitions(pub, store, author, log)
		mux.Handle("/v1/definitions", defs)
		mux.Handle("/v1/definitions/{kind}/{name}", defs)

		// Behind the admin key and never the embed token: a run record names
		// every recipient of a burst.
		if runs != nil {
			h := NewRuns(runs, author, log)
			mux.Handle("/v1/runs", h)
			mux.Handle("/v1/runs/{id}", h)
		}
	}
	return NewCORS(origins, mux)
}
