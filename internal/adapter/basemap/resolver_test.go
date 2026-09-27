package basemap

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gsoultan/cronos/internal/app/run"
	"github.com/gsoultan/cronos/internal/core/definition"
)

// secrets is a resolver over a map, standing in for the environment.
type secrets map[string]string

func (s secrets) Secret(name string) (string, bool) { v, ok := s[name]; return v, ok }

// around is the box of a map of the Netherlands, in world units.
var around = run.Bounds{MinX: 0.508, MinY: 0.325, MaxX: 0.521, MaxY: 0.337}

func resolver(s secrets) (*Resolver, *bytes.Buffer) {
	var logs bytes.Buffer
	return New(s, slog.New(slog.NewTextHandler(&logs, nil))), &logs
}

func TestOpenStreetMapCreditsItsContributors(t *testing.T) {
	r, _ := resolver(nil)
	tiles, err := r.Tiles(context.Background(), definition.Basemap{Provider: definition.OpenStreetMap, MaxZoom: 12}, around)
	if err != nil {
		t.Fatal(err)
	}
	if tiles.URL != "https://tile.openstreetmap.org/{z}/{x}/{y}.png" {
		t.Errorf("url = %q", tiles.URL)
	}
	if len(tiles.Credits) == 0 || tiles.Credits[0].Href != "https://www.openstreetmap.org/copyright" {
		t.Errorf("the credit does not link the copyright page, which the licence asks for: %+v", tiles.Credits)
	}
	if tiles.MaxZoom != 12 {
		t.Errorf("maxZoom = %d, want the author's cap of 12", tiles.MaxZoom)
	}
}

func TestMapboxTilesCarryTheTokenTheLogoAndTheCredits(t *testing.T) {
	r, _ := resolver(secrets{"mapbox-token": "pk.test-token"})
	tiles, err := r.Tiles(context.Background(),
		definition.Basemap{Provider: definition.Mapbox, Style: "satellite-streets"}, around)
	if err != nil {
		t.Fatal(err)
	}
	want := "https://api.mapbox.com/styles/v1/mapbox/satellite-streets-v12/tiles/512/{z}/{x}/{y}?access_token=pk.test-token"
	if tiles.URL != want {
		t.Errorf("url = %q\nwant  %q", tiles.URL, want)
	}
	if !strings.Contains(tiles.URL2x, "{y}@2x?access_token=") {
		t.Errorf("url2x = %q, want the @2x tiles", tiles.URL2x)
	}
	if tiles.TileSize != 512 {
		t.Errorf("tileSize = %d, want 512 — a viewer assuming 256 asks for one zoom too deep", tiles.TileSize)
	}
	if !strings.HasPrefix(tiles.Logo, "data:image/svg+xml,") || tiles.LogoAlt != "Mapbox" {
		t.Errorf("the wordmark Mapbox's terms require is missing: %.40q / %q", tiles.Logo, tiles.LogoAlt)
	}
	var texts []string
	for _, c := range tiles.Credits {
		texts = append(texts, c.Text)
	}
	if got := strings.Join(texts, " | "); got != "© Mapbox | © OpenStreetMap | © Maxar | Improve this map" {
		t.Errorf("credits = %q", got)
	}
}

func TestAStudioStyleIsAskedForByItsOwnID(t *testing.T) {
	r, _ := resolver(secrets{"mapbox-token": "pk.x"})
	tiles, err := r.Tiles(context.Background(),
		definition.Basemap{Provider: definition.Mapbox, Style: "acme/ckx1y2z3"}, around)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(tiles.URL, "/styles/v1/acme/ckx1y2z3/tiles/") {
		t.Errorf("url = %q", tiles.URL)
	}
}

