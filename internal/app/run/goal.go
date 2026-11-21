package run

import (
	"math"
	"strconv"

	"github.com/gsoultan/cronos/internal/core/definition"
)

// Goal is a stat's figure against what it is meant to reach: the target,
// written in the measure's format, and how far along the figure is.
type Goal struct {
	Label string `json:"label"`
	// Value is the target, formatted.
	Value string `json:"value"`
	// Share is the figure over the target: 0.79 is 79% of the way, and past
	// 1 is past it. Sent as the fraction so a bar can be drawn from it; Said
	// is the same fraction as the words beside the bar.
	Share float64 `json:"share"`
	Said  string  `json:"said"`

	target float64
}

// goalOf sets a stat's figure against its target. None where there is no
// target, no figure, or a target of nothing, which nothing is a share of.
func goalOf(t definition.Target, figure, target *float64) *Goal {
	if !t.Set() || figure == nil || target == nil || *target == 0 {
		return nil
	}
	share := *figure / *target
	return &Goal{
		Label: t.Heading(), Value: compact(*target),
		Share: math.Round(share*1e4) / 1e4, Said: percentOf(share),
		target: *target,
	}
}

// percentOf is a share in whole percent, or to one place below ten, where a
// whole percent rounds a third of the bar away.
func percentOf(share float64) string {
	p := share * 100
	if math.Abs(p) < 10 {
		return strconv.FormatFloat(math.Round(p*10)/10, 'f', -1, 64) + "%"
	}
	return strconv.FormatFloat(math.Round(p), 'f', -1, 64) + "%"
}
