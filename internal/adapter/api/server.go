package api

import (
	"log/slog"
	"net/http"

	"github.com/gsoultan/cronos/internal/app/publish"
	"github.com/gsoultan/cronos/internal/extension"
	"github.com/gsoultan/cronos/internal/platform/token"
)

/*
Deps is everything the HTTP surface can be given.

A struct rather than an argument list. It reached sixteen positional
parameters, ten of which were nil at the only caller that did not want them,
and the failure mode of that shape is silent: two adjacent interfaces of the
same kind swapped by a hand that miscounted commas, compiling cleanly and
serving the wrong thing.

Almost every field is optional, and absent means the routes it would have
served are not mounted at all rather than mounted and refusing. A read-only
server is a legitimate deployment, and an endpoint that exists only to say no
is one somebody will spend an afternoon probing.
*/
type Deps struct {
	/*
	   Projects resolves the runtime a request acts in — its definitions, its
	   connections, and the things that read them.

	   A port rather than five fields, because they must never be mixed: a
	   report resolved from one project and run against another's warehouse is
	   one customer's numbers on another customer's screen. api.One is the
	   single-project deployment every one of these was before, and it checks
	   the caller belongs to it rather than assuming the narrowness of the
	   deployment does that.
	*/
	Projects Projects
	Signer   *token.Signer

	Origins []string
	Log     *slog.Logger

	// Publish and Store are the management API. Publish resolves the caller's
	// own project: each has its own datasets to check a report against.
	Publish Publishing
	Store   publish.Store
	Admin   *AdminKey

	Runs   History
	Users  Users
	Shares Sharing
	// Channels are the ways this deployment can deliver something, so an
	// interface offers what exists rather than what the format supports.
	Channels []string
	// Sends delivers one report now, to people somebody names. Absent, the
	// share panel's Send tab has nothing behind it — which is what it had
	// since it was drawn.
	Sends Sending
	// Directory records people an identity provider vouched for, so they can
	// be seen on the People page and disabled there like anybody else.
	Directory Directory
	// Roster is who has access. Absent, the People endpoints are not mounted
	// and a deployment manages accounts with the CLI, which is where they were
	// managed before this existed.
	Roster Roster

	// Org and Project are the single tenant this process serves. The store is
	// multi-tenant; the process is not, and the read fallback is gated on
	// these because a directory is not multi-tenant either.
	Org     string
	Project string

	// Ready are the dependencies a readiness probe asks. Empty means the probe
	// answers ok, which is honest for a deployment with nothing to ask.
	Ready []Check
	// Metrics counts what is served. Absent, nothing is counted and /v1/metrics
	// is not mounted — a deployment that scrapes nothing should not be serving
	// an endpoint that lists its routes and their volumes.
	Metrics *Metrics

	// BehindProxy says something in front terminates the connection, so the
	// caller's address arrives in X-Forwarded-For. Off by default: reading it
	// with no proxy in front keys every rate limit by a value the caller
	// chooses, which is not a limit.
	BehindProxy bool
}

/*
The rates, which are a judgement rather than a calculation.

Each is generous enough that an office behind one address does not notice, and
small enough that a script does. They are per instance and per address, which
is the honest scope: a deployment behind several needs the limit at its edge
too, and what this one buys is that a single instance cannot be walked through
on its own.
*/
var (
	// Sign-in: five a minute, ten at once. A person mistyping a password three
	// times in a row is ordinary; three hundred attempts is not a person.
	signInRate, signInBurst = 5.0 / 60, 10.0
	// Opening a share: the id is the credential and there is no other, so this
	// is what stands between the id space and somebody enumerating it. Thirty
	// a minute is more than any reader needs and far less than a script wants.
	shareRate, shareBurst = 30.0 / 60, 20.0
	// Rendering: each one executes SQL against somebody else's warehouse,
	// where the cost of a request is not ours to spend. Per token rather than
	// per address — see Limited — so this is what one reader may do, not what
	// everyone behind one NAT may do between them. Twenty a second sustained
	// is far above anybody clicking and far below a loop.
	renderRate, renderBurst = 20.0, 120.0
)

