package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"github.com/gsoultan/cronos/internal/core/identity"
	"github.com/gsoultan/cronos/internal/core/principal"
	"github.com/gsoultan/cronos/internal/platform/token"
)

/*
The first run: how a fresh install gets its first way in.

Until this, it did not have one. A new deployment had a database, a server and
no accounts, and the only way to create the first was `cronos-user` on the
machine — which is fine for somebody with shell access and impossible for
anybody handed a URL.

This is the most dangerous endpoint in the product, because for a moment it
hands out a deployment administrator to whoever asks. Everything about it is
built around closing that moment:

  - It is open only while **no account exists at all**. Not "no administrator" —
    no account, in any organisation, in any project. The first successful use
    closes it, and nothing reopens it short of emptying the users table.
  - The check is inside the write. Two requests arriving together both see an
    empty deployment if it is checked with a SELECT; only one of them can hold
    the mutex and find it still empty.
  - It needs a store. A file-backed deployment has nowhere to put an account, so
    setup is not offered rather than offered and broken.

What it deliberately does not have is a token in the log or an environment
variable to unlock it. Both are things people leave switched on, and an endpoint
that can be reopened is an endpoint somebody reopens.
*/

// Setup is the first-run bootstrap.
type Setup struct {
	roster   Roster
	platform Platform
	accounts Accounts
	signer   *token.Signer
	// serving is the single-project deployment this is setting up, so it can
	// be told what it was named. Nil where the deployment was configured to
	// serve several, which is a deployment that was configured at all and so
	// is not being named here.
	serving *One
	log     *slog.Logger

	// once serialises the write. The window this closes is small and real:
	// two people opening the setup page and pressing the button together, or
	// one person double-clicking it.
	once sync.Mutex
}

// Accounts is how setup asks whether this deployment is still empty.
type Accounts interface {
	CountAccounts(ctx context.Context) (int, error)
}

// NewSetup wires the handler.
func NewSetup(r Roster, p Platform, a Accounts, s *token.Signer, log *slog.Logger) *Setup {
	return &Setup{roster: r, platform: p, accounts: a, signer: s, log: log}
}

// Serving names the deployment this sets up, so a first run can tell it what it
// is called. Without this the account is created in a project the process does
// not serve, and the first person signs in successfully and sees nothing.
func (h *Setup) Serving(one *One) *Setup {
	h.serving = one
	return h
}

// Available reports whether this deployment can be set up at all.
//
// A file-backed one cannot: it has no accounts and no place to keep them, and
// offering a page that ends in "could not create an account" is worse than not
// offering it.
func (h *Setup) Available() bool {
	return h != nil && h.roster != nil && h.platform != nil && h.accounts != nil
}

// ServeHTTP handles GET and POST /v1/setup.
func (h *Setup) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.needed(w, r)
	case http.MethodPost:
		h.create(w, r)
	default:
		fail(w, http.StatusMethodNotAllowed, "Not a method this endpoint takes.")
	}
}

/*
needed says whether a first run is still outstanding.

Unauthenticated, and safe to be: on a deployment that has been set up it answers
`false` and nothing else, which tells a stranger only that somebody got here
first. On one that has not, there is nothing to protect yet — the whole point is
that no credential exists.
*/
func (h *Setup) needed(w http.ResponseWriter, r *http.Request) {
	n, err := h.accounts.CountAccounts(r.Context())
	if err != nil {
		h.log.Error("could not count accounts", "err", err)
		// Fail closed. Answering "yes, set me up" because a query failed would
		// offer a stranger an administrator on a deployment that has one.
		send(w, http.StatusOK, map[string]bool{"needed": false})
		return
	}
	send(w, http.StatusOK, map[string]bool{"needed": n == 0})
}

type firstRun struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Password string `json:"password"`
	// Where this deployment's first project lives. Named here rather than
	// inherited from CRONOS_ORG and CRONOS_PROJECT so a deployment ends up
	// called what somebody chose instead of "default/default".
	Org     string `json:"org"`
	Project string `json:"project"`
}

