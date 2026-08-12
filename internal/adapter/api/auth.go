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
	"github.com/gsoultan/cronos/internal/core/principal"
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
	// attempts throttles per account, which is the attack the per-address
	// limit on this route does not see: many machines, one email.
	attempts *Limit
	// factors is the second step, where this deployment has somewhere to
	// record one.
	factors Factors
	/*
	   codes throttles the second step, separately from the password.

	   They are different attacks and adding 2FA to one budget broke the other.
	   The password limit is tight because a password list is long; a code is
	   six digits from a window of three, so guessing is hopeless at any rate a
	   person would notice — and the person is the one retrying, because the
	   code expired, or they mistyped it, or their phone's clock drifted.

	   Sharing one budget meant three mistyped codes locked somebody out of
	   their own password for a minute. A live run found it by making a dozen
	   attempts in two seconds, which is what a person having a bad morning
	   does more slowly.
	*/
	codes *Limit
}

// WithFactors adds the second step. Absent — a file-backed deployment — sign-in
// is the password alone, because there is nowhere to record a factor.
func (a *Auth) WithFactors(f Factors) *Auth {
	a.factors = f
	return a
}

/*
checkFactor accepts a TOTP code or a recovery code.

Both at one field, because the person signing in knows which they have and does
not care what it is called. A separate "use a recovery code instead" path is a
second form to build, a second thing to rate limit, and a second answer that
differs — and a differing answer is how somebody learns which accounts have
recovery codes left.
*/
func (a *Auth) checkFactor(ctx context.Context, id, code string) error {
	err := a.factors.CheckFactor(ctx, id, code)
	if err == nil || errors.Is(err, identity.ErrCodeUsed) {
		// A used code is not retried as a recovery code: it was a real one,
		// and saying so is more useful than "that code is not right".
		return err
	}
	return a.factors.SpendRecoveryCode(ctx, id, code)
}

// factorMessage says what to do without saying what was tried.
func factorMessage(err error) string {
	if errors.Is(err, identity.ErrCodeUsed) {
		return "That code has already been used. Wait for the next one."
	}
	return "That code is not right."
}

// NewAuth wires the handler.
func NewAuth(u Users, s *token.Signer, log *slog.Logger) *Auth {
	return &Auth{
		users: u, signer: s, log: log,
		// Five a minute with ten in hand. Mistyping a password three times is
		// ordinary; three hundred attempts against one address is not.
		attempts: NewLimit(signInRate, signInBurst),
		// Ten a minute with twenty in hand. Generous, because the legitimate
		// retry is common and the illegitimate one is arithmetic: six digits
		// with a window of three is three chances in a million per guess, and
		// at this rate an attacker who already has the password needs
		// somewhere upward of nine years for an even chance.
		codes: NewLimit(codeRate, codeBurst),
	}
}

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	/*
	   Code is the second factor, sent with the password rather than after it.

	   One exchange rather than two, which is a deliberate choice. A two-step
	   sign-in needs a challenge that says "this password was right" — and that
	   challenge is a credential worth stealing, a thing to expire, a thing to
	   rate limit, and a way to learn which accounts have a second factor by
	   watching which ones issue one.

	   Sending both together avoids all of it. The cost is that the portal has
	   to know to ask for a code, which it learns by being told `factorRequired`
	   after a first attempt with the password alone — an answer that is only
	   ever given to somebody who already proved the password.
	*/
	Code string `json:"code,omitempty"`
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

	/*
	   Per account, on top of the per-address limit the route already carries.

	   The two catch different attacks. An address limit stops one machine
	   working through a password list; it does nothing about a thousand
	   machines each trying twice against one known address. This is a rate
	   rather than a lockout on purpose: a lockout is a denial of service
	   somebody else can trigger against a real person by guessing wrong on
	   their behalf, and the point is to make guessing slow, not to hand an
	   attacker a way to lock the account out.
	*/
	account := strings.ToLower(strings.TrimSpace(in.Email))
	if account != "" && !a.attempts.Allow(account) {
		a.log.Warn("sign-in throttled", "email", redact(in.Email))
		audit(r.Context(), a.log, principal.Principal{Subject: in.Email},
			ActionSignIn, in.Email, Refused, map[string]any{"reason": "too many attempts"})
		// The same wording as a wrong password, and the same status. Telling
		// somebody they have hit a limit tells them the account exists.
		fail(w, http.StatusUnauthorized, "That email and password do not match.")
		return
	}

	user, err := a.users.Authenticate(r.Context(), in.Email, in.Password)
	if err == nil {
		/*
		   The password was right, so this limiter has nothing left to say.

		   Forgetting here rather than after the whole sign-in succeeds is what
		   stops the second step spending the password's budget. It is safe for
		   the reason the limiter exists: it is there to slow somebody working
		   through a password list, and a correct password is proof that is not
		   what is happening. An attacker who reaches this line has already
		   beaten it.
		*/
		a.attempts.Forget(account)
	}
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

	/*
	   The second factor, where there is one.

	   After the password, always. Asking for a code first — or telling somebody
	   which accounts have one — turns this into a way to enumerate who has
	   protection worth attacking. Reaching this point means the password was
	   right, and saying "now the code" to somebody who has already proved that
	   tells them nothing they did not know.
	*/
	if a.factors != nil && a.factors.Protected(r.Context(), user.ID) {
		if in.Code == "" {
			// Not an error. The portal shows a code field and asks again, and
			// the attempt is not counted against the throttle because nothing
			// was guessed.
			send(w, http.StatusOK, map[string]any{"factorRequired": true})
			return
		}
		// Its own budget, spent only when a code is actually offered. The
		// answer above — "a code is needed" — guesses nothing and costs
		// nothing, and must not reset this either, or somebody holding the
		// password could clear the counter between attempts.
		if !a.codes.Allow(user.ID) {
			a.log.Warn("second factor throttled", "user", user.ID)
			fail(w, http.StatusTooManyRequests, "Too many codes. Wait a minute.")
			return
		}
		if err := a.checkFactor(r.Context(), user.ID, in.Code); err != nil {
			a.log.Info("second factor refused", "user", user.ID, "err", err)
			audit(r.Context(), a.log, principal.Principal{Subject: user.ID, Email: user.Email},
				ActionSignIn, user.Email, Refused, map[string]any{"reason": "second factor"})
			// Counted, unlike a missing code: this was a guess.
			fail(w, http.StatusUnauthorized, factorMessage(err))
			return
		}
	}

	issued, err := a.signer.Mint(token.Claims{
		Audience: token.Portal, Role: user.Role,
		Org: user.Org, Project: user.Project, Subject: user.ID,
		Platform: user.Platform,
	}, SessionLifetime)
	if err != nil {
		a.log.Error("could not mint a session", "err", err)
		fail(w, http.StatusInternalServerError, "Could not start a session.")
		return
	}

	a.codes.Forget(user.ID)
	a.log.Info("signed in", "user", user.ID, "project", user.Org+"/"+user.Project, "role", user.Role)
	audit(r.Context(), a.log, principal.Principal{
		Subject: user.ID, Email: user.Email,
		OrgID: user.Org, ProjectID: user.Project,
	}, ActionSignIn, user.Email, Allowed, map[string]any{"role": user.Role})
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
