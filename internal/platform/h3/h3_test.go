package h3

import (
	"math"
	"testing"
)

/*
The port against H3 itself. Every case below was produced by H3 4.1's own C
library — through its Go transpilation, outside this repository — and the port
has to land in the same cell and put its corners where H3 puts them. Among
them: H3's documented example, both poles, both sides of the antimeridian, and
pentagons at two resolutions, which are where its special cases live.

The same comparison was run over a million places, a quarter of them close to
the twelve pentagons: every cell identical, every corner within a millimetre.
H3's corners and these differ only in the last bits of a longitude near a pole,
where the transpiled library's own sine and arcsine round differently from Go's
and a billionth of a degree is micrometres of ground.
*/

var golden = []struct {
	lat, lon float64
	res      int
	cell     string
	corners  [][2]float64
}{
	{37.3615593, -122.0553238, 7, "87283472bffffff", [][2]float64{{37.341099093236, -122.041561351643}, {37.352896581103, -122.034031719088}, {37.363515223626, -122.042796660949}, {37.362335222444, -122.059091243303}, {37.350537699108, -122.066617540274}, {37.339920212323, -122.057852591376}}},
	{52.37, 4.9, 9, "891969c9b27ffff", [][2]float64{{52.369276201098, 4.897598873768}, {52.367687417887, 4.897920474165}, {52.367217968113, 4.900467123047}, {52.368337299262, 4.902692295047}, {52.369926104492, 4.902370796894}, {52.370395556555, 4.899824024493}}},
	{-33.8688, 151.2093, 5, "85be0e37fffffff", [][2]float64{{-33.931430813748, 151.335062514380}, {-33.836249376339, 151.310386355632}, {-33.806390472679, 151.198943519947}, {-33.871645481119, 151.111995067671}, {-33.966856258340, 151.136450901818}, {-33.996782829142, 151.248075773312}}},
	{0, 0, 0, "8075fffffffffff", [][2]float64{{11.545295975415, -4.013998443470}, {6.270965136276, -13.708146703918}, {-4.467031609785, -11.664747542126}, {-5.889921754314, -0.782839175106}, {3.968796976610, 3.943036155786}}},
	{89.99, 10, 3, "830326fffffffff", [][2]float64{{89.124600079462, 68.571080410256}, {88.973118857553, 106.898569020869}, {89.074149089712, 146.008397243327}, {89.374030166922, -169.590420232619}, {89.722562839413, -89.477055352830}, {89.495702292808, 21.599649137586}}},
	{-89.99, -170, 4, "84f2939ffffffff", [][2]float64{{-89.719853856386, 143.020972566975}, {-89.722562839413, 90.522944647170}, {-89.759257468850, 35.809761402810}, {-89.798933505958, -31.488852671229}, {-89.780266585636, -103.227958204074}, {-89.740561900409, -162.750051834784}}},
	{0.5, 179.999, 8, "887eb1c347fffff", [][2]float64{{0.505378323563, 179.994907573464}, {0.501913053122, 179.992887712426}, {0.497862888712, 179.994825082073}, {0.497277707300, 179.998782449283}, {0.500742937272, -179.999197407786}, {0.504793389137, 179.998865086046}}},
	{0.5, -179.999, 8, "887eb1c345fffff", [][2]float64{{0.500742937272, -179.999197407786}, {0.497277707300, 179.998782449283}, {0.493227393887, -179.999280114265}, {0.492642022998, -179.995322398390}, {0.496107212436, -179.993301973575}, {0.500157813309, -179.995239546515}}},
	{64.7, 10.536, 1, "81083ffffffffff", [][2]float64{{63.327061328018, 4.012620898450}, {61.890838475326, 8.644221197607}, {61.540514600025, 11.080660058482}, {62.889968357963, 15.771773841154}, {63.800792653212, 17.535446308408}, {66.327261733428, 16.433696996747}, {67.351768675236, 14.813658725827}, {67.468427884550, 8.130261032189}, {67.015632628418, 5.239258880169}, {64.525608421968, 3.699933260288}}},
	{-3.2, -80.1, 15, "8f8f2b6e8d95430", [][2]float64{{-3.199991026903, -80.100002740609}, {-3.199995621608, -80.100006113528}, {-3.200000851802, -80.100003773255}, {-3.200001487291, -80.099998060064}, {-3.199996892586, -80.099994687145}, {-3.199991662392, -80.099997027418}}},
	{40.7128, -74.006, 12, "8c2a107289061ff", [][2]float64{{40.712837986390, -74.005922653886}, {40.712816466954, -74.006052735333}, {40.712725490230, -74.006092776076}, {40.712656033079, -74.006002735750}, {40.712677552423, -74.005872654729}, {40.712768529009, -74.005832613608}}},
	{35.6762, 139.6503, 2, "822f5ffffffffff", [][2]float64{{33.846135074995, 141.379662399306}, {35.500079204201, 141.516472005077}, {36.441777650477, 139.948213054686}, {35.714831785315, 138.304265138118}, {34.090249338836, 138.240187841247}, {33.162807817664, 139.748324545009}}},
	{64.70000012793487, 10.53619907546767, 2, "820807fffffffff", [][2]float64{{64.665070305104, 7.646201116757}, {63.519707127446, 9.693860813818}, {63.960727463737, 12.823918552178}, {65.413227948216, 12.930936799121}, {65.870718693911, 9.586063396818}}},
	{64.70000012793487, 10.53619907546767, 7, "870800000ffffff", [][2]float64{{64.696570542951, 10.517022461506}, {64.692091251356, 10.530299119267}, {64.691145543133, 10.537901422786}, {64.695156660720, 10.551972953546}, {64.697954664573, 10.556429042411}, {64.704915152127, 10.551854131806}, {64.707590996493, 10.547002950990}, {64.707880793081, 10.530094201212}, {64.706735249490, 10.522639499645}, {64.699953601977, 10.516774971508}}},
	{23.71792527122296, -67.13232636643566, 2, "824c07fffffffff", [][2]float64{{24.809924112927, -66.496637070969}, {24.601684776960, -68.079781398823}, {23.167939348431, -68.339115902008}, {22.494125223502, -66.942081188500}, {23.501303342310, -65.804016225040}}},
	{23.71792527122296, -67.13232636643566, 7, "874c00000ffffff", [][2]float64{{23.726769697534, -67.131406476378}, {23.723878901184, -67.138644988270}, {23.721459014335, -67.141229928176}, {23.714263325995, -67.140463354226}, {23.711264840460, -67.138748420805}, {23.709708432497, -67.131037190525}, {23.710274904443, -67.127392585934}, {23.716508464866, -67.123392442502}, {23.719857142771, -67.122854420886}, {23.725266571076, -67.128093856655}}},
	{-64.7000001279349, -169.4638009245324, 2, "82ea07fffffffff", [][2]float64{{-63.960727463737, -167.176081447822}, {-63.519707127446, -170.306139186182}, {-64.665070305104, -172.353798883243}, {-65.870718693911, -170.413936603182}, {-65.413227948216, -167.069063200879}}},
	{-64.7000001279349, -169.4638009245324, 7, "87ea00000ffffff", [][2]float64{{-64.692459235283, -169.452810109686}, {-64.692091251356, -169.469700880733}, {-64.693201019299, -169.477181120285}, {-64.699953601977, -169.483225028492}, {-64.703337654528, -169.483068665749}, {-64.707880793081, -169.469905798788}, {-64.708862103834, -169.462323537848}, {-64.704915152127, -169.448145868194}, {-64.702136983696, -169.443621189095}, {-64.695156660720, -169.448027046454}}},
}

