package boot

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gsoultan/cronos/internal/adapter/api"
	sqlstore "github.com/gsoultan/cronos/internal/adapter/store/sql"
	"github.com/gsoultan/cronos/internal/core/identity"
	"github.com/gsoultan/cronos/internal/core/principal"
	"github.com/gsoultan/cronos/internal/platform/config"
)

/*
First run: a server with no configuration at all, serving one endpoint.

Until this, a deployment had to be configured before it would start —
CRONOS_SIGNING_KEY had no default and Load refused without one. That is correct
for a container, where configuration arrives with the container, and it is a
wall for somebody who installed a package and was handed a URL.

So a server with no key now starts and serves /v1/setup and nothing else. What
makes that safe rather than reckless is the token below.

# Why there is a token

The old /v1/setup handed out an administrator to whoever asked first, and was
defensible because the deployment already had a signing key: reaching that point
meant somebody had shell access and had configured it. Setting the key through
the page removes that proof, and whoever wins the race would own the
deployment's whole root of trust rather than one account on it.

So the proof is restored explicitly. On first boot cronosd writes a random token
to a file only its own user can read, and logs where it put it — the path, never
the value. Setup will not proceed without it. The bar to bootstrap is what it
has always been, shell access on the box, and the file is gone the moment setup
succeeds.

This is not the "token in the log or an environment variable" that
api/setup.go's comment rejects, and the difference is the part that matters: it
cannot be left switched on. It is single use, it is deleted on success, and
nothing reopens it short of deleting the configuration.
*/

// TokenName is the file a first-run token is written to, beside the
// configuration it will produce.
const TokenName = "setup-token"

// tokenPath is where the token lives, derived from the configuration path so
// the two always travel together.
func tokenPath() string { return filepath.Join(filepath.Dir(config.Path()), TokenName) }

/*
issueToken writes a fresh first-run token and returns its path.

Rewritten on every unconfigured boot rather than kept. A token that survives a
restart is one that was readable for as long as the machine has been up, and
the operator who is about to use it is looking at the log line from this boot.
*/
func issueToken(log *slog.Logger) (string, error) {
	path := tokenPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return "", fmt.Errorf("setup: %w", err)
	}

	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("setup: %w", err)
	}
	secret := hex.EncodeToString(b[:])

	// 0600 and truncated: this is the credential, and a token appended to is a
	// file with two of them.
	if err := os.WriteFile(path, []byte(secret+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("setup: %w", err)
	}

	// The path, never the value. A log is copied into support tickets and
	// shipped to whatever collects stdout, and a secret in one is a secret in
	// all of them.
	log.Warn("this deployment has no configuration — open /setup to configure it",
		"token", path)
	return path, nil
}

// readToken reads the issued token back, for comparison.
func readToken(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(raw)), nil
}

/*
setupGate answers whether a presented token is the one on disk.

Constant time, because a byte-by-byte comparison leaks how much of a guess was
right and this is the only thing standing between a stranger and the
deployment. Re-read per request rather than held, so a token replaced by
another boot is the one that counts.
*/
func setupGate(path string) func(string) bool {
	return func(presented string) bool {
		want, err := readToken(path)
		if err != nil || want == "" {
			// No token file means setup is not open. Refusing is the only safe
			// reading: a missing file is either a deployment that has been set
			// up or one somebody has tampered with.
			return false
		}
		return subtle.ConstantTimeCompare([]byte(presented), []byte(want)) == 1
	}
}

// clearToken removes the token once it has been spent.
func clearToken(path string, log *slog.Logger) {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		// Not fatal, and said loudly. The configuration is written by this
		// point so setup is closed by the next boot anyway, but a token left
		// on disk is a credential nobody meant to keep.
		log.Error("could not remove the first-run token — delete it by hand",
			"path", path, "err", err)
	}
}

/*
NewSigningKey generates the key a deployment signs with.

Generated rather than typed. A person choosing a signing key chooses a
memorable one, and this is the root of trust for every token the deployment
ever issues; 32 bytes from crypto/rand is both stronger than anything anybody
types and one fewer field on the form.
*/
func NewSigningKey() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawStdEncoding.EncodeToString(b[:]), nil
}

