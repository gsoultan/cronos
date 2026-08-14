package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	sqlstore "github.com/gsoultan/cronos/internal/adapter/store/sql"
	"github.com/gsoultan/cronos/internal/core/identity"
	"github.com/gsoultan/cronos/internal/core/principal"
	"github.com/gsoultan/cronos/internal/platform/token"
)

/*
Second factors, and the shape of the sign-in that has one.

The portal has shown an enrolment wizard since before there was a server to enrol
against, and it accepted any six digits. That is worse than offering nothing:
somebody who believes their account has a second factor picks a weaker password,
and an administrator who sees "2FA enabled" in a list stops asking.

Two halves, and they are asymmetric in the same way the invitation endpoints are.

Enrolling needs a session — it is somebody strengthening their own account, and
the secret must only ever go to them. Signing in with a code does not have one
yet: that is the whole point, and it is why the second step is a separate
exchange carrying a short-lived challenge rather than a session.
*/

// Factors is what the API may ask about second factors.
type Factors interface {
	Enrol(ctx context.Context, id, secret, label string) error
	Enrolling(ctx context.Context, id string) (string, error)
	Confirm(ctx context.Context, id, code string) error
	FactorOf(ctx context.Context, id string) (sqlstore.Factor, error)
	RemoveFactor(ctx context.Context, id string) error

	Protected(ctx context.Context, id string) bool
	CheckFactor(ctx context.Context, id, code string) error

	SetRecoveryCodes(ctx context.Context, id string, hashes []string) error
	SpendRecoveryCode(ctx context.Context, id, code string) error
}

/*
Factor serves /v1/auth/factor: what protects this account, and changing it.

Every route here is about the caller's own account. There is no path by which an
administrator enrols, inspects or removes somebody else's second factor —
enrolling for another person is meaningless (they hold the phone), and removing
one is the exact thing a social-engineering call asks for.
*/
type Factor struct {
	factors Factors
	roster  Roster
	auth    Principals
	// signer upgrades an enrolment-only session once it has enrolled.
	signer *token.Signer
	// issuer is what an authenticator app shows beside the code. A deployment
	// name, so somebody with three cronos accounts can tell them apart.
	issuer string
	log    *slog.Logger
}

// NewFactor wires the handler.
func NewFactor(f Factors, r Roster, a Principals, issuer string, log *slog.Logger) *Factor {
	if issuer == "" {
		issuer = "cronos"
	}
	return &Factor{factors: f, roster: r, auth: a, issuer: issuer, log: log}
}

// Upgrading lets a session that could only enrol become an ordinary one, the
// moment it has.
func (h *Factor) Upgrading(s *token.Signer) *Factor {
	h.signer = s
	return h
}

// ServeHTTP handles /v1/auth/factor and its sub-paths.
func (h *Factor) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.auth.Principal(r)
	if !ok {
		fail(w, http.StatusUnauthorized, "Not authorised.")
		return
	}

	switch {
	case strings.HasSuffix(r.URL.Path, "/start") && r.Method == http.MethodPost:
		h.start(w, r, pr)
	case strings.HasSuffix(r.URL.Path, "/confirm") && r.Method == http.MethodPost:
		h.confirm(w, r, pr)
	case strings.HasSuffix(r.URL.Path, "/codes") && r.Method == http.MethodPost:
		h.regenerate(w, r, pr)
	case strings.HasSuffix(r.URL.Path, "/factor") && r.Method == http.MethodGet:
		h.show(w, r, pr)
	case strings.HasSuffix(r.URL.Path, "/factor") && r.Method == http.MethodDelete:
		h.remove(w, r, pr)
	default:
		fail(w, http.StatusMethodNotAllowed, "Not a method this endpoint takes.")
	}
}

// show says what protects this account, without the secret.
func (h *Factor) show(w http.ResponseWriter, r *http.Request, pr principal.Principal) {
	f, err := h.factors.FactorOf(r.Context(), pr.Subject)
	if errors.Is(err, identity.ErrNoFactor) {
		send(w, http.StatusOK, map[string]any{"enrolled": false})
		return
	}
	if err != nil {
		h.log.Error("could not read a second factor", "err", err)
		fail(w, http.StatusInternalServerError, "Could not read your second factor.")
		return
	}
	send(w, http.StatusOK, map[string]any{
		"enrolled": true, "label": f.Label, "addedAt": f.AddedAt,
		"remainingCodes": f.Remaining,
	})
}