// Routes builds the HTTP surface.
//
// Every route is behind a token, with one exception that earns it: opening a
// share, where the id is the credential and the recipient has no account here.
// There is no other unauthenticated read of a definition, not even its name —
// a report's existence is information about our customer's business.
func Routes(d Deps) http.Handler {
	embed := NewEmbed(d.Projects, d.Signer, d.Log)
	if rv, ok := d.Shares.(Revocations); ok {
		embed = embed.WithRevocations(rv)
	}
	author := NewAuthor(d.Signer, d.Admin)
	// A portal token is checked against the account it names, so disabling
	// somebody takes effect on their next request rather than in eight hours.
	if standing, ok := d.Roster.(Standing); ok {
		author = author.WithStanding(standing)
	}

	// One limiter per concern, shared by the routes that serve it: an embed
	// render and a portal render cost the same warehouse the same, so they
	// draw on one allowance rather than two that add up to twice the limit.
	renders := NewLimit(renderRate, renderBurst)
	limited := func(h http.Handler, l *Limit, message string) http.Handler {
		return NewLimited(h, l, message).BehindProxy(d.BehindProxy)
	}
	// Renders are keyed by the token, so one reader's loop does not throttle
	// the colleague sitting next to them.
	perReader := func(h http.Handler) http.Handler {
		return NewLimited(h, renders, "Too many requests. Try again shortly.").
			BehindProxy(d.BehindProxy).By(ByBearer)
	}

	mux := http.NewServeMux()
	mux.Handle("/v1/embed/reports/{name}", perReader(embed))
	// The portal's own read. A separate path from the embed one because the
	// two have different callers and different audiences, and the audience
	// check should be the first thing a handler does rather than a branch
	// inside it.
	mux.Handle("/v1/reports/{name}", perReader(NewPortalReports(embed, author, d.Log)))

	// Sending renders a document and hands it to a delivery channel, so it is
	// limited like a render rather than not at all: the cost is a typesetter
	// and somebody else's mail relay.
	if d.Sends != nil {
		mux.Handle("/v1/reports/{name}/send", perReader(NewSend(d.Sends, author, d.Log)))
	}

	// What the project contains, in one request. A browsing interface asking
	// for the names and then once per name is a page that loads in a hundred
	// round trips.
	mux.Handle("/v1/catalog", NewCatalog(d.Projects, author, d.Log).WithChannels(d.Channels))

	// Sharing needs somewhere to record what was handed out, so that it can be
	// withdrawn. A deployment without one has no way to take a link back, and
	// a link that cannot be taken back is not a link anybody should be offered.
	if d.Shares != nil {
		h := NewShares(d.Shares, author, d.Log)
		mux.Handle("/v1/shares", h)
		mux.Handle("/v1/shares/{id}", h)
		// Opening is the only route here with no credential but the id itself,
		// so it is the only one whose rate decides whether the id space can be
		// walked. The other two are behind a token already.
		mux.Handle("/v1/shares/{id}/open",
			limited(h, NewLimit(shareRate, shareBurst),
				"Too many attempts. Try again shortly."))
	}

	// Liveness and readiness are different questions, and answering only the
	// first is how a load balancer keeps sending work to a process whose
	// warehouse has gone away.
	//
	// Liveness: this process is running, do not restart it. Unconditional on
	// purpose — a liveness probe that fails because a database is unreachable
	// restarts a healthy process and does not fix the database.
	mux.HandleFunc("/v1/health", func(w http.ResponseWriter, _ *http.Request) {
		send(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	// Readiness: send it work. This one asks.
	mux.Handle("/v1/ready", NewReady(d.Log, d.Ready...))

	if d.Metrics != nil {
		mux.Handle("/v1/metrics", d.Metrics)
	}

	/*
	   What a sign-in page needs to know before anybody has signed in.

	   Unauthenticated, and it has to be: the page asking is the one nobody has
	   a session for yet. It says which methods exist and nothing else — not
	   the issuer, not the client id, not whether a given address has an
	   account — because a page that renders a button is all this is for.
	*/
	mux.HandleFunc("/v1/auth/methods", func(w http.ResponseWriter, _ *http.Request) {
		methods := map[string]any{"password": d.Users != nil}
		if flow := extension.SignIn(); flow != nil {
			methods["sso"] = map[string]string{"provider": flow.Name()}
		}
		send(w, http.StatusOK, methods)
	})

	// Sign-in exists only where there is somewhere to check credentials.
	// Mounted against nothing it would refuse every attempt identically, which
	// is indistinguishable from a wrong password and impossible to debug.
	if d.Users != nil {
		mux.Handle("/v1/auth/login",
			limited(NewAuth(d.Users, d.Signer, d.Log), NewLimit(signInRate, signInBurst),
				"Too many sign-in attempts. Try again in a minute."))
	}

	/*
	   Sign-in through somebody else's directory, where a provider registered
	   one. Mounted from the seam rather than from configuration: the whole
	   point of internal/extension is that a build decides what it can do, and
	   a route that exists to say "no provider" is one somebody spends an
	   afternoon probing.
	*/
	if flow := extension.SignIn(); flow != nil {
		sso := NewSSO(flow, d.Signer, d.Directory, d.Log).In(d.Org, d.Project, "viewer")
		// Limited like sign-in: it reaches an identity provider, which is
		// somebody else's service and somebody else's rate limit.
		mux.Handle("/v1/auth/sso/start",
			limited(sso, NewLimit(signInRate, signInBurst), "Too many attempts. Try again in a minute."))
		mux.Handle("/v1/auth/sso/callback", sso)
	}

	// Who has access, and the ways it changes. Only where there is somewhere
	// to keep people: a file-backed deployment has no accounts to manage.
	if d.Roster != nil {
		people := NewPeople(d.Roster, author, d.Log)
		mux.Handle("/v1/people", people)
		mux.Handle("/v1/people/{id}", people)

		// Limited like sign-in rather than like a render: it takes the current
		// password, so an unbounded rate is an unbounded number of guesses at
		// it from inside a borrowed session.
		mux.Handle("/v1/auth/password",
			limited(NewPassword(d.Roster, author, d.Log), NewLimit(signInRate, signInBurst),
				"Too many attempts. Try again in a minute."))
	}

	// Management is open to an author with a portal token or to a pipeline
	// with the shared key. Mounted when either can exist.
	if d.Publish != nil && (author.Enabled() || (d.Admin != nil && d.Admin.Enabled())) {
		// A read falls back to what the process booted with, for the
		// definitions no store has a copy of — a directory-bootstrapped
		// deployment answering for a report that plainly renders. Resolved per
		// request now, so the fallback is this caller's project and not
		// whichever one the process was configured with.
		handler := NewDefinitions(d.Publish, d.Store, author, d.Log).WithProjects(d.Projects)
		mux.Handle("/v1/definitions", handler)
		mux.Handle("/v1/definitions/{kind}/{name}", handler)

		// Both resolve their project per request. A deployment with no named
		// sources, or no scheduler armed, answers from the project rather than
		// from a nil check here — see Project.
		mux.Handle("/v1/datasources/{name}/test", NewDataSources(d.Projects, author, d.Log))
		mux.Handle("/v1/schedules/{name}/run", NewSchedules(d.Projects, author, d.Log))

		// Behind the admin key and never the embed token: a run record names
		// every recipient of a burst.
		if d.Runs != nil {
			h := NewRuns(d.Runs, author, d.Log)
			mux.Handle("/v1/runs", h)
			mux.Handle("/v1/runs/{id}", h)
		}
	}

	// Outermost, so a panic in the CORS layer is caught too and every request
	// gets an id — including the ones refused before they reach a handler.
	observed := NewObserved(NewCORS(d.Origins, mux), d.Log)
	if d.Metrics != nil {
		observed = observed.WithMetrics(d.Metrics)
	}
	return observed
}
