package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/4nonX/interlinea/qa"
	"github.com/4nonX/interlinea/spell"
	"github.com/4nonX/interlinea/term"
	"github.com/4nonX/interlinea/term/termdb"
)

// loadDictionary reads a Hunspell dictionary: a .aff or .dic file (the
// other is found next to it), a base path without extension, or a
// LibreOffice extension (.oxt) or zip, from which the dictionary for
// lang is taken (or the only one).
func loadDictionary(path, lang string) (*spell.Dictionary, error) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".oxt", ".zip", ".xpi":
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		bundles, err := spell.ReadArchive(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		b := pickBundle(bundles, lang)
		if b == nil {
			var names []string
			for _, x := range bundles {
				names = append(names, x.Name+" ("+strings.Join(x.Locales, ", ")+")")
			}
			return nil, fmt.Errorf("%s has several dictionaries and none for %q: %s", path, lang, strings.Join(names, "; "))
		}
		return spell.Load(b.Aff, b.Dic)
	case ".aff", ".dic":
		path = strings.TrimSuffix(path, filepath.Ext(path))
	}
	aff, err := os.ReadFile(path + ".aff")
	if err != nil {
		return nil, err
	}
	dic, err := os.ReadFile(path + ".dic")
	if err != nil {
		return nil, err
	}
	d, err := spell.Load(aff, dic)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return d, nil
}

func pickBundle(bs []spell.Bundled, lang string) *spell.Bundled {
	if len(bs) == 1 {
		return &bs[0]
	}
	norm := func(s string) string { return strings.ToLower(strings.ReplaceAll(s, "_", "-")) }
	l := norm(lang)
	primary := strings.SplitN(l, "-", 2)[0]
	var fallback *spell.Bundled
	for i := range bs {
		for _, loc := range bs[i].Locales {
			if norm(loc) == l {
				return &bs[i]
			}
			if fallback == nil && strings.SplitN(norm(loc), "-", 2)[0] == primary {
				fallback = &bs[i]
			}
		}
	}
	return fallback
}

// readWordList reads a user dictionary: one word per line, "#" starts a
// comment.
func readWordList(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, l := range strings.Split(string(data), "\n") {
		l = strings.TrimSpace(strings.TrimPrefix(l, "\xef\xbb\xbf"))
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		out = append(out, l)
	}
	return out, nil
}

func parseKinds(s string) ([]qa.Kind, error) {
	var out []qa.Kind
	for _, k := range splitList(s) {
		ok := false
		for _, x := range qa.Kinds {
			if string(x.Kind) == k {
				ok = true
			}
		}
		if !ok {
			return nil, usageErr("unknown check %q (see interlinea qa -checks)", k)
		}
		out = append(out, qa.Kind(k))
	}
	return out, nil
}

