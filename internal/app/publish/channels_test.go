package publish_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gsoultan/cronos/internal/app/publish"
)

/*
A schedule may only name a channel the deployment can actually deliver through.

Publishing checks everything else about a schedule — the report exists, the
output exists, the datasets exist and are not row-scoped — and then accepted
any delivery channel at all, because Validate() only asked that `via` was not
empty. The channel was resolved for the first time in the burst, at the hour
the schedule fires, where the failure is Fatal and nobody is watching.

The portal made that reachable in one click: the share panel filters what it
offers by what the deployment has, and the schedule form did not — it offered
Telegram to every deployment, including the ones with no Telegram at all. So
the guard existed on the path where a person is waiting for the answer, and was
missing on the path where the answer arrives at 06:00.

It is not one channel's problem. `via: email` on a deployment with no
CRONOS_SMTP_HOST publishes clean and dies in the burst the same way.
*/

// scheduleVia is the fixture schedule with its delivery channel swapped.
func scheduleVia(name string) string {
	return strings.Replace(schedule, "via: file", "via: "+name, 1)
}

// ready is a publish service that can resolve the schedule's report and
// datasets, with the fixture's row-level security removed so the only thing
// left to refuse is the channel.
func ready(t *testing.T, channels []string) *publish.Service {
	t.Helper()

	s, repo, _ := setup(t)
	s = s.WithReports(repo).WithChannels(channels)
	unscoped := strings.Replace(dataset,
		"  rowLevelSecurity:\n    - predicate: customer_id = {{ .scope.customer_id }}\n", "", 1)
	mustPublish(t, s, unscoped)
	mustPublish(t, s, report)
	return s
}

func TestAScheduleNamingAChannelTheDeploymentHasNotGotIsRefused(t *testing.T) {
	s := ready(t, []string{"file", "s3"})

	_, err := s.Publish(context.Background(), []byte(scheduleVia("telegram")), admin())
	if !errors.Is(err, publish.ErrNoSuchChannel) {
		t.Fatalf("got %v, want ErrNoSuchChannel", err)
	}
	// The list, because "no telegram channel" leaves somebody guessing at the
	// spelling of the one they should have used.
	if !strings.Contains(err.Error(), "file") || !strings.Contains(err.Error(), "s3") {
		t.Errorf("the message should say what this deployment does have: %v", err)
	}
}

// The ordinary case still publishes.
func TestAScheduleNamingAConfiguredChannelPublishes(t *testing.T) {
	s := ready(t, []string{"file", "email"})

	if _, err := s.Publish(context.Background(), []byte(scheduleVia("email")), admin()); err != nil {
		t.Fatalf("a schedule on a configured channel was refused: %v", err)
	}
}

/*
A deployment with nothing configured refuses every delivery rather than
accepting all of them.

This is the case the nil check below must not swallow: no SMTP, no S3 and no
file output is a deployment that cannot deliver, and a schedule that delivers
is broken on it — which is worth saying at publish rather than at 06:00.
*/
func TestADeploymentWithNoChannelsRefusesAScheduleThatDelivers(t *testing.T) {
	s := ready(t, []string{})

	_, err := s.Publish(context.Background(), []byte(scheduleVia("email")), admin())
	if !errors.Is(err, publish.ErrNoSuchChannel) {
		t.Fatalf("got %v, want ErrNoSuchChannel", err)
	}
	if !strings.Contains(err.Error(), "no delivery channels") {
		t.Errorf("the message should say the deployment has none at all: %v", err)
	}
}

/*
And a service nobody told about channels does not refuse anything.

The distinction is nil versus empty, and it carries weight: empty is a
deployment that has none, nil is a caller that did not say. Refusing on nil
would break every embedder that wires publish itself, and would refuse on the
one path — a boot-time channel construction error — where the list is unknown
rather than known to be empty.
*/
func TestAServiceWithNoChannelListDoesNotJudgeTheChannel(t *testing.T) {
	s := ready(t, nil)

	if _, err := s.Publish(context.Background(), []byte(scheduleVia("telegram")), admin()); err != nil {
		t.Fatalf("a service that was never told the channels refused one: %v", err)
	}
}
