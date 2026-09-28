package vault_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/gsoultan/cronos/internal/app/vault"
	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/principal"
	"github.com/gsoultan/cronos/internal/platform/secret"
)

// memory is a store in a map, counting reads so a test can see the cache.
type memory struct {
	mu    sync.Mutex
	rows  map[string]vault.Stored
	reads int
}

func newMemory() *memory { return &memory{rows: map[string]vault.Stored{}} }

func key(org, project, name string) string { return org + "|" + project + "|" + name }

func (m *memory) Sealed(_ context.Context, org, project, name string) ([]byte, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reads++
	st, ok := m.rows[key(org, project, name)]
	return st.Sealed, ok, nil
}

func (m *memory) SecretsOf(_ context.Context, org, project string) ([]vault.Stored, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []vault.Stored
	for _, st := range m.rows {
		if st.Org == org && st.Project == project {
			st.Sealed = nil
			out = append(out, st)
		}
	}
	return out, nil
}

func (m *memory) PutSecret(_ context.Context, st vault.Stored) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rows[key(st.Org, st.Project, st.Name)] = st
	return nil
}

func (m *memory) DeleteSecret(_ context.Context, org, project, name string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.rows[key(org, project, name)]
	delete(m.rows, key(org, project, name))
	return ok, nil
}

func (m *memory) AllSecrets(context.Context) ([]vault.Stored, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []vault.Stored
	for _, st := range m.rows {
		out = append(out, st)
	}
	return out, nil
}

func (m *memory) Reseal(_ context.Context, st vault.Stored, was []byte) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := key(st.Org, st.Project, st.Name)
	if !bytes.Equal(m.rows[k].Sealed, was) {
		return false, nil
	}
	m.rows[k] = st
	return true, nil
}

var sealKey = []byte("a-secrets-key-that-is-long-enough-to-seal")

