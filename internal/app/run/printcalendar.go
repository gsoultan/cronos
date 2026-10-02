package run

import (
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/gsoultan/cronos/internal/core/document"
)

const (
	// calendarLeft is the room a year's name takes on the left.
	calendarLeft = 0.06
	// calendarRows is a year's height in cells: seven days, and room above
	// them for the months' names.
	calendarRows = 8.6
)

// printCalendar draws a calendar as the screen does: a year a block of weeks,
// Monday at the top, each day shaded by the quantile it falls in — a day with
// nothing in it paler than any that has — the months named along the top, and
// the shades keyed underneath.
func printCalendar(c *document.Chart, days []Day) {
	if len(days) == 0 {
		return
	}
	byDate := make(map[string]Day, len(days))
	for _, d := range days {
		byDate[d.Date] = d
	}
	first, err1 := time.Parse(time.DateOnly, days[0].Date)
	last, err2 := time.Parse(time.DateOnly, days[len(days)-1].Date)
	if err1 != nil || err2 != nil {
		return
	}
	years := last.Year() - first.Year() + 1
	row := 1 / (float64(years) * calendarRows)
	c.Height = math.Max(28, math.Min(float64(years)*25, 150))
	for y := range years {
		calendarYear(c, first.Year()+y, (float64(y)*calendarRows+1.4)*row, row, byDate)
	}
	c.Keys = calendarKeys(days)
}

// calendarYear draws one year's days from top, a row a weekday.
func calendarYear(c *document.Chart, year int, top, row float64, byDate map[string]Day) {
	cell := (1 - calendarLeft) / 53
	c.Marks = append(c.Marks, text(0, top+3.5*row, strconv.Itoa(year), "ink", document.StartAnchor, true))
	jan1 := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
	offset := weekday(jan1)
	for d := jan1; d.Year() == year; d = d.AddDate(0, 0, 1) {
		col := float64((d.YearDay() - 1 + offset) / 7)
		x, y := calendarLeft+col*cell, top+float64(weekday(d))*row
		tone := "frame"
		if day, ok := byDate[d.Format(time.DateOnly)]; ok {
			tone = fmt.Sprintf("ramp-%d", min(day.Step, RampSteps-1)+1)
		}
		c.Marks = append(c.Marks, document.Mark{Kind: document.RectMark,
			X: x, Y: y, W: cell * 0.84, H: row * 0.84, Tone: tone})
		if d.Day() == 1 {
			c.Marks = append(c.Marks, text(x, top-0.7*row, d.Format("Jan"), "muted", document.StartAnchor, false))
		}
	}
}

// weekday is a day's row: Monday at the top, Sunday at the bottom.
func weekday(t time.Time) int { return (int(t.Weekday()) + 6) % 7 }

// calendarKeys names each shade by the least and most a day in it measured.
func calendarKeys(days []Day) []document.Key {
	lo, hi := map[int]Day{}, map[int]Day{}
	for _, d := range days {
		if at, ok := lo[d.Step]; !ok || d.Value < at.Value {
			lo[d.Step] = d
		}
		if at, ok := hi[d.Step]; !ok || d.Value > at.Value {
			hi[d.Step] = d
		}
	}
	var out []document.Key
	for s := range RampSteps {
		if from, ok := lo[s]; ok {
			label := from.Formatted
			if to := hi[s]; to.Formatted != from.Formatted {
				label += "–" + to.Formatted
			}
			out = append(out, document.Key{Tone: fmt.Sprintf("ramp-%d", s+1), Label: label})
		}
	}
	return out
}
