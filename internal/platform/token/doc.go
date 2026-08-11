// Package token mints and verifies embed tokens.
//
// # Not JWT
//
// A JWT carries its own algorithm in a header the attacker also controls,
// which is where alg confusion and the `none` algorithm come from — a decade
// of library CVEs about a field that exists to let two parties who never met
// negotiate. Cronos mints these and cronos verifies them. There is no
// negotiation to support, so there is no header: one version prefix, one
// algorithm, and a signature over both.
//
// The shape is deliberately boring: v1.<payload>.<mac>, base64url, HMAC-SHA256.
// Anything a JWT library would do for us is something that can also be done to
// us.
//
// # What the token is for
//
// It carries the constraints a host application decided when it minted the
// token server-side: which organization and project, which report, and the row
// scope the end user is confined to. The browser never decodes it and cannot
// change it — see docs/tenancy.md.
package token
