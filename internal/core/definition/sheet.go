package definition

// Sheet is one tab of a spreadsheet output.
type Sheet struct {
	Name    string   `json:"name" yaml:"name"`
	Columns []string `json:"columns" yaml:"columns"`
	// FreezeHeader and AutoFilter are the two things every recipient does by
	// hand the moment they open the file, so the export does them.
	FreezeHeader bool `json:"freezeHeader,omitempty" yaml:"freezeHeader,omitempty"`
	AutoFilter   bool `json:"autoFilter,omitempty" yaml:"autoFilter,omitempty"`
}
