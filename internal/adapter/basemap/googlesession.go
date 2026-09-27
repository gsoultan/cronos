package basemap

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/gsoultan/cronos/internal/platform/secret"
)

const (
	// maxSessions bounds the cache. Its keys come from definitions — a key, a
	// style, a language and a region per basemap — so an author can grow it,
	// and every map keyed by input somebody else chose needs a ceiling. A
	// session is a few dozen bytes, so the ceiling is set by how many a
	// deployment could honestly use, not by memory.
	maxSessions = 256
	// renewBefore is how long before a session expires it is replaced, so a
	// tile a reader requests at the last moment is not refused mid-pan.
	renewBefore = time.Hour
	// retryAfter is how long a failed mint is remembered. Without it, every
	// render during a Google outage — or with a key Google has revoked —
	// waits out a timeout of its own before drawing the map without tiles.
	retryAfter = time.Minute
	// mintTimeout bounds the one network call a render may wait on.
	mintTimeout = 5 * time.Second
	// maxBody bounds what is read of Google's answer. A session is a few
	// hundred bytes; anything past this is not one.
	maxBody = 64 << 10
)

// errPending answers a caller that would rather go without than wait for a
// mint in flight — see tiles, for the sharper of the two sessions.
var errPending = errors.New("a session is being minted")

// sessionRequest is createSession's body.
type sessionRequest struct {
	MapType    string   `json:"mapType"`
	Language   string   `json:"language"`
	Region     string   `json:"region"`
	LayerTypes []string `json:"layerTypes,omitempty"`
	Scale      string   `json:"scale,omitempty"`
	HighDPI    bool     `json:"highDpi,omitempty"`
}

// sessionKey is what one session serves. The API key is held as a digest: the
// cache outlives the request that resolved it, and a map key is not a place a
// credential needs to be readable from.
type sessionKey struct {
	key      [sha256.Size]byte
	mapType  string
	layers   string
	language string
	region   string
	scale    string
}

// session is one minted session, or the attempt to mint it.
type session struct {
	token   string
	expires time.Time
	// failed is when the last attempt failed, zero after a success, and err
	// is why. Kept, so every render inside retryAfter logs Google's reason —
	// "API key not valid" — rather than that there was one.
	failed time.Time
	err    error
	// minting is closed when an attempt in flight finishes; nil when none is.
	minting chan struct{}
}

