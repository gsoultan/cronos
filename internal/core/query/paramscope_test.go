package query_test

import (
	"strings"
	"testing"

	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/principal"
	"github.com/gsoultan/cronos/internal/core/query"
)

/*
A row scope must read the token's scope, not the request's params.

check.go refused a predicate with no template hole at all, and its message said
why: "reads no .scope value, so it restricts every caller identically". The
condition tested `len(refs) == 0`, so *any* hole satisfied it — including a
`.params` hole.

That gap is the whole product's claim inverted. `.params` is merged from the
request body wherever the host did not pin the name, so a predicate reading
`{{ .params.customer_id }}` let the browser choose the value it is confined to.
An end user sends `{"params":{"customer_id":"c-2"}}` and reads another
customer's rows, with a correctly minted token that says `c-1`.

It is a shape an author is pushed towards rather than an exotic one:
docs/tenancy.md tells them a dataset read by a schedule must not carry a
`.scope` predicate and to scope it with a parameter instead.
*/

func scoped(predicate string) definition.Dataset {
	return definition.Dataset{
		Name:    "invoices",
		Sources: []definition.SourceRef{{Ref: "warehouse"}},
		Query:   "SELECT customer_id, total FROM invoices",
		Params: []definition.Param{
			{Name: "customer_id", Type: definition.String},
		},
		Fields: []definition.Field{
			{Name: "customer_id", Type: "string", Role: definition.Dimension},
			{Name: "total", Type: "decimal", Role: definition.Measure, Aggregate: "sum"},
		},
		RowLevelSecurity: []definition.RowScope{{Predicate: predicate}},
	}
}

// Check is what publishing runs, so this is the gate that should have refused
// the definition before it was ever stored.
func TestARowScopeMustReadTheTokensScope(t *testing.T) {
	err := query.Check(scoped("customer_id = {{ .params.customer_id }}"))
	if err == nil {
		t.Fatal("a row scope confining on a request parameter was accepted — " +
			"the caller chooses the value it is confined to")
	}
	// The message already claimed this was the rule; now it is.
	if !strings.Contains(err.Error(), ".scope") {
		t.Errorf("the refusal does not say what is missing: %v", err)
	}
}

// The correct shape still passes, or every scoped dataset stops publishing.
func TestARowScopeReadingTheScopeIsFine(t *testing.T) {
	if err := query.Check(scoped("customer_id = {{ .scope.customer_id }}")); err != nil {
		t.Fatalf("a correct row scope was refused: %v", err)
	}
	// And one that reads both: a scope value and a declared param.
	if err := query.Check(scoped(
		"customer_id = {{ .scope.customer_id }} AND status = {{ .params.customer_id }}")); err != nil {
		t.Fatalf("a row scope reading scope and a param was refused: %v", err)
	}
}

/*
And a definition already in the store cannot exploit it either.

Check runs at publish. A deployment that stored such a dataset before this was
fixed would keep serving it, because nothing re-validates on read — so the
compiler refuses too, and returns the fail-closed predicate rather than one the
caller filled in.
*/
func TestAStoredParamOnlyScopeMatchesNoRows(t *testing.T) {
	customer := principal.Principal{
		Subject: "u1", OrgID: "acme", ProjectID: "finance",
		ProjectRole: principal.ProjectViewer,
		Scope:       map[string]string{"customer_id": "c-1"},
	}

	// The browser supplies the value the predicate reads.
	plan, _, err := query.NewBuilder(query.SQLite{}).BuildWith(
		scoped("customer_id = {{ .params.customer_id }}"),
		map[string]any{"customer_id": "c-2"}, query.Filters{}, customer)
	if err != nil {
		// Refusing to compile is an equally good answer.
		return
	}

	if strings.Contains(plan.SQL(), "FALSE") {
		return // fail-closed, which is the point
	}
	for _, arg := range plan.Args() {
		if s, ok := arg.(string); ok && s == "c-2" {
			t.Fatalf("the caller's own value was bound into a row scope: %s %v",
				plan.SQL(), plan.Args())
		}
	}
}
