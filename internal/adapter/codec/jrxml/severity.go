package jrxml

// Severity grades a finding by what somebody has to do about it.
//
// Three levels, because an estate is triaged in three passes and a finer scale
// would only make the sort order arbitrary. The question each answers is "does
// this file need a person?", and they are ordered so that the worst sorts first.
type Severity int

const (
	// Note is appearance. A font, a colour, a border, a pixel position — the
	// things nobody expected a semantic format to keep. Reported so the list is
	// complete, and expected to be ignored.
	Note Severity = iota
	// Review is meaning that changed or went missing while the import still
	// produced something. A dropped subreport, a chart drawn as a different
	// kind, a computed column left out. The definition works; it is not yet the
	// report it was.
	Review
	// Blocked is a file that produced no usable definition. Someone has to open
	// it. Counted separately because it is the only number that decides whether
	// a migration is finished.
	Blocked
)

// String is what the CLI prints and what the tests assert on.
func (s Severity) String() string {
	switch s {
	case Blocked:
		return "blocked"
	case Review:
		return "review"
	case Note:
		return "note"
	}
	return "unknown"
}

// worseThan orders findings so the ones needing a person come first.
func (s Severity) worseThan(other Severity) bool { return s > other }