// A secret token can write to the account, and every tile request carries it.
func TestAMapboxSecretTokenIsNeverSentToABrowser(t *testing.T) {
	r, logs := resolver(secrets{"mapbox-token": "sk.can-write-everything"})
	_, err := r.Tiles(context.Background(), definition.Basemap{Provider: definition.Mapbox}, around)

	var u run.Unavailable
	if !errors.As(err, &u) {
		t.Fatalf("a secret token was accepted: %v", err)
	}
	if !strings.Contains(logs.String(), "public token (pk.)") {
		t.Errorf("the log does not tell the operator what to use instead:\n%s", logs)
	}
	if strings.Contains(logs.String(), "can-write-everything") {
		t.Error("the refused token was written to the log")
	}
}

func TestAMissingKeyTellsTheReaderLittleAndTheOperatorEverything(t *testing.T) {
	r, logs := resolver(secrets{})
	_, err := r.Tiles(context.Background(), definition.Basemap{Provider: definition.Mapbox}, around)

	var u run.Unavailable
	if !errors.As(err, &u) {
		t.Fatalf("err = %v, want Unavailable", err)
	}
	if strings.Contains(u.Reason, "CRONOS") || strings.Contains(u.Reason, "secret") {
		t.Errorf("the reader was shown a setting: %q", u.Reason)
	}
	if !strings.Contains(logs.String(), "CRONOS_SECRET_MAPBOX_TOKEN") {
		t.Errorf("the operator was not told what to set:\n%s", logs)
	}

	// Once, not once per render.
	logs.Reset()
	_, _ = r.Tiles(context.Background(), definition.Basemap{Provider: definition.Mapbox}, around)
	if logs.Len() != 0 {
		t.Errorf("the same warning was logged again:\n%s", logs)
	}
}

// definition refuses these when a report is stored. The adapter refuses them
// again, because a store written before that rule existed still reaches it.
func TestOnlyATileKeyIsResolvedIntoABrowserURL(t *testing.T) {
	r, _ := resolver(secrets{"warehouse-password": "hunter2", "tiles-example": "k-123"})

	tiles, err := r.Tiles(context.Background(), definition.Basemap{
		URL: "https://t.example/{z}/{x}/{y}{r}.png?key=${secret:tiles-example}", Attribution: "© Example",
	}, around)
	if err != nil {
		t.Fatal(err)
	}
	if tiles.URL != "https://t.example/{z}/{x}/{y}.png?key=k-123" ||
		tiles.URL2x != "https://t.example/{z}/{x}/{y}@2x.png?key=k-123" {
		t.Errorf("urls = %q, %q", tiles.URL, tiles.URL2x)
	}

	for _, b := range []definition.Basemap{
		{URL: "https://evil.example/{z}/{x}/{y}.png?k=${secret:warehouse-password}", Attribution: "©"},
		{Provider: definition.Mapbox, Key: "${secret:warehouse-password}"},
	} {
		tiles, err := r.Tiles(context.Background(), b, around)
		if err == nil || (tiles != nil && strings.Contains(tiles.URL, "hunter2")) {
			t.Fatalf("the warehouse password was put in a tile url: %+v", tiles)
		}
	}
}

// google is a fake Map Tiles API that counts what it is asked.
type google struct {
	sessions atomic.Int32
	views    atomic.Int32
	fail     atomic.Bool
	// gate holds createSession until it is closed, so a test can pile
	// renders up behind one mint; gate2x holds the high-density one alone.
	gate   chan struct{}
	gate2x chan struct{}
	mu     sync.Mutex
	seen   []map[string]any
}