func TestEveryPlaceIsInTheCellH3PutsItIn(t *testing.T) {
	for _, g := range golden {
		c := Cell(g.lat, g.lon, g.res)
		if got := String(c); got != g.cell {
			t.Errorf("(%v, %v) at %d: cell %s, H3 says %s", g.lat, g.lon, g.res, got, g.cell)
			continue
		}
		corners := Boundary(c)
		if len(corners) != len(g.corners) {
			t.Errorf("%s: %d corners, H3 draws %d", g.cell, len(corners), len(g.corners))
			continue
		}
		for i, want := range g.corners {
			if d := km(corners[i], want); d > 1e-6 {
				t.Errorf("%s corner %d: %v, H3 puts it at %v, %.9f km away", g.cell, i, corners[i], want, d)
			}
		}
	}
}

func TestACellIdReadsBackAsItself(t *testing.T) {
	for _, g := range golden {
		h, ok := Parse(g.cell)
		if !ok || String(h) != g.cell || Resolution(h) != g.res {
			t.Errorf("%s: read as %x (%v), resolution %d", g.cell, h, ok, Resolution(h))
		}
	}
}

// A warehouse column is somebody's data: anything in it that is not a cell is
// refused, not drawn somewhere.
func TestSomethingThatIsNotACellIsNotOne(t *testing.T) {
	for _, s := range []string{"", "zzz", "0", "ffffffffffffffff", "87283472bffffff; DROP TABLE",
		"87283472bfffff0", "8f28308280f18f2f"} {
		if h, ok := Parse(s); ok {
			t.Errorf("%q read as cell %x", s, h)
		}
	}
	if Boundary(0x87283472bffff00) != nil {
		t.Error("an index with its trailing digits wrong has corners")
	}
	for _, c := range []uint64{Cell(91, 0, 5), Cell(0, 181, 5), Cell(math.NaN(), 0, 5),
		Cell(math.Inf(1), 0, 5), Cell(10, 10, 16), Cell(10, 10, -1)} {
		if c != 0 {
			t.Errorf("a place off the globe, or a resolution H3 lacks, has cell %x", c)
		}
	}
}

