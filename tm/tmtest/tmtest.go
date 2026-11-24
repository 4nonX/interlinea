// Package tmtest is a conformance suite for tm.Memory implementations.
// Every backend runs it, so all backends store, match and search alike.
package tmtest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/tm"
)

// Factory returns a new, empty memory for info.
type Factory func(t *testing.T, info tm.Info) tm.Memory

// DefaultInfo is the memory description used by the suite.
var DefaultInfo = tm.Info{Name: "Recht", SourceLang: "de-DE", TargetLang: "en-GB"}

// Run executes the whole suite against the backend built by newMemory.
func Run(t *testing.T, newMemory Factory) {
	tests := []struct {
		name string
		fn   func(*testing.T, Factory)
	}{
		{"AddPolicies", testAddPolicies},
		{"Validation", testValidation},
		{"RoundTripFields", testRoundTripFields},
		{"Lookup", testLookup},
		{"LookupTags", testLookupTags},
		{"LookupPrefersRecent", testLookupPrefersRecent},
		{"LookupShortAndCJK", testLookupShortAndCJK},
		{"LookupWordForms", testLookupWordForms},
		{"Concordance", testConcordance},
		{"TMX", testTMX},
		{"Each", testEach},
		{"ContextCancel", testContextCancel},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) { tc.fn(t, newMemory) })
	}
}

var ctx = context.Background()

