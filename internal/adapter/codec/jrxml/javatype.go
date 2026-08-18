package jrxml

import (
	"strings"

	"github.com/gsoultan/cronos/internal/core/definition"
)

// javaTypes maps a Jasper class to the field type a cronos dataset publishes.
//
// The vocabulary is the one the portal reads — string, number, decimal, date,
// bool — and it is metadata rather than coercion: nothing in the engine switches
// on it, but the builder offers a date picker for a date and the table renderer
// formats one, so getting it wrong is visible without being fatal.
//
// Keyed on the bare class name as well as the qualified one, because both spellings
// appear in real files and `java.lang.` is noise in front of `String`.
var javaTypes = map[string]string{
	"java.lang.String":    "string",
	"java.lang.Character": "string",
	"char":                "string",
	"String":              "string",

	"java.lang.Integer":                         "number",
	"java.lang.Short":                           "number",
	"java.lang.Byte":                            "number",
	"java.lang.Long":                            "number",
	"java.math.BigInteger":                      "number",
	"java.util.concurrent.atomic.AtomicInteger": "number",
	"Integer":                                   "number", "Short": "number", "Byte": "number", "Long": "number",
	"BigInteger": "number",
	"int":        "number", "long": "number", "short": "number", "byte": "number",

	"java.lang.Double":     "decimal",
	"java.lang.Float":      "decimal",
	"java.lang.Number":     "decimal",
	"java.math.BigDecimal": "decimal",
	"Double":               "decimal", "Float": "decimal", "Number": "decimal",
	"BigDecimal": "decimal",
	"double":     "decimal", "float": "decimal",

	"java.lang.Boolean": "bool",
	"Boolean":           "bool",
	"boolean":           "bool",

	"java.util.Date":              "date",
	"java.sql.Date":               "date",
	"java.sql.Timestamp":          "date",
	"java.sql.Time":               "date",
	"java.time.LocalDate":         "date",
	"java.time.LocalDateTime":     "date",
	"java.time.LocalTime":         "date",
	"java.time.Instant":           "date",
	"java.time.OffsetDateTime":    "date",
	"java.time.ZonedDateTime":     "date",
	"java.util.Calendar":          "date",
	"java.util.GregorianCalendar": "date",
	"Date":                        "date", "Timestamp": "date", "Time": "date",
	"LocalDate": "date", "LocalDateTime": "date", "Instant": "date",
}

// collections are the classes that mean "several of these", which is how Jasper
// spells a parameter for an IN clause.
var collections = map[string]bool{
	"java.util.Collection": true, "java.util.List": true, "java.util.Set": true,
	"java.util.ArrayList": true, "java.util.HashSet": true, "java.util.LinkedList": true,
	"Collection": true, "List": true, "Set": true, "ArrayList": true, "HashSet": true,
	"Object[]": true, "java.lang.Object[]": true,
}

// fieldType reads a Jasper field's class. ok false means the class is not one
// that maps to a value a report can display.
func fieldType(class string) (kind string, ok bool) {
	kind, ok = javaTypes[strings.TrimSpace(class)]
	return kind, ok
}

// paramType reads a Jasper parameter's class as a cronos param type.
//
// multiple reports a collection, which cronos expresses as Multiple on a param
// rather than as a type of its own. The element type comes from nestedType, and
// a collection with no nestedType is a list of strings as far as anything can
// tell.
func paramType(class, nested string) (kind definition.ParamType, multiple bool, ok bool) {
	class = strings.TrimSpace(class)
	if collections[class] {
		inner, found := scalarParamType(nested)
		if !found {
			// A collection of unknown things binds as a list of strings, which
			// is what an IN clause over an unknown column wanted anyway.
			return definition.String, true, nested == ""
		}
		return inner, true, true
	}
	kind, ok = scalarParamType(class)
	return kind, false, ok
}

func scalarParamType(class string) (definition.ParamType, bool) {
	switch javaTypes[strings.TrimSpace(class)] {
	case "string":
		return definition.String, true
	case "number", "decimal":
		return definition.Number, true
	case "bool":
		return definition.Bool, true
	case "date":
		return definition.Date, true
	}
	return "", false
}

// calculations maps a Jasper variable's calculation to a cronos aggregate.
//
// The five cronos has, and nothing invented for the rest: a standard deviation
// that silently became a sum would be a subtotal nobody could account for.
var calculations = map[string]string{
	"Sum":     "sum",
	"Count":   "count",
	"Average": "avg",
	"Lowest":  "min",
	"Highest": "max",
}

// aggregateOf reads a variable's calculation. ok false means cronos has no
// equivalent fold, which the caller reports rather than approximates.
func aggregateOf(calculation string) (string, bool) {
	agg, ok := calculations[strings.TrimSpace(calculation)]
	return agg, ok
}

// dimensionWords are the last words of a field name that mean "this number
// identifies something" rather than "this number is a quantity".
//
// A heuristic, and the only place the import guesses at meaning rather than
// reading it. It earns its place because the alternative defaults are both
// wrong: every numeric field a measure offers "sum of invoice_id" in the
// builder, and every numeric field a dimension makes an amount column
// unsummable, which is a broken report rather than a silly menu entry.
//
// Read from the Jasper name's words, so `CustomerId` and `customer_id` are the
// same answer.
var dimensionWords = map[string]bool{
	"id": true, "code": true, "no": true, "num": true, "number": true,
	"key": true, "ref": true, "reference": true, "year": true, "month": true,
	"quarter": true, "week": true, "day": true, "zip": true, "postcode": true,
	"phone": true, "version": true,
}

// looksLikeDimension reports whether a numeric field is an identifier rather
// than a quantity, judged by the last word of its Jasper name.
func looksLikeDimension(jasperName string) bool {
	parts := words(jasperName)
	if len(parts) == 0 {
		return false
	}
	return dimensionWords[parts[len(parts)-1]]
}
