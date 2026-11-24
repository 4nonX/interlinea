package term

import (
	"sort"
	"strings"
	"testing"
)

// A glossary as tools really write them: a bare ampersand in the text, an
// element of the tool's own, a block for each term of a language (as
// MultiTerm exports them), fields named like language tags, semicolons
// inside terms and values, and a value of several lines.
const awkwardTBX = `<?xml version="1.0" encoding="UTF-8"?>
<martif type="TBX" xml:lang="en"><text><body>
<termEntry id="c1">
  <ShortInfo>(syn & lay) leververgroting</ShortInfo>
  <descripGrp><descrip type="Nl-term">clubbing, horlogeglasnagels</descrip></descripGrp>
  <descripGrp><descrip type="En-term">clubbing</descrip></descripGrp>
  <descripGrp><descrip type="Note">Kept as a field</descrip></descripGrp>
  <langSet xml:lang="EN"><tig><term>hepatomegaly</term>
    <descripGrp><descrip type="Contexten">First line; with a semicolon.
Second line.</descrip></descripGrp></tig></langSet>
  <langSet xml:lang="NL"><tig><term>hepatomegalie</term></tig></langSet>
  <langSet xml:lang="NL"><tig><term>leververgroting</term>
    <descripGrp><descrip type="Flexie">de; -en</descrip></descripGrp></tig></langSet>
  <langSet xml:lang="NL"><tig><term>lever; vergrote</term></tig></langSet>
</termEntry>
<termEntry id="c2">
  <langSet xml:lang="EN"><tig><term>R&amp;D &#160;unit</term></tig></langSet>
  <langSet xml:lang="NL"><tig><term>O&O-afdeling</term></tig></langSet>
</termEntry>
</body></text></martif>`

// flat lists everything an entry holds, to compare two termbases whatever
// the order of their parts.
func flat(tb *Termbase) []string {
	var out []string
	for _, e := range tb.Entries {
		var parts []string
		for _, f := range e.Fields {
			parts = append(parts, "E|"+f.Field+"="+f.Value)
		}
		for _, l := range e.Languages {
			for _, f := range l.Fields {
				parts = append(parts, "L|"+strings.ToLower(l.Lang)+"|"+f.Field+"="+f.Value)
			}
			for _, t := range l.Terms {
				parts = append(parts, "T|"+strings.ToLower(l.Lang)+"|"+t.Text+"|"+t.Status)
				for _, f := range t.Fields {
					parts = append(parts, "TF|"+strings.ToLower(l.Lang)+"|"+t.Text+"|"+f.Field+"="+f.Value)
				}
			}
		}
		sort.Strings(parts)
		out = append(out, strings.Join(parts, "\n"))
	}
	sort.Strings(out)
	return out
}

func TestAwkwardGlossary(t *testing.T) {
	tb, _, err := ReadTBX([]byte(awkwardTBX))
	if err != nil {
		t.Fatal(err)
	}
	if len(tb.Entries) != 2 {
		t.Fatalf("%d entries", len(tb.Entries))
	}
	e := tb.Entries[0]
	// The bare ampersand is read as written, and the tool's own element kept.
	if got := firstValue(e.Fields, "ShortInfo"); got != "(syn & lay) leververgroting" {
		t.Errorf("ShortInfo = %q", got)
	}
	// The three Dutch blocks are one language with three terms.
	if len(e.Languages) != 2 || e.Lang("nl") == nil || len(e.Lang("nl").Terms) != 3 {
		t.Fatalf("languages = %+v", e.Languages)
	}
	if got := tb.Entries[1].Lang("nl").Terms[0].Text; got != "O&O-afdeling" {
		t.Errorf("term with a bare ampersand = %q", got)
	}
	if got := tb.Entries[1].Lang("en").Terms[0].Text; got != "R&D \u00a0unit" {
		t.Errorf("term with references = %q", got)
	}

	want := flat(tb)
	for _, f := range []Format{FormatTBX, FormatTBX2, FormatMultiTerm, FormatXLSX, FormatCSV, FormatTSV} {
		out, err := Write(tb, f)
		if err != nil {
			t.Errorf("%s: write: %v", f, err)
			continue
		}
		back, _, err := Read(f, out, nil)
		if err != nil {
			t.Errorf("%s: read: %v", f, err)
			continue
		}
		got := flat(back)
		if strings.Join(got, "\n--\n") != strings.Join(want, "\n--\n") {
			t.Errorf("%s: after writing and reading\n%s\nwant\n%s", f, strings.Join(got, "\n--\n"), strings.Join(want, "\n--\n"))
		}
	}
}

func TestRepairAmpersands(t *testing.T) {
	for in, want := range map[string]string{
		"a & b":                     "a &amp; b",
		"a &amp; b &#160; &#xA0; c": "a &amp; b &#160; &#xA0; c",
		"R&D":                       "R&amp;D",
		"&nbsp;&":                   "&nbsp;&amp;",
		"<![CDATA[a & b]]> & c":     "<![CDATA[a & b]]> &amp; c",
		"<!-- a & b --> x":          "<!-- a & b --> x",
		"&;&#;":                     "&amp;;&amp;#;",
	} {
		if got := string(repairAmpersands([]byte(in))); got != want {
			t.Errorf("repairAmpersands(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHeadingsThatLookLikeTags(t *testing.T) {
	for _, h := range []string{"Nl-term", "En-term", "Note", "Term", "Source", "Begrip", "Datum"} {
		if l, ok := LangFromName(h); ok {
			t.Errorf("LangFromName(%q) = %q; it is a field", h, l)
		}
	}
	for h, want := range map[string]string{
		"NL": "nl", "en-GB": "en-GB", "zh_TW": "zh-TW", "sr-Latn-RS": "sr-Latn-RS", "Thai": "th", "ไทย": "th",
		"Thai (Thailand)": "th-TH", "Deutsch": "de", "Hrvatski": "hr", "es-419": "es-419",
	} {
		if l, ok := LangFromName(h); !ok || l != want {
			t.Errorf("LangFromName(%q) = %q, %v; want %q", h, l, ok, want)
		}
	}
}

func TestCellValuesWithSemicolons(t *testing.T) {
	in := []string{"lever; vergrote", `C:\tmp`, "", "plain"}
	got := splitCell(joinCell(in), ";")
	if strings.Join(got, "|") != strings.Join(in, "|") {
		t.Fatalf("round trip = %q", got)
	}
	// A sheet from elsewhere, without escapes, splits as before.
	if got := splitCell("a; b;c", ";"); strings.Join(got, "|") != "a|b|c" {
		t.Fatalf("plain split = %q", got)
	}
}
