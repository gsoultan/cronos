package jrxml

import "testing"

// TestNames pins the three normalisations, which differ on purpose: a definition
// name and a param name may be rewritten freely, and a field name may only be
// lower-cased because it still has to name a column the query returns.
func TestNames(t *testing.T) {
	t.Run("definition names", func(t *testing.T) {
		for in, want := range map[string]string{
			"Monthly_Invoice_Statement": "monthly-invoice-statement",
			"Sales Summary":             "sales-summary",
			"CustomerAging":             "customer-aging",
			"HTTPStatusReport":          "http-status-report",
			"invoices.v2":               "invoices-v2",
			"2024_sales":                "report-2024-sales",
			"Auftragsübersicht Müller":  "auftragsubersicht-muller",
			"Straße":                    "strasse",
			"København":                 "kobenhavn",
			"---":                       "report",
			"отчёт":                     "report",
		} {
			if got := slugify(in); got != want {
				t.Errorf("slugify(%q) = %q, want %q", in, got, want)
			}
		}
	})

	t.Run("param names may be readable", func(t *testing.T) {
		for in, want := range map[string]string{
			"FROM_DATE":  "from_date",
			"fromDate":   "from_date",
			"CustomerId": "customer_id",
			"p_status":   "p_status",
			"Order By":   "order_by",
		} {
			if got := paramName(in); got != want {
				t.Errorf("paramName(%q) = %q, want %q", in, got, want)
			}
		}
	})

	t.Run("field names may only be lower-cased", func(t *testing.T) {
		// The rule that keeps the emitted field naming a real column.
		cases := []struct {
			in    string
			want  string
			clean bool
		}{
			{"customer_name", "customer_name", true},
			{"CustomerName", "customername", true},
			{"TOTAL", "total", true},
			{"Total Amount", "total_amount", false}, // needed more than case folding
			{"betrag€", "betrag", false},
		}
		for _, c := range cases {
			got, clean := fieldName(c.in)
			if got != c.want || clean != c.clean {
				t.Errorf("fieldName(%q) = %q,%v — want %q,%v", c.in, got, clean, c.want, c.clean)
			}
		}
	})

	t.Run("collisions are kept apart", func(t *testing.T) {
		var u unique
		if got, renamed := u.pick("from_date", '_'); got != "from_date" || renamed {
			t.Errorf("first pick = %q,%v", got, renamed)
		}
		if got, renamed := u.pick("from_date", '_'); got != "from_date_2" || !renamed {
			t.Errorf("second pick = %q,%v, want from_date_2,true", got, renamed)
		}
		if got, _ := u.pick("from_date", '_'); got != "from_date_3" {
			t.Errorf("third pick = %q", got)
		}
	})
}

// TestScanRefs covers the expression scanner, whose one job that matters is
// telling $P{} from $P!{}.
func TestScanRefs(t *testing.T) {
	t.Run("finds each form", func(t *testing.T) {
		got := scanRefs(`$P{a} + $F{b} + $V{c} + $P!{d}`)
		if len(got) != 4 {
			t.Fatalf("found %d refs, want 4: %+v", len(got), got)
		}
		want := []struct {
			sigil  byte
			name   string
			splice bool
		}{{'P', "a", false}, {'F', "b", false}, {'V', "c", false}, {'P', "d", true}}
		for i, w := range want {
			if got[i].sigil != w.sigil || got[i].name != w.name || got[i].splice != w.splice {
				t.Errorf("ref %d = %c/%s/%v, want %c/%s/%v",
					i, got[i].sigil, got[i].name, got[i].splice, w.sigil, w.name, w.splice)
			}
		}
	})

	t.Run("a splice is never read as a bind", func(t *testing.T) {
		// The failure this guards: a scanner that treats the bang as optional
		// silently turns an injection into a bound constant.
		refs := scanRefs(`ORDER BY $P!{col}`)
		if len(refs) != 1 || !refs[0].splice {
			t.Fatalf("$P!{} was not recognised as a splice: %+v", refs)
		}
	})

	t.Run("ignores what is not a reference", func(t *testing.T) {
		for _, s := range []string{`$X{a}`, `$P a`, `$P{unclosed`, `price $ 5`, ``} {
			if got := scanRefs(s); len(got) != 0 {
				t.Errorf("scanRefs(%q) found %+v", s, got)
			}
		}
	})

	t.Run("plainRef is strict", func(t *testing.T) {
		if r, ok := plainRef(`  $F{total}  `); !ok || r.name != "total" {
			t.Errorf("a lone field reference was not read as one: %+v %v", r, ok)
		}
		for _, s := range []string{
			`$F{a} + $F{b}`,
			`"Total: " + $F{a}`,
			`$F{a}.multiply(x)`,
			`$P!{a}`,
			`$F{a} ` + "\n" + `+ 1`,
		} {
			if _, ok := plainRef(s); ok {
				t.Errorf("plainRef(%q) accepted an expression that is not a column", s)
			}
		}
	})
}

// TestJavaDefault covers the defaults worth carrying, and the ones that must not
// be guessed at.
func TestJavaDefault(t *testing.T) {
	carried := map[string]any{
		`new Date()`:            "today",
		`new java.util.Date()`:  "today",
		`"paid"`:                "paid",
		`Boolean.TRUE`:          true,
		`false`:                 false,
		`0`:                     0,
		`-12`:                   -12,
		`100L`:                  100,
		`new BigDecimal("0.5")`: "0.5",
		`Integer.valueOf(7)`:    7,
	}
	for in, want := range carried {
		got, ok := javaDefault(in)
		if !ok || got != want {
			t.Errorf("javaDefault(%q) = %v,%v — want %v,true", in, got, ok, want)
		}
	}

	// A computation is not a default. A default that is quietly wrong is worse
	// than one the author has to supply.
	for _, in := range []string{
		`new SimpleDateFormat("yyyy").format(new Date())`,
		`$P{other}`,
		`"a" + "b"`,
		`Calendar.getInstance().getTime()`,
		``,
	} {
		if got, ok := javaDefault(in); ok {
			t.Errorf("javaDefault(%q) invented the default %v", in, got)
		}
	}
}

// TestPaperAndMargins covers the page translation, which is arithmetic on the
// only unit a .jrxml measures in.
func TestPaperAndMargins(t *testing.T) {
	for _, c := range []struct {
		w, h int
		want string
	}{
		{595, 842, "A4"}, {842, 595, "A4"}, // landscape states the rotated size
		{612, 792, "Letter"}, {612, 1008, "Legal"},
		{842, 1191, "A3"}, {420, 595, "A5"}, {792, 1224, "us-tabloid"},
		{596, 843, "A4"}, // rounding between millimetres and points
		{500, 500, ""},   // not a paper cronos names
	} {
		tr := &translation{doc: document{PageWidth: c.w, PageHeight: c.h}}
		if got := tr.paperSize(); got != c.want {
			t.Errorf("%dx%d = %q, want %q", c.w, c.h, got, c.want)
		}
	}

	for points, want := range map[int]string{20: "7.1mm", 36: "12.7mm", 72: "25.4mm", 0: ""} {
		tr := &translation{doc: document{LeftMargin: points, RightMargin: points,
			TopMargin: points, BottomMargin: points}}
		if got := tr.margins(); got != want {
			t.Errorf("%d points = %q, want %q", points, got, want)
		}
	}
}
