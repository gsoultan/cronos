package query

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/principal"
)

/*
The charts that draw how a number spreads — a histogram's bins and a box
plot's quartiles — read it row by row, and every row is read in the database.
What crosses the wire is a bin or a category at a time: a histogram of four
million invoices is twelve numbers, and a box plot of them eight a category.

Only what every dialect here has: FLOOR and CASE for the bins, and window
functions for the ranks. percentile_cont is two dialects' of four.
*/

// Bins is where a histogram's bins fall: the first one's lower edge, how
// wide each is, and how many — the server's arithmetic, from the range the
// histogram's survey read.
type Bins struct {
	Origin, Step float64
	Count        int
}

// rangeSQL is a histogram's survey: how far its number reaches and how many
// rows have one, over the rows the caller may read.
func (b Builder) rangeSQL(ds definition.Dataset, blk definition.Block, inner string) (string, error) {
	x, err := numberColumn(ds, blk.X.Field, "histogram")
	if err != nil {
		return "", err
	}
	sel := []string{"MIN(" + x + ") AS lo", "MAX(" + x + ") AS hi", "COUNT(" + x + ") AS n"}
	return b.wrap(sel, inner, blk, nil, orderedBy{})
}

// BuildHistogram compiles a histogram's bins: each row's bin worked out in
// the database from where the bins fall, and each bin's rows counted — or y
// folded, where the chart names a measure. Over the same scoped rows as
// BuildBlock, so a bin counts nothing the caller could not read.
//
// The edges are the server's numbers, written as literals as a map's grid
// is: computed from what the survey read, never from anything a request
// carries.
func (b Builder) BuildHistogram(ds definition.Dataset, blk definition.Block, in map[string]any,
	f Filters, pr principal.Principal, bins Bins) (Plan, error) {

	if bins.Count < 1 || bins.Count > definition.MaxBins || !finite(bins.Origin) ||
		!finite(bins.Step) || bins.Step <= 0 {
		return Plan{}, fmt.Errorf("%w: %d bins of %g from %g", ErrBadTemplate, bins.Count, bins.Step, bins.Origin)
	}
	base, _, err := b.BuildWith(ds, in, f, pr)
	if err != nil {
		return Plan{}, err
	}
	x, err := numberColumn(ds, blk.X.Field, "histogram")
	if err != nil {
		return Plan{}, err
	}
	value, carry, err := binValue(ds, blk)
	if err != nil {
		return Plan{}, err
	}
	at := fmt.Sprintf("SELECT FLOOR((%s - %s) / %s) AS at%s\nFROM (\n%s\n) AS %s%s",
		x, literal(bins.Origin), literal(bins.Step), carry.inner, base.sql, blockAlias,
		whereAll(append(filterOf(blk), x+" IS NOT NULL")))
	// Clamped, both ends: the largest value lands on the last bin's upper
	// edge, and floating point can put the smallest a hair below the first.
	last := strconv.Itoa(bins.Count - 1)
	binned := fmt.Sprintf("SELECT CASE WHEN f.at < 0 THEN 0 WHEN f.at > %[1]s THEN %[1]s "+
		"ELSE f.at END AS bin%[2]s\nFROM (\n%[3]s\n) AS f", last, carry.middle, at)
	top, tail := b.dialect.Limit(definition.MaxBins + 1)
	return Plan{sql: fmt.Sprintf("SELECT %sh.bin AS bin, %s AS value\nFROM (\n%s\n) AS h\n"+
		"GROUP BY h.bin\nORDER BY 1%s", top, value, binned, tail), args: base.args}, nil
}

// carried is a measure taken through a histogram's nested selects to the
// level that folds it.
type carried struct{ inner, middle string }

// binValue is what a bin says: how many rows, or y folded over them.
func binValue(ds definition.Dataset, blk definition.Block) (string, carried, error) {
	if blk.Y.Field == "" {
		return "COUNT(*)", carried{}, nil
	}
	fn, err := aggregateOf(ds, blk.Y)
	if err != nil {
		return "", carried{}, err
	}
	y, err := column(ds, blk.Y.Field)
	if err != nil {
		return "", carried{}, err
	}
	return fn + "(h.y)", carried{inner: ", " + y + " AS y", middle: ", f.y AS y"}, nil
}

