package spreadsheet

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Write renders the workbook as an .xlsx.
//
// The four parts an .xlsx needs, and nothing else: the package relationships,
// the workbook, its relationships, and one sheet each. No shared string table
// — inline strings cost a few more bytes and remove an entire index to keep in
// step with the cells that point into it.
func Write(w io.Writer, b Book) error {
	if len(b.Sheets) == 0 {
		// A workbook with no sheets is a file Excel refuses to open, which
		// reads to a recipient as a corrupt download rather than an empty
		// report.
		return fmt.Errorf("spreadsheet: a workbook needs at least one sheet")
	}

	z := zip.NewWriter(w)
	parts := map[string]string{
		"[Content_Types].xml":        contentTypes(b),
		"_rels/.rels":                rootRels,
		"xl/workbook.xml":            workbook(b),
		"xl/_rels/workbook.xml.rels": workbookRels(b),
	}
	for i, s := range b.Sheets {
		parts[fmt.Sprintf("xl/worksheets/sheet%d.xml", i+1)] = worksheet(s)
	}

	for name, body := range parts {
		f, err := z.Create(name)
		if err != nil {
			return err
		}
		if _, err := io.WriteString(f, body); err != nil {
			return err
		}
	}
	return z.Close()
}

const rootRels = xml.Header + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>
</Relationships>`

// contentTypes declares what each part is.
//
// Every sheet is listed individually. A reader that finds a part with no
// declared type treats the whole package as damaged rather than skipping it.
func contentTypes(b Book) string {
	var s strings.Builder
	s.WriteString(xml.Header)
	s.WriteString(`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">`)
	s.WriteString(`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>`)
	s.WriteString(`<Default Extension="xml" ContentType="application/xml"/>`)
	s.WriteString(`<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>`)
	for i := range b.Sheets {
		fmt.Fprintf(&s, `<Override PartName="/xl/worksheets/sheet%d.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>`, i+1)
	}
	s.WriteString(`</Types>`)
	return s.String()
}

func workbook(b Book) string {
	var s strings.Builder
	s.WriteString(xml.Header)
	s.WriteString(`<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets>`)
	for i, sheet := range b.Sheets {
		fmt.Fprintf(&s, `<sheet name="%s" sheetId="%d" r:id="rId%d"/>`,
			escape(clean(sheet.Name, i)), i+1, i+1)
	}
	s.WriteString(`</sheets></workbook>`)
	return s.String()
}

func workbookRels(b Book) string {
	var s strings.Builder
	s.WriteString(xml.Header)
	s.WriteString(`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
	for i := range b.Sheets {
		fmt.Fprintf(&s, `<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet%d.xml"/>`, i+1, i+1)
	}
	s.WriteString(`</Relationships>`)
	return s.String()
}

// worksheet writes the cells.
func worksheet(sheet Sheet) string {
	var s strings.Builder
	s.WriteString(xml.Header)
	s.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">`)

	if sheet.FreezeHeader && len(sheet.Headers) > 0 {
		// Split below row one and make the bottom-left pane the active one, or
		// the sheet opens with the cursor in a pane the reader cannot see.
		s.WriteString(`<sheetViews><sheetView workbookViewId="0" tabSelected="1">` +
			`<pane ySplit="1" topLeftCell="A2" activePane="bottomLeft" state="frozen"/>` +
			`</sheetView></sheetViews>`)
	}

	s.WriteString(`<sheetData>`)
	row := 1
	if len(sheet.Headers) > 0 {
		writeRow(&s, row, headerCells(sheet.Headers))
		row++
	}
	for _, cells := range sheet.Rows {
		writeRow(&s, row, cells)
		row++
	}
	s.WriteString(`</sheetData>`)

	if sheet.AutoFilter && len(sheet.Headers) > 0 {
		// The filter covers the header and every row under it, so the dropdown
		// filters the data rather than only labelling it.
		fmt.Fprintf(&s, `<autoFilter ref="A1:%s%d"/>`,
			column(len(sheet.Headers)-1), len(sheet.Rows)+1)
	}
	s.WriteString(`</worksheet>`)
	return s.String()
}

func headerCells(headers []string) []Cell {
	out := make([]Cell, len(headers))
	for i, h := range headers {
		out[i] = Str(h)
	}
	return out
}

func writeRow(s *strings.Builder, n int, cells []Cell) {
	fmt.Fprintf(s, `<row r="%d">`, n)
	for i, c := range cells {
		ref := column(i) + strconv.Itoa(n)
		if c.Numeric {
			// No t attribute: the default is numeric, and the value is written
			// unformatted so the recipient's locale decides how it looks.
			fmt.Fprintf(s, `<c r="%s"><v>%s</v></c>`, ref,
				strconv.FormatFloat(c.Number, 'f', -1, 64))
			continue
		}
		if c.Text == "" {
			// An empty cell rather than an empty inline string: the latter is
			// legal and some readers show it as a space.
			continue
		}
		fmt.Fprintf(s, `<c r="%s" t="inlineStr"><is><t xml:space="preserve">%s</t></is></c>`,
			ref, escape(c.Text))
	}
	s.WriteString(`</row>`)
}

// column turns a zero-based index into a spreadsheet column name.
//
// Bijective base-26, which is not quite base-26: there is no zero digit, so
// after Z comes AA rather than BA. Getting this wrong is invisible until a
// report has twenty-seven columns.
func column(n int) string {
	name := ""
	for n >= 0 {
		name = string(rune('A'+n%26)) + name
		n = n/26 - 1
	}
	return name
}

// escape writes text that XML will survive.
//
// Cell values come from a customer database, so this is the same reasoning as
// the embed component's refusal to use innerHTML: an ampersand in a company
// name should not be able to produce a file that will not open.
func escape(s string) string {
	var out strings.Builder
	if err := xml.EscapeText(&out, []byte(strip(s))); err != nil {
		return ""
	}
	return out.String()
}

// strip removes the control characters XML 1.0 cannot represent at all.
//
// Not an edge case: a NUL or a stray 0x1F in a database column would otherwise
// produce a file every reader rejects, and the report would look broken rather
// than the data.
func strip(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' || r == '\r' {
			return r
		}
		if r < 0x20 || (r >= 0x7F && r <= 0x9F) {
			return -1
		}
		return r
	}, s)
}

// clean makes a sheet name Excel will accept.
//
// It forbids five characters and caps the length at 31, and a name that breaks
// either produces a file that will not open — which reads as a corrupt
// download rather than as a naming mistake.
func clean(name string, index int) string {
	name = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`:\/?*[]`, r) {
			return '-'
		}
		return r
	}, strip(name))

	name = strings.TrimSpace(name)
	if name == "" {
		name = fmt.Sprintf("Sheet%d", index+1)
	}
	if runes := []rune(name); len(runes) > 31 {
		name = string(runes[:31])
	}
	return name
}
