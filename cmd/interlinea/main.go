// Command interlinea is the command-line interface to the Interlinea core:
// segmentation, stored translation memories, TMX inspection, lookup,
// concordance, Word documents and termbase files.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

type command struct {
	name    string
	summary string
	run     func(args []string, stdin io.Reader, stdout, stderr io.Writer) error
}

var commands = []command{
	{"segment", "split text into segments (SRX rules)", runSegment},
	{"tm", "manage stored translation memories (create, import, lookup, ...)", runTM},
	{"tmx-info", "show header and statistics of a TMX file", runTMXInfo},
	{"lookup", "fuzzy-match a segment directly against a TMX file", runLookup},
	{"concordance", "search a TMX file directly for a word or phrase", runConcordance},
	{"score", "show the match percentage between two segments", runScore},
	{"extract", "list the translatable units of a Word document", runExtract},
	{"merge", "write a translated copy of a Word document", runMerge},
	{"xliff", "bilingual XLIFF, SDLXLIFF and MQXLIFF files: info, extract, merge", runXLIFF},
	{"package", "Trados project packages: info, extract, return package", runPackage},
	{"term", "termbase files: info, convert (TBX, MultiTerm, CSV, Excel), duplicates", runTerm},
	{"termbase", "manage stored termbases (create, import, export, search, edit history, ...)", runTermbase},
	{"analyze", "count words and analyse files against translation memories", runAnalyze},
	{"qa", "check translations in bilingual files: tags, numbers, terms, spelling, ...", runQA},
	{"spell", "check spelling with a Hunspell dictionary and suggest corrections", runSpell},
	{"version", "print the version", runVersion},
}

// errUsage marks errors caused by wrong invocation (exit code 2).
var errUsage = errors.New("usage error")

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		usage(stderr)
		if len(args) == 0 {
			return 2
		}
		return 0
	}
	for _, c := range commands {
		if c.name != args[0] {
			continue
		}
		err := c.run(args[1:], stdin, stdout, stderr)
		switch {
		case err == nil:
			return 0
		case errors.Is(err, flag.ErrHelp):
			return 0
		case errors.Is(err, errUsage):
			fmt.Fprintf(stderr, "interlinea %s: %v\n", c.name, err)
			return 2
		default:
			fmt.Fprintf(stderr, "interlinea %s: %v\n", c.name, err)
			return 1
		}
	}
	fmt.Fprintf(stderr, "interlinea: unknown command %q\n\n", args[0])
	usage(stderr)
	return 2
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "Usage: interlinea <command> [options]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Commands:")
	for _, c := range commands {
		fmt.Fprintf(w, "  %-12s %s\n", c.name, c.summary)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Run 'interlinea <command> -h' for the options of a command.")
}

func newFlags(name, args string, stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintf(stderr, "Usage: interlinea %s [options] %s\n\nOptions:\n", name, args)
		fs.PrintDefaults()
	}
	return fs
}

// parseFlags parses args; a wrong option is a usage error (exit code 2).
func parseFlags(fs *flag.FlagSet, args []string) error {
	err := fs.Parse(args)
	if err == nil || errors.Is(err, flag.ErrHelp) {
		return err
	}
	return fmt.Errorf("%w: %v", errUsage, err)
}

func usageErr(format string, a ...any) error {
	return fmt.Errorf("%w: %s", errUsage, fmt.Sprintf(format, a...))
}

func runVersion(args []string, _ io.Reader, stdout, stderr io.Writer) error {
	fs := newFlags("version", "", stderr)
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "interlinea %s\n", version)
	return nil
}
