package definition

// Schedule is a report, a time, and where the result goes.
//
// The artifact that puts cronos in the path of a recurring business
// obligation: somebody's invoices go out on the first of the month because
// this file says so.
type Schedule struct {
	Name string `json:"name" yaml:"name"`
	// Title is what a person calls it. The name is an identifier — stable,
	// referenced by other definitions, awkward to change once anything points
	// at it — and those are two different jobs for one string to hold.
	Title       string `json:"title,omitempty" yaml:"title,omitempty"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	// Report and Output name what to render.
	Report string `json:"report" yaml:"report"`
	Output string `json:"output" yaml:"output"`
	// Cron and Timezone say when. The timezone is required rather than assumed
	// to be UTC: "the first of the month" is a local claim, and a statement
	// dated an hour early in the wrong month is a support ticket.
	Cron     string `json:"cron" yaml:"cron"`
	Timezone string `json:"timezone" yaml:"timezone"`

	Params    map[string]any `json:"params,omitempty" yaml:"params,omitempty"`
	Burst     *BurstSpec     `json:"burst,omitempty" yaml:"burst,omitempty"`
	Deliver   []DeliverSpec  `json:"deliver" yaml:"deliver"`
	OnFailure FailureSpec    `json:"onFailure,omitzero" yaml:"onFailure,omitempty"`
}

// Bursts reports whether this schedule fans out per row.
func (s Schedule) Bursts() bool { return s.Burst != nil && s.Burst.Over.Dataset != "" }

// Heading is what a catalog puts in the list.
func (s Schedule) Heading() string {
	if s.Title != "" {
		return s.Title
	}
	return s.Name
}
