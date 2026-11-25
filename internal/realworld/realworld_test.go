// Package realworld tests the readers of Trados, MultiTerm and memoQ files
// on files those tools wrote (see testdata/README.md for where each comes
// from).
package realworld

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/4nonX/interlinea/filter"
	"github.com/4nonX/interlinea/filter/mqpackage"
	"github.com/4nonX/interlinea/filter/xliff"
	"github.com/4nonX/interlinea/term"
	"github.com/4nonX/interlinea/tm"
)

func read(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// translatable is what a bilingual file holds: its variant, languages,
// segments and locked segments.
type translatable struct {
	format, source, target string
	units, locked          int
}

var bilingual = map[string]translatable{
	"trados/disfr-change-tracking.sdlxliff":      {"sdlxliff", "en-us", "ja-JP", 5, 0},
	"trados/disfr-spelling-checker.doc.sdlxliff": {"sdlxliff", "en-US", "fr-FR", 14, 0},
	"trados/disfr-language-support.doc.sdlxliff": {"sdlxliff", "en-US", "fr-FR", 3, 0},
	"trados/disfr-new-features.ppt.sdlxliff":     {"sdlxliff", "en-US", "fr-FR", 7, 0},
	"trados/rws-quotes.docx.sdlxliff":            {"sdlxliff", "en-US", "it-IT", 8, 2},
	"trados/rws-tts.docx.sdlxliff":               {"sdlxliff", "en-us", "de-de", 4, 0},
	"trados/rws-quickplace.docx.sdlxliff":        {"sdlxliff", "en-US", "de-de", 8, 0},
	"trados/rws-photo-printer.doc.sdlxliff":      {"sdlxliff", "en-US", "de-DE", 19, 0},
	"trados/rws-perfect-match.docx.sdlxliff":     {"sdlxliff", "en-US", "de-de", 15, 0},
	"memoq/disfr-change-tracking.mqxliff":        {"mqxliff", "en", "ja", 5, 0},
	"memoq/disfr-git.html.mqxlz":                 {"mqxliff", "en", "ja", 11, 0},
	"memoq/xliffkit-ph.mqxliff":                  {"mqxliff", "en", "ja", 3, 0},
	"memoq/xliffkit-split.mqxliff":               {"mqxliff", "en", "ja", 33, 0},
	"memoq/xliffkit-translated.mqxliff":          {"mqxliff", "en", "ja", 33, 0},
}

// outside masks what a translation may change in an XLIFF file: targets,
// Trados's segment definitions and the start tags of translation units
// (which carry memoQ's and XLIFF's states). The rest must stay as it was.
var outside = regexp.MustCompile(`(?s)<target\b[^>]*/>|<target\b.*?</target>|<sdl:seg-defs\b.*?</sdl:seg-defs>|<trans-unit\b[^>]*>`)

// TestBilingualRoundTrip reads each file, translates every segment that
// is not locked (keeping the source's tags), writes the translations back
// and reads the result: the translations and states are there, the
// sources and everything outside the targets are as the tool wrote them.
func TestBilingualRoundTrip(t *testing.T) {
	for name, want := range bilingual {
		t.Run(name, func(t *testing.T) {
			file := read(t, name)
			doc := file
			if filepath.Ext(name) == ".mqxlz" {
				var err error
				if doc, err = mqpackage.ReadArchive(file); err != nil {
					t.Fatal(err)
				}
			}
			ex, err := xliff.Extract(doc)
			if err != nil {
				t.Fatal(err)
			}
			locked := 0
			for _, u := range ex.Units {
				if u.Locked {
					locked++
				}
			}
			got := translatable{ex.Format, ex.SourceLang, ex.TargetLang, len(ex.Units), locked}
			if got != want {
				t.Fatalf("read %+v, want %+v", got, want)
			}

			up := map[string]xliff.Update{}
			for _, u := range ex.Units {
				if !u.Locked {
					up[u.ID] = xliff.Update{Target: u.Source.Clone().Append(" (übersetzt, 翻訳)"), State: filter.StateTranslated}
				}
			}
			out, err := xliff.Merge(doc, up)
			if err != nil {
				t.Fatal(err)
			}
			again, err := xliff.Extract(out)
			if err != nil {
				t.Fatalf("the written file does not read: %v", err)
			}
			if len(again.Units) != len(ex.Units) {
				t.Fatalf("%d segments after writing, %d before", len(again.Units), len(ex.Units))
			}
			for i, u := range again.Units {
				before := ex.Units[i]
				if u.ID != before.ID || !u.Source.Equal(before.Source) {
					t.Fatalf("segment %s changed its source: %q, was %q", u.ID, u.Source.String(), before.Source.String())
				}
				w, ok := up[u.ID]
				if !ok {
					if !u.Target.Equal(before.Target) || u.State != before.State {
						t.Fatalf("locked segment %s changed", u.ID)
					}
					continue
				}
				if !u.Target.Normalize().Equal(w.Target.Normalize()) || u.State != filter.StateTranslated {
					t.Fatalf("segment %s: %q (%s), want %q", u.ID, u.Target.String(), u.State, w.Target.String())
				}
			}
			if a, b := outside.ReplaceAll(doc, nil), outside.ReplaceAll(out, nil); string(a) != string(b) {
				t.Fatalf("writing changed the file outside its targets")
			}

			if filepath.Ext(name) == ".mqxlz" {
				archive, err := mqpackage.WriteArchive(file, out)
				if err != nil {
					t.Fatal(err)
				}
				back, err := mqpackage.ReadArchive(archive)
				if err != nil || string(back) != string(out) {
					t.Fatalf("the archive does not hold the written document: %v", err)
				}
			}
		})
	}
}

// TestTradosOrigins: a segment from a memory or machine translation that
// is changed keeps where it came from in sdl:prev-origin, as Trados does.
func TestTradosOrigins(t *testing.T) {
	doc := read(t, "trados/rws-quotes.docx.sdlxliff")
	ex, err := xliff.Extract(doc)
	if err != nil {
		t.Fatal(err)
	}
	u := ex.Units[0]
	if u.Origin != "mt" {
		t.Fatalf("first segment: origin %q", u.Origin)
	}
	out, err := xliff.Merge(doc, map[string]xliff.Update{u.ID: {Target: u.Source.Clone().Append("!"), State: filter.StateTranslated}})
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`<sdl:seg id="1" conf="Translated" origin="interactive"><sdl:prev-origin origin="mt" origin-system="SDL Machine Translation"><sdl:prev-origin/><sdl:value key="SegmentIdentityHash">`)
	if !re.Match(out) {
		t.Fatalf("the segment's earlier origin is not kept:\n%s", regexp.MustCompile(`<sdl:seg id="1".{0,300}`).Find(out))
	}
}

