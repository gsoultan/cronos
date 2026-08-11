package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gsoultan/cronos/internal/core/identity"
	"github.com/gsoultan/cronos/internal/platform/token"
)

// Users is what a sign-in checks against.
type Users interface {
	Authenticate(ctx context.Context, email, password string) (identity.User, error)
}

// SessionLifetime is how long a portal token lasts.
//
// A working day. Long enough that nobody signs in twice before lunch, short
// enough that a token copied out of a browser is useless by the morning. There
// is no refresh: a token that renews itself is a permanent credential wearing
// an expiry.
const SessionLifetime = 8 * time.Hour

// Auth issues portal tokens to people who can prove who they are.
type Auth struct {
	users  Users
	signer *token.Signer
	log    *slog.Logger
}

// NewAuth wires the handler.
func NewAuth(u Users, s *token.Signer, log *slog.Logger) *Auth {
	return &Auth{users: u, signer: s, log: log}
}

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type session struct {
	Token     string        `json:"token"`
	ExpiresIn int           `json:"expiresIn"`
	User      identity.User `json:"user"`
}

// ServeHTTP handles POST /v1/auth/login.
func (a *Auth) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		fail(w, http.StatusMethodNotAllowed, "Use POST.")
		return
	}

	var in credentials
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		fail(w, http.StatusBadRequest, "Send an email and a password.")
		return
	}

	user, err := a.users.Authenticate(r.Context(), in.Email, in.Password)
	if err != nil {
		// The email is logged and the password is not, obviously — but note
		// the email is logged at info rather than returned in any form that
		// distinguishes it from a wrong password.
		a.log.Info("sign-in refused", "email", redact(in.Email))
		if !errors.Is(err, identity.ErrBadCredentials) {
			a.log.Error("sign-in failed", "err", err)
		}
		fail(w, http.StatusUnauthorized, "That email and password do not match.")
		return
	}

	issued, err := a.signer.Mint(token.Claims{
		Audience: token.Portal, Role: user.Role,
		Org: user.Org, Project: user.Project, Subject: user.ID,
	}, SessionLifetime)
	if err != nil {
		a.log.Error("could not mint a session", "err", err)
		fail(w, http.StatusInternalServerError, "Could not start a session.")
		return
	}

	a.log.Info("signed in", "user", user.ID, "project", user.Org+"/"+user.Project, "role", user.Role)
	send(w, http.StatusOK, session{
		Token: issued, ExpiresIn: int(SessionLifetime.Seconds()), User: user,
	})
}

// redact keeps a log useful without making it a list of who has an account.
//
// A failed sign-in is worth recording — a hundred of them is an attack — but
// the log is read by more people than the user table is, and an address in it
// is an address that has leaked.
func redact(email string) string {
	name, domain, ok := strings.Cut(strings.TrimSpace(email), "@")
	if !ok || name == "" {
		return "?"
	}
	return name[:1] + "***@" + domain
}
