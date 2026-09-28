package vault

import (
	"context"
	"time"
)

/*
Watch notices secrets changed by another replica.

A change made here rebuilds this replica's connections at once. The others have
a cached answer and a connection pool opened with the old password, and nothing
told them — so without this, rotating a warehouse password through the portal
works on whichever instance served the request and fails on the rest until they
restart.

So each replica reads the project's list every interval and compares when each
secret was last set. Only names and times: nothing is opened until something
asks for it.
*/
func (s *Service) Watch(ctx context.Context, every time.Duration) {
	if !s.Available() {
		return
	}
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		s.look(ctx)
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// look reads the list once and reports what differs from the last read. The
// first read only remembers: there is nothing it could differ from.
func (s *Service) look(ctx context.Context) {
	org, project := s.where()
	stored, err := s.store.SecretsOf(ctx, org, project)
	if err != nil {
		if ctx.Err() == nil {
			s.log.Warn("could not check for changed secrets",
				"project", org+"/"+project, "err", err)
		}
		return
	}
	now := make(map[string]time.Time, len(stored))
	for _, st := range stored {
		now[st.Name] = st.UpdatedAt
	}

	s.mu.Lock()
	var changed []string
	if s.watching {
		changed = differences(s.seen, now)
	}
	s.seen, s.watching = now, true
	s.mu.Unlock()

	for _, name := range changed {
		s.cache.forget(org, project, name)
		s.notify(name)
	}
}

// differences is every name set, replaced or removed between two reads.
func differences(was, now map[string]time.Time) []string {
	var out []string
	for name, at := range now {
		if before, ok := was[name]; !ok || !before.Equal(at) {
			out = append(out, name)
		}
	}
	for name := range was {
		if _, ok := now[name]; !ok {
			out = append(out, name)
		}
	}
	return out
}