// boxSQL reads a box plot: for each category of x, or once for the whole set,
// how many rows, the quartiles and median, the whiskers and how many rows lie
// beyond them.
//
// A quartile is the smallest value whose rank reaches its share of the count
// — the nearest-rank definition, which needs only a rank — and the median is
// the mean of the middle two. The whiskers reach the furthest values inside
// one and a half interquartile ranges of the box, Tukey's fences, which needs
// the quartiles first: so three levels, the rows ranked, each row told its
// category's quartiles, and each category folded.
func (b Builder) boxSQL(ds definition.Dataset, blk definition.Block, inner string) (string, error) {
	v, err := numberColumn(ds, blk.Y.Field, "box plot")
	if err != nil {
		return "", err
	}
	x, err := b.boxBucket(ds, blk)
	if err != nil {
		return "", err
	}
	part, parted, bucket, group := "", "", "", ""
	if x != "" {
		part, parted = "PARTITION BY "+x+" ", "PARTITION BY r.bucket"
		bucket, group = x+" AS bucket, ", "\nGROUP BY q.bucket\nORDER BY 1"
	}
	ranked := fmt.Sprintf("SELECT %s%s AS v, ROW_NUMBER() OVER (%sORDER BY %s) AS i, "+
		"COUNT(*) OVER (%s) AS n\nFROM (\n%s\n) AS %s%s", bucket, v, part, v, strings.TrimSpace(part),
		inner, blockAlias, whereAll(append(filterOf(blk), v+" IS NOT NULL")))
	quart := func(k string) string {
		return fmt.Sprintf("MIN(CASE WHEN r.i * %s THEN r.v END) OVER (%s)", k, parted)
	}
	carry := ""
	if x != "" {
		carry = "r.bucket AS bucket, "
	}
	// The median is the mean of the two middle ranks — the same rank twice
	// when the count is odd — rather than the lower of them: nearest rank,
	// the median of 300 and 1,200 was 300.
	med := "(" + quart("2 >= r.n") + " + " + quart("2 >= r.n + 1") + ") / 2.0"
	quartered := fmt.Sprintf("SELECT %sr.v AS v, r.n AS n, %s AS q1, %s AS med, %s AS q3\nFROM (\n%s\n) AS r",
		carry, quart("4 >= r.n"), med, quart("4 >= r.n * 3"), ranked)
	return b.boxFold(x != "", quartered, group), nil
}

// boxFold folds each category's rows, told their quartiles, to its box.
func (b Builder) boxFold(bucketed bool, quartered, group string) string {
	fence := "1.5 * (q.q3 - q.q1)"
	sel := []string{"MIN(q.n) AS n",
		"MIN(CASE WHEN q.v >= q.q1 - " + fence + " THEN q.v END) AS low",
		"MIN(q.q1) AS q1", "MIN(q.med) AS median", "MIN(q.q3) AS q3",
		"MAX(CASE WHEN q.v <= q.q3 + " + fence + " THEN q.v END) AS high",
		"SUM(CASE WHEN q.v < q.q1 - " + fence + " OR q.v > q.q3 + " + fence + " THEN 1 ELSE 0 END) AS outliers"}
	if bucketed {
		sel = append([]string{"q.bucket AS bucket"}, sel...)
	}
	top, tail := b.dialect.Limit(ChartLimit)
	return fmt.Sprintf("SELECT %s%s\nFROM (\n%s\n) AS q%s%s", top, strings.Join(sel, ", "),
		quartered, group, tail)
}

// boxBucket is what a box plot draws a box per: x, bucketed by its grain.
func (b Builder) boxBucket(ds definition.Dataset, blk definition.Block) (string, error) {
	if blk.X.Field == "" {
		return "", nil
	}
	x, err := column(ds, blk.X.Field)
	if err != nil || blk.X.Grain == "" {
		return x, err
	}
	return b.dialect.Bucket(blk.X.Grain, x)
}

// numberColumn is a field a chart reads row by row as a number, refused
// where the dataset says it is something else — binning a name is a cast the
// database would report as its own error, about a query nobody wrote.
func numberColumn(ds definition.Dataset, name, chart string) (string, error) {
	col, err := column(ds, name)
	if err != nil {
		return "", err
	}
	f, _ := ds.Field(name)
	switch strings.ToLower(f.Type) {
	case "string", "text", "enum", "date", "datetime", "timestamp", "time", "bool", "boolean":
		return "", fmt.Errorf("%w: a %s reads a number, and %q is a %s", ErrBadTemplate, chart, name, f.Type)
	}
	return col, nil
}

// filterOf is a block's own filter as a condition to join others with.
func filterOf(blk definition.Block) []string {
	if strings.TrimSpace(blk.Filter) == "" {
		return nil
	}
	return []string{"(" + blk.Filter + ")"}
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
