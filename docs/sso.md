# Single sign-on

Available in the enterprise build (`cmd/cronosd-ee`). The community build signs
people in with passwords and says so at `/v1/auth/methods`, which is what the
sign-in page reads to decide whether to draw a button.

OIDC, authorization code with PKCE. SAML is not implemented; the seam it would
fill (`extension.SignInFlow`) mentions no protocol, so it can be added without
the core learning any of OIDC's vocabulary.

## Configuring it

```bash
export CRONOS_OIDC_ISSUER=https://acme.okta.com
export CRONOS_OIDC_CLIENT_ID=0oa1b2c3
export CRONOS_OIDC_CLIENT_SECRET=…
export CRONOS_OIDC_REDIRECT_URL=https://reports.acme.com/v1/auth/sso/callback

# Optional.
export CRONOS_OIDC_SCOPES=openid,email,profile,groups
export CRONOS_OIDC_ROLES="analytics-admins=admin,analysts=editor"
export CRONOS_OIDC_DEFAULT_ROLE=viewer
export CRONOS_OIDC_DOMAINS=acme.com
export CRONOS_OIDC_POST_LOGOUT_URL=https://reports.acme.com/
```

The redirect URL must be registered with the provider exactly, path included.
A mismatch is the most common failure and the provider's own message names it,
which is why that message is passed through rather than replaced.

## Signing out

Signing out of cronos alone is half a sign-out: the provider's session is still
open, so the next sign-in is silent, and on a shared machine the next person is
signed in as the last one.

The sign-out button posts to `/v1/auth/sso/logout` while the session is still
valid, and the answer is where to send the browser next:

```json
{ "redirect": "https://acme.okta.com/oauth2/v1/logout?client_id=0oa1b2c3&id_token_hint=…" }
```

Built from the provider's own `end_session_endpoint`, which cronos reads from
the discovery document it already fetches. A provider that publishes none —
Dex, for instance — gets an empty redirect, the local session ends, and nothing
is said about the other one. It is not guessed at: a guessed `/logout` is a 404
on a domain the person recognises, which reads as cronos being broken.

`id_token_hint` is the token the provider signed at sign-in. Okta requires it
and refuses a logout without one; Entra and Keycloak accept `client_id` alone,
so both are sent. It is held **in this process's memory only**, keyed by
account, dropped when spent and swept when the session it belongs to expires. It
is never written to the database, never in a backup, and never handed to the
browser — an identity token is a credential at the provider, and localStorage is
readable by any script the page loads. A restart therefore loses the hints, and
a sign-out after one goes without: some providers refuse that, which is the
honest outcome, because the cronos session has already ended by then.

`CRONOS_OIDC_POST_LOGOUT_URL` is where the provider sends somebody afterwards.
It must be registered with them, exactly like the redirect URL, and is omitted
when unset rather than guessed — an unregistered value is refused outright and
turns every sign-out into an error page at the identity provider. Nothing in
the request influences it: the route takes no landing parameter, which is what
keeps an open redirect off the one page somebody reaches expecting to have just
left.

Setting an issuer makes single sign-on **required to start**: discovery runs at
startup and a failure stops the server. A deployment that asked for SSO and
quietly fell back to passwords is one where nobody notices until the passwords
have been removed.

## Roles

`CRONOS_OIDC_ROLES` maps a group the provider asserts to a role here. Somebody
in several matching groups gets the strongest — that is what whoever put them
in both meant, and the alternative depends on a directory's iteration order.
Somebody in none gets `CRONOS_OIDC_DEFAULT_ROLE`, which is `viewer`, because a
role mapping that fails should give too little access rather than too much.

Groups are read from `groups` and from `roles`, because Entra and Auth0 call
the same thing the second one.

## What it does not do

**It does not own who has access.** A person who signs in through the provider
gets an account here, appears on the People page, and can be disabled there
like anybody else — and a disabled account is refused at the next sign-in even
though the provider still vouches for them. The directory is authoritative
about who somebody is; this project is authoritative about whether they may
read it.

**It does not re-apply roles on every sign-in.** The role is set the first time
somebody appears. An administrator who demoted them in Settings should not find
the identity provider promoting them back at the next login.

**There is no single log-out.** Signing out of cronos ends the cronos session;
it does not reach the provider, and the next sign-in will be silent if the
provider's own session is still valid. Ending both is the provider's
`end_session_endpoint`, which is not implemented.

## Restricting who may sign in

`CRONOS_OIDC_DOMAINS` is empty by default, which admits anybody the provider
vouches for. That is right when the provider is the company's own directory and
wrong when it is a public one: a Google client with no domain restriction is a
sign-in button for the entire internet.

An address the provider marks unverified is refused whatever the domain list
says. Otherwise anybody could self-assert `someone@yourcompany.example` and the
restriction would mean nothing.

## Checking it against a real provider

## When a sign-in does not finish

The sentence the browser gets names the system to go and look at, because for a
while it named the wrong one. Every refusal read as *the identity provider
refused this sign-in*, and only one of them is that.

| What it says | Where the problem is |
| :--- | :--- |
| the identity provider refused this sign-in | The provider. Usually a declined consent screen (`access_denied`), or an account it will not sign in. |
| this server and the identity provider disagree about the time | Neither. NTP on this host, or on the provider's. A minute of leeway is allowed; beyond that a token minted seconds ago arrives outside its window. |
| that account is not one this deployment admits | Here, and nothing is broken. `CRONOS_OIDC_ALLOWED_DOMAINS` does not list their domain, or the provider marks their address unverified. |
| this sign-in could not be verified | Configuration. A signature that does not check out, a token minted for another client, or an issuer that does not match — usually a client id or issuer URL that belongs to a different environment. |
| this sign-in did not start here / expired | The browser. A stale bookmark, a replayed callback, or a tab left open past the state's life. |

The full error is in the log beside each one, at `msg="sign-in refused"` with a
`reason=` field carrying the same classification, so an alert can count them
apart. The clock case is the one worth having a name for: it looks exactly like
a provider outage, and it cost an afternoon here before it had one.

Every part of SSO that can be wrong is wrong at a boundary — a redirect URL the
provider does not recognise, a discovery document without the field we expected,
a cookie the browser withholds, a logout refused because it wanted a hint we did
not keep. Unit tests answer none of those, because in a unit test both sides of
each boundary are ours.

```bash
./scripts/live-sso.sh
```

Stands up Keycloak in a container, creates a realm, a client and a person, and
then behaves like a browser: follows the redirects, keeps the cookies, signs in
at the provider's own login page, signs out, and — the assertion the whole
feature exists for — starts a second sign-in and requires that it asks who you
are. Before single log-out that step went through silently.
