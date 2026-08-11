package api

import (
	"log/slog"
	"net/http"

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

	mux := http.NewServeMux()
	mux.Handle("/v1/embed/reports/{name}", NewEmbed(reports, runner, signer, log))
	mux.HandleFunc("/v1/health", func(w http.ResponseWriter, _ *http.Request) {
		send(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	return NewCORS(origins, mux)
}
