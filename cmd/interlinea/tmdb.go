package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/tm"
	"github.com/4nonX/interlinea/tm/tmdb"
)

// defaultDBPath returns the database used when -db is not given:
// $INTERLINEA_DB, or "Interlinea/memories.db" in the user configuration
// directory (%AppData% on Windows, ~/Library/Application Support on macOS,
// $XDG_CONFIG_HOME or ~/.config on Linux).
func defaultDBPath() string {
	if p := os.Getenv("INTERLINEA_DB"); p != "" {
		return p
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "memories.db"
	}
	return filepath.Join(dir, "Interlinea", "memories.db")
}

var tmCommands = []command{
	{"create", "create a translation memory", runTMCreate},
	{"list", "list translation memories", runTMList},
	{"delete", "delete a translation memory and all its units", runTMDelete},
	{"import", "import a memory file (TMX, Trados .sdltm, Wordfast .txt) into a memory", runTMImport},
	{"export", "export a memory as TMX", runTMExport},
	{"lookup", "fuzzy-match a segment against a memory", runTMLookup},
	{"concordance", "search a memory for a word or phrase", runTMConcordance},
}

func runTM(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprintln(stderr, "Usage: interlinea tm <command> [options]")
		fmt.Fprintln(stderr)
		fmt.Fprintln(stderr, "Commands:")
		for _, c := range tmCommands {
			fmt.Fprintf(stderr, "  %-12s %s\n", c.name, c.summary)
		}
		fmt.Fprintf(stderr, "\nMemories are stored in %s unless -db is given.\n", defaultDBPath())
		if len(args) == 0 {
			return usageErr("missing tm command")
		}
		return nil
	}
	for _, c := range tmCommands {
		if c.name == args[0] {
			return c.run(args[1:], stdin, stdout, stderr)
		}
	}
	return usageErr("unknown tm command %q", args[0])
}

func dbFlag(fs *flag.FlagSet) *string {
	return fs.String("db", defaultDBPath(), "database file")
}

func openMemory(ctx context.Context, dbPath, name string) (*tmdb.DB, *tmdb.Memory, error) {
	if strings.TrimSpace(name) == "" {
		return nil, nil, usageErr("-name is required")
	}
	db, err := tmdb.Open(dbPath)
	if err != nil {
		return nil, nil, err
	}
	m, err := db.Memory(ctx, name)
	if err != nil {
		db.Close()
		if errors.Is(err, tmdb.ErrNotFound) {
			return nil, nil, fmt.Errorf("no memory named %q in %s", name, dbPath)
		}
		return nil, nil, err
	}
	return db, m, nil
}

