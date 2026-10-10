package api_test

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gsoultan/cronos/internal/adapter/api"
)

const (
	scriptPath = "/v1/embed/cronos-embed.js"
	hostOrigin = "https://app.acme.example"
)

func scriptServer(body []byte) http.Handler {
	return api.Routes(api.Deps{
		Log:     logger(&bytes.Buffer{}),
		Origins: []string{hostOrigin},
		Script:  body,
	})
}

func getScript(h http.Handler, method, origin string, header map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, scriptPath, nil)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// A host page loads the element with no token, and gets exactly the bytes
// this binary was built with, labelled so a browser will run them as nothing
// but a script.
func TestAHostPageLoadsTheElementFromTheServer(t *testing.T) {
	body := []byte(`export const TAG = 'cronos-report'`)
	rec := getScript(scriptServer(body), http.MethodGet, hostOrigin, nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", rec.Code)
	}
	if !bytes.Equal(rec.Body.Bytes(), body) {
		t.Fatalf("served %q, want the built bundle", rec.Body.String())
	}
	for header, want := range map[string]string{
		"Content-Type":           "text/javascript; charset=utf-8",
		"X-Content-Type-Options": "nosniff",
		"Cache-Control":          "no-cache",
		// A module script is fetched in CORS mode: without this the browser
		// refuses to run it however good the response.
		"Access-Control-Allow-Origin": hostOrigin,
	} {
		if got := rec.Header().Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
}

// The script goes through the same allow-list as the API. A wildcard here
// would let any page on the internet run the element against this server —
// harmless without a token, and still not something to hand out.
func TestAnUnlistedOriginCannotLoadTheElement(t *testing.T) {
	rec := getScript(scriptServer([]byte("x")), http.MethodGet, "https://not-ours.example", nil)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("an origin nobody listed may load the element: %q", got)
	}
}

// Revalidated rather than cached for a time, so an upgraded server is never
// paired with yesterday's element — and an unchanged one costs a 304.
func TestAnUnchangedElementIsNotSentTwice(t *testing.T) {
	h := scriptServer([]byte("export {}"))
	first := getScript(h, http.MethodGet, hostOrigin, nil)
	etag := first.Header().Get("ETag")
	if etag == "" {
		t.Fatal("no ETag, so every page load downloads the element again")
	}

	again := getScript(h, http.MethodGet, hostOrigin, map[string]string{"If-None-Match": etag})
	if again.Code != http.StatusNotModified {
		t.Fatalf("status %d for an unchanged element, want 304", again.Code)
	}

	changed := getScript(scriptServer([]byte("export const v = 2")), http.MethodGet, hostOrigin,
		map[string]string{"If-None-Match": etag})
	if changed.Code != http.StatusOK {
		t.Fatalf("status %d after an upgrade, want 200 with the new element", changed.Code)
	}
}

// Sent compressed to a browser that asks — a third of the bytes — under an
// ETag of its own, and plain to one that refuses with q=0.
func TestTheElementIsSentCompressedToABrowserThatAsks(t *testing.T) {
	body := bytes.Repeat([]byte("export const cronos = 'report';\n"), 200)
	h := scriptServer(body)

	packed := getScript(h, http.MethodGet, hostOrigin, map[string]string{"Accept-Encoding": "br, gzip"})
	if packed.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("Content-Encoding %q, want gzip", packed.Header().Get("Content-Encoding"))
	}
	r, err := gzip.NewReader(packed.Body)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := io.ReadAll(r); !bytes.Equal(got, body) {
		t.Fatal("the compressed element does not unpack to the built one")
	}

	plain := getScript(h, http.MethodGet, hostOrigin, map[string]string{"Accept-Encoding": "gzip;q=0"})
	if plain.Header().Get("Content-Encoding") != "" || !bytes.Equal(plain.Body.Bytes(), body) {
		t.Fatal("gzip;q=0 refuses gzip, and was sent it anyway")
	}
	if packed.Header().Get("ETag") == plain.Header().Get("ETag") {
		t.Fatal("one ETag for two different bodies, so a cache can serve either for the other")
	}
}

func TestTheElementIsOnlyRead(t *testing.T) {
	h := scriptServer([]byte("export {}"))
	if rec := getScript(h, http.MethodHead, hostOrigin, nil); rec.Code != http.StatusOK || rec.Body.Len() != 0 {
		t.Errorf("HEAD: status %d with %d bytes, want 200 and no body", rec.Code, rec.Body.Len())
	}
	rec := getScript(h, http.MethodPost, hostOrigin, nil)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST: status %d, want 405", rec.Code)
	}
}

// A binary built without the bundle mounts nothing, rather than a route that
// answers an empty script a page would run without complaint.
func TestNoBundleMeansNoRoute(t *testing.T) {
	if rec := getScript(scriptServer(nil), http.MethodGet, hostOrigin, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("status %d without a bundle, want 404", rec.Code)
	}
}
