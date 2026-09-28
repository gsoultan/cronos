package vault

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gsoultan/cronos/internal/core/principal"
	"github.com/gsoultan/cronos/internal/platform/secret"
)

/*
List is the project's secrets as somebody managing them sees them: what is
stored here, and every name a definition uses whether or not anything answers
it — the second half being the point. A map with no basemap and a datasource
that will not open are both a secret nobody set, and this is where that shows.
*/
func (s *Service) List(ctx context.Context, pr principal.Principal) ([]Entry, error) {
	if err := s.may(pr); err != nil {
		return nil, err
	}
	uses := s.uses()
	var stored []Stored
	if s.Available() {
		org, project := s.where()
		var err error
		if stored, err = s.store.SecretsOf(ctx, org, project); err != nil {
			return nil, err
		}
	}

	out := make([]Entry, 0, len(stored)+len(uses))
	have := make(map[string]bool, len(stored))
	for _, st := range stored {
		at := st.UpdatedAt
		have[st.Name] = true
		out = append(out, Entry{Name: st.Name, Source: FromProject,
			UpdatedAt: &at, UpdatedBy: st.UpdatedBy, UsedBy: orNone(uses[st.Name])})
	}
	for name, by := range uses {
		if have[name] {
			continue
		}
		out = append(out, Entry{Name: name, Source: s.elsewhere(name), UsedBy: by})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// elsewhere says whether the deployment answers a name the project does not
// store. Asked, never returned: only whether there is a value.
func (s *Service) elsewhere(name string) Source {
	if s.shared != nil {
		if v, ok := s.shared.Secret(name); ok && v != "" {
			return FromDeployment
		}
	}
	return Missing
}

// Set seals value and stores it under name, replacing whatever was there.
func (s *Service) Set(ctx context.Context, pr principal.Principal, name, value string) error {
	if err := s.may(pr); err != nil {
		return err
	}
	if !s.Available() {
		return ErrUnavailable
	}
	// A trailing newline is what pasting from a file or a terminal adds, and
	// the same thing a mounted secret file has trimmed.
	value = strings.TrimRight(value, "\r\n")
	if err := valid(name, value); err != nil {
		return err
	}
	org, project := s.where()
	if err := s.room(ctx, org, project, name); err != nil {
		return err
	}

	at := s.now().UTC()
	err := s.store.PutSecret(ctx, Stored{
		Org: org, Project: project, Name: name,
		Sealed:    s.sealer.Seal(value, org, project, name),
		UpdatedAt: at, UpdatedBy: pr.Subject,
	})
	if err != nil {
		return err
	}
	s.settled(org, project, name, &at)
	return nil
}

// Delete removes a stored secret. What used it falls back to the deployment's
// own, or to nothing.
func (s *Service) Delete(ctx context.Context, pr principal.Principal, name string) error {
	if err := s.may(pr); err != nil {
		return err
	}
	if !s.Available() {
		return ErrUnavailable
	}
	org, project := s.where()
	gone, err := s.store.DeleteSecret(ctx, org, project, name)
	if err != nil {
		return err
	}
	if !gone {
		return fmt.Errorf("%w: %q", ErrNotFound, name)
	}
	s.settled(org, project, name, nil)
	return nil
}

// settled is everything that follows a change this replica made: the cached
// answer dropped, Watch told so it does not hear about it again, and whatever
// was built from the old value rebuilt.
func (s *Service) settled(org, project, name string, at *time.Time) {
	s.cache.forget(org, project, name)
	s.mu.Lock()
	if at == nil {
		delete(s.seen, name)
	} else {
		s.seen[name] = *at
	}
	s.mu.Unlock()
	s.notify(name)
}

// room refuses a new name once the project holds MaxSecrets. Replacing one it
// already holds is always allowed.
func (s *Service) room(ctx context.Context, org, project, name string) error {
	stored, err := s.store.SecretsOf(ctx, org, project)
	if err != nil {
		return err
	}
	for _, st := range stored {
		if st.Name == name {
			return nil
		}
	}
	if len(stored) >= MaxSecrets {
		return fmt.Errorf("%w: a project holds at most %d", ErrFull, MaxSecrets)
	}
	return nil
}

// valid checks a name can be referenced and a value can be stored.
func valid(name, value string) error {
	switch {
	case !secret.ValidName(name) || len(name) > maxNameBytes:
		return fmt.Errorf("%w: a name is letters, digits, dots, dashes and underscores, "+
			"at most %d of them — it is what ${secret:name} says", ErrInvalid, maxNameBytes)
	case value == "":
		return fmt.Errorf("%w: an empty value — delete the secret instead", ErrInvalid)
	case len(value) > MaxValueBytes:
		return fmt.Errorf("%w: at most %d bytes", ErrInvalid, MaxValueBytes)
	}
	return nil
}

func orNone(u []Use) []Use {
	if u == nil {
		return []Use{}
	}
	return u
}
