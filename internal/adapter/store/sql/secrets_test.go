package sql_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	store "github.com/gsoultan/cronos/internal/adapter/store/sql"
	"github.com/gsoultan/cronos/internal/app/vault"
)

func sealedRow(org, project, name, sealed string, at time.Time) vault.Stored {
	return vault.Stored{Org: org, Project: project, Name: name,
		Sealed: []byte(sealed), UpdatedAt: at, UpdatedBy: "u-1"}
}

// Every read carries the tenant: the same name in two projects is two rows,
// and neither project's reads return the other's.
func TestSecretsAreScopedToTheirProject(t *testing.T) {
	both(t, func(t *testing.T, s *store.Store) {
		ctx := context.Background()
		for _, st := range []vault.Stored{
			sealedRow("acme", "finance", "pw", "finance-ciphertext", StoreNow),
			sealedRow("acme", "sales", "pw", "sales-ciphertext", StoreNow),
		} {
			if err := s.PutSecret(ctx, st); err != nil {
				t.Fatal(err)
			}
		}
		got, ok, err := s.Sealed(ctx, "acme", "finance", "pw")
		if err != nil || !ok || string(got) != "finance-ciphertext" {
			t.Fatalf("Sealed = %q, %v, %v", got, ok, err)
		}
		if _, ok, _ := s.Sealed(ctx, "globex", "finance", "pw"); ok {
			t.Error("another organisation read acme's secret")
		}
		listed, err := s.SecretsOf(ctx, "acme", "sales")
		if err != nil {
			t.Fatal(err)
		}
		if len(listed) != 1 || listed[0].Name != "pw" || listed[0].Sealed != nil {
			t.Errorf("SecretsOf(acme/sales) = %+v — one name, no ciphertext", listed)
		}
	})
}

// Watch tells a change from no change by the time, so two changes inside one
// second must still read back as two different times.
func TestASecretKeepsWhenItWasSetToTheNanosecond(t *testing.T) {
	both(t, func(t *testing.T, s *store.Store) {
		ctx := context.Background()
		first := StoreNow.Add(123456789 * time.Nanosecond)
		if err := s.PutSecret(ctx, sealedRow("acme", "finance", "pw", "a", first)); err != nil {
			t.Fatal(err)
		}
		second := first.Add(time.Millisecond)
		if err := s.PutSecret(ctx, sealedRow("acme", "finance", "pw", "b", second)); err != nil {
			t.Fatal(err)
		}
		listed, _ := s.SecretsOf(ctx, "acme", "finance")
		if len(listed) != 1 || !listed[0].UpdatedAt.Equal(second) {
			t.Fatalf("after two sets in one second: %+v", listed)
		}
		got, _, _ := s.Sealed(ctx, "acme", "finance", "pw")
		if string(got) != "b" {
			t.Errorf("the replacement did not replace: %q", got)
		}
	})
}

func TestDeletingASecretSaysWhetherThereWasOne(t *testing.T) {
	both(t, func(t *testing.T, s *store.Store) {
		ctx := context.Background()
		_ = s.PutSecret(ctx, sealedRow("acme", "finance", "pw", "x", StoreNow))

		// Another project's delete of the same name removes nothing.
		if gone, err := s.DeleteSecret(ctx, "acme", "sales", "pw"); err != nil || gone {
			t.Fatalf("acme/sales deleted acme/finance's secret: %v, %v", gone, err)
		}
		if gone, err := s.DeleteSecret(ctx, "acme", "finance", "pw"); err != nil || !gone {
			t.Fatalf("DeleteSecret = %v, %v", gone, err)
		}
		if gone, _ := s.DeleteSecret(ctx, "acme", "finance", "pw"); gone {
			t.Error("deleted the same secret twice")
		}
	})
}

// A reseal racing somebody's new password must not put the old one back.
func TestResealReplacesOnlyTheValueItRead(t *testing.T) {
	both(t, func(t *testing.T, s *store.Store) {
		ctx := context.Background()
		_ = s.PutSecret(ctx, sealedRow("acme", "finance", "pw", "old-key", StoreNow))
		all, err := s.AllSecrets(ctx)
		if err != nil || len(all) != 1 {
			t.Fatalf("AllSecrets = %+v, %v", all, err)
		}
		read := all[0]

		// Somebody sets a new password between the read and the reseal.
		_ = s.PutSecret(ctx, sealedRow("acme", "finance", "pw", "new-password", StoreNow))

		resealed := read
		resealed.Sealed = []byte("old-password-new-key")
		if ok, err := s.Reseal(ctx, resealed, read.Sealed); err != nil || ok {
			t.Fatalf("Reseal over a changed value = %v, %v", ok, err)
		}
		got, _, _ := s.Sealed(ctx, "acme", "finance", "pw")
		if !bytes.Equal(got, []byte("new-password")) {
			t.Errorf("the reseal put the old value back: %q", got)
		}
	})
}
