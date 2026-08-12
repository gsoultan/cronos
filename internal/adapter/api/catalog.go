package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/principal"
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
	// channels are the delivery channels this deployment has configured.
	channels []string
	defs     Repository
	due      Due
	auth     Principals
	log      *slog.Logger
}

// NewCatalog wires the handler.
func NewCatalog(d Repository, due Due, a Principals, log *slog.Logger) *CatalogHandler {
	return &CatalogHandler{defs: d, due: due, auth: a, log: log}
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
	send(w, http.StatusOK, c.build(pr))
}

func (c *CatalogHandler) build(_ principal.Principal) Catalog {
	out := Catalog{
		Sources: []SourceSummary{}, Datasets: []DatasetSummary{},
		Reports: []ReportSummary{}, Schedules: []ScheduleSummary{},
		Channels: c.channels,
	}
	if out.Channels == nil {
		out.Channels = []string{}
	}

	readers := map[string]int{}
	for _, ds := range c.defs.Datasets() {
		names := make([]string, 0, len(ds.Sources))
		for _, s := range ds.Sources {
			names = append(names, s.Ref)
			readers[s.Ref]++
		}
		measures := 0
		for _, f := range ds.Fields {
			if f.Role == definition.Measure {
				measures++
			}
		}
		out.Datasets = append(out.Datasets, DatasetSummary{
			Name: ds.Name, Title: ds.Title, Description: ds.Description, Sources: names,
			Fields: len(ds.Fields), Measures: measures, Params: len(ds.Params),
			RowScoped: len(ds.RowLevelSecurity) > 0,
		})
	}

	for _, src := range c.defs.DataSources() {
		out.Sources = append(out.Sources, SourceSummary{
			Name: src.Name, Title: src.Title, Description: src.Description, Driver: src.Driver,
			Detail: detail(src), Datasets: readers[src.Name], Federated: src.Federated(),
			MaxRows: src.Limits.Rows(), Timeout: src.Limits.Timeout().String(),
		})
	}

	for _, rep := range c.defs.Reports() {
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
	if c.due != nil {
		due = c.due.Due()
	}
	for _, s := range c.defs.Schedules() {
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
