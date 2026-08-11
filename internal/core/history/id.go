package history

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// NewID returns an identifier that sorts by time and does not collide.
//
// Time first so a listing is chronological without an index on a timestamp,
// and random second because two schedules firing in the same second is the
// normal case at six on the first of the month. Not a UUID: this appears in
// URLs and support tickets, and something a person can read back over the
// phone is worth the eight characters.
func NewID(at time.Time) string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand does not fail in practice, and a run that could not be
		// identified must not be a run that did not happen.
		return fmt.Sprintf("run_%d_%s", at.UTC().Unix(), "00000000")
	}
	return fmt.Sprintf("run_%d_%s", at.UTC().Unix(), hex.EncodeToString(b[:]))
}
