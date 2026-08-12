package api

import "errors"

// ErrNoProject means this process does not serve the project the caller acts
// in.
//
// Answered the same way whichever it is: a project this deployment was never
// told about, and one it serves but the caller does not belong to, are the
// same 404. Telling them apart would let somebody enumerate which customers a
// deployment holds.
var ErrNoProject = errors.New("api: no such project here")
