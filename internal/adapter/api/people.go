package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gsoultan/cronos/internal/core/identity"
	"github.com/gsoultan/cronos/internal/core/principal"
)

// Roster is what the API may ask about the people in a project.
type Roster interface {
	People(ctx context.Context, pr principal.Principal) ([]identity.User, error)
	CreateUser(ctx context.Context, u identity.User, password string) error
	SetRole(ctx context.Context, pr principal.Principal, id, role string) error
	SetDisabled(ctx context.Context, pr principal.Principal, id string, disabled bool) error
	ChangePassword(ctx context.Context, id, current, next string) error
}

/*
People serves who has access, and the ways that changes.

Until this existed the answer to "somebody left, take away their access" was a
SQL statement against production. The column that does it has been in the
schema since the first migration and nothing ever wrote to it.

Administering is admin-only, which is a narrower bar than everything else in
this API: an editor may change what a report says, and that is a mistake
somebody can see and undo. Adding a person, or removing one, is neither.
*/
type People struct {
	roster Roster
	auth   Principals
	// invitations is nil where this deployment cannot send mail, in which case
	// adding somebody still means choosing their password.
	invitations *Invite
	log         *slog.Logger
}

// NewPeople wires the handler.
func NewPeople(r Roster, a Principals, log *slog.Logger) *People {
	return &People{roster: r, auth: a, log: log}
}

// Inviting adds the option to mail somebody instead of choosing for them.
func (h *People) Inviting(inv *Invite) *People {
	if inv.Available() {
		h.invitations = inv
	}
	return h
}

// ServeHTTP handles /v1/people and /v1/people/{id}.
func (h *People) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.auth.Principal(r)
	if !ok {
		fail(w, http.StatusUnauthorized, "Not authorised.")
		return
	}
	if !pr.CanAdminProject() {
		// The list too, not only the changes. Who works here, what their
		// addresses are and when each of them last signed in is a description
		// of an organisation, and a viewer has no call for it.
		fail(w, http.StatusForbidden, "Only a project administrator may manage people.")
		return
	}

	id := r.PathValue("id")
	// An invitation is not a person yet, so it lives under its own path rather
	// than in the roster with a flag on it. An account that does not exist and
	// an account that is disabled are different things, and one list holding
	// both is a list where "remove them" means two operations.
	if strings.HasPrefix(r.URL.Path, "/v1/people/invitations") {
		switch {
		case r.Method == http.MethodGet && id == "":
			h.pending(w, r, pr)
		case r.Method == http.MethodDelete && id != "":
			h.uninvite(w, r, pr, id)
		default:
			fail(w, http.StatusMethodNotAllowed, "Not a method this endpoint takes.")
		}
		return
	}

	switch {
	case r.Method == http.MethodGet && id == "":
		h.list(w, r, pr)
	case r.Method == http.MethodPost && id == "":
		h.add(w, r, pr)
	case r.Method == http.MethodPatch && id != "":
		h.amend(w, r, pr, id)
	default:
		fail(w, http.StatusMethodNotAllowed, "Not a method this endpoint takes.")
	}
}

func (h *People) list(w http.ResponseWriter, r *http.Request, pr principal.Principal) {
	people, err := h.roster.People(r.Context(), pr)
	if err != nil {
		h.log.Error("could not read the roster", "err", err)
		fail(w, http.StatusInternalServerError, "Could not read who has access.")
		return
	}
	if people == nil {
		people = []identity.User{}
	}
	// Whether this deployment can invite is answered here rather than on the
	// unauthenticated methods endpoint: it describes the deployment's mail
	// configuration, and only somebody who may add people needs to know.
	send(w, http.StatusOK, map[string]any{
		"people": people, "canInvite": h.invitations != nil,
	})
}

type newPerson struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Role     string `json:"role"`
	Password string `json:"password"`
}

/*
add registers somebody, one of two ways.

With no password, an invitation: the account is not created, a single-use secret
is mailed to them, and the password is one only they ever see. That is the right
default and what the portal asks for when this deployment can send mail.

With a password, the account is created immediately and somebody else has chosen
their credential. Kept because a deployment with no mail server has to be able
to add its second administrator somehow, and because the first one is created by
the CLI the same way. It is the weaker path and the portal says so.
*/
func (h *People) add(w http.ResponseWriter, r *http.Request, pr principal.Principal) {
	var in newPerson
	if err := decodeJSON(w, r, &in); err != nil {
		fail(w, http.StatusBadRequest, "Send an email, a name and a role.")
		return
	}
	if !validRole(in.Role) {
		fail(w, http.StatusBadRequest, "A role is admin, editor or viewer.")
		return
	}

	if in.Password == "" {
		h.invite(w, r, pr, in)
		return
	}
	if err := identity.Acceptable(in.Password); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}

	// The acting principal's project, never one named in the request. A body
	// that could choose the project is an administrator of one project adding
	// themselves to another.
	user := identity.User{
		ID:      identity.NewID(),
		Email:   in.Email,
		Name:    in.Name,
		Org:     pr.OrgID,
		Project: pr.ProjectID,
		Role:    in.Role,
	}

	if err := h.roster.CreateUser(r.Context(), user, in.Password); err != nil {
		if errors.Is(err, identity.ErrExists) {
			fail(w, http.StatusConflict, "That email already has an account here.")
			return
		}
		h.log.Error("could not add a person", "err", err)
		fail(w, http.StatusInternalServerError, "Could not add them.")
		return
	}

	h.log.Info("person added", "user", user.ID, "role", user.Role, "by", pr.Subject)
	audit(r.Context(), h.log, pr, ActionPersonAdd, in.Email, Allowed,
		map[string]any{"role": in.Role})
	send(w, http.StatusCreated, user)
}

