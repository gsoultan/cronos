package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gsoultan/cronos/internal/core/identity"
	"github.com/gsoultan/cronos/internal/core/principal"
	"github.com/gsoultan/cronos/internal/platform/token"
)

/*
Invitations: adding somebody without choosing their password.

Adding a person used to mean typing a password into a form and telling them what
it was. That password then lives in whatever carried it — a chat message, a
ticket, somebody's sent folder — and is known to at least two people from the
moment it exists, one of whom cannot know how many more. "Change it when you
sign in" is advice, and advice is not a control.

This inverts it. The account does not exist until they accept; the only thing
that travels is a secret that stops working the moment it is used; and the
password is one only they have ever seen.

Two endpoints, and the asymmetry between them is the whole design:

  - POST /v1/people, with no password, needs an administrator's session.
  - /v1/auth/invitation needs none, because the person using it has no account
    yet. Its only credential is the secret from the email, which is why that
    secret is 256 random bits, single-use, expiring, and stored as a hash.
*/

// Invitations is what the API may ask about places held for people.
type Invitations interface {
	Invite(ctx context.Context, inv identity.Invitation, hash string) error
	Invitation(ctx context.Context, secret string) (identity.Invitation, error)
	Accept(ctx context.Context, secret, password string) (identity.User, error)
	Invitations(ctx context.Context, pr principal.Principal) ([]identity.Invitation, error)
	Uninvite(ctx context.Context, pr principal.Principal, id string) error
}

// Postman delivers an invitation.
//
// Its own interface rather than the delivery channel's, because an invitation
// is not a report: no attachment, no schedule, no run record. What they share
// is an SMTP connection, and that is the adapter's business rather than this
// package's.
type Postman interface {
	Post(ctx context.Context, to, subject, body string) error
}

/*
Invite is the sending half, mounted under /v1/people.

Kept beside People rather than inside it because the two failure modes are
different: adding somebody with a password fails or succeeds here, and inviting
them fails here, or at a mail server, or in a spam filter, or not at all until
somebody asks a week later why they never got an email.
*/
type Invite struct {
	invitations Invitations
	post        Postman
	// where the portal is, for building the link. Without it there is nothing
	// to put in the email, which is why it is checked at startup.
	portal string
	log    *slog.Logger
}

// NewInvite wires the sender.
func NewInvite(inv Invitations, post Postman, portal string, log *slog.Logger) *Invite {
	return &Invite{invitations: inv, post: post, portal: strings.TrimRight(portal, "/"), log: log}
}

// Available reports whether this deployment can invite anybody.
//
// A button that produces "no mail server is configured" is worse than no
// button, so the portal asks first and offers a password field instead.
func (h *Invite) Available() bool {
	return h != nil && h.invitations != nil && h.post != nil && h.portal != ""
}

/*
Send writes an invitation and mails it.

The order matters. Written first, then sent: a mail that goes out before the row
exists is a link that 404s, and somebody who clicks it learns nothing except
that this product is broken. The other way round leaves a row nobody can use,
which expires quietly in a week and can be sent again.
*/
func (h *Invite) Send(ctx context.Context, pr principal.Principal,
	invitedBy, email, name, role string) (identity.Invitation, error) {

	secret, hash, err := identity.NewInvitation(32)
	if err != nil {
		return identity.Invitation{}, err
	}

	inv := identity.Invitation{
		ID:    identity.NewInvitationID(),
		Email: email,
		Name:  name,
		// The acting principal's project, never one named in the request — the
		// same rule as adding somebody directly. A body that could choose
		// would be an administrator of one project inviting into another.
		Org:     pr.OrgID,
		Project: pr.ProjectID,
		Role:    role,
		/*
		   The inviter's address, resolved by the caller from the roster.

		   Not taken from the principal: a portal token carries a subject and
		   no email, so reading it off there yields "usr_9f2c4a…" — which the
		   recipient sees at the top of an email asking them to click a link
		   and type a password, and which tells them nothing about whether it
		   is genuine. The one thing they can actually check is whether it
		   names somebody they know.
		*/
		InvitedBy: firstOf(invitedBy, pr.Email),
		Expires:   time.Now().Add(identity.InvitationLife),
	}

	if err := h.invitations.Invite(ctx, inv, hash); err != nil {
		return identity.Invitation{}, err
	}

	if err := h.post.Post(ctx, email, h.subject(inv), h.body(inv, secret)); err != nil {
		// The row stays. It expires on its own, and leaving it means the
		// administrator can resend rather than being told to try again with no
		// idea whether the first one went.
		h.log.Error("could not send an invitation", "to", email, "err", err)
		return inv, fmt.Errorf("invitation: written but not sent: %w", err)
	}
	return inv, nil
}

func (h *Invite) subject(inv identity.Invitation) string {
	return fmt.Sprintf("You have been invited to %s on cronos", inv.Project)
}

/*
body is the email.

Plain text, because an invitation asking somebody to click a link and type a
password is exactly what a phishing email looks like, and the difference a
recipient can actually check is whether it reads like a person wrote it and
names somebody they know.

The secret goes in the fragment, after the `#`. A browser does not send a
fragment to any server, so the link does not reach the portal's access log, its
CDN, or a Referer header on the way to whatever the page loads next. The page
reads it out of location.hash and posts it back.
*/
func (h *Invite) body(inv identity.Invitation, secret string) string {
	who := inv.InvitedBy
	if who == "" {
		who = "Somebody"
	}
	name := inv.Name
	if name == "" {
		name = "Hello"
	} else {
		name = "Hello " + name
	}

	return fmt.Sprintf(`%s,

%s has invited you to the %s project on cronos, as %s.

Set a password and sign in:

  %s

The link works once and expires on %s. If you were not expecting this, you can
ignore it — nothing has been created in your name, and the link stops working
by itself.
`,
		name, who, inv.Project, article(inv.Role), h.link(secret),
		inv.Expires.Format("2 January 2006"))
}

