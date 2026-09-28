package basemap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gsoultan/cronos/internal/app/run"
	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/platform/secret"
)

// Resolver turns every basemap a report can name into tiles. It implements
// run.Basemaps. The Google sessions and the memory of what was logged are the
// deployment's: a session is good for every project using the same key.
type Resolver struct {
	secrets secret.Resolver
	google  *googleTiles
	log     *slog.Logger
	warned  *warnings
	// project names whose keys these are, in a log line and in what is
	// remembered about it — two projects each missing their own Mapbox token
	// are two things for an operator to fix.
	project string
}

// New wires a Resolver reading keys from secrets.
func New(secrets secret.Resolver, log *slog.Logger) *Resolver {
	return &Resolver{
		secrets: secrets,
		google:  newGoogle(&http.Client{Timeout: 10 * time.Second}, googleBase),
		log:     log,
		warned:  &warnings{last: map[string]time.Time{}},
	}
}

/*
For is the same resolver reading one project's keys.

A tile key is a project's own — the portal stores it per project — so each
project resolves against its own secrets while sharing the sessions, which are
keyed by a digest of the key and so cannot hand one key's session to a project
that holds another.
*/
func (r *Resolver) For(project string, secrets secret.Resolver) *Resolver {
	c := *r
	c.secrets, c.project = secrets, project
	return &c
}

// Tiles resolves b.
func (r *Resolver) Tiles(ctx context.Context, b definition.Basemap, around run.Bounds) (*run.Tiles, error) {
	if b.Styled() == definition.AutoStyle {
		return r.themed(ctx, b, around)
	}
	return r.resolve(ctx, b, around, true)
}

// resolve is Tiles for one style. With wait false a Google session still
// being minted is an error rather than a wait — see themed.
func (r *Resolver) resolve(ctx context.Context, b definition.Basemap, around run.Bounds,
	wait bool) (*run.Tiles, error) {

	switch b.Provider {
	case "":
		return r.template(b)
	case definition.OpenStreetMap:
		return openStreetMap(b), nil
	}
	key, err := r.key(b)
	if err != nil {
		return nil, err
	}
	switch b.Provider {
	case definition.Mapbox:
		if strings.HasPrefix(key, "sk.") {
			// A secret token can read and write the account. It would be in
			// every tile request every reader's browser makes.
			r.warn("mapbox-secret", "basemap key refused", "provider", b.Provider,
				"why", "a Mapbox secret token (sk.) would be sent to every reader's browser; "+
					"use a public token (pk.) with the styles:tiles scope")
			return nil, unavailable(b.Provider)
		}
		return mapbox(b, key), nil
	case definition.GoogleMaps:
		t, err := r.google.tiles(ctx, b, key, around, wait)
		if errors.Is(err, errPending) {
			return nil, err
		}
		if err != nil {
			r.warn("google:"+b.Styled(), "basemap unavailable", "provider", b.Provider, "err", err)
			return nil, run.Unavailable{Reason: "Google Maps could not be reached, so the " +
				"map is drawn without its basemap."}
		}
		return t, nil
	}
	return nil, unavailable(b.Provider)
}

/*
themed resolves a basemap that follows the page's theme: the provider's light
map, carrying its dark one for a dark page. The light one is the map — a viewer
that predates themes draws it everywhere — so it is the one whose failure is
the map's; a dark one that cannot be had leaves the light one under both.
*/
func (r *Resolver) themed(ctx context.Context, b definition.Basemap, around run.Bounds) (*run.Tiles, error) {
	light, dark := b.Provider.Themes()
	b.Style = light
	t, err := r.resolve(ctx, b, around, true)
	if err != nil {
		return nil, err
	}
	// Minted behind the reader, as the sharper tiles are: waiting for it put a
	// second round trip to Google ahead of the first map anybody opened. The
	// first render after a restart draws the light map under both themes, and
	// the next has the dark one.
	b.Style = dark
	d, err := r.resolve(ctx, b, around, false)
	if err != nil {
		return t, nil
	}
	if d.Attribution == "" {
		// The same map data, the same credit: the dark session's own line is
		// asked for by the viewer as the reader pans.
		d.Attribution, d.Credits = t.Attribution, t.Credits
	}
	t.Dark = d
	return t, nil
}

