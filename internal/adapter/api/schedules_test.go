package api_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gsoultan/cronos/internal/adapter/api"
	"github.com/gsoultan/cronos/internal/app/schedule"
	"github.com/gsoultan/cronos/internal/core/history"
	"github.com/gsoultan/cronos/internal/core/principal"
)

/*
Running a schedule by hand.

The handler had no tests, and the reason to write them is the answer it gives
when it will not run one. "No such schedule" covered three different
situations, and only two of them are that: a name nobody has, a project that is
not the caller's, and an instance where the scheduler was never armed. The
third is a schedule the caller can see in /v1/catalog and is being told does
not exist.

That matters because the three have different fixes and only one of them is
"check the name". An operator hitting a replica with CRONOS_SCHEDULER unset
gets a 404 about their spelling, and the spelling is right.

The distinction is safe to make here. Refusing to confirm a name protects
against somebody probing another tenant's project — but this caller has already
resolved the project, which means they are in it, and the catalogue has already
told them the schedule exists.
*/

/* -- fakes ---------------------------------------------------------------- */

type firing struct {
	fired string
	err   error
}

func (f *firing) Fire(_ context.Context, name string) error {
	f.fired = name
	return f.err
}

// oneProject answers with the project it was built with, or refuses.
type oneProject struct {
	project *api.Project
	err     error
}

func (p oneProject) Project(context.Context, principal.Principal) (*api.Project, error) {
	return p.project, p.err
}

// signedInAs authenticates every request as pr.
type signedInAs struct{ pr principal.Principal }

func (s signedInAs) Principal(*http.Request) (principal.Principal, bool) {
	return s.pr, s.pr.Subject != ""
}

func editor() principal.Principal {
	return principal.Principal{
		Subject: "u1", OrgID: "acme", ProjectID: "finance",
		ProjectRole: principal.ProjectEditor,
	}
}

func fire(t *testing.T, projects api.Projects, pr principal.Principal) *httptest.ResponseRecorder {
	t.Helper()

	r := httptest.NewRequest(http.MethodPost, "/v1/schedules/monthly/run", nil)
	// The mux would set this from the pattern; nothing here is routed through
	// one, and the handler reads it.
	r.SetPathValue("name", "monthly")

	w := httptest.NewRecorder()
	api.NewSchedules(projects, signedInAs{pr}, quiet()).ServeHTTP(w, r)
	return w
}

/* -- the three refusals, told apart ---------------------------------------- */

/*
An instance with no scheduler says so, rather than denying the schedule.

503 rather than 404: this instance cannot serve the request and another replica
may be able to, which is exactly what the status means. In a fleet where only
some replicas are armed, a load balancer reading it as retryable is right.
*/
func TestAnUnarmedInstanceSaysTheSchedulerIsOffRatherThanDenyingTheSchedule(t *testing.T) {
	// A project that resolves — the caller is in it — with no scheduler.
	w := fire(t, oneProject{project: &api.Project{Fires: nil}}, editor())

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("answered %d, want 503: %s", w.Code, w.Body.String())
	}
	// Named, because the fix is one variable and the operator cannot guess it.
	if !strings.Contains(w.Body.String(), "CRONOS_SCHEDULER") {
		t.Errorf("the message does not say what to set: %s", w.Body.String())
	}
	if strings.Contains(w.Body.String(), "No such schedule") {
		t.Errorf("still denying a schedule the caller can see: %s", w.Body.String())
	}
}

// A project that does not resolve is still a flat 404. That is the case where
// refusing to confirm a name is the point: it is somebody else's project, and
// telling them it exists is telling them something they were not granted.
func TestAProjectThatIsNotTheCallersIsStillDenied(t *testing.T) {
	w := fire(t, oneProject{err: errors.New("no such project")}, editor())

	if w.Code != http.StatusNotFound {
		t.Fatalf("answered %d, want 404", w.Code)
	}
	if !strings.Contains(w.Body.String(), "No such schedule") {
		t.Errorf("body is %s", w.Body.String())
	}
	// And it must not leak that the reason was the project rather than the name.
	if strings.Contains(w.Body.String(), "project") {
		t.Errorf("the refusal names the project: %s", w.Body.String())
	}
}

// A name nobody has, on an armed instance, is the genuine 404.
func TestAScheduleNobodyHasIsNotFound(t *testing.T) {
	w := fire(t, oneProject{project: &api.Project{
		Fires: &firing{err: schedule.ErrNoSchedule},
	}}, editor())

	if w.Code != http.StatusNotFound {
		t.Fatalf("answered %d, want 404: %s", w.Code, w.Body.String())
	}
}

/* -- and the rest of the contract ------------------------------------------ */

func TestRunningASchedulePutsRealDocumentsOutSoAReaderMayNot(t *testing.T) {
	viewer := editor()
	viewer.ProjectRole = principal.ProjectViewer

	if w := fire(t, oneProject{project: &api.Project{Fires: &firing{}}}, viewer); w.Code != http.StatusForbidden {
		t.Fatalf("a viewer got %d, want 403", w.Code)
	}
}

func TestAnUnauthenticatedCallerIsRefusedBeforeAnythingIsLookedUp(t *testing.T) {
	if w := fire(t, oneProject{project: &api.Project{Fires: &firing{}}},
		principal.Principal{}); w.Code != http.StatusUnauthorized {
		t.Fatalf("answered %d, want 401", w.Code)
	}
}

// Already running is a conflict, not a failure: the schedule is fine and the
// answer is to wait.
func TestAScheduleAlreadyRunningIsAConflict(t *testing.T) {
	w := fire(t, oneProject{project: &api.Project{
		Fires: &firing{err: schedule.ErrRunning},
	}}, editor())

	if w.Code != http.StatusConflict {
		t.Fatalf("answered %d, want 409: %s", w.Code, w.Body.String())
	}
}

func TestAnArmedInstanceFiresTheScheduleItWasAsked(t *testing.T) {
	fires := &firing{}
	w := fire(t, oneProject{project: &api.Project{Fires: fires}}, editor())

	if w.Code != http.StatusAccepted && w.Code != http.StatusOK {
		t.Fatalf("answered %d: %s", w.Code, w.Body.String())
	}
	if fires.fired != "monthly" {
		t.Errorf("fired %q, want monthly", fires.fired)
	}
}

// Resuming makes the same distinction, for the same reason. A half-finished
// burst is repaired from the instance that has a scheduler, and an operator
// told "no such run" about a run they are looking at goes hunting for the
// wrong thing.
func TestResumingOnAnUnarmedInstanceSaysSoRatherThanDenyingTheRun(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/v1/runs/r-1/resume", nil)
	r.SetPathValue("id", "r-1")
	w := httptest.NewRecorder()

	api.NewRuns(runsThatHave{}, signedInAs{editor()}, quiet()).
		WithProjects(oneProject{project: &api.Project{Resumes: nil}}).ServeHTTP(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("answered %d, want 503: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "CRONOS_SCHEDULER") {
		t.Errorf("the message does not say what to set: %s", w.Body.String())
	}
}

// runsThatHave is a history holding one run, so the handler gets past its own
// existence check and reaches the scheduler question.
type runsThatHave struct{}

func (runsThatHave) Runs(context.Context, principal.Principal, int) ([]history.Run, error) {
	return nil, nil
}

func (runsThatHave) Run(_ context.Context, _ principal.Principal, id string) (history.Run, []history.Delivery, error) {
	return history.Run{ID: id, Schedule: "monthly"}, nil, nil
}
