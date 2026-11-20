package definition

import (
	"fmt"
	"regexp"
)

// fieldFormats are the ways a field's figures can be written. Closed, for the reason
// chart types are: a format nobody draws is a field that silently reads plain.
var fieldFormats = map[string]bool{
	"": true, "number": true, "currency": true, "percent": true, "preformatted": true,
}

// currencyCode is an ISO 4217 code: three capitals.
var currencyCode = regexp.MustCompile(`^[A-Z]{3}$`)

// MaxDecimals is the most places a figure may keep. Past six a figure is
// noise, and a report that asks for more has mistaken a value for a key.
const MaxDecimals = 6

// validateFormat checks how the field asks to be written.
func (f Field) validateFormat() error {
	switch {
	case !fieldFormats[f.Format]:
		return fmt.Errorf("%w: field %q has format %q, want number, currency, percent or preformatted",
			ErrInvalid, f.Name, f.Format)
	case f.Currency != "" && !currencyCode.MatchString(f.Currency):
		return fmt.Errorf("%w: field %q has currency %q, want a three-letter ISO 4217 code such as EUR",
			ErrInvalid, f.Name, f.Currency)
	case f.Currency != "" && f.Format != "currency":
		// A currency on a field that is not money is a format somebody
		// forgot to write, and it would otherwise be ignored without a word.
		return fmt.Errorf("%w: field %q names a currency but is not format: currency",
			ErrInvalid, f.Name)
	case f.Decimals != nil && (*f.Decimals < 0 || *f.Decimals > MaxDecimals):
		return fmt.Errorf("%w: field %q keeps %d decimals, want 0 to %d",
			ErrInvalid, f.Name, *f.Decimals, MaxDecimals)
	}
	return nil
}