// session returns a live session for req, minting one if there is none.
//
// One mint per key at a time: the first render after a restart asks Google,
// and every render that arrives meanwhile waits on the same answer rather
// than asking again. The mint runs on its own, so a render whose reader has
// gone stops waiting and the rest still get the session; and a session near
// its expiry is renewed behind the reader rather than in front of them — the
// old one works until it lapses. With wait false a caller is told errPending
// instead of waiting for a mint in flight.
func (g *googleTiles) session(ctx context.Context, key string, req sessionRequest,
	wait bool) (string, error) {

	k := sessionKey{
		key: sha256.Sum256([]byte(key)), mapType: req.MapType, layers: fmt.Sprint(req.LayerTypes),
		language: req.Language, region: req.Region, scale: req.Scale,
	}
	// Found once and held: an entry evicted while this caller waits is still
	// the one its mint writes into.
	g.mu.Lock()
	s := g.sessions[k]
	if s == nil {
		s = g.room(k)
	}
	g.mu.Unlock()

	for {
		g.mu.Lock()
		now := g.now()
		live := s.token != "" && now.Before(s.expires)
		fresh := live && now.Before(s.expires.Add(-renewBefore))
		failed := !s.failed.IsZero() && now.Sub(s.failed) < retryAfter
		switch {
		case fresh, live && (s.minting != nil || failed):
			token := s.token
			g.mu.Unlock()
			return token, nil
		case live:
			g.start(ctx, s, key, req)
			token := s.token
			g.mu.Unlock()
			return token, nil
		case failed && s.minting == nil:
			err := s.err
			g.mu.Unlock()
			return "", err
		case s.minting == nil:
			g.start(ctx, s, key, req)
		}
		done := s.minting
		g.mu.Unlock()
		if !wait {
			return "", errPending
		}
		select {
		case <-done:
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
}

// start mints into s on its own goroutine. Called with the lock held.
//
// Detached from the caller's cancellation, with a timeout of its own: every
// render waiting on this mint shares its answer, so the first reader closing
// their tab must not fail it for the rest.
func (g *googleTiles) start(ctx context.Context, s *session, key string, req sessionRequest) {
	s.minting = make(chan struct{})
	go g.mintInto(context.WithoutCancel(ctx), s, key, req)
}

// room makes a place for k in the cache. Called with the lock held.
//
// A full cache gives up a session that cannot serve anybody — failed, or
// lapsed — before one that can, and the live session nearest its expiry
// before the others. One whose every entry is mid-mint gives up nothing: the
// caller gets a session of its own that the cache does not keep, rather than
// a cache that grows past its ceiling.
func (g *googleTiles) room(k sessionKey) *session {
	s := &session{}
	if len(g.sessions) >= maxSessions {
		victim, ok := g.victim()
		if !ok {
			return s
		}
		delete(g.sessions, victim)
	}
	g.sessions[k] = s
	return s
}

// victim is the entry room gives up, if any may be. Called with the lock
// held.
func (g *googleTiles) victim() (sessionKey, bool) {
	now := g.now()
	var (
		pick  sessionKey
		until time.Time
		found bool
	)
	for k, s := range g.sessions {
		if s.minting != nil {
			continue
		}
		// A session that cannot serve counts as expiring at the zero time,
		// which is before any that can. It was treated as "nothing picked
		// yet", and a failed entry handed the eviction to whichever live
		// session came after it.
		at := s.expires
		if s.token == "" || !now.Before(s.expires) {
			at = time.Time{}
		}
		if !found || at.Before(until) {
			pick, until, found = k, at, true
		}
	}
	return pick, found
}

// mintInto asks Google for a session and records the answer in s.
//
// A session that is expiring but not expired survives a failed renewal: the
// tiles it names are still served, and a reader is better off with them than
// with a map that lost its streets because Google was slow for a minute.
func (g *googleTiles) mintInto(ctx context.Context, s *session, key string, req sessionRequest) {
	token, expires, err := g.mint(ctx, key, req)

	g.mu.Lock()
	defer g.mu.Unlock()
	close(s.minting)
	s.minting = nil
	if err != nil {
		s.failed, s.err = g.now(), err
		return
	}
	s.token, s.expires, s.failed, s.err = token, expires, time.Time{}, nil
}

// mint is one createSession call.
func (g *googleTiles) mint(ctx context.Context, key string, req sessionRequest) (string, time.Time, error) {
	ctx, cancel := context.WithTimeout(ctx, mintTimeout)
	defer cancel()

	body, err := json.Marshal(req)
	if err != nil {
		return "", time.Time{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		g.base+"/v1/createSession?key="+url.QueryEscape(key), bytes.NewReader(body))
	if err != nil {
		return "", time.Time{}, secret.Redact(err, key, url.QueryEscape(key))
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(httpReq)
	if err != nil {
		// net/http quotes the URL it failed to reach, and the key is in it.
		return "", time.Time{}, secret.Redact(err, key, url.QueryEscape(key))
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return "", time.Time{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return "", time.Time{}, secret.Redact(fmt.Errorf("createSession answered %s: %s",
			resp.Status, googleMessage(raw)), key)
	}
	return parseSession(raw)
}

// parseSession reads createSession's answer.
func parseSession(raw []byte) (string, time.Time, error) {
	var out struct {
		Session string `json:"session"`
		Expiry  string `json:"expiry"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", time.Time{}, fmt.Errorf("createSession answered something else: %w", err)
	}
	secs, err := strconv.ParseInt(out.Expiry, 10, 64)
	if out.Session == "" || err != nil {
		return "", time.Time{}, errors.New("createSession answered without a session and its expiry")
	}
	return out.Session, time.Unix(secs, 0), nil
}

// googleMessage is the sentence out of Google's error envelope, or the start
// of the body when there is none.
func googleMessage(raw []byte) string {
	var envelope struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &envelope) == nil && envelope.Error.Message != "" {
		return envelope.Error.Message
	}
	if len(raw) > 200 {
		raw = raw[:200]
	}
	return string(raw)
}