// key resolves the provider's key, which is only ever a secret named as one.
func (r *Resolver) key(b definition.Basemap) (string, error) {
	ref := b.KeyRef()
	names := secret.Names(ref)
	if len(names) != 1 || !strings.HasPrefix(names[0], b.Provider.KeyPrefix()) {
		// definition refuses this when the report is stored. A store written
		// before that rule, or by something that skipped it, still does not
		// get to send an arbitrary secret to a browser.
		r.warn("name:"+ref, "basemap key refused", "provider", b.Provider,
			"why", "only a secret named "+b.Provider.KeyPrefix()+"… is sent to a browser")
		return "", unavailable(b.Provider)
	}
	v, err := secret.Resolve(ref, r.secrets)
	if err != nil || strings.TrimSpace(v) == "" {
		r.warn("missing:"+names[0], "basemap key is not configured — set it in the "+
			"project's Settings → Secrets, or in the environment", "provider", b.Provider,
			"secret", names[0], "environment", envName(names[0]))
		return "", unavailable(b.Provider)
	}
	return strings.TrimSpace(v), nil
}

// template resolves a URL basemap, and any tile key in it.
func (r *Resolver) template(b definition.Basemap) (*run.Tiles, error) {
	for _, name := range secret.Names(b.URL) {
		if !strings.HasPrefix(name, definition.TileKeyPrefix) {
			r.warn("name:"+name, "basemap key refused", "secret", name,
				"why", "only a secret named "+definition.TileKeyPrefix+"… is sent to a browser")
			return nil, run.Unavailable{Reason: "The basemap cannot be drawn on this server, " +
				"so the map is drawn without it."}
		}
	}
	resolved, err := secret.Resolve(b.URL, r.secrets)
	if err != nil {
		r.warn("missing-url:"+b.URL, "basemap key is not configured", "err", err)
		return nil, run.Unavailable{Reason: "The basemap cannot be drawn on this server, " +
			"so the map is drawn without it."}
	}
	b.URL = resolved
	url, url2x := b.Templates()
	return &run.Tiles{
		URL: url, URL2x: url2x, Attribution: b.Attribution,
		MaxZoom: b.Zoom(), TileSize: 256,
	}, nil
}

// unavailable is the reader's sentence for a provider this server cannot draw.
// It names no setting: the reader is our customer's customer, and the setting
// is in the log for the person who can change it.
func unavailable(p definition.BasemapProvider) error {
	return run.Unavailable{Reason: fmt.Sprintf(
		"%s is not set up on this server, so the map is drawn without its basemap.", p.Title())}
}

// envName is where a secret is read from in an environment, for the log line
// that tells an operator what to set.
func envName(name string) string {
	return "CRONOS_SECRET_" + strings.NewReplacer(".", "_", "-", "_").Replace(strings.ToUpper(name))
}

// warn logs once per reason every ten minutes.
//
// A basemap that cannot be drawn is resolved again on every render of every
// report that names it, and a line per render is a log nobody can read the
// rest of.
func (r *Resolver) warn(reason, msg string, args ...any) {
	if r.project != "" {
		reason = r.project + "\x00" + reason
		args = append([]any{"project", r.project}, args...)
	}
	if r.warned.due(reason, time.Now()) {
		r.log.Warn(msg, args...)
	}
}

// warnings remembers when each reason was last logged.
type warnings struct {
	mu   sync.Mutex
	last map[string]time.Time
}

// due reports whether reason may be logged now, and records that it was. The
// reasons carry names from definitions, so the map is bounded and emptied
// rather than allowed to grow with every name an author invents.
func (w *warnings) due(reason string, now time.Time) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if at, ok := w.last[reason]; ok && now.Sub(at) < 10*time.Minute {
		return false
	}
	if len(w.last) >= 256 {
		clear(w.last)
	}
	w.last[reason] = now
	return true
}
