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
```

The redirect URL must be registered with the provider exactly, path included.
A mismatch is the most common failure and the provider's own message names it,
which is why that message is passed through rather than replaced.

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
