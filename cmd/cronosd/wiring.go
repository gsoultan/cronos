package main

import (
	"context"
	"fmt"
	"log/slog"

	alertemail "github.com/gsoultan/cronos/internal/adapter/alert/email"
	"github.com/gsoultan/cronos/internal/adapter/api"
	emailchannel "github.com/gsoultan/cronos/internal/adapter/deliver/email"
	filechannel "github.com/gsoultan/cronos/internal/adapter/deliver/file"
	s3channel "github.com/gsoultan/cronos/internal/adapter/deliver/s3"
	"github.com/gsoultan/cronos/internal/adapter/render/paginated"
	"github.com/gsoultan/cronos/internal/adapter/render/spreadsheet"
	"github.com/gsoultan/cronos/internal/adapter/store/file"
	sqlstore "github.com/gsoultan/cronos/internal/adapter/store/sql"
	"github.com/gsoultan/cronos/internal/app/burst"
	"github.com/gsoultan/cronos/internal/app/run"
	"github.com/gsoultan/cronos/internal/app/schedule"
	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/principal"
	"github.com/gsoultan/cronos/internal/platform/config"
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
func scheduler(cfg config.Server, repo *file.Repository, runner *run.Service,
	records *sqlstore.Store, log *slog.Logger) (*schedule.Service, error) {

	chans, err := channels(cfg, log)
	if err != nil {
		return nil, err
	}

	statements := run.NewStatements(runner, paginated.New(paginated.TypstCLI{})).
		WithWorkbooks(spreadsheet.New())
	bursts := burst.New(repo, recipients{runner}, statements, log, chans...)
	if records != nil {
		bursts = bursts.WithHistory(records)
	}

	sched := schedule.New(repo, bursts, owner{cfg}, log)
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
type owner struct{ cfg config.Server }

func (o owner) Owner(s definition.Schedule) principal.Principal {
	return principal.Principal{
		Subject:     "schedule:" + s.Name,
		OrgID:       o.cfg.Org,
		ProjectID:   o.cfg.Project,
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
