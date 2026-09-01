package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gsoultan/cronos/internal/adapter/api"
	"github.com/gsoultan/cronos/internal/platform/token"
)

/*
The session that may only set up a second factor.

A project requires one and this account has none. Refusing the sign-in would
lock a team out of its own reporting on the afternoon somebody switches the
requirement on — and would put an administrator on the phone being asked to turn
a second factor off, which is the exact call the second factor exists to make
suspicious. So they sign in, and get nowhere.

"Nowhere" is the thing to test. The gate is an allow-list around the whole mux,
and the two ways it can be wrong are opposite: too narrow and somebody cannot
finish enrolling, too wide and a session that was supposed to do one thing is
reading a customer's reports.
*/

func enrolOnly(t *testing.T, signer *token.Signer) string {
	t.Helper()

	issued, err := signer.Mint(token.Claims{
		Audience: token.Portal, Role: "admin",
		Org: "acme", Project: "finance", Subject: "usr_ada", Enrol: true,
	}, api.SessionLifetime)
	if err != nil {
		t.Fatal(err)
	}
	return issued
}

// behind wraps a handler that answers 200 to everything, so what is under test
// is the gate rather than whatever would have handled the request.
func behind(auth api.Principals) http.Handler {
	return api.OnlyEnrolment(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), auth)
}

func reach(h http.Handler, path, session string) int {
	r := httptest.NewRequest(http.MethodGet, path, nil)
	if session != "" {
		r.Header.Set("Authorization", "Bearer "+session)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w.Code
}

func TestAnEnrolmentSessionReachesNothingElse(t *testing.T) {
	signer, err := token.NewSigner([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	h := behind(api.NewAuthor(signer, nil))
	only := enrolOnly(t, signer)

	// Everything a person might reach for while stuck on the enrolment screen,
	// and the routes that carry a project's data.
	for _, path := range []string{
		"/v1/catalog",
		"/v1/reports/monthly",
		"/v1/reports/monthly/send",
		"/v1/definitions",
		"/v1/definitions/Report/monthly",
		"/v1/runs",
		"/v1/people",
		"/v1/people/invitations",
		"/v1/shares",
		"/v1/datasources/warehouse/test",
		"/v1/schedules/monthly/run",
		"/v1/platform/tenants",
		"/v1/policy",
		"/v1/auth/password",
	} {
		if got := reach(h, path, only); got != http.StatusForbidden {
			t.Errorf("%s answered %d to a session that may only enrol", path, got)
		}
	}
}

// And it reaches what enrolment needs, or somebody is stuck with no way
// forward and no way out.
func TestAnEnrolmentSessionReachesWhatItNeeds(t *testing.T) {
	signer, _ := token.NewSigner([]byte("0123456789abcdef0123456789abcdef"))
	h := behind(api.NewAuthor(signer, nil))
	only := enrolOnly(t, signer)

	for _, path := range []string{
		"/v1/auth/factor",
		"/v1/auth/factor/start",
		"/v1/auth/factor/confirm",
		"/v1/auth/factor/codes",
		// Who they are, so the page can address them by name rather than
		// asking a stranger to scan a QR code.
		"/v1/auth/profile",
		// The way out of a half-finished enrolment that is not clearing
		// browser storage by hand.
		"/v1/auth/sessions/end",
		"/v1/auth/sso/logout",
		"/v1/health",
	} {
		if got := reach(h, path, only); got != http.StatusOK {
			t.Errorf("%s answered %d, so enrolment cannot be finished", path, got)
		}
	}
}

/*
The allow-list matches on a path segment, not a prefix.

`strings.HasPrefix(path, "/v1/auth/factor")` also matches `/v1/auth/factories`
and `/v1/auth/factor-history`, neither of which exists today. The point is that
adding one tomorrow would silently open it to a session that may only enrol, and
nobody would be looking.
*/
func TestTheAllowListDoesNotMatchNeighbouringPaths(t *testing.T) {
	signer, _ := token.NewSigner([]byte("0123456789abcdef0123456789abcdef"))
	h := behind(api.NewAuthor(signer, nil))
	only := enrolOnly(t, signer)

	for _, path := range []string{
		"/v1/auth/factories",
		"/v1/auth/factor-history",
		"/v1/auth/profiles",
		"/v1/healthz",
		"/v1/setups",
	} {
		if got := reach(h, path, only); got != http.StatusForbidden {
			t.Errorf("%s was allowed by a prefix match: %d", path, got)
		}
	}
}

// An ordinary session is untouched. The gate exists for one bit on one kind of
// token and must be invisible to everybody else.
func TestAnOrdinarySessionIsNotGated(t *testing.T) {
	signer, _ := token.NewSigner([]byte("0123456789abcdef0123456789abcdef"))
	h := behind(api.NewAuthor(signer, nil))

	ordinary, _ := signer.Mint(token.Claims{
		Audience: token.Portal, Role: "admin",
		Org: "acme", Project: "finance", Subject: "usr_ada",
	}, api.SessionLifetime)

	for _, path := range []string{"/v1/catalog", "/v1/reports/monthly", "/v1/people"} {
		if got := reach(h, path, ordinary); got != http.StatusOK {
			t.Errorf("%s answered %d to an ordinary session", path, got)
		}
	}
}

// And so is a request with no session at all: whether it is refused is the
// handler's business, and answering it here would change what every
// unauthenticated route says.
func TestNoSessionPassesStraightThrough(t *testing.T) {
	signer, _ := token.NewSigner([]byte("0123456789abcdef0123456789abcdef"))
	h := behind(api.NewAuthor(signer, nil))

	if got := reach(h, "/v1/catalog", ""); got != http.StatusOK {
		t.Fatalf("an unauthenticated request was answered %d by the gate", got)
	}
}

/*
An embed token cannot carry the restriction, for the same reason it cannot carry
the others.

Harmless in this direction — the claim only ever takes things away — and set the
same way as its neighbours so that nobody reading the three has to work out why
one is different.
*/
func TestAnEmbedTokenCannotBeAnEnrolmentSession(t *testing.T) {
	signer, _ := token.NewSigner([]byte("0123456789abcdef0123456789abcdef"))

	issued, err := signer.Mint(token.Claims{
		Audience: token.Embed, Org: "acme", Project: "finance",
		Subject: "customer-42", Enrol: true,
	}, api.SessionLifetime)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := signer.Verify(issued, token.Embed)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Principal().Enrol {
		t.Fatal("an embed token claims to be an enrolment session")
	}
}
