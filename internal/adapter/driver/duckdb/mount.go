//go:build duckdb

package duckdb

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gsoultan/cronos/internal/core/definition"
)

/*
statement is one thing to execute, and the values in it that must not escape.

DuckDB echoes a failing statement back inside its error text — a parser error
prints the line it could not parse. For a CREATE SECRET that line holds the
key, so the caller needs to know which values to strip before the error reaches
a log. Carrying them beside the SQL beats guessing at the far end.
*/
type statement struct {
	sql string
	// holds are substrings of sql that are credentials.
	holds []string
}

// mount is the SQL that makes one source readable under its alias.
//
// A slice rather than one string: an object store needs an extension, then a
// credential, then a view, and the credential's failure has to be reported
// differently from the other two.
func mount(name string, src definition.DataSource) ([]statement, error) {
	if !MountName(name) {
		return nil, fmt.Errorf("duckdb: %q is not a mount name", name)
	}
	switch src.Driver {
	case "postgres":
		return attach("postgres", name, src.DSN), nil
	case "mysql":
		return attach("mysql", name, src.DSN), nil
	case "sqlite":
		return attach("sqlite", name, src.DSN), nil
	case "duckdb":
		// Already this engine. Attaching a DuckDB file needs no extension and
		// no type.
		return []statement{{sql: fmt.Sprintf(
			"ATTACH %s AS %s (READ_ONLY);", quote(src.DSN), name)}}, nil
	case "object-store":
		return view(name, src)
	}
	return nil, fmt.Errorf("duckdb: cannot mount a %s source", src.Driver)
}

// attach loads the extension and mounts the database, read-only.
//
// INSTALL then LOAD every time: both are idempotent, and the alternative is
// tracking which extensions a connection has seen — state that is wrong the
// first time a pooled connection is replaced.
func attach(extension, name, dsn string) []statement {
	return []statement{{
		sql: fmt.Sprintf(
			"INSTALL %s; LOAD %s; ATTACH %s AS %s (TYPE %s, READ_ONLY);",
			extension, extension, quote(dsn), name, strings.ToUpper(extension)),
		// The DSN holds the password, and an ATTACH that fails to parse prints
		// the line it choked on.
		holds: []string{dsn},
	}}
}

/*
view exposes an object store as a table.

A view rather than an attachment, because a bucket of Parquet is not a
database: there is no catalogue to mount, only files to read. The glob is
recursive so a lake partitioned by date does not need one view per day.

union_by_name because that is what a lake partitioned by date does to a schema.
Without it the reader takes its types from whichever file it globs first and
imposes them on every other, so the day somebody widened a column is the day
every query over the whole range starts failing — and the error names a file,
not a schema change.
*/
func view(name string, src definition.DataSource) ([]statement, error) {
	reader, ok := readers[src.Format]
	if !ok {
		return nil, fmt.Errorf("duckdb: cannot read %q from an object store", src.Format)
	}

	var out []statement
	if scheme := remote(src.URI); scheme != "" {
		ext := "httpfs"
		if stores[scheme] == "azure" {
			// A different extension, not a different flag on the same one.
			// httpfs speaks HTTP and the S3 API over it; Azure Blob is its own
			// protocol and its own secret type.
			ext = "azure"
		}
		out = append(out, statement{sql: fmt.Sprintf("INSTALL %s; LOAD %s;", ext, ext)})
		cred, err := credential(name, scheme, src)
		if err != nil {
			return nil, err
		}
		if cred != nil {
			out = append(out, *cred)
		}
	} else if src.Credentials != "" {
		// A local path with a credential on it is a definition that means
		// something its author would not recognise. Refused rather than
		// ignored: a credential that does nothing is one nobody rotates.
		return nil, fmt.Errorf(
			"duckdb: source %q has credentials and a uri that is not a remote store", src.Name)
	}

	uri := strings.TrimSuffix(src.URI, "/") + "/**/*." + src.Format
	out = append(out, statement{sql: fmt.Sprintf(
		"CREATE OR REPLACE VIEW %s AS SELECT * FROM %s(%s, union_by_name=true);",
		name, reader, quote(uri))})
	return out, nil
}

var readers = map[string]string{
	"parquet": "read_parquet",
	"csv":     "read_csv_auto",
	"json":    "read_json_auto",
}

// remote returns the URI's scheme when it names an object store, and "" when
// the URI is a local path.
func remote(uri string) string {
	scheme, _, ok := strings.Cut(uri, "://")
	if !ok {
		return ""
	}
	return strings.ToLower(scheme)
}

