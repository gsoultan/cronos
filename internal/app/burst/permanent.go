package burst

import "errors"

// Permanent marks a failure that retrying cannot fix.
//
// The distinction matters more than it looks. A relay that is down and an
// address that does not exist both fail; retrying the first is the point of
// having retries and retrying the second is three more rejections, three more
// delays, and a burst that takes an hour longer to reach the same answer.
//
// Channels wrap their own errors: only they know which is which.
type Permanent struct{ Err error }

func (p Permanent) Error() string { return p.Err.Error() }
func (p Permanent) Unwrap() error { return p.Err }

// Fatal wraps err as something not worth retrying.
func Fatal(err error) error { return Permanent{Err: err} }

// permanent reports whether retrying err is pointless.
func permanent(err error) bool {
	var p Permanent
	return errors.As(err, &p)
}
