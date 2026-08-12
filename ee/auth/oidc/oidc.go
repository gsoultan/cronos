/*
Package oidc signs somebody in through their own identity provider.

Authorization code with PKCE, which is what every current identity provider
expects from a client that cannot keep a secret perfectly — and cronos is a
server that can, so the secret is used as well. PKCE on top costs one hash and
closes the case where an authorization code is intercepted before the exchange.

What this deliberately does not do: read a token from a request. That is the
other seam, for a deployment whose proxy already authenticated somebody, and
mixing the two would put a second way in beside the one an administrator
configured.

Licensed under ee/LICENSE, not the repository's BSL.
*/
package oidc

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gsoultan/cronos/internal/extension"
)

// Config is what a deployment sets to point cronos at its directory.
type Config struct {
	// Issuer is the identity provider's URL. Discovery hangs off it, so this
	// is the only address anybody has to know.
	Issuer       string
	ClientID     string
	ClientSecret string
	// RedirectURL is where the provider sends the browser back, and must be
	// registered with them exactly. Ours is /v1/auth/sso/callback.
	RedirectURL string
	// Scopes beyond openid. `email` and `profile` are the usual two; `groups`
	// is how most providers are asked for role information.
	Scopes []string

	// Where people land. A deployment serves one project, so these are its
	// answer unless a group says otherwise.
	Org, Project string
	// DefaultRole is what somebody gets with no matching group. Viewer,
	// because the failure of a role mapping should be too little access rather
	// than too much.
	DefaultRole string
	// Roles maps a group the provider asserts to a role here. First match in
	// the order admin, editor, viewer — a person in two groups gets the
	// stronger, which is what somebody who put them in both meant.
	Roles map[string]string
	// AllowedDomains restricts who may sign in at all. Empty permits anybody
	// the provider vouches for, which is right when the provider is the
	// company's own and wrong when it is a public one.
	AllowedDomains []string
}

// Provider implements extension.SignInFlow.
type Provider struct {
	cfg      Config
	client   *http.Client
	metadata metadata
	keys     *keySet
	now      func() time.Time
}

// metadata is the part of the discovery document this needs.
type metadata struct {
	Issuer        string `json:"issuer"`
	Authorization string `json:"authorization_endpoint"`
	Token         string `json:"token_endpoint"`
	JWKS          string `json:"jwks_uri"`
}

/*
New discovers the provider and returns a flow.

At startup, and fatal if it fails: a deployment configured for single sign-on
that starts without it is one where everybody is quietly sent to a password
form, and somebody notices on the morning the passwords have been removed.
*/
func New(ctx context.Context, cfg Config) (*Provider, error) {
	switch {
	case cfg.Issuer == "":
		return nil, fmt.Errorf("oidc: no issuer")
	case cfg.ClientID == "" || cfg.ClientSecret == "":
		return nil, fmt.Errorf("oidc: a client id and secret are required")
	case cfg.RedirectURL == "":
		return nil, fmt.Errorf("oidc: no redirect url — the provider must be told where to send people back")
	}

	client := &http.Client{Timeout: 10 * time.Second}
	found, err := discover(ctx, client, cfg.Issuer)
	if err != nil {
		return nil, err
	}

	return &Provider{
		cfg: cfg, client: client, metadata: found,
		keys: newKeySet(client, found.JWKS),
		now:  time.Now,
	}, nil
}

// Name identifies the provider in a log line and in an account id.
func (p *Provider) Name() string { return "oidc" }

/*
Start sends the browser to the identity provider.

The nonce and the PKCE verifier are minted here and kept in the state the core
holds against a cookie. Neither travels anywhere a page can read it: the nonce
comes back inside a signed token and is compared, and the verifier is sent only
in the back-channel exchange.
*/
func (p *Provider) Start(_ context.Context, returning string) (string, extension.State, error) {
	nonce, err := random()
	if err != nil {
		return "", extension.State{}, err
	}
	verifier, err := random()
	if err != nil {
		return "", extension.State{}, err
	}
	id, err := random()
	if err != nil {
		return "", extension.State{}, err
	}

	challenge := sha256.Sum256([]byte(verifier))

	query := url.Values{
		"response_type": {"code"},
		"client_id":     {p.cfg.ClientID},
		"redirect_uri":  {p.cfg.RedirectURL},
		"scope":         {strings.Join(p.scopes(), " ")},
		// The state parameter is the cookie's id. The provider hands it back
		// and it must match what this browser was given, which is what stops
		// somebody else's completed sign-in being finished in this browser.
		"state":                 {id},
		"nonce":                 {nonce},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(challenge[:])},
		"code_challenge_method": {"S256"},
	}

	return p.metadata.Authorization + "?" + query.Encode(), extension.State{
		ID: id,
		Data: map[string]string{
			"nonce": nonce, "verifier": verifier, "returning": returning,
		},
		// Ten minutes. Long enough for somebody to find their phone and
		// approve a push, short enough that an abandoned sign-in is not still
		// completable after lunch.
		Expires: p.now().Add(10 * time.Minute),
	}, nil
}

