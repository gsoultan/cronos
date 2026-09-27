package basemap

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"time"

	"github.com/gsoultan/cronos/internal/app/run"
)

const (
	// maxCredits bounds the copyright cache, which grows with every distinct
	// view a filter produces.
	maxCredits = 256
	// creditLife is how long a copyright line is reused. Google changes the
	// year once a year and the imagery providers rarely; an hour is a request
	// an hour per view instead of one per render.
	creditLife = time.Hour
	// creditTimeout is shorter than a mint's: a render waits on it, and the
	// line it fetches has a fallback where the session has none.
	creditTimeout = 2 * time.Second
)

// creditKey is one view of one session: the tokens as a digest, for the
// reason sessionKey holds the API key as one, and the box rounded to a
// hundredth of a world unit — about four hundred kilometres, well inside the
// area one copyright line covers.
type creditKey struct {
	session [sha256.Size]byte
	zoom    int
	box     [4]int
}

type credit struct {
	line string
	at   time.Time
}

/*
copyright is the credit line Google requires under its tiles, for the view the
map is first drawn at.

Google's terms make it mandatory and make it depend on the view — the imagery
over a desert and over a city come from different providers — so it is asked
for rather than written. This is the line for the map as it arrives; the viewer
asks again at Tiles.Viewport as the reader pans. Should Google not answer, the
line falls back to Google's own name rather than to nothing: an incomplete
credit is a smaller breach than a blank one, and the map still draws.
*/
func (g *googleTiles) copyright(ctx context.Context, token, key string, around run.Bounds,
	most int) string {

	zoom := fitZoom(around, most)
	k := creditKey{session: sha256.Sum256([]byte(token)), zoom: zoom, box: [4]int{
		int(around.MinX * 100), int(around.MinY * 100), int(around.MaxX * 100), int(around.MaxY * 100),
	}}
	g.mu.Lock()
	c, ok := g.credits[k]
	g.mu.Unlock()
	if ok && g.now().Sub(c.at) < creditLife {
		return c.line
	}

	line, err := g.viewport(ctx, token, key, around, zoom)
	if err != nil || line == "" {
		return fmt.Sprintf("Map data ©%d Google", g.now().Year())
	}
	g.mu.Lock()
	if len(g.credits) >= maxCredits {
		clear(g.credits)
	}
	g.credits[k] = credit{line: line, at: g.now()}
	g.mu.Unlock()
	return line
}

// viewport is one viewport-information request.
func (g *googleTiles) viewport(ctx context.Context, token, key string, b run.Bounds,
	zoom int) (string, error) {

	ctx, cancel := context.WithTimeout(ctx, creditTimeout)
	defer cancel()

	q := url.Values{
		"session": {token}, "key": {key}, "zoom": {fmt.Sprint(zoom)},
		"north": {degrees(latitudeOf(b.MinY))}, "south": {degrees(latitudeOf(b.MaxY))},
		"west": {degrees(b.MinX*360 - 180)}, "east": {degrees(b.MaxX*360 - 180)},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		g.base+"/tile/v1/viewport?"+q.Encode(), nil)
	if err != nil {
		return "", err
	}
	resp, err := g.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("viewport answered %s", resp.Status)
	}
	var out struct {
		Copyright string `json:"copyright"`
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return "", err
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", err
	}
	return out.Copyright, nil
}

// latitudeOf is the inverse of the projection for a world-unit y.
func latitudeOf(y float64) float64 {
	return math.Atan(math.Sinh(math.Pi*(1-2*y))) * 180 / math.Pi
}

func degrees(v float64) string { return fmt.Sprintf("%.6f", v) }
