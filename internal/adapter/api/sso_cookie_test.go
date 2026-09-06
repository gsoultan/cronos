package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gsoultan/cronos/internal/adapter/api"
	"github.com/gsoultan/cronos/internal/extension"
)

/*
The state cookie has to be Secure where the deployment is served over TLS, and
r.TLS cannot answer that.

docs/deploying.md tells an operator to terminate TLS in front and set
CRONOS_BEHIND_PROXY=1. In that topology every request reaches this process over
plaintext HTTP, so r.TLS is nil and the cookie went out without Secure — in the
one arrangement the documentation recommends. A state cookie a browser will
send over http:// is a sign-in an on-path attacker can read and replay against
the callback.

BehindProxy is what the deployment already tells the server about its own
front, and the limiter has read it since it existed. These are the same fact.
*/

func startOver(t *testing.T, h *api.SSO, tls bool) *http.Cookie {
	t.Helper()

	r := httptest.NewRequest(http.MethodGet, "/v1/auth/sso/start", nil)
	if !tls {
		// httptest.NewRequest leaves TLS nil for an http:// target, which is
		// what a request forwarded by a terminating proxy looks like.
		r.TLS = nil
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	for _, c := range w.Result().Cookies() {
		if c.Name == "cronos_sso" {
			return c
		}
	}
	t.Fatalf("sign-in set no state cookie (%d)", w.Code)
	return nil
}

func TestStateCookieIsSecureBehindATerminatingProxy(t *testing.T) {
	h, _ := handler(t, &fakeFlow{})
	h.BehindProxy(true)

	if got := startOver(t, h, false); !got.Secure {
		t.Fatal("state cookie went out without Secure behind a terminating " +
			"proxy: a browser will send it over http://")
	}
}

// Clearing it has to match. A Set-Cookie whose attributes differ from the one
// it is replacing is a second cookie in some browsers, and the spent state
// stays in the jar.
func TestClearedStateCookieIsSecureBehindATerminatingProxy(t *testing.T) {
	h, _ := handler(t, &fakeFlow{})
	h.BehindProxy(true)

	back := httptest.NewRequest(http.MethodGet, "/v1/auth/sso/callback?code=x", nil)
	back.AddCookie(startOver(t, h, false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, back)

	for _, c := range w.Result().Cookies() {
		if c.Name == "cronos_sso" && c.MaxAge < 0 {
			if !c.Secure {
				t.Fatal("the cookie clearing the spent state was not Secure, " +
					"so it does not replace the one that is")
			}
			return
		}
	}
	t.Fatalf("callback cleared no state cookie: %s", w.Result().Header.Get("Location"))
}

// And a plain http:// deployment still works. Secure on a cookie no browser
// will send back over http:// is a sign-in that cannot complete, which is how
// this gets reverted by whoever runs cronos on a laptop.
func TestStateCookieIsNotSecureOnAPlainDeployment(t *testing.T) {
	h, _ := handler(t, &fakeFlow{})

	if got := startOver(t, h, false); got.Secure {
		t.Fatal("state cookie was Secure with no TLS and no proxy in front: " +
			"the browser will not send it back and the sign-in cannot finish")
	}
}

var _ extension.SignInFlow = (*fakeFlow)(nil)
