package term

import (
	"bytes"
	"fmt"
	"path"
	"strings"
)

// Format is a termbase exchange format.
type Format string

const (
	FormatTBX       Format = "tbx"       // TBX 3 (ISO 30042:2019)
	FormatTBX2      Format = "tbx2"      // TBX 2 (ISO 30042:2008, <martif>)
	FormatMultiTerm Format = "multiterm" // MultiTerm XML
	FormatXDT       Format = "xdt"       // MultiTerm termbase definition (languages only)
	FormatCSV       Format = "csv"
	FormatTSV       Format = "tsv"
	FormatXLSX      Format = "xlsx"
)

// Detect works out the format from the content, using the file name only
// to tell CSV from tab-separated text.
func Detect(name string, data []byte) (Format, error) {
	if bytes.HasPrefix(data, []byte("PK\x03\x04")) {
		if bytes.Contains(data, []byte("xl/workbook.xml")) {
			return FormatXLSX, nil
		}
		return "", fmt.Errorf("%s is a ZIP file but not an Excel workbook", name)
	}
	text := ToUTF8(data)
	head := text
	if len(head) > 4096 {
		head = head[:4096]
	}
	s := string(head)
	switch {
	case strings.Contains(s, "<martif"):
		return FormatTBX2, nil
	case strings.Contains(s, "<tbx"):
		return FormatTBX, nil
	case strings.Contains(s, "<mtf"):
		return FormatMultiTerm, nil
	case strings.Contains(s, "<TermbaseDefinition"):
		return FormatXDT, nil
	case strings.HasPrefix(strings.TrimSpace(s), "<"):
		return "", fmt.Errorf("%s is XML but not TBX or MultiTerm XML", name)
	}
	switch strings.ToLower(path.Ext(name)) {
	case ".tsv", ".tab":
		return FormatTSV, nil
	}
	return FormatCSV, nil
}

// IsTabular reports whether the format needs a column mapping.
func (f Format) IsTabular() bool {
	return f == FormatCSV || f == FormatTSV || f == FormatXLSX
}

// Rows reads a spreadsheet format into rows.
func Rows(f Format, data []byte) ([][]string, error) {
	switch f {
	case FormatXLSX:
		return readXLSX(data)
	case FormatTSV:
		rows, _, err := ReadCSV(data, '\t')
		return rows, err
	case FormatCSV:
		rows, _, err := ReadCSV(data, 0)
		return rows, err
	}
	return nil, fmt.Errorf("%s is not a spreadsheet format", f)
}

// Read reads a termbase. Spreadsheets need a mapping; nil uses
// GuessMapping.
func Read(f Format, data []byte, m *Mapping) (*Termbase, Report, error) {
	switch f {
	case FormatTBX, FormatTBX2:
		return ReadTBX(data)
	case FormatMultiTerm:
		return ReadMultiTerm(data)
	case FormatXDT:
		d, err := ReadXDT(data)
		if err != nil {
			return nil, Report{}, err
		}
		return &Termbase{Definition: d}, Report{Warnings: []string{"A termbase definition holds the structure only: languages are taken over, entries come from the MultiTerm XML file."}}, nil
	case FormatCSV, FormatTSV, FormatXLSX:
		rows, err := Rows(f, data)
		if err != nil {
			return nil, Report{}, err
		}
		mm := GuessMapping(rows)
		if m != nil {
			mm = *m
		}
		return FromRows(rows, mm)
	}
	return nil, Report{}, fmt.Errorf("unknown format %q", f)
}

// Write writes a termbase.
func Write(tb *Termbase, f Format) ([]byte, error) {
	switch f {
	case FormatTBX:
		return WriteTBX(tb, TBXOptions{Version: 3}), nil
	case FormatTBX2:
		return WriteTBX(tb, TBXOptions{Version: 2}), nil
	case FormatMultiTerm:
		return WriteMultiTerm(tb), nil
	case FormatCSV:
		return WriteCSV(ToRows(tb), ','), nil
	case FormatTSV:
		return WriteCSV(ToRows(tb), '\t'), nil
	case FormatXLSX:
		return writeXLSX(tb.Name, ToRows(tb)), nil
	}
	return nil, fmt.Errorf("cannot write %q", f)
}

// Extension is the usual file extension of a format.
func (f Format) Extension() string {
	switch f {
	case FormatTBX, FormatTBX2:
		return ".tbx"
	case FormatMultiTerm:
		return ".xml"
	case FormatXDT:
		return ".xdt"
	}
	return "." + string(f)
}
