package yaml

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/gsoultan/cronos/internal/core/definition"
)

// TestRoundTrip is the property that matters: anything Encoder writes, Loader
// reads back as the same value.
//
// Written as a round trip rather than a golden file because the golden file
// tests yaml.v3's formatting and this tests the contract — the importer's whole
// output is only useful if it loads.
func TestRoundTrip(t *testing.T) {
	var (
		enc Encoder
		ldr Loader
	)

	t.Run("dataset", func(t *testing.T) {
		want := definition.Dataset{
			Name:        "invoices",
			Title:       "Invoices",
			Description: "Issued invoices.",
			Sources:     []definition.SourceRef{{Ref: "warehouse"}, {Ref: "events-lake", As: "events"}},
			Query:       "SELECT id, total FROM invoices\nWHERE issued_at >= {{ .params.from }}\n",
			Params: []definition.Param{
				{Name: "from", Type: definition.Date, Required: true, Label: "From", Default: "today"},
				{Name: "status", Type: definition.Enum, Values: []string{"paid", "overdue"}, Multiple: true},
			},
			Fields: []definition.Field{
				{Name: "id", Type: "string", Role: definition.Dimension, Hidden: true},
				{Name: "currency", Type: "string", Role: definition.Dimension},
				{Name: "total", Type: "decimal", Role: definition.Measure, Aggregate: "sum",
					Label: "Amount", Format: "currency", CurrencyField: "currency"},
			},
			RowLevelSecurity: []definition.RowScope{{Predicate: "customer_id = {{ .scope.customer_id }}"}},
		}

		raw, err := enc.Dataset(want)
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		got, err := ldr.Dataset(raw)
		if err != nil {
			t.Fatalf("load back: %v\n%s", err, raw)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("round trip changed the dataset\n got %+v\nwant %+v\n\n%s", got, want, raw)
		}
	})

	t.Run("report", func(t *testing.T) {
		want := definition.Report{
			Name:    "monthly-statement",
			Title:   "Monthly statement",
			Folder:  "/finance/statements",
			Dataset: "invoices",
			Params:  map[string]definition.ParamOverride{"status": {Pin: true}},
			Filters: []definition.Filter{
				{Name: "period", Label: "Period", Type: definition.Date,
					Bind: map[string]string{"invoices": "issued_at"}},
			},
			Outputs: []definition.Output{
				{
					Name: "interactive", Renderer: definition.Interactive,
					Layout: []definition.Block{
						{Kind: definition.StatBlock, Label: "Billed",
							Value: definition.MeasureRef{Field: "total", Aggregate: "sum"}},
						{Kind: definition.ChartBlock, Chart: "bar", Title: "By month",
							X: definition.DimensionRef{Field: "issued_at", Grain: "month"},
							Y: definition.MeasureRef{Field: "total", Aggregate: "sum"}},
						{Kind: definition.TableBlock, Columns: []string{"issued_at", "total"},
							Sort: []definition.SortKey{{Field: "issued_at", Dir: "desc"}}, PageSize: 50},
					},
				},
				{
					Name: "pdf", Renderer: definition.Paginated,
					Page:   definition.PageSpec{Size: "A4", Orientation: "portrait", Margins: "20mm"},
					Header: definition.Furniture{Template: "templates/letterhead.typ"},
					Footer: definition.Furniture{Text: "Page {{ .page }} of {{ .pages }}"},
					Layout: []definition.Block{
						{Kind: definition.TextBlock, Style: "h1", Text: "Statement"},
						{Kind: definition.TableBlock, Columns: []string{"issued_at", "total"},
							GroupBy: "customer_name", PageBreak: "perGroup", Subtotals: []string{"total"}},
					},
				},
				{
					Name: "xlsx", Renderer: definition.Spreadsheet,
					Sheets: []definition.Sheet{
						{Name: "Invoices", Columns: []string{"issued_at", "total"},
							FreezeHeader: true, AutoFilter: true},
					},
				},
			},
		}

		raw, err := enc.Report(want)
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		got, err := ldr.Report(raw)
		if err != nil {
			t.Fatalf("load back: %v\n%s", err, raw)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("round trip changed the report\n got %+v\nwant %+v\n\n%s", got, want, raw)
		}
	})

	t.Run("datasource", func(t *testing.T) {
		want := definition.DataSource{
			Name:   "warehouse",
			Driver: "postgres",
			DSN:    "${secret:warehouse_dsn}",
			Labels: map[string]string{"team": "finance"},
			Pool:   definition.Pool{MaxOpen: 20},
			Limits: definition.Limits{MaxRows: 1_000_000},
		}
		raw, err := enc.DataSource(want)
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		got, err := ldr.DataSource(raw)
		if err != nil {
			t.Fatalf("load back: %v\n%s", err, raw)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("round trip changed the datasource\n got %+v\nwant %+v\n\n%s", got, want, raw)
		}
	})

	t.Run("schedule", func(t *testing.T) {
		want := definition.Schedule{
			Name:     "monthly-statements",
			Report:   "monthly-statement",
			Output:   "pdf",
			Cron:     "0 6 1 * *",
			Timezone: "Europe/Berlin",
			Burst: &definition.BurstSpec{
				Over:        definition.OverSpec{Dataset: "active-customers"},
				Bind:        map[string]string{"customer_id": "{{ .row.id }}"},
				Concurrency: 8,
			},
			Deliver: []definition.DeliverSpec{
				{Via: "email", To: "{{ .row.billing_email }}", Subject: "Your statement",
					Attach: definition.AttachSpec{Filename: "statement.pdf"}},
			},
			OnFailure: definition.FailureSpec{Retries: 3, Backoff: "exponential"},
		}
		raw, err := enc.Schedule(want)
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		got, err := ldr.Schedule(raw)
		if err != nil {
			t.Fatalf("load back: %v\n%s", err, raw)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("round trip changed the schedule\n got %+v\nwant %+v\n\n%s", got, want, raw)
		}
	})
}

