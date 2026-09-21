package publish_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gsoultan/cronos/internal/app/publish"
)

/*
What the running process is told about a change.

Storing a definition is half of publishing it: on a deployment whose store is a
database there is no file to reload, so something has to hand the document to
the running view. That half was wired for a publish and missing for a delete —
a definition removed from the store carried on being served, and a datasource
carried on holding its connection, until somebody restarted the server.

The deployment wiring is in platform/boot, which is also where the second thing
that needs telling lives: the registry of connections. This is the contract
between them — that Delete tells the live view, in the same order Publish does,
and only when the store agreed.
*/

// view records what the running process was told.
type view struct {
	applied   []string
	forgot    []string
	applyErr  error
	forgetErr error
}

func (v *view) Apply(raw []byte) error {
	v.applied = append(v.applied, string(raw))
	return v.applyErr
}

func (v *view) Forget(kind, name string) error {
	v.forgot = append(v.forgot, kind+"/"+name)
	return v.forgetErr
}

const aDataSource = `apiVersion: cronos.dev/v1
kind: DataSource
metadata: {name: warehouse}
spec: {driver: sqlite, dsn: "file:x.db"}
`

// A published datasource reaches the running process, which is what opens the
// connection to it. Stored and not applied is a source the catalogue lists and
// no report can read.
func TestAPublishedDataSourceReachesTheRunningProcess(t *testing.T) {
	store, running := &recorder{}, &view{}
	svc := publish.New(store, nil).WithLive(running)

	if _, err := svc.Publish(context.Background(), []byte(aDataSource), editor); err != nil {
		t.Fatal(err)
	}
	if len(running.applied) != 1 || !strings.Contains(running.applied[0], "warehouse") {
		t.Fatalf("the running process was told %v", running.applied)
	}
}

// A publish the process could not apply is reported rather than answered with
// a 200, which would leave the store and the running view disagreeing with
// nobody told. The same contract the delete below has.
func TestAPublishTheProcessCouldNotApplyIsReported(t *testing.T) {
	store, running := &recorder{}, &view{applyErr: errors.New("the document is not one this build knows")}
	svc := publish.New(store, nil).WithLive(running)

	_, err := svc.Publish(context.Background(), []byte(aDataSource), editor)
	if err == nil || !strings.Contains(err.Error(), "not live") {
		t.Fatalf("got %v, want a publish that says it is not live", err)
	}
}

// And a deleted one is taken back, in the same breath.
func TestADeletedDefinitionIsTakenOutOfTheRunningProcess(t *testing.T) {
	store, running := &recorder{}, &view{}
	svc := publish.New(store, nil).WithCatalog(catalog{}).WithLive(running)

	if err := svc.Delete(context.Background(), editor, "DataSource", "warehouse"); err != nil {
		t.Fatal(err)
	}
	if store.deleted != "DataSource/warehouse" {
		t.Fatalf("the store was told %q", store.deleted)
	}
	if len(running.forgot) != 1 || running.forgot[0] != "DataSource/warehouse" {
		t.Fatalf("the running process was told %v — it goes on serving what was deleted",
			running.forgot)
	}
}

/*
A delete the store refused is not one the process acts on.

The order is the same one Publish uses and for the same reason: forgetting
first would leave a definition the store still holds and the process no longer
serves, which is the disagreement that survives a restart in the worst
direction — it comes back.
*/
func TestADeleteTheStoreRefusedChangesNothingLive(t *testing.T) {
	store, running := &recorder{deleteErr: publish.ErrNotFound}, &view{}
	svc := publish.New(store, nil).WithCatalog(catalog{}).WithLive(running)

	err := svc.Delete(context.Background(), editor, "Report", "billing")
	if !errors.Is(err, publish.ErrNotFound) {
		t.Fatalf("got %v, want the store's error", err)
	}
	if len(running.forgot) != 0 {
		t.Fatalf("a refused delete was applied to the running process: %v", running.forgot)
	}
}

// And a live view that could not take the change back says so, rather than
// reporting a delete that only half happened.
func TestADeleteTheProcessCouldNotApplyIsReported(t *testing.T) {
	store, running := &recorder{}, &view{forgetErr: errors.New("still in use by a render")}
	svc := publish.New(store, nil).WithCatalog(catalog{}).WithLive(running)

	err := svc.Delete(context.Background(), editor, "Report", "billing")
	if err == nil || !strings.Contains(err.Error(), "still live") {
		t.Fatalf("got %v, want a delete that says it is not live", err)
	}
}
