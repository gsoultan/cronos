package query

// Op is a comparison a filter may make.
//
// A closed set, and small. The operator arrives from a caller — it is what the
// filter bar sends — so it is the one part of a predicate that is not authored
// by anyone trusted. Mapping it through a fixed table means a caller chooses
// among comparisons rather than supplying one.
type Op string

const (
	Eq       Op = "eq"
	Ne       Op = "ne"
	Lt       Op = "lt"
	Lte      Op = "lte"
	Gt       Op = "gt"
	Gte      Op = "gte"
	In       Op = "in"
	Between  Op = "between"
	Contains Op = "contains"
	IsNull   Op = "isNull"
	NotNull  Op = "notNull"
)

// sqlOp is the comparison each op writes. Anything absent here cannot be
// expressed, which is the point of the table.
var sqlOp = map[Op]string{
	Eq: "=", Ne: "<>", Lt: "<", Lte: "<=", Gt: ">", Gte: ">=",
}

// arity is how many values each op consumes.
var arity = map[Op]int{
	Eq: 1, Ne: 1, Lt: 1, Lte: 1, Gt: 1, Gte: 1,
	Contains: 1, Between: 2, IsNull: 0, NotNull: 0,
	// In takes any number, checked separately.
	In: -1,
}

// Valid reports whether o is an operator the compiler knows.
func (o Op) Valid() bool {
	_, ok := arity[o]
	return ok
}