// TestMultiTermTermbases reads the termbases as MultiTerm keeps them. The
// counts and statuses were taken from the files with an independent
// reader of Access databases.
func TestMultiTermTermbases(t *testing.T) {
	for _, c := range []struct {
		name                       string
		entries, terms             int
		approved, forbidden, other int
		languages                  []string
	}{
		{"trados/rws-sample.sdltb", 25, 80, 74, 6, 0, []string{"en-US", "de", "fr"}},
		{"trados/rws-printer.sdltb", 34, 146, 13, 13, 120, []string{"de", "en", "fr", "es", "ja", "pt", "zh"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			tb, rep, err := term.ReadSDLTB(read(t, c.name))
			if err != nil {
				t.Fatal(err)
			}
			if rep.Entries != c.entries || rep.Terms != c.terms || rep.Skipped != 0 || len(rep.Warnings) != 0 {
				t.Fatalf("report %+v", rep)
			}
			if len(tb.Languages) != len(c.languages) {
				t.Fatalf("languages %v, want %v", tb.Languages, c.languages)
			}
			for i, l := range c.languages {
				if tb.Languages[i] != l {
					t.Fatalf("languages %v, want %v", tb.Languages, c.languages)
				}
			}
			var approved, forbidden, other int
			for _, e := range tb.Entries {
				for _, l := range e.Languages {
					for _, tm := range l.Terms {
						switch tm.Status {
						case term.StatusApproved:
							approved++
						case term.StatusForbidden:
							forbidden++
						default:
							other++
						}
					}
				}
			}
			if approved != c.approved || forbidden != c.forbidden || other != c.other {
				t.Fatalf("statuses: %d approved, %d forbidden, %d other", approved, forbidden, other)
			}
		})
	}
}

