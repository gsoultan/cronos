package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/gsoultan/cronos/internal/core/query"
)

// errorBody is what a failure looks like on the wire.
//
// One field, always the same shape, safe to show a person. The embed component
// puts it straight in front of an end user, so it is written for them and not
// for us.
type errorBody struct {
	Error string `json:"error"`
}

func send(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	// A report is per-caller and per-token. Any cache between here and the
	// browser holding one is a cross-tenant leak with extra steps.
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func fail(w http.ResponseWriter, status int, msg string) {
	send(w, status, errorBody{Error: msg})
}

// plainly strips the sentinel a caller has no use for.
//
// "query: bad argument: \"from\" wants a date as YYYY-MM-DD" is a sentence
// with our package name bolted to the front. The part after it was written for
// the person who typed the value.
func plainly(err error) string {
	msg := err.Error()
	if _, after, ok := strings.Cut(msg, query.ErrBadArgument.Error()+": "); ok {
		return after
	}
	return msg
}

// isCallerError reports whether the failure is one the caller can fix.
//
// Only the query package's argument errors qualify, and that is deliberate:
// they are written about the request rather than about the schema. Everything
// else — a template that will not compile, a driver that refused — describes
// the definition or the database, and neither is the caller's business.
func isCallerError(err error) bool {
	return errors.Is(err, query.ErrBadArgument)
}
