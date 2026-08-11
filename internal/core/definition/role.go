package definition

// Role says whether a field is something to group by or something to
// aggregate. It is the distinction a report builder needs in order to stop
// offering "sum of customer name".
type Role string

const (
	Dimension Role = "dimension"
	Measure   Role = "measure"
)

// Valid reports whether r is a known role.
func (r Role) Valid() bool { return r == Dimension || r == Measure }