// TestMultiTermCaseVariants: the printer termbase keeps the brand name
// "MistJet" as the preferred spelling and "Mistjet" as a forbidden one in
// the same entry. Both are read, and a check finds the forbidden one only.
func TestMultiTermCaseVariants(t *testing.T) {
	tb, _, err := term.ReadSDLTB(read(t, "trados/rws-printer.sdltb"))
	if err != nil {
		t.Fatal(err)
	}
	var mistjet *term.Entry
	for i := range tb.Entries {
		if tb.Entries[i].ID == "24" {
			mistjet = &tb.Entries[i]
		}
	}
	if mistjet == nil {
		t.Fatal("entry 24 is missing")
	}
	var en []term.Term
	for _, l := range mistjet.Languages {
		if l.Lang == "en" {
			en = l.Terms
		}
	}
	if len(en) != 2 || en[0].Text != "MistJet" || en[0].Status != term.StatusApproved || en[1].Text != "Mistjet" || en[1].Status != term.StatusForbidden {
		t.Fatalf("English terms: %+v", en)
	}
	x := term.NewIndex()
	x.Add("printer", tb.Entries)
	hits := x.Recognize("Der MistJet druckt schnell.", "de", "en")
	if issues := x.Check(hits, "The MistJet prints fast.", "en"); len(issues) != 0 {
		t.Fatalf("the preferred spelling is reported: %+v", issues)
	}
	issues := x.Check(hits, "The Mistjet prints fast.", "en")
	if len(issues) != 1 || issues[0].Kind != "forbidden" || issues[0].Found != "Mistjet" {
		t.Fatalf("the forbidden spelling: %+v", issues)
	}
}

// TestMemoryFiles reads the Trados Studio and Wordfast memories. The counts
// of units and tags were taken from the files with SQLite itself and by
// counting their lines.
func TestMemoryFiles(t *testing.T) {
	for _, c := range []struct {
		name, format, memory, source, target string
		units, tags                          int
	}{
		{"trados/sdltm-importer-fr-en.sdltm", tm.FormatSDLTM, "French to US English", "fr-FR", "en-US", 537, 1178},
		{"trados/sdltm-importer-es-de.sdltm", tm.FormatSDLTM, "Alfredo_Texas_enus-de", "en-US", "de-DE", 102, 198},
		{"trados/disfr-tea-party.sdltm", tm.FormatSDLTM, "tea-party", "en-GB", "ja-JP", 9, 0},
		{"trados/rws-general-en-de.sdltm", tm.FormatSDLTM, "General English - German", "en-US", "de-DE", 3, 0},
		{"wordfast/txt-tm-importer-pt-en.txt", tm.FormatWordfast, "", "pt-BR", "en-US", 407, 458},
		{"wordfast/txt-tm-importer-en-es.txt", tm.FormatWordfast, "", "en", "es", 1859, 2028},
	} {
		t.Run(c.name, func(t *testing.T) {
			data := read(t, c.name)
			f, err := tm.ReadFile(data)
			if err != nil {
				t.Fatal(err)
			}
			tags := 0
			for _, u := range f.Units {
				tags += len(u.Source.Tags()) + len(u.Target.Tags())
				if err := tm.ValidateUnit(u.Unit); err != nil && len(u.Source) > 0 && len(u.Target) > 0 {
					t.Fatalf("unit %q: %v", u.Source.String(), err)
				}
			}
			if f.Format != c.format || f.Name != c.memory || f.SourceLang != c.source || f.TargetLang != c.target || len(f.Units) != c.units || f.Count != c.units || f.Skipped != 0 || tags != c.tags {
				t.Fatalf("%s %q %s>%s: %d units, %d tags, %d skipped", f.Format, f.Name, f.SourceLang, f.TargetLang, len(f.Units), tags, f.Skipped)
			}
			// Into a memory of its languages, every unit with a source and
			// a target is stored and found again.
			m, err := tm.NewInMemory(tm.Info{Name: "m", SourceLang: c.source, TargetLang: c.target})
			if err != nil {
				t.Fatal(err)
			}
			st, err := tm.Import(context.Background(), m, data, tm.ImportOptions{})
			if err != nil || st.Read != c.units || st.Stored+st.Skipped != c.units || st.Stored < c.units*9/10 {
				t.Fatalf("import %+v %v", st, err)
			}
			u := f.Units[len(f.Units)/2]
			ms, err := m.Lookup(context.Background(), u.Source, tm.LookupOptions{MinScore: 100})
			if err != nil || len(ms) == 0 || ms[0].Score < 100 {
				t.Fatalf("lookup of %q: %+v %v", u.Source.String(), ms, err)
			}
		})
	}
}