func (g *google) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/v1/createSession":
		n := g.sessions.Add(1)
		var body map[string]any
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		if g.gate != nil {
			<-g.gate
		}
		if g.gate2x != nil && body["scale"] == "scaleFactor2x" {
			<-g.gate2x
		}
		if g.fail.Load() || r.URL.Query().Get("key") != "AIza-test-key" {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"error":{"code":403,"message":"Map Tiles API has not been used in project 42"}}`)
			return
		}
		g.mu.Lock()
		g.seen = append(g.seen, body)
		g.mu.Unlock()
		fmt.Fprintf(w, `{"session":"s%d","expiry":"%d","tileWidth":256,"tileHeight":256}`,
			n, time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC).Unix())
	case "/tile/v1/viewport":
		g.views.Add(1)
		fmt.Fprint(w, `{"copyright":"Map data ©2026 Google, Imagery ©2026 TerraMetrics"}`)
	default:
		http.NotFound(w, r)
	}
}

// release opens a gate once, and again at cleanup — before the fake server
// closes, which waits for its handlers. A test that fails while one is held
// at the gate would otherwise hang until the test binary's own timeout.
func release(t *testing.T, gate chan struct{}) func() {
	t.Helper()
	open := sync.OnceFunc(func() { close(gate) })
	t.Cleanup(open)
	return open
}

// minted waits until the fake has been asked for n sessions — the sharper
// one is minted behind the render that wanted it, so it arrives after.
func minted(t *testing.T, g *google, n int32) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for g.sessions.Load() < n && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := g.sessions.Load(); got != n {
		t.Fatalf("Google was asked for %d sessions, want %d", got, n)
	}
}

func googleResolver(t *testing.T, g *google, now *time.Time) (*Resolver, *bytes.Buffer) {
	t.Helper()
	srv := httptest.NewServer(g)
	t.Cleanup(srv.Close)
	r, logs := resolver(secrets{"google-maps-key": "AIza-test-key"})
	r.google = newGoogle(srv.Client(), srv.URL)
	r.google.now = func() time.Time { return *now }
	return r, logs
}

func TestGoogleTilesNeedASessionAndCarryTheCopyrightInView(t *testing.T) {
	g := &google{}
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	r, _ := googleResolver(t, g, &now)

	tiles, err := r.Tiles(context.Background(),
		definition.Basemap{Provider: definition.GoogleMaps, Style: "terrain", Language: "id", Region: "id"}, around)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(tiles.URL, "/v1/2dtiles/{z}/{x}/{y}?session=s1&key=AIza-test-key") {
		t.Errorf("url = %q", tiles.URL)
	}
	// The sharper session is minted behind the first render and served to
	// the next.
	minted(t, g, 2)
	time.Sleep(20 * time.Millisecond)
	again, err := r.Tiles(context.Background(),
		definition.Basemap{Provider: definition.GoogleMaps, Style: "terrain", Language: "id", Region: "id"}, around)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(again.URL2x, "session=s2") {
		t.Errorf("url2x = %q, want the high-density session", again.URL2x)
	}
	if tiles.Attribution != "Map data ©2026 Google, Imagery ©2026 TerraMetrics" {
		t.Errorf("attribution = %q, want the line Google gave for the view", tiles.Attribution)
	}
	if tiles.LogoAlt != "Google Maps" || !strings.HasPrefix(tiles.Logo, "data:image/svg+xml,") {
		t.Error("the Google Maps logo the terms require is missing")
	}
	if !strings.Contains(tiles.Viewport, "/tile/v1/viewport?session=s1&key=") {
		t.Errorf("viewport = %q, want where the viewer asks again as the reader pans", tiles.Viewport)
	}

	first := g.seen[0]
	if first["mapType"] != "terrain" || first["language"] != "id" || first["region"] != "ID" {
		t.Errorf("createSession was asked for %v", first)
	}
	if layers, _ := first["layerTypes"].([]any); len(layers) != 1 || layers[0] != "layerRoadmap" {
		t.Errorf("terrain needs layerRoadmap, and was asked with %v", first["layerTypes"])
	}
}

// Every reader shares a session. Minting one per render would put a round
// trip to Google in front of every map anybody opened.
func TestOneSessionServesEveryRenderThatArrivesWhileItIsMinted(t *testing.T) {
	g := &google{gate: make(chan struct{})}
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	r, _ := googleResolver(t, g, &now)
	open := release(t, g.gate)

	var wg sync.WaitGroup
	urls := make([]string, 20)
	for i := range urls {
		wg.Go(func() {
			tiles, err := r.Tiles(context.Background(), definition.Basemap{Provider: definition.GoogleMaps}, around)
			if err == nil {
				urls[i] = tiles.URL
			}
		})
	}
	time.Sleep(50 * time.Millisecond)
	open()
	wg.Wait()

	minted(t, g, 2)
	time.Sleep(50 * time.Millisecond)
	if n := g.sessions.Load(); n != 2 {
		t.Errorf("twenty renders minted %d sessions, want one per pixel density", n)
	}
	for _, u := range urls {
		if u != urls[0] || u == "" {
			t.Fatalf("renders drew different or no tiles: %q vs %q", u, urls[0])
		}
	}
}

func TestASessionIsRenewedBeforeItLapsesAndNotAfter(t *testing.T) {
	g := &google{}
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	r, _ := googleResolver(t, g, &now)
	ask := func() string {
		tiles, err := r.Tiles(context.Background(), definition.Basemap{Provider: definition.GoogleMaps}, around)
		if err != nil {
			t.Fatal(err)
		}
		return tiles.URL
	}

	first := ask()
	minted(t, g, 2)
	now = now.Add(24 * time.Hour)
	if ask() != first || g.sessions.Load() != 2 {
		t.Fatalf("a day-old session was replaced (%d mints)", g.sessions.Load())
	}

	// Inside the last hour: the old session answers, and a new one is minted
	// behind it for the next reader.
	now = time.Date(2026, 10, 10, 23, 30, 0, 0, time.UTC)
	if ask() != first {
		t.Error("the reader waited for a renewal the old session could still serve")
	}
	deadline := time.Now().Add(2 * time.Second)
	for g.sessions.Load() < 4 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if ask() == first {
		t.Error("an expiring session was never replaced")
	}
}

func TestAGoogleOutageIsRememberedRatherThanWaitedOutByEveryRender(t *testing.T) {
	g := &google{}
	g.fail.Store(true)
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	r, logs := googleResolver(t, g, &now)

	for range 5 {
		_, err := r.Tiles(context.Background(), definition.Basemap{Provider: definition.GoogleMaps}, around)
		var u run.Unavailable
		if !errors.As(err, &u) {
			t.Fatalf("err = %v, want Unavailable", err)
		}
	}
	if n := g.sessions.Load(); n != 1 {
		t.Errorf("five renders asked Google %d times in the same minute", n)
	}
	if !strings.Contains(logs.String(), "has not been used in project 42") {
		t.Errorf("Google's own diagnosis did not reach the log:\n%s", logs)
	}
	if strings.Contains(logs.String(), "AIza-test-key") {
		t.Errorf("the API key was written to the log:\n%s", logs)
	}

	now = now.Add(2 * time.Minute)
	g.fail.Store(false)
	if _, err := r.Tiles(context.Background(), definition.Basemap{Provider: definition.GoogleMaps}, around); err != nil {
		t.Errorf("Google came back and the basemap did not: %v", err)
	}
}

// The key is in the URL net/http quotes when a request fails, so an
// unreachable Google would otherwise print it.
func TestAnUnreachableGoogleDoesNotLogTheKey(t *testing.T) {
	r, logs := resolver(secrets{"google-maps-key": "AIza-test-key"})
	r.google = newGoogle(&http.Client{Timeout: time.Second}, "http://127.0.0.1:1")

	_, err := r.Tiles(context.Background(), definition.Basemap{Provider: definition.GoogleMaps}, around)
	if err == nil {
		t.Fatal("an unreachable Google answered")
	}
	if strings.Contains(logs.String(), "AIza-test-key") {
		t.Errorf("the API key was written to the log:\n%s", logs)
	}
	if !strings.Contains(logs.String(), "[redacted]") {
		t.Errorf("want the failure logged with the key removed, got:\n%s", logs)
	}
}

// The first render after a restart draws the ordinary tiles; it does not wait
// on a second round trip to Google for sharper ones.
func TestAReaderIsNotKeptWaitingForSharperTiles(t *testing.T) {
	g := &google{gate2x: make(chan struct{})}
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	r, _ := googleResolver(t, g, &now)
	open := release(t, g.gate2x)

	done := make(chan *run.Tiles, 1)
	go func() {
		tiles, _ := r.Tiles(context.Background(), definition.Basemap{Provider: definition.GoogleMaps}, around)
		done <- tiles
	}()
	select {
	case tiles := <-done:
		if tiles == nil || tiles.URL == "" || tiles.URL2x != "" {
			t.Fatalf("tiles = %+v, want the ordinary tiles and no sharper ones yet", tiles)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the render waited for the high-density session")
	}
	open()
	minted(t, g, 2)
	time.Sleep(20 * time.Millisecond)
	tiles, err := r.Tiles(context.Background(), definition.Basemap{Provider: definition.GoogleMaps}, around)
	if err != nil || tiles.URL2x == "" {
		t.Errorf("the next render still has no sharper tiles: %+v, %v", tiles, err)
	}
}

// A reader who leaves stops their render waiting. The mint goes on for
// everybody else, and is not asked for again.
func TestARenderWhoseReaderLeftStopsWaitingAndTheMintGoesOn(t *testing.T) {
	g := &google{gate: make(chan struct{})}
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	r, _ := googleResolver(t, g, &now)
	open := release(t, g.gate)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	began := time.Now()
	if _, err := r.Tiles(ctx, definition.Basemap{Provider: definition.GoogleMaps}, around); err == nil {
		t.Fatal("a render with nobody waiting for it drew a basemap it had not got")
	}
	if waited := time.Since(began); waited > time.Second {
		t.Errorf("the render waited %v after its reader left", waited)
	}
	open()
	minted(t, g, 1)
	time.Sleep(20 * time.Millisecond)
	if _, err := r.Tiles(context.Background(), definition.Basemap{Provider: definition.GoogleMaps}, around); err != nil {
		t.Fatalf("the mint the first render started did not reach the next: %v", err)
	}
	minted(t, g, 2) // the one high-density session, and no second ordinary one
}

// A full cache gives up a session that serves nobody before one that serves
// everybody. A failed entry expires at the zero time, which the eviction read
// as "nothing picked yet" — and handed the eviction to the next live session.
func TestAFullCacheGivesUpADeadSessionBeforeALiveOne(t *testing.T) {
	g := newGoogle(http.DefaultClient, "http://unused.invalid")
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	g.now = func() time.Time { return now }
	live := func(i int) sessionKey { return sessionKey{region: fmt.Sprint("live", i)} }
	for i := range maxSessions - 1 {
		g.sessions[live(i)] = &session{token: "t", expires: now.Add(time.Duration(i+1) * time.Hour)}
	}
	dead := sessionKey{region: "failed"}
	g.sessions[dead] = &session{failed: now, err: fmt.Errorf("refused")}

	for i := range 50 {
		g.room(sessionKey{region: fmt.Sprint("new", i)})
		if _, ok := g.sessions[dead]; !ok {
			break
		}
	}
	if _, ok := g.sessions[dead]; ok {
		t.Fatal("the failed session outlived fifty evictions")
	}
	for i := range maxSessions - 50 {
		if _, ok := g.sessions[live(maxSessions-2-i)]; !ok {
			t.Fatalf("a live session with %dh left was evicted while dead ones remained", maxSessions-1-i)
		}
	}
	if len(g.sessions) > maxSessions {
		t.Errorf("the cache holds %d sessions, over its %d", len(g.sessions), maxSessions)
	}
}

// Every entry mid-mint, nothing can go — and the cache still does not grow.
func TestASaturatedCacheDoesNotGrowPastItsCeiling(t *testing.T) {
	g := newGoogle(http.DefaultClient, "http://unused.invalid")
	for i := range maxSessions {
		g.sessions[sessionKey{region: fmt.Sprint(i)}] = &session{minting: make(chan struct{})}
	}
	s := g.room(sessionKey{region: "one more"})
	if s == nil || len(g.sessions) != maxSessions {
		t.Errorf("session = %v and the cache holds %d, want a session of its own and %d", s,
			len(g.sessions), maxSessions)
	}
}

