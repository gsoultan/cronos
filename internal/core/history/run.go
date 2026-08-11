package history

import "time"

// Run is one execution of a schedule.
type Run struct {
	ID string `json:"id"`
	// Org and Project scope it. Every read carries them; see the store.
	Org     string `json:"org"`
	Project string `json:"project"`

	Schedule string `json:"schedule"`
	Report   string `json:"report"`
	// ReportVersion is the content address of the definition that ran.
	//
	// The whole point. "This report" changes; this version does not, so a run
	// naming one can be replayed against exactly what produced it.
	ReportVersion string `json:"reportVersion,omitempty"`
	Output        string `json:"output"`

	PeriodStart string `json:"periodStart,omitempty"`
	PeriodEnd   string `json:"periodEnd,omitempty"`
	// TriggeredBy is the principal it ran as.
	TriggeredBy string `json:"triggeredBy"`

	StartedAt  time.Time  `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`

	Recipients int    `json:"recipients"`
	Delivered  int    `json:"delivered"`
	Status     Status `json:"status"`
	// Error is what stopped the run, when something did. Deliveries that
	// failed individually are recorded against themselves, not here.
	Error string `json:"error,omitempty"`
}

// Took is how long the run lasted, or zero while it is still going.
func (r Run) Took() time.Duration {
	if r.FinishedAt == nil {
		return 0
	}
	return r.FinishedAt.Sub(r.StartedAt)
}
