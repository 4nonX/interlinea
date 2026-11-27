// Package i18n marks the core's texts that the apps show: the names and
// descriptions of the checks, the rows of an analysis, how a tag of a
// file is described. The core keeps them in English, in the files it
// writes as well; the apps look each one up in the catalog of their
// interface language (i18n/de.json, i18n/ko.json), which `go run
// ./i18n/gen` keeps in step with these marks.
//
// A text with a part that changes is marked with a named placeholder,
// such as N("Font {value}"), and filled with Fill: the apps recognize the
// filled text by the marked one and translate it with the part kept.
package i18n

import "strings"

// N marks a text the apps translate. It returns the text unchanged.
func N(s string) string { return s }

// Fill puts values into the placeholders of a marked text, given as
// name, value pairs: Fill(N("Font {value}"), "value", "Arial").
func Fill(s string, pairs ...string) string {
	for i := 0; i+1 < len(pairs); i += 2 {
		s = strings.ReplaceAll(s, "{"+pairs[i]+"}", pairs[i+1])
	}
	return s
}