// TestEncodesTheEnvelope pins the parts a person reads: a definitions directory
// is a git repository, and a file whose first lines move on every save produces
// a diff nobody can review.
func TestEncodesTheEnvelope(t *testing.T) {
	raw, err := Encoder{}.Dataset(minimalDataset())
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)

	head := "apiVersion: cronos.dev/v1\nkind: Dataset\nmetadata:\n  name: invoices\nspec:\n"
	if !strings.HasPrefix(got, head) {
		t.Errorf("envelope is not the shape the examples use:\n%s", got)
	}
	// Two-space indent, not yaml.v3's default four.
	if !strings.Contains(got, "\n  sources:\n    - ref: warehouse\n") {
		t.Errorf("spec is not indented by %d:\n%s", Indent, got)
	}
	// The name lives in metadata. Written in both places, strict decoding would
	// refuse the document the encoder just produced.
	_, spec, _ := strings.Cut(got, "\nspec:\n")
	if strings.Contains(spec, "  name: invoices") {
		t.Errorf("name appears in the spec as well as the metadata:\n%s", got)
	}
	// The SQL is the part a person reviews in a migration diff, so it is a
	// block rather than one quoted line with \n in it.
	if !strings.Contains(got, "query: |") {
		t.Errorf("query is not a literal block:\n%s", got)
	}
}

// TestQueryThatCannotBeABlock covers the fallback: yaml.v3 refuses literal
// style for content it cannot re-read, and a prettier file that means something
// else is the one outcome worse than a quoted line.
func TestQueryThatCannotBeABlock(t *testing.T) {
	ds := minimalDataset()
	// A line ending in a space. A block scalar would silently lose it.
	ds.Query = "SELECT id \nFROM invoices"

	raw, err := Encoder{}.Dataset(ds)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Loader{}.Dataset(raw)
	if err != nil {
		t.Fatalf("load back: %v\n%s", err, raw)
	}
	if got.Query != ds.Query {
		t.Errorf("query changed\n got %q\nwant %q\n\n%s", got.Query, ds.Query, raw)
	}
}

// TestRefusesInvalid is the reason Encoder validates: a file that looks like
// every other one in the directory and fails on load blames the file, not
// whatever wrote it.
func TestRefusesInvalid(t *testing.T) {
	t.Run("dataset", func(t *testing.T) {
		ds := minimalDataset()
		ds.Query = ""
		if _, err := (Encoder{}).Dataset(ds); !errors.Is(err, definition.ErrInvalid) {
			t.Errorf("encoded a dataset with no query: %v", err)
		}
	})
	t.Run("report", func(t *testing.T) {
		if _, err := (Encoder{}).Report(definition.Report{Name: "r", Dataset: "d"}); !errors.Is(err, definition.ErrInvalid) {
			t.Errorf("encoded a report with no outputs: %v", err)
		}
	})
}

func minimalDataset() definition.Dataset {
	return definition.Dataset{
		Name:    "invoices",
		Sources: []definition.SourceRef{{Ref: "warehouse"}},
		Query:   "SELECT id FROM invoices",
		Fields:  []definition.Field{{Name: "id", Type: "string", Role: definition.Dimension}},
	}
}
