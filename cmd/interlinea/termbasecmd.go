package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/4nonX/interlinea/term"
	"github.com/4nonX/interlinea/term/termdb"
)

// defaultTermDBPath returns the termbase database used when -db is not
// given: $INTERLINEA_TERMDB, or "Interlinea/termbases.db" in the user
// configuration directory.
func defaultTermDBPath() string {
	if p := os.Getenv("INTERLINEA_TERMDB"); p != "" {
		return p
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "termbases.db"
	}
	return filepath.Join(dir, "Interlinea", "termbases.db")
}

// currentUser names the person in entry histories.
func currentUser() string {
	if u, err := user.Current(); err == nil {
		if u.Name != "" {
			return u.Name
		}
		if u.Username != "" {
			return u.Username
		}
	}
	for _, k := range []string{"USER", "USERNAME"} {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return "unknown"
}

var termbaseCommands = []command{
	{"create", "create a termbase", runTBCreate},
	{"list", "list termbases", runTBList},
	{"rename", "rename a termbase", runTBRename},
	{"delete", "delete a termbase and all its entries", runTBDelete},
	{"import", "import a TBX, MultiTerm (XML or .sdltb), CSV, TSV or Excel file", runTBImport},
	{"export", "export a termbase as TBX, MultiTerm XML, CSV, TSV or Excel", runTBExport},
	{"lookup", "find the entries that have a term", runTBLookup},
	{"search", "search terms and field values", runTBSearch},
	{"show", "show an entry", runTBShow},
	{"history", "show every version of an entry", runTBHistory},
	{"delete-entry", "delete an entry", runTBDeleteEntry},
	{"duplicates", "list entries that share a term", runTBDuplicates},
	{"merge", "merge entries into one", runTBMerge},
	{"remove-language", "remove a language from a termbase and its entries", runTBRemoveLanguage},
	{"remove-field", "remove a field from a termbase and its entries", runTBRemoveField},
	{"recognize", "find the terms of stored termbases in a sentence", runTBRecognize},
}

func runTermbase(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprintln(stderr, "Usage: interlinea termbase <command> [options]")
		fmt.Fprintln(stderr)
		fmt.Fprintln(stderr, "Commands:")
		for _, c := range termbaseCommands {
			fmt.Fprintf(stderr, "  %-16s %s\n", c.name, c.summary)
		}
		fmt.Fprintf(stderr, "\nTermbases are stored in %s unless -db is given.\n", defaultTermDBPath())
		if len(args) == 0 {
			return usageErr("missing termbase command")
		}
		return nil
	}
	for _, c := range termbaseCommands {
		if c.name == args[0] {
			return c.run(args[1:], stdin, stdout, stderr)
		}
	}
	return usageErr("unknown termbase command %q", args[0])
}

func termDBFlag(fs *flag.FlagSet) *string {
	return fs.String("db", defaultTermDBPath(), "database file")
}

func byFlag(fs *flag.FlagSet) *string {
	return fs.String("by", currentUser(), "name recorded in the entry history")
}

