package boot

import (
	"context"
	"fmt"
	"log/slog"

	alertemail "github.com/gsoultan/cronos/internal/adapter/alert/email"
	"github.com/gsoultan/cronos/internal/adapter/api"
	"github.com/gsoultan/cronos/internal/adapter/audit"
	codec "github.com/gsoultan/cronos/internal/adapter/codec/yaml"
	emailchannel "github.com/gsoultan/cronos/internal/adapter/deliver/email"
	filechannel "github.com/gsoultan/cronos/internal/adapter/deliver/file"
	s3channel "github.com/gsoultan/cronos/internal/adapter/deliver/s3"
	"github.com/gsoultan/cronos/internal/adapter/driver/registry"
	"github.com/gsoultan/cronos/internal/adapter/render/paginated"
	"github.com/gsoultan/cronos/internal/adapter/render/spreadsheet"
	"github.com/gsoultan/cronos/internal/adapter/store/file"
	sqlstore "github.com/gsoultan/cronos/internal/adapter/store/sql"
	"github.com/gsoultan/cronos/internal/app/burst"
	"github.com/gsoultan/cronos/internal/app/publish"
	"github.com/gsoultan/cronos/internal/app/run"
	"github.com/gsoultan/cronos/internal/app/schedule"
	"github.com/gsoultan/cronos/internal/app/send"
	"github.com/gsoultan/cronos/internal/app/share"
	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/principal"
	"github.com/gsoultan/cronos/internal/extension"
	"github.com/gsoultan/cronos/internal/platform/config"
	"github.com/gsoultan/cronos/internal/platform/secret"
	"github.com/gsoultan/cronos/internal/platform/token"
)

// channels registers every delivery channel the configuration supports.
//
// A channel with no configuration is not registered rather than registered and
// always failing. A schedule delivering via a channel nobody set up should be
// told "no channel named email" when it runs — and an operator reading the
// startup line should be able to see which ones exist.
func channels(cfg config.Server, log *slog.Logger) ([]burst.Channel, error) {
	var out []burst.Channel

	if cfg.Deliveries != "" {
		out = append(out, filechannel.New(cfg.Deliveries))
	}
	if cfg.SMTP.Configured() {
		c, err := emailchannel.New(emailchannel.Config{
			Host: cfg.SMTP.Host, From: cfg.SMTP.From,
			Username: cfg.SMTP.Username, Password: cfg.SMTP.Password,
		})
		if err != nil {
			// Refused at startup, where somebody is watching, rather than once
			// per recipient in the middle of a burst.
			return nil, err
		}
		out = append(out, c)
	}
	if cfg.S3.Configured() {
		c, err := s3channel.New(s3channel.Config{
			Endpoint: cfg.S3.Endpoint, Region: cfg.S3.Region,
			AccessKey: cfg.S3.AccessKey, SecretKey: cfg.S3.SecretKey,
		})
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}

	names := make([]string, 0, len(out))
	for _, c := range out {
		names = append(names, c.Name())
	}
	log.Info("delivery channels", "registered", names)
	return out, nil
}

// scheduler wires the burst pipeline behind a cron loop.
func scheduler(cfg config.Server, org, project string, serving func() (string, string),
	repo *file.Repository, runner *run.Service, records *sqlstore.Store,
	log *slog.Logger) (*schedule.Service, error) {

	chans, err := channels(cfg, log)
	if err != nil {
		return nil, err
	}

	bursts := burst.New(repo, recipients{runner}, documents(runner), log, chans...).
		// The repository, because it holds what is running rather than what was
		// last published — and the run record must name the bytes that produced
		// the document, not the ones somebody stored a moment later.
		WithVersions(repo)
	if records != nil {
		bursts = bursts.WithHistory(records)
	}

	sched := schedule.New(repo, bursts, owner{org: org, project: project, serving: serving}, log)
	if cfg.SchedulerTick > 0 {
		sched = sched.WithTick(cfg.SchedulerTick)
	}
	if mail := mailer(chans); mail != nil {
		sched = sched.WithAlerts(alertemail.New(mail))
	} else {
		// Said out loud. A schedule naming onFailure.alert with no mail relay
		// configured reaches a log and no human, which is the state this whole
		// feature exists to replace.
		log.Warn("no mail relay — schedule failures will not alert anybody")
	}

	/*
	   Every schedule is parsed before the listener opens, and one that will not
	   parse is named rather than fatal.

	   This used to return the error and stop the process. The reasoning held
	   while definitions came from a directory an operator controlled: a
	   timezone the host does not have was a configuration mistake, and failing
	   loudly beat serving with two of five schedules quietly missing.

	   Definitions come from the store now. An editor publishing "Europe/Berln"
	   got a 200, the running instance carried on, and the next restart would
	   not come back — with the API down, so the only way to remove the typo was
	   a psql prompt. One person's misspelling was an outage for every project in
	   the deployment, days later, looking like the deploy had broken it.

	   Publishing validates the timezone now. This is the net under the ones
	   already stored, and it keeps the loudness that made refusing attractive:
	   each is logged at error and counted for the metric, so a schedule that is
	   not running is visible rather than absent.
	*/
	for _, u := range schedule.Check(repo) {
		log.Error("schedule will not arm — it is stored but will never run",
			"schedule", u.Name, "err", u.Err)
		unarmed.Add(1)
	}
	return sched, nil
}