func runTMCreate(args []string, _ io.Reader, stdout, stderr io.Writer) error {
	fs := newFlags("tm create", "", stderr)
	dbPath := dbFlag(fs)
	name := fs.String("name", "", "memory name (required)")
	src := fs.String("src", "", "source language, e.g. de-DE (required)")
	tgt := fs.String("tgt", "", "target language, e.g. en-GB (required)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return usageErr("unexpected arguments: %v", fs.Args())
	}
	db, err := tmdb.Open(*dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	m, err := db.Create(context.Background(), tm.Info{Name: *name, SourceLang: *src, TargetLang: *tgt})
	if errors.Is(err, tm.ErrInvalidInfo) {
		return usageErr("-name, -src and -tgt are required and must be valid")
	}
	if err != nil {
		return err
	}
	info := m.Info()
	fmt.Fprintf(stdout, "Created %q (%s → %s) in %s\n", info.Name, info.SourceLang, info.TargetLang, *dbPath)
	return nil
}

func runTMList(args []string, _ io.Reader, stdout, stderr io.Writer) error {
	fs := newFlags("tm list", "", stderr)
	dbPath := dbFlag(fs)
	asJSON := fs.Bool("json", false, "print as JSON")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	db, err := tmdb.Open(*dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	list, err := db.List(context.Background())
	if err != nil {
		return err
	}
	if *asJSON {
		type entry struct {
			Name       string `json:"name"`
			SourceLang string `json:"sourceLang"`
			TargetLang string `json:"targetLang"`
			Units      int    `json:"units"`
			CreatedAt  string `json:"createdAt"`
		}
		out := make([]entry, 0, len(list))
		for _, s := range list {
			out = append(out, entry{s.Name, s.SourceLang, s.TargetLang, s.Units, s.CreatedAt.Format(time.RFC3339)})
		}
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}
	if len(list) == 0 {
		fmt.Fprintf(stdout, "No memories in %s.\n", *dbPath)
		return nil
	}
	for _, s := range list {
		fmt.Fprintf(stdout, "%-30s %-8s → %-8s %8d units  created %s\n",
			s.Name, s.SourceLang, s.TargetLang, s.Units, s.CreatedAt.Local().Format("2006-01-02"))
	}
	return nil
}

func runTMDelete(args []string, _ io.Reader, stdout, stderr io.Writer) error {
	fs := newFlags("tm delete", "", stderr)
	dbPath := dbFlag(fs)
	name := fs.String("name", "", "memory name (required)")
	yes := fs.Bool("yes", false, "confirm permanent deletion (required)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if strings.TrimSpace(*name) == "" {
		return usageErr("-name is required")
	}
	db, err := tmdb.Open(*dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	ctx := context.Background()
	m, err := db.Memory(ctx, *name)
	if errors.Is(err, tmdb.ErrNotFound) {
		return fmt.Errorf("no memory named %q in %s", *name, *dbPath)
	}
	if err != nil {
		return err
	}
	n, err := m.Count(ctx)
	if err != nil {
		return err
	}
	if !*yes {
		return usageErr("this permanently deletes %q and its %d units; repeat with -yes to confirm", m.Info().Name, n)
	}
	if err := db.Delete(ctx, *name); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Deleted %q (%d units)\n", m.Info().Name, n)
	return nil
}

func runTMImport(args []string, _ io.Reader, stdout, stderr io.Writer) error {
	fs := newFlags("tm import", "<file.tmx|file.sdltm|wordfast.txt>", stderr)
	dbPath := dbFlag(fs)
	name := fs.String("name", "", "memory name (required)")
	overwrite := fs.Bool("overwrite", false, "replace existing translations of the same source instead of keeping both")
	primary := fs.Bool("primary", false, "also accept variants that match only by primary language (de-AT for de-DE)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return usageErr("expected one memory file")
	}
	ctx := context.Background()
	db, m, err := openMemory(ctx, *dbPath, *name)
	if err != nil {
		return err
	}
	defer db.Close()
	data, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return err
	}
	policy := tm.KeepBoth
	if *overwrite {
		policy = tm.Overwrite
	}
	start := time.Now()
	st, err := tm.Import(ctx, m, data, tm.ImportOptions{Policy: policy, AllowPrimaryMatch: *primary})
	if err != nil {
		return fmt.Errorf("%s: %w", fs.Arg(0), err)
	}
	fmt.Fprintf(stdout, "Read %d units: %d stored, %d skipped (no usable %s/%s pair) in %s\n",
		st.Read, st.Stored, st.Skipped, m.Info().SourceLang, m.Info().TargetLang, time.Since(start).Round(time.Millisecond))
	return nil
}

func runTMExport(args []string, _ io.Reader, stdout, stderr io.Writer) error {
	fs := newFlags("tm export", "", stderr)
	dbPath := dbFlag(fs)
	name := fs.String("name", "", "memory name (required)")
	outPath := fs.String("o", "", "output file (default: standard output)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	ctx := context.Background()
	db, m, err := openMemory(ctx, *dbPath, *name)
	if err != nil {
		return err
	}
	defer db.Close()
	if *outPath == "" {
		return tm.ExportTMX(ctx, m, stdout, version)
	}
	f, err := os.Create(*outPath)
	if err != nil {
		return err
	}
	if err := tm.ExportTMX(ctx, m, f, version); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func runTMLookup(args []string, _ io.Reader, stdout, stderr io.Writer) error {
	fs := newFlags("tm lookup", "<segment text>", stderr)
	dbPath := dbFlag(fs)
	name := fs.String("name", "", "memory name (required)")
	minScore := fs.Int("min", 70, "minimum match percentage")
	limit := fs.Int("n", 5, "maximum number of matches")
	asJSON := fs.Bool("json", false, "print results as JSON")
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
	db, m, err := openMemory(ctx, *dbPath, *name)
	if err != nil {
		return err
	}
	defer db.Close()
	matches, err := m.Lookup(ctx, content.Text(strings.Join(fs.Args(), " ")), tm.LookupOptions{MinScore: *minScore, Limit: *limit})
	if err != nil {
		return err
	}
	return printMatches(stdout, matches, *asJSON)
}

func runTMConcordance(args []string, _ io.Reader, stdout, stderr io.Writer) error {
	fs := newFlags("tm concordance", "<word or phrase>", stderr)
	dbPath := dbFlag(fs)
	name := fs.String("name", "", "memory name (required)")
	inTarget := fs.Bool("target", false, "search the target language instead of the source")
	limit := fs.Int("n", 20, "maximum number of results")
	asJSON := fs.Bool("json", false, "print results as JSON")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return usageErr("expected a word or phrase")
	}
	ctx := context.Background()
	db, m, err := openMemory(ctx, *dbPath, *name)
	if err != nil {
		return err
	}
	defer db.Close()
	units, err := m.Concordance(ctx, strings.Join(fs.Args(), " "), tm.ConcordanceOptions{InTarget: *inTarget, Limit: *limit})
	if err != nil {
		return err
	}
	return printUnits(stdout, units, *asJSON)
}