func add(t *testing.T, m tm.Memory, src, tgt string) tm.Unit {
	t.Helper()
	u, err := m.Add(ctx, tm.Unit{Source: content.Text(src), Target: content.Text(tgt), CreatedBy: "dan"}, tm.KeepBoth)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func lookup(t *testing.T, m tm.Memory, src string, opts tm.LookupOptions) []tm.Match {
	t.Helper()
	ms, err := m.Lookup(ctx, content.Text(src), opts)
	if err != nil {
		t.Fatal(err)
	}
	return ms
}

func scores(ms []tm.Match) []int {
	out := make([]int, len(ms))
	for i, m := range ms {
		out[i] = m.Score
	}
	return out
}

func testAddPolicies(t *testing.T, newMemory Factory) {
	m := newMemory(t, DefaultInfo)
	a := add(t, m, "Der Mieter haftet.", "The tenant is liable.")
	if a.ID == 0 || a.CreatedBy != "dan" || a.ChangedBy != "dan" || a.CreatedAt.IsZero() || a.ChangedAt.IsZero() {
		t.Fatalf("stored unit = %+v", a)
	}
	b := add(t, m, "Der  Mieter haftet. ", "The tenant is liable.")
	if b.ID != a.ID {
		t.Fatalf("identical pair duplicated: %d vs %d", b.ID, a.ID)
	}
	c := add(t, m, "Der Mieter haftet.", "The lessee is liable.")
	if c.ID == a.ID {
		t.Fatal("alternative translation overwrote the first")
	}
	d, err := m.Add(ctx, tm.Unit{Source: content.Text("Der Mieter haftet."), Target: content.Text("The tenant shall be liable."), ChangedBy: "eva"}, tm.Overwrite)
	if err != nil {
		t.Fatal(err)
	}
	if d.ID != c.ID || d.Target.Plain() != "The tenant shall be liable." || d.ChangedBy != "eva" || d.CreatedBy != "dan" {
		t.Fatalf("overwrite = %+v", d)
	}
	if n, err := m.Count(ctx); err != nil || n != 2 {
		t.Fatalf("count = %d, %v", n, err)
	}
	ms := lookup(t, m, "Der Mieter haftet.", tm.LookupOptions{})
	got := map[string]bool{}
	for _, x := range ms {
		got[x.Unit.Target.Plain()] = true
	}
	if !got["The tenant is liable."] || !got["The tenant shall be liable."] || len(ms) != 2 {
		t.Fatalf("stored targets = %v", got)
	}
}

func testValidation(t *testing.T, newMemory Factory) {
	m := newMemory(t, DefaultInfo)
	if _, err := m.Add(ctx, tm.Unit{Source: content.Text(" "), Target: content.Text("x")}, tm.KeepBoth); !errors.Is(err, tm.ErrEmptySource) {
		t.Errorf("empty source: %v", err)
	}
	if _, err := m.Add(ctx, tm.Unit{Source: content.Text("x"), Target: content.Text("")}, tm.KeepBoth); !errors.Is(err, tm.ErrEmptyTarget) {
		t.Errorf("empty target: %v", err)
	}
	bad := content.Content{{Tag: &content.Tag{Kind: content.TagOpen}}, {Text: "x"}}
	if _, err := m.Add(ctx, tm.Unit{Source: bad, Target: content.Text("y")}, tm.KeepBoth); err == nil {
		t.Error("invalid tags accepted")
	}
	if info := m.Info(); info.SourceLang != "de-DE" || info.TargetLang != "en-GB" || info.Name == "" {
		t.Errorf("info = %+v", info)
	}
}

func testRoundTripFields(t *testing.T, newMemory Factory) {
	m := newMemory(t, DefaultInfo)
	created := time.Date(2025, 5, 6, 7, 8, 9, 123000000, time.UTC)
	var src content.Content
	src = src.Append("Siehe ")
	src = src.AppendTag(content.Tag{Kind: content.TagOpen, ID: "1", Type: "bold", Native: "<b>"})
	src = src.Append("Anlage")
	src = src.AppendTag(content.Tag{Kind: content.TagClose, ID: "1", Type: "bold", Native: "</b>"})
	src = src.AppendTag(content.Tag{Kind: content.TagStandalone, ID: "2", Type: "lb", Native: "<br/>"})
	in := tm.Unit{
		Source: src, Target: content.Text("See annex"),
		CreatedAt: created, CreatedBy: "anna", UsageCount: 4,
		Attributes: []tm.Attribute{{Name: "x-client", Value: "Kanzlei Weber"}},
		Notes:      []string{"geprüft"},
	}
	if _, err := m.Add(ctx, in, tm.KeepBoth); err != nil {
		t.Fatal(err)
	}
	ms := lookup(t, m, "Siehe Anlage", tm.LookupOptions{MinScore: 50})
	if len(ms) != 1 {
		t.Fatalf("matches = %v", scores(ms))
	}
	u := ms[0].Unit
	if !u.Source.Equal(src) || u.Source[1].Tag.Native != "<b>" || u.Source[4].Tag.Native != "<br/>" {
		t.Errorf("source = %v", u.Source)
	}
	if !u.CreatedAt.Equal(created) || !u.ChangedAt.Equal(created) || u.CreatedBy != "anna" || u.ChangedBy != "anna" {
		t.Errorf("dates/users = %+v", u)
	}
	if u.UsageCount != 4 || len(u.Attributes) != 1 || u.Attributes[0].Value != "Kanzlei Weber" || len(u.Notes) != 1 || u.Notes[0] != "geprüft" {
		t.Errorf("metadata = %+v", u)
	}
}

func testLookup(t *testing.T, newMemory Factory) {
	m := newMemory(t, DefaultInfo)
	add(t, m, "Der Mieter haftet für alle Schäden an der Mietsache.", "The tenant is liable for all damage to the leased property.")
	add(t, m, "Der Vermieter haftet für alle Schäden an der Mietsache.", "The landlord is liable for all damage to the leased property.")
	add(t, m, "Die Kündigungsfrist beträgt drei Monate.", "The notice period is three months.")
	for i := 0; i < 300; i++ {
		add(t, m, fmt.Sprintf("Füllsatz Nummer %d ohne Bezug.", i), fmt.Sprintf("Filler sentence number %d.", i))
	}
	ms := lookup(t, m, "Der Mieter haftet für alle Schäden an der Mietsache.", tm.LookupOptions{})
	if len(ms) != 2 || ms[0].Score != 100 || ms[1].Score >= 100 || ms[1].Score < 85 {
		t.Fatalf("matches = %v", scores(ms))
	}
	if !strings.Contains(ms[0].Unit.Target.Plain(), "tenant") {
		t.Fatalf("best = %q", ms[0].Unit.Target.Plain())
	}
	ms = lookup(t, m, "Die Kündigungsfrist beträgt sechs Monate.", tm.LookupOptions{MinScore: 50, Limit: 1})
	if len(ms) != 1 || ms[0].Score >= 100 || ms[0].Score < 70 {
		t.Fatalf("fuzzy = %v", scores(ms))
	}
	if ms := lookup(t, m, "Völlig anderer Inhalt hier.", tm.LookupOptions{}); len(ms) != 0 {
		t.Fatalf("unrelated matched: %v", scores(ms))
	}
	ms = lookup(t, m, "Füllsatz Nummer 17 ohne Bezug.", tm.LookupOptions{Limit: 3})
	if len(ms) != 3 || ms[0].Score != 100 || ms[0].Unit.Target.Plain() != "Filler sentence number 17." {
		t.Fatalf("filler = %v", scores(ms))
	}
	// Results are copies.
	ms = lookup(t, m, "Die Kündigungsfrist beträgt drei Monate.", tm.LookupOptions{})
	ms[0].Unit.Target[0].Text = "changed"
	again := lookup(t, m, "Die Kündigungsfrist beträgt drei Monate.", tm.LookupOptions{})
	if again[0].Unit.Target.Plain() != "The notice period is three months." {
		t.Fatal("lookup result aliases stored data")
	}
}

func testLookupTags(t *testing.T, newMemory Factory) {
	m := newMemory(t, DefaultInfo)
	var src content.Content
	src = src.Append("Siehe ")
	src = src.AppendTag(content.Tag{Kind: content.TagOpen, ID: "a", Type: "bold"})
	src = src.Append("Anlage 1")
	src = src.AppendTag(content.Tag{Kind: content.TagClose, ID: "a", Type: "bold"})
	if _, err := m.Add(ctx, tm.Unit{Source: src, Target: content.Text("See annex 1")}, tm.KeepBoth); err != nil {
		t.Fatal(err)
	}
	other := src.Clone()
	other[1].Tag.ID, other[3].Tag.ID = "7", "7"
	ms, err := m.Lookup(ctx, other, tm.LookupOptions{})
	if err != nil || len(ms) != 1 || ms[0].Score != 100 {
		t.Fatalf("same tag shape, other ids: %v %v", scores(ms), err)
	}
	ms = lookup(t, m, "Siehe Anlage 1", tm.LookupOptions{})
	if len(ms) != 1 || ms[0].Score >= 100 {
		t.Fatalf("missing tags must cost: %v", scores(ms))
	}
}

func testLookupPrefersRecent(t *testing.T, newMemory Factory) {
	m := newMemory(t, DefaultInfo)
	old := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := m.Add(ctx, tm.Unit{Source: content.Text("Siehe oben."), Target: content.Text("See above."), ChangedAt: old}, tm.KeepBoth); err != nil {
		t.Fatal(err)
	}
	add(t, m, "Siehe oben.", "Refer to the above.")
	ms := lookup(t, m, "Siehe oben.", tm.LookupOptions{})
	if len(ms) != 2 || ms[0].Unit.Target.Plain() != "Refer to the above." {
		t.Fatalf("order = %+v", ms)
	}
}

func testLookupShortAndCJK(t *testing.T, newMemory Factory) {
	m := newMemory(t, tm.Info{Name: "ja", SourceLang: "ja-JP", TargetLang: "de-DE"})
	add(t, m, "契約は来月に始まります。", "Der Vertrag beginnt nächsten Monat.")
	add(t, m, "\u2014", "\u2014")
	add(t, m, "OK", "OK")
	ms := lookup(t, m, "契約は来年に始まります。", tm.LookupOptions{MinScore: 60})
	if len(ms) != 1 || ms[0].Score >= 100 || ms[0].Score < 60 {
		t.Fatalf("CJK fuzzy = %v", scores(ms))
	}
	if ms := lookup(t, m, "\u2014", tm.LookupOptions{}); len(ms) != 1 || ms[0].Score != 100 {
		t.Fatalf("punctuation only = %v", scores(ms))
	}
	if ms := lookup(t, m, "OK", tm.LookupOptions{}); len(ms) != 1 || ms[0].Score != 100 {
		t.Fatalf("two letters = %v", scores(ms))
	}
}

// A short segment whose only word is in another form shares no word with
// its match and is found all the same; a change of ending in a sentence
// leaves a close match, in every script.
func testLookupWordForms(t *testing.T, newMemory Factory) {
	m := newMemory(t, DefaultInfo)
	add(t, m, "Kündigungsfristen", "Notice periods")
	add(t, m, "Garten", "Garden")
	add(t, m, "Sonstige Vereinbarungen", "Other agreements")
	add(t, m, "Die Pflichten des Mieters", "The tenant's obligations")
	add(t, m, "임차인은 차임을 지급한다", "The tenant pays the rent")
	add(t, m, "العَقْدُ مُلْزِمٌ للطرفين", "The contract binds both parties")
	add(t, m, "Арендатор платит арендную плату", "The tenant pays the rent")

	ms := lookup(t, m, "Kündigungsfrist", tm.LookupOptions{MinScore: 50})
	if len(ms) != 1 || ms[0].Unit.Target.Plain() != "Notice periods" || ms[0].Score >= 70 {
		t.Fatalf("a heading in the singular: %v", scores(ms))
	}
	if ms := lookup(t, m, "Gartenhaus", tm.LookupOptions{MinScore: 50}); len(ms) != 0 {
		t.Fatalf("another word with the same beginning is no match: %v", scores(ms))
	}
	ms = lookup(t, m, "Sonstigen Vereinbarung", tm.LookupOptions{MinScore: 50})
	if len(ms) != 1 || ms[0].Unit.Target.Plain() != "Other agreements" {
		t.Fatalf("two words, both in another form: %v", scores(ms))
	}
	for _, c := range []struct{ query, target string }{
		{"Die Pflichten des Mieter", "The tenant's obligations"},
		{"임차인이 차임을 지급한다", "The tenant pays the rent"},
		{"العقد ملزم للطرفين", "The contract binds both parties"},
		{"Арендатор платит арендной платы", "The tenant pays the rent"},
	} {
		ms := lookup(t, m, c.query, tm.LookupOptions{})
		if len(ms) == 0 || ms[0].Unit.Target.Plain() != c.target || ms[0].Score >= 100 {
			t.Errorf("%q: %v", c.query, scores(ms))
		}
	}
}

func testConcordance(t *testing.T, newMemory Factory) {
	m := newMemory(t, DefaultInfo)
	add(t, m, "Die Mietsache ist zurückzugeben.", "The leased property must be returned.")
	add(t, m, "Schäden an der MIETSACHE.", "Damage to the leased property.")
	add(t, m, "Die Kaution beträgt 500 EUR.", "The deposit is EUR 500.")
	add(t, m, "Größe: XL", "Size: XL")
	got, err := m.Concordance(ctx, "mietsache", tm.ConcordanceOptions{})
	if err != nil || len(got) != 2 || got[0].Source.Plain() != "Schäden an der MIETSACHE." {
		t.Fatalf("concordance = %v, %v", got, err)
	}
	got, _ = m.Concordance(ctx, "leased  property", tm.ConcordanceOptions{InTarget: true, Limit: 1})
	if len(got) != 1 {
		t.Fatalf("target concordance = %v", got)
	}
	got, _ = m.Concordance(ctx, "xl", tm.ConcordanceOptions{})
	if len(got) != 1 || got[0].Target.Plain() != "Size: XL" {
		t.Fatalf("short query = %v", got)
	}
	got, _ = m.Concordance(ctx, "GRÖSSE", tm.ConcordanceOptions{})
	if len(got) != 0 {
		t.Fatalf("ß and SS must not be folded together: %v", got)
	}
	got, _ = m.Concordance(ctx, "größe", tm.ConcordanceOptions{})
	if len(got) != 1 {
		t.Fatalf("umlaut case folding = %v", got)
	}
	if got, _ := m.Concordance(ctx, "  ", tm.ConcordanceOptions{}); len(got) != 0 {
		t.Fatal("blank query returned results")
	}
}

const tmxDoc = `<?xml version="1.0" encoding="UTF-8"?>
<tmx version="1.4"><header srclang="de-DE" segtype="sentence" o-tmf="x" adminlang="en" datatype="plaintext" creationtool="t" creationtoolversion="1"/>
<body>
<tu tuid="a1" creationdate="20250101T000000Z" creationid="anna"><prop type="x-client">Kanzlei Weber</prop>
 <tuv xml:lang="de-DE"><seg>Der Vertrag endet am <bpt i="1" type="bold">&lt;b&gt;</bpt>31.12.<ept i="1">&lt;/b&gt;</ept></seg></tuv>
 <tuv xml:lang="en-GB"><seg>The contract ends on <bpt i="1" type="bold">&lt;b&gt;</bpt>31 December<ept i="1">&lt;/b&gt;</ept></seg></tuv></tu>
<tu><tuv xml:lang="de-AT"><seg>Jänner</seg></tuv><tuv xml:lang="en-US"><seg>January</seg></tuv></tu>
<tu><tuv xml:lang="de-DE"><seg>Nur Quelle</seg></tuv></tu>
<tu><tuv xml:lang="de-DE"><seg>Leer</seg></tuv><tuv xml:lang="en-GB"><seg></seg></tuv></tu>
</body></tmx>`

func testTMX(t *testing.T, newMemory Factory) {
	m := newMemory(t, DefaultInfo)
	st, err := tm.ImportTMX(ctx, m, strings.NewReader(tmxDoc), tm.ImportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if st.Read != 4 || st.Stored != 1 || st.Skipped != 3 {
		t.Fatalf("strict import = %+v", st)
	}
	m2 := newMemory(t, tm.Info{Name: "Recht 2", SourceLang: "de-DE", TargetLang: "en-GB"})
	st, err = tm.ImportTMX(ctx, m2, strings.NewReader(tmxDoc), tm.ImportOptions{AllowPrimaryMatch: true})
	if err != nil || st.Stored != 2 {
		t.Fatalf("primary-match import = %+v, %v", st, err)
	}
	var u tm.Unit
	if err := m.Each(ctx, func(x tm.Unit) error { u = x; return nil }); err != nil {
		t.Fatal(err)
	}
	if u.CreatedBy != "anna" || u.CreatedAt.Year() != 2025 || !u.Source.HasTags() {
		t.Fatalf("imported unit = %+v", u)
	}
	if len(u.Attributes) != 2 || u.Attributes[1].Value != "Kanzlei Weber" {
		t.Fatalf("attributes = %+v", u.Attributes)
	}
	var buf bytes.Buffer
	if err := tm.ExportTMX(ctx, m, &buf, "test"); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{`srclang="de-DE"`, `xml:lang="en-GB"`, `<bpt i="1" type="bold">&lt;b&gt;</bpt>`, `<prop type="x-client">Kanzlei Weber</prop>`, `creationid="anna"`} {
		if !strings.Contains(out, want) {
			t.Errorf("export lacks %s:\n%s", want, out)
		}
	}
	m3 := newMemory(t, tm.Info{Name: "Recht 3", SourceLang: "de-DE", TargetLang: "en-GB"})
	if st, err := tm.ImportTMX(ctx, m3, bytes.NewReader(buf.Bytes()), tm.ImportOptions{}); err != nil || st.Stored != 1 {
		t.Fatalf("re-import = %+v, %v", st, err)
	}
	// Importing the same file again must not duplicate units.
	if _, err := tm.ImportTMX(ctx, m3, bytes.NewReader(buf.Bytes()), tm.ImportOptions{}); err != nil {
		t.Fatal(err)
	}
	if n, _ := m3.Count(ctx); n != 1 {
		t.Fatalf("re-import duplicated units: %d", n)
	}
}

func testEach(t *testing.T, newMemory Factory) {
	m := newMemory(t, DefaultInfo)
	for i := 0; i < 1203; i++ {
		add(t, m, fmt.Sprintf("Satz %d.", i), fmt.Sprintf("Sentence %d.", i))
	}
	n := 0
	var last int64
	err := m.Each(ctx, func(u tm.Unit) error {
		if u.ID <= last {
			return fmt.Errorf("not in insertion order: %d after %d", u.ID, last)
		}
		last = u.ID
		n++
		return nil
	})
	if err != nil || n != 1203 {
		t.Fatalf("each visited %d, %v", n, err)
	}
	stop := errors.New("stop")
	n = 0
	err = m.Each(ctx, func(tm.Unit) error {
		n++
		if n == 10 {
			return stop
		}
		return nil
	})
	if !errors.Is(err, stop) || n != 10 {
		t.Fatalf("each did not stop: %d %v", n, err)
	}
}

func testContextCancel(t *testing.T, newMemory Factory) {
	m := newMemory(t, DefaultInfo)
	add(t, m, "a b c", "x y z")
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := m.Lookup(cctx, content.Text("a b c"), tm.LookupOptions{}); !errors.Is(err, context.Canceled) {
		t.Errorf("lookup ignored cancel: %v", err)
	}
	if _, err := m.Add(cctx, tm.Unit{Source: content.Text("a"), Target: content.Text("b")}, tm.KeepBoth); !errors.Is(err, context.Canceled) {
		t.Errorf("add ignored cancel: %v", err)
	}
	if err := m.Each(cctx, func(tm.Unit) error { return nil }); !errors.Is(err, context.Canceled) {
		t.Errorf("each ignored cancel: %v", err)
	}
}
