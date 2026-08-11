// Command cronos-token mints an embed token.
//
// What a host application's backend does server-to-server before rendering a
// report. It exists as a command because the alternative during development is
// pasting a token from a log, and a token you cannot mint on demand is one
// people extend the lifetime of instead.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gsoultan/cronos/internal/platform/token"
)

func main() {
	var (
		org      = flag.String("org", "acme", "organization id")
		project  = flag.String("project", "finance", "project id")
		subject  = flag.String("subject", "demo-user", "who this is, in the host's own model")
		report   = flag.String("report", "", "pin the token to one report")
		audience = flag.String("audience", token.Embed, "embed (an end customer) or portal (an author)")
		role     = flag.String("role", "editor", "project role, for a portal token")
		scope    = flag.String("scope", "", "row scope, as key=value,key=value")
		params   = flag.String("params", "", "pinned dataset params, as key=value,key=value")
		lifetime = flag.Duration("for", time.Hour, "how long it lives")
	)
	flag.Parse()

	key := os.Getenv("CRONOS_SIGNING_KEY")
	if key == "" {
		fmt.Fprintln(os.Stderr, "CRONOS_SIGNING_KEY is required")
		os.Exit(1)
	}
	signer, err := token.NewSigner([]byte(key))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	tok, err := signer.Mint(token.Claims{
		Audience: *audience, Role: *role,
		Org: *org, Project: *project, Subject: *subject, Report: *report,
		Scope: pairs(*scope), Params: anyPairs(*params),
	}, *lifetime)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if flag.Arg(0) == "-v" {
		claims, _ := signer.Verify(tok, *audience)
		b, _ := json.MarshalIndent(claims, "", "  ")
		fmt.Fprintln(os.Stderr, string(b))
	}
	fmt.Println(tok)
}

func pairs(s string) map[string]string {
	if s == "" {
		return nil
	}
	out := map[string]string{}
	for _, kv := range strings.Split(s, ",") {
		if k, v, ok := strings.Cut(kv, "="); ok {
			out[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return out
}

func anyPairs(s string) map[string]any {
	out := map[string]any{}
	for k, v := range pairs(s) {
		out[k] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
