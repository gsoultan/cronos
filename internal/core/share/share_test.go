package share_test

import (
	"testing"
	"time"

	"github.com/gsoultan/cronos/internal/core/share"
)

/*
Whether a link still opens, and the word a list shows for it.

Two methods, and the interesting part of both is the ordering. A share can be
expired and withdrawn at once, and the two answers disagree about which to
report: Live only cares that it does not open, State has to pick a word. It
picks "revoked", because that is the deliberate one — somebody did it, and a
list that shows "expired" hides the fact that the withdrawal happened.
*/

var now = time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)

func at(d time.Duration) *time.Time {
	t := now.Add(d)
	return &t
}

func TestALinkWithNoExpiryOpensForEver(t *testing.T) {
	s := share.Share{ID: "shr_1"}

	if !s.Live(now) {
		t.Error("a share with no expiry and no revocation does not open")
	}
	if got := s.State(now); got != "live" {
		t.Errorf("State is %q, want live", got)
	}
}

func TestExpiryIsTheInstantItStops(t *testing.T) {
	for _, c := range []struct {
		name    string
		expires *time.Time
		live    bool
		state   string
	}{
		{"an hour left", at(time.Hour), true, "live"},
		// Exactly at the expiry is expired: Live uses now.Before, so the
		// instant named is the first one it does not open.
		{"exactly now", at(0), false, "expired"},
		{"an hour ago", at(-time.Hour), false, "expired"},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := share.Share{ID: "shr_1", ExpiresAt: c.expires}

			if got := s.Live(now); got != c.live {
				t.Errorf("Live is %v, want %v", got, c.live)
			}
			if got := s.State(now); got != c.state {
				t.Errorf("State is %q, want %q", got, c.state)
			}
		})
	}
}

func TestAWithdrawnLinkDoesNotOpenWhateverItsExpiry(t *testing.T) {
	for _, c := range []struct {
		name    string
		expires *time.Time
	}{
		{"no expiry", nil},
		{"still in date", at(time.Hour)},
		{"already expired", at(-time.Hour)},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := share.Share{ID: "shr_1", ExpiresAt: c.expires, RevokedAt: at(-time.Minute)}

			if s.Live(now) {
				t.Error("a withdrawn share opened")
			}
			// Revoked wins over expired. Somebody withdrew it, and a list
			// reporting "expired" hides that they did.
			if got := s.State(now); got != "revoked" {
				t.Errorf("State is %q, want revoked", got)
			}
		})
	}
}
