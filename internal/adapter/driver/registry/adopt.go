package registry

import (
	"fmt"
	"strings"

	"github.com/gsoultan/cronos/internal/core/definition"
)

/*
Taking a datasource on, and letting one go, while the process is running.

New opens what a deployment starts with, which was the whole story while
datasources only ever came from a directory. They come from the API now: the
portal has a wizard for connecting one, and publishing it stored the definition
and left the running process with no connection to it. The catalogue listed the
source, the Test button said "no such datasource", and every report reading it
answered 500 until somebody restarted the server — so a deployment could have
exactly the datasources it booted with, and adding a second one was a deploy.

Adopting is the same work New does per definition, at a different time. What is
not the same is that something may already hold that name: an edit re-publishes
a source under the name it already had, and the connection it replaces has to be
closed rather than leaked, and every federation that mounted it has to be
dropped rather than left pointing at the old address.
*/

/*
Adopt opens a datasource and makes it the one this registry answers with.

Replaces a source of the same name, which is what re-publishing an edited
definition means. The old pool is closed after the swap rather than before it,
and closing one does not interrupt a query already running on it: the
connection that query holds is closed when it gives it back, so a render
mid-stream finishes against the address it started on.

An error means the source is not registered and nothing changed: this build has
no driver for it, or a secret it names does not resolve. The caller decides
whether that is fatal, exactly as it does for Unavailable at startup — a report
that does not read this source must keep working.
*/
func (r *Registry) Adopt(def definition.DataSource) error {
	if def.Name == "" {
		// A nameless source would be registered under "" and matched by no
		// dataset, which is a connection nothing can use and nothing can close.
		return fmt.Errorf("registry: a datasource needs a name")
	}

	// Outside the lock. Resolving a secret can read a file and opening a pool
	// can parse a DSN, and neither should queue every query in the process.
	s, err := r.prepare(def)
	if err != nil {
		/*
		   Recorded before it is returned, so the deployment can be asked
		   about it afterwards.

		   A source that will not open is still a source this project has
		   defined, and the report reading it fails. Without this the only
		   trace was the line the caller logged at the moment it happened:
		   /v1/ready answered healthy, the test endpoint said "no such
		   datasource" — which is not what is wrong — and the definition sat
		   in the catalogue looking like every other one.
		*/
		r.mu.Lock()
		r.unopened[def.Name] = err
		r.mu.Unlock()
		return err
	}

	r.mu.Lock()
	if r.closed {
		/*
		   Readable here although closed is written under fedMu: Close holds
		   this lock for its whole body, so the write happens with both held
		   and a read under this one cannot see it half made.

		   Refused rather than registered, because a registry that has been
		   closed has nobody left to close what it is handed — adopting into
		   one leaks a pool to somebody else's warehouse for the life of the
		   process.
		*/
		r.mu.Unlock()
		closeSource(s)
		return fmt.Errorf("%w: adopting %q", ErrClosed, def.Name)
	}
	old := r.sources[def.Name]
	r.sources[def.Name] = s
	// Whatever it failed with last time is no longer true.
	delete(r.unopened, def.Name)
	r.mu.Unlock()

	// Both after the swap, so the window where this source is unanswerable is
	// the one statement above rather than however long a warehouse takes to
	// hang up.
	r.evict(def.Name)
	closeSource(old)

	if s.db != nil {
		r.log.Info("datasource adopted", "name", def.Name, "driver", def.Driver,
			"timeout", def.Limits.Timeout(), "maxRows", def.Limits.Rows(),
			"replaced", old != nil)
	}
	return nil
}

/*
Drop closes a datasource and forgets it.

Deleting a definition is the only caller. Without it the pool stays open to a
database nobody has asked about since, and — worse than the connection — a
dataset naming the deleted source keeps returning rows, so the delete appears
to have done nothing until the next restart, when the report finally breaks.

Silent about a name it does not hold: a deployment that never opened the source
has nothing to close, and the definition is being deleted either way.
*/
func (r *Registry) Drop(name string) {
	r.mu.Lock()
	old := r.sources[name]
	delete(r.sources, name)
	// Including one that never opened: the definition is going either way,
	// and a reason to be unready about a source nobody has any more is a
	// deployment that cannot be made ready again.
	unopened := r.unopened[name]
	delete(r.unopened, name)
	r.mu.Unlock()

	if old == nil && unopened != nil {
		r.log.Info("datasource dropped", "name", name, "was", "never opened")
		return
	}

	if old == nil {
		return
	}
	r.evict(name)
	closeSource(old)
	r.log.Info("datasource dropped", "name", name)
}

/*
Empty reports whether anything is registered.

For the one caller that has to tell "no datasources are defined here" from "the
one this dataset names is not among them": a deployment with no sources at all
reads the configured database, and a deployment with three does not get a
fourth by naming it.
*/
func (r *Registry) Empty() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.sources) == 0
}

/*
evict closes every federation that mounted a source, by name.

A federation is keyed by which sources it holds and under what names — not by
what those sources point at — so a source re-published at a new address would
otherwise keep being read at the old one through a DuckDB connection that
attached it minutes ago. Which is the failure mode a rotated password produces:
the single-source path picks up the new credential and the federated path keeps
using the old one, on the same definition.
*/
func (r *Registry) evict(name string) {
	r.fedMu.Lock()
	var stale []*federation
	for key, f := range r.federations {
		if !mounts(key, name) {
			continue
		}
		stale = append(stale, f)
		delete(r.federations, key)
	}
	r.fedMu.Unlock()

	for _, f := range stale {
		// once, so a federation another goroutine is still opening is waited
		// for rather than left behind holding an attachment.
		f.once.Do(func() {})
		if f.fed == nil {
			continue
		}
		mounted.Add(-1)
		if err := f.fed.Close(); err != nil {
			r.log.Warn("could not close a federation over a changed datasource",
				"source", name, "err", err)
		}
	}
}

// mounts reports whether a federation key names this source. The key is
// `mount=source` pairs joined by commas — see fingerprint.
func mounts(key, source string) bool {
	for _, part := range strings.Split(key, ",") {
		if _, named, ok := strings.Cut(part, "="); ok && named == source {
			return true
		}
	}
	return false
}

// closeSource releases a source's pool, if it had one. An object store has
// none: it is addressed rather than connected to.
func closeSource(s *source) {
	if s == nil || s.db == nil {
		return
	}
	s.db.Close()
}