// link is where the person goes.
func (h *Invite) link(secret string) string {
	return h.portal + "/invitation#secret=" + url.QueryEscape(secret)
}

// article reads "an editor" rather than "a editor".
func article(role string) string {
	if role == "" {
		return "a viewer"
	}
	if strings.ContainsRune("aeiou", rune(role[0])) {
		return "an " + role
	}
	return "a " + role
}

/*
Accepting: the one endpoint in this API with no session.

Everything else here is reached by somebody who has already proved who they are.
This is reached by somebody who by definition has not, and cannot — the whole
point is that they do not have an account yet. So the secret is the only
credential, and every property it needs is a property of the secret: 256 bits of
randomness, one use, an expiry, and a hash rather than the thing itself in the
database.

GET says who the invitation is for, so the page can address them by name rather
than asking a stranger to type a password into an unlabelled box. POST spends
it.
*/
type Acceptance struct {
	invitations Invitations
	signer      *token.Signer
	// org, project and role come from the invitation, never the request.
	log *slog.Logger
}

// NewAcceptance wires the handler.
//
// Rate limiting is the mount's business, like every other route: this one is
// wrapped per address in server.go. The secret is unguessable, so a limit is
// not what stops an attack on it — it stops an unauthenticated endpoint that
// runs bcrypt from being a way to spend somebody's CPU.
func NewAcceptance(inv Invitations, s *token.Signer, log *slog.Logger) *Acceptance {
	return &Acceptance{invitations: inv, signer: s, log: log}
}

// AcceptRate and AcceptBurst throttle it. Five a minute with ten in hand:
// somebody setting a password mistypes it and tries again; nobody does it three
// hundred times.
const (
	AcceptRate  = 5.0 / 60
	AcceptBurst = 10
)

type acceptance struct {
	Secret   string `json:"secret"`
	Password string `json:"password"`
}

// ServeHTTP handles GET and POST /v1/auth/invitation.
func (h *Acceptance) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.describe(w, r)
	case http.MethodPost:
		h.accept(w, r)
	default:
		fail(w, http.StatusMethodNotAllowed, "Not a method this endpoint takes.")
	}
}

// describe says who an invitation is for, without spending it.
func (h *Acceptance) describe(w http.ResponseWriter, r *http.Request) {
	inv, err := h.invitations.Invitation(r.Context(), r.URL.Query().Get("secret"))
	if err != nil {
		h.refuse(w, err)
		return
	}

	// Deliberately narrow. Enough to say "you are joining acme/finance as an
	// editor, at this address" and nothing about who else is there.
	send(w, http.StatusOK, map[string]string{
		"email": inv.Email, "name": inv.Name,
		"org": inv.Org, "project": inv.Project, "role": inv.Role,
		"invitedBy": inv.InvitedBy,
	})
}

// accept turns the invitation into an account and signs them in.
func (h *Acceptance) accept(w http.ResponseWriter, r *http.Request) {
	var in acceptance
	if err := decodeJSON(w, r, &in); err != nil {
		fail(w, http.StatusBadRequest, "Send a secret and a password.")
		return
	}
	// Checked here as well as in the store, so somebody who typed six
	// characters is told so before anything is spent.
	if err := identity.Acceptable(in.Password); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}

	user, err := h.invitations.Accept(r.Context(), in.Secret, in.Password)
	if err != nil {
		h.refuse(w, err)
		return
	}

	/*
	   Signed in immediately.

	   They have just proved control of the mailbox the invitation went to and
	   chosen a password nobody else has seen, which is more than a sign-in
	   form asks for. Making them type it again at a login page is a step that
	   proves nothing and is where people give up.
	*/
	issued, err := h.signer.Mint(token.Claims{
		Audience: token.Portal, Role: user.Role,
		Org: user.Org, Project: user.Project, Subject: user.ID,
	}, SessionLifetime)
	if err != nil {
		h.log.Error("could not mint a session after an invitation", "err", err)
		// The account exists and the invitation is spent, so this is not a
		// failure to accept — it is a failure to skip the login page.
		send(w, http.StatusCreated, map[string]any{"user": user})
		return
	}

	h.log.Info("invitation accepted", "user", user.ID, "role", user.Role,
		"project", user.Org+"/"+user.Project)
	audit(r.Context(), h.log, principal.Principal{
		Subject: user.ID, Email: user.Email, OrgID: user.Org, ProjectID: user.Project,
	}, ActionInviteAccept, user.Email, Allowed, map[string]any{"role": user.Role})

	send(w, http.StatusCreated, map[string]any{
		"token":     issued,
		"expiresIn": int(SessionLifetime.Seconds()),
		"user":      user,
	})
}

// refuse answers every unusable invitation the same way.
func (h *Acceptance) refuse(w http.ResponseWriter, err error) {
	if errors.Is(err, identity.ErrInvitation) || errors.Is(err, identity.ErrExists) {
		// Expired, spent, withdrawn, never existed, or overtaken by an account
		// created another way. One sentence for all of them: this endpoint has
		// no session, and telling them apart is a way to learn which addresses
		// somebody is onboarding.
		fail(w, http.StatusGone,
			"This invitation cannot be used. Ask whoever invited you for a new one.")
		return
	}
	h.log.Error("could not accept an invitation", "err", err)
	fail(w, http.StatusInternalServerError, "Could not accept this invitation.")
}
