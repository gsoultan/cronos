package identity

import "errors"

var (
	// ErrBadCredentials covers every reason a sign-in failed.
	//
	// One error, deliberately. "No such user" and "wrong password" told apart
	// is a way to enumerate who has an account, and the second is worth more
	// to an attacker than the first is to anybody honest.
	ErrBadCredentials = errors.New("identity: email or password is wrong")
	// ErrWeakPassword is raised where a password is set, not where it is used.
	ErrWeakPassword = errors.New("identity: password is too short")
	// ErrExists means that email already has an account here.
	ErrExists = errors.New("identity: already registered")
	// ErrNotFound means no such user.
	ErrNotFound = errors.New("identity: no such user")
)