/*
Complete exchanges the code and reads who came back.

Everything is checked before anything is believed: the state matches this
browser's cookie, the token exchange happens over the back channel with the
client secret and the PKCE verifier, the id token's signature is verified
against the provider's published keys, and its issuer, audience, expiry and
nonce are all compared to what was expected. A single one of those skipped is a
sign-in somebody else can forge.
*/
func (p *Provider) Complete(ctx context.Context, r *http.Request,
	state extension.State) (extension.Identity, error) {

	if returned := r.URL.Query().Get("state"); returned != state.ID {
		// The provider's answer belongs to a different sign-in than this
		// browser started.
		return extension.Identity{}, fmt.Errorf("oidc: the state did not match")
	}
	if failed := r.URL.Query().Get("error"); failed != "" {
		// The provider's own refusal, which is usually the useful one:
		// access_denied means somebody said no to a consent screen.
		return extension.Identity{}, fmt.Errorf("oidc: %s", failed)
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		return extension.Identity{}, fmt.Errorf("oidc: no code came back")
	}

	raw, err := p.exchange(ctx, code, state.Data["verifier"])
	if err != nil {
		return extension.Identity{}, err
	}

	claims, err := p.verify(ctx, raw, state.Data["nonce"])
	if err != nil {
		return extension.Identity{}, err
	}
	if err := p.permitted(claims.Email); err != nil {
		return extension.Identity{}, err
	}

	return extension.Identity{
		Subject:   claims.Subject,
		Email:     claims.Email,
		Name:      firstOf(claims.Name, claims.PreferredUsername, claims.Email),
		Groups:    claims.Groups,
		Org:       p.cfg.Org,
		Project:   p.cfg.Project,
		Role:      p.role(claims.Groups),
		Returning: state.Data["returning"],
	}, nil
}

// exchange trades the code for tokens, over the back channel.
func (p *Provider) exchange(ctx context.Context, code, verifier string) (string, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {p.cfg.RedirectURL},
		"client_id":     {p.cfg.ClientID},
		"code_verifier": {verifier},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.metadata.Token,
		strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// In the header rather than the body: every provider accepts basic auth
	// here and some refuse a secret in the form.
	req.SetBasicAuth(url.QueryEscape(p.cfg.ClientID), url.QueryEscape(p.cfg.ClientSecret))

	res, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("oidc: could not reach the token endpoint: %w", err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if res.StatusCode != http.StatusOK {
		// The provider's own words, which name the misconfiguration: a
		// redirect_uri that does not match what was registered is the usual
		// one and is impossible to guess from "unauthorized".
		return "", fmt.Errorf("oidc: the token endpoint refused: %s", strings.TrimSpace(string(body)))
	}

	var answer struct {
		IDToken string `json:"id_token"`
	}
	if err := json.Unmarshal(body, &answer); err != nil {
		return "", fmt.Errorf("oidc: could not read the token response: %w", err)
	}
	if answer.IDToken == "" {
		return "", fmt.Errorf("oidc: no id token came back — is `openid` among the scopes?")
	}
	return answer.IDToken, nil
}

/*
role maps the groups the provider asserts to a role here.

Strongest wins. Somebody in both `analytics-admins` and `analytics-viewers` was
put in both by an administrator who meant the first — and the alternative,
taking whichever the provider happened to list first, makes a person's access
depend on a directory's iteration order.
*/
func (p *Provider) role(groups []string) string {
	best := ""
	rank := map[string]int{"viewer": 1, "editor": 2, "admin": 3}

	for _, group := range groups {
		mapped, ok := p.cfg.Roles[group]
		if !ok {
			continue
		}
		if rank[mapped] > rank[best] {
			best = mapped
		}
	}
	if best == "" {
		return firstOf(p.cfg.DefaultRole, "viewer")
	}
	return best
}

/*
permitted refuses an address outside the domains a deployment allows.

Empty allows anybody the provider vouches for, which is right when the provider
is the company's own directory and wrong when it is a public one: a Google
client with no domain restriction is a sign-in button for the entire internet.
*/
func (p *Provider) permitted(email string) error {
	if len(p.cfg.AllowedDomains) == 0 {
		return nil
	}
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return fmt.Errorf("oidc: the provider returned no email to check")
	}
	domain := strings.ToLower(email[at+1:])
	for _, allowed := range p.cfg.AllowedDomains {
		if domain == strings.ToLower(strings.TrimSpace(allowed)) {
			return nil
		}
	}
	return fmt.Errorf("oidc: %s is not a domain this deployment admits", domain)
}

func (p *Provider) scopes() []string {
	// openid always, because without it the provider returns no id token and
	// the failure is a confusing one.
	out := []string{"openid"}
	for _, s := range p.cfg.Scopes {
		if s != "openid" {
			out = append(out, s)
		}
	}
	if len(out) == 1 {
		return append(out, "email", "profile")
	}
	return out
}

// discover reads the provider's well-known document.
func discover(ctx context.Context, client *http.Client, issuer string) (metadata, error) {
	where := strings.TrimSuffix(issuer, "/") + "/.well-known/openid-configuration"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, where, nil)
	if err != nil {
		return metadata{}, err
	}
	res, err := client.Do(req)
	if err != nil {
		return metadata{}, fmt.Errorf("oidc: could not reach %s: %w", where, err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return metadata{}, fmt.Errorf("oidc: %s answered %d", where, res.StatusCode)
	}

	var found metadata
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&found); err != nil {
		return metadata{}, fmt.Errorf("oidc: could not read %s: %w", where, err)
	}

	/*
	   The issuer in the document must be the one that was asked for.

	   This is the check that makes discovery safe: without it, anything that
	   can answer at the well-known path can name its own authorization and
	   token endpoints, and every subsequent verification is against keys it
	   chose.
	*/
	if strings.TrimSuffix(found.Issuer, "/") != strings.TrimSuffix(issuer, "/") {
		return metadata{}, fmt.Errorf(
			"oidc: %s says its issuer is %q, not %q", where, found.Issuer, issuer)
	}
	if found.Authorization == "" || found.Token == "" || found.JWKS == "" {
		return metadata{}, fmt.Errorf("oidc: %s is missing an endpoint this needs", where)
	}
	return found, nil
}

// random is 256 bits, base64url. Used for the nonce, the PKCE verifier and the
// state id, all of which are only as good as the entropy behind them.
func random() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("oidc: no entropy: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

func firstOf(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
