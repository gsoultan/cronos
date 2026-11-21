package run

import (
	"fmt"
	"math"
	"time"

	"github.com/gsoultan/cronos/internal/core/definition"
)

// Span is one bar of a timeline: what it is, the days it runs between, and
// the series it is coloured by.
//
// From and To are days since 1970 rather than dates, so the bar is placed on
// XAxis by the same arithmetic as any other mark; Start and End are the dates
// as a reader reads them, and Said is the whole of it for a tooltip.
type Span struct {
	Label  string  `json:"label"`
	Series string  `json:"series,omitempty"`
	Slot   int     `json:"slot"`
	From   float64 `json:"from"`
	To     float64 `json:"to"`
	Start  string  `json:"start"`
	End    string  `json:"end"`
	Said   string  `json:"said"`
	// Milestone is a row that starts and ends on one day: a launch, a
	// deadline, a sign-off. Drawn as a point on its day rather than as a bar
	// a day wide, which at a year's scale is a sliver nobody sees.
	Milestone bool `json:"milestone,omitempty"`
}

const day = 24 * time.Hour

// readTimeline reads (label, start, finish[, series]) into one bar a row,
// and the calendar they are read along.
//
// A bar runs to the end of its last day, so a task that starts and finishes
// on the same day is a day long rather than nothing. A row missing either
// date is not a bar anybody can place, and is left out.
func readTimeline(blk definition.Block, rows Rows) ([]Span, *Axis, error) {
	cols := 3
	if blk.Series.Field != "" {
		cols++
	}
	out := []Span{}
	slots := map[string]int{}
	for rows.Next() {
		cells := make([]any, cols)
		into := make([]any, cols)
		for i := range cells {
			into[i] = &cells[i]
		}
		if err := rows.Scan(into...); err != nil {
			return nil, nil, err
		}
		from, ok1 := asTime(cells[1])
		to, ok2 := asTime(cells[2])
		if !ok1 || !ok2 {
			continue
		}
		if to.Before(from) {
			from, to = to, from
		}
		s := Span{Label: cell(cells[0]), From: daysOf(from), To: daysOf(to) + 1,
			Start: from.Format("2 Jan 2006"), End: to.Format("2 Jan 2006")}
		if cols == 4 {
			s.Series = cell(cells[3])
			if _, seen := slots[s.Series]; !seen {
				slots[s.Series] = len(slots)
			}
			s.Slot = slots[s.Series]
		}
		s.Said = said(from, to)
		if daysOf(from) == daysOf(to) {
			s.Milestone, s.Said = true, from.Format("2 Jan 2006")
		}
		out = append(out, s)
	}
	if len(out) == 0 {
		return out, nil, rows.Err()
	}
	lo, hi := out[0].From, out[0].To
	for _, s := range out {
		lo, hi = math.Min(lo, s.From), math.Max(hi, s.To)
	}
	a := calendarAxis(lo, hi)
	return out, &a, rows.Err()
}

func daysOf(t time.Time) float64 {
	y, m, d := t.Date()
	return float64(time.Date(y, m, d, 0, 0, 0, 0, time.UTC).Unix() / 86400)
}

func dateOf(days float64) time.Time { return time.Unix(int64(days)*86400, 0).UTC() }

// said is a bar's dates and how long it runs, in the unit it is read in:
// days under a month, weeks under half a year, months past that.
func said(from, to time.Time) string {
	n := int(to.Sub(from)/day) + 1
	var long string
	switch {
	case n == 1:
		long = "1 day"
	case n < 28:
		long = fmt.Sprintf("%d days", n)
	case n < 182:
		long = fmt.Sprintf("%d weeks", int(math.Round(float64(n)/7)))
	default:
		long = fmt.Sprintf("%d months", int(math.Round(float64(n)/30.44)))
	}
	start := from.Format("2 Jan")
	if from.Year() != to.Year() {
		start = from.Format("2 Jan 2006")
	}
	return fmt.Sprintf("%s – %s · %s", start, to.Format("2 Jan 2006"), long)
}

// calendarAxis is a timeline's scale: from the first of the month the first
// bar starts in to the first of the month after the last ends, ticked on
// months — every one, or every second, third or sixth where a dozen would
// crowd — each named for its month, and for its year in January and at the
// first tick.
func calendarAxis(lo, hi float64) Axis {
	first := dateOf(lo)
	start := time.Date(first.Year(), first.Month(), 1, 0, 0, 0, 0, time.UTC)
	last := dateOf(hi - 1)
	end := time.Date(last.Year(), last.Month()+1, 1, 0, 0, 0, 0, time.UTC)
	months := (end.Year()-start.Year())*12 + int(end.Month()-start.Month())
	every := 1
	for _, e := range []int{1, 2, 3, 6, 12} {
		every = e
		if months/e <= 12 {
			break
		}
	}
	min, max := daysOf(start), daysOf(end)
	out := Axis{Min: min, Max: max}
	for k := 0; k <= months; k += every {
		t := start.AddDate(0, k, 0)
		name := t.Format("Jan")
		if k == 0 || t.Month() == time.January || every == 12 {
			name = t.Format("Jan 2006")
		}
		out.Ticks = append(out.Ticks, Tick{At: (daysOf(t) - min) / (max - min), Label: name})
	}
	return out
}

// todayOn places the present day on a timeline's calendar, at its middle,
// named with its date; none when the calendar does not reach it.
func todayOn(axis Axis, now time.Time) *RefLine {
	d := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	at := daysOf(d) + 0.5
	if at < axis.Min || at > axis.Max {
		return nil
	}
	return &RefLine{Label: "Today", Value: at, Formatted: d.Format("2 Jan")}
}
