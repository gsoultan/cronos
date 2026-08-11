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
	return RoutesWith(reports, runner, signer, origins, log,
		nil, nil, nil, nil, nil, nil, nil, nil, "", "")
}

// RoutesWith adds the management API when an admin key is configured.
//
// Absent, the endpoints are not mounted at all rather than mounted and always
// refusing. A read-only server is a legitimate deployment, and an endpoint
// that exists only to say no is an endpoint somebody will spend an afternoon
// probing.
func RoutesWith(reports Reports, runner *run.Service, signer *token.Signer,
	origins []string, log *slog.Logger,
	pub *publish.Service, store publish.Store, admin *AdminKey, runs History,
	users Users, defs Repository, due Due, fires Firing, org, project string) http.Handler {

	embed := NewEmbed(reports, runner, signer, log)
	author := NewAuthor(signer, admin)

	mux := http.NewServeMux()
	mux.Handle("/v1/embed/reports/{name}", embed)
	// The portal's own read. A separate path from the embed one because the
	// two have different callers and different audiences, and the audience
	// check should be the first thing a handler does rather than a branch
	// inside it.
	mux.Handle("/v1/reports/{name}", NewPortalReports(embed, author, log))

	// What the project contains, in one request. A browsing interface asking
	// for the names and then once per name is a page that loads in a hundred
	// round trips.
	if defs != nil {
		mux.Handle("/v1/catalog", NewCatalog(defs, due, author, log))
	}
	mux.HandleFunc("/v1/health", func(w http.ResponseWriter, _ *http.Request) {
		send(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// Sign-in exists only where there is somewhere to check credentials.
	// Mounted against nothing it would refuse every attempt identically, which
	// is indistinguishable from a wrong password and impossible to debug.
	if users != nil {
		mux.Handle("/v1/auth/login", NewAuth(users, signer, log))
	}

	// Management is open to an author with a portal token or to a pipeline
	// with the shared key. Mounted when either can exist.
	if pub != nil && (author.Enabled() || (admin != nil && admin.Enabled())) {
		handler := NewDefinitions(pub, store, author, log)
		// A read falls back to what the process booted with, for the
		// definitions no store has a copy of — a directory-bootstrapped
		// deployment answering for a report that plainly renders.
		if loaded, ok := defs.(Loaded); ok {
			handler = handler.WithLoaded(loaded, org, project)
		}
		mux.Handle("/v1/definitions", handler)
		mux.Handle("/v1/definitions/{kind}/{name}", handler)

		// Only where a scheduler is armed. A deployment that renders on
		// request has no schedules to fire, and an endpoint that exists only
		// to say no is one somebody will spend an afternoon probing.
		if fires != nil {
			mux.Handle("/v1/schedules/{name}/run", NewSchedules(fires, author, log))
		}

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
