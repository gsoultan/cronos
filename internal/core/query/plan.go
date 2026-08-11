package query

// Plan is a statement and its bind arguments, ready to execute.
//
// The fields are unexported and only Build sets them. That is the enforcement
// mechanism for row scope: an executor takes a Plan, a Plan can only come from
// Build, and Build always wraps. There is no way to hand an executor a
// statement that skipped the wrapper, so "did this path apply RLS?" is not a
// question anyone has to ask of a code path again.
type Plan struct {
	sql  string
	args []any
}

// SQL is the statement to execute.
func (p Plan) SQL() string { return p.sql }

// Args are its bind arguments, in placeholder order.
//
// Returned as-is rather than copied: this is on the hot path of every query,
// and a defensive copy per execution buys nothing an internal caller could not
// achieve by mutating the Plan it was handed anyway.
func (p Plan) Args() []any { return p.args }

// Empty reports whether the plan is the zero value — something an executor
// should refuse rather than send.
func (p Plan) Empty() bool { return p.sql == "" }
