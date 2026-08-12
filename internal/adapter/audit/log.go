/*
Package audit writes the audit trail to the log.

The seam's default discards, which is correct for a seam and wrong for a
deployment: a product whose whole claim is governed access to somebody else's
customers' data, shipping with no audit unless a commercial build is installed,
has no answer to the only question an auditor asks.

The log is where it goes because the log is already going somewhere. Every
deployment has a pipeline that collects stdout, indexes it and retains it for
as long as the compliance answer needs — building a second one inside cronos
would produce a table nobody backs up and nobody can query alongside everything
else that happened at 06:00.

One line per event, at INFO, under a fixed message so it can be selected by it.
*/
package audit

import (
	"context"
	"log/slog"

	"github.com/gsoultan/cronos/internal/extension"
)

// Log is an AuditSink that writes events to a slog.Logger.
type Log struct {
	log *slog.Logger
}

// NewLog returns a sink writing to log.
func NewLog(log *slog.Logger) *Log { return &Log{log: log} }

// Name identifies the sink in the startup line, so an operator can see which
// one is installed without reading the configuration back.
func (l *Log) Name() string { return "log" }

// Record writes one event.
//
// Never returns an error. A logger that cannot write has already failed in a
// way this cannot fix, and returning an error would make every call site log
// about not being able to log.
func (l *Log) Record(_ context.Context, e extension.Event) error {
	attrs := []any{
		"at", e.At,
		"actor", e.Actor,
		"org", e.OrgID,
		"project", e.ProjectID,
		"action", e.Action,
		"target", e.Target,
		"result", e.Result,
	}
	// Flattened rather than nested under "detail", so a log pipeline can index
	// and filter on the fields — a nested map arrives as one opaque string in
	// most of them, which is the same as not recording it.
	for k, v := range e.Detail {
		attrs = append(attrs, "detail."+k, v)
	}

	l.log.Info("audit", attrs...)
	return nil
}
