package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

// Probes asks a datasource whether it is there.
type Probes interface {
	Probe(ctx context.Context, name string) (time.Duration, error)
}

/*
DataSources answers whether a connection works.

The one question about a datasource that cannot be answered by reading its
definition, and the one whose wrong answer surfaces as a report failing at six
in the morning on the first of the month with a connection error nobody was
watching for.

Only where sources are defined. A deployment reading a single configured
database has nothing to name in the URL, and an endpoint that exists to say
"which one?" is one somebody will spend an afternoon probing.
*/
type DataSources struct {
	// projects resolves whose sources these are. A probe opens a connection to
	// somebody's warehouse, and doing it on behalf of a caller from another
	// project is both a leak of which sources exist and a use of their
	// database.
	projects Projects
	auth     Principals
	log      *slog.Logger
}

// NewDataSources wires the handler.
func NewDataSources(projects Projects, a Principals, log *slog.Logger) *DataSources {
	return &DataSources{projects: projects, auth: a, log: log}
}

// ServeHTTP handles POST /v1/datasources/{name}/test.
func (h *DataSources) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.auth.Principal(r)
	if !ok {
		fail(w, http.StatusUnauthorized, "Not authorised.")
		return
	}
	// The same bar as changing a definition. A probe opens a connection to
	// somebody's warehouse, and which sources exist and whether they answer is
	// not something a reader needs to know.
	if !pr.CanEdit() {
		fail(w, http.StatusForbidden, "You may not test connections in this project.")
		return
	}
	if r.Method != http.MethodPost {
		fail(w, http.StatusMethodNotAllowed, "Not a method this endpoint takes.")
		return
	}

	project, err := h.projects.Project(r.Context(), pr)
	if err != nil || project.Probes == nil {
		// No named sources here, or not this caller's project. The same answer
		// for both: a deployment reading one configured database has nothing
		// to name in the URL.
		fail(w, http.StatusNotFound, "No such datasource.")
		return
	}

	name := r.PathValue("name")
	took, err := project.Probes.Probe(r.Context(), name)

	if err != nil {
		// The driver's own sentence, in full. It is the only party that knows
		// whether this was a wrong password, a closed port or a database that
		// does not exist, and "could not connect" is the same three words for
		// all of them.
		h.log.Info("datasource probe failed", "source", name, "took", took, "err", err)
		send(w, http.StatusOK, map[string]any{
			"source": name, "ok": false, "ms": took.Milliseconds(), "error": err.Error(),
		})
		return
	}

	// Two hundred either way. The probe ran and this is what it found; a 502
	// would say cronos failed, which is not what happened.
	send(w, http.StatusOK, map[string]any{
		"source": name, "ok": true, "ms": took.Milliseconds(),
	})
}
