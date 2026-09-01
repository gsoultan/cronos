package ee

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gsoultan/cronos/ee/audit"
	"github.com/gsoultan/cronos/ee/auth/oidc"
	"github.com/gsoultan/cronos/internal/extension"
)

/*
init fills the seams before anything reads one.

The audit sink unconditionally, because a build that has it should use it. The
sign-in flow only where a deployment configured one: an enterprise build with
no identity provider is a perfectly ordinary deployment that signs people in
with passwords, and registering a flow pointed at nothing would put a button on
the sign-in page that leads to an error.
*/
func init() {
	extension.RegisterAuditSink(audit.NewJSONL(os.Stderr))

	if os.Getenv("CRONOS_OIDC_ISSUER") == "" {
		return
	}

	// Discovery reaches the identity provider, so it is bounded. A provider
	// that does not answer in ten seconds at startup is one this deployment
	// cannot sign anybody in through, and saying so now beats saying it to
	// the first person who tries.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	provider, err := oidc.New(ctx, oidc.Config{
		Issuer:       os.Getenv("CRONOS_OIDC_ISSUER"),
		ClientID:     os.Getenv("CRONOS_OIDC_CLIENT_ID"),
		ClientSecret: os.Getenv("CRONOS_OIDC_CLIENT_SECRET"),
		RedirectURL:  os.Getenv("CRONOS_OIDC_REDIRECT_URL"),
		// Where the provider sends somebody after ending its own session.
		// Registered with them too; omitted rather than guessed, because an
		// unregistered one turns every sign-out into their error page.
		PostLogoutURL: os.Getenv("CRONOS_OIDC_POST_LOGOUT_URL"),
		Scopes:        split(os.Getenv("CRONOS_OIDC_SCOPES")),

		Org:            os.Getenv("CRONOS_ORG"),
		Project:        os.Getenv("CRONOS_PROJECT"),
		DefaultRole:    envOr("CRONOS_OIDC_DEFAULT_ROLE", "viewer"),
		Roles:          pairs(os.Getenv("CRONOS_OIDC_ROLES")),
		AllowedDomains: split(os.Getenv("CRONOS_OIDC_DOMAINS")),
	})
	if err != nil {
		/*
		   Fatal, and deliberately so.

		   A deployment that set an issuer asked for single sign-on. Starting
		   without it is a server that quietly falls back to passwords — which
		   is the wrong thing on the morning somebody removes the passwords,
		   and is invisible until then because everything else works.
		*/
		fmt.Fprintf(os.Stderr, "cronos: single sign-on is configured and could not start: %v\n", err)
		os.Exit(1)
	}
	extension.RegisterSignInFlow(provider)
}

// split reads a comma-separated list, ignoring the spaces people leave.
func split(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// pairs reads `group=role,group=role`, which is how a role mapping is written
// in an environment variable without inventing a file format for four lines.
func pairs(raw string) map[string]string {
	out := map[string]string{}
	for _, pair := range split(raw) {
		group, role, ok := strings.Cut(pair, "=")
		if !ok {
			continue
		}
		out[strings.TrimSpace(group)] = strings.TrimSpace(role)
	}
	return out
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
