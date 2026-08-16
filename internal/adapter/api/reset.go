package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gsoultan/cronos/internal/core/identity"
	"github.com/gsoultan/cronos/internal/core/principal"
)

// Resets is what the API may ask about a forgotten password.
type Resets interface {
	ByEmail(ctx context.Context, email string) (identity.User, error)
	StartReset(ctx context.Context, r identity.Reset, hash string) error
	RecentResets(ctx context.Context, userID string, since time.Duration) (int, error)
	CompleteReset(ctx context.Context, secret, password string) (identity.User, error)
}

/*
Reset is the way back in for somebody who cannot sign in.

Before this there was none. cronos-user creates accounts and deliberately will
not reset one, so the recovery path for the commonest support request in
software was a shell on the server and a bcrypt hash written by hand — which is
an outage for the person and a standing reason for somebody to keep a production
DSN on a laptop.

Two routes, and the asymmetry between them is the design. Asking is
unauthenticated, cheap and says nothing; spending is unauthenticated, expensive
and says one thing. Everything below follows from that.
*/
type Reset struct {
	resets Resets
	post   Postman
	portal string
	log    *slog.Logger

	// perAccount bounds how many links one account can be sent in an hour. The
	// limiter in front of the route is per address and per IP and is about
	// noise; this is about the mailbox, which is the thing a flood actually
	// lands in.
	perAccount int
}

// NewReset wires the handler.
func NewReset(r Resets, post Postman, portal string, log *slog.Logger) *Reset {
	return &Reset{
		resets: r, post: post, portal: strings.TrimRight(portal, "/"),
		log: log, perAccount: 5,
	}
}

/*
Available reports whether this deployment can reset anything.

A "forgot your password?" link that leads to "no mail server is configured" is
worse than no link: it is a promise made to somebody who is already locked out.
The portal asks first and says who to contact instead.
*/
func (h *Reset) Available() bool {
	return h != nil && h.resets != nil && h.post != nil && h.portal != ""
}

// ServeHTTP handles POST /v1/auth/password/forgot and /v1/auth/password/reset.
func (h *Reset) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		fail(w, http.StatusMethodNotAllowed, "Not a method this endpoint takes.")
		return
	}
	if !h.Available() {
		// 501 rather than 404: the route exists in this build and this
		// deployment cannot serve it, which are different things to an ISV
		// reading their own logs.
		fail(w, http.StatusNotImplemented,
			"This deployment cannot send email, so passwords are reset by an administrator.")
		return
	}
	if strings.HasSuffix(r.URL.Path, "/forgot") {
		h.ask(w, r)
		return
	}
	h.spend(w, r)
}

