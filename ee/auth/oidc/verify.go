package oidc

import (
	"context"
	"crypto"
	"crypto/rsa"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

/*
Verifying an id token.

Written out rather than taken from a library, and the reason is the same one
the token package gives for its own signing: this is the code that decides
whether somebody is who they say, and it is worth being able to read all of it
on one screen.

Every check here has a failure that is somebody else signing in as you:

  - the algorithm, taken from the token's own header, is the classic one — a
    token that says `alg: none`, or asks to be verified with HMAC against a
    public key everybody has;
  - the key id, so a provider rotating keys does not lock everybody out and a
    token naming an unknown key is not verified against whichever key is first;
  - the issuer, so a token from a different provider entirely is refused;
  - the audience, so a token minted for another application at the same
    provider is not accepted here;
  - the expiry, and the nonce, which is what ties the answer to the request
    that asked for it.
*/

// claims is the part of an id token this reads.
type claims struct {
	Issuer            string   `json:"iss"`
	Subject           string   `json:"sub"`
	Audience          audience `json:"aud"`
	Expiry            int64    `json:"exp"`
	IssuedAt          int64    `json:"iat"`
	Nonce             string   `json:"nonce"`
	Email             string   `json:"email"`
	EmailVerified     *bool    `json:"email_verified"`
	Name              string   `json:"name"`
	PreferredUsername string   `json:"preferred_username"`
	Groups            []string `json:"groups"`
	// Roles, because Entra and Auth0 call the same thing that.
	Roles []string `json:"roles"`
}

// audience is one string or several, which the specification permits and every
// provider disagrees about.
type audience []string

func (a *audience) UnmarshalJSON(data []byte) error {
	var one string
	if err := json.Unmarshal(data, &one); err == nil {
		*a = audience{one}
		return nil
	}
	var many []string
	if err := json.Unmarshal(data, &many); err != nil {
		return fmt.Errorf("oidc: aud is neither a string nor a list")
	}
	*a = many
	return nil
}

func (a audience) contains(want string) bool {
	for _, v := range a {
		if v == want {
			return true
		}
	}
	return false
}

// verify checks the token and returns what it asserts.
func (p *Provider) verify(ctx context.Context, raw, nonce string) (claims, error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return claims{}, fmt.Errorf("oidc: the id token is not a JWT")
	}

	var head struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := decodeSegment(parts[0], &head); err != nil {
		return claims{}, fmt.Errorf("oidc: unreadable token header: %w", err)
	}

	/*
	   An allow-list, and never the token's own word for it.

	   `alg: none` is a token that verifies itself. `alg: HS256` against a
	   provider's RSA public key is the other half of the same attack: the key
	   is published, so anybody can compute that MAC. Both are refused by
	   deciding here which algorithms exist.
	*/
	digest, ok := algorithms[head.Alg]
	if !ok {
		return claims{}, fmt.Errorf("oidc: refusing a token signed with %q", head.Alg)
	}

	key, err := p.keys.rsa(ctx, head.Kid)
	if err != nil {
		return claims{}, err
	}

	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return claims{}, fmt.Errorf("oidc: unreadable signature")
	}
	signed := []byte(parts[0] + "." + parts[1])

	hasher := digest.New()
	hasher.Write(signed)
	if err := rsa.VerifyPKCS1v15(key, digest, hasher.Sum(nil), signature); err != nil {
		return claims{}, fmt.Errorf("oidc: the signature does not verify")
	}

	var c claims
	if err := decodeSegment(parts[1], &c); err != nil {
		return claims{}, fmt.Errorf("oidc: unreadable claims: %w", err)
	}

	now := p.now()
	switch {
	case strings.TrimSuffix(c.Issuer, "/") != strings.TrimSuffix(p.metadata.Issuer, "/"):
		return claims{}, fmt.Errorf("oidc: the token was issued by %q", c.Issuer)
	case !c.Audience.contains(p.cfg.ClientID):
		// A token minted for a different application at the same provider.
		// Without this, any other client there can sign somebody into cronos.
		return claims{}, fmt.Errorf("oidc: the token was not minted for this application")
	case c.Expiry == 0 || now.After(time.Unix(c.Expiry, 0).Add(leeway)):
		return claims{}, fmt.Errorf("oidc: the token has expired")
	case c.IssuedAt != 0 && now.Add(leeway).Before(time.Unix(c.IssuedAt, 0)):
		return claims{}, fmt.Errorf("oidc: the token was issued in the future — check the clocks")
	case nonce != "" && c.Nonce != nonce:
		// What ties this answer to the request that asked for it. Without it,
		// a token captured from another sign-in can be replayed here.
		return claims{}, fmt.Errorf("oidc: the nonce did not match")
	case c.Subject == "":
		return claims{}, fmt.Errorf("oidc: the token names nobody")
	}

	/*
	   An unverified email is not an email.

	   A provider that lets somebody self-assert an address and says so is one
	   where domain restriction means nothing — anybody can claim
	   `someone@yourcompany.example`. Absent is permitted, because providers
	   that only ever hold verified addresses do not send the claim.
	*/
	if c.EmailVerified != nil && !*c.EmailVerified {
		return claims{}, fmt.Errorf("oidc: %s is not a verified address at this provider", c.Email)
	}

	// Entra and Auth0 put group membership in `roles`. Both are read, because
	// which one a deployment gets depends on a provider setting nobody
	// remembers changing.
	c.Groups = append(c.Groups, c.Roles...)
	return c, nil
}