/*
start mints a secret and hands back what the app needs to scan.

The secret is in this response and in no other. It is a credential — whoever has
it can produce codes for this account for ever — so it exists in the answer to
this one authenticated request, in a QR code on the page, and in the database.
Not in a log, not in a URL, not in an email.

Nothing is protected yet. The account gains a second factor at /confirm, when a
code computed from this exact secret comes back.
*/
func (h *Factor) start(w http.ResponseWriter, r *http.Request, pr principal.Principal) {
	if _, err := h.factors.FactorOf(r.Context(), pr.Subject); err == nil {
		// Already protected. Replacing it is remove-then-enrol, deliberately
		// two acts: the removal is the one worth auditing and worth making
		// somebody mean.
		fail(w, http.StatusConflict,
			"This account already has a second factor. Remove it first.")
		return
	}

	/*
	   An enrolment already under way is returned as it stands.

	   This used to mint a new secret every time, and the second call silently
	   replaced the first — so two starts left the page showing a QR code the
	   server had already thrown away. Whoever had scanned it was then told
	   "that code is not right" for every code their app would ever produce,
	   with nothing on screen to suggest why.

	   Two starts is not exotic. React runs an effect twice in development by
	   design; a component that remounts, a page somebody refreshes half way
	   through, a request the browser retries — each is one. The browser check
	   found it because a development server does it every single time.

	   Idempotent is also the kinder behaviour: somebody who reloads mid-
	   enrolment keeps the QR they already scanned rather than having to scan
	   again and wonder which entry in their app is the real one. Nothing is
	   protected until /confirm, so an unconfirmed secret being handed back is
	   the same credential to the same authenticated person.
	*/
	secret, err := h.factors.Enrolling(r.Context(), pr.Subject)
	if err != nil || secret == "" {
		secret, err = identity.NewTOTPSecret()
		if err != nil {
			h.log.Error("could not mint a factor secret", "err", err)
			fail(w, http.StatusInternalServerError, "Could not start enrolment.")
			return
		}
		if err := h.factors.Enrol(r.Context(), pr.Subject, secret, "Authenticator app"); err != nil {
			h.refuse(w, err, "Could not start enrolment.")
			return
		}
	}

	send(w, http.StatusOK, map[string]string{
		"secret": secret,
		// The otpauth:// URI the QR code encodes. Built here rather than in the
		// browser so the two checks — what the app scanned and what the server
		// stored — cannot come from different strings.
		"uri": identity.TOTPURI(h.issuer, h.addressOf(r.Context(), pr), secret),
	})
}

