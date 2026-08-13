package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gsoultan/cronos/internal/core/definition"
)

// Catalog is what the project contains, as a browsing interface needs it.
//
// One request for the whole navigation rather than a listing plus a fetch per
// entry. The portal's data page wants a dataset's description, its field count
// and which source it reads; getting there through /v1/definitions would be one
// request to learn the names and one per name to learn anything about them,
// with YAML parsing in a browser at the end of it.
type Catalog struct {
	Sources   []SourceSummary   `json:"sources"`
	Datasets  []DatasetSummary  `json:"datasets"`
	Reports   []ReportSummary   `json:"reports"`
	Schedules []ScheduleSummary `json:"schedules"`
	// Channels are the ways this deployment can deliver something. The share
	// panel offered email and Telegram whatever was configured, so a
	// deployment with neither showed two options that could only fail — and
	// the failure arrived after somebody had typed eight addresses.
	Channels []string `json:"channels"`
}

// SourceSummary is a connection, without its credentials.
//
// No DSN, ever. It holds a password, this response reaches a browser, and
// "the portal shows the connection string" is a sentence that ends in an
// incident report.
type SourceSummary struct {
	Name        string `json:"name"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Driver      string `json:"driver"`
	// Detail is what a person recognises the connection by — a host, a bucket
	// — with everything secret removed.
	Detail    string `json:"detail,omitempty"`
	Datasets  int    `json:"datasets"`
	Federated bool   `json:"federated,omitempty"`
	MaxRows   int    `json:"maxRows"`
	Timeout   string `json:"timeout"`
}

type DatasetSummary struct {
	Name        string   `json:"name"`
	Title       string   `json:"title,omitempty"`
	Description string   `json:"description,omitempty"`
	Sources     []string `json:"sources"`
	Fields      int      `json:"fields"`
	Measures    int      `json:"measures"`
	Params      int      `json:"params"`
	// RowScoped says whether an embedded end customer sees only their rows.
	// The one property of a dataset somebody should not have to open the file
	// to learn.
	RowScoped bool `json:"rowScoped"`
	/*
	   Columns is the field list, for the one caller that needs it.

	   The counts above are what a browsing interface wants; the report builder
	   wants the fields themselves, because every block editor it draws is a
	   choice among them. Until now it had no way to ask, so it read a fixture
	   of two datasets with invented columns — which is why a project holding
	   five offered a choice of two, and why picking one gave you the fixture's
	   columns rather than your own.

	   Carried here rather than behind a second endpoint because the caller has
	   already made this request: a builder that fetched per dataset would fetch
	   on every change of the picker, to draw a control that has to be complete
	   before it is opened.

	   Datasets in a project are tens and their fields are tens, so this is a
	   few kilobytes on a request the portal makes once a minute. A deployment
	   where that is not true has a catalogue nobody can browse either.
	*/
	Columns []Column `json:"columns,omitempty"`
	// Parameters are what a dataset takes, so a report can supply them and a
	// schedule can bind them. Same argument as Columns: the caller choosing
	// them is choosing among a known set.
	Parameters []Parameter `json:"parameters,omitempty"`
}

/*
Column is a field as an interface needs to offer it.

definition.Field with the parts that only mean something to the engine left
out — an aggregate is how a measure is computed, not something to show
somebody choosing what to chart, and CurrencyField names another column rather
than describing this one.
*/
type Column struct {
	Name  string `json:"name"`
	Label string `json:"label,omitempty"`
	Type  string `json:"type"`
	Role  string `json:"role"`
	// Format is how the value is rendered, and matters here because a builder
	// showing "Amount" beside "Days late" should not offer to sum them the
	// same way.
	Format string `json:"format,omitempty"`
	// Hidden fields stay out of a builder and remain available to row scope
	// predicates and joins. Reported rather than dropped, so the one interface
	// that needs them — a filter over an id nobody charts — can have them.
	Hidden bool `json:"hidden,omitempty"`
}

// Parameter is a dataset's input, as something to bind.
type Parameter struct {
	Name     string   `json:"name"`
	Label    string   `json:"label,omitempty"`
	Type     string   `json:"type"`
	Required bool     `json:"required,omitempty"`
	Multiple bool     `json:"multiple,omitempty"`
	Values   []string `json:"values,omitempty"`
	Default  string   `json:"default,omitempty"`
}

type ReportSummary struct {
	Name        string   `json:"name"`
	Title       string   `json:"title,omitempty"`
	Description string   `json:"description,omitempty"`
	Folder      string   `json:"folder,omitempty"`
	Datasets    []string `json:"datasets"`
	Outputs     []string `json:"outputs"`
	Blocks      int      `json:"blocks"`
}

type ScheduleSummary struct {
	Name        string   `json:"name"`
	Title       string   `json:"title,omitempty"`
	Description string   `json:"description,omitempty"`
	Report      string   `json:"report"`
	Output      string   `json:"output"`
	Cron        string   `json:"cron"`
	Timezone    string   `json:"timezone"`
	Bursts      bool     `json:"bursts"`
	Over        string   `json:"over,omitempty"`
	Channels    []string `json:"channels"`
	// Next is when it fires, when a scheduler is armed. Absent otherwise, and
	// the interface says "not scheduled here" rather than showing a time
	// nothing will honour.
	Next *time.Time `json:"next,omitempty"`
}

// Repository is the live view a catalogue is built from.
type Repository interface {
	DataSources() []definition.DataSource
	Datasets() []definition.Dataset
	Reports() []definition.Report
	Schedules() []definition.Schedule
}

// Due reports when armed schedules next fire.
type Due interface {
	Due() map[string]time.Time
}

// CatalogHandler serves it.
type CatalogHandler struct {
	// projects resolves whose catalogue this is, per request.
	projects Projects
	// channels are the delivery channels this deployment has configured.
	channels []string
	auth     Principals
	log      *slog.Logger
}

// NewCatalog wires the handler.
func NewCatalog(projects Projects, a Principals, log *slog.Logger) *CatalogHandler {
	return &CatalogHandler{projects: projects, auth: a, log: log}
}

// WithChannels names the ways this deployment can deliver something, so an
// interface offers what exists rather than what the format supports.
func (c *CatalogHandler) WithChannels(names []string) *CatalogHandler {
	c.channels = names
	return c
}

// ServeHTTP handles GET /v1/catalog.
func (c *CatalogHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	pr, ok := c.auth.Principal(r)
	if !ok {
		fail(w, http.StatusUnauthorized, "Sign in to browse this project.")
		return
	}
	if !pr.CanRead() {
		fail(w, http.StatusForbidden, "You do not have access to this project.")
		return
	}
	if r.Method != http.MethodGet {
		fail(w, http.StatusMethodNotAllowed, "Not a method this endpoint takes.")
		return
	}
	project, err := c.projects.Project(r.Context(), pr)
	if err != nil {
		// The same answer as a project that does not exist. Telling them apart
		// would let somebody enumerate which customers a deployment holds.
		fail(w, http.StatusForbidden, "You do not have access to this project.")
		return
	}
	send(w, http.StatusOK, c.build(project))
}

func (c *CatalogHandler) build(project *Project) Catalog {
	out := Catalog{
		Sources: []SourceSummary{}, Datasets: []DatasetSummary{},
		Reports: []ReportSummary{}, Schedules: []ScheduleSummary{},
		Channels: c.channels,
	}
	if out.Channels == nil {
		out.Channels = []string{}
	}

	readers := map[string]int{}
	for _, ds := range project.Definitions.Datasets() {
		names := make([]string, 0, len(ds.Sources))
		for _, s := range ds.Sources {
			names = append(names, s.Ref)
			readers[s.Ref]++
		}
		measures := 0
		columns := make([]Column, 0, len(ds.Fields))
		for _, f := range ds.Fields {
			if f.Role == definition.Measure {
				measures++
			}
			columns = append(columns, Column{
				Name: f.Name, Label: f.Label, Type: f.Type,
				Role: string(f.Role), Format: f.Format, Hidden: f.Hidden,
			})
		}
		params := make([]Parameter, 0, len(ds.Params))
		for _, p := range ds.Params {
			params = append(params, Parameter{
				Name: p.Name, Label: p.Label, Type: string(p.Type),
				Required: p.Required, Multiple: p.Multiple, Values: p.Values,
				// A default reaches a browser as text, because that is what a
				// control holds. Its type is already stated beside it.
				Default: defaultText(p.Default),
			})
		}
		out.Datasets = append(out.Datasets, DatasetSummary{
			Name: ds.Name, Title: ds.Title, Description: ds.Description, Sources: names,
			Fields: len(ds.Fields), Measures: measures, Params: len(ds.Params),
			RowScoped: len(ds.RowLevelSecurity) > 0,
			Columns:   columns, Parameters: params,
		})
	}

	for _, src := range project.Definitions.DataSources() {
		out.Sources = append(out.Sources, SourceSummary{
			Name: src.Name, Title: src.Title, Description: src.Description, Driver: src.Driver,
			Detail: detail(src), Datasets: readers[src.Name], Federated: src.Federated(),
			MaxRows: src.Limits.Rows(), Timeout: src.Limits.Timeout().String(),
		})
	}

	for _, rep := range project.Definitions.Reports() {
		outputs, blocks := []string{}, 0
		for _, o := range rep.Outputs {
			outputs = append(outputs, o.Name)
			blocks += len(o.Layout)
		}
		out.Reports = append(out.Reports, ReportSummary{
			Name: rep.Name, Title: rep.Title, Description: rep.Description,
			Folder: rep.Folder, Datasets: rep.Datasets(), Outputs: outputs, Blocks: blocks,
		})
	}

	due := map[string]time.Time{}
	if project.Due != nil {
		due = project.Due.Due()
	}
	for _, s := range project.Definitions.Schedules() {
		summary := ScheduleSummary{
			Name: s.Name, Title: s.Title, Description: s.Description, Report: s.Report,
			Output: s.Output, Cron: s.Cron, Timezone: s.Timezone,
			Bursts: s.Bursts(), Channels: channels(s),
		}
		if s.Bursts() {
			summary.Over = s.Burst.Over.Dataset
		}
		if when, armed := due[s.Name]; armed {
			summary.Next = &when
		}
		out.Schedules = append(out.Schedules, summary)
	}
	return out
}

// detail describes a connection without describing how to open it.
//
// A DSN holds a password and this response reaches a browser. What somebody
// recognises a source by is its kind and where it points, not its credentials.
func detail(src definition.DataSource) string {
	if src.Federated() {
		return src.Format + " in " + src.URI
	}
	return src.Driver
}

func channels(s definition.Schedule) []string {
	out := make([]string, 0, len(s.Deliver))
	for _, d := range s.Deliver {
		out = append(out, d.Via)
	}
	return out
}

// defaultText renders a param's default as the text a control holds.
//
// fmt.Sprint rather than a type switch: a default is whatever YAML decoded,
// and the alternative is a switch that has to be extended every time somebody
// writes `default: 30` where the last person wrote `default: "30"`. A nil
// default is the empty string, which is how "there isn't one" reaches a form.
func defaultText(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}
