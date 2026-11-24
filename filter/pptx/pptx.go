// Package pptx reads and writes PowerPoint presentations (.pptx).
//
// Extract returns the text of each slide in the order of the show: titles,
// text boxes and shapes, tables, charts with the names from their data,
// and SmartArt, followed by the slide's speaker notes. Formatting that
// changes inside a paragraph becomes paired tags, line breaks and fields
// standalone tags. Merge writes the translations back and copies every
// other byte of the package unchanged.
//
// Slide masters and layouts are not read, and comments are kept as they
// are.
package pptx

import (
	"github.com/4nonX/interlinea/filter"
	"github.com/4nonX/interlinea/filter/docx"
)

// ErrNotPresentation is returned for files that are not PowerPoint
// presentations.
var ErrNotPresentation = docx.ErrNotPresentation

// Extract returns the translatable units of a presentation. lang is the
// source language (a BCP 47 tag); it selects the sentence rules.
func Extract(data []byte, lang string) (*filter.Extraction, error) {
	return docx.ExtractPresentation(data, lang)
}

// Merge returns a copy of the presentation with the units replaced by
// their translations. lang must be the language used for Extract;
// targetLang is written to the rewritten text.
func Merge(data []byte, lang, targetLang string, tr filter.Translations) ([]byte, error) {
	return docx.MergePresentation(data, lang, targetLang, tr)
}
