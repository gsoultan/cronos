package api_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gsoultan/cronos/internal/adapter/api"
	"github.com/gsoultan/cronos/internal/app/publish"
	"github.com/gsoultan/cronos/internal/core/principal"
	"github.com/gsoultan/cronos/internal/platform/token"
)

// oneDocument is a definition store holding a single datasource.
type oneDocument struct{ raw []byte }

func (o oneDocument) Put(context.Context, principal.Principal, string, string, []byte) (string, error) {
	return "", nil
}
func (o oneDocument) Get(_ context.Context, pr principal.Principal, kind, name string) ([]byte, error) {
	if pr.OrgID != "acme" || kind != "DataSource" || name != "warehouse" {
		return nil, publish.ErrNotFound
	}
	return o.raw, nil
}
func (o oneDocument) List(context.Context, principal.Principal) ([]publish.Entry, error) {
	return []publish.Entry{{Kind: "DataSource", Name: "warehouse"}}, nil
}
func (o oneDocument) Delete(context.Context, principal.Principal, string, string) error { return nil }

/*
A datasource's document is for the people who may change it.

It may hold a password written inline — allowed, and what a first deployment
does — and it names the host and the account every report reads with. A viewer
reads reports; nothing a viewer does needs the connection string, and the
definitions route served it to them. Every kind spelling is tried, because the
check has to hold for whichever one the store accepts.
*/
func TestAViewerCannotReadADatasourcesDocument(t *testing.T) {
	signer, err := token.NewSigner([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	doc := []byte("kind: DataSource\nmetadata: {name: warehouse}\n" +
		"spec: {driver: postgres, dsn: 'postgres://report:hunter2@db/warehouse'}\n")
	h := api.Routes(api.Deps{
		Signer: signer, Log: logger(&bytes.Buffer{}),
		Publish: nobodyPublishes{}, Store: oneDocument{raw: doc},
	})
	read := func(role, kind string) *httptest.ResponseRecorder {
		tok, err := signer.Mint(token.Claims{Audience: token.Portal, Role: role,
			Org: "acme", Project: "finance", Subject: "usr_1"}, time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(http.MethodGet, "/v1/definitions/"+kind+"/warehouse", nil)
		r.Header.Set("Authorization", "Bearer "+tok)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}

	for _, kind := range []string{"DataSource", "datasource", "datasources", "DATASOURCE"} {
		if w := read("viewer", kind); bytes.Contains(w.Body.Bytes(), []byte("hunter2")) {
			t.Errorf("a viewer read the %s document: %d %s", kind, w.Code, w.Body)
		}
	}
	if w := read("editor", "DataSource"); w.Code != http.StatusOK {
		t.Errorf("an editor could not read the datasource they edit: %d %s", w.Code, w.Body)
	}
}
