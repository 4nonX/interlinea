package term

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

const tbx3 = `<?xml version="1.0" encoding="UTF-8"?>
<tbx type="TBX-Basic" style="dca" xml:lang="en" xmlns="urn:iso:std:iso:30042:ed-2">
<tbxHeader><fileDesc><sourceDesc><p>test</p></sourceDesc></fileDesc></tbxHeader>
<text><body>
<conceptEntry id="c1">
  <descrip type="subjectField">Tenancy law</descrip>
  <transacGrp><transac type="transactionType">origination</transac><date>2025-03-04</date><transacNote type="responsibility">Dan</transacNote></transacGrp>
  <langSec xml:lang="de">
    <descrip type="definition">Gegenstand des Mietvertrags</descrip>
    <termSec><term>Mietsache</term><termNote type="partOfSpeech">noun</termNote><termNote type="grammaticalGender">feminine</termNote><termNote type="administrativeStatus">preferredTerm-admn-sts</termNote></termSec>
    <termSec><term>Mietobjekt</term><termNote type="administrativeStatus">admittedTerm-admn-sts</termNote><note>[Region] AT</note></termSec>
  </langSec>
  <langSec xml:lang="en-GB">
    <termSec><term>leased property</term><descrip type="context">The leased property comprises three rooms.</descrip><termNote type="administrativeStatus">preferredTerm-admn-sts</termNote></termSec>
    <termSec><term>rented object</term><termNote type="administrativeStatus">deprecatedTerm-admn-sts</termNote></termSec>
  </langSec>
</conceptEntry>
<conceptEntry id="c2"><langSec xml:lang="de"><termSec><term> </term></termSec></langSec></conceptEntry>
</body></text></tbx>`

const tbx2 = `<?xml version="1.0" encoding="UTF-8"?>
<martif type="TBX" xml:lang="en"><martifHeader><fileDesc><sourceDesc><p>x</p></sourceDesc></fileDesc></martifHeader><text><body>
<termEntry id="t1"><descripGrp><descrip type="definition">A deposit</descrip></descripGrp>
<langSet xml:lang="de"><tig><term>Kaution</term><termNote type="administrativeStatus">preferredTerm-admn-sts</termNote></tig></langSet>
<langSet xml:lang="en"><ntig><termGrp><term>deposit</term><termNote type="administrativeStatus">admittedTerm-admn-sts</termNote></termGrp><admin type="source">BGB</admin></ntig></langSet>
</termEntry></body></text></martif>`

func TestReadTBX3(t *testing.T) {
	tb, rep, err := ReadTBX([]byte(tbx3))
	if err != nil {
		t.Fatal(err)
	}
	if rep.Entries != 1 || rep.Skipped != 1 || rep.Terms != 4 {
		t.Fatalf("report = %+v", rep)
	}
	e := tb.Entries[0]
	if e.ID != "c1" || e.CreatedBy != "Dan" || e.CreatedAt.Year() != 2025 {
		t.Errorf("entry = %+v", e)
	}
	if e.Fields[0] != (Value{"Domain", "Tenancy law"}) {
		t.Errorf("entry fields = %+v", e.Fields)
	}
	de := e.Lang("de")
	if de.Fields[0] != (Value{"Definition", "Gegenstand des Mietvertrags"}) {
		t.Errorf("language fields = %+v", de.Fields)
	}
	if de.Terms[0].Status != StatusApproved || de.Terms[1].Status != StatusProposed {
		t.Errorf("statuses = %+v", de.Terms)
	}
	if de.Terms[0].Fields[1] != (Value{"Gender", "feminine"}) || de.Terms[1].Fields[0] != (Value{"Region", "AT"}) {
		t.Errorf("term fields = %+v / %+v", de.Terms[0].Fields, de.Terms[1].Fields)
	}
	if en := e.Lang("en-GB"); en.Terms[1].Status != StatusForbidden || en.Terms[0].Fields[0].Field != "Context" {
		t.Errorf("en = %+v", en)
	}
	if strings.Join(tb.Languages, ",") != "de,en-GB" {
		t.Errorf("languages = %v", tb.Languages)
	}
}

