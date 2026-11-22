package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/4nonX/interlinea/term"
)

// runTerm handles termbase files: inspection, conversion between exchange
// formats, column mappings for spreadsheets and duplicate detection.
func runTerm(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	sub := map[string]func([]string, io.Writer, io.Writer) error{
		"info":       runTermInfo,
		"convert":    runTermConvert,
		"mapping":    runTermMapping,
		"duplicates": runTermDuplicates,
	}
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprintln(stderr, "Usage: interlinea term <info|convert|mapping|duplicates> [options] file")
		fmt.Fprintln(stderr)
		fmt.Fprintln(stderr, "  info        show languages, fields and counts of a termbase file")
		fmt.Fprintln(stderr, "  convert     convert between TBX 3, TBX 2, MultiTerm XML, CSV, TSV and Excel")
		fmt.Fprintln(stderr, "  mapping     print the guessed column mapping of a spreadsheet as JSON")
		fmt.Fprintln(stderr, "  duplicates  list entries that share a term")
		fmt.Fprintln(stderr)
		fmt.Fprintln(stderr, "Formats: "+formatList())
		if len(args) == 0 {
			return usageErr("expected a subcommand")
		}
		return nil
	}
	fn, ok := sub[args[0]]
	if !ok {
		return usageErr("unknown subcommand %q", args[0])
	}
	return fn(args[1:], stdout, stderr)
}

var termFormats = []term.Format{term.FormatTBX, term.FormatTBX2, term.FormatMultiTerm, term.FormatXDT, term.FormatCSV, term.FormatTSV, term.FormatXLSX}

func formatList() string {
	var s []string
	for _, f := range termFormats {
		s = append(s, string(f))
	}
	return strings.Join(s, ", ")
}

func parseFormat(s string) (term.Format, error) {
	for _, f := range termFormats {
		if string(f) == strings.ToLower(s) {
			return f, nil
		}
	}
	return "", usageErr("unknown format %q (use %s)", s, formatList())
}

// formatForOutput picks the output format from the file extension.
func formatForOutput(name string) (term.Format, error) {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".tbx":
		return term.FormatTBX, nil
	case ".xml", ".mtf":
		return term.FormatMultiTerm, nil
	case ".csv":
		return term.FormatCSV, nil
	case ".tsv", ".tab", ".txt":
		return term.FormatTSV, nil
	case ".xlsx":
		return term.FormatXLSX, nil
	}
	return "", usageErr("cannot tell the format from %q; use -to", name)
}

// readTermFile reads a termbase file in the given or detected format.
func readTermFile(name, format, mappingFile string) (*term.Termbase, term.Report, term.Format, error) {
	data, err := os.ReadFile(name)
	if err != nil {
		return nil, term.Report{}, "", err
	}
	var f term.Format
	if format != "" {
		if f, err = parseFormat(format); err != nil {
			return nil, term.Report{}, "", err
		}
	} else if f, err = term.Detect(filepath.Base(name), data); err != nil {
		return nil, term.Report{}, "", err
	}
	var m *term.Mapping
	if mappingFile != "" {
		if !f.IsTabular() {
			return nil, term.Report{}, "", usageErr("-mapping only applies to CSV, TSV and Excel files")
		}
		b, err := os.ReadFile(mappingFile)
		if err != nil {
			return nil, term.Report{}, "", err
		}
		m = new(term.Mapping)
		if err := json.Unmarshal(b, m); err != nil {
			return nil, term.Report{}, "", fmt.Errorf("mapping %s: %w", mappingFile, err)
		}
	}
	tb, rep, err := term.Read(f, data, m)
	if err != nil {
		return nil, rep, f, err
	}
	if tb.Name == "" {
		tb.Name = strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	}
	return tb, rep, f, nil
}

func printWarnings(stderr io.Writer, ws []string) {
	for _, w := range ws {
		fmt.Fprintf(stderr, "note: %s\n", w)
	}
}

