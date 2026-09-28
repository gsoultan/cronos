package vault

import (
	"context"
	"time"
)

// readTimeout bounds a read the caller cannot cancel: secret.Resolver takes no
// context, and a render waiting on it should not wait on a stalled database for
// longer than a person waits on a map.
const readTimeout = 2 * time.Second

// Secret answers a ${secret:…} reference for this project: stored here first,
// then the deployment's own.
func (s *Service) Secret(name string) (string, bool) {
	if v, ok := s.stored(name); ok {
		return v, true
	}
	if s.shared != nil {
		return s.shared.Secret(name)
	}
	return "", false
}

// stored reads and opens one of the project's own, through the cache.
func (s *Service) stored(name string) (string, bool) {
	if !s.Available() {
		return "", false
	}
	org, project := s.where()
	now := s.now()
	if v, ok, hit := s.cache.get(org, project, name, now); hit {
		return v, ok
	}

	ctx, cancel := context.WithTimeout(context.Background(), readTimeout)
	defer cancel()
	sealed, found, err := s.store.Sealed(ctx, org, project, name)
	if err != nil {
		// Not remembered, so the next ask tries again once the store is back.
		s.log.Error("could not read a stored secret",
			"project", org+"/"+project, "secret", name, "err", err)
		return "", false
	}
	if !found {
		s.cache.put(org, project, name, "", false, now)
		return "", false
	}
	v, err := s.sealer.Open(sealed, org, project, name)
	if err != nil {
		// The one cause worth naming: the key changed and the old one was not
		// kept as a previous key, so everything sealed before is unreadable.
		// Remembered as absent, so a map rendered every second says this once
		// a minute rather than every second.
		s.log.Error("a stored secret could not be opened — was CRONOS_SECRETS_KEY "+
			"replaced without keeping the old one in CRONOS_SECRETS_KEY_PREVIOUS?",
			"project", org+"/"+project, "secret", name)
		s.cache.put(org, project, name, "", false, now)
		return "", false
	}
	s.cache.put(org, project, name, v, true, now)
	return v, true
}
