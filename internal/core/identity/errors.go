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
	// ErrNoUser means there is no such person in this project. Deliberately
	// distinct from ErrBadCredentials: this one answers an administrator
	// asking about somebody, not a stranger asking whether an account exists.
	ErrNoUser = errors.New("identity: no such person")
	// ErrExists means that email already has an account here.
	ErrExists = errors.New("identity: already registered")
	// ErrNotFound means no such user.
	ErrNotFound = errors.New("identity: no such user")

	// ErrNoFactor means this account has no confirmed second factor.
	ErrNoFactor = errors.New("identity: no second factor")
	// ErrFactorExists means it already has one, and turning that off is its
	// own act rather than a side effect of starting another.
	ErrFactorExists = errors.New("identity: a second factor is already set up")
	/*
	   ErrBadCode is a code that is wrong, and is deliberately the same error
	   for a wrong TOTP code and a wrong recovery code.

	   Telling them apart says which of the two an attacker is closer to, and
	   the person typing knows perfectly well which box they used.
	*/
	ErrBadCode = errors.New("identity: that code is not right")
	// ErrCodeUsed means this code was already spent. A TOTP code is good for a
	// whole step, which is long enough to be read off somebody's screen.
	ErrCodeUsed = errors.New("identity: that code has already been used")
)
