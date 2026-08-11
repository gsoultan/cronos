package schedule

import (
	"testing"
	"time"
)

func TestAlertsAreSentOnEveryChangeOfState(t *testing.T) {
	a := newAlerts()
	now := time.Date(2026, 8, 1, 6, 0, 0, 0, time.UTC)

	send, recovered := a.should("monthly", true, now)
	if !send || recovered {
		t.Errorf("the first failure should alert: send=%v recovered=%v", send, recovered)
	}

	// A pager that only ever fires is one people mute.
	send, recovered = a.should("monthly", false, now.Add(time.Minute))
	if !send || !recovered {
		t.Errorf("recovery should alert: send=%v recovered=%v", send, recovered)
	}

	// And a run that keeps working says nothing at all.
	if send, _ := a.should("monthly", false, now.Add(2*time.Minute)); send {
		t.Error("a working schedule alerted")
	}
}

// A schedule failing every minute produces an alert every minute, and a
// mailbox with four hundred identical alerts is one where the next is unread.
func TestAContinuingFailureIsRepeatedAtMostOncePerInterval(t *testing.T) {
	a := newAlerts()
	now := time.Date(2026, 8, 1, 6, 0, 0, 0, time.UTC)

	if send, _ := a.should("monthly", true, now); !send {
		t.Fatal("the first should send")
	}
	for i := 1; i < 30; i++ {
		if send, _ := a.should("monthly", true, now.Add(time.Duration(i)*time.Minute)); send {
			t.Fatalf("it alerted again after %d minutes", i)
		}
	}
	if send, _ := a.should("monthly", true, now.Add(AlertInterval)); !send {
		t.Error("it went quiet forever")
	}
}

// One schedule failing must not suppress another's alert.
func TestSchedulesAreTrackedSeparately(t *testing.T) {
	a := newAlerts()
	now := time.Date(2026, 8, 1, 6, 0, 0, 0, time.UTC)

	if send, _ := a.should("monthly", true, now); !send {
		t.Fatal("monthly should alert")
	}
	if send, _ := a.should("weekly", true, now); !send {
		t.Error("weekly was suppressed by monthly's alert")
	}
}

// A failure after a recovery is news again, whatever the interval says.
func TestFailingAgainAfterRecoveryAlertsImmediately(t *testing.T) {
	a := newAlerts()
	now := time.Date(2026, 8, 1, 6, 0, 0, 0, time.UTC)

	a.should("monthly", true, now)
	a.should("monthly", false, now.Add(time.Minute))

	if send, recovered := a.should("monthly", true, now.Add(2*time.Minute)); !send || recovered {
		t.Errorf("send=%v recovered=%v", send, recovered)
	}
}
