package tm

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/4nonX/interlinea/content"
	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
)

// The tables of a Trados Studio memory, as Trados creates them (the
// columns the reader uses, with Trados's own comment in attributes).
var sdltmSchema = []string{
	"CREATE TABLE translation_memories(\r\n\tid INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,\r\n\tguid BLOB NOT NULL,\r\n\tname TEXT NOT NULL UNIQUE,\r\n\tsource_language TEXT NOT NULL,\r\n\ttarget_language TEXT NOT NULL,\r\n\tcopyright TEXT,\r\n\tdescription TEXT\r\n)",
	"CREATE TABLE translation_units(\r\n\tid INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,\r\n\tguid BLOB NOT NULL,\r\n\ttranslation_memory_id INT NOT NULL CONSTRAINT FK_tu_tm REFERENCES translation_memories(id) ON DELETE CASCADE,\r\n\tsource_hash INTEGER NOT NULL,\r\n\tsource_segment TEXT,\r\n\ttarget_hash INTEGER NOT NULL,\r\n\ttarget_segment TEXT,\r\n\tcreation_date DATETIME NOT NULL,\r\n\tcreation_user TEXT NOT NULL,\r\n\tchange_date DATETIME NOT NULL,\r\n\tchange_user TEXT NOT NULL,\r\n\tlast_used_date DATETIME NOT NULL,\r\n\tlast_used_user TEXT NOT NULL,\r\n\tusage_counter INT NOT NULL,\r\n\tflags INT\r\n)",
	"CREATE TABLE attributes(\r\n\tid INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,\r\n\tguid BLOB NOT NULL,\r\n\tname TEXT NOT NULL,\r\n\ttype INT NOT NULL,\r\n\ttm_id INT NOT NULL, -- CONSTRAINT FK_a_t REFERENCES translation_memories(id) ON DELETE CASCADE,\r\nCONSTRAINT CK_a UNIQUE (\r\n\tname, tm_id\r\n)\r\n)",
	"CREATE TABLE string_attributes(translation_unit_id INT NOT NULL, attribute_id INT NOT NULL, value TEXT NOT NULL)",
	"CREATE TABLE numeric_attributes(translation_unit_id INT NOT NULL, attribute_id INT NOT NULL, value INT NOT NULL)",
	"CREATE TABLE date_attributes(translation_unit_id INT NOT NULL, attribute_id INT NOT NULL, value DATETIME NOT NULL)",
	"CREATE TABLE picklist_values(id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL, guid BLOB NOT NULL, attribute_id INT NOT NULL, value TEXT NOT NULL)",
	"CREATE TABLE picklist_attributes(translation_unit_id INT NOT NULL, picklist_value_id INT NOT NULL)",
}

func seg(lang, elements string) string {
	return `<Segment xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns:xsd="http://www.w3.org/2001/XMLSchema"><Elements>` + elements + `</Elements><CultureName>` + lang + `</CultureName></Segment>`
}

func tag(kind string, anchor, align int, id string) string {
	return "<Tag><Type>" + kind + "</Type><Anchor>" + itoa(anchor) + "</Anchor><AlignmentAnchor>" + itoa(align) + "</AlignmentAnchor><TagID>" + id + "</TagID></Tag>"
}

func itoa(n int) string { return strconv.Itoa(n) }

func text(s string) string { return "<Text><Value>" + s + "</Value></Text>" }