func TestReadTBX2(t *testing.T) {
	tb, rep, err := ReadTBX([]byte(tbx2))
	if err != nil || rep.Entries != 1 {
		t.Fatalf("%v %+v", err, rep)
	}
	e := tb.Entries[0]
	if e.Fields[0] != (Value{"Definition", "A deposit"}) || e.Lang("en").Terms[0].Text != "deposit" || e.Lang("en").Terms[0].Fields[0] != (Value{"Source", "BGB"}) {
		t.Errorf("entry = %+v", e)
	}
}

func roundTrip(t *testing.T, f Format, tb *Termbase) *Termbase {
	t.Helper()
	out, err := Write(tb, f)
	if err != nil {
		t.Fatal(err)
	}
	g, err := Detect("terms"+f.Extension(), out)
	if err != nil || g != f {
		t.Fatalf("detect %s: %v %v", f, g, err)
	}
	back, rep, err := Read(f, out, nil)
	if err != nil {
		t.Fatalf("%s: %v\n%s", f, err, out)
	}
	if len(rep.Warnings) > 0 {
		t.Errorf("%s warnings: %v", f, rep.Warnings)
	}
	return back
}

func sample(t *testing.T) *Termbase {
	tb, _, err := ReadTBX([]byte(tbx3))
	if err != nil {
		t.Fatal(err)
	}
	tb.Name = "Mietrecht"
	// A custom field with no TBX category.
	tb.Entries[0].Languages[0].Terms[0].Fields = append(tb.Entries[0].Languages[0].Terms[0].Fields, Value{"Grammar", "noun, fem."})
	tb.Entries = append(tb.Entries, Entry{ID: "c3", Languages: []Language{{Lang: "de", Terms: []Term{{Text: "Kaution", Status: StatusApproved}}}, {Lang: "en-GB", Terms: []Term{{Text: "deposit"}, {Text: "security deposit", Status: StatusProposed}}}}})
	return tb
}

func comparable(tb *Termbase, keepDates bool) string {
	var b strings.Builder
	for _, e := range tb.Entries {
		b.WriteString("E:")
		for _, v := range e.Fields {
			b.WriteString(v.Field + "=" + v.Value + ";")
		}
		if keepDates {
			b.WriteString(e.CreatedBy + e.CreatedAt.Format("2006-01-02"))
		}
		for _, l := range e.Languages {
			b.WriteString(" L:" + l.Lang + ":")
			for _, v := range l.Fields {
				b.WriteString(v.Field + "=" + v.Value + ";")
			}
			for _, tm := range l.Terms {
				st := tm.Status
				if st == "" {
					st = StatusProposed
				}
				b.WriteString(" T:" + tm.Text + "/" + st + ":")
				for _, v := range tm.Fields {
					b.WriteString(v.Field + "=" + v.Value + ";")
				}
			}
		}
		b.WriteString("\n")
	}
	return b.String()
}

func TestRoundTrips(t *testing.T) {
	src := sample(t)
	want := comparable(src, false)
	for _, f := range []Format{FormatTBX, FormatTBX2, FormatMultiTerm, FormatCSV, FormatTSV, FormatXLSX} {
		got := comparable(roundTrip(t, f, src), false)
		if got != want {
			t.Errorf("%s round trip:\n got %s\nwant %s", f, got, want)
		}
	}
	// TBX and MultiTerm keep who created an entry and when.
	for _, f := range []Format{FormatTBX, FormatMultiTerm} {
		if got := comparable(roundTrip(t, f, src), true); got != comparable(src, true) {
			t.Errorf("%s lost history:\n%s", f, got)
		}
	}
}

