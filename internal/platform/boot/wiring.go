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
func scheduler(cfg config.Server, org, project string, repo *file.Repository,
	runner *run.Service, records *sqlstore.Store, log *slog.Logger) (*schedule.Service, error) {

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

	sched := schedule.New(repo, bursts, owner{org: org, project: project}, log)
	if mail := mailer(chans); mail != nil {
		sched = sched.WithAlerts(alertemail.New(mail))
	} else {
		// Said out loud. A schedule naming onFailure.alert with no mail relay
		// configured reaches a log and no human, which is the state this whole
		// feature exists to replace.
		log.Warn("no mail relay — schedule failures will not alert anybody")
	}

	// Every schedule is parsed before the listener opens. A timezone the host
	// does not have should stop a deployment, not surprise somebody at six on
	// the first of the month.
	if err := schedule.Check(repo); err != nil {
		return nil, err
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
type owner struct{ org, project string }

func (o owner) Owner(s definition.Schedule) principal.Principal {
	return principal.Principal{
		Subject:     "schedule:" + s.Name,
		OrgID:       o.org,
		ProjectID:   o.project,
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
	engines run.Engines) *publish.Service {
	svc := publish.New(store, repo).WithReports(repo).
		// So a delete can say what would break rather than breaking it.
		WithCatalog(repo).
		// And so a publish is proved against the database it will read, not
		// only against the dialect this package compiles for.
		WithEngines(engines)
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
		return fmt.Errorf("adopting the stored definitions: %w", err)
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
			// The schema too, not only the connection. A store answering at a
			// version this build does not know is one this build must not
			// write to, and it is the state a half-finished deploy leaves
			// behind.
			at, err := records.SchemaVersion(ctx)
			if err != nil {
				return err
			}
			if at != sqlstore.Wanted() {
				return fmt.Errorf("schema is at %d, this build wants %d", at, sqlstore.Wanted())
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
