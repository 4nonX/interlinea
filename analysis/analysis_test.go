package analysis

import (
	"bytes"
	"context"
	"encoding/csv"
	"strings"
	"testing"
	"time"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/tm"
)

func TestWords(t *testing.T) {
	cases := map[string]int{
		"Der Mieter zahlt 1.250,50 EUR.": 5,
		"well-known l'été don’t":         3,
		"  ":                             0,
		"Hello, world!":                  2,
		"日本語のテキスト":                       8,
		"한국어 문장입니다":                      2,
		"ภาษาไทย":                        7,
		"A - B":                          2,
		"Section 3.2.1 and 10 000 users": 5,
		"e-mail-Adresse":                 1,
	}
	for text, want := range cases {
		if got := len(Words(text)); got != want {
			t.Errorf("Words(%q) = %d %q, want %d", text, got, Words(text), want)
		}
	}
	c := content.Text("Pay ").AppendTag(content.Tag{Kind: content.TagOpen, ID: "1"}).Append("1,250").AppendTag(content.Tag{Kind: content.TagClose, ID: "1"}).Append(" now.")
	n := Count(c)
	if n != (Counts{Segments: 1, Words: 3, Characters: 12, Tags: 2, Placeables: 1}) {
		t.Fatalf("Count: %+v", n)
	}
}

func memory(t *testing.T, units ...tm.Unit) tm.Memory {
	t.Helper()
	m, err := tm.NewInMemory(tm.Info{Name: "Client TM", SourceLang: "de-DE", TargetLang: "en-GB"})
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range units {
		if _, err := m.Add(context.Background(), u, tm.KeepBoth); err != nil {
			t.Fatal(err)
		}
	}
	return m
}

func seg(id, text string) Segment { return Segment{ID: id, Source: content.Text(text)} }

func row(rows []Row, b Band) Row {
	for _, r := range rows {
		if r.Band == b {
			return r
		}
	}
	return Row{}
}

func TestAnalyze(t *testing.T) {
	ctx := context.Background()
	m := memory(t,
		tm.Unit{Source: content.Text("Der Mieter zahlt die Miete."), Target: content.Text("The tenant pays the rent."),
			Attributes: ContextAttributes(content.Text("Mietvertrag"), content.Text("Ein anderer Satz."))},
		tm.Unit{Source: content.Text("Die Kaution beträgt 1.000 EUR."), Target: content.Text("The deposit is EUR 1,000.")},
		tm.Unit{Source: content.Text("Der Vertrag endet am 31. Dezember 2026."), Target: content.Text("The contract ends on 31 December 2026.")},
	)
	files := []File{
		{Name: "a.docx", Segments: []Segment{
			seg("1", "Mietvertrag"),
			seg("2", "Der Mieter zahlt die Miete."),    // context match
			seg("3", "Die Kaution beträgt 1.000 EUR."), // 100%
			seg("4", "Der Vertrag endet am 31. Dezember 2027."),
			seg("5", "Mietvertrag"), // repetition
			{ID: "6", Source: content.Text("Nicht übersetzen."), Locked: true},
		}},
		{Name: "b.docx", Segments: []Segment{
			seg("1", "Mietvertrag"), // cross-file repetition
			seg("2", "Etwas völlig Neues."),
			seg("3", "Der Mieter zahlt die Miete."), // the segment before differs: no context match
		}},
	}
	o := Options{SourceLang: "de-DE", TargetLang: "en-GB"}
	r, err := Analyze(ctx, files, []tm.Memory{m}, o, time.Date(2026, 11, 23, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	var bands []string
	for _, s := range r.Segments {
		bands = append(bands, s.File+"/"+s.ID+"="+string(s.Band))
	}
	want := "a.docx/1=new a.docx/2=context a.docx/3=100 a.docx/4=85 a.docx/5=repetition a.docx/6=locked b.docx/1=cross-file b.docx/2=new b.docx/3=cross-file"
	if strings.Join(bands, " ") != want {
		t.Fatalf("bands:\n got %s\nwant %s", strings.Join(bands, " "), want)
	}
	if r.Segments[3].Score < 85 || r.Segments[3].Memory != "Client TM" {
		t.Fatalf("fuzzy: %+v", r.Segments[3])
	}
	if len(r.Rows) != len(Bands) || row(r.Rows, Exact).Segments != 1 || row(r.Rows, Exact).Words != 5 || row(r.Rows, CrossFile).Segments != 2 {
		t.Fatalf("rows: %+v", r.Rows)
	}
	if r.Total.Segments != 9 || r.Files[0].Total.Segments != 6 || r.Files[1].Total.Segments != 3 {
		t.Fatalf("totals: %+v", r.Total)
	}
	// memoQ's rule needs the segment after to match as well.
	o.Context = ContextBoth
	r2, _ := Analyze(ctx, files, []tm.Memory{m}, o, time.Now())
	if r2.Segments[1].Band != Exact {
		t.Fatalf("both: %+v", r2.Segments[1])
	}
	both := []File{{Name: "c.docx", Segments: []Segment{seg("1", "Mietvertrag"), seg("2", "Der Mieter zahlt die Miete."), seg("3", "Ein  anderer Satz.")}}}
	if r5, _ := Analyze(ctx, both, []tm.Memory{m}, o, time.Now()); r5.Segments[1].Band != ContextMatch {
		t.Fatalf("both, matching: %+v", r5.Segments[1])
	}
	// Without cross-file counting, b.docx/1 is new (no memory match).
	o = Options{SourceLang: "de-DE", TargetLang: "en-GB", NoCrossFile: true}
	r3, _ := Analyze(ctx, files, []tm.Memory{m}, o, time.Now())
	if r3.Segments[6].Band != New {
		t.Fatalf("no cross-file: %+v", r3.Segments[6])
	}
	// A word count without memories.
	r4, _ := Analyze(ctx, files, nil, Options{}, time.Now())
	if row(r4.Rows, New).Segments != 5 || row(r4.Rows, Repetition).Segments != 1 || row(r4.Rows, CrossFile).Segments != 2 {
		t.Fatalf("word count: %+v", r4.Rows)
	}
}

func TestWeightsAndReports(t *testing.T) {
	files := []File{{Name: "a.docx", Segments: []Segment{seg("1", "Eins zwei drei vier."), seg("2", "Eins zwei drei vier.")}}}
	w, err := ParseWeights("repetition=25, new=100%")
	if err != nil {
		t.Fatal(err)
	}
	r, err := Analyze(context.Background(), files, nil, Options{SourceLang: "de", TargetLang: "en", Weights: w}, time.Date(2026, 11, 23, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if r.Weighted != 5 {
		t.Fatalf("weighted: %v", r.Weighted)
	}
	data, err := r.CSV()
	if err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(bytes.NewReader(data[3:])).ReadAll()
	if err != nil || len(rows) != 1+len(Bands)+1 || rows[0][8] != "Weighted words" || rows[len(rows)-1][8] != "5.0" {
		t.Fatalf("csv: %v %v", rows, err)
	}
	h := string(r.HTML())
	if !strings.Contains(h, "no translation memory (word count only)") || !strings.Contains(h, "<td>25.0%</td><td>1.0</td>") {
		t.Fatalf("html: %s", h)
	}
	if strings.Contains(h, "—") {
		t.Fatal("em dash")
	}
	if _, err := r.JSON(); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"foo=10", "new", "new=abc", "new=-5"} {
		if _, err := ParseWeights(bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	if _, err := Analyze(context.Background(), files, nil, Options{Context: "next"}, time.Now()); err == nil {
		t.Fatal("bad context rule accepted")
	}
}
