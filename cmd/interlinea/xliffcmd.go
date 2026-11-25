package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter"
	"github.com/4nonX/interlinea/filter/xliff"
)

var xliffCommands = []command{
	{"info", "show the variant, languages and segment states of an XLIFF file", runXLIFFInfo},
	{"extract", "list the segments of an XLIFF file with their translations", runXLIFFExtract},
	{"merge", "write translations and states into an XLIFF file", runXLIFFMerge},
}

func runXLIFF(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprintln(stderr, "Usage: interlinea xliff <command> [options] file")
		fmt.Fprintln(stderr)
		fmt.Fprintln(stderr, "Commands:")
		for _, c := range xliffCommands {
			fmt.Fprintf(stderr, "  %-8s %s\n", c.name, c.summary)
		}
		fmt.Fprintln(stderr, "\nReads XLIFF 1.2, Trados SDLXLIFF, memoQ MQXLIFF and XLIFF 2.")
		if len(args) == 0 {
			return usageErr("missing xliff command")
		}
		return nil
	}
	for _, c := range xliffCommands {
		if c.name == args[0] {
			return c.run(args[1:], stdin, stdout, stderr)
		}
	}
	return usageErr("unknown xliff command %q", args[0])
}

func readXLIFF(name string) ([]byte, *filter.Extraction, error) {
	data, err := os.ReadFile(name)
	if err != nil {
		return nil, nil, err
	}
	ex, err := xliff.Extract(data)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", name, err)
	}
	return data, ex, nil
}

var stateOrder = []filter.State{filter.StateNew, filter.StateDraft, filter.StateTranslated, filter.StateReviewed, filter.StateFinal, filter.StateRejected}

func runXLIFFInfo(args []string, _ io.Reader, stdout, stderr io.Writer) error {
	fs := newFlags("xliff info", "file", stderr)
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return usageErr("expected one file")
	}
	_, ex, err := readXLIFF(fs.Arg(0))
	if err != nil {
		return err
	}
	counts := map[filter.State]int{}
	words := map[filter.State]int{}
	locked := 0
	for _, u := range ex.Units {
		counts[u.State]++
		words[u.State] += len(strings.Fields(u.Source.Plain()))
		if u.Locked {
			locked++
		}
	}
	fmt.Fprintf(stdout, "Format:    %s\n", ex.Format)
	fmt.Fprintf(stdout, "Languages: %s → %s\n", ex.SourceLang, ex.TargetLang)
	fmt.Fprintf(stdout, "Segments:  %d (%d locked)\n", len(ex.Units), locked)
	for _, s := range stateOrder {
		if counts[s] > 0 {
			fmt.Fprintf(stdout, "  %-11s %6d segments %8d words\n", s, counts[s], words[s])
		}
	}
	printWarnings(stderr, ex.Warnings)
	return nil
}

// showContent writes content with tags as {id}, {/id} and {id/}.
func showContent(c content.Content) string {
	var b strings.Builder
	for _, r := range c {
		if !r.IsTag() {
			b.WriteString(r.Text)
			continue
		}
		switch r.Tag.Kind {
		case content.TagOpen:
			b.WriteString("{" + r.Tag.ID + "}")
		case content.TagClose:
			b.WriteString("{/" + r.Tag.ID + "}")
		default:
			b.WriteString("{" + r.Tag.ID + "/}")
		}
	}
	return b.String()
}

func runXLIFFExtract(args []string, _ io.Reader, stdout, stderr io.Writer) error {
	fs := newFlags("xliff extract", "file", stderr)
	asJSON := fs.Bool("json", false, "print the extraction as JSON (units with runs, targets and states)")
	state := fs.String("state", "", "only segments in this state: new, draft, translated, reviewed, final or rejected")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return usageErr("expected one file")
	}
	if *state != "" && !filter.State(*state).Valid() {
		return usageErr("-state must be one of new, draft, translated, reviewed, final, rejected")
	}
	_, ex, err := readXLIFF(fs.Arg(0))
	if err != nil {
		return err
	}
	if *state != "" {
		var keep []filter.Unit
		for _, u := range ex.Units {
			if string(u.State) == *state {
				keep = append(keep, u)
			}
		}
		ex.Units = keep
	}
	if *asJSON {
		if ex.Units == nil {
			ex.Units = []filter.Unit{}
		}
		return printJSON(stdout, ex)
	}
	for _, u := range ex.Units {
		lock := ""
		if u.Locked {
			lock = " locked"
		}
		match := ""
		if u.Match > 0 {
			match = fmt.Sprintf(" %d%%", u.Match)
		}
		fmt.Fprintf(stdout, "%s [%s%s%s]\n  %s\n  %s\n", u.ID, u.State, match, lock, showContent(u.Source), showContent(u.Target))
	}
	printWarnings(stderr, ex.Warnings)
	return nil
}

// parseUpdates reads {"id": {"target": runs or "text", "state": "..."}} or
// {"id": "text"} (a translation, state translated).
func parseUpdates(b []byte) (map[string]xliff.Update, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	out := make(map[string]xliff.Update, len(m))
	ids := make([]string, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		v := strings.TrimSpace(string(m[id]))
		if strings.HasPrefix(v, `"`) {
			var text string
			if err := json.Unmarshal(m[id], &text); err != nil {
				return nil, fmt.Errorf("segment %s: %w", id, err)
			}
			out[id] = xliff.Update{Target: content.Text(text), State: filter.StateTranslated}
			continue
		}
		var raw struct {
			Target json.RawMessage `json:"target"`
			State  filter.State    `json:"state"`
		}
		if err := json.Unmarshal(m[id], &raw); err != nil {
			return nil, fmt.Errorf("segment %s: %w", id, err)
		}
		u := xliff.Update{State: raw.State}
		if t := strings.TrimSpace(string(raw.Target)); t != "" && t != "null" {
			if strings.HasPrefix(t, `"`) {
				var text string
				if err := json.Unmarshal(raw.Target, &text); err != nil {
					return nil, fmt.Errorf("segment %s: %w", id, err)
				}
				u.Target = content.Text(text)
			} else {
				var c content.Content
				if err := c.UnmarshalJSON(raw.Target); err != nil {
					return nil, fmt.Errorf("segment %s: %w", id, err)
				}
				u.Target = c
			}
		}
		out[id] = u
	}
	return out, nil
}

func runXLIFFMerge(args []string, _ io.Reader, stdout, stderr io.Writer) error {
	fs := newFlags("xliff merge", "file updates.json", stderr)
	outName := fs.String("o", "", "output file (required; may be the input file)")
	tgt := fs.String("tgt", "", "target language, for a file that names none (an unsegmented TagEditor file)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		return usageErr("expected a bilingual file and an updates file")
	}
	if *outName == "" {
		return usageErr("-o is required")
	}
	data, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(fs.Arg(1))
	if err != nil {
		return err
	}
	up, err := parseUpdates(raw)
	if err != nil {
		return fmt.Errorf("%s: %w", fs.Arg(1), err)
	}
	out, err := xliff.MergeTo(data, up, *tgt)
	if err != nil {
		return err
	}
	if err := os.WriteFile(*outName, out, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "wrote %s (%d segments updated)\n", *outName, len(up))
	return nil
}