// mailer finds the email channel among those registered, if it is there.
//
// An alert is a delivery with no attachment, so it goes out through the same
// relay a statement does rather than a second SMTP client to configure and
// keep patched.
func mailer(chans []burst.Channel) alertemail.Sender {
	for _, c := range chans {
		if c.Name() == "email" {
			return c
		}
	}
	return nil
}

// owner resolves who a schedule runs as.
//
// The project it lives in, with no row scope. A burst runs as a project member
// and not as an end customer — see docs/tenancy.md, and note that publishing
// refuses a schedule whose datasets carry a scope predicate for exactly this
// reason.
// The project is carried rather than read from configuration: a process may
// serve several, and a schedule that ran as the wrong one would read the wrong
// warehouse and mail the result to the wrong customers.
/*
owner is who a scheduled run acts as.

The tenancy is asked for rather than remembered, because it can change once: a
deployment that starts as default/default and is named through /setup is a
different project afterwards, and a scheduler still holding the old name files
every run where the people who own it will never see one.

serving is nil for a deployment that named itself in configuration, which is
every deployment that cannot be adopted — there the two are the same and asking
would only be a slower way to get the same answer.
*/
type owner struct {
	org, project string
	serving      func() (string, string)
}

func (o owner) Owner(s definition.Schedule) principal.Principal {
	org, project := o.org, o.project
	if o.serving != nil {
		org, project = o.serving()
	}
	return principal.Principal{
		Subject:     "schedule:" + s.Name,
		OrgID:       org,
		ProjectID:   project,
		ProjectRole: principal.ProjectEditor,
		// A schedule runs as a project member. docs/tenancy.md is explicit
		// that burst targets rely on membership alone.
		Member: true,
	}
}

// recipients reads the rows a burst fans out over, through the same query
// pipeline everything else uses.
type recipients struct{ runner *run.Service }

func (r recipients) Rows(ctx context.Context, dataset string, params map[string]any,
	pr principal.Principal) ([]burst.Row, error) {

	rows, err := r.runner.Rows(ctx, dataset, params, pr)
	if err != nil {
		return nil, fmt.Errorf("reading recipients from %q: %w", dataset, err)
	}
	out := make([]burst.Row, 0, len(rows))
	for _, row := range rows {
		out = append(out, burst.Row(row))
	}
	return out, nil
}

// history adapts a possibly-absent store to the API's port.
//
// A typed nil would satisfy the interface and panic on first use, which is the
// oldest trap in Go's book: the check has to be on the concrete value.
func history(records *sqlstore.Store) api.History {
	if records == nil {
		return nil
	}
	return records
}

// users adapts a possibly-absent store to the sign-in port.
//
// Sign-in needs somewhere to keep people, which is the definition store when
// it is a database. A file-backed deployment has nowhere to put a password
// hash, so the endpoint is not mounted at all rather than mounted and refusing
// every attempt — a login that always fails is indistinguishable from a wrong
// password and impossible to debug.
func users(records *sqlstore.Store) api.Users {
	if records == nil {
		return nil
	}
	return records
}

