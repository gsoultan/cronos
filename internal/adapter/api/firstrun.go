package api

import (
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gsoultan/cronos/internal/core/identity"
	"github.com/gsoultan/cronos/internal/platform/config"
)

/*
FirstRun configures a deployment that has none.

Distinct from Setup, which creates the first account on a deployment that is
already configured. The two look similar from a browser and answer different
questions: Setup asks "who administers this", FirstRun asks "what is this" and
then hands off to Setup by restarting into a configured server.

They are also protected differently, and that is the interesting part. Setup is
open until the first account exists, which is safe because reaching it means
somebody had already configured a signing key — proof of shell access. FirstRun
sets the signing key, so that proof does not exist yet and is restored by the
token: a random secret written to a file only the service user can read. See
boot/setup.go.
*/
type FirstRun struct {
	path   string
	accept func(string) bool
	newKey func() (string, error)
	done   func()
	env    []string
	log    *slog.Logger
}

// FirstRunDeps is what the handler needs. A struct because five of these are
// functions and a positional constructor would be five things to get in order.
type FirstRunDeps struct {
	// ConfigPath is where the configuration will be written.
	ConfigPath string
	// Accepts checks the first-run token.
	Accepts func(string) bool
	// NewKey generates the signing key, so a person never chooses one.
	NewKey func() (string, error)
	// Finished is called once the configuration is on disk, and stops the
	// server so a service manager restarts it into an ordinary boot.
	Finished func()
	// Environment names variables already set, so the form can show them as
	// fixed rather than collect values the environment will override.
	Environment []string
	Log         *slog.Logger
}

// NewFirstRun wires the handler.
func NewFirstRun(d FirstRunDeps) *FirstRun {
	return &FirstRun{
		path: d.ConfigPath, accept: d.Accepts, newKey: d.NewKey,
		done: d.Finished, env: d.Environment, log: d.Log,
	}
}

func (h *FirstRun) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.state(w)
	case http.MethodPost:
		h.configure(w, r)
	default:
		fail(w, http.StatusMethodNotAllowed, "Not a method this endpoint takes.")
	}
}

/*
state tells the portal this is a first run, and what it cannot change.

Unauthenticated, and safe to be. It reports that the deployment is
unconfigured, which anybody who can reach an unconfigured deployment can see by
its own 503, and the names — never the values — of variables the environment
has fixed.
*/
func (h *FirstRun) state(w http.ResponseWriter) {
	send(w, http.StatusOK, map[string]any{
		"needed":       true,
		"unconfigured": true,
		"config":       h.path,
		"fixed":        h.env,
	})
}

// firstRunRequest is the whole form.
type firstRunRequest struct {
	// Token proves local access. Without it this endpoint would hand the
	// deployment's root of trust to whoever reached it first.
	Token string `json:"token"`

	// The administrator. Username here is the email they will sign in with —
	// cronos has no separate username, and inventing one would be a second
	// identifier for the same person.
	Email    string `json:"email"`
	Name     string `json:"name,omitempty"`
	Password string `json:"password"`

	Org     string `json:"org"`
	Project string `json:"project"`

	// Where reports read from, and where definitions are kept.
	Driver      string `json:"driver,omitempty"`
	DSN         string `json:"dsn,omitempty"`
	StoreDriver string `json:"storeDriver,omitempty"`
	StoreDSN    string `json:"storeDsn,omitempty"`
	Definitions string `json:"definitions,omitempty"`

	Origins     []string `json:"origins,omitempty"`
	PortalURL   string   `json:"portalUrl,omitempty"`
	BehindProxy bool     `json:"behindProxy,omitempty"`
}

