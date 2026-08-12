package api

import (
	"context"
	"log/slog"
	"time"

	"github.com/gsoultan/cronos/internal/core/principal"
	"github.com/gsoultan/cronos/internal/extension"
)

// The actions this server records. Named constants rather than literals at the
// call sites, because an audit is queried by action and a typo makes one
// action into two that nobody notices until the query returns half of it.
const (
	ActionRead        = "report.read"
	ActionPublish     = "definition.publish"
	ActionDelete      = "definition.delete"
	ActionShare       = "share.create"
	ActionShareOpen   = "share.open"
	ActionRevoke      = "share.revoke"
	ActionSignIn      = "auth.signin"
	ActionSignOut     = "auth.signout"
	ActionSessionsEnd = "auth.sessions.end"
	// Who has access, and every change to it. The one part of an audit that
	// answers "how did they still have a login in March".
	ActionPersonAdd     = "person.add"
	ActionInvite        = "person.invite"
	ActionInviteAccept  = "person.invite.accept"
	ActionUninvite      = "person.invite.revoke"
	ActionPersonRole    = "person.role"
	ActionPersonDisable = "person.disable"
	ActionPersonEnable  = "person.enable"
	ActionPassword      = "auth.password"
	// Sending a report to somebody by name. Recorded with every recipient:
	// "who was this emailed to" is a question an audit exists to answer.
	ActionSend = "report.send"
)

// Results. Two, because an audit that only records what succeeded answers the
// wrong half of every question anybody asks it.
const (
	Allowed = "allowed"
	Refused = "refused"
)

/*
audit records one action.

The sink is a seam with a discarding default, so a build with nothing plugged
in pays a function call and does nothing. What matters is that the call sites
exist: wiring an audit sink into a product that never emits an event produces a
compliance answer of "we have the capability", which is not an answer.

A failure to record is logged at error and does not fail the request. That is a
choice with a cost either way — a read that happened and was not recorded is
exactly the hole an auditor asks about — and it is made this way because the
alternative is a reporting product that stops serving when a log sink is
unreachable. The error level is what makes it alertable, which is the part that
turns the choice into a decision somebody can act on.
*/
func audit(ctx context.Context, log *slog.Logger, pr principal.Principal,
	action, target, result string, detail map[string]any) {

	sink := extension.Audit()
	if sink == nil {
		return
	}

	// The request id, so an audit entry and the log lines around it are the
	// same story rather than two accounts of it.
	if id := RequestID(ctx); id != "" {
		if detail == nil {
			detail = map[string]any{}
		}
		detail["request"] = id
	}

	event := extension.Event{
		At:        time.Now().UTC(),
		Actor:     actor(pr),
		OrgID:     pr.OrgID,
		ProjectID: pr.ProjectID,
		Action:    action,
		Target:    target,
		Result:    result,
		Detail:    detail,
	}

	// Not the request's context. An audit entry for a request that was
	// cancelled is the entry most worth having, and writing it on a context
	// that has already been cancelled writes nothing.
	if err := sink.Record(context.WithoutCancel(ctx), event); err != nil {
		log.Error("audit not recorded",
			"action", action, "target", target, "actor", event.Actor, "err", err)
	}
}

// actor names who did it, in the terms this deployment has.
//
// A subject is whatever the minting host chose to put in the token — cronos
// never sees their user table — so it may be an id, an email, or a share. It
// is recorded as given rather than resolved, because resolving it would mean
// inventing a mapping we do not have.
func actor(pr principal.Principal) string {
	if pr.Subject == "" {
		return "anonymous"
	}
	return pr.Subject
}

// scopeOf describes the row constraint a read ran under.
//
// The values are included, and that is deliberate: "somebody read a report
// with some scope" answers nothing, and the whole question an audit exists to
// answer is which customer's rows were returned to whom. An audit log holds
// what it must to be an audit log, and is protected accordingly.
func scopeOf(pr principal.Principal) map[string]any {
	if len(pr.Scope) == 0 {
		// Said explicitly rather than omitted. A member reading the whole
		// project and an end customer reading their own rows are different
		// events, and an absent field reads as neither.
		return map[string]any{"scope": "none", "member": pr.Member}
	}
	scope := make(map[string]any, len(pr.Scope))
	for k, v := range pr.Scope {
		scope[k] = v
	}
	return map[string]any{"scope": scope, "member": pr.Member}
}