func runTermInfo(args []string, stdout, stderr io.Writer) error {
	fs := newFlags("term info", "file", stderr)
	format := fs.String("from", "", "input format (default: detected)")
	mapping := fs.String("mapping", "", "column mapping JSON for CSV, TSV and Excel (see 'term mapping')")
	asJSON := fs.Bool("json", false, "print the whole termbase as JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return usageErr("expected one termbase file")
	}
	tb, rep, f, err := readTermFile(fs.Arg(0), *format, *mapping)
	if err != nil {
		return err
	}
	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(tb); err != nil {
			return err
		}
		printWarnings(stderr, rep.Warnings)
		return nil
	}
	entries, terms, perLang := tb.Stats()
	fmt.Fprintf(stdout, "Format:    %s\n", f)
	fmt.Fprintf(stdout, "Name:      %s\n", tb.Name)
	fmt.Fprintf(stdout, "Entries:   %d\n", entries)
	fmt.Fprintf(stdout, "Terms:     %d\n", terms)
	if rep.Skipped > 0 {
		fmt.Fprintf(stdout, "Skipped:   %d\n", rep.Skipped)
	}
	fmt.Fprintln(stdout, "Languages:")
	for _, l := range tb.Languages {
		fmt.Fprintf(stdout, "  %-8s %-20s %d terms\n", l, term.LangName(l), perLang[l])
	}
	if len(tb.Fields) > 0 {
		fmt.Fprintln(stdout, "Fields:")
		for _, fd := range tb.Fields {
			fmt.Fprintf(stdout, "  %-24s %-9s %s\n", fd.Name, fd.Level, fd.Kind)
		}
	}
	printWarnings(stderr, rep.Warnings)
	return nil
}

func runTermConvert(args []string, stdout, stderr io.Writer) error {
	fs := newFlags("term convert", "input output", stderr)
	from := fs.String("from", "", "input format (default: detected)")
	to := fs.String("to", "", "output format (default: from the output file extension; .tbx is TBX 3)")
	mapping := fs.String("mapping", "", "column mapping JSON for CSV, TSV and Excel input")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		return usageErr("expected an input and an output file")
	}
	var out term.Format
	var err error
	if *to != "" {
		out, err = parseFormat(*to)
	} else {
		out, err = formatForOutput(fs.Arg(1))
	}
	if err != nil {
		return err
	}
	if out == term.FormatXDT {
		return usageErr("termbase definitions (xdt) can be read but not written")
	}
	tb, rep, _, err := readTermFile(fs.Arg(0), *from, *mapping)
	if err != nil {
		return err
	}
	data, err := term.Write(tb, out)
	if err != nil {
		return err
	}
	if err := os.WriteFile(fs.Arg(1), data, 0o644); err != nil {
		return err
	}
	entries, terms, _ := tb.Stats()
	fmt.Fprintf(stdout, "%d entries, %d terms written to %s (%s)\n", entries, terms, fs.Arg(1), out)
	printWarnings(stderr, rep.Warnings)
	return nil
}

func runTermMapping(args []string, stdout, stderr io.Writer) error {
	fs := newFlags("term mapping", "file", stderr)
	from := fs.String("from", "", "input format: csv, tsv or xlsx (default: detected)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return usageErr("expected one spreadsheet file")
	}
	data, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return err
	}
	var f term.Format
	if *from != "" {
		f, err = parseFormat(*from)
	} else {
		f, err = term.Detect(filepath.Base(fs.Arg(0)), data)
	}
	if err != nil {
		return err
	}
	if !f.IsTabular() {
		return usageErr("%s is %s; only CSV, TSV and Excel files need a mapping", fs.Arg(0), f)
	}
	rows, err := term.Rows(f, data)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(term.GuessMapping(rows))
}

func runTermDuplicates(args []string, stdout, stderr io.Writer) error {
	fs := newFlags("term duplicates", "file", stderr)
	from := fs.String("from", "", "input format (default: detected)")
	mapping := fs.String("mapping", "", "column mapping JSON for CSV, TSV and Excel")
	lang := fs.String("lang", "", "only compare terms of this language (default: all)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return usageErr("expected one termbase file")
	}
	tb, rep, _, err := readTermFile(fs.Arg(0), *from, *mapping)
	if err != nil {
		return err
	}
	groups := term.Duplicates(tb.Entries, *lang)
	for _, g := range groups {
		var parts []string
		for _, i := range g {
			parts = append(parts, describeEntry(tb.Entries[i], i))
		}
		fmt.Fprintln(stdout, strings.Join(parts, "  |  "))
	}
	fmt.Fprintf(stdout, "%d groups of entries share a term\n", len(groups))
	printWarnings(stderr, rep.Warnings)
	return nil
}

func describeEntry(e term.Entry, i int) string {
	id := e.ID
	if id == "" {
		id = fmt.Sprintf("#%d", i+1)
	}
	var terms []string
	for _, l := range e.Languages {
		for _, t := range l.Terms {
			terms = append(terms, l.Lang+":"+t.Text)
		}
	}
	sort.Strings(terms)
	return id + " " + strings.Join(terms, ", ")
}
