package vault

import (
	"sync"
	"time"
)

/*
Cache remembers what the store answered, per project and name, for a while.

A basemap key is resolved on every render of every map, and without this each
one is a round trip to the database and a decryption — per request, which is
the cost a check must not have. So an answer, including "not stored", is kept
for the TTL and dropped the moment this process changes the secret; another
replica's change reaches this one through Watch.

Keyed by organisation and project as well as name: two projects each have a
mapbox-token, and a cache keyed by the name alone would hand one project's key
to the other. Bounded, because the names come from definitions editors write.
*/
type Cache struct {
	mu      sync.Mutex
	ttl     time.Duration
	max     int
	entries map[string]cached
}

type cached struct {
	value string
	ok    bool
	at    time.Time
}

// NewCache holds up to max answers, each for ttl.
func NewCache(ttl time.Duration, max int) *Cache {
	return &Cache{ttl: ttl, max: max, entries: map[string]cached{}}
}

func (c *Cache) get(org, project, name string, now time.Time) (value string, ok, hit bool) {
	if c == nil {
		return "", false, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	e, found := c.entries[cacheKey(org, project, name)]
	if !found || now.Sub(e.at) >= c.ttl {
		return "", false, false
	}
	return e.value, e.ok, true
}

func (c *Cache) put(org, project, name, value string, ok bool, now time.Time) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) >= c.max {
		for k, e := range c.entries {
			if now.Sub(e.at) >= c.ttl {
				delete(c.entries, k)
			}
		}
		// Still full of live answers: start again rather than pick. The cost
		// is one read each, and a map that grows with every name an editor
		// invents is the alternative.
		if len(c.entries) >= c.max {
			clear(c.entries)
		}
	}
	c.entries[cacheKey(org, project, name)] = cached{value: value, ok: ok, at: now}
}

func (c *Cache) forget(org, project, name string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, cacheKey(org, project, name))
}

// cacheKey joins the three with a byte none of them may contain, so "a"+"b/c"
// and "a/b"+"c" are different keys.
func cacheKey(org, project, name string) string {
	return org + "\x00" + project + "\x00" + name
}
