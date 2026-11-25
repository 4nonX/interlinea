package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/fuzzy"
	"github.com/4nonX/interlinea/langtag"
	"github.com/4nonX/interlinea/segment"
	"github.com/4nonX/interlinea/tm"
	"github.com/4nonX/interlinea/tmx"
)

var paragraphBreak = regexp.MustCompile(`\r?\n[ \t]*\r?\n`)

func openInput(name string, stdin io.Reader) (io.Reader, func() error, error) {
	if name == "" || name == "-" {
		return stdin, func() error { return nil }, nil
	}
	f, err := os.Open(name)
	if err != nil {
		return nil, nil, err
	}
	return f, f.Close, nil
}

func runSegment(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	fs := newFlags("segment", "[file]", stderr)
	lang := fs.String("lang", "en", "language of the text (BCP 47, e.g. de-DE)")
	srxFile := fs.String("srx", "", "SRX 2.0 file with custom rules (default: built-in rules)")
	asJSON := fs.Bool("json", false, "print paragraphs and segments as JSON")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() > 1 {
		return usageErr("expected at most one input file")
	}
	if _, err := langtag.Normalize(*lang); err != nil {
		return usageErr("invalid language %q", *lang)
	}
	var rules *segment.Rules
	if *srxFile != "" {
		f, err := os.Open(*srxFile)
		if err != nil {
			return err
		}
		rules, err = segment.Parse(f)
		f.Close()
		if err != nil {
			return err
		}
	}
	in, closeIn, err := openInput(fs.Arg(0), stdin)
	if err != nil {
		return err
	}
	defer closeIn()
	data, err := io.ReadAll(in)
	if err != nil {
		return err
	}
	seg := segment.New(rules, *lang)
	var paragraphs [][]string
	for _, p := range paragraphBreak.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), -1) {
		if segs := seg.Strings(p); len(segs) > 0 {
			paragraphs = append(paragraphs, segs)
		}
	}
	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if paragraphs == nil {
			paragraphs = [][]string{}
		}
		return enc.Encode(paragraphs)
	}
	for i, p := range paragraphs {
		if i > 0 {
			fmt.Fprintln(stdout)
		}
		for _, s := range p {
			fmt.Fprintln(stdout, s)
		}
	}
	return nil
}

func runTMXInfo(args []string, _ io.Reader, stdout, stderr io.Writer) error {
	fs := newFlags("tmx-info", "<file.tmx>", stderr)
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return usageErr("expected one TMX file")
	}
	f, err := os.Open(fs.Arg(0))
	if err != nil {
		return err
	}
	defer f.Close()
	rd, err := tmx.NewReader(f)
	if err != nil {
		return err
	}
	units := 0
	langs := map[string]int{}
	tagged := 0
	for {
		u, err := rd.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		units++
		hasTags := false
		for _, v := range u.Variants {
			l, err := langtag.Normalize(v.Lang)
			if err != nil {
				l = v.Lang
			}
			langs[l]++
			hasTags = hasTags || v.Content.HasTags()
		}
		if hasTags {
			tagged++
		}
	}
	h := rd.Header
	fmt.Fprintf(stdout, "Creation tool:   %s %s\n", h.CreationTool, h.CreationToolVersion)
	fmt.Fprintf(stdout, "Source language: %s\n", h.SrcLang)
	fmt.Fprintf(stdout, "Segment type:    %s\n", h.SegType)
	if !h.CreationDate.IsZero() {
		fmt.Fprintf(stdout, "Created:         %s\n", h.CreationDate.Format("2006-01-02 15:04 MST"))
	}
	fmt.Fprintf(stdout, "Units:           %d (%d with inline tags)\n", units, tagged)
	keys := make([]string, 0, len(langs))
	for k := range langs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Fprintln(stdout, "Variants per language:")
	for _, k := range keys {
		fmt.Fprintf(stdout, "  %-10s %d\n", k, langs[k])
	}
	return nil
}

type memFlags struct {
	file    *string
	src     *string
	tgt     *string
	primary *bool
	asJSON  *bool
}

func addMemFlags(fs *flag.FlagSet) memFlags {
	return memFlags{
		file:    fs.String("tmx", "", "memory file to search: TMX, Trados .sdltm or Wordfast .txt (required)"),
		src:     fs.String("src", "", "source language (default: the file's)"),
		tgt:     fs.String("tgt", "", "target language (required)"),
		primary: fs.Bool("primary", false, "also accept variants that match only by primary language (de-AT for de-DE)"),
		asJSON:  fs.Bool("json", false, "print results as JSON"),
	}
}

