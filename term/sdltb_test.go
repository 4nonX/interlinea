package term

import (
	"flag"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/4nonX/interlinea/internal/jet/jettest"
)

var updateSDLTB = flag.Bool("update-sdltb", false, "rewrite testdata/orchard.sdltb")

// orchard is a made-up MultiTerm termbase, written the way MultiTerm
// stores one: an entry a row, as XML with shortened element names.
func orchard(key uint32) []byte {
	entry := func(id int, who, subject, note string, langs ...[]string) string {
		var b strings.Builder
		b.WriteString(`<cG><c alternativeId="">` + strconv.Itoa(id) + `</c>`)
		b.WriteString(`<trG><tr type="origination">` + who + `</tr><dt>2021-03-04T09:15:00</dt></trG>`)
		b.WriteString(`<trG><tr type="modification">` + who + `</tr><dt>2022-11-30T16:40:12</dt></trG>`)
		b.WriteString(`<dG><d type="Subject">` + subject + `</d></dG>`)
		if note != "" {
			b.WriteString(`<dG><d type="Note">` + note + `</d></dG>`)
		}
		for _, l := range langs {
			b.WriteString(`<lG><l type="` + l[0] + `" lang="` + l[1] + `"></l>`)
			for _, t := range l[2:] {
				p := strings.SplitN(t, "|", 3)
				b.WriteString(`<tG><t>` + p[0] + `</t>`)
				if len(p) > 1 && p[1] != "" {
					b.WriteString(`<dG><d type="Status">` + p[1] + `</d></dG>`)
				}
				if len(p) > 2 && p[2] != "" {
					b.WriteString(`<dG><d type="Definition">` + p[2] + `</d></dG>`)
				}
				b.WriteString(`</tG>`)
			}
			b.WriteString(`</lG>`)
		}
		b.WriteString(`</cG>`)
		return b.String()
	}
	page := strings.Repeat("A saw for cutting branches, with a curved blade. ", 50)
	pages := strings.Repeat("Ein Messer zum Veredeln von Obstbäumen, mit gerader Klinge. ", 180)
	concepts := jettest.Table{
		Name: "mtConcepts",
		Columns: []jettest.Column{
			{Name: "conceptid", Type: jettest.Int32}, {Name: "text", Type: jettest.Memo}, {Name: "incomplete", Type: jettest.Bool},
		},
		Rows: [][]any{
			// Out of order: MultiTerm gives an entry back by its number.
			{2, entry(2, "mara", "fruit", "Salt &amp; pepper are not fruit; 2 &lt; 3",
				[]string{"English", "EN", "pear"}, []string{"German", "DE", "Birne"}), false},
			{1, entry(1, "mara", "fruit", `See also (<xref Tlink="English:pear">pear</xref> 2021)`,
				[]string{"English", "EN", "apple|approved|The fruit of the apple tree.", "eating apple|deprecated"},
				[]string{"German", "DE", "Apfel|approved|Die Frucht des Apfelbaums.", "Tafelapfel"},
				[]string{"Greek", "EL", "μήλο"},
				[]string{"Japanese", "JA", "りんご||バラ科の果実。"},
				[]string{"Arabic", "AR", "تفاحة"}), false},
			{3, entry(3, "jon", "tools", "",
				[]string{"English", "EN", "pruning saw||" + page}, []string{"German", "DE", "Baumsäge"}), false},
			{4, entry(4, "jon", "tools", "",
				[]string{"English", "EN", "grafting knife"}, []string{"German", "DE", "Okuliermesser||" + pages}), false},
			{5, entry(5, "jon", "removed", "", []string{"English", "EN", "withdrawn term"}), false},
			{6, entry(6, "mara", "varieties", "renamed",
				[]string{"English", "EN", "Orchard Queen|approved|A late variety that keeps through the winter."},
				[]string{"German", "DE", "Obstgartenkönigin|approved"}), true},
		},
		Deleted: map[int]bool{4: true},
		Moved:   map[int]bool{5: true},
	}
	blobs := jettest.Table{
		Name: "mtBlobs",
		Columns: []jettest.Column{
			{Name: "objectid", Type: jettest.Int32}, {Name: "conceptid", Type: jettest.Int32},
			{Name: "blob", Type: jettest.Memo}, {Name: "name", Type: jettest.Text},
		},
		Rows: [][]any{
			{-6, -6, "A made-up termbase for Interlinea's tests", "Description"},
			{-5, -5, "Orchard", "FriendlyName"},
		},
	}
	indexes := jettest.Table{
		Name:    "mtIndexes",
		Columns: []jettest.Column{{Name: "name", Type: jettest.Text}, {Name: "locale", Type: jettest.Text}},
		Rows:    [][]any{{"English", "EN"}, {"German", "DE"}, {"Greek", "EL"}, {"Japanese", "JA"}, {"Arabic", "AR"}},
	}
	return jettest.Build([]jettest.Table{concepts, blobs, indexes}, key)
}