/*
serveSetup runs the server in its unconfigured state.

Only /v1/setup and the probes. Not the catalogue, not a report, not sign-in —
there is no signing key to authenticate anybody with, and an endpoint that can
only fail is one somebody spends an afternoon probing.

It returns when setup has written a configuration, and the caller exits so the
service manager starts it again. Restarting rather than reconfiguring in place:
the second boot is then an ordinary boot, with no half-applied state to reason
about and no code path that exists only on the first run.
*/
func serveSetup(cfg config.Server, log *slog.Logger) error {
	path, err := issueToken(log)
	if err != nil {
		return err
	}

	done := make(chan struct{})
	var once sync.Once
	finish := func() { once.Do(func() { clearToken(path, log); close(done) }) }

	handler := api.NewFirstRun(api.FirstRunDeps{
		ConfigPath:  config.Path(),
		Accepts:     setupGate(path),
		NewKey:      NewSigningKey,
		Finished:    finish,
		Log:         log,
		Environment: environmentAlready(),
	})

	mux := http.NewServeMux()
	mux.Handle("/v1/setup", handler)
	mux.HandleFunc("/v1/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"status":"setup"}`))
	})
	/*
	   Not ready, and saying so.

	   A load balancer must not send anybody here: the deployment cannot serve a
	   report and will restart the moment it is configured. 503 is what takes an
	   instance out of rotation, and it is the honest answer to "can this serve
	   traffic".
	*/
	mux.HandleFunc("/v1/ready", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"status":"setup","checks":{"config":"not configured"}}`))
	})

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           api.NewObserved(mux, log),
		ReadTimeout:       time.Minute,
		ReadHeaderTimeout: 20 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}

	go func() {
		<-done
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	log.Info("cronosd listening for setup", "addr", cfg.Addr, "config", config.Path())
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	select {
	case <-done:
		log.Info("configured — restarting", "config", config.Path())
		return nil
	default:
		// The listener closed without setup finishing: a signal, or a port
		// taken. Reported rather than treated as success, so a service manager
		// does not read a failed start as a completed one.
		return errors.New("setup: stopped before this deployment was configured")
	}
}

/*
environmentAlready names the settings the environment has already fixed.

Shown on the form as read-only rather than hidden, because the environment
wins: a field somebody fills in that is then ignored is worse than one they
were told they cannot change. Values are never included — several of these are
secrets and this reaches a browser.
*/
func environmentAlready() []string {
	var set []string
	for _, name := range []string{
		"CRONOS_SIGNING_KEY", "CRONOS_ORG", "CRONOS_PROJECT",
		"CRONOS_STORE_DSN", "CRONOS_DSN", "CRONOS_ADDR", "CRONOS_ORIGINS",
	} {
		if os.Getenv(name) != "" {
			set = append(set, name)
		}
	}
	return set
}

/*
adoptPendingAdmin creates the account first-run setup collected.

Setup ran in a process with no database — the DSN arrived in the same request —
so it wrote the administrator down and this boot makes them. The password never
travelled: what was written is a bcrypt hash, and it is handed to the store as
one.

Skipped without a store, because a file-backed deployment has nowhere to put an
account and setup would not have offered to make one. Skipped where accounts
already exist, so a pending file somebody left behind cannot add an
administrator to a running deployment — the only thing this may do is populate
an empty deployment.
*/
func adoptPendingAdmin(ctx context.Context, records *sqlstore.Store, log *slog.Logger) error {
	if records == nil {
		return nil
	}

	pending, found, err := api.ReadPendingAdmin(config.Path())
	if err != nil {
		return err
	}
	if !found {
		return nil
	}

	done, err := records.SetUp(ctx)
	if err != nil {
		return fmt.Errorf("reading whether this deployment has accounts: %w", err)
	}
	if done {
		/*
		   Somebody got here first, or this file outlived the setup that wrote
		   it. Either way it is not used, and it is removed rather than left:
		   a credential on disk for an account that already exists is one
		   nobody is watching.
		*/
		log.Warn("a first-run administrator was recorded but this deployment already has accounts",
			"email", pending.Email)
		return api.ClearPendingAdmin(config.Path())
	}

	user := identity.User{
		ID: identity.NewID(), Email: pending.Email, Name: pending.Name,
		Org: pending.Org, Project: pending.Project,
		// A project administrator as well as a platform one, the same as the
		// account /v1/setup creates: platform to administer the deployment,
		// project because somebody has to write the first report and platform
		// administration deliberately does not grant that.
		Role: string(principal.ProjectAdmin),
	}
	if err := records.CreateUserWithHash(ctx, user, pending.Hash); err != nil {
		return fmt.Errorf("creating the first administrator: %w", err)
	}
	if err := records.GrantPlatform(ctx, user.ID, "first-run setup"); err != nil {
		return fmt.Errorf("granting the first administrator: %w", err)
	}

	log.Info("created the administrator from first-run setup",
		"email", pending.Email, "project", pending.Org+"/"+pending.Project)
	return api.ClearPendingAdmin(config.Path())
}
