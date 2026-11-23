package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/4nonX/interlinea/analysis"
	"github.com/4nonX/interlinea/filter"
	"github.com/4nonX/interlinea/filter/docx"
	"github.com/4nonX/interlinea/filter/xliff"
	"github.com/4nonX/interlinea/tm"
)

// listFlag collects a repeatable option.
type listFlag []string

func (l *listFlag) String() string { return strings.Join(*l, ",") }
func (l *listFlag) Set(v string) error {
	*l = append(*l, v)
	return nil
}

// loadTMX reads a TMX file into an in-memory TM for the language pair.
func loadTMX(ctx context.Context, path, src, tgt string, primary bool) (*tm.InMemory, tm.ImportStats, error) {
	mem, err := tm.NewInMemory(tm.Info{Name: filepath.Base(path), SourceLang: src, TargetLang: tgt})
	if err != nil {
		return nil, tm.ImportStats{}, usageErr("invalid languages %q/%q", src, tgt)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, tm.ImportStats{}, err
	}
	defer f.Close()
	st, err := tm.ImportTMX(ctx, mem, f, tm.ImportOptions{AllowPrimaryMatch: primary})
	if err != nil {
		return nil, st, fmt.Errorf("%s: %w", path, err)
	}
	return mem, st, nil
}

// readProjectFile extracts a document for analysis: Word files are
// segmented in the source language; bilingual files bring their own
// segments, languages and locks.
func readProjectFile(path, src string) (*filter.Extraction, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if _, ok := xliff.Detect(data); ok {
		ex, err := xliff.Extract(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		return ex, nil
	}
	if strings.EqualFold(filepath.Ext(path), ".docx") {
		if src == "" {
			return nil, usageErr("%s: -src is required for Word files", path)
		}
		ex, err := docx.Extract(data, src, docx.Options{})
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		return ex, nil
	}
	return nil, fmt.Errorf("%s: not a Word or XLIFF file", path)
}

func runAnalyze(args []string, _ io.Reader, stdout, stderr io.Writer) error {
	fs := newFlags("analyze", "file...", stderr)
	var tmxFiles, memNames listFlag
	fs.Var(&tmxFiles, "tmx", "TMX file to analyse against (repeatable)")
	fs.Var(&memNames, "tm", "stored memory to analyse against (repeatable)")
	dbPath := dbFlag(fs)
	src := fs.String("src", "", "source language (default: from bilingual files)")
	tgt := fs.String("tgt", "", "target language (default: from bilingual files)")
	primary := fs.Bool("primary", false, "accept TMX units whose languages match only by primary language")
	rule := fs.String("context", "previous", "context matches compare the segment before (previous, as Trados) or before and after (both, as memoQ)")
	noCross := fs.Bool("no-cross-file", false, "count repetitions across files by their matches instead of as cross-file repetitions")
	weights := fs.String("weights", "", "share of the full rate per band in percent, e.g. context=0,repetition=10,100=30,95=60")
	format := fs.String("format", "text", "report format: text, json, csv or html")
	out := fs.String("o", "", "write the report to this file (default: standard output)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return usageErr("expected one or more Word or XLIFF files")
	}
	switch *format {
	case "text", "json", "csv", "html":
	default:
		return usageErr("unknown format %q", *format)
	}
	w, err := analysis.ParseWeights(*weights)
	if err != nil {
		return usageErr("%v", err)
	}
	o := analysis.Options{SourceLang: *src, TargetLang: *tgt, Context: analysis.ContextRule(*rule), NoCrossFile: *noCross, Weights: w}
	var files []analysis.File
	for _, name := range fs.Args() {
		ex, err := readProjectFile(name, o.SourceLang)
		if err != nil {
			return err
		}
		if o.SourceLang == "" {
			o.SourceLang = ex.SourceLang
		}
		if o.TargetLang == "" {
			o.TargetLang = ex.TargetLang
		}
		f := analysis.File{Name: filepath.Base(name)}
		for _, u := range ex.Units {
			f.Segments = append(f.Segments, analysis.Segment{ID: u.ID, Source: u.Source, Locked: u.Locked})
		}
		files = append(files, f)
	}
	ctx := context.Background()
	var memories []tm.Memory
	if len(tmxFiles)+len(memNames) > 0 && (o.SourceLang == "" || o.TargetLang == "") {
		return usageErr("-src and -tgt are needed to analyse against memories")
	}
	for _, path := range tmxFiles {
		mem, st, err := loadTMX(ctx, path, o.SourceLang, o.TargetLang, *primary)
		if err != nil {
			return err
		}
		if st.Stored == 0 {
			fmt.Fprintf(stderr, "warning: %s has no units for %s to %s\n", path, o.SourceLang, o.TargetLang)
		}
		memories = append(memories, mem)
	}
	for _, name := range memNames {
		db, m, err := openMemory(ctx, *dbPath, name)
		if err != nil {
			return err
		}
		defer db.Close()
		memories = append(memories, m)
	}
	rep, err := analysis.Analyze(ctx, files, memories, o, time.Now())
	if err != nil {
		return err
	}
	dst := stdout
	if *out != "" {
		f, err := os.Create(*out)
		if err != nil {
			return err
		}
		defer f.Close()
		dst = f
	}
	var data []byte
	switch *format {
	case "json":
		data, err = rep.JSON()
		data = append(data, '\n')
	case "csv":
		data, err = rep.CSV()
	case "html":
		data = rep.HTML()
	default:
		data = []byte(analysisText(rep))
	}
	if err != nil {
		return err
	}
	_, err = dst.Write(data)
	return err
}

func analysisText(r analysis.Report) string {
	var b strings.Builder
	weighted := r.Weights != nil
	table := func(title string, rows []analysis.Row, total analysis.Counts, wsum float64) {
		fmt.Fprintf(&b, "%s\n", title)
		tw := tabwriter.NewWriter(&b, 0, 0, 2, ' ', tabwriter.AlignRight)
		head := "Band\tSegments\tWords\tCharacters\tTags\tPlaceables\t"
		if weighted {
			head += "Weighted\t"
		}
		fmt.Fprintln(tw, head)
		for _, row := range rows {
			if row.Segments == 0 {
				continue
			}
			line := fmt.Sprintf("%s\t%d\t%d\t%d\t%d\t%d\t", row.Name, row.Segments, row.Words, row.Characters, row.Tags, row.Placeables)
			if weighted {
				line += fmt.Sprintf("%.1f\t", row.Weighted)
			}
			fmt.Fprintln(tw, line)
		}
		line := fmt.Sprintf("Total\t%d\t%d\t%d\t%d\t%d\t", total.Segments, total.Words, total.Characters, total.Tags, total.Placeables)
		if weighted {
			line += fmt.Sprintf("%.1f\t", wsum)
		}
		fmt.Fprintln(tw, line)
		tw.Flush()
		b.WriteString("\n")
	}
	for _, f := range r.Files {
		table(f.Name, f.Rows, f.Total, f.Weighted)
	}
	if len(r.Files) > 1 {
		table("All files", r.Rows, r.Total, r.Weighted)
	}
	return b.String()
}
