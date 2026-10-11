package run

// fold is one measure folded over some set of rows, kept in a form that can be
// folded again.
//
// A pivot's totals are folds of its cells, and how two cells combine depends on
// the aggregate: sums and counts add, a minimum keeps the smaller, and an
// average cannot be combined from averages at all — which is why it carries the
// sum and the count it was divided from.
type fold struct {
	value      float64
	sum, count float64
	// set says something was folded into it. A pair no row matched and a pair
	// whose measure was null both leave it unset, and print differently: see
	// pivotCell.
	set bool
}

// merge folds o into f by the aggregate the measure was read with.
func (f *fold) merge(o fold, aggregate string) {
	if !o.set {
		return
	}
	if !f.set {
		*f = o
		return
	}
	switch aggregate {
	case "min":
		f.value = min(f.value, o.value)
	case "max":
		f.value = max(f.value, o.value)
	case "avg":
		f.sum, f.count = f.sum+o.sum, f.count+o.count
		if f.count > 0 {
			f.value = f.sum / f.count
		}
	default:
		f.value += o.value
	}
}
