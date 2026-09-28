package secret

import "errors"

// ErrUnresolved means a definition names a secret nothing can supply.
//
// Raised at startup rather than at the connection that needed it: a
// deployment missing a password should fail to start, visibly, rather than
// start and fail on the first report somebody opens.
var ErrUnresolved = errors.New("secret: not resolved")

// ErrWeakKey means the key sealing stored secrets is too short to seal with.
// Raised at startup, like the signing key's.
var ErrWeakKey = errors.New("secret: secrets key is too short")

/*
ErrSealed means a stored value could not be opened.

One error for every reason — damaged, moved from another project's row, sealed
by a key this process was not given — because the one thing a caller does with
any of them is the same, and a reason per case would tell somebody probing the
table which of their edits got furthest.
*/
var ErrSealed = errors.New("secret: cannot open a stored value")
