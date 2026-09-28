package h3

// An H3 index as bits, and the walk between an index and the face cell it
// names. Ported from H3's h3Index.c and baseCells.c.

const (
	maxRes         = 15
	numBaseCells   = 122
	modeOffset     = 59
	baseCellOffset = 45
	resOffset      = 52
	cellMode       = 1
	// initIndex is an index with every digit invalid, which the digits a
	// resolution has are then written over.
	initIndex uint64 = 35184372088831
	// maxFaceCoord is the largest res-0 coordinate a face's base cells take.
	maxFaceCoord = 2
)

// baseCellRotation is the base cell at a face's res-0 coordinate, and the
// ccw rotations into that cell's own coordinate system.
type baseCellRotation struct{ baseCell, ccwRot60 int }

// baseCell is where a base cell's coordinates are at home, and, for the
// twelve pentagons, the faces clockwise of them.
type baseCell struct {
	home     faceIJK
	pentagon bool
	cwOffset [2]int
}

func resolution(h uint64) int { return int(h >> resOffset & 15) }
func baseCellOf(h uint64) int { return int(h >> baseCellOffset & 127) }

func digitAt(h uint64, r int) int { return int(h >> ((maxRes - r) * 3) & 7) }

func withDigit(h uint64, r, d int) uint64 {
	shift := (maxRes - r) * 3
	return h&^(7<<shift) | uint64(d)<<shift
}

func isPentagonCell(bc int) bool { return bc >= 0 && bc < numBaseCells && baseCells[bc].pentagon }

// leadingDigit is the first digit that is not the centre, or the centre.
func leadingDigit(h uint64) int {
	for r := 1; r <= resolution(h); r++ {
		if d := digitAt(h, r); d != centerDigit {
			return d
		}
	}
	return centerDigit
}

func rotate60ccw(h uint64) uint64 {
	for r := 1; r <= resolution(h); r++ {
		h = withDigit(h, r, rotateDigit60ccw(digitAt(h, r)))
	}
	return h
}

func rotate60cw(h uint64) uint64 {
	for r := 1; r <= resolution(h); r++ {
		h = withDigit(h, r, rotateDigit60cw(digitAt(h, r)))
	}
	return h
}

// rotatePent60ccw turns a pentagon's index, stepping over the k-axis
// sequence a pentagon does not have.
func rotatePent60ccw(h uint64) uint64 {
	found := false
	for r := 1; r <= resolution(h); r++ {
		h = withDigit(h, r, rotateDigit60ccw(digitAt(h, r)))
		if !found && digitAt(h, r) != centerDigit {
			found = true
			if leadingDigit(h) == kAxesDigit {
				h = rotate60ccw(h)
			}
		}
	}
	return h
}

// faceIJKToH3 is the index of a cell given as a face coordinate.
func faceIJKToH3(f faceIJK, res int) uint64 {
	h := initIndex&^(15<<modeOffset) | cellMode<<modeOffset
	h = h&^(15<<resOffset) | uint64(res)<<resOffset
	c := f.coord
	for r := res - 1; r >= 0; r-- {
		last := c
		var centre coordIJK
		if classIII(r + 1) {
			c = c.upAp7()
			centre = c.downAp7()
		} else {
			c = c.upAp7r()
			centre = c.downAp7r()
		}
		h = withDigit(h, r+1, last.sub(centre).normalize().digit())
	}
	if c.i > maxFaceCoord || c.j > maxFaceCoord || c.k > maxFaceCoord {
		return 0
	}
	rot := faceBaseCells[f.face*27+c.i*9+c.j*3+c.k]
	h = h&^(127<<baseCellOffset) | uint64(rot.baseCell)<<baseCellOffset
	if !isPentagonCell(rot.baseCell) {
		for i := 0; i < rot.ccwRot60; i++ {
			h = rotate60ccw(h)
		}
		return h
	}
	// Out of the k-axis sub-sequence a pentagon does not have.
	if leadingDigit(h) == kAxesDigit {
		if cw := baseCells[rot.baseCell].cwOffset; cw[0] == f.face || cw[1] == f.face {
			h = rotate60cw(h)
		} else {
			h = rotate60ccw(h)
		}
	}
	for i := 0; i < rot.ccwRot60; i++ {
		h = rotatePent60ccw(h)
	}
	return h
}

// h3ToFaceIJK is the face coordinate of a cell: from its base cell's home,
// down its digits, and onto whichever face that walk ends on.
func h3ToFaceIJK(h uint64) faceIJK {
	bc := baseCellOf(h)
	if isPentagonCell(bc) && leadingDigit(h) == ikAxesDigit {
		h = rotate60cw(h)
	}
	f := baseCells[bc].home
	res := resolution(h)
	possibleOverage := isPentagonCell(bc) || (res != 0 && f.coord != coordIJK{})
	for r := 1; r <= res; r++ {
		if classIII(r) {
			f.coord = f.coord.downAp7()
		} else {
			f.coord = f.coord.downAp7r()
		}
		f.coord = f.coord.neighbor(digitAt(h, r))
	}
	if !possibleOverage {
		return f
	}
	orig := f.coord
	if classIII(res) {
		f.coord = f.coord.downAp7r()
		res++
	}
	pentLeading4 := isPentagonCell(bc) && leadingDigit(h) == iAxesDigit
	if adjustOverageClassII(&f, res, pentLeading4, false) != noOverage {
		if isPentagonCell(bc) {
			for adjustOverageClassII(&f, res, false, false) != noOverage {
			}
		}
		if res != resolution(h) {
			f.coord = f.coord.upAp7r()
		}
	} else if res != resolution(h) {
		f.coord = orig
	}
	return f
}

// valid reports whether h is a cell index H3 could have produced.
func valid(h uint64) bool {
	if h>>63 != 0 || int(h>>modeOffset&15) != cellMode || h>>56&7 != 0 {
		return false
	}
	bc, res := baseCellOf(h), resolution(h)
	if bc >= numBaseCells {
		return false
	}
	found := false
	for r := 1; r <= res; r++ {
		d := digitAt(h, r)
		if !found && d != centerDigit {
			found = true
			if isPentagonCell(bc) && d == kAxesDigit {
				return false
			}
		}
		if d >= invalidDigit {
			return false
		}
	}
	for r := res + 1; r <= maxRes; r++ {
		if digitAt(h, r) != invalidDigit {
			return false
		}
	}
	return true
}

// pentagon reports whether a cell is one of the twelve at each resolution.
func pentagon(h uint64) bool {
	return isPentagonCell(baseCellOf(h)) && leadingDigit(h) == centerDigit
}