func TestMultiTermUTF16(t *testing.T) {
	out := WriteMultiTerm(sample(t))
	if !bytes.HasPrefix(out, []byte{0xFF, 0xFE, '<', 0}) {
		t.Fatal("MultiTerm XML should be UTF-16 LE with a byte order mark")
	}
	s := string(ToUTF8(out))
	for _, want := range []string{`<language type="German" lang="DE"/>`, `<language type="English (United Kingdom)" lang="EN-GB"/>`, `<descrip type="Status">forbidden</descrip>`, `<descrip type="Grammar">noun, fem.</descrip>`} {
		if !strings.Contains(s, want) {
			t.Errorf("output lacks %s", want)
		}
	}
}

func TestReadXDT(t *testing.T) {
	xdt := `<?xml version="1.0" encoding="UTF-8"?><TermbaseDefinition><Name>Recht</Name><Languages><ItemsList><ItemText>German</ItemText><ItemLocale>DE</ItemLocale><ItemText>English</ItemText><ItemLocale>EN-GB</ItemLocale></ItemsList></Languages></TermbaseDefinition>`
	d, err := ReadXDT([]byte(xdt))
	if err != nil || d.Name != "Recht" || strings.Join(d.Languages, ",") != "de,en-GB" {
		t.Errorf("%+v %v", d, err)
	}
	if f, _ := Detect("x.xdt", []byte(xdt)); f != FormatXDT {
		t.Errorf("detect = %s", f)
	}
}

func TestGuessMapping(t *testing.T) {
	cases := map[string]string{
		"German;English (UK);Definition (de);Status EN;Note":                "term:de|term:en-GB|field:language:de:Definition|status:en|field:entry::Note",
		"ID,de-DE,de-DE Status,en-GB,en-GB Status,en-GB Term Gender,Domain": "id|term:de-DE|status:de-DE|term:en-GB|status:en-GB|field:term:en-GB:Gender|field:entry::Domain",
		"Deutsch\tEnglisch\tStatus\tAnmerkung_en":                           "term:de|term:en|status:en|field:language:en:Anmerkung",
		"Mietsache,leased property":                                         "term:|term:",
	}
	for header, want := range cases {
		rows, _, err := ReadCSV([]byte(header+"\nx,y\n"), 0)
		if err != nil {
			t.Fatal(err)
		}
		m := GuessMapping(rows)
		var got []string
		for _, c := range m.Columns {
			switch c.Role {
			case RoleTerm, RoleStatus:
				got = append(got, string(c.Role)+":"+c.Lang)
			case RoleField:
				got = append(got, "field:"+string(c.Level)+":"+c.Lang+":"+c.Field)
			case RoleID:
				got = append(got, "id")
			}
		}
		if strings.Join(got, "|") != want {
			t.Errorf("%q:\n got %s\nwant %s", header, strings.Join(got, "|"), want)
		}
	}
}

