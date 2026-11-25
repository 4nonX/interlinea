package typeset

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/4nonX/interlinea/filter/pdf"
	"github.com/4nonX/interlinea/typeset/fonts"
	"github.com/4nonX/interlinea/typeset/sfnt"
)

func set(t *testing.T, system bool) *FontSet {
	fs, err := NewFontSet(fonts.All(), system)
	if err != nil {
		t.Fatal(err)
	}
	return fs
}

func TestArabicForms(t *testing.T) {
	// beh, alef, beh: initial, final (alef joins right only), isolated.
	got := arabicFormsOf([]rune{0x0628, 0x0627, 0x0628})
	want := []int{formInitial, formFinal, formIsolated}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("forms %v, want %v", got, want)
		}
	}
	// beh beh beh: initial, medial, final; a mark in between changes nothing.
	got = arabicFormsOf([]rune{0x0628, 0x064E, 0x0628, 0x0628})
	if got[0] != formInitial || got[1] != -1 || got[2] != formMedial || got[3] != formFinal {
		t.Errorf("forms with a mark: %v", got)
	}
}

func TestShapingUsesJoinedGlyphs(t *testing.T) {
	fs := set(t, false)
	items := fs.shapeSpan("بب", Style{Lang: "ar"}, 10, 0)
	if len(items) != 2 || items[0].glyphs[0] == items[1].glyphs[0] {
		t.Errorf("an initial and a final beh are different glyphs: %+v", items)
	}
	lam := fs.shapeSpan("لا", Style{Lang: "ar"}, 10, 0)
	if len(lam) != 1 || lam[0].text != "لا" {
		t.Errorf("lam with alef is one glyph that stands for both: %+v", lam)
	}
}

func TestReorder(t *testing.T) {
	fs := set(t, false)
	text := func(its []item) string {
		var b strings.Builder
		for _, it := range its {
			b.WriteString(it.text)
		}
		return b.String()
	}
	// Hebrew in a left-to-right line is read right to left, the rest stays.
	got := text(reorder(fs.shapeSpan("ab שלום cd", Style{}, 10, 0), false))
	if got != "ab םולש cd" {
		t.Errorf("left to right: %q", got)
	}
	// In a right-to-left line a number keeps its order and brackets turn.
	// The closing bracket comes first and is drawn as an opening one.
	items := reorder(fs.shapeSpan("שלום (12)", Style{}, 10, 0), true)
	if got := text(items); got != ")12( םולש" {
		t.Errorf("right to left: %q", got)
	}
	if f := items[0].face.Font; items[0].glyphs[0] != f.Glyph('(') {
		t.Errorf("the bracket is mirrored")
	}
}

func TestBreaksLines(t *testing.T) {
	fs := set(t, false)
	ls := fs.lines(&Para{Spans: []Span{{Text: strings.Repeat("word ", 40)}}, Size: 10}, 100)
	if len(ls) < 5 {
		t.Fatalf("lines: %d", len(ls))
	}
	for _, l := range ls {
		if l.width > 100.01 {
			t.Errorf("a line is wider than the column: %.1f", l.width)
		}
		if strings.HasSuffix(l.items[len(l.items)-1].text, " ") {
			t.Errorf("a line ends in a space")
		}
	}
	// A line break in the text starts a new line.
	if n := len(fs.lines(&Para{Spans: []Span{{Text: "a\nb"}}}, 300)); n != 2 {
		t.Errorf("a line break makes two lines: %d", n)
	}
}

func TestSubsetIsAFont(t *testing.T) {
	b := fonts.All()[0]
	f, err := sfnt.Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	used := map[uint16]bool{f.Glyph('A'): true, f.Glyph('é'): true}
	sub, err := sfnt.Parse(f.Subset(used))
	if err != nil {
		t.Fatalf("the subset reads as a font: %v", err)
	}
	if sub.NumGlyphs != f.NumGlyphs || sub.Advance(f.Glyph('A')) != f.Advance(f.Glyph('A')) {
		t.Errorf("glyph numbers and widths stay as they are")
	}
	if len(f.Subset(used)) > len(b)/4 {
		t.Errorf("the subset is much smaller than the font: %d of %d", len(f.Subset(used)), len(b))
	}
}

func TestPDFReadsBack(t *testing.T) {
	fs := set(t, true)
	d := NewDocument(fs, A4Width, A4Height)
	d.Title = "Test"
	d.Columns = 2
	d.Head = func(a, b string) string { return a + " – " + b }
	for i := 0; i < 200; i++ {
		d.Add(&Para{Spans: []Span{{Text: "Kaution", Style: Style{Bold: true}}, {Text: " Sicherheit, die der Mieter leistet. Ελληνικά Кириллица"}}, Mark: "Kaution"})
	}
	d.Add(&Para{Spans: []Span{{Text: "שכר דירה", Style: Style{Lang: "he"}}}, RTL: true})
	data := d.PDF(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	doc, err := pdf.Read(data)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Pages != d.Pages() || doc.Pages < 3 {
		t.Errorf("pages: %d, %d", doc.Pages, d.Pages())
	}
	var all strings.Builder
	for _, p := range doc.Paragraphs {
		all.WriteString(p.Text + "\n")
	}
	for _, want := range []string{"Sicherheit, die der Mieter leistet", "Ελληνικά", "Кириллица", "Kaution – Kaution"} {
		if !strings.Contains(all.String(), want) {
			t.Errorf("text lacks %q", want)
		}
	}
}

// TestSystemCJK uses a Chinese or Japanese font of the computer, if it has
// one: a CFF font is subset too.
func TestSystemCJK(t *testing.T) {
	fs := set(t, true)
	items := fs.shapeSpan("押金", Style{Lang: "ja"}, 10, 0)
	if len(fs.Missing()) > 0 {
		t.Skip("no font for Japanese on this computer")
	}
	d := NewDocument(fs, A4Width, A4Height)
	d.Add(&Para{Spans: []Span{{Text: "押金と敷金", Style: Style{Lang: "ja"}}}})
	data := d.PDF(time.Now())
	if items[0].face.Font.CFF && !strings.Contains(string(data), "/CIDFontType0C") {
		t.Errorf("a CFF font is embedded as CFF")
	}
	if len(data) > 4<<20 {
		t.Errorf("the PDF stays small: %d bytes", len(data))
	}
	if os.Getenv("INTERLINEA_TYPESET_OUT") != "" {
		os.WriteFile(os.Getenv("INTERLINEA_TYPESET_OUT"), data, 0o644)
	}
}
