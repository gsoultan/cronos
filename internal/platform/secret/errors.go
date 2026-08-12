package secret

import "errors"

// ErrUnresolved means a definition names a secret nothing can supply.
//
// Raised at startup rather than at the connection that needed it: a
// deployment missing a password should fail to start, visibly, rather than
// start and fail on the first report somebody opens.
var ErrUnresolved = errors.New("secret: not resolved")