func TestReadSDLTB(t *testing.T) {
	if *updateSDLTB {
		if err := os.WriteFile("testdata/orchard.sdltb", orchard(0), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	file, err := os.ReadFile("testdata/orchard.sdltb")
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{"file": file, "built": orchard(0), "encrypted": orchard(0x1D2C3B4A)} {
		t.Run(name, func(t *testing.T) {
			f, err := Detect("whatever.bin", data)
			if err != nil || f != FormatSDLTB {
				t.Fatalf("detected %q, %v", f, err)
			}
			tb, rep, err := Read(f, data, nil)
			if err != nil {
				t.Fatal(err)
			}
			if tb.Name != "Orchard" {
				t.Errorf("name %q", tb.Name)
			}
			if rep.Entries != 5 || rep.Terms != 15 || rep.Skipped != 0 {
				t.Errorf("report %+v", rep)
			}
			if got := strings.Join(tb.Languages, " "); got != "en de el ja ar" {
				t.Errorf("languages %q", got)
			}
			var ids []string
			for _, e := range tb.Entries {
				ids = append(ids, e.ID)
			}
			if got := strings.Join(ids, " "); got != "1 2 3 4 6" {
				t.Fatalf("entries %q: the deleted one must be gone, the moved one read once, all in order", got)
			}
			apple := tb.Entries[0]
			if apple.CreatedBy != "mara" || apple.CreatedAt.Format("2006-01-02") != "2021-03-04" || apple.ChangedAt.Format("2006-01-02") != "2022-11-30" {
				t.Errorf("history of entry 1: %+v", apple)
			}
			if v := fieldValue(apple.Fields, "Note"); v != "See also (pear 2021)" {
				t.Errorf("a cross reference inside a note: %q", v)
			}
			en := apple.Languages[0]
			if en.Lang != "en" || len(en.Terms) != 2 || en.Terms[0].Text != "apple" || en.Terms[0].Status != StatusApproved ||
				en.Terms[1].Text != "eating apple" || en.Terms[1].Status != StatusForbidden {
				t.Errorf("English of entry 1: %+v", en)
			}
			if v := fieldValue(en.Terms[0].Fields, "Definition"); v != "The fruit of the apple tree." {
				t.Errorf("definition %q", v)
			}
			var others []string
			for _, l := range apple.Languages[2:] {
				others = append(others, l.Lang+"="+l.Terms[0].Text)
			}
			if got := strings.Join(others, " "); got != "el=μήλο ja=りんご ar=تفاحة" {
				t.Errorf("other scripts: %q", got)
			}
			if v := fieldValue(apple.Languages[3].Terms[0].Fields, "Definition"); v != "バラ科の果実。" {
				t.Errorf("Japanese definition %q", v)
			}
			if v := fieldValue(tb.Entries[1].Fields, "Note"); v != "Salt & pepper are not fruit; 2 < 3" {
				t.Errorf("escaped characters: %q", v)
			}
			if v := fieldValue(tb.Entries[2].Languages[0].Terms[0].Fields, "Definition"); len(v) != len(strings.TrimSpace(strings.Repeat("A saw for cutting branches, with a curved blade. ", 50))) {
				t.Errorf("a definition on a page of its own: %d characters", len(v))
			}
			long := fieldValue(tb.Entries[3].Languages[1].Terms[0].Fields, "Definition")
			if want := strings.TrimSpace(strings.Repeat("Ein Messer zum Veredeln von Obstbäumen, mit gerader Klinge. ", 180)); long != want {
				t.Errorf("a definition over several pages: %d bytes, want %d", len(long), len(want))
			}
			if tb.Entries[4].Languages[0].Terms[0].Text != "Orchard Queen" {
				t.Errorf("the moved entry: %+v", tb.Entries[4])
			}

			// What was read goes out again in the formats MultiTerm takes.
			for _, out := range []Format{FormatMultiTerm, FormatTBX} {
				written, err := Write(tb, out)
				if err != nil {
					t.Fatal(err)
				}
				back, _, err := Read(out, written, nil)
				if err != nil {
					t.Fatal(err)
				}
				if len(back.Entries) != 5 || back.Entries[0].Languages[3].Terms[0].Text != "りんご" {
					t.Errorf("%s: read back %d entries", out, len(back.Entries))
				}
			}
			if _, err := Write(tb, FormatSDLTB); err == nil || !strings.Contains(err.Error(), "MultiTerm XML") {
				t.Errorf("writing the format: %v", err)
			}
		})
	}
}

func TestSDLTBRefuses(t *testing.T) {
	other := jettest.Build([]jettest.Table{{
		Name: "Customers", Columns: []jettest.Column{{Name: "name", Type: jettest.Text}}, Rows: [][]any{{"Mara"}},
	}}, 0)
	if _, _, err := ReadSDLTB(other); err == nil || !strings.Contains(err.Error(), "not a MultiTerm termbase") {
		t.Errorf("another Access database: %v", err)
	}
	data := orchard(0)
	if _, _, err := ReadSDLTB(data[:3*4096]); err == nil {
		t.Error("a file cut short was read")
	}
	// A damaged file gives an error or what is left of it, never a crash.
	for at := 2 * 4096; at < len(data); at += 53 {
		bad := append([]byte(nil), data...)
		bad[at] ^= 0xFF
		_, _, _ = ReadSDLTB(bad)
	}
}

func fieldValue(vs []Value, name string) string {
	for _, v := range vs {
		if v.Field == name {
			return v.Value
		}
	}
	return ""
}
