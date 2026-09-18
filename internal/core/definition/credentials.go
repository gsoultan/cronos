package definition

import (
	"fmt"
	"sort"
	"strings"
)

/*
CredentialChain is the credentials value meaning "ask the environment".

The alternative to a key in a definition. An instance with a role attached, a
pod with a projected token, a laptop with a profile — all of them already hold
the credential, and the deployment that uses one wants to say so rather than
copy a key into a secret store so that cronos can pass it back.
*/
const CredentialChain = "chain"

// credentialKeys are the parts of a credential a definition may set.
//
// A closed set. An unknown key here is a typo — `secret_key` for `secret` — and
// the failure it causes otherwise is a 403 from the object store, which reads
// like the key is wrong rather than unread.
var credentialKeys = map[string]bool{
	"key_id":        true,
	"secret":        true,
	"session_token": true,
	// Cloudflare R2 addresses a bucket by account rather than by region.
	"account_id": true,
	// Azure names the account and holds its key beside it. Not key_id and
	// secret under other names: Azure's own tooling calls them this, and a
	// definition that renames what the portal shows is one somebody has to
	// translate at the moment they are least able to.
	"account_name": true,
	"account_key":  true,
	// provider says where the credential comes from rather than what it is.
	// `chain` on its own is the shorthand; this is the form that can carry
	// what a chain still needs, which for Azure is the account name.
	"provider": true,
}

/*
ParseCredentials reads the key=value pairs a remote store is read with.

`key_id=AKIA...;secret=wJal...`, semicolon or newline separated. Not a single
opaque string, because an S3 credential is at least two values and at most
four, and a format that packs them positionally is one nobody can read back.

The values are not validated beyond being present: what a key looks like is the
object store's business, and a length check here would reject the next kind of
key before anybody could use it.
*/
func ParseCredentials(s string) (map[string]string, error) {
	out := map[string]string{}
	var unknown []string
	for _, field := range strings.FieldsFunc(s, func(r rune) bool {
		return r == ';' || r == '\n' || r == '\r'
	}) {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		k, v, ok := strings.Cut(field, "=")
		if !ok {
			// Named without the value, because the value is the key.
			return nil, fmt.Errorf("%w: credentials hold a field with no '='", ErrInvalid)
		}
		k = strings.ToLower(strings.TrimSpace(k))
		v = strings.TrimSpace(v)
		if !credentialKeys[k] {
			unknown = append(unknown, k)
			continue
		}
		if v == "" {
			return nil, fmt.Errorf("%w: credentials set %s to nothing", ErrInvalid, k)
		}
		out[k] = v
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return nil, fmt.Errorf("%w: credentials hold %s, and take %s or %q",
			ErrInvalid, strings.Join(unknown, ", "), known(), CredentialChain)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: credentials are set and hold nothing", ErrInvalid)
	}
	// A secret without the key it belongs to, or the reverse, is a credential
	// that cannot authenticate anything. Caught here rather than as a 403.
	if (out["key_id"] == "") != (out["secret"] == "") {
		return nil, fmt.Errorf("%w: credentials need key_id and secret together", ErrInvalid)
	}
	if (out["account_name"] == "") != (out["account_key"] == "") && out["provider"] == "" {
		return nil, fmt.Errorf(
			"%w: credentials need account_name and account_key together", ErrInvalid)
	}
	if p, ok := out["provider"]; ok && p != CredentialChain {
		// One value, because it is the only one that means anything. A typo
		// here would otherwise build a secret with a provider the store has
		// never heard of and fail where nobody is looking.
		return nil, fmt.Errorf("%w: credentials name provider %q, and the only one is %q",
			ErrInvalid, p, CredentialChain)
	}
	return out, nil
}

func known() string {
	out := make([]string, 0, len(credentialKeys))
	for k := range credentialKeys {
		out = append(out, k)
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}

// unresolved reports whether s still carries a ${secret:…} reference.
//
// definition is below the package that resolves them and cannot ask it, which
// is the right way round: this only needs to know that a value is not yet
// itself, not what it will become.
func unresolved(s string) bool { return strings.Contains(s, "${secret:") }
