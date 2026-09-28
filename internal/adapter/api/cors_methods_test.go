package api_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gsoultan/cronos/internal/adapter/api"
)

/*
The preflight names every method a handler in this package compares against.

Read from the handlers themselves, because the list the other test checks was
typed by hand and the handlers were not asked. PUT is the third method to be
missed: /v1/policy and /v1/platform/org-roles took it for months, and a browser
on the portal's own origin refused both before sending them — while every test
here, which calls the handler directly, passed.
*/
func TestThePreflightNamesEveryMethodAHandlerAnswers(t *testing.T) {
	h := api.NewCORS([]string{"http://localhost:5174"}, http.NotFoundHandler())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/v1/policy", nil)
	req.Header.Set("Origin", "http://localhost:5174")
	h.ServeHTTP(rec, req)
	allowed := strings.Split(rec.Header().Get("Access-Control-Allow-Methods"), ", ")

	for method, where := range methodsAnswered(t) {
		found := false
		for _, a := range allowed {
			found = found || a == method
		}
		if !found {
			t.Errorf("%s is answered by %s and missing from the preflight %v, "+
				"so a browser will not send one", method, where, allowed)
		}
	}
}

// methodsAnswered is every http.MethodX the package's own code mentions, and
// the first file that does.
func methodsAnswered(t *testing.T) map[string]string {
	t.Helper()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	fset := token.NewFileSet()
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(fset, f, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "http" &&
				strings.HasPrefix(sel.Sel.Name, "Method") {
				method := strings.ToUpper(strings.TrimPrefix(sel.Sel.Name, "Method"))
				if _, seen := out[method]; !seen {
					out[method] = f
				}
			}
			return true
		})
	}
	if len(out) == 0 {
		t.Fatal("found no handler methods at all — is this running in the package directory?")
	}
	return out
}