func sealer(t *testing.T, key []byte, retired ...[]byte) *secret.Sealer {
	t.Helper()
	s, err := secret.NewSealer(key, retired...)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func project(org, name string) func() (string, string) {
	return func() (string, string) { return org, name }
}

func editor(org, project string) principal.Principal {
	return principal.Principal{Subject: "u-1", OrgID: org, ProjectID: project,
		ProjectRole: principal.ProjectEditor, Member: true}
}

// defs is a project's running view: one datasource and one Mapbox map that
// names no key, so it reads the default.
type defs struct{}

func (defs) DataSources() []definition.DataSource {
	return []definition.DataSource{
		{Name: "warehouse", Driver: "postgres", DSN: "postgres://r:${secret:warehouse_password}@db/x"},
		{Name: "lake", Driver: "object-store", URI: "s3://lake/", Format: "parquet"},
	}
}

func (defs) Reports() []definition.Report {
	return []definition.Report{{Name: "drops", Outputs: []definition.Output{{
		Name: "screen",
		Layout: []definition.Block{{Map: &definition.MapSpec{
			Basemap: &definition.Basemap{Provider: definition.Mapbox}}}},
	}}}}
}

func TestAStoredSecretResolvesForItsProject(t *testing.T) {
	ctx := context.Background()
	v := vault.New(newMemory(), sealer(t, sealKey), project("acme", "finance"), nil)

	if err := v.Set(ctx, editor("acme", "finance"), "mapbox-token", "pk.abc\n"); err != nil {
		t.Fatal(err)
	}
	got, ok := v.Secret("mapbox-token")
	if !ok || got != "pk.abc" {
		t.Fatalf("Secret = %q, %v — want the value without the pasted newline", got, ok)
	}
}

// The proof the sec profile asks for: two projects, one store, and neither
// reads the other's secret — through the resolver or the listing.
func TestAnotherProjectReadsNoneOfIt(t *testing.T) {
	ctx := context.Background()
	store := newMemory()
	seal := sealer(t, sealKey)
	cache := vault.NewCache(time.Minute, 64)
	finance := vault.New(store, seal, project("acme", "finance"), nil).WithCache(cache)
	sales := vault.New(store, seal, project("acme", "sales"), nil).WithCache(cache)
	globex := vault.New(store, seal, project("globex", "finance"), nil).WithCache(cache)

	if err := finance.Set(ctx, editor("acme", "finance"), "warehouse_password", "hunter2"); err != nil {
		t.Fatal(err)
	}
	// Warmed in the shared cache first, so a cache keyed by name alone would
	// answer the others from it.
	if _, ok := finance.Secret("warehouse_password"); !ok {
		t.Fatal("the project that stored it cannot read it")
	}
	for name, other := range map[string]*vault.Service{"acme/sales": sales, "globex/finance": globex} {
		if v, ok := other.Secret("warehouse_password"); ok {
			t.Errorf("%s read another project's secret: %q", name, v)
		}
	}
	listed, err := sales.List(ctx, editor("acme", "sales"))
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 0 {
		t.Errorf("acme/sales lists another project's secrets: %+v", listed)
	}
}

// A Service handed a principal from somewhere else refuses, rather than
// writing into the project it happens to hold.
func TestOnlyAnEditorOfThisProjectManagesIt(t *testing.T) {
	ctx := context.Background()
	v := vault.New(newMemory(), sealer(t, sealKey), project("acme", "finance"), nil)

	viewer := editor("acme", "finance")
	viewer.ProjectRole = principal.ProjectViewer
	for who, pr := range map[string]principal.Principal{
		"a viewer":               viewer,
		"an editor of elsewhere": editor("acme", "sales"),
	} {
		if err := v.Set(ctx, pr, "x", "y"); !errors.Is(err, vault.ErrForbidden) {
			t.Errorf("%s set a secret: %v", who, err)
		}
		if _, err := v.List(ctx, pr); !errors.Is(err, vault.ErrForbidden) {
			t.Errorf("%s listed secrets: %v", who, err)
		}
		if err := v.Delete(ctx, pr, "x"); !errors.Is(err, vault.ErrForbidden) {
			t.Errorf("%s deleted a secret: %v", who, err)
		}
	}
}

func TestTheProjectsOwnBeatsTheDeployments(t *testing.T) {
	ctx := context.Background()
	shared := secret.Map{"mapbox-token": "pk.everybody", "google-maps-key": "AIza-everybody"}
	v := vault.New(newMemory(), sealer(t, sealKey), project("acme", "finance"), nil).WithShared(shared)

	if err := v.Set(ctx, editor("acme", "finance"), "mapbox-token", "pk.ours"); err != nil {
		t.Fatal(err)
	}
	if got, _ := v.Secret("mapbox-token"); got != "pk.ours" {
		t.Errorf("mapbox-token = %q, want the project's own", got)
	}
	if got, _ := v.Secret("google-maps-key"); got != "AIza-everybody" {
		t.Errorf("google-maps-key = %q, want the deployment's", got)
	}
}

// Settings is where somebody finds out why a map has no basemap and a source
// will not open: every name a definition uses, and what answers it.
func TestTheListSaysWhatIsUsedAndWhatIsMissing(t *testing.T) {
	ctx := context.Background()
	shared := secret.Map{"mapbox-token": "pk.everybody"}
	v := vault.New(newMemory(), sealer(t, sealKey), project("acme", "finance"), nil).
		WithShared(shared).WithDefinitions(defs{})
	if err := v.Set(ctx, editor("acme", "finance"), "spare", "x"); err != nil {
		t.Fatal(err)
	}

	listed, err := v.List(ctx, editor("acme", "finance"))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]vault.Entry{}
	for _, e := range listed {
		got[e.Name] = e
	}
	want := map[string]vault.Source{
		"mapbox-token": vault.FromDeployment, "warehouse_password": vault.Missing, "spare": vault.FromProject,
	}
	for name, source := range want {
		if got[name].Source != source {
			t.Errorf("%s is %q, want %q", name, got[name].Source, source)
		}
	}
	if u := got["mapbox-token"].UsedBy; len(u) != 1 || u[0] != (vault.Use{Kind: "Report", Name: "drops"}) {
		t.Errorf("mapbox-token is used by %+v, want the report whose map names no key", u)
	}
	if u := got["warehouse_password"].UsedBy; len(u) != 1 || u[0].Name != "warehouse" {
		t.Errorf("warehouse_password is used by %+v", u)
	}
	if got["spare"].UpdatedAt == nil || got["spare"].UpdatedBy != "u-1" {
		t.Errorf("a stored secret does not say when or by whom: %+v", got["spare"])
	}
}

// A map resolves its key on every render; the store is asked once, and asked
// again the moment the secret changes.
func TestTheStoreIsAskedOnceUntilTheSecretChanges(t *testing.T) {
	ctx := context.Background()
	store := newMemory()
	v := vault.New(store, sealer(t, sealKey), project("acme", "finance"), nil).
		WithCache(vault.NewCache(time.Minute, 64))
	pr := editor("acme", "finance")
	if err := v.Set(ctx, pr, "mapbox-token", "pk.one"); err != nil {
		t.Fatal(err)
	}

	for range 5 {
		v.Secret("mapbox-token")
		v.Secret("tiles-unset") // absent is remembered too
	}
	if store.reads != 2 {
		t.Errorf("the store was read %d times for two names", store.reads)
	}
	if err := v.Set(ctx, pr, "mapbox-token", "pk.two"); err != nil {
		t.Fatal(err)
	}
	if got, _ := v.Secret("mapbox-token"); got != "pk.two" {
		t.Errorf("after a change the key is %q", got)
	}
}