func TestCSVEncodingsAndSynonyms(t *testing.T) {
	// Windows-1252 with a semicolon, as older Excel versions save.
	data := []byte("Deutsch;Englisch;Status Englisch\n\"Mietk\xe4ution; Kaution\";deposit;approved\n;;\n")
	tb, rep, err := Read(FormatCSV, data, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Entries != 1 || rep.Skipped != 0 {
		t.Errorf("report = %+v", rep)
	}
	e := tb.Entries[0]
	if d := e.Lang("de"); len(d.Terms) != 2 || d.Terms[0].Text != "Mietkäution" {
		t.Errorf("de = %+v", d)
	}
	if en := e.Lang("en"); en.Terms[0].Status != StatusApproved {
		t.Errorf("en = %+v", en)
	}
	// UTF-16 tab-separated, as "Unicode text" from Excel.
	u := utf16LE("de\ten\nMiete\trent\n")
	tb, _, err = Read(FormatTSV, u, nil)
	if err != nil || tb.Entries[0].Lang("en").Terms[0].Text != "rent" {
		t.Errorf("utf-16: %v %+v", err, tb)
	}
	// Without a header the user names the languages.
	rows, _, _ := ReadCSV([]byte("Miete,rent\n"), 0)
	m := GuessMapping(rows)
	if m.Header {
		t.Error("a row of terms is not a header")
	}
	if _, _, err := FromRows(rows, m); err == nil {
		t.Error("term columns without a language must be refused")
	}
	m.Columns[0].Lang, m.Columns[1].Lang = "de", "en"
	tb, _, err = FromRows(rows, m)
	if err != nil || tb.Entries[0].Lang("de").Terms[0].Text != "Miete" {
		t.Errorf("%v %+v", err, tb)
	}
}

func TestPlan(t *testing.T) {
	existing := []Entry{
		{ID: "e1", Languages: []Language{{Lang: "de", Terms: []Term{{Text: "Kaution", Status: StatusApproved}}}, {Lang: "en", Terms: []Term{{Text: "deposit"}}}}},
		{ID: "e2", Languages: []Language{{Lang: "de", Terms: []Term{{Text: "Miete"}}}}},
	}
	incoming := []Entry{
		{Languages: []Language{{Lang: "de", Terms: []Term{{Text: "kaution"}}}, {Lang: "en", Terms: []Term{{Text: "security deposit", Status: StatusProposed}}}, {Lang: "fr", Terms: []Term{{Text: "caution"}}}}},
		{Languages: []Language{{Lang: "de", Terms: []Term{{Text: "Untermiete"}}}, {Lang: "en", Terms: []Term{{Text: "subletting"}}}}},
		{Languages: []Language{{Lang: "de", Terms: []Term{{Text: "Untermiete"}}}, {Lang: "fr", Terms: []Term{{Text: "sous-location"}}}}},
		{Languages: []Language{{Lang: "de", Terms: []Term{{Text: "Miete"}}}}},
	}
	acts, rep := Plan(existing, incoming, Policy{Match: MatchTerm, Lang: "de", OnMatch: Merge})
	if rep.Added != 1 || rep.Merged != 2 || rep.Unchanged != 1 {
		t.Fatalf("report = %+v", rep)
	}
	m := acts[0]
	if m.Kind != "merge" || m.Existing != "e1" || len(m.Entry.Languages) != 3 || len(m.Entry.Lang("en").Terms) != 2 || m.Entry.Lang("de").Terms[0].Status != StatusApproved {
		t.Errorf("merge = %+v", m)
	}
	// The second "Untermiete" merges into the entry the file added.
	if acts[1].Kind != "add" || len(acts[1].Entry.Languages) != 3 {
		t.Errorf("file-internal duplicate not merged: %+v", acts[1])
	}
	_, rep = Plan(existing, incoming, Policy{Match: MatchTerm, OnMatch: Skip})
	// The second "Untermiete" of the file meets the first, which the file added.
	if rep.Skipped != 3 || rep.Added != 1 {
		t.Errorf("skip report = %+v", rep)
	}
	acts, rep = Plan(existing, incoming[:1], Policy{Match: MatchTerm, OnMatch: Overwrite})
	if rep.Overwritten != 1 || acts[0].Entry.ID != "e1" || acts[0].Entry.Lang("fr") == nil || acts[0].Entry.Lang("de").Terms[0].Status != "" {
		t.Errorf("overwrite = %+v %+v", rep, acts[0].Entry)
	}
	_, rep = Plan(existing, incoming, Policy{Match: MatchNone})
	if rep.Added != 4 {
		t.Errorf("add all = %+v", rep)
	}
	_, rep = Plan(existing, []Entry{{ID: "e2", Languages: []Language{{Lang: "en", Terms: []Term{{Text: "rent"}}}}}}, Policy{Match: MatchID, OnMatch: Merge})
	if rep.Merged != 1 {
		t.Errorf("id match = %+v", rep)
	}
	// An incoming entry matching two entries is combined with the first and reported.
	_, rep = Plan(existing, []Entry{{Languages: []Language{{Lang: "de", Terms: []Term{{Text: "Miete"}, {Text: "Kaution"}}}}}}, Policy{Match: MatchTerm, OnMatch: Merge})
	if len(rep.Warnings) != 1 {
		t.Errorf("ambiguous match not reported: %+v", rep)
	}
}

func TestDuplicates(t *testing.T) {
	es := []Entry{
		{Languages: []Language{{Lang: "de", Terms: []Term{{Text: "Kaution"}}}}},
		{Languages: []Language{{Lang: "de", Terms: []Term{{Text: "Miete"}}}}},
		{Languages: []Language{{Lang: "de", Terms: []Term{{Text: "KAUTION"}, {Text: "Mietsicherheit"}}}}},
		{Languages: []Language{{Lang: "de", Terms: []Term{{Text: "Mietsicherheit"}}}}},
	}
	g := Duplicates(es, "de")
	if len(g) != 1 || len(g[0]) != 3 {
		t.Errorf("groups = %v", g)
	}
}

func TestJSON(t *testing.T) {
	tb := sample(t)
	b, err := json.Marshal(tb)
	if err != nil {
		t.Fatal(err)
	}
	var back Termbase
	if err := json.Unmarshal(b, &back); err != nil || comparable(&back, true) != comparable(tb, true) {
		t.Errorf("json round trip: %v", err)
	}
}

func TestNotATermbase(t *testing.T) {
	if _, err := Detect("x.xml", []byte(`<?xml version="1.0"?><html/>`)); err == nil {
		t.Error("HTML is not a termbase")
	}
	if _, _, err := ReadTBX([]byte(`<mtf/>`)); err == nil {
		t.Error("MultiTerm XML is not TBX")
	}
	if _, err := Detect("a.zip", []byte("PK\x03\x04rest")); err == nil {
		t.Error("a ZIP that is not a workbook")
	}
}

func TestGuessSeparator(t *testing.T) {
	rows, _, err := ReadCSV([]byte("German,English\nMieter,tenant | renter\n"), 0)
	if err != nil {
		t.Fatal(err)
	}
	m := GuessMapping(rows)
	if m.Separator != "|" {
		t.Fatalf("separator %q", m.Separator)
	}
	tb, _, err := FromRows(rows, m)
	if err != nil || len(tb.Entries) != 1 || len(tb.Entries[0].Lang("en").Terms) != 2 || tb.Entries[0].Lang("en").Terms[1].Text != "renter" {
		t.Fatalf("synonyms: %v %+v", err, tb)
	}
}

func TestTBXUniqueIDs(t *testing.T) {
	tb := &Termbase{Definition: Definition{Languages: []string{"de"}}, Entries: []Entry{
		{ID: "1", Languages: []Language{{Lang: "de", Terms: []Term{{Text: "a"}}}}},
		{ID: "c1", Languages: []Language{{Lang: "de", Terms: []Term{{Text: "b"}}}}},
		{ID: "c1", Languages: []Language{{Lang: "de", Terms: []Term{{Text: "c"}}}}},
		{Languages: []Language{{Lang: "de", Terms: []Term{{Text: "d"}}}}},
	}}
	data, err := Write(tb, FormatTBX)
	if err != nil {
		t.Fatal(err)
	}
	back, _, err := Read(FormatTBX, data, nil)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, e := range back.Entries {
		ids = append(ids, e.ID)
	}
	if strings.Join(ids, " ") != "c2 c1 c3 c4" {
		t.Fatalf("ids: %v", ids)
	}
}

// The status words of real termbases: Trados's sample termbase marks its
// forbidden terms "not allowed".
func TestNormalizeStatusWords(t *testing.T) {
	for in, want := range map[string]string{
		"not allowed": StatusForbidden, "Not Allowed": StatusForbidden, "nicht erlaubt": StatusForbidden, "à éviter": StatusForbidden,
		"Forbidden": StatusForbidden, "Preferred": StatusApproved, "allowed": StatusApproved, "Approved": StatusApproved,
		"admitted": StatusProposed, "": "",
	} {
		if got := NormalizeStatus(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}
