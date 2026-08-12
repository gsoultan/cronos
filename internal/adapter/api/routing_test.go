package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Go's ServeMux prefers the literal segment over the wildcard, so
// /v1/people/invitations is the invitation list rather than a person called
// "invitations". Asserted because the two patterns overlap and the resolution
// is a property of the router, not of this code.
func TestInvitationRoutesBeatThePersonWildcard(t *testing.T) {
	mux := http.NewServeMux()
	hit := ""
	mux.HandleFunc("/v1/people/{id}", func(_ http.ResponseWriter, r *http.Request) {
		hit = "person:" + r.PathValue("id")
	})
	mux.HandleFunc("/v1/people/invitations", func(http.ResponseWriter, *http.Request) {
		hit = "invitations"
	})
	mux.HandleFunc("/v1/people/invitations/{id}", func(_ http.ResponseWriter, r *http.Request) {
		hit = "invitation:" + r.PathValue("id")
	})

	for path, want := range map[string]string{
		"/v1/people/usr_1":             "person:usr_1",
		"/v1/people/invitations":       "invitations",
		"/v1/people/invitations/inv_1": "invitation:inv_1",
	} {
		hit = ""
		mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
		if hit != want {
			t.Fatalf("%s reached %q, wanted %q", path, hit, want)
		}
	}
}
