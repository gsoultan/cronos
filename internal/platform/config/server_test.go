package config_test

import (
	"testing"
	"time"

	"github.com/gsoultan/cronos/internal/platform/config"
)

/*
Reading the environment.

The package had no tests, and it is the one that decides what every other
package is handed. Most of it is defaults, which are only interesting where
getting one wrong is silent: a signing key that is generated rather than
demanded, a retention that deletes because somebody mistyped a duration, a
BehindProxy that believes a header on a deployment with nothing in front.

Load reads process environment, so these use t.Setenv and do not run in
parallel. The key is set in each because Load refuses without it, which is
itself the first thing asserted.
*/

// signed sets the one variable Load will not proceed without.
func signed(t *testing.T) {
	t.Helper()
	t.Setenv("CRONOS_SIGNING_KEY", "0123456789abcdef0123456789abcdef")
}

func load(t *testing.T) config.Server {
	t.Helper()
	s, err := config.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return s
}

/*
No key, no server.

Refused rather than generated, and this is the assertion that keeps it that
way. A key generated at startup invalidates every token on restart; a key
defaulted in the source is the same key as everybody else's deployment.
*/
func TestLoadRefusesWithoutASigningKey(t *testing.T) {
	t.Setenv("CRONOS_SIGNING_KEY", "")

	if _, err := config.Load(); err == nil {
		t.Fatal("a server with no signing key was configured")
	}
}

func TestTheDefaultsAreTheOnesDocumented(t *testing.T) {
	signed(t)
	s := load(t)

	for _, c := range []struct{ name, got, want string }{
		{"Addr", s.Addr, ":8787"},
		{"Definitions", s.Definitions, "examples"},
		{"Driver", s.Driver, "sqlite"},
		{"Org", s.Org, "default"},
		{"Project", s.Project, "default"},
		{"StoreDriver", s.StoreDriver, "postgres"},
		{"Deliveries", s.Deliveries, "deliveries"},
		// On by default rather than off: a product whose claim is governed
		// access to somebody else's customers' data has no answer to an
		// auditor if nothing is recorded unless it is switched on.
		{"Audit", s.Audit, "log"},
	} {
		if c.got != c.want {
			t.Errorf("%s is %q, want %q", c.name, c.got, c.want)
		}
	}

	// Off by default, both of them. A scheduler armed by default sends
	// everybody a statement the first time somebody starts a second replica.
	if s.Scheduler {
		t.Error("the scheduler is armed by default")
	}
	if s.BehindProxy {
		t.Error("X-Forwarded-For is believed by default")
	}
	// For ever, because how long a business must be able to show what it sent
	// is a legal question with a different answer in every jurisdiction.
	if s.Retention != 0 {
		t.Errorf("history retention defaults to %s, want for ever", s.Retention)
	}
}

/*
CRONOS_BEHIND_PROXY is "1" and nothing else.

It governs two things that both fail quietly when it is wrong: rate limits keyed
by an address the caller can choose, and the SSO state cookie's Secure
attribute. "true" reading as false is the safe direction, and asserting it
means nobody widens this to strconv.ParseBool without meaning to.
*/
func TestBehindProxyIsOnlyEverTheStringOne(t *testing.T) {
	for _, c := range []struct {
		raw  string
		want bool
	}{
		{"1", true},
		{"true", false},
		{"yes", false},
		{"0", false},
		{"", false},
	} {
		t.Run("CRONOS_BEHIND_PROXY="+c.raw, func(t *testing.T) {
			signed(t)
			t.Setenv("CRONOS_BEHIND_PROXY", c.raw)

			if got := load(t).BehindProxy; got != c.want {
				t.Errorf("%q gave %v, want %v", c.raw, got, c.want)
			}
		})
	}
}

/*
A bad duration is zero, and zero here means "keep for ever".

Deliberate, and worth pinning: this value governs deletion, so a typo that
stops the server is safer than a typo that deletes more than somebody meant. A
negative one is the same case — "-2160h" is not a retention.
*/
func TestABadRetentionKeepsEverythingRatherThanGuessing(t *testing.T) {
	for _, c := range []struct {
		raw  string
		want time.Duration
	}{
		{"2160h", 2160 * time.Hour},
		{"90m", 90 * time.Minute},
		{"", 0},
		{"90 days", 0},
		{"-2160h", 0},
		{"nonsense", 0},
	} {
		t.Run("CRONOS_HISTORY_RETENTION="+c.raw, func(t *testing.T) {
			signed(t)
			t.Setenv("CRONOS_HISTORY_RETENTION", c.raw)

			if got := load(t).Retention; got != c.want {
				t.Errorf("%q gave %s, want %s", c.raw, got, c.want)
			}
		})
	}
}

