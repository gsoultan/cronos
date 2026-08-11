// Package schedule fires bursts when they are due.
//
// # What it deliberately does not do
//
// It does not catch up. A server that was down for a week comes back and runs
// each schedule once, at its next due time — not seven times in a row to make
// up for the ones it missed. Nobody wants seven copies of last week's invoices,
// and the person who would have to explain them is not the one who wrote the
// retry loop.
//
// It does not overlap. A run still going when the next one is due is skipped
// and logged, because two bursts of the same statements racing each other
// deliver each customer two documents that disagree.
//
// # The period comes from the cadence
//
// A schedule does not say what period it covers; it says when it runs. The
// span between the previous firing and this one *is* the period, whatever the
// cron expression happens to be, and that is what {{ .run.periodStart }} and
// {{ .run.periodEnd }} resolve to.
package schedule