// publishing wires the publish service to the running repository when storing
// alone would not change what runs.
//
// A file-backed store rewrites the file and reloads the directory it was read
// from, so it already has this property. A database-backed one has no file to
// rewrite: without the live view, a definition published through the API would
// sit in the store and in the catalogue while every render kept using what the
// process read at startup — until somebody restarted it.
func publishing(store publish.Store, repo *file.Repository, records *sqlstore.Store,
	engines run.Engines, channels []string) *publish.Service {
	svc := publish.New(store, repo).WithReports(repo).
		// So a delete can say what would break rather than breaking it.
		WithCatalog(repo).
		// And so a publish is proved against the database it will read, not
		// only against the dialect this package compiles for.
		WithEngines(engines).
		// And so a schedule naming a channel this deployment has not got is
		// refused by the person who typed it, rather than by the burst at the
		// hour it fires.
		WithChannels(channels)
	if records != nil {
		svc = svc.WithLive(repo)
	}
	return svc
}

// reconcile settles which of the definitions directory and the store is the
// truth, and says so at startup.
//
// The store, once it holds anything. A definition edited in the portal must
// not be reverted by the next deploy, and a definition deleted through the API
// must not come back because its file is still on disk — both of which are
// what consulting the directory every boot would do.
//
// An empty store adopts the directory whole. That is how a project starts:
// somebody's YAML in git, published once, and from then on the store is where
// changes land. Nothing here runs for a file-backed deployment, where the
// directory is the store and the question does not arise.
func reconcile(ctx context.Context, store publish.Store, repo *file.Repository,
	org, project string, log *slog.Logger) error {

	// Acting as the deployment rather than as a person: this is the process
	// adopting its own configuration, and there is no user to attribute it to.
	pr := principal.Principal{
		Subject: "cronosd", OrgID: org, ProjectID: project,
		ProjectRole: principal.ProjectEditor, Member: true,
	}

	stored, err := store.List(ctx, pr)
	if err != nil {
		return fmt.Errorf("reading the definition store: %w", err)
	}

	if len(stored) == 0 {
		seeded, err := adoptDirectory(ctx, store, repo, pr)
		if err != nil {
			return err
		}
		log.Info("definitions", "authority", "store", "seeded", seeded,
			"from", "the definitions directory")
		return nil
	}

	docs := make([][]byte, 0, len(stored))
	for _, e := range stored {
		raw, err := store.Get(ctx, pr, e.Kind, e.Name)
		if err != nil {
			return fmt.Errorf("reading %s %q: %w", e.Kind, e.Name, err)
		}
		docs = append(docs, raw)
	}
	if err := repo.Adopt(docs); err != nil {
		/*
		   One stored definition this build will not accept, and forty-nine it
		   will. Refusing all fifty is not the safer answer: it takes the
		   deployment down, and with the API down the only way to remove the one
		   bad definition is a prompt on the database — the shape of every
		   unrecoverable failure, where fixing the broken thing needs the broken
		   thing.

		   Validation gets stricter over time and the store outlives any one
		   build, so this is not hypothetical: a schedule published against a
		   timezone nobody checked becomes, one release later, a process that
		   will not start. It happens on the upgrade, which is when nobody is
		   looking for a definition somebody published in March.

		   The ordinary path is still all or nothing. This runs only when that
		   one refused, and it is loud — every reason at error, and counted for
		   cronos_definitions_refused, which should be zero.
		*/
		log.Error("the store holds definitions this build will not accept",
			"project", pr.OrgID+"/"+pr.ProjectID, "err", err)
		for _, refused := range repo.AdoptUsable(docs) {
			log.Error("definition refused — it is stored and will not be served",
				"project", pr.OrgID+"/"+pr.ProjectID, "err", refused)
			rejected.Add(1)
		}
	}
	// Stated rather than left to be discovered by somebody who edited a file
	// and could not work out why nothing changed.
	log.Info("definitions", "authority", "store", "loaded", len(docs),
		"directory", "not consulted")
	return nil
}

// adoptDirectory copies the directory into an empty store.
func adoptDirectory(ctx context.Context, store publish.Store, repo *file.Repository,
	pr principal.Principal) (int, error) {

	var n int
	// Sources before datasets before reports before schedules, so a store that
	// fails part way through holds a prefix that resolves rather than a report
	// pointing at a dataset nothing put there.
	for _, kind := range []string{
		codec.KindDataSource, codec.KindDataset, codec.KindReport, codec.KindSchedule,
	} {
		for _, name := range repo.Names(kind) {
			raw, ok := repo.Raw(kind, name)
			if !ok {
				continue
			}
			if _, err := store.Put(ctx, pr, kind, name, raw); err != nil {
				return n, fmt.Errorf("seeding %s %q: %w", kind, name, err)
			}
			n++
		}
	}
	return n, nil
}

