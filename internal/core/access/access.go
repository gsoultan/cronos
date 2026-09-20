/*
Package access decides who may open which report.

A project role answers "may this person read reports here", which is one
question short. A finance department has people who should read the receivables
summary and people who should not, and until this existed the only way to
express that was a second project — which splits the datasets too, and is a
sledgehammer for a permission.

Grants are recorded against a report and never inside it. A definition is
content-addressed and versioned, so putting access in the YAML would make every
permission change a new version of the report and every grant an editor's job.
Who may read something is an administrator's decision about people, not an
author's decision about content.

# Absent means open

A report nobody has granted is readable by the project, exactly as before this
package existed. It becomes restricted by its first grant and not a moment
earlier.

That is the opposite of the usual instinct, and it is deliberate. Closed-until-
granted turns an upgrade into an outage for every report in every deployment —
the burst at 06:00 delivers nothing, and the operator's first sign is a customer
asking where their statement went. A feature nobody asked for must not be able
to do that. The cost is that restricting a report is an act somebody has to
take, which is the same cost every allow-list has.
*/
package access

import (
	"errors"

	"github.com/gsoultan/cronos/internal/core/principal"
)

/*
ErrNoSuchSubject means a grant named somebody or something that is not here.

Worth a sentinel rather than a generic refusal, because of which way this fails.
A grant to an account id that does not exist matches nobody — and the first
grant on a report is what makes it restricted, so a mistyped id does not grant
nothing, it takes the report away from everybody who could read it before and
gives it to no one. The report goes quiet and the list of grants looks right.

Named here because it is the domain's rule and two packages need the word: the
store detects it, and the API turns it into a sentence saying so rather than
"that is not a grant".
*/
var ErrNoSuchSubject = errors.New("access: no such subject in this project")

// Kind is what a grant names.
type Kind string

const (
	// KindUser grants one person by account id.
	KindUser Kind = "user"
	// KindGroup grants everybody in a named group.
	KindGroup Kind = "group"
	/*
	   KindInvited grants somebody who has been invited and has not accepted,
	   by the address they were invited at.

	   It exists because the alternative is worse than a missing feature. A
	   report is restricted by its first grant, so "invite Sam and give them
	   the receivables summary" had to be done in that order and then
	   remembered — and the way it was remembered was an administrator coming
	   back days later, if at all. Granting it up front was refused, because
	   there is no account to name yet.

	   It matches nobody. There is no session to match: the row is rewritten
	   to a KindUser grant naming the new account the moment the invitation is
	   accepted, which is where it starts opening anything. Until then it is a
	   promise, and it does restrict the report — which is what the
	   administrator asked for.
	*/
	KindInvited Kind = "invited"
)

/*
Group is a named set of people and the row scope its members read through.

Here rather than in the store or the API because both need to name it and Go
interfaces match on exact types: a consumer-declared interface returning the
API's copy is one the store cannot satisfy, and the failure is silent — the
assertion fails, the handler is never wired, and every test stays green.
*/
type Group struct {
	ID    string            `json:"id"`
	Name  string            `json:"name"`
	Scope map[string]string `json:"scope,omitempty"`
	// Members is how many people are in it, for a list that would otherwise
	// need a query per row to say anything useful.
	Members int `json:"members"`
}

// Grant is one permission an administrator gave.
type Grant struct {
	Report  string `json:"report"`
	Kind    Kind   `json:"kind"`
	Subject string `json:"subject"`
}

/*
Allowed reports whether pr may open a report, given every grant recorded
against that report and the groups pr belongs to.

grants must already be filtered to the one report being asked about. Passing
the whole project's grants would silently grant every report to anyone holding
one — a shape worth refusing rather than documenting, which is why Report is on
the struct and asserted in the tests.

Administrators are not subject to grants. They are who adds and removes them,
so a deployment where an admin can be locked out of a report is a deployment
where the recovery path is a psql prompt — the same reasoning docs/tenancy.md
already gives for an org owner entering any project.
*/
func Allowed(pr principal.Principal, grants []Grant, groups []string) bool {
	if !pr.CanRead() {
		// Not in the project at all. Grants are the second question and this
		// is the first; reaching here without membership would let a grant
		// substitute for one.
		return false
	}
	if pr.CanAdminProject() {
		return true
	}
	if len(grants) == 0 {
		// Nobody restricted it. See the package comment: this is what makes an
		// upgrade a no-op rather than an outage.
		return true
	}

	for _, g := range grants {
		switch g.Kind {
		case KindUser:
			if g.Subject != "" && g.Subject == pr.Subject {
				return true
			}
		case KindInvited:
			/*
			   Never. Said out loud rather than left to the default, because
			   the subject here is an email address and the one mistake this
			   case exists to prevent is somebody matching it against one.

			   An invitation is not an account. It becomes a KindUser grant
			   when it is accepted; before that there is nobody holding it,
			   and a session that happened to carry a matching address would
			   be an account that was never created by this invitation.
			*/
		case KindGroup:
			for _, in := range groups {
				if g.Subject != "" && g.Subject == in {
					return true
				}
			}
		}
	}
	return false
}

/*
Restricted reports whether anybody has narrowed this report.

Separate from Allowed because the two answer different questions and a caller
that conflates them shows the wrong thing: "you may not open this" and "this is
open to everybody" are both false for a report you were granted, and a
catalogue needs to tell them apart to put a padlock next to the right entries.
*/
func Restricted(grants []Grant) bool { return len(grants) > 0 }

/*
For narrows a project's grants to one report.

A helper rather than a caller's loop, because the one mistake this package can
make is judging a report against another report's grants, and it should be
possible to grep for every place that filtering happens.
*/
func For(report string, grants []Grant) []Grant {
	out := make([]Grant, 0, len(grants))
	for _, g := range grants {
		if g.Report == report {
			out = append(out, g)
		}
	}
	return out
}