// loadMemory reads a memory file (TMX, Trados .sdltm or Wordfast .txt)
// into an in-memory TM.
func (mf memFlags) loadMemory(ctx context.Context) (*tm.InMemory, tm.ImportStats, error) {
	if *mf.file == "" {
		return nil, tm.ImportStats{}, usageErr("-tmx is required")
	}
	if *mf.tgt == "" {
		return nil, tm.ImportStats{}, usageErr("-tgt is required")
	}
	data, err := os.ReadFile(*mf.file)
	if err != nil {
		return nil, tm.ImportStats{}, err
	}
	src := *mf.src
	if src == "" {
		f, err := tm.ReadFile(data)
		if err != nil {
			return nil, tm.ImportStats{}, err
		}
		src = f.SourceLang
		if src == "" {
			return nil, tm.ImportStats{}, usageErr("the file names no single source language; use -src")
		}
	}
	mem, err := tm.NewInMemory(tm.Info{Name: *mf.file, SourceLang: src, TargetLang: *mf.tgt})
	if err != nil {
		return nil, tm.ImportStats{}, usageErr("invalid languages %q/%q", src, *mf.tgt)
	}
	st, err := tm.Import(ctx, mem, data, tm.ImportOptions{AllowPrimaryMatch: *mf.primary})
	return mem, st, err
}

type jsonMatch struct {
	Score  int    `json:"score"`
	Source string `json:"source"`
	Target string `json:"target"`
	By     string `json:"changedBy,omitempty"`
	At     string `json:"changedAt,omitempty"`
}

func runLookup(args []string, _ io.Reader, stdout, stderr io.Writer) error {
	fs := newFlags("lookup", "<segment text>", stderr)
	mf := addMemFlags(fs)
	minScore := fs.Int("min", 70, "minimum match percentage")
	limit := fs.Int("n", 5, "maximum number of matches")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return usageErr("expected the segment text")
	}
	if *minScore < 1 || *minScore > 100 {
		return usageErr("-min must be between 1 and 100")
	}
	ctx := context.Background()
	mem, st, err := mf.loadMemory(ctx)
	if err != nil {
		return err
	}
	query := strings.Join(fs.Args(), " ")
	matches, err := mem.Lookup(ctx, content.Text(query), tm.LookupOptions{MinScore: *minScore, Limit: *limit})
	if err != nil {
		return err
	}
	fmt.Fprintf(stderr, "%d units loaded, %d skipped\n", st.Stored, st.Skipped)
	return printMatches(stdout, matches, *mf.asJSON)
}

func printMatches(w io.Writer, matches []tm.Match, asJSON bool) error {
	if asJSON {
		out := make([]jsonMatch, 0, len(matches))
		for _, m := range matches {
			jm := jsonMatch{Score: m.Score, Source: m.Unit.Source.Plain(), Target: m.Unit.Target.Plain(), By: m.Unit.ChangedBy}
			if !m.Unit.ChangedAt.IsZero() {
				jm.At = m.Unit.ChangedAt.Format("2006-01-02T15:04:05Z")
			}
			out = append(out, jm)
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}
	if len(matches) == 0 {
		fmt.Fprintln(w, "No matches.")
		return nil
	}
	for i, m := range matches {
		if i > 0 {
			fmt.Fprintln(w)
		}
		fmt.Fprintf(w, "%3d%%  %s\n      %s\n", m.Score, m.Unit.Source.String(), m.Unit.Target.String())
	}
	return nil
}

func runConcordance(args []string, _ io.Reader, stdout, stderr io.Writer) error {
	fs := newFlags("concordance", "<word or phrase>", stderr)
	mf := addMemFlags(fs)
	inTarget := fs.Bool("target", false, "search the target language instead of the source")
	limit := fs.Int("n", 20, "maximum number of results")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return usageErr("expected a word or phrase")
	}
	ctx := context.Background()
	mem, _, err := mf.loadMemory(ctx)
	if err != nil {
		return err
	}
	units, err := mem.Concordance(ctx, strings.Join(fs.Args(), " "), tm.ConcordanceOptions{InTarget: *inTarget, Limit: *limit})
	if err != nil {
		return err
	}
	return printUnits(stdout, units, *mf.asJSON)
}

func printUnits(w io.Writer, units []tm.Unit, asJSON bool) error {
	if asJSON {
		out := make([]jsonMatch, 0, len(units))
		for _, u := range units {
			out = append(out, jsonMatch{Source: u.Source.Plain(), Target: u.Target.Plain(), By: u.ChangedBy})
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}
	if len(units) == 0 {
		fmt.Fprintln(w, "No results.")
		return nil
	}
	for i, u := range units {
		if i > 0 {
			fmt.Fprintln(w)
		}
		fmt.Fprintf(w, "%s\n%s\n", u.Source.String(), u.Target.String())
	}
	return nil
}

func runScore(args []string, _ io.Reader, stdout, stderr io.Writer) error {
	fs := newFlags("score", "<segment A> <segment B>", stderr)
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		return usageErr("expected exactly two segments (quote them)")
	}
	fmt.Fprintf(stdout, "%d%%\n", fuzzy.ScoreText(fs.Arg(0), fs.Arg(1)))
	return nil
}