/*
ask sends a link, or does not, and answers the same either way.

The answer is 202 whatever happened: address unknown, account disabled, five
links already sent this hour, mail server refused. An endpoint that answers
differently for an address that has an account is a way to ask, one address at
a time, who your customer's staff are — and that list is worth more to somebody
phishing than to anybody honest.

That means real failures are invisible to the caller by design, so they are
logged here rather than swallowed: an operator has to be able to find out that
their mail relay has been refusing every reset for a week.
*/
func (h *Reset) ask(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email string `json:"email"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		fail(w, http.StatusBadRequest, "That request could not be read.")
		return
	}

	// Answered before any of the work, so that the time taken carries no
	// information either. A known address that takes eighty milliseconds to
	// hash and send, beside an unknown one that takes two, is the same
	// enumeration attack with a stopwatch.
	send(w, http.StatusAccepted, map[string]string{
		"status": "If that address has an account, a link is on its way.",
	})

	// Detached from the request, which is over. The work is bounded and short;
	// a cancelled context here would mean the mail stops being sent as soon as
	// the browser has its answer, which is always.
	ctx, done := context.WithTimeout(context.WithoutCancel(r.Context()), 30*time.Second)
	go func() {
		defer done()
		h.issue(ctx, strings.TrimSpace(in.Email))
	}()
}

// issue does the work ask promised nothing about.
func (h *Reset) issue(ctx context.Context, email string) {
	if email == "" {
		return
	}
	who := principal.Principal{Email: email}

	user, err := h.resets.ByEmail(ctx, email)
	if err != nil {
		// Not an error worth a line at warn: an address nobody has is the
		// ordinary case for a typo, and a log full of them is a log nobody
		// reads. It is still audited, because "somebody is guessing addresses"
		// is a thing an audit should be able to show.
		audit(ctx, h.log, who, ActionResetAsk, email, Refused,
			map[string]any{"reason": "no such account"})
		return
	}
	if user.Disabled {
		audit(ctx, h.log, who, ActionResetAsk, email, Refused,
			map[string]any{"reason": "account disabled"})
		return
	}

	if n, err := h.resets.RecentResets(ctx, user.ID, time.Hour); err != nil {
		h.log.Error("password reset", "err", err)
		return
	} else if n >= h.perAccount {
		// The owner is being flooded. Refusing quietly is right: telling the
		// sender they have hit a limit tells them the address is real.
		audit(ctx, h.log, who, ActionResetAsk, email, Refused,
			map[string]any{"reason": "too many recently", "recent": n})
		return
	}

	secret, hash, err := identity.NewReset()
	if err != nil {
		h.log.Error("password reset", "err", err)
		return
	}
	now := time.Now()
	rec := identity.Reset{
		ID: identity.NewResetID(), UserID: user.ID, Email: user.Email,
		CreatedAt: now, Expires: now.Add(identity.ResetLife),
	}

	// Written first, then sent — the same order as an invitation. A mail that
	// goes out before the row exists is a link that fails, and the person
	// clicking it learns only that this product is broken.
	if err := h.resets.StartReset(ctx, rec, hash); err != nil {
		h.log.Error("password reset", "err", err)
		return
	}

	/*
	   In the fragment, not the query string.

	   A browser does not send a fragment to any server: it reaches no proxy
	   log, no access log and no Referer header on the way to whatever the page
	   loads next. A reset secret in `?secret=` is a working key to the account
	   written into every log between the mailbox and here — which is the same
	   reasoning an invitation link already follows, and the first version of
	   this did not.
	*/
	link := fmt.Sprintf("%s/reset#secret=%s", h.portal, secret)
	body := fmt.Sprintf(`Somebody asked to reset the password for %s.

If that was you, set a new one here:

  %s

The link works once and expires in an hour. Signing in afterwards will still
ask for your second factor if you have one.

If it was not you, nothing has happened and you can ignore this. Your password
has not changed.
`, user.Email, link)

	if err := h.post.Post(ctx, user.Email, "Reset your cronos password", body); err != nil {
		// The row stays. It expires in an hour and asking again is free, which
		// is a better failure than a person told "sent" who never receives it.
		h.log.Error("password reset could not be mailed", "err", err)
		audit(ctx, h.log, who, ActionResetAsk, email, Refused,
			map[string]any{"reason": "mail failed"})
		return
	}
	audit(ctx, h.log, who, ActionResetAsk, email, Allowed, map[string]any{"reset": rec.ID})
}

/*
spend sets the new password.

Unauthenticated, because the whole point is that the caller cannot sign in, and
the secret is the authentication. 256 bits from a CSPRNG, so the limit in front
of this route is not what stops an attack on it — it stops an unauthenticated
endpoint that runs bcrypt from being a way to spend somebody's CPU.
*/
func (h *Reset) spend(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Secret   string `json:"secret"`
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		fail(w, http.StatusBadRequest, "That request could not be read.")
		return
	}
	// The same rule the rest of the product applies, applied here too: a reset
	// is the one moment somebody chooses a password without being signed in,
	// and it would otherwise be the way to get a weak one in.
	if err := identity.Acceptable(in.Password); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}

	user, err := h.resets.CompleteReset(r.Context(), in.Secret, in.Password)
	if err != nil {
		if errors.Is(err, identity.ErrReset) {
			audit(r.Context(), h.log, principal.Principal{}, ActionResetSpend, "", Refused,
				map[string]any{"reason": "link not usable"})
			fail(w, http.StatusGone,
				"This link has expired or has already been used. Ask for another.")
			return
		}
		h.log.Error("password reset", "err", err)
		fail(w, http.StatusInternalServerError, "The password could not be changed.")
		return
	}

	audit(r.Context(), h.log,
		principal.Principal{Subject: user.ID, Email: user.Email,
			OrgID: user.Org, ProjectID: user.Project},
		ActionResetSpend, user.Email, Allowed, nil)

	/*
	   No session handed back, deliberately.

	   Signing somebody in because they proved control of a mailbox is exactly
	   what a second factor exists to prevent, and it would make a reset the way
	   around one. They sign in, which asks for a code if the account has one —
	   and every session that account already had ended in the same transaction
	   as the password change, including whoever prompted this.
	*/
	send(w, http.StatusOK, map[string]string{
		"status": "Your password has been changed. Sign in with it.",
	})
}