func sdltmFile(t *testing.T, extra ...string) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "m.sdltm")
	db, err := sql.Open("sqlite3", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatal(q, err)
		}
	}
	for _, s := range sdltmSchema {
		exec(s)
	}
	exec(`INSERT INTO translation_memories VALUES (1, x'01', 'Kunde Weber', 'de-DE', 'en-GB', NULL, 'Mietverträge')`)
	tu := func(src, tgt string) {
		exec(`INSERT INTO translation_units (guid, translation_memory_id, source_hash, source_segment, target_hash, target_segment, creation_date, creation_user, change_date, change_user, last_used_date, last_used_user, usage_counter, flags)
			VALUES (x'02', 1, 0, ?, 0, ?, '2024-03-01 10:00:00', 'EVA\eva', '2024-03-02 11:30:00', 'DAN\dan', '9999-12-31 23:59:59', 'x', 3, 1)`, src, tgt)
	}
	// Tags paired by Anchor, matched between the sides by AlignmentAnchor.
	tu(seg("de-DE", text("Der ")+tag("Start", 1, 1, "7")+text("Mieter")+tag("End", 1, 0, "7")+text(" zahlt.")+tag("Standalone", 2, 2, "8")),
		seg("en-GB", text("The ")+tag("Start", 1, 1, "7")+text("tenant")+tag("End", 1, 0, "7")+text(" pays.")+tag("Standalone", 2, 2, "8")))
	// A placeholder with its text, a target tag with no source tag, an end
	// without a start and a start without an end.
	tu(seg("de-DE", text("Seite ")+`<Tag><Type>TextPlaceholder</Type><Anchor>1</Anchor><AlignmentAnchor>1</AlignmentAnchor><TagID>9</TagID><TextEquivalent>3</TextEquivalent></Tag>`+tag("End", 5, 0, "x")+tag("Start", 6, 0, "y")+text("offen")),
		seg("en-GB", text("Page ")+`<Tag><Type>TextPlaceholder</Type><Anchor>1</Anchor><AlignmentAnchor>1</AlignmentAnchor><TagID>9</TagID><TextEquivalent>3</TextEquivalent></Tag>`+tag("Standalone", 2, 0, "z")))
	tu(seg("de-DE", "<Text><Value>kaputt"), seg("en-GB", text("broken")))
	exec(`INSERT INTO attributes VALUES (1, x'03', 'StructureContext', 2, 1), (2, x'04', 'Client', 1, 1), (3, x'05', 'Words', 6, 1), (4, x'06', 'Due', 3, 1), (5, x'07', 'Domain', 5, 1)`)
	exec(`INSERT INTO string_attributes VALUES (1, 1, 'x-tm-heading'), (1, 2, 'Weber GmbH')`)
	exec(`INSERT INTO numeric_attributes VALUES (1, 3, 4)`)
	exec(`INSERT INTO date_attributes VALUES (2, 4, '2024-05-01 00:00:00')`)
	exec(`INSERT INTO picklist_values VALUES (1, x'08', 5, 'Mietrecht'), (2, x'09', 5, 'Kaufrecht')`)
	exec(`INSERT INTO picklist_attributes VALUES (1, 1), (1, 2)`)
	for _, q := range extra {
		exec(q)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// shows renders content with tags as {id}, {/id} and {id/}.
func shows(c content.Content) string {
	var b strings.Builder
	for _, r := range c {
		switch {
		case !r.IsTag():
			b.WriteString(r.Text)
		case r.Tag.Kind == content.TagOpen:
			b.WriteString("{" + r.Tag.ID + "}")
		case r.Tag.Kind == content.TagClose:
			b.WriteString("{/" + r.Tag.ID + "}")
		default:
			b.WriteString("{" + r.Tag.ID + "/}")
		}
	}
	return b.String()
}

func TestReadSDLTM(t *testing.T) {
	data := sdltmFile(t)
	if FormatOf(data) != FormatSDLTM {
		t.Fatal(FormatOf(data))
	}
	f, err := ReadFile(data)
	if err != nil {
		t.Fatal(err)
	}
	if f.Name != "Kunde Weber" || f.SourceLang != "de-DE" || f.TargetLang != "en-GB" || f.Description != "Mietverträge" || len(f.Units) != 2 || f.Skipped != 1 || len(f.Warnings) != 1 {
		t.Fatalf("file: %+v", f)
	}
	u := f.Units[0]
	if shows(u.Source) != "Der {1}Mieter{/1} zahlt.{2/}" || shows(u.Target) != "The {1}tenant{/1} pays.{2/}" {
		t.Fatalf("tags: %q %q", shows(u.Source), shows(u.Target))
	}
	if u.Source[1].Tag.Native != "tag 7" {
		t.Fatalf("native: %+v", u.Source[1].Tag)
	}
	if u.CreatedBy != `EVA\eva` || u.ChangedBy != `DAN\dan` || u.UsageCount != 3 ||
		!u.CreatedAt.Equal(time.Date(2024, 3, 1, 10, 0, 0, 0, time.UTC)) || !u.ChangedAt.Equal(time.Date(2024, 3, 2, 11, 30, 0, 0, time.UTC)) {
		t.Fatalf("unit: %+v", u.Unit)
	}
	want := []Attribute{{"Client", "Weber GmbH"}, {"Domain", "Mietrecht"}, {"Domain", "Kaufrecht"}, {"Words", "4"}}
	if !reflect.DeepEqual(u.Attributes, want) {
		t.Fatalf("fields: %+v", u.Attributes)
	}
	u = f.Units[1]
	if shows(u.Source) != "Seite {1/}{s5/}{s6}offen{/s6}" || shows(u.Target) != "Page {1/}{t2/}" || u.Source[1].Tag.Desc != "3" {
		t.Fatalf("odd tags: %q %q", shows(u.Source), shows(u.Target))
	}
	if !reflect.DeepEqual(u.Attributes, []Attribute{{"Due", "2024-05-01T00:00:00Z"}}) {
		t.Fatalf("date field: %+v", u.Attributes)
	}
}

func TestImportFile(t *testing.T) {
	ctx := context.Background()
	data := sdltmFile(t)
	// Into a memory of the same languages.
	m, _ := NewInMemory(Info{Name: "a", SourceLang: "de-DE", TargetLang: "en-GB"})
	st, err := Import(ctx, m, data, ImportOptions{})
	if err != nil || st.Read != 3 || st.Stored != 2 || st.Skipped != 1 {
		t.Fatalf("import: %+v %v", st, err)
	}
	ms, err := m.Lookup(ctx, content.Text("Seite offen"), LookupOptions{MinScore: 50})
	if err != nil || len(ms) == 0 || !strings.HasPrefix(ms[0].Unit.Target.Plain(), "Page") {
		t.Fatalf("lookup: %+v %v", ms, err)
	}
	// The other way round.
	r, _ := NewInMemory(Info{Name: "b", SourceLang: "en-GB", TargetLang: "de-DE"})
	if st, err := Import(ctx, r, data, ImportOptions{}); err != nil || st.Stored != 2 {
		t.Fatalf("reversed: %+v %v", st, err)
	}
	var first Unit
	r.Each(ctx, func(u Unit) error {
		if first.Source == nil {
			first = u
		}
		return nil
	})
	if shows(first.Source) != "The {1}tenant{/1} pays.{2/}" {
		t.Fatalf("reversed unit: %q", shows(first.Source))
	}
	// A variant of the language only with AllowPrimaryMatch.
	v, _ := NewInMemory(Info{Name: "c", SourceLang: "de-AT", TargetLang: "en-US"})
	if st, _ := Import(ctx, v, data, ImportOptions{}); st.Stored != 0 {
		t.Fatalf("variants without primary match: %+v", st)
	}
	if st, _ := Import(ctx, v, data, ImportOptions{AllowPrimaryMatch: true}); st.Stored != 2 {
		t.Fatalf("variants with primary match: %+v", st)
	}
	// Other languages: nothing, and the note says why.
	o, _ := NewInMemory(Info{Name: "d", SourceLang: "fr-FR", TargetLang: "it-IT"})
	f, _ := ReadFile(data)
	if st, _ := ImportUnits(ctx, o, f, ImportOptions{}); st.Stored != 0 || st.Skipped != 3 {
		t.Fatalf("other languages: %+v", st)
	}
	if n := LanguagesNote(f, o.Info()); !strings.Contains(n, "de-DE to en-GB") {
		t.Fatalf("note: %q", n)
	}
	if LanguagesNote(f, r.Info()) != "" {
		t.Fatal("note for a reversed memory")
	}
}

func TestReadTMXLanguages(t *testing.T) {
	data := []byte(`<?xml version="1.0"?><tmx version="1.4"><header creationtool="x" creationtoolversion="1" segtype="sentence" o-tmf="x" adminlang="en" srclang="*all*" datatype="plaintext"/><body>` +
		`<tu><tuv xml:lang="de-DE"><seg>Hallo</seg></tuv><tuv xml:lang="fr-FR"><seg>Salut</seg></tuv></tu></body></tmx>`)
	f, err := ReadFile(data)
	if err != nil || f.Format != FormatTMX || f.SourceLang != "de-DE" || f.TargetLang != "fr-FR" {
		t.Fatalf("%+v %v", f, err)
	}
	m, _ := NewInMemory(Info{Name: "t", SourceLang: "de-DE", TargetLang: "fr-FR"})
	if st, err := Import(context.Background(), m, data, ImportOptions{}); err != nil || st.Stored != 1 {
		t.Fatalf("%+v %v", st, err)
	}
}

func utf16le(s string) []byte {
	u := utf16.Encode([]rune(s))
	b := []byte{0xff, 0xfe}
	for _, x := range u {
		b = append(b, byte(x), byte(x>>8))
	}
	return b
}

func TestReadWordfast(t *testing.T) {
	lines := "%20141201~221624\t%User ID,JU,JU Judy\t%TU=00000002\t%DE-01\t%Wordfast TM v.546/00\t%EN-GB\t%-----------\t%Client,Weber\t\t\t\t \r\n" +
		"20141202~064725\tJudy\t2\tDE-01\t&tA;Mieter&'92;s Kaution &'26; Miete\\nZeile 2\tEN-GB\t&tA;tenant&'92;s deposit &'26; rent\\nline 2\tWeber\r\n" +
		"20141203~070000\tJudy\t0\tDE-01\tzu kurz\r\n" +
		"20141204~080000\tAB\t0\tDE-01\tFrei\tEN-GB\tFree\r\n"
	for name, data := range map[string][]byte{
		"utf-16le": utf16le(lines),
		"utf-8":    []byte(lines),
	} {
		t.Run(name, func(t *testing.T) {
			if FormatOf(data) != FormatWordfast {
				t.Fatal(FormatOf(data))
			}
			f, err := ReadFile(data)
			if err != nil {
				t.Fatal(err)
			}
			if f.SourceLang != "de" || f.TargetLang != "en-GB" || len(f.Units) != 2 || f.Skipped != 1 {
				t.Fatalf("file: %+v", f)
			}
			u := f.Units[0]
			if shows(u.Source) != "{1/}Mieter’s Kaution & Miete\nZeile 2" || shows(u.Target) != "{1/}tenant’s deposit & rent\nline 2" || u.Source[0].Tag.Native != "&tA;" {
				t.Fatalf("unit: %q %q", shows(u.Source), shows(u.Target))
			}
			if u.CreatedBy != "Judy" || u.UsageCount != 2 || !u.CreatedAt.Equal(time.Date(2014, 12, 2, 6, 47, 25, 0, time.UTC)) || !reflect.DeepEqual(u.Attributes, []Attribute{{"Client", "Weber"}}) {
				t.Fatalf("unit: %+v", u.Unit)
			}
		})
	}
	// Latin-1, as older versions write.
	latin := []byte("%20141201~221624\t%U\t%TU=1\t%DE-DE\t%Wordfast TM v.5\t%FR-FR\r\n20141202~064725\tJ\t0\tDE-DE\tStra\xdfe\tFR-FR\true\r\n")
	f, err := ReadFile(latin)
	if err != nil || len(f.Units) != 1 || f.Units[0].Source.Plain() != "Straße" {
		t.Fatalf("latin-1: %+v %v", f, err)
	}
	// Not Wordfast: a text file that merely starts with "%".
	if IsWordfastTM([]byte("%PDF-1.4\tx\ty\tz\tw\tv\n")) {
		t.Fatal("a PDF is Wordfast")
	}
}