func runQA(args []string, _ io.Reader, stdout, stderr io.Writer) error {
	fs := newFlags("qa", "bilingual-file...", stderr)
	format := fs.String("format", "text", "report format: text, json, csv or html")
	out := fs.String("o", "", "write the report to this file (default: standard output)")
	dict := fs.String("dict", "", "Hunspell dictionary for spelling: .aff/.dic path or .oxt extension")
	words := fs.String("words", "", "file of words always spelled right (one per line)")
	terms := fs.String("terms", "", "termbase file to check terminology against (TBX, MultiTerm XML, CSV, Excel)")
	termFrom := fs.String("terms-format", "", "format of -terms (default: detected)")
	termMapping := fs.String("terms-mapping", "", "column mapping JSON for a CSV or Excel -terms file")
	dbPath := termDBFlag(fs)
	termbases := fs.String("termbase", "", "stored termbases to check against, comma-separated, highest priority first")
	rules := fs.String("rules", "", "JSON file with rules of your own (regular expressions)")
	disable := fs.String("disable", "", "checks to leave out, comma-separated")
	enable := fs.String("enable", "", "checks that are off by default to run, comma-separated")
	src := fs.String("src", "", "source language (default: from the file)")
	tgt := fs.String("tgt", "", "target language (default: from the file)")
	showChecks := fs.Bool("checks", false, "list the checks and exit")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if *showChecks {
		for _, k := range qa.Kinds {
			def := "on"
			if !k.Default {
				def = "off"
			}
			fmt.Fprintf(stdout, "%-26s %-4s %s\n", k.Kind, def, k.Description)
		}
		return nil
	}
	if fs.NArg() == 0 {
		return usageErr("expected one or more XLIFF, SDLXLIFF or MQXLIFF files")
	}
	switch *format {
	case "text", "json", "csv", "html":
	default:
		return usageErr("unknown format %q", *format)
	}
	opts := qa.Options{}
	var err error
	if opts.Disabled, err = parseKinds(*disable); err != nil {
		return err
	}
	if opts.Enabled, err = parseKinds(*enable); err != nil {
		return err
	}
	if *rules != "" {
		data, err := os.ReadFile(*rules)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(data, &opts.Rules); err != nil {
			return fmt.Errorf("%s: %w", *rules, err)
		}
	}
	if *terms != "" || *termbases != "" {
		x := term.NewIndex()
		if *termbases != "" {
			db, err := termdb.Open(*dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			ctx := context.Background()
			for _, n := range splitList(*termbases) {
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
		}
		if *terms != "" {
			tb, rep, _, err := readTermFile(*terms, *termFrom, *termMapping)
			if err != nil {
				return err
			}
			printWarnings(stderr, rep.Warnings)
			x.Add(tb.Name, tb.Entries)
		}
		opts.Terms = x
	}
	if *words != "" {
		if opts.SpellIgnore, err = readWordList(*words); err != nil {
			return err
		}
	}
	type fileReport struct {
		name string
		rep  qa.Report
	}
	var reports []fileReport
	var speller *spell.Dictionary
	for _, name := range fs.Args() {
		_, ex, err := readXLIFF(name)
		if err != nil {
			return err
		}
		o := opts
		o.SourceLang, o.TargetLang = ex.SourceLang, ex.TargetLang
		if *src != "" {
			o.SourceLang = *src
		}
		if *tgt != "" {
			o.TargetLang = *tgt
		}
		if *dict != "" {
			if speller == nil {
				if speller, err = loadDictionary(*dict, o.TargetLang); err != nil {
					return err
				}
			}
			o.Spell = speller
		}
		var segs []qa.Segment
		for _, u := range ex.Units {
			segs = append(segs, qa.Segment{ID: u.ID, Source: u.Source, Target: u.Target, MaxLength: u.MaxLength, Timing: u.Timing, Skip: u.Locked})
		}
		issues, err := qa.Check(segs, o)
		if err != nil {
			return err
		}
		reports = append(reports, fileReport{name, qa.NewReport(filepath.Base(name), segs, issues, o, time.Now())})
	}
	w := stdout
	if *out != "" {
		f, err := os.Create(*out)
		if err != nil {
			return err
		}
		defer f.Close()
		w = f
	}
	errorsFound := 0
	for i, fr := range reports {
		errorsFound += fr.rep.Errors
		var data []byte
		switch *format {
		case "json":
			if len(reports) > 1 {
				continue
			}
			data, err = fr.rep.JSON()
			data = append(data, '\n')
		case "csv":
			data, err = fr.rep.CSV()
			if i > 0 {
				data = stripCSVHeader(data)
			}
		case "html":
			data = fr.rep.HTML()
		default:
			data = []byte(textReport(fr.rep))
		}
		if err != nil {
			return err
		}
		if _, err := w.Write(data); err != nil {
			return err
		}
	}
	if *format == "json" && len(reports) > 1 {
		var all []qa.Report
		for _, fr := range reports {
			all = append(all, fr.rep)
		}
		if err := printJSON(w, all); err != nil {
			return err
		}
	}
	if errorsFound > 0 {
		return fmt.Errorf("%d errors found", errorsFound)
	}
	return nil
}

// stripCSVHeader removes the byte order mark and header row of a second
// report appended to the first.
func stripCSVHeader(data []byte) []byte {
	s := strings.TrimPrefix(string(data), "\xef\xbb\xbf")
	if i := strings.Index(s, "\n"); i >= 0 {
		return []byte(s[i+1:])
	}
	return nil
}

func textReport(r qa.Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s: %d of %d segments checked, %d errors, %d warnings\n", r.Document, r.Checked, r.Segments, r.Errors, r.Warnings)
	for _, is := range r.Issues {
		line := fmt.Sprintf("  %d\t%s\t%s\t%s", is.Number, is.Severity, qa.KindName(is.Kind), is.Message)
		if len(is.Suggestions) > 0 {
			line += " Suggestions: " + strings.Join(is.Suggestions, ", ")
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

func runSpell(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	fs := newFlags("spell", "[words...]", stderr)
	dict := fs.String("dict", "", "Hunspell dictionary: .aff/.dic path or .oxt extension (required)")
	lang := fs.String("lang", "", "language to pick from an extension with several dictionaries")
	suggest := fs.Int("suggest", 5, "suggestions per misspelled word (0 for none)")
	all := fs.Bool("all", false, "also print correctly spelled words")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if *dict == "" {
		return usageErr("-dict is required")
	}
	d, err := loadDictionary(*dict, *lang)
	if err != nil {
		return err
	}
	words := fs.Args()
	if len(words) == 0 {
		data, err := io.ReadAll(stdin)
		if err != nil {
			return err
		}
		words = strings.Fields(string(data))
	}
	wrong := 0
	for _, w := range words {
		w = strings.Trim(w, `,;:!?()[]{}"“”„«»`)
		if w == "" {
			continue
		}
		if d.Spell(w) {
			if *all {
				fmt.Fprintf(stdout, "ok\t%s\n", w)
			}
			continue
		}
		wrong++
		line := "wrong\t" + w
		if *suggest > 0 {
			line += "\t" + strings.Join(d.Suggest(w, *suggest), ", ")
		}
		fmt.Fprintln(stdout, line)
	}
	if wrong > 0 {
		return fmt.Errorf("%d misspelled", wrong)
	}
	return nil
}
