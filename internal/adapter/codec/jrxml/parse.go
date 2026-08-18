package jrxml

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/unicode"
)

// parse reads a .jrxml file into the wire model.
//
// Namespace-insensitive by consequence rather than by effort: the struct tags
// name local elements, and encoding/xml matches those in any namespace. That is
// what the file needs — JasperReports moved namespace twice, files carry a
// DOCTYPE instead in 1.x, and Jasper Studio writes a third variant — and none of
// those differences change what the report says.
func parse(data []byte) (document, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.CharsetReader = charsetReader
	// Jasper Studio writes files with an internal DTD subset on occasion, and
	// entity definitions there are not something this needs to resolve.
	dec.Strict = false
	// Autoclose keeps a stray unclosed element from consuming the rest of the
	// document; Strict false already tolerates the mismatch.
	dec.Entity = xml.HTMLEntity

	var doc document
	if err := dec.Decode(&doc); err != nil {
		return document{}, fmt.Errorf("%w: %v", ErrParse, err)
	}
	if doc.XMLName.Local != "jasperReport" {
		// A .jasper (compiled), a .jrpxml (print output) or a datasource
		// adapter file — all of which live in the same directory as the
		// reports and all of which are worth naming rather than parsing badly.
		return document{}, fmt.Errorf("%w: root element is <%s>, not <jasperReport>",
			ErrParse, doc.XMLName.Local)
	}
	if doc.Name == "" {
		return document{}, fmt.Errorf("%w: <jasperReport> has no name attribute", ErrParse)
	}
	return doc, nil
}

// charsetReader decodes the single-byte encodings a Jasper estate actually
// contains.
//
// Not a nicety. JasperReports Studio wrote the platform default encoding for
// years, so a report authored in Warsaw or Cologne is ISO-8859-2 or
// windows-1252, and Go's XML decoder refuses a declared encoding it does not
// know rather than guessing. Refusing the file would strand exactly the estates
// this exists to migrate — and guessing UTF-8 would turn a customer's name into
// replacement characters in every statement they receive.
func charsetReader(label string, input io.Reader) (io.Reader, error) {
	enc, ok := charsets[strings.ToLower(strings.TrimSpace(label))]
	if !ok {
		return nil, fmt.Errorf("%w: encoding %q is not one this reads — re-save the file as UTF-8",
			ErrParse, label)
	}
	if enc == nil {
		return input, nil
	}
	return enc.NewDecoder().Reader(input), nil
}

// charsets are the labels seen on real files. A nil entry is already UTF-8.
var charsets = map[string]encoding.Encoding{
	"utf-8": nil, "utf8": nil, "us-ascii": nil, "ascii": nil,
	"iso-8859-1": charmap.ISO8859_1, "latin1": charmap.ISO8859_1,
	"iso-8859-2": charmap.ISO8859_2, "iso-8859-3": charmap.ISO8859_3,
	"iso-8859-4": charmap.ISO8859_4, "iso-8859-5": charmap.ISO8859_5,
	"iso-8859-7": charmap.ISO8859_7, "iso-8859-9": charmap.ISO8859_9,
	"iso-8859-15":  charmap.ISO8859_15,
	"windows-1250": charmap.Windows1250, "windows-1251": charmap.Windows1251,
	"windows-1252": charmap.Windows1252, "windows-1253": charmap.Windows1253,
	"windows-1254": charmap.Windows1254, "windows-1257": charmap.Windows1257,
	"cp1250": charmap.Windows1250, "cp1252": charmap.Windows1252,
	"utf-16":   unicode.UTF16(unicode.BigEndian, unicode.UseBOM),
	"utf-16le": unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM),
	"utf-16be": unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM),
}
