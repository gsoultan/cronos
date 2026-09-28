package run

import "math"

/*
ckmeans splits sorted values into at most k groups with the smallest total
squared distance from each group's mean — natural breaks, found exactly by
dynamic programming (Wang and Song's Ckmeans.1d.dp) rather than by the
iterations Jenks' own method approximates them with, so the same values always
break in the same places. It returns the index each group starts at.

O(k·n·log n): each column of the cost table is filled by divide and conquer,
since the best split for a longer prefix never lies left of a shorter one's.
Five thousand values take about a millisecond.
*/
func ckmeans(sorted []float64, k int) []int {
	n := len(sorted)
	if n == 0 || k <= 0 {
		return nil
	}
	k = min(k, distinct(sorted))
	if k == 1 {
		return []int{0}
	}
	t := ckTable{cost: make([][]float64, k), back: make([][]int, k)}
	for c := range t.cost {
		t.cost[c], t.back[c] = make([]float64, n), make([]int, n)
	}
	t.sums, t.squares = prefixSums(sorted)
	for i := 0; i < n; i++ {
		t.cost[0][i] = t.ssq(0, i)
	}
	for c := 1; c < k; c++ {
		lo := c
		if c == k-1 {
			lo = n - 1
		}
		t.column(lo, n-1, c)
	}
	starts := make([]int, k)
	right := n - 1
	for c := k - 1; c >= 0; c-- {
		starts[c] = t.back[c][right]
		right = starts[c] - 1
	}
	return starts
}

// ckTable is the dynamic programme: the least cost of splitting the first
// i+1 values into c+1 groups, and where the last of them starts.
type ckTable struct {
	cost          [][]float64
	back          [][]int
	sums, squares []float64
}

// column fills cost[c][lo..hi], knowing the answers are ordered.
func (t ckTable) column(lo, hi, c int) {
	if lo > hi {
		return
	}
	i := (lo + hi) / 2
	t.cost[c][i], t.back[c][i] = t.cost[c-1][i-1], i
	jlo := max(c, t.back[c-1][i])
	if lo > c {
		jlo = max(jlo, t.back[c][lo-1])
	}
	jhi := i - 1
	if hi < len(t.sums)-1 {
		jhi = min(jhi, t.back[c][hi+1])
	}
	for j := jhi; j >= jlo; j-- {
		sji := t.ssq(j, i)
		if sji+t.cost[c-1][jlo-1] >= t.cost[c][i] {
			break
		}
		if s := t.ssq(jlo, i) + t.cost[c-1][jlo-1]; s < t.cost[c][i] {
			t.cost[c][i], t.back[c][i] = s, jlo
		}
		jlo++
		if s := sji + t.cost[c-1][j-1]; s < t.cost[c][i] {
			t.cost[c][i], t.back[c][i] = s, j
		}
	}
	t.column(lo, i-1, c)
	t.column(i+1, hi, c)
}

// ssq is the squared distance of values j..i from their mean.
func (t ckTable) ssq(j, i int) float64 {
	var s float64
	if j > 0 {
		mean := (t.sums[i] - t.sums[j-1]) / float64(i-j+1)
		s = t.squares[i] - t.squares[j-1] - float64(i-j+1)*mean*mean
	} else {
		s = t.squares[i] - t.sums[i]*t.sums[i]/float64(i+1)
	}
	return math.Max(s, 0)
}

// prefixSums are running sums and sums of squares, shifted by the median so
// that large values do not cancel away their own differences.
func prefixSums(sorted []float64) (sums, squares []float64) {
	shift := sorted[len(sorted)/2]
	sums, squares = make([]float64, len(sorted)), make([]float64, len(sorted))
	var s, q float64
	for i, v := range sorted {
		d := v - shift
		s, q = s+d, q+d*d
		sums[i], squares[i] = s, q
	}
	return sums, squares
}

// distinct is how many different values sorted holds.
func distinct(sorted []float64) int {
	n := 0
	for i, v := range sorted {
		if i == 0 || v != sorted[i-1] {
			n++
		}
	}
	return n
}
