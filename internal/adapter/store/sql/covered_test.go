package sql_test

import (
	"context"
	"testing"

	store "github.com/gsoultan/cronos/internal/adapter/store/sql"
	"github.com/gsoultan/cronos/internal/core/principal"
)

// A project with nobody in it. Reachable with a machine credential — the admin
// key carries a project and has no account row — and by any project whose
// people have all been turned off.
func TestCoveredInAnEmptyProject(t *testing.T) {
	both(t, func(t *testing.T, s *store.Store) {
		with, without, err := s.Covered(context.Background(),
			principal.Principal{OrgID: "acme", ProjectID: "empty"})
		if err != nil {
			t.Fatalf("counting an empty project failed: %v", err)
		}
		if with != 0 || without != 0 {
			t.Fatalf("got %d covered, %d not", with, without)
		}
	})
}
