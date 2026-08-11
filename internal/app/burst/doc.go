// Package burst renders one document per recipient and delivers each.
//
// This is the job cronos exists to be in the path of: on the first of the
// month, five thousand customers each receive a statement that reads as though
// it were produced for them alone.
//
// # The unit of work is one recipient
//
// A burst is not one big document split up. Each row of the `over` dataset
// becomes its own render, with the row's values bound to the report's
// parameters, so peak memory is the largest single recipient rather than the
// run — which is why five thousand statements do not need a browser farm.
//
// # It runs as the schedule's owner
//
// Every row's query still applies the dataset's row-level security. A schedule
// cannot be used to widen access: it is a way to run a report on a timer, not
// a way to run it as somebody else. See docs/tenancy.md.
package burst
