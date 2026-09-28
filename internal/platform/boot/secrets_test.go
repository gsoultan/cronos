package boot

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gsoultan/cronos/internal/platform/config"
)

/*
One process serving two customers' projects has one environment. Before this,
every project resolved against all of it, so an editor in one could publish a
datasource naming the other's warehouse password and a host of their own, and
the process would connect there with it.

Now a project of several reads only what the operator shared, and its own
directory under the secrets directory.
*/
func TestAProjectOfSeveralReadsOnlyWhatIsShared(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "acme", "finance"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "acme", "finance", "warehouse_pw"), []byte("acme-pw\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CRONOS_SECRET_GLOBEX_WAREHOUSE_PW", "globex-pw")
	t.Setenv("CRONOS_SECRET_MAPBOX_TOKEN", "pk.everybody")
	cfg := config.Server{SecretsDir: dir, SharedSecrets: []string{"mapbox-token"}}

	acme := deploymentSecrets(cfg, tenant{org: "acme", project: "finance"}, true)
	if v, ok := acme.Secret("globex_warehouse_pw"); ok {
		t.Errorf("acme read globex's password from the environment: %q", v)
	}
	if v, _ := acme.Secret("mapbox-token"); v != "pk.everybody" {
		t.Errorf("a shared name = %q", v)
	}
	if v, _ := acme.Secret("warehouse_pw"); v != "acme-pw" {
		t.Errorf("acme's own file = %q", v)
	}
	globex := deploymentSecrets(cfg, tenant{org: "globex", project: "finance"}, true)
	if v, ok := globex.Secret("warehouse_pw"); ok {
		t.Errorf("globex read acme's file: %q", v)
	}

	// One project is the deployment: everything resolves, exactly as before.
	one := deploymentSecrets(cfg, tenant{org: "acme", project: "finance"}, false)
	if v, _ := one.Secret("globex_warehouse_pw"); v != "globex-pw" {
		t.Errorf("a single-project deployment lost its environment: %q", v)
	}
}

// A key set too short is refused at startup, like a short signing key; no key
// at all is a deployment that stores nothing.
func TestTheSecretsKeyIsCheckedAtStartup(t *testing.T) {
	if s, err := sealing(config.Server{}); s != nil || err != nil {
		t.Errorf("no key = %v, %v; want nothing and no error", s, err)
	}
	if _, err := sealing(config.Server{SecretsKey: []byte("short")}); err == nil {
		t.Error("a five-byte key was accepted")
	}
	s, err := sealing(config.Server{SecretsKey: []byte("a-secrets-key-that-is-long-enough-to-seal")})
	if err != nil || s == nil {
		t.Errorf("a good key = %v, %v", s, err)
	}
}