// leeway forgives a little clock drift. Small: the cost of a generous window
// is a token that outlives its expiry.
const leeway = 60 * time.Second

// algorithms is the allow-list. RSA only: every provider that matters signs
// with it, and each additional family is another verification path to get
// right for a case nobody has asked for.
var algorithms = map[string]crypto.Hash{
	"RS256": crypto.SHA256,
	"RS384": crypto.SHA384,
	"RS512": crypto.SHA512,
}

func decodeSegment(segment string, into any) error {
	raw, err := base64.RawURLEncoding.DecodeString(segment)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, into)
}

/*
keySet holds the provider's public keys and refetches them when they change.

Providers rotate, and they do it without warning: a deployment that read the
keys once at startup signs everybody out on the morning the old key is retired.
Refetched when a token names a key that is not held — which is the signal that
rotation happened, and is also what an attacker would send to make this refetch
in a loop, so it is rate-limited.
*/
type keySet struct {
	client *http.Client
	url    string

	mu      sync.RWMutex
	keys    map[string]*rsa.PublicKey
	fetched time.Time
}

func newKeySet(client *http.Client, url string) *keySet {
	return &keySet{client: client, url: url, keys: map[string]*rsa.PublicKey{}}
}

func (k *keySet) rsa(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	if key := k.held(kid); key != nil {
		return key, nil
	}
	if err := k.refresh(ctx); err != nil {
		return nil, err
	}
	if key := k.held(kid); key != nil {
		return key, nil
	}
	return nil, fmt.Errorf("oidc: the provider has no key %q", kid)
}

func (k *keySet) held(kid string) *rsa.PublicKey {
	k.mu.RLock()
	defer k.mu.RUnlock()

	if key, ok := k.keys[kid]; ok {
		return key
	}
	// A provider with one key sometimes omits the id from the token. Only
	// answer that way when there is exactly one, because picking one of
	// several is picking which signature to trust.
	if kid == "" && len(k.keys) == 1 {
		for _, only := range k.keys {
			return only
		}
	}
	return nil
}

func (k *keySet) refresh(ctx context.Context) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	// At most once a minute. A token naming a key nobody has is the shape of
	// rotation and also the shape of somebody making this fetch in a loop.
	if time.Since(k.fetched) < time.Minute && len(k.keys) > 0 {
		return fmt.Errorf("oidc: unknown signing key, and the key set was just refreshed")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, k.url, nil)
	if err != nil {
		return err
	}
	res, err := k.client.Do(req)
	if err != nil {
		return fmt.Errorf("oidc: could not reach the key set: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("oidc: the key set answered %d", res.StatusCode)
	}

	var document struct {
		Keys []struct {
			Kty string `json:"kty"`
			Kid string `json:"kid"`
			Use string `json:"use"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&document); err != nil {
		return fmt.Errorf("oidc: could not read the key set: %w", err)
	}

	fresh := map[string]*rsa.PublicKey{}
	for _, key := range document.Keys {
		if key.Kty != "RSA" || (key.Use != "" && key.Use != "sig") {
			continue
		}
		parsed, err := rsaKey(key.N, key.E)
		if err != nil {
			continue // one unreadable key is not a reason to reject the rest
		}
		fresh[key.Kid] = parsed
	}
	if len(fresh) == 0 {
		return fmt.Errorf("oidc: the key set has no usable RSA signing key")
	}

	k.keys, k.fetched = fresh, time.Now()
	return nil
}

// rsaKey rebuilds a public key from the base64url modulus and exponent.
func rsaKey(modulus, exponent string) (*rsa.PublicKey, error) {
	n, err := base64.RawURLEncoding.DecodeString(modulus)
	if err != nil {
		return nil, err
	}
	e, err := base64.RawURLEncoding.DecodeString(exponent)
	if err != nil {
		return nil, err
	}
	if len(e) == 0 || len(e) > 8 {
		return nil, fmt.Errorf("oidc: implausible exponent")
	}

	// Left-padded to eight bytes, because the exponent arrives as the shortest
	// big-endian encoding that fits — usually three bytes for 65537.
	padded := make([]byte, 8)
	copy(padded[8-len(e):], e)

	return &rsa.PublicKey{
		N: new(big.Int).SetBytes(n),
		E: int(binary.BigEndian.Uint64(padded)),
	}, nil
}
