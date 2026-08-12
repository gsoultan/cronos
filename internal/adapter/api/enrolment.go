package api

import (
	"net/http"
	"strings"
)

/*
The gate for a session that may only set up a second factor.

When a project requires one and somebody has none, they still sign in — refusing
the sign-in locks a team out of its own reporting on the afternoon the
requirement is switched on, and there is no self-service way back from that. What
they get instead is a session that reaches the enrolment endpoints and nothing
else.

One place, wrapping the whole mux, and an allow-list rather than a deny-list.
That direction is the entire design: a route added tomorrow is refused to these
sessions until somebody deliberately lists it, whereas a deny-list would let
every new route through and nobody would notice. The cost of the allow-list
being wrong is a person who cannot reach something during enrolment; the cost of
a deny-list being wrong is somebody browsing a customer's reports on a session
that was supposed to do one thing.
*/

/*
duringEnrolment is everything such a session may reach.

Kept as short as it can be and no shorter. Each entry is here because the
enrolment page genuinely needs it:

  - the factor routes are the point;
  - the profile is how the page addresses somebody by name rather than asking a
    stranger to scan a QR code;
  - signing out has to work, or the way out of a half-finished enrolment is
    clearing browser storage by hand;
  - health is unauthenticated anyway and answering it differently here would be
    a difference with no reason.
*/
var duringEnrolment = []string{
	"/v1/auth/factor",
	"/v1/auth/profile",
	"/v1/auth/sso/logout",
	"/v1/auth/sessions/end",
	"/v1/health",
	"/v1/setup",
}

/*
OnlyEnrolment refuses a restricted session everything but the enrolment routes.

Wrapped around the mux rather than checked per handler, for the reason every
security check in this package that lives in one place lives in one place: a
check repeated per handler is a check somebody forgets to add to the next
handler, and here the failure of forgetting is a session that was supposed to do
one thing reading a project's reports.
*/
func OnlyEnrolment(next http.Handler, auth Principals) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pr, ok := auth.Principal(r)
		// Note that removing this condition does not compile: `pr` is used
		// nowhere else, so the restriction cannot be dropped by deleting a
		// clause. A mutation test found that, which is a small thing and a
		// pleasant one — the compiler is holding part of the guarantee.
		if !ok || !pr.Enrol {
			// No session, or an ordinary one. Anything unauthenticated is
			// somebody else's problem — the handlers below decide, as they
			// always have.
			next.ServeHTTP(w, r)
			return
		}

		if !allowedDuringEnrolment(r.URL.Path) {
			/*
			   403 with a reason, which is unusual for this API and right here.

			   Everywhere else a refusal that explains itself tells somebody
			   something they should not learn. This is their own account,
			   they know why they are here, and the portal needs to tell them
			   what to do — "not authorised" on every panel would read as the
			   product being broken during the one flow where they have to
			   trust it.
			*/
			fail(w, http.StatusForbidden,
				"This project requires a second factor. Finish setting one up to continue.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// allowedDuringEnrolment reports whether a path is on the list.
//
// Prefix-matched on a path segment, so /v1/auth/factor covers /start, /confirm
// and /codes — and does not cover a hypothetical /v1/auth/factories, which a
// bare strings.HasPrefix would have let through.
func allowedDuringEnrolment(path string) bool {
	for _, allowed := range duringEnrolment {
		if path == allowed || strings.HasPrefix(path, allowed+"/") {
			return true
		}
	}
	return false
}