func (h *Setup) create(w http.ResponseWriter, r *http.Request) {
	var in firstRun
	if err := decodeJSON(w, r, &in); err != nil {
		fail(w, http.StatusBadRequest, "Send an email, a password, an organisation and a project.")
		return
	}

	in.Email = strings.TrimSpace(in.Email)
	in.Org, in.Project = slug(in.Org), slug(in.Project)

	switch {
	case !strings.Contains(in.Email, "@"):
		fail(w, http.StatusBadRequest, "That is not an email address.")
		return
	case in.Org == "" || in.Project == "":
		fail(w, http.StatusBadRequest,
			"An organisation and a project need names — letters, digits and hyphens.")
		return
	}
	if err := identity.Acceptable(in.Password); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}

	/*
	   From here to the insert is the window, and the mutex is what closes it.

	   Checked and written under one lock, in one process. That is the whole
	   guarantee and it is worth being exact about its limit: two cronos
	   processes against one database, both freshly installed, could both pass
	   this check. The unique index on email catches an identical address; two
	   different ones would produce two administrators. It is a real gap and a
	   narrow one — it needs a multi-process deployment brought up before
	   anybody has been given the URL — and closing it properly means a lock in
	   the database, which is a table and a migration for a race that lasts
	   the first thirty seconds of a deployment's life.
	*/
	h.once.Lock()
	defer h.once.Unlock()

	n, err := h.accounts.CountAccounts(r.Context())
	if err != nil {
		h.log.Error("could not count accounts", "err", err)
		fail(w, http.StatusInternalServerError, "Could not check whether this is a first run.")
		return
	}
	if n != 0 {
		// Somebody got here first, or this deployment was set up long ago. The
		// same answer either way, and a 409 rather than a 403 because nothing
		// was refused — the request simply arrived too late.
		fail(w, http.StatusConflict,
			"This deployment has already been set up. Sign in instead.")
		return
	}

	user := identity.User{
		ID: identity.NewID(), Email: in.Email, Name: strings.TrimSpace(in.Name),
		Org: in.Org, Project: in.Project,
		// A project administrator as well as a platform one. They are separate
		// permissions and the first person needs both: platform to administer
		// the deployment, project because somebody has to be able to write the
		// first report and platform administration deliberately does not grant
		// that.
		Role: string(principal.ProjectAdmin),
	}
	if err := h.roster.CreateUser(r.Context(), user, in.Password); err != nil {
		if errors.Is(err, identity.ErrExists) {
			fail(w, http.StatusConflict, "That email already has an account here.")
			return
		}
		h.log.Error("could not create the first account", "err", err)
		fail(w, http.StatusInternalServerError, "Could not create the account.")
		return
	}

	if err := h.platform.GrantPlatform(r.Context(), user.ID, "setup"); err != nil {
		// The account exists and is a project administrator; it is not a
		// platform one. Said plainly, because the remedy is a command on the
		// machine and somebody has to know to run it.
		h.log.Error("could not grant platform administration at setup", "err", err)
		fail(w, http.StatusInternalServerError,
			"The account was created, but it could not be made a platform administrator. "+
				"Grant it from the command line before signing in.")
		return
	}
	user.Platform = true

	/*
	   And the running process learns what it was called.

	   Found by driving it: without this the account is created in
	   acme-logistics/finance, the process is still serving default/default from
	   its environment, and the first person signs in successfully and sees an
	   empty portal — refused by the very check that keeps tenants apart.

	   Refused where the deployment was configured explicitly, because a process
	   told to serve a named project must not be renamed by an HTTP request. In
	   that case the operator meant what they configured, and the account they
	   just made is in the wrong place — which is worth saying rather than
	   silently overriding one of the two.
	*/
	if h.serving != nil && !h.serving.Adopt(user.Org, user.Project) {
		h.log.Warn("first run named a project this process does not serve",
			"named", user.Org+"/"+user.Project)
	}

	issued, err := h.signer.Mint(token.Claims{
		Audience: token.Portal, Role: user.Role,
		Org: user.Org, Project: user.Project, Subject: user.ID, Platform: true,
	}, SessionLifetime)
	if err != nil {
		h.log.Error("could not mint a session at setup", "err", err)
		// Everything worked except the last step; signing in is the remedy and
		// it will work.
		send(w, http.StatusCreated, map[string]any{"user": user})
		return
	}

	h.log.Warn("deployment set up",
		"user", user.ID, "email", user.Email, "project", user.Org+"/"+user.Project)
	audit(r.Context(), h.log, principal.Principal{
		Subject: user.ID, Email: user.Email, OrgID: user.Org, ProjectID: user.Project,
		Platform: true,
	}, ActionSetup, user.Email, Allowed,
		map[string]any{"org": user.Org, "project": user.Project})

	send(w, http.StatusCreated, map[string]any{
		"token": issued, "expiresIn": int(SessionLifetime.Seconds()), "user": user,
	})
}

/*
slug reduces a typed name to something safe to be an identifier.

Organisation and project are not display names here — they are half of every
tenancy check in the store and they end up in file paths when definitions are
kept on disk. A name with a slash in it would create a directory nobody meant;
one with a NUL would split the key that separates the two halves.

Lowercased, because the store compares them exactly and "Acme" and "acme" being
two different tenants is a support call nobody enjoys.
*/
func slug(name string) string {
	var out strings.Builder
	last := byte('-')
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'A' && c <= 'Z':
			c += 'a' - 'A'
			fallthrough
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
			out.WriteByte(c)
			last = c
		case c == ' ' || c == '-' || c == '_' || c == '.':
			// Collapsed, and never leading: "  Acme  Logistics " is one name
			// with one hyphen in it, not three.
			if last != '-' && out.Len() > 0 {
				out.WriteByte('-')
				last = '-'
			}
		}
	}
	return strings.Trim(out.String(), "-")
}
