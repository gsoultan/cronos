package api_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gsoultan/cronos/internal/adapter/api"
	"github.com/gsoultan/cronos/internal/platform/token"
)

func signerOf(t *testing.T, key string) *token.Signer {
	t.Helper()
	s, err := token.NewSigner([]byte(key))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

/*
askAs presents a token pinned to one report and asks for another.

That is refused either way, and how it is refused says whether the token
verified: 403 means it did and the pin was honoured, 401 means it never got
that far. It needs no project and no warehouse to tell the two apart.
*/
func askAs(h http.Handler, tok string) int {
	req := httptest.NewRequest(http.MethodPost, "/v1/embed/reports/another", bytes.NewReader([]byte("{}")))
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

func pinned(t *testing.T, s *token.Signer, audience string) string {
	t.Helper()
	tok, err := s.Mint(token.Claims{
		Audience: audience, Org: "acme", Project: "finance", Subject: "host",
		Role: "admin", OrgRole: "owner", Platform: true, Report: "billing",
	}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

/*
The key a host application holds opens embedded reports and nothing else.

Root cause of what this replaces: one key signed embed tokens and portal
sessions, and a host's backend held it — so the audience that separates the
two was a claim the host wrote for itself.
*/
func TestAHostsKeyOpensReportsAndNotThePortal(t *testing.T) {
	portal := signerOf(t, "the-key-that-never-leaves-cronos-xx")
	host := signerOf(t, "the-key-a-host-application-is-given")

	h := api.Routes(api.Deps{
		Signer: portal, EmbedSigner: host.For(token.Embed),
		Log: logger(&bytes.Buffer{}),
	})

	if got := askAs(h, pinned(t, host, token.Embed)); got != http.StatusForbidden {
		t.Fatalf("status %d for the host's own embed token, want 403: verified, and held to its pin", got)
	}

	// A portal session the host signed for itself, claiming everything.
	forged := pinned(t, host, token.Portal)
	req := httptest.NewRequest(http.MethodGet, "/v1/catalog", nil)
	req.Header.Set("Authorization", "Bearer "+forged)
	if pr, ok := api.NewAuthor(portal, nil).Principal(req); ok {
		t.Fatalf("the portal accepted a session signed with the embed key, as %+v", pr)
	}
	if got := askAs(h, forged); got != http.StatusUnauthorized {
		t.Fatalf("status %d for a portal token at the embed route, want 401", got)
	}

	// And the signing key no longer opens a report: with a key of their own,
	// embed tokens are verified with that one alone.
	if got := askAs(h, pinned(t, portal, token.Embed)); got != http.StatusUnauthorized {
		t.Fatalf("status %d for an embed token signed with the signing key, want 401", got)
	}
}

// A deployment that has not set an embed key works as it did: embed tokens are
// signed with the signing key. Startup warns; nothing breaks on upgrade.
func TestWithoutAnEmbedKeyTheSigningKeyStillOpensReports(t *testing.T) {
	signer := signerOf(t, "the-key-that-never-leaves-cronos-xx")
	h := api.Routes(api.Deps{Signer: signer, Log: logger(&bytes.Buffer{})})

	if got := askAs(h, pinned(t, signer, token.Embed)); got != http.StatusForbidden {
		t.Fatalf("status %d, want 403: verified, and held to its pin", got)
	}
}
