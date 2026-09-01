package jrxml

import "encoding/xml"

// document is the jasperReport root, as the file spells it.
//
// Unexported, with the rest of the wire model: this is JasperReports' shape and
// it should not reach anything outside this package. Fields are only those the
// translation reads — everything else in the schema is found by the census in
// unsupported.go, which does not need a struct field per element to report one.
type document struct {
	XMLName xml.Name `xml:"jasperReport"`
	Name    string   `xml:"name,attr"`
	// Language is the *expression* language (java, groovy, javascript), not the
	// query's. The query carries its own.
	Language string `xml:"language,attr"`

	PageWidth    int    `xml:"pageWidth,attr"`
	PageHeight   int    `xml:"pageHeight,attr"`
	Orientation  string `xml:"orientation,attr"`
	LeftMargin   int    `xml:"leftMargin,attr"`
	RightMargin  int    `xml:"rightMargin,attr"`
	TopMargin    int    `xml:"topMargin,attr"`
	BottomMargin int    `xml:"bottomMargin,attr"`
	ColumnCount  int    `xml:"columnCount,attr"`

	Query queryString `xml:"queryString"`
	// QueryJR7 is the same element under the name JasperReports 7 gives it.
	// Identical shape — a language attribute and the SQL as chardata — so the
	// newer spelling costs one field rather than a second parser.
	QueryJR7   queryString `xml:"query"`
	Parameters []parameter `xml:"parameter"`
	Fields     []field     `xml:"field"`
	SortFields []sortField `xml:"sortField"`
	Variables  []variable  `xml:"variable"`
	Groups     []group     `xml:"group"`
	// SubDatasets back charts, crosstabs and list components. None of them
	// survive the import, but a file that has them is a file whose second query
	// went missing, which is worth saying.
	SubDatasets []subDataset `xml:"subDataset"`
	// FilterExpression drops rows in the engine rather than the query. cronos
	// has no equivalent that is not a WHERE clause the author must write.
	FilterExpression string `xml:"filterExpression"`

	Title          section `xml:"title"`
	PageHeader     section `xml:"pageHeader"`
	ColumnHeader   section `xml:"columnHeader"`
	Detail         section `xml:"detail"`
	ColumnFooter   section `xml:"columnFooter"`
	PageFooter     section `xml:"pageFooter"`
	LastPageFooter section `xml:"lastPageFooter"`
	Summary        section `xml:"summary"`
}

// section is one of the report's fixed slots. Each holds bands — plural since
// JasperReports 6, and a report that uses two of them in the detail is stacking
// two rows per record, which the table inference has to notice.
type section struct {
	Bands []band `xml:"band"`
}

// queryString is the SQL, and the claim that it is SQL.
type queryString struct {
	// Language is empty in most files and means SQL. Anything else is refused:
	// see the package doc.
	Language string `xml:"language,attr"`
	SQL      string `xml:",chardata"`
}

// parameter is a question the report accepts, in Java's terms.
type parameter struct {
	Name  string `xml:"name,attr"`
	Class string `xml:"class,attr"`
	// NestedType is the element type when Class is a collection, which is how
	// Jasper expresses a multi-valued parameter for an IN clause.
	NestedType string `xml:"nestedType,attr"`
	// ForPrompting false marks a parameter the report computes rather than
	// asks for. A pointer because absent means true.
	ForPrompting *bool  `xml:"isForPrompting,attr"`
	Description  string `xml:"parameterDescription"`
	Default      string `xml:"defaultValueExpression"`
}

// prompts reports whether a caller is expected to supply this.
func (p parameter) prompts() bool { return p.ForPrompting == nil || *p.ForPrompting }

// field is one column of the query's result.
type field struct {
	Name  string `xml:"name,attr"`
	Class string `xml:"class,attr"`
	// Description is where Jasper Studio puts a column comment, and where many
	// authors put the label they wanted. Used as a fallback label.
	Description string `xml:"fieldDescription"`
}

// variable is a running calculation. The ones with a calculation and a plain
// field expression are subtotals; the rest are Java the import cannot carry.
type variable struct {
	Name        string `xml:"name,attr"`
	Class       string `xml:"class,attr"`
	Calculation string `xml:"calculation,attr"`
	// ResetType is Report, Page, Column or Group. Group, with ResetGroup, is
	// what makes a variable a per-group subtotal.
	ResetType  string `xml:"resetType,attr"`
	ResetGroup string `xml:"resetGroup,attr"`
	Expression string `xml:"variableExpression"`
}

// sortField is an ORDER BY the engine applies after the query returns.
type sortField struct {
	Name  string `xml:"name,attr"`
	Order string `xml:"order,attr"`
	// Type is Field or Variable. A variable sort has nothing to map to.
	Type string `xml:"type,attr"`
}

// group is a break in the detail, and the reason paginated output exists.
type group struct {
	Name string `xml:"name,attr"`
	// StartNewPage is pageBreak: perGroup, the one property of a Jasper group
	// that has an exact cronos equivalent.
	StartNewPage bool `xml:"isStartNewPage,attr"`
	// ReprintHeaderOnEachPage is what cronos does unconditionally for a grouped
	// table, so a file that says false is asking for something not available.
	ReprintHeaderOnEachPage *bool   `xml:"isReprintHeaderOnEachPage,attr"`
	Expression              string  `xml:"groupExpression"`
	Header                  section `xml:"groupHeader"`
	Footer                  section `xml:"groupFooter"`
}

// subDataset is a second query inside the file, feeding a chart or a crosstab.
type subDataset struct {
	Name  string      `xml:"name,attr"`
	Query queryString `xml:"queryString"`
	// QueryJR7 is the same element under the name JasperReports 7 gives it.
	// Identical shape — a language attribute and the SQL as chardata — so the
	// newer spelling costs one field rather than a second parser.
	QueryJR7   queryString `xml:"query"`
	Fields     []field     `xml:"field"`
	Parameters []parameter `xml:"parameter"`
}