func (h *FirstRun) configure(w http.ResponseWriter, r *http.Request) {
	var in firstRunRequest
	if err := decodeJSON(w, r, &in); err != nil {
		fail(w, http.StatusBadRequest, "That is not a setup request.")
		return
	}

	/*
	   The token first, before anything is read from the body.

	   Not because parsing is dangerous but because the answer must not depend
	   on the rest: a validation message returned to somebody without the token
	   tells them what a valid request looks like, and this is the one endpoint
	   where that matters.
	*/
	if h.accept == nil || !h.accept(strings.TrimSpace(in.Token)) {
		h.log.Warn("first-run setup attempted without the token",
			"request", RequestID(r.Context()))
		fail(w, http.StatusForbidden,
			"That is not the setup token. It is in the file named in the server's log.")
		return
	}

	in.Email = strings.TrimSpace(in.Email)
	switch {
	case !strings.Contains(in.Email, "@"):
		fail(w, http.StatusBadRequest, "That is not an email address.")
		return
	case in.Org == "" || in.Project == "":
		fail(w, http.StatusBadRequest, "An organisation and a project need names.")
		return
	}
	if err := identity.Acceptable(in.Password); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}

	// Generated, never typed. This is the root of trust for every token the
	// deployment will issue, and a person choosing one chooses a memorable one.
	key, err := h.newKey()
	if err != nil {
		h.log.Error("could not generate a signing key", "err", err)
		fail(w, http.StatusInternalServerError, "Could not generate a signing key.")
		return
	}

	/*
	   Where definitions live, always written explicitly.

	   Left out, Definitions falls back to config's default of "examples" — a
	   relative path that suits running from a checkout and does not exist for
	   a service started from /. Setup is writing the configuration for a real
	   deployment, so it resolves the value rather than leaving it to a default
	   chosen for development: a first run that ends in a server refusing to
	   start over a path nobody chose is the worst possible first impression,
	   and it is only fixable by hand-editing the file setup just wrote.

	   Beside the configuration, which is the directory the package already
	   creates and owns.
	*/
	definitions := strings.TrimSpace(in.Definitions)
	if definitions == "" {
		definitions = filepath.Join(filepath.Dir(h.path), "definitions")
	}
	// Created, because the next boot reads it and an empty deployment has
	// nothing to have made it.
	if err := os.MkdirAll(definitions, 0o750); err != nil {
		h.log.Error("could not create the definitions directory",
			"path", definitions, "err", err)
		fail(w, http.StatusInternalServerError,
			"Could not create "+definitions+". Check the server can write there.")
		return
	}

	file := config.File{
		SigningKey:  key,
		Org:         in.Org,
		Project:     in.Project,
		Driver:      in.Driver,
		DSN:         in.DSN,
		StoreDriver: in.StoreDriver,
		StoreDSN:    in.StoreDSN,
		Definitions: definitions,
		Origins:     in.Origins,
		Portal:      strings.TrimRight(in.PortalURL, "/"),
		BehindProxy: in.BehindProxy,
	}
	if err := file.Write(h.path); err != nil {
		h.log.Error("could not write the configuration", "path", h.path, "err", err)
		fail(w, http.StatusInternalServerError,
			"Could not write the configuration. Check the server can write "+h.path+".")
		return
	}

	/*
	   The administrator is created on the next boot, not this one.

	   This process has no store: the DSN it would use is the one that arrived
	   in this request, and opening it here would mean running migrations from
	   an endpoint that has just authenticated a stranger holding a file. The
	   account is instead written beside the configuration and picked up by the
	   configured server, which has a store, a signer, and every check that
	   normally guards account creation.
	*/
	if err := writePendingAdmin(h.path, in); err != nil {
		h.log.Error("could not record the first administrator", "err", err)
		fail(w, http.StatusInternalServerError, "Could not record the administrator.")
		return
	}

	h.log.Info("configured by first-run setup",
		"config", h.path, "org", in.Org, "project", in.Project, "admin", redact(in.Email))

	send(w, http.StatusOK, map[string]any{
		"ok": true,
		"message": "Configured. cronos is restarting — sign in as " + in.Email +
			" once it is back.",
	})

	// After the response, so the caller reads it rather than a closed
	// connection. The server stops, and the service manager starts it again.
	if h.done != nil {
		go h.done()
	}
}
