package vault

import (
	"sort"

	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/platform/secret"
)

/*
uses is every secret the project's definitions name, and who names it.

A datasource by its DSN, bucket URI and credentials; a report by each map's
basemap, including the key a provider uses when a report names none — a Mapbox
map with no key reads mapbox-token, and that is exactly the secret somebody
opening Settings needs to be told is missing.
*/
func (s *Service) uses() map[string][]Use {
	out := map[string][]Use{}
	if s.defs == nil {
		return out
	}
	add := func(kind, name string, refs ...string) {
		for _, ref := range refs {
			for _, n := range secret.Names(ref) {
				out[n] = appendOnce(out[n], Use{Kind: kind, Name: name})
			}
		}
	}
	for _, ds := range s.defs.DataSources() {
		add("DataSource", ds.Name, ds.DSN, ds.URI, ds.Credentials)
	}
	for _, r := range s.defs.Reports() {
		add("Report", r.Name, basemapRefs(r)...)
	}
	for name := range out {
		sort.Slice(out[name], func(i, j int) bool {
			a, b := out[name][i], out[name][j]
			return a.Kind < b.Kind || (a.Kind == b.Kind && a.Name < b.Name)
		})
	}
	return out
}

// basemapRefs are the references a report's maps resolve for their tiles.
func basemapRefs(r definition.Report) []string {
	var refs []string
	for _, o := range r.Outputs {
		for _, b := range o.Layout {
			if b.Map == nil || b.Map.Basemap == nil {
				continue
			}
			bm := b.Map.Basemap
			if bm.Provider == "" {
				refs = append(refs, bm.URL)
				continue
			}
			refs = append(refs, bm.KeyRef())
		}
	}
	return refs
}

// usesOf is the datasources that read a secret, for rebuilding their
// connections when it changes.
func (s *Service) usesOf(name string) []definition.DataSource {
	if s.defs == nil {
		return nil
	}
	var out []definition.DataSource
	for _, ds := range s.defs.DataSources() {
		for _, n := range secret.Names(ds.DSN + " " + ds.URI + " " + ds.Credentials) {
			if n == name {
				out = append(out, ds)
				break
			}
		}
	}
	return out
}

func appendOnce(us []Use, u Use) []Use {
	for _, have := range us {
		if have == u {
			return us
		}
	}
	return append(us, u)
}
