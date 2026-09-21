package boot

import (
	"log/slog"

	codec "github.com/gsoultan/cronos/internal/adapter/codec/yaml"
	"github.com/gsoultan/cronos/internal/adapter/store/file"
)

/*
live applies a published definition to the running process.

Two things have to hear about a definition that arrived through the API, and
until now only one did. The repository holds what the runtime reads — that part
was wired, for a database-backed store. The registry holds the connections, and
it was not: publishing a datasource stored it, listed it in the catalogue, and
left the process with no connection to it, so every report reading it answered
500 until a restart. The portal has a four-step wizard for connecting a source,
which means the ordinary way to get a second datasource was a deploy.

The repository half stays conditional on there being a records store, because a
file-backed one rewrites its own files and reloads the directory itself —
applying the document again would re-insert it with no path, and the next write
would have nowhere to go. The registry half is unconditional: neither store
opens a connection.
*/
type live struct {
	// repo is nil for a file-backed deployment, which reloads itself.
	repo *file.Repository
	// sources is never nil. A datasource is a connection either way.
	sources *sources
	log     *slog.Logger
}

// Apply makes a published definition the one the next request uses.
func (l live) Apply(raw []byte) error {
	if l.repo != nil {
		if err := l.repo.Apply(raw); err != nil {
			return err
		}
	}

	kind, err := codec.Loader{}.Kind(raw)
	if err != nil {
		return err
	}
	if kind != codec.KindDataSource {
		return nil
	}
	def, err := codec.Loader{}.DataSource(raw)
	if err != nil {
		return err
	}

	if err := l.sources.registry.Adopt(def); err != nil {
		/*
		   Named and counted, not refused.

		   The same answer startup gives a source it cannot open, and for the
		   same reason: a build without the duckdb driver can hold a duckdb
		   definition, and refusing the publish would mean a definition that
		   cannot be stored on the deployment that has to store it before an
		   operator can fix or delete it. The report that reads this source
		   fails with a message naming it, which is where somebody can act.
		*/
		l.log.Error("datasource published but not opened — reports that read it will fail",
			"source", def.Name, "err", err)
		unopenable.Add(1)
	}
	return nil
}

/*
Forget removes a deleted definition from the running process.

Deleting went to the store and stopped there, so on a database-backed
deployment the definition stayed live until a restart: the report a person had
just deleted kept rendering, and the datasource kept its pool open to a
warehouse nobody had a definition for any more.
*/
func (l live) Forget(kind, name string) error {
	if kind == codec.KindDataSource {
		l.sources.registry.Drop(name)
	}
	if l.repo != nil {
		l.repo.Forget(kind, name)
	}
	return nil
}