/*
Retired signing keys, for a rotation.

The empty cases matter more than the populated one. A trailing comma must not
become a zero-length key, because the signer refuses one and the deployment
would fail to boot over a typo in a list that is optional in the first place.
*/
func TestPreviousSigningKeysAreSplitAndTrimmed(t *testing.T) {
	for _, c := range []struct {
		name, raw string
		want      []string
	}{
		{"unset", "", nil},
		{"one", "aaa", []string{"aaa"}},
		{"two", "aaa,bbb", []string{"aaa", "bbb"}},
		{"spaced", " aaa , bbb ", []string{"aaa", "bbb"}},
		{"trailing comma", "aaa,", []string{"aaa"}},
		{"only commas", ",,,", nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			signed(t)
			t.Setenv("CRONOS_SIGNING_KEY_PREVIOUS", c.raw)

			got := load(t).PreviousKeys
			if len(got) != len(c.want) {
				t.Fatalf("%q gave %d keys, want %d", c.raw, len(got), len(c.want))
			}
			for i := range got {
				if string(got[i]) != c.want[i] {
					t.Errorf("key %d is %q, want %q", i, got[i], c.want[i])
				}
			}
		})
	}
}

// The portal URL is used to build links in email, so a trailing slash would
// produce "https://host//invitations/…".
func TestThePortalURLLosesItsTrailingSlash(t *testing.T) {
	signed(t)
	t.Setenv("CRONOS_PORTAL_URL", "https://reports.example.com/")

	if got := load(t).Portal; got != "https://reports.example.com" {
		t.Errorf("Portal is %q, want no trailing slash", got)
	}
}

func TestOriginsAreSplitAndUnsetMeansNone(t *testing.T) {
	signed(t)
	t.Setenv("CRONOS_ORIGINS", "https://a.example,https://b.example")

	if got := load(t).Origins; len(got) != 2 || got[0] != "https://a.example" {
		t.Fatalf("Origins is %v, want two entries", got)
	}

	t.Setenv("CRONOS_ORIGINS", "")
	if got := load(t).Origins; len(got) != 0 {
		t.Errorf("Origins is %v with none set, want empty", got)
	}
}

/*
A channel is configured or it is not, and half of one is not.

Both of these gate registration, and the point of the gate is that an
unregistered channel is refused at publish rather than at six in the morning. A
host with no From address would register a channel that fails on every send.
*/
func TestAChannelNeedsAllOfItsSettingsBeforeItCounts(t *testing.T) {
	for _, c := range []struct {
		name string
		smtp config.SMTP
		want bool
	}{
		{"both", config.SMTP{Host: "mail", From: "a@b"}, true},
		{"no from", config.SMTP{Host: "mail"}, false},
		{"no host", config.SMTP{From: "a@b"}, false},
		{"neither", config.SMTP{}, false},
	} {
		if got := c.smtp.Configured(); got != c.want {
			t.Errorf("SMTP %s: %v, want %v", c.name, got, c.want)
		}
	}

	for _, c := range []struct {
		name string
		s3   config.S3
		want bool
	}{
		{"both", config.S3{AccessKey: "k", SecretKey: "s"}, true},
		{"no secret", config.S3{AccessKey: "k"}, false},
		{"no access", config.S3{SecretKey: "s"}, false},
		{"endpoint alone", config.S3{Endpoint: "https://s3"}, false},
	} {
		if got := c.s3.Configured(); got != c.want {
			t.Errorf("S3 %s: %v, want %v", c.name, got, c.want)
		}
	}
}

// An empty variable falls back rather than overriding with nothing. Unsetting
// a variable in a unit file usually means setting it to "", and that has to
// behave like absence or the server binds to no address at all.
func TestAnEmptyVariableFallsBackToTheDefault(t *testing.T) {
	signed(t)
	t.Setenv("CRONOS_ADDR", "")
	t.Setenv("CRONOS_DRIVER", "")

	s := load(t)
	if s.Addr != ":8787" {
		t.Errorf("Addr is %q, want the default", s.Addr)
	}
	if s.Driver != "sqlite" {
		t.Errorf("Driver is %q, want the default", s.Driver)
	}
}