/*
confirm proves the enrolment and issues the recovery codes.

The codes come back here and only here, for the same reason the secret does:
they are passwords, shown once, stored as hashes. Issuing them at this moment
rather than earlier means somebody who abandoned enrolment halfway never has a
set of live credentials for an account with no second factor.
*/
func (h *Factor) confirm(w http.ResponseWriter, r *http.Request, pr principal.Principal) {
	var in struct {
		Code string `json:"code"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		fail(w, http.StatusBadRequest, "Send the six-digit code from your app.")
		return
	}

	if err := h.factors.Confirm(r.Context(), pr.Subject, in.Code); err != nil {
		h.refuse(w, err, "Could not confirm your second factor.")
		return
	}

	codes, err := h.issueCodes(r.Context(), pr.Subject)
	if err != nil {
		// The factor is on and there are no recovery codes, which is a way to
		// lose an account. Said plainly, with the one action that fixes it.
		h.log.Error("could not issue recovery codes", "user", pr.Subject, "err", err)
		fail(w, http.StatusInternalServerError,
			"Your second factor is on, but recovery codes could not be issued. "+
				"Generate them from your account page before signing out.")
		return
	}

	h.log.Info("second factor enrolled", "user", pr.Subject)
	audit(r.Context(), h.log, pr, ActionFactorAdd, pr.Subject, Allowed, nil)

	out := map[string]any{"recoveryCodes": codes}

	/*
	   A session that could only enrol has finished enrolling.

	   Without this they would have to sign in again — with the password, and
	   now a code — immediately after proving both. Minting the unrestricted
	   session here is not a shortcut around the requirement: the requirement is
	   that they have a second factor, and they now do, thirty seconds after
	   proving it with a code from it.
	*/
	if pr.Enrol && h.signer != nil {
		issued, err := h.signer.Mint(token.Claims{
			Audience: token.Portal, Role: string(pr.ProjectRole),
			Org: pr.OrgID, Project: pr.ProjectID, Subject: pr.Subject,
			Platform: pr.Platform,
		}, SessionLifetime)
		if err != nil {
			// The factor is on and the codes are shown; they sign in again,
			// which now works. Worth logging and not worth failing over.
			h.log.Error("could not upgrade a session after enrolment", "err", err)
		} else {
			out["token"] = issued
			out["expiresIn"] = int(SessionLifetime.Seconds())
		}
	}

	send(w, http.StatusOK, out)
}

// regenerate replaces the recovery codes, retiring the old set.
func (h *Factor) regenerate(w http.ResponseWriter, r *http.Request, pr principal.Principal) {
	if _, err := h.factors.FactorOf(r.Context(), pr.Subject); err != nil {
		// No factor, so recovery codes would be ten live credentials for an
		// account they do not protect.
		fail(w, http.StatusConflict, "This account has no second factor to recover.")
		return
	}

	codes, err := h.issueCodes(r.Context(), pr.Subject)
	if err != nil {
		h.log.Error("could not regenerate recovery codes", "err", err)
		fail(w, http.StatusInternalServerError, "Could not generate new codes.")
		return
	}

	audit(r.Context(), h.log, pr, ActionFactorCodes, pr.Subject, Allowed, nil)
	send(w, http.StatusOK, map[string]any{"recoveryCodes": codes})
}

/*
remove turns protection off.

Their own, and only with a current code or a recovery code. Without that, a
stolen session is enough to strip the second factor off the account it stole —
which makes the factor protect nothing at the moment it matters most.
*/
func (h *Factor) remove(w http.ResponseWriter, r *http.Request, pr principal.Principal) {
	if pr.Enrol {
		// This session exists because the project requires a factor and this
		// account has none. There is nothing to remove, and a route that said
		// otherwise would be the way around the requirement.
		fail(w, http.StatusForbidden, "This project requires a second factor.")
		return
	}

	var in struct {
		Code string `json:"code"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		fail(w, http.StatusBadRequest, "Send a current code to turn this off.")
		return
	}

	if err := h.prove(r.Context(), pr.Subject, in.Code); err != nil {
		h.refuse(w, err, "Could not turn off your second factor.")
		return
	}
	if err := h.factors.RemoveFactor(r.Context(), pr.Subject); err != nil {
		h.refuse(w, err, "Could not turn off your second factor.")
		return
	}

	h.log.Warn("second factor removed", "user", pr.Subject)
	audit(r.Context(), h.log, pr, ActionFactorRemove, pr.Subject, Allowed, nil)
	w.WriteHeader(http.StatusNoContent)
}

// prove accepts either a current code or a recovery code.
//
// Both, because somebody removing a factor because they lost the phone has only
// the second kind — and telling them to use the app they no longer have is how
// this ends in an administrator doing it over chat instead.
func (h *Factor) prove(ctx context.Context, id, code string) error {
	if err := h.factors.CheckFactor(ctx, id, code); err == nil {
		return nil
	}
	return h.factors.SpendRecoveryCode(ctx, id, code)
}

// issueCodes mints a set and stores their hashes.
func (h *Factor) issueCodes(ctx context.Context, id string) ([]string, error) {
	codes, hashes, err := identity.NewRecoveryCodes()
	if err != nil {
		return nil, err
	}
	if err := h.factors.SetRecoveryCodes(ctx, id, hashes); err != nil {
		return nil, err
	}
	return codes, nil
}

// addressOf is what the authenticator app shows under the issuer.
func (h *Factor) addressOf(ctx context.Context, pr principal.Principal) string {
	if pr.Email != "" {
		return pr.Email
	}
	if h.roster == nil {
		return pr.Subject
	}
	people, err := h.roster.People(ctx, pr)
	if err != nil {
		return pr.Subject
	}
	for _, person := range people {
		if person.ID == pr.Subject {
			return person.Email
		}
	}
	return pr.Subject
}

// refuse maps the store's errors onto answers a person can act on.
func (h *Factor) refuse(w http.ResponseWriter, err error, generic string) {
	switch {
	case errors.Is(err, identity.ErrBadCode):
		fail(w, http.StatusUnauthorized,
			"That code is not right. Check the app is showing the current one.")
	case errors.Is(err, identity.ErrCodeUsed):
		fail(w, http.StatusUnauthorized,
			"That code has already been used. Wait for the next one.")
	case errors.Is(err, identity.ErrNoFactor):
		fail(w, http.StatusConflict, "This account has no second factor.")
	case errors.Is(err, identity.ErrFactorExists):
		fail(w, http.StatusConflict, "This account already has a second factor.")
	default:
		h.log.Error("second factor", "err", err)
		fail(w, http.StatusInternalServerError, generic)
	}
}
