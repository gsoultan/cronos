package boot

import "sync/atomic"

/*
unarmed counts the schedules this process read and will never run.

A schedule that does not arm used to stop the process, so there was nothing to
count: either every schedule worked or nothing served. Now the ones that will
not parse are skipped so that one bad definition cannot take a deployment down,
and the whole risk of that trade is the word "quietly" — a schedule nobody runs
and nobody notices is exactly the six-in-the-morning surprise the old behaviour
was protecting against.

So it is not quiet. Each one is logged at error when it is found, and this is
what makes it alertable afterwards: `cronos_schedules_unarmed` is a number an
operator can put a rule on, and it should be zero.

A process counter rather than per project. It is read once at startup and never
decremented, because nothing re-reads a definition without a restart.
*/
var unarmed atomic.Int64

// Unarmed is how many schedules were read and will never run.
func Unarmed() int64 { return unarmed.Load() }

/*
rejected counts the stored definitions this build would not accept.

Its own number rather than folded into unarmed, because they answer different
questions and have different fixes. An unarmed schedule parsed as a definition
and failed as a schedule; a rejected one did not get that far, so it is not
being served at all — a report that has vanished from the catalogue rather than
one that runs and does not send.

Both should be zero, and both exist so that a deployment which chose to keep
serving rather than refuse to start is still saying what it dropped.
*/
var rejected atomic.Int64

// Rejected is how many stored definitions this build would not accept.
func Rejected() int64 { return rejected.Load() }

/*
unopenable counts the datasources this build could not open.

The third of these, and the third distinct failure: a definition that would not
parse, a schedule that parsed and will not arm, and now a source that is defined
and cannot be reached. Each has its own fix, so each has its own number — the
first is a report missing from the catalogue, the second a send that never
happens, the third a report that is listed and errors when opened.

What they share is why they are counted at all: each used to stop the process,
and each one stopping the process meant the API was down and the definition
could only be removed with a prompt on the database.
*/
var unopenable atomic.Int64

// Unopenable is how many datasources this build could not open.
func Unopenable() int64 { return unopenable.Load() }