// stores maps a URI scheme to the DuckDB secret type that reads it.
//
// A closed table. A scheme absent from it reaches the reader without a
// credential, which is right for http and for a public bucket, and which is
// why credentials on one are an error rather than a silent omission.
var stores = map[string]string{
	"s3":  "s3",
	"gs":  "gcs",
	"gcs": "gcs",
	"r2":  "r2",
	// Three spellings of Azure Blob, because three tools write three of them:
	// az:// is what the CLI uses, azure:// what DuckDB's own documentation
	// writes, and abfss:// what Hadoop and everything descended from it emits.
	// Refusing two of the three is a definition rejected for using the wrong
	// correct word.
	"az":    "azure",
	"azure": "azure",
	"abfss": "azure",
}

/*
credential is what the store is read with, or nil for a public one.

Scoped to the source's own URI. An unscoped secret is the default for every
bucket on the connection, so two datasources in one dataset — a lake each, with
a key each — would resolve to whichever was created last. The scope is what
makes two of them mean two.
*/
func credential(name, scheme string, src definition.DataSource) (*statement, error) {
	typ, ok := stores[scheme]
	if !ok {
		if src.Credentials != "" || src.Region != "" {
			return nil, fmt.Errorf(
				"duckdb: source %q is %s://, which takes no credentials or region here",
				src.Name, scheme)
		}
		return nil, nil
	}
	if typ == "azure" {
		return azure(name, src)
	}
	if src.Credentials == "" {
		if src.Region == "" && src.Endpoint == "" {
			return nil, nil
		}
		// No credentials is a public bucket, and one can still need a secret:
		// it is what carries the region, and what carries the address of a
		// store that is not the cloud's own.
		return &statement{sql: fmt.Sprintf(
			"CREATE OR REPLACE SECRET %s_store (TYPE %s%s%s, SCOPE %s);",
			name, typ, region(src.Region), endpoint(src.Endpoint),
			quote(scope(src.URI)))}, nil
	}

	pairs, err := definition.ParseCredentials(chainPairs(src.Credentials))
	if err != nil {
		return nil, fmt.Errorf("duckdb: source %q: %w", src.Name, err)
	}
	// An Azure key on an S3 bucket is a definition somebody pasted from the
	// wrong page. Named here, because passing it through builds a statement
	// with a parameter the store has never heard of and fails with the
	// database's words for a mistake this one can describe.
	for _, k := range []string{"account_name", "account_key"} {
		if pairs[k] != "" {
			return nil, fmt.Errorf("duckdb: source %q is %s:// and its credentials hold %s, "+
				"which is Azure's", src.Name, scheme, k)
		}
	}

	if pairs["provider"] == definition.CredentialChain {
		// No key in the definition at all: DuckDB asks the environment, which
		// is how an instance with a role attached reads its own bucket.
		return &statement{sql: fmt.Sprintf(
			"CREATE OR REPLACE SECRET %s_store (TYPE %s, PROVIDER credential_chain%s%s, SCOPE %s);",
			name, typ, region(src.Region), endpoint(src.Endpoint),
			quote(scope(src.URI)))}, nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "CREATE OR REPLACE SECRET %s_store (TYPE %s", name, typ)
	var holds []string
	for _, k := range sorted(pairs) {
		fmt.Fprintf(&b, ", %s %s", strings.ToUpper(k), quote(pairs[k]))
		if k != "account_id" {
			holds = append(holds, pairs[k])
		}
	}
	b.WriteString(region(src.Region))
	b.WriteString(endpoint(src.Endpoint))
	fmt.Fprintf(&b, ", SCOPE %s);", quote(scope(src.URI)))
	return &statement{sql: b.String(), holds: holds}, nil
}

// region is the clause, or nothing when the definition did not say.
func region(r string) string {
	if r == "" {
		return ""
	}
	return ", REGION " + quote(r)
}

/*
endpoint is the clauses that point a read somewhere other than the cloud.

Three of them, because one is never enough. ENDPOINT is the host; USE_SSL comes
from the scheme the operator wrote, so a plain-text store is one somebody asked
for rather than one they got; and URL_STYLE is path, because bucket-as-subdomain
needs DNS for every bucket and an appliance on an internal address does not have
it. Every S3-compatible store this is for wants path style, and the ones that
want vhost are the cloud's own, which take no endpoint at all.
*/
func endpoint(e string) string {
	if e == "" {
		return ""
	}
	host := strings.TrimPrefix(strings.TrimPrefix(e, "https://"), "http://")
	ssl := "true"
	if strings.HasPrefix(e, "http://") {
		ssl = "false"
	}
	return fmt.Sprintf(", ENDPOINT %s, URL_STYLE 'path', USE_SSL %s",
		quote(strings.TrimSuffix(host, "/")), ssl)
}