// sharing wires the share service, where there is somewhere to record shares.
//
// A file-backed deployment has nowhere to write one, and a share that cannot
// be recorded cannot be withdrawn — so the endpoints are not mounted at all
// rather than mounted and handing out links nobody can take back.
func sharing(records *sqlstore.Store, signer *token.Signer, repo *file.Repository) api.Sharing {
	if records == nil {
		return nil
	}
	return share.New(records, signer, repo)
}

// probing exposes the connection test, where there are named sources to test.
//
// A deployment reading one configured database has nothing to name, so the
// endpoint is not mounted rather than mounted and answering "which one?".
func probing(engines run.Engines) api.Probes {
	if reg, ok := engines.(*registry.Registry); ok {
		return reg
	}
	return nil
}

// secrets is where ${secret:name} is looked up.
//
// Files first, then the environment. A mounted file is not visible in /proc to
// everything running as the same user and does not appear in a crash dump, so
// a deployment that can use one should; a deployment that cannot has the
// environment, which is what every orchestrator already fills in.
func secrets(cfg config.Server) secret.Resolver {
	return secret.Chain{
		secret.Files{Dir: cfg.SecretsDir},
		secret.Env{},
	}
}

// auditing installs the audit sink named by the configuration.
//
// A commercial build registers its own from init(), and that one wins: it was
// chosen deliberately and it durably records what a log pipeline only retains.
// This is the default for everything else, and "off" is a decision somebody
// has to make rather than the state they arrive in.
func auditing(cfg config.Server, log *slog.Logger) string {
	if extension.Audit().Name() != "discard" {
		return extension.Audit().Name() // a commercial sink registered itself
	}
	if cfg.Audit == "off" {
		return "off"
	}
	extension.RegisterAuditSink(audit.NewLog(log))
	return extension.Audit().Name()
}

/*
storeCheck is the one dependency every project shares.

Required: with a database configured, a process that cannot reach it cannot
publish, cannot sign anybody in and cannot record a run — for any project — so
there is nothing useful to send it. Datasources are per project and are not
required, because one warehouse being unreachable leaves every report that does
not read it working, and taking the instance out of rotation would fail those
too. That distinction is the whole reason the answer has three states.
*/
func storeCheck(records *sqlstore.Store) api.Check {
	return api.Check{
		Name:     "store",
		Required: true,
		Probe: func(ctx context.Context) error {
			// The schema too, not only the connection — but only when it is
			// behind, which is the direction that breaks this build.
			//
			// Behind means a table this build reads is not there: the state a
			// restore of an older dump underneath a running process leaves,
			// and there is nothing this instance can serve.
			//
			// Ahead means a newer cronos has migrated, which is every rolling
			// deploy for the length of the rollout. This used to report down
			// for that too, on the reasoning that a schema this build does not
			// know is one it must not write to. Driving it says otherwise:
			// scripts/live-upgrade.sh keeps an old build serving while a new
			// one migrates, and every route still answers — reads, publishes
			// and sign-ins alike — because migrations only ever add tables.
			// That is not luck, it is why they add tables rather than columns.
			//
			// And reporting down was worse than useless. Readiness is what
			// decides whether a load balancer sends work here, so the first
			// new instance to migrate took every old instance out of rotation
			// at once: an eight-replica deployment served the whole of its
			// traffic from the one new pod for the rest of the rollout. The
			// instance that is provably answering correctly is the last one
			// that should be removed.
			at, err := records.SchemaVersion(ctx)
			if err != nil {
				return err
			}
			if at < sqlstore.Wanted() {
				return fmt.Errorf("schema is at %d and this build needs %d", at, sqlstore.Wanted())
			}
			return nil
		},
	}
}