type amendment struct {
	// Pointers, so "not mentioned" and "set to false" are different requests.
	// A PATCH that could not express "leave this alone" would make every role
	// change also an enable.
	Role     *string `json:"role,omitempty"`
	Disabled *bool   `json:"disabled,omitempty"`
}

func (h *People) amend(w http.ResponseWriter, r *http.Request, pr principal.Principal, id string) {
	var in amendment
	if err := decodeJSON(w, r, &in); err != nil {
		fail(w, http.StatusBadRequest, "Send a role, or whether they are disabled.")
		return
	}

	// Nobody removes their own access. An administrator who disables
	// themselves by mistake leaves a project with one fewer administrator and
	// no way to undo it, and if they are the last one, none at all.
	if id == pr.Subject && in.Disabled != nil && *in.Disabled {
		fail(w, http.StatusConflict, "You cannot disable your own account.")
		return
	}
	if id == pr.Subject && in.Role != nil && *in.Role != string(principal.ProjectAdmin) {
		fail(w, http.StatusConflict, "You cannot take away your own administrator role.")
		return
	}

	if in.Role != nil {
		if !validRole(*in.Role) {
			fail(w, http.StatusBadRequest, "A role is admin, editor or viewer.")
			return
		}
		if err := h.roster.SetRole(r.Context(), pr, id, *in.Role); err != nil {
			h.refuse(w, err)
			return
		}
		h.log.Info("role changed", "user", id, "role", *in.Role, "by", pr.Subject)
		audit(r.Context(), h.log, pr, ActionPersonRole, id, Allowed,
			map[string]any{"role": *in.Role})
	}

	if in.Disabled != nil {
		if err := h.roster.SetDisabled(r.Context(), pr, id, *in.Disabled); err != nil {
			h.refuse(w, err)
			return
		}
		action := ActionPersonEnable
		if *in.Disabled {
			action = ActionPersonDisable
		}
		h.log.Info("access changed", "user", id, "disabled", *in.Disabled, "by", pr.Subject)
		audit(r.Context(), h.log, pr, action, id, Allowed, nil)
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *People) refuse(w http.ResponseWriter, err error) {
	if errors.Is(err, identity.ErrNoUser) {
		fail(w, http.StatusNotFound, "No such person in this project.")
		return
	}
	h.log.Error("could not change access", "err", err)
	fail(w, http.StatusInternalServerError, "Could not change their access.")
}

/*
Password changes somebody's own password.

Their own, and nobody else's. An administrator resetting a password is a
different feature with a different risk — it is the one an attacker who
compromises an admin session wants — and it needs a delivery channel to be
worth having, because the reset has to reach the person rather than the person
who triggered it.
*/
type Password struct {
	roster Roster
	auth   Principals
	log    *slog.Logger
}

// NewPassword wires the handler.
func NewPassword(r Roster, a Principals, log *slog.Logger) *Password {
	return &Password{roster: r, auth: a, log: log}
}

type passwordChange struct {
	Current string `json:"current"`
	New     string `json:"new"`
}

// ServeHTTP handles POST /v1/auth/password.
func (h *Password) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.auth.Principal(r)
	if !ok {
		fail(w, http.StatusUnauthorized, "Not authorised.")
		return
	}
	if r.Method != http.MethodPost {
		fail(w, http.StatusMethodNotAllowed, "Use POST.")
		return
	}

	var in passwordChange
	if err := decodeJSON(w, r, &in); err != nil {
		fail(w, http.StatusBadRequest, "Send the current password and the new one.")
		return
	}
	if err := identity.Acceptable(in.New); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.roster.ChangePassword(r.Context(), pr.Subject, in.Current, in.New); err != nil {
		h.log.Info("password change refused", "user", pr.Subject)
		audit(r.Context(), h.log, pr, ActionPassword, pr.Subject, Refused, nil)
		// The same answer whether the current password was wrong or the
		// account is not one that has a password — a session borrowed for a
		// minute should not learn anything from trying.
		fail(w, http.StatusForbidden, "That is not your current password.")
		return
	}

	h.log.Info("password changed", "user", pr.Subject)
	audit(r.Context(), h.log, pr, ActionPassword, pr.Subject, Allowed, nil)
	// The session is deliberately not ended. Changing a password on the device
	// you are holding should not log you out of the device you are holding —
	// and the sessions that matter are the other ones, which this cannot reach
	// because a signed token has no server-side record to delete.
	w.WriteHeader(http.StatusNoContent)
}