func openTermbase(ctx context.Context, dbPath, name string) (*termdb.DB, *termdb.Termbase, error) {
	if strings.TrimSpace(name) == "" {
		return nil, nil, usageErr("-name is required")
	}
	db, err := termdb.Open(dbPath)
	if err != nil {
		return nil, nil, err
	}
	tb, err := db.Termbase(ctx, name)
	if err != nil {
		db.Close()
		if errors.Is(err, termdb.ErrNotFound) {
			return nil, nil, fmt.Errorf("no termbase named %q in %s", name, dbPath)
		}
		return nil, nil, err
	}
	return db, tb, nil
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func printJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func runTBCreate(args []string, _ io.Reader, stdout, stderr io.Writer) error {
	fs := newFlags("termbase create", "", stderr)
	dbPath := termDBFlag(fs)
	name := fs.String("name", "", "termbase name (required)")
	langs := fs.String("langs", "", "languages, comma-separated, e.g. de-DE,en-GB (more are added as entries need them)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return usageErr("unexpected arguments: %v", fs.Args())
	}
	if strings.TrimSpace(*name) == "" {
		return usageErr("-name is required")
	}
	db, err := termdb.Open(*dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	tb, err := db.Create(context.Background(), term.Definition{Name: *name, Languages: splitList(*langs)})
	if errors.Is(err, termdb.ErrExists) {
		return fmt.Errorf("a termbase named %q already exists in %s", *name, *dbPath)
	}
	if err != nil {
		return err
	}
	def, err := tb.Definition(context.Background())
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Created %q (%s) in %s\n", def.Name, strings.Join(def.Languages, ", "), *dbPath)
	return nil
}

func runTBList(args []string, _ io.Reader, stdout, stderr io.Writer) error {
	fs := newFlags("termbase list", "", stderr)
	dbPath := termDBFlag(fs)
	asJSON := fs.Bool("json", false, "print as JSON")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	db, err := termdb.Open(*dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	list, err := db.List(context.Background())
	if err != nil {
		return err
	}
	if *asJSON {
		if list == nil {
			list = []termdb.Summary{}
		}
		return printJSON(stdout, list)
	}
	if len(list) == 0 {
		fmt.Fprintf(stdout, "No termbases in %s.\n", *dbPath)
		return nil
	}
	for _, s := range list {
		fmt.Fprintf(stdout, "%-30s %7d entries %8d terms  %s\n", s.Name, s.Entries, s.Terms, strings.Join(s.Languages, ", "))
	}
	return nil
}

func runTBRename(args []string, _ io.Reader, stdout, stderr io.Writer) error {
	fs := newFlags("termbase rename", "", stderr)
	dbPath := termDBFlag(fs)
	name := fs.String("name", "", "current name (required)")
	to := fs.String("to", "", "new name (required)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if strings.TrimSpace(*to) == "" {
		return usageErr("-to is required")
	}
	ctx := context.Background()
	db, tb, err := openTermbase(ctx, *dbPath, *name)
	if err != nil {
		return err
	}
	defer db.Close()
	old := tb.Name()
	if err := tb.Rename(ctx, *to); errors.Is(err, termdb.ErrExists) {
		return fmt.Errorf("a termbase named %q already exists", *to)
	} else if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Renamed %q to %q\n", old, tb.Name())
	return nil
}

func runTBDelete(args []string, _ io.Reader, stdout, stderr io.Writer) error {
	fs := newFlags("termbase delete", "", stderr)
	dbPath := termDBFlag(fs)
	name := fs.String("name", "", "termbase name (required)")
	yes := fs.Bool("yes", false, "confirm permanent deletion (required)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	ctx := context.Background()
	db, tb, err := openTermbase(ctx, *dbPath, *name)
	if err != nil {
		return err
	}
	defer db.Close()
	n, _, err := tb.Count(ctx)
	if err != nil {
		return err
	}
	if !*yes {
		return usageErr("this permanently deletes %q and its %d entries; repeat with -yes to confirm", tb.Name(), n)
	}
	if err := db.Delete(ctx, tb.Name()); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Deleted %q (%d entries)\n", tb.Name(), n)
	return nil
}

func runTBImport(args []string, _ io.Reader, stdout, stderr io.Writer) error {
	fs := newFlags("termbase import", "file", stderr)
	dbPath := termDBFlag(fs)
	by := byFlag(fs)
	name := fs.String("name", "", "termbase name (required)")
	create := fs.Bool("create", false, "create the termbase if it does not exist")
	from := fs.String("from", "", "file format (default: detected)")
	mapping := fs.String("mapping", "", "column mapping JSON for CSV, TSV and Excel (see 'term mapping')")
	match := fs.String("match", "term", "find existing entries by: term, id or none")
	lang := fs.String("lang", "", "with -match term: only compare terms of this language")
	on := fs.String("on", "merge", "when an entry exists: merge, overwrite, skip or add")
	dry := fs.Bool("dry-run", false, "show what the import would do without changing anything")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return usageErr("expected one file")
	}
	p := term.Policy{Match: term.Match(*match), Lang: *lang, OnMatch: term.OnMatch(*on)}
	switch p.Match {
	case term.MatchTerm, term.MatchID, term.MatchNone:
	default:
		return usageErr("-match must be term, id or none")
	}
	switch p.OnMatch {
	case term.Merge, term.Overwrite, term.Skip, term.AddNew:
	default:
		return usageErr("-on must be merge, overwrite, skip or add")
	}
	file, rrep, _, err := readTermFile(fs.Arg(0), *from, *mapping)
	if err != nil {
		return err
	}
	if strings.TrimSpace(*name) == "" {
		return usageErr("-name is required")
	}
	ctx := context.Background()
	db, err := termdb.Open(*dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	tb, err := db.Termbase(ctx, *name)
	if errors.Is(err, termdb.ErrNotFound) {
		if !*create {
			return fmt.Errorf("no termbase named %q in %s (add -create to create it)", *name, *dbPath)
		}
		if *dry {
			fmt.Fprintf(stdout, "Would create %q\n", *name)
			tb = nil
		} else if tb, err = db.Create(ctx, term.Definition{Name: *name, Languages: file.Languages}); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	printWarnings(stderr, rrep.Warnings)
	var rep termdb.ImportReport
	switch {
	case tb == nil:
		entries, terms, _ := file.Stats()
		fmt.Fprintf(stdout, "Would add %d entries (%d terms)\n", entries, terms)
		return nil
	case *dry:
		_, rep, err = tb.Plan(ctx, file, p)
	default:
		rep, err = tb.Import(ctx, file, p, *by, filepath.Base(fs.Arg(0)))
	}
	if err != nil {
		return err
	}
	verb := "Imported"
	if *dry {
		verb = "Would import"
	}
	fmt.Fprintf(stdout, "%s %s into %q: %d added, %d merged, %d replaced, %d left as they were\n",
		verb, filepath.Base(fs.Arg(0)), tb.Name(), rep.Added, rep.Merged, rep.Overwritten, rep.Skipped+rep.Unchanged)
	if rep.Invalid > 0 {
		fmt.Fprintf(stdout, "%d entries without terms were left out\n", rep.Invalid)
	}
	if len(rep.Languages) > 0 {
		fmt.Fprintf(stdout, "New languages: %s\n", strings.Join(rep.Languages, ", "))
	}
	if len(rep.Fields) > 0 {
		fmt.Fprintf(stdout, "New fields: %s\n", strings.Join(rep.Fields, ", "))
	}
	printWarnings(stderr, rep.Warnings)
	return nil
}

func runTBExport(args []string, _ io.Reader, stdout, stderr io.Writer) error {
	fs := newFlags("termbase export", "file", stderr)
	dbPath := termDBFlag(fs)
	name := fs.String("name", "", "termbase name (required)")
	to := fs.String("to", "", "format (default: from the file extension; .tbx is TBX 3)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return usageErr("expected one output file")
	}
	var f term.Format
	var err error
	if *to != "" {
		f, err = parseFormat(*to)
	} else {
		f, err = formatForOutput(fs.Arg(0))
	}
	if err != nil {
		return err
	}
	if f == term.FormatXDT {
		return usageErr("termbase definitions (xdt) can be read but not written")
	}
	if f == term.FormatSDLTB {
		return usageErr("MultiTerm termbase files (sdltb) can be read but not written; write MultiTerm XML, which MultiTerm imports")
	}
	ctx := context.Background()
	db, tb, err := openTermbase(ctx, *dbPath, *name)
	if err != nil {
		return err
	}
	defer db.Close()
	out, err := tb.Export(ctx)
	if err != nil {
		return err
	}
	data, err := term.Write(out, f)
	if err != nil {
		return err
	}
	if err := os.WriteFile(fs.Arg(0), data, 0o644); err != nil {
		return err
	}
	entries, terms, _ := out.Stats()
	fmt.Fprintf(stdout, "%d entries, %d terms written to %s (%s)\n", entries, terms, fs.Arg(0), f)
	return nil
}

func printEntries(w io.Writer, list []termdb.Stored) {
	for _, s := range list {
		fmt.Fprintln(w, describeEntry(s.Entry, 0))
	}
}

func runTBLookup(args []string, _ io.Reader, stdout, stderr io.Writer) error {
	fs := newFlags("termbase lookup", "term", stderr)
	dbPath := termDBFlag(fs)
	name := fs.String("name", "", "termbase name (required)")
	lang := fs.String("lang", "", "language of the term (default: any)")
	asJSON := fs.Bool("json", false, "print as JSON")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return usageErr("expected one term")
	}
	ctx := context.Background()
	db, tb, err := openTermbase(ctx, *dbPath, *name)
	if err != nil {
		return err
	}
	defer db.Close()
	list, err := tb.Lookup(ctx, fs.Arg(0), *lang)
	if err != nil {
		return err
	}
	if *asJSON {
		if list == nil {
			list = []termdb.Stored{}
		}
		return printJSON(stdout, list)
	}
	if len(list) == 0 {
		fmt.Fprintf(stdout, "No entry has %q\n", fs.Arg(0))
		return nil
	}
	printEntries(stdout, list)
	return nil
}

func runTBSearch(args []string, _ io.Reader, stdout, stderr io.Writer) error {
	fs := newFlags("termbase search", "[text]", stderr)
	dbPath := termDBFlag(fs)
	name := fs.String("name", "", "termbase name (required)")
	lang := fs.String("lang", "", "only entries with terms in this language; the text is searched in them")
	status := fs.String("status", "", "only entries with a term of this status: approved, proposed or forbidden")
	limit := fs.Int("limit", 50, "most entries to print (0 for all)")
	offset := fs.Int("offset", 0, "entries to skip")
	asJSON := fs.Bool("json", false, "print as JSON")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() > 1 {
		return usageErr("expected at most one search text (quote phrases)")
	}
	switch *status {
	case "", term.StatusApproved, term.StatusProposed, term.StatusForbidden:
	default:
		return usageErr("-status must be approved, proposed or forbidden")
	}
	ctx := context.Background()
	db, tb, err := openTermbase(ctx, *dbPath, *name)
	if err != nil {
		return err
	}
	defer db.Close()
	list, total, err := tb.Search(ctx, termdb.Query{Text: fs.Arg(0), Lang: *lang, Status: *status, Limit: *limit, Offset: *offset})
	if err != nil {
		return err
	}
	if *asJSON {
		if list == nil {
			list = []termdb.Stored{}
		}
		return printJSON(stdout, struct {
			Total   int             `json:"total"`
			Entries []termdb.Stored `json:"entries"`
		}{total, list})
	}
	printEntries(stdout, list)
	fmt.Fprintf(stdout, "%d of %d matching entries\n", len(list), total)
	return nil
}

func runTBShow(args []string, _ io.Reader, stdout, stderr io.Writer) error {
	fs := newFlags("termbase show", "entry-id", stderr)
	dbPath := termDBFlag(fs)
	name := fs.String("name", "", "termbase name (required)")
	asJSON := fs.Bool("json", false, "print as JSON")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return usageErr("expected one entry ID")
	}
	ctx := context.Background()
	db, tb, err := openTermbase(ctx, *dbPath, *name)
	if err != nil {
		return err
	}
	defer db.Close()
	s, err := tb.Get(ctx, fs.Arg(0))
	if errors.Is(err, termdb.ErrNotFound) {
		return fmt.Errorf("no entry %q in %q", fs.Arg(0), tb.Name())
	}
	if err != nil {
		return err
	}
	if *asJSON {
		return printJSON(stdout, s)
	}
	fmt.Fprintf(stdout, "Entry %s, version %d, changed %s by %s\n", s.ID, s.Version, s.ChangedAt.Local().Format("2006-01-02 15:04"), s.ChangedBy)
	for _, v := range s.Fields {
		fmt.Fprintf(stdout, "  %s: %s\n", v.Field, v.Value)
	}
	for _, l := range s.Languages {
		fmt.Fprintf(stdout, "%s\n", l.Lang)
		for _, v := range l.Fields {
			fmt.Fprintf(stdout, "  %s: %s\n", v.Field, v.Value)
		}
		for _, t := range l.Terms {
			st := t.Status
			if st == "" {
				st = "no status"
			}
			fmt.Fprintf(stdout, "  %s (%s)\n", t.Text, st)
			for _, v := range t.Fields {
				fmt.Fprintf(stdout, "    %s: %s\n", v.Field, v.Value)
			}
		}
	}
	return nil
}

func runTBHistory(args []string, _ io.Reader, stdout, stderr io.Writer) error {
	fs := newFlags("termbase history", "entry-id", stderr)
	dbPath := termDBFlag(fs)
	name := fs.String("name", "", "termbase name (required)")
	asJSON := fs.Bool("json", false, "print as JSON")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return usageErr("expected one entry ID")
	}
	ctx := context.Background()
	db, tb, err := openTermbase(ctx, *dbPath, *name)
	if err != nil {
		return err
	}
	defer db.Close()
	h, err := tb.History(ctx, fs.Arg(0))
	if errors.Is(err, termdb.ErrNotFound) {
		return fmt.Errorf("no entry %q in %q", fs.Arg(0), tb.Name())
	}
	if err != nil {
		return err
	}
	if *asJSON {
		return printJSON(stdout, h)
	}
	for _, v := range h {
		fmt.Fprintf(stdout, "%3d  %s  %-20s %s\n", v.Version, v.At.Local().Format("2006-01-02 15:04"), v.By, v.Summary)
	}
	return nil
}

func runTBDeleteEntry(args []string, _ io.Reader, stdout, stderr io.Writer) error {
	fs := newFlags("termbase delete-entry", "entry-id", stderr)
	dbPath := termDBFlag(fs)
	by := byFlag(fs)
	name := fs.String("name", "", "termbase name (required)")
	yes := fs.Bool("yes", false, "confirm the deletion (required)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return usageErr("expected one entry ID")
	}
	ctx := context.Background()
	db, tb, err := openTermbase(ctx, *dbPath, *name)
	if err != nil {
		return err
	}
	defer db.Close()
	s, err := tb.Get(ctx, fs.Arg(0))
	if errors.Is(err, termdb.ErrNotFound) {
		return fmt.Errorf("no entry %q in %q", fs.Arg(0), tb.Name())
	}
	if err != nil {
		return err
	}
	if !*yes {
		return usageErr("this deletes entry %s (%s); repeat with -yes to confirm", s.ID, describeEntry(s.Entry, 0))
	}
	if err := tb.DeleteEntry(ctx, s.ID, *by, ""); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Deleted entry %s\n", s.ID)
	return nil
}

func runTBDuplicates(args []string, _ io.Reader, stdout, stderr io.Writer) error {
	fs := newFlags("termbase duplicates", "", stderr)
	dbPath := termDBFlag(fs)
	name := fs.String("name", "", "termbase name (required)")
	lang := fs.String("lang", "", "only compare terms of this language (default: all)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	ctx := context.Background()
	db, tb, err := openTermbase(ctx, *dbPath, *name)
	if err != nil {
		return err
	}
	defer db.Close()
	groups, err := tb.Duplicates(ctx, *lang)
	if err != nil {
		return err
	}
	for _, g := range groups {
		var parts []string
		for _, id := range g {
			s, err := tb.Get(ctx, id)
			if err != nil {
				return err
			}
			parts = append(parts, describeEntry(s.Entry, 0))
		}
		fmt.Fprintln(stdout, strings.Join(parts, "  |  "))
	}
	fmt.Fprintf(stdout, "%d groups of entries share a term\n", len(groups))
	return nil
}

func runTBMerge(args []string, _ io.Reader, stdout, stderr io.Writer) error {
	fs := newFlags("termbase merge", "keep-id other-id...", stderr)
	dbPath := termDBFlag(fs)
	by := byFlag(fs)
	name := fs.String("name", "", "termbase name (required)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() < 2 {
		return usageErr("expected the entry to keep and at least one entry to merge into it")
	}
	ctx := context.Background()
	db, tb, err := openTermbase(ctx, *dbPath, *name)
	if err != nil {
		return err
	}
	defer db.Close()
	s, err := tb.Merge(ctx, fs.Arg(0), fs.Args()[1:], *by)
	if errors.Is(err, termdb.ErrNotFound) {
		return fmt.Errorf("%v: check the entry IDs", err)
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Merged into %s\n", describeEntry(s.Entry, 0))
	return nil
}

func runTBRemoveLanguage(args []string, _ io.Reader, stdout, stderr io.Writer) error {
	fs := newFlags("termbase remove-language", "", stderr)
	dbPath := termDBFlag(fs)
	by := byFlag(fs)
	name := fs.String("name", "", "termbase name (required)")
	lang := fs.String("lang", "", "language to remove (required)")
	yes := fs.Bool("yes", false, "confirm (required): removes the language's terms from every entry")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if strings.TrimSpace(*lang) == "" {
		return usageErr("-lang is required")
	}
	if !*yes {
		return usageErr("this removes every %s term and deletes entries left without terms; repeat with -yes to confirm", *lang)
	}
	ctx := context.Background()
	db, tb, err := openTermbase(ctx, *dbPath, *name)
	if err != nil {
		return err
	}
	defer db.Close()
	n, err := tb.RemoveLanguage(ctx, *lang, *by)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Removed %s; %d entries changed\n", term.NormalizeLang(*lang), n)
	return nil
}

func runTBRemoveField(args []string, _ io.Reader, stdout, stderr io.Writer) error {
	fs := newFlags("termbase remove-field", "", stderr)
	dbPath := termDBFlag(fs)
	by := byFlag(fs)
	name := fs.String("name", "", "termbase name (required)")
	field := fs.String("field", "", "field to remove (required)")
	yes := fs.Bool("yes", false, "confirm (required): removes the field's values from every entry")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if strings.TrimSpace(*field) == "" {
		return usageErr("-field is required")
	}
	if !*yes {
		return usageErr("this removes every value of %q; repeat with -yes to confirm", *field)
	}
	ctx := context.Background()
	db, tb, err := openTermbase(ctx, *dbPath, *name)
	if err != nil {
		return err
	}
	defer db.Close()
	n, err := tb.RemoveField(ctx, *field, *by)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Removed %q; %d entries changed\n", *field, n)
	return nil
}

func runTBRecognize(args []string, _ io.Reader, stdout, stderr io.Writer) error {
	fs := newFlags("termbase recognize", "\"source text\"", stderr)
	dbPath := termDBFlag(fs)
	names := fs.String("name", "", "termbases, comma-separated, highest priority first (required)")
	src := fs.String("src", "", "source language (required)")
	tgt := fs.String("tgt", "", "target language (required)")
	target := fs.String("target", "", "translation to check for missing and forbidden terms")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return usageErr("expected one source text")
	}
	if *src == "" || *tgt == "" {
		return usageErr("-src and -tgt are required")
	}
	list := splitList(*names)
	if len(list) == 0 {
		return usageErr("-name is required")
	}
	ctx := context.Background()
	db, err := termdb.Open(*dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	x := term.NewIndex()
	for _, n := range list {
		tb, err := db.Termbase(ctx, n)
		if errors.Is(err, termdb.ErrNotFound) {
			return fmt.Errorf("no termbase named %q in %s", n, *dbPath)
		}
		if err != nil {
			return err
		}
		if err := tb.AddTo(ctx, x); err != nil {
			return err
		}
	}
	printRecognition(stdout, x, fs.Arg(0), *src, *tgt, *target)
	return nil
}

// printRecognition prints the terms found in text and, with a target, the
// problems of the translation.
func printRecognition(w io.Writer, x *term.Index, text, src, tgt, target string) {
	hits := x.Recognize(text, src, tgt)
	for _, h := range hits {
		var ts []string
		for _, t := range h.Targets {
			s := t.Text
			if t.Status != "" && t.Status != term.StatusApproved {
				s += " (" + t.Status + ")"
			}
			ts = append(ts, s)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", text[h.Start:h.End], h.Term.Text, strings.Join(ts, "; "))
	}
	if target == "" {
		return
	}
	for _, is := range x.Check(hits, target, tgt) {
		switch is.Kind {
		case "missing":
			fmt.Fprintf(w, "missing: %q is usually %q\n", is.Term, is.Expected)
		case "forbidden":
			fmt.Fprintf(w, "forbidden: %q\n", is.Found)
		}
	}
}