func km(a, b [2]float64) float64 {
	r := math.Pi / 180
	h := math.Pow(math.Sin((b[0]-a[0])*r/2), 2) +
		math.Cos(a[0]*r)*math.Cos(b[0]*r)*math.Pow(math.Sin((b[1]-a[1])*r/2), 2)
	return 2 * 6371 * math.Asin(math.Sqrt(math.Min(1, h)))
}

// A cell's parent is the one H3's cellToParent gives: the coarser cell its
// ancestry names. Not always the coarser cell its middle is in — H3's seven
// children only approximate their parent's shape — which is why rolling cells
// up goes by parents, as H3 does, and not by binning their centres again.
var parents = []struct {
	cell   string
	res    int
	parent string
}{
	{"87283472bffffff", 0, "8029fffffffffff"},
	{"87283472bffffff", 1, "81283ffffffffff"},
	{"87283472bffffff", 2, "822837fffffffff"},
	{"87283472bffffff", 3, "832834fffffffff"},
	{"87283472bffffff", 4, "8428347ffffffff"},
	{"87283472bffffff", 5, "85283473fffffff"},
	{"87283472bffffff", 6, "86283472fffffff"},
	{"891969c9b27ffff", 0, "8019fffffffffff"},
	{"891969c9b27ffff", 1, "81197ffffffffff"},
	{"891969c9b27ffff", 2, "82196ffffffffff"},
	{"891969c9b27ffff", 3, "831969fffffffff"},
	{"891969c9b27ffff", 4, "841969dffffffff"},
	{"891969c9b27ffff", 5, "851969cbfffffff"},
	{"891969c9b27ffff", 6, "861969c9fffffff"},
	{"891969c9b27ffff", 7, "871969c9bffffff"},
	{"891969c9b27ffff", 8, "881969c9b3fffff"},
	{"8f8f2b6e8d95430", 0, "808ffffffffffff"},
	{"8f8f2b6e8d95430", 1, "818f3ffffffffff"},
	{"8f8f2b6e8d95430", 2, "828f2ffffffffff"},
	{"8f8f2b6e8d95430", 3, "838f2bfffffffff"},
	{"8f8f2b6e8d95430", 4, "848f2b7ffffffff"},
	{"8f8f2b6e8d95430", 5, "858f2b6ffffffff"},
	{"8f8f2b6e8d95430", 6, "868f2b6efffffff"},
	{"8f8f2b6e8d95430", 7, "878f2b6e8ffffff"},
	{"8f8f2b6e8d95430", 8, "888f2b6e8dfffff"},
	{"8f8f2b6e8d95430", 9, "898f2b6e8dbffff"},
	{"8f8f2b6e8d95430", 10, "8a8f2b6e8d97fff"},
	{"8f8f2b6e8d95430", 11, "8b8f2b6e8d95fff"},
	{"8f8f2b6e8d95430", 12, "8c8f2b6e8d955ff"},
	{"8f8f2b6e8d95430", 13, "8d8f2b6e8d9543f"},
	{"8f8f2b6e8d95430", 14, "8e8f2b6e8d95437"},
	{"8c2a107289061ff", 0, "802bfffffffffff"},
	{"8c2a107289061ff", 1, "812a3ffffffffff"},
	{"8c2a107289061ff", 2, "822a17fffffffff"},
	{"8c2a107289061ff", 3, "832a10fffffffff"},
	{"8c2a107289061ff", 4, "842a107ffffffff"},
	{"8c2a107289061ff", 5, "852a1073fffffff"},
	{"8c2a107289061ff", 6, "862a1072fffffff"},
	{"8c2a107289061ff", 7, "872a10728ffffff"},
	{"8c2a107289061ff", 8, "882a107289fffff"},
	{"8c2a107289061ff", 9, "892a1072893ffff"},
	{"8c2a107289061ff", 10, "8a2a10728907fff"},
	{"8c2a107289061ff", 11, "8b2a10728906fff"},
	{"830326fffffffff", 0, "8003fffffffffff"},
	{"830326fffffffff", 1, "81033ffffffffff"},
	{"830326fffffffff", 2, "820327fffffffff"},
}

func TestACellsParentIsTheOneH3Names(t *testing.T) {
	for _, p := range parents {
		h, _ := Parse(p.cell)
		if got := String(Parent(h, p.res)); got != p.parent {
			t.Errorf("%s at %d: parent %s, H3 names %s", p.cell, p.res, got, p.parent)
		}
		if Parent(h, Resolution(h)+1) != h {
			t.Errorf("%s has a parent finer than itself", p.cell)
		}
	}
}
