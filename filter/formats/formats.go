// Package formats names the file formats Interlinea reads and sends a
// document to the filter of its format.
package formats

import (
	"fmt"
	"path"
	"strings"

	"github.com/4nonX/interlinea/filter"
	"github.com/4nonX/interlinea/filter/docx"
	"github.com/4nonX/interlinea/filter/html"
	"github.com/4nonX/interlinea/filter/idml"
	"github.com/4nonX/interlinea/filter/pptx"
	"github.com/4nonX/interlinea/filter/xlsx"
)

// The formats, by the names stored with a project's files.
const (
	Word       = "docx"
	PowerPoint = "pptx"
	Excel      = "xlsx"
	HTML       = "html"
	InDesign   = "idml"
	// XLIFF is a bilingual file: XLIFF 1.2 and 2.0, Trados SDLXLIFF, memoQ
	// MQXLIFF. MemoQArchive is a memoQ XLIFF in its archive (.mqxlz).
	XLIFF        = "xliff"
	MemoQArchive = "mqxlz"
	// TradosPackage and MemoQPackage are packages of bilingual files.
	TradosPackage = "sdlppx"
	MemoQPackage  = "mqout"
	// Text is plain text.
	Text = "txt"
)

var byExtension = map[string]string{
	".docx": Word, ".docm": Word,
	".pptx": PowerPoint, ".pptm": PowerPoint, ".ppsx": PowerPoint,
	".xlsx": Excel, ".xlsm": Excel,
	".html": HTML, ".htm": HTML, ".xhtml": HTML,
	".idml": InDesign,
	".xlf":  XLIFF, ".xliff": XLIFF, ".sdlxliff": XLIFF, ".mqxliff": XLIFF,
	".mqxlz":  MemoQArchive,
	".sdlppx": TradosPackage,
	".mqout":  MemoQPackage,
}

// Of is the format of a file by its name; Text for any other name.
func Of(name string) string {
	if f, ok := byExtension[strings.ToLower(path.Ext(name))]; ok {
		return f
	}
	return Text
}

// Document reports whether the format is a document in one language that
// a filter cuts into units: Word, PowerPoint, Excel, a web page, InDesign.
func Document(format string) bool {
	switch format {
	case Word, PowerPoint, Excel, HTML, InDesign:
		return true
	}
	return false
}

// Package reports whether the format is a package of bilingual files.
func Package(format string) bool {
	return format == TradosPackage || format == MemoQPackage
}

// Name is the format as people call it.
func Name(format string) string {
	switch format {
	case Word:
		return "Word document"
	case PowerPoint:
		return "PowerPoint presentation"
	case Excel:
		return "Excel workbook"
	case HTML:
		return "web page"
	case InDesign:
		return "InDesign document (IDML)"
	case XLIFF:
		return "bilingual file"
	case MemoQArchive:
		return "memoQ XLIFF archive"
	case TradosPackage:
		return "Trados package"
	case MemoQPackage:
		return "memoQ handoff package"
	case Text:
		return "text file"
	}
	return format
}

// Extract returns the translatable units of a document. lang is the
// source language; it selects the sentence rules.
func Extract(format string, data []byte, lang string) (*filter.Extraction, error) {
	switch format {
	case Word:
		return docx.Extract(data, lang, docx.Options{})
	case PowerPoint:
		return pptx.Extract(data, lang)
	case Excel:
		return xlsx.Extract(data, lang)
	case HTML:
		return html.Extract(data, lang)
	case InDesign:
		return idml.Extract(data, lang)
	}
	return nil, fmt.Errorf("formats: %q is not a document format", format)
}

// Merge returns the document with its units replaced by their
// translations. lang must be the language used for Extract.
func Merge(format string, data []byte, lang, targetLang string, tr filter.Translations) ([]byte, error) {
	switch format {
	case Word:
		return docx.Merge(data, lang, targetLang, tr, docx.Options{})
	case PowerPoint:
		return pptx.Merge(data, lang, targetLang, tr)
	case Excel:
		return xlsx.Merge(data, lang, targetLang, tr)
	case HTML:
		return html.Merge(data, lang, targetLang, tr)
	case InDesign:
		return idml.Merge(data, lang, targetLang, tr)
	}
	return nil, fmt.Errorf("formats: %q is not a document format", format)
}