// Another replica rotates the warehouse password. This one hears about it on
// its next look, drops what it cached and rebuilds the datasource that reads it.
func TestAChangeOnAnotherReplicaIsNoticed(t *testing.T) {
	ctx := context.Background()
	store := newMemory()
	seal := sealer(t, sealKey)
	here := vault.New(store, seal, project("acme", "finance"), nil).
		WithDefinitions(defs{}).WithCache(vault.NewCache(time.Hour, 64))
	there := vault.New(store, seal, project("acme", "finance"), nil)
	pr := editor("acme", "finance")

	var rebuilt []string
	here.OnChange(func(name string, reads []definition.DataSource) {
		for _, ds := range reads {
			rebuilt = append(rebuilt, name+"→"+ds.Name)
		}
	})
	if err := there.Set(ctx, pr, "warehouse_password", "old"); err != nil {
		t.Fatal(err)
	}
	watch(t, here) // the first look only remembers
	if got, _ := here.Secret("warehouse_password"); got != "old" {
		t.Fatalf("before the rotation: %q", got)
	}

	if err := there.Set(ctx, pr, "warehouse_password", "new"); err != nil {
		t.Fatal(err)
	}
	watch(t, here)
	if got, _ := here.Secret("warehouse_password"); got != "new" {
		t.Errorf("after the rotation this replica still answers %q", got)
	}
	if !slices.Equal(rebuilt, []string{"warehouse_password→warehouse"}) {
		t.Errorf("rebuilt %v", rebuilt)
	}
}

// watch runs one look: Watch looks straight away, then waits for a tick that
// the cancelled context means never comes.
func watch(t *testing.T, v *vault.Service) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	v.Watch(ctx, time.Hour)
}

func TestWithoutAKeyNothingIsStoredAndTheDeploymentStillAnswers(t *testing.T) {
	v := vault.New(newMemory(), nil, project("acme", "finance"), nil).
		WithShared(secret.Map{"mapbox-token": "pk.env"})

	err := v.Set(context.Background(), editor("acme", "finance"), "mapbox-token", "pk.x")
	if !errors.Is(err, vault.ErrUnavailable) {
		t.Errorf("stored without a key: %v", err)
	}
	if got, _ := v.Secret("mapbox-token"); got != "pk.env" {
		t.Errorf("mapbox-token = %q, want the environment's", got)
	}
}

func TestWhatCannotBeStoredIsRefused(t *testing.T) {
	ctx := context.Background()
	v := vault.New(newMemory(), sealer(t, sealKey), project("acme", "finance"), nil)
	pr := editor("acme", "finance")

	for _, c := range []struct{ name, value string }{
		{"../etc/passwd", "x"}, {"has space", "x"}, {"..", "x"}, {"", "x"},
		{"ok", ""}, {"ok", "\n"}, {"ok", string(make([]byte, vault.MaxValueBytes+1))},
	} {
		if err := v.Set(ctx, pr, c.name, c.value); !errors.Is(err, vault.ErrInvalid) {
			t.Errorf("Set(%q, %d bytes) = %v", c.name, len(c.value), err)
		}
	}
}

func TestAProjectHoldsABoundedNumber(t *testing.T) {
	ctx := context.Background()
	v := vault.New(newMemory(), sealer(t, sealKey), project("acme", "finance"), nil)
	pr := editor("acme", "finance")
	for i := range vault.MaxSecrets {
		if err := v.Set(ctx, pr, fmt.Sprintf("s%d", i), "x"); err != nil {
			t.Fatal(err)
		}
	}
	if err := v.Set(ctx, pr, "one-more", "x"); !errors.Is(err, vault.ErrFull) {
		t.Errorf("stored past the limit: %v", err)
	}
	// Replacing one it holds is not a new one.
	if err := v.Set(ctx, pr, "s0", "y"); err != nil {
		t.Errorf("replacing at the limit: %v", err)
	}
}

func TestDeletingWhatIsNotThereSaysSo(t *testing.T) {
	v := vault.New(newMemory(), sealer(t, sealKey), project("acme", "finance"), nil)
	err := v.Delete(context.Background(), editor("acme", "finance"), "nothing")
	if !errors.Is(err, vault.ErrNotFound) {
		t.Errorf("Delete = %v", err)
	}
}

// Rotating the key: the new one in, the old one retired. Startup reseals what
// the old one sealed, and names what no key opens.
func TestResealMovesEverythingToTheCurrentKey(t *testing.T) {
	ctx := context.Background()
	store := newMemory()
	old := []byte("the-old-secrets-key-long-enough-to-seal!!")
	before := vault.New(store, sealer(t, old), project("acme", "finance"), nil)
	if err := before.Set(ctx, editor("acme", "finance"), "pw", "hunter2"); err != nil {
		t.Fatal(err)
	}
	stranger := []byte("a-key-nobody-configured-long-enough-too!!")
	lostOne := sealer(t, stranger).Seal("x", "acme", "finance", "orphan")
	_ = store.PutSecret(ctx, vault.Stored{Org: "acme", Project: "finance", Name: "orphan", Sealed: lostOne})

	rotated := sealer(t, sealKey, old)
	resealed, lost, err := vault.Reseal(ctx, store, rotated)
	if err != nil {
		t.Fatal(err)
	}
	if resealed != 1 || !slices.Equal(lost, []string{"acme/finance/orphan"}) {
		t.Errorf("resealed %d, lost %v", resealed, lost)
	}
	// The retired key can go now.
	after := vault.New(store, sealer(t, sealKey), project("acme", "finance"), nil)
	if got, ok := after.Secret("pw"); !ok || got != "hunter2" {
		t.Errorf("after the rotation: %q, %v", got, ok)
	}
}
