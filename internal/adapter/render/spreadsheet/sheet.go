package spreadsheet

// Sheet is one tab.
type Sheet struct {
	// Name is what the tab is called. Excel forbids several characters and
	// caps the length; see clean.
	Name    string
	Headers []string
	Rows    [][]Cell
	// FreezeHeader keeps the first row visible while scrolling, and AutoFilter
	// puts a dropdown on each heading. Both are the first two things every
	// recipient does by hand, so the export does them.
	FreezeHeader bool
	AutoFilter   bool
}

// Book is a workbook: at least one sheet.
type Book struct {
	Sheets []Sheet
}