// roster adapts a possibly-absent store to the people port.
//
// Managing accounts needs somewhere to keep them, which is the definition
// store when it is a database. A file-backed deployment has nowhere, so the
// endpoints are not mounted rather than mounted and refusing — and accounts
// there are managed with cronos-user, which is where they were managed before
// any of this existed.
func roster(records *sqlstore.Store) api.Roster {
	if records == nil {
		return nil
	}
	return records
}

// documents is the renderer both the scheduler and a one-off send use.
//
// One construction rather than two, because a report emailed from the share
// panel and the same report attached to a monthly schedule must be the same
// document — and two renderers built from the same parts are two places for
// that to stop being true.
func documents(runner *run.Service) burst.Documents {
	return run.NewStatements(runner, paginated.New(paginated.TypstCLI{})).
		WithWorkbooks(spreadsheet.New())
}

// sending wires "email this report now", where there is somewhere to send it.
//
// Independent of the scheduler: a deployment that renders on request and has a
// mail relay can still send one, and requiring a cron loop to be armed before
// anybody may email a colleague would be an odd thing to require.
func sending(cfg config.Server, repo *file.Repository, runner *run.Service,
	log *slog.Logger) api.Sending {

	chans, err := channels(cfg, log)
	if err != nil || len(chans) == 0 {
		// No channel, no endpoint. The share panel's Send tab then says the
		// deployment has none rather than offering to use one.
		return nil
	}
	return send.New(repo, documents(runner), chans...)
}

// channelNames is what this deployment can deliver through.
//
// So the share panel offers what exists. It offered email and Telegram
// whatever was configured, which meant a deployment with neither showed two
// options that could only fail — and the failure arrived after somebody had
// typed eight addresses into one of them.
func channelNames(cfg config.Server, log *slog.Logger) []string {
	chans, err := channels(cfg, log)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(chans))
	for _, c := range chans {
		names = append(names, c.Name())
	}
	return names
}

// directory records people an identity provider vouched for.
//
// The same store the People page reads, so somebody who signed in through Okta
// appears there and can be turned off there — an SSO account nobody can revoke
// is the hole this whole area exists to close.
func directory(records *sqlstore.Store) api.Directory {
	if records == nil {
		return nil
	}
	return records
}

// invitations is where places held for people are recorded.
//
// Nil for a file-backed deployment, which has no accounts either — so adding
// somebody there is the CLI's job, and there is nothing to invite them to.
func invitations(records *sqlstore.Store) api.Invitations {
	if records == nil {
		return nil
	}
	return records
}

/*
postman is how an invitation is delivered.

The mail channel, or nothing. Nothing is the ordinary case for a deployment that
has never configured a relay, and it means the portal offers a password field
instead of an invite button — rather than an invite button that produces an
error somebody has to interpret.

Built here rather than taken from the channel list because that list is
addressed by name for deliveries, and reaching into it for "the one that happens
to be email" would tie invitations to how bursts are configured.
*/
func postman(cfg config.Server, log *slog.Logger) api.Postman {
	if !cfg.SMTP.Configured() {
		return nil
	}
	c, err := emailchannel.New(emailchannel.Config{
		Host: cfg.SMTP.Host, From: cfg.SMTP.From,
		Username: cfg.SMTP.Username, Password: cfg.SMTP.Password,
	})
	if err != nil {
		// The same configuration channels() already refused startup over, so
		// this cannot be reached by a running server. Logged rather than
		// returned so one bad relay does not take out the whole boot twice.
		log.Error("no invitation mail", "err", err)
		return nil
	}
	return c
}

// factors is where second factors are recorded.
//
// Nil for a file-backed deployment, which has no accounts either — so there is
// nothing to protect and sign-in is the password alone.
func factors(records *sqlstore.Store) api.Factors {
	if records == nil {
		return nil
	}
	return records
}

// platform administers the deployment across tenants.
//
// Nil for a file-backed deployment, which has no accounts anywhere and so
// nothing to administer.
func platform(records *sqlstore.Store) api.Platform {
	if records == nil {
		return nil
	}
	return records
}

// accounts counts them, for the first-run check.
func accounts(records *sqlstore.Store) api.Accounts {
	if records == nil {
		return nil
	}
	return records
}

// policies is what each project requires of the people in it.
//
// Nil for a file-backed deployment, which has no accounts to require anything
// of — and where sign-in does not exist at all.
func policies(records *sqlstore.Store) api.Policies {
	if records == nil {
		return nil
	}
	return records
}