// scope is the prefix a secret applies to: the source's own bucket and path.
func scope(uri string) string { return strings.TrimSuffix(uri, "/") + "/" }

/*
azure is the credential for Azure Blob, which is shaped unlike the others.

DuckDB's azure secret takes a connection string or an account name, and refuses
ACCOUNT_KEY as a parameter of its own — the key belongs inside the connection
string. A connection string is also semicolons and equals signs, which are what
this format separates fields with, so a definition holding one whole would be
read as four broken fields.

So the definition names the parts and this assembles them. `account_name` and
`account_key` are what Azure's own portal calls them, which is the point: a
definition should not rename what somebody is copying from.
*/
func azure(name string, src definition.DataSource) (*statement, error) {
	if src.Region != "" {
		// Azure has regions and its blob endpoints do not take one: the
		// account name resolves to its region. Refused rather than dropped,
		// because a region that does nothing reads as a region that works.
		return nil, fmt.Errorf(
			"duckdb: source %q is Azure, which takes an account rather than a region", src.Name)
	}
	if src.Credentials == "" {
		if src.Endpoint == "" {
			return nil, nil
		}
		return nil, fmt.Errorf(
			"duckdb: source %q gives an endpoint and no credentials — an Azure endpoint "+
				"is part of the account's connection string and cannot be set on its own",
			src.Name)
	}

	pairs, err := definition.ParseCredentials(chainPairs(src.Credentials))
	if err != nil {
		return nil, fmt.Errorf("duckdb: source %q: %w", src.Name, err)
	}
	account := pairs["account_name"]
	if account == "" {
		return nil, fmt.Errorf(
			"duckdb: source %q is Azure and its credentials name no account_name", src.Name)
	}

	if pairs["provider"] == definition.CredentialChain {
		// The managed-identity path. The account name stays, because a chain
		// answers "who are you" and not "which account".
		return &statement{sql: fmt.Sprintf(
			"CREATE OR REPLACE SECRET %s_store (TYPE azure, PROVIDER credential_chain, "+
				"ACCOUNT_NAME %s, SCOPE %s);",
			name, quote(account), quote(scope(src.URI)))}, nil
	}

	key := pairs["account_key"]
	if key == "" {
		return nil, fmt.Errorf(
			"duckdb: source %q is Azure and its credentials name no account_key", src.Name)
	}
	return &statement{
		sql: fmt.Sprintf(
			"CREATE OR REPLACE SECRET %s_store (TYPE azure, CONNECTION_STRING %s, SCOPE %s);",
			name, quote(connection(account, key, src.Endpoint)), quote(scope(src.URI))),
		// The whole string, because the key is inside it and a driver that
		// quotes the statement back quotes all of it.
		holds: []string{connection(account, key, src.Endpoint), key},
	}, nil
}

// connection assembles what Azure calls a connection string.
//
// With an endpoint for a local emulator or a private one, and with the public
// suffix otherwise. The protocol comes from the endpoint's scheme for the same
// reason it does for S3: plain text is something somebody asks for.
func connection(account, key, ep string) string {
	if ep == "" {
		return "DefaultEndpointsProtocol=https;AccountName=" + account +
			";AccountKey=" + key + ";EndpointSuffix=core.windows.net;"
	}
	proto := "https"
	if strings.HasPrefix(ep, "http://") {
		proto = "http"
	}
	return "DefaultEndpointsProtocol=" + proto + ";AccountName=" + account +
		";AccountKey=" + key + ";BlobEndpoint=" + strings.TrimSuffix(ep, "/") + "/" + account + ";"
}

// chainPairs lets the bare `chain` shorthand through the pair parser.
//
// It shipped as a whole value rather than a field, and Azure needs a chain to
// carry an account name beside it. Rewriting the shorthand here keeps one
// grammar downstream and the shorthand working.
func chainPairs(creds string) string {
	if creds == definition.CredentialChain {
		return "provider=" + definition.CredentialChain
	}
	return creds
}

func sorted(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// quote makes a string literal.
//
// DSNs and URIs come from a datasource definition, which an operator wrote —
// but an operator is not a reason to concatenate, and a password containing an
// apostrophe would otherwise end the literal and the statement with it.
func quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