func validRole(role string) bool {
	switch principal.Role(role) {
	case principal.ProjectAdmin, principal.ProjectEditor, principal.ProjectViewer:
		return true
	default:
		return false
	}
}

// decodeJSON reads a small JSON body, refusing keys nothing knows about.
func decodeJSON(w http.ResponseWriter, r *http.Request, into any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10))
	dec.DisallowUnknownFields()
	return dec.Decode(into)
}

/*
invite mails somebody a link instead of creating their account.

Refused rather than silently downgraded when this deployment cannot send mail.
An administrator who left the password field empty meant to invite; creating an
account with a password of the server's choosing, or with none, would be a
different thing done quietly.
*/
func (h *People) invite(w http.ResponseWriter, r *http.Request,
	pr principal.Principal, in newPerson) {

	if h.invitations == nil {
		fail(w, http.StatusBadRequest,
			"This deployment cannot send email, so a new person needs a password here.")
		return
	}

	inv, err := h.invitations.Send(r.Context(), pr, h.addressOf(r.Context(), pr),
		in.Email, in.Name, in.Role)
	if err != nil {
		if errors.Is(err, identity.ErrExists) {
			fail(w, http.StatusConflict, "That email already has an account here.")
			return
		}
		if inv.ID != "" {
			// Written but not delivered. Saying which half failed is the
			// difference between "try again" and "check the mail server",
			// and an administrator can do something about the second.
			h.log.Error("invitation not delivered", "to", in.Email, "err", err)
			fail(w, http.StatusBadGateway,
				"They were invited, but the email could not be sent. Check the mail server.")
			return
		}
		h.log.Error("could not invite", "to", in.Email, "err", err)
		fail(w, http.StatusInternalServerError, "Could not invite them.")
		return
	}

	h.log.Info("person invited", "to", in.Email, "role", in.Role, "by", pr.Subject)
	audit(r.Context(), h.log, pr, ActionInvite, in.Email, Allowed,
		map[string]any{"role": in.Role, "expires": inv.Expires})
	send(w, http.StatusCreated, inv)
}

// invitations lists the places held for people who have not arrived.
func (h *People) pending(w http.ResponseWriter, r *http.Request, pr principal.Principal) {
	if h.invitations == nil {
		send(w, http.StatusOK, []identity.Invitation{})
		return
	}
	out, err := h.invitations.invitations.Invitations(r.Context(), pr)
	if err != nil {
		h.log.Error("could not list invitations", "err", err)
		fail(w, http.StatusInternalServerError, "Could not read the invitations.")
		return
	}
	send(w, http.StatusOK, out)
}

// uninvite withdraws one that has not been used.
func (h *People) uninvite(w http.ResponseWriter, r *http.Request,
	pr principal.Principal, id string) {

	if h.invitations == nil {
		fail(w, http.StatusNotFound, "No such invitation.")
		return
	}
	if err := h.invitations.invitations.Uninvite(r.Context(), pr, id); err != nil {
		if errors.Is(err, identity.ErrInvitation) {
			fail(w, http.StatusNotFound, "No such invitation.")
			return
		}
		h.log.Error("could not withdraw an invitation", "err", err)
		fail(w, http.StatusInternalServerError, "Could not withdraw it.")
		return
	}

	audit(r.Context(), h.log, pr, ActionUninvite, id, Allowed, nil)
	w.WriteHeader(http.StatusNoContent)
}

/*
addressOf is the acting person's own email, for the invitation to name.

From the roster, because a portal token carries a subject and no address — and
an invitation signed "usr_9f2c4a…" is one the recipient cannot tell from a
phishing attempt. Empty when they are not in the roster, which is a machine
credential inviting somebody, and in that case the email says "Somebody" rather
than an id nobody recognises.

A whole-roster read for one address. A project's roster is tens of rows and this
happens once per invitation, which is a worse trade than a query would be and a
better one than a second method on the interface for it.
*/
func (h *People) addressOf(ctx context.Context, pr principal.Principal) string {
	if pr.Email != "" {
		return pr.Email
	}
	people, err := h.roster.People(ctx, pr)
	if err != nil {
		return ""
	}
	for _, person := range people {
		if person.ID == pr.Subject {
			return person.Email
		}
	}
	return ""
}
