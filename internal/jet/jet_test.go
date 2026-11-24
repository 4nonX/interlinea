package jet_test

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/4nonX/interlinea/internal/jet"
	"github.com/4nonX/interlinea/internal/jet/jettest"
)

// sample is a database with every kind of field and row the reader knows.
func sample() ([]jettest.Table, []jet.Row) {
	page := strings.Repeat("A definition that needs a page of its own. ", 60)
	pages := strings.Repeat("Eine Erläuterung über mehrere Seiten, mit Ωμέγα und 日本語. ", 400)
	notes := jettest.Table{
		Name: "Notes",
		Columns: []jettest.Column{
			{Name: "id", Type: jettest.Int32}, {Name: "title", Type: jettest.Text},
			{Name: "body", Type: jettest.Memo}, {Name: "done", Type: jettest.Bool},
			{Name: "rank", Type: jettest.Int16},
		},
		Rows: [][]any{
			{1, "plain", "short text", true, 7},
			{2, "Größe", "μήλο, りんご, تفاحة", false, -3},
			{3, "one page", page, false, 0},
			{4, "many pages", pages, true, 12},
			{5, "gone", "a deleted row", false, 1},
			{6, "moved", "a row that outgrew its page", true, 2},
			{7, nil, nil, false, nil},
			{-8, "", "", true, 32000},
		},
		Deleted: map[int]bool{4: true},
		Moved:   map[int]bool{5: true},
	}
	want := []jet.Row{
		{"id": int64(1), "title": "plain", "body": "short text", "done": true, "rank": int64(7)},
		{"id": int64(2), "title": "Größe", "body": "μήλο, りんご, تفاحة", "done": false, "rank": int64(-3)},
		{"id": int64(3), "title": "one page", "body": page, "done": false, "rank": int64(0)},
		{"id": int64(4), "title": "many pages", "body": pages, "done": true, "rank": int64(12)},
		{"id": int64(6), "title": "moved", "body": "a row that outgrew its page", "done": true, "rank": int64(2)},
		{"id": int64(7), "title": nil, "body": nil, "done": false, "rank": nil},
		{"id": int64(-8), "title": "", "body": "", "done": true, "rank": int64(32000)},
	}
	many := jettest.Table{Name: "Many", Columns: []jettest.Column{{Name: "n", Type: jettest.Int32}, {Name: "text", Type: jettest.Text}}}
	for i := 0; i < 900; i++ {
		many.Rows = append(many.Rows, []any{i, fmt.Sprintf("row number %d of a table over several pages", i)})
	}
	swapped := notes
	swapped.Name, swapped.Swapped = "Swapped", true
	return []jettest.Table{notes, many, swapped}, want
}

func rows(t *testing.T, db *jet.DB, name string) []jet.Row {
	t.Helper()
	table, err := db.Table(name)
	if err != nil {
		t.Fatal(err)
	}
	var out []jet.Row
	if err := table.Rows(func(r jet.Row) error { out = append(out, r); return nil }); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestRead(t *testing.T) {
	tables, want := sample()
	for _, key := range []uint32{0, 0x5A17C3E9} {
		name := "plain"
		if key != 0 {
			name = "encrypted"
		}
		t.Run(name, func(t *testing.T) {
			data := jettest.Build(tables, key)
			if !jet.IsDatabase(data) {
				t.Fatal("not recognized as a database")
			}
			db, err := jet.Open(data)
			if err != nil {
				t.Fatal(err)
			}
			if got := db.Tables(); !reflect.DeepEqual(got, []string{"Notes", "Many", "Swapped"}) {
				t.Fatalf("tables: %v", got)
			}
			for _, name := range []string{"notes", "Swapped"} {
				got := rows(t, db, name)
				if len(got) != len(want) {
					t.Fatalf("%s: %d rows, want %d", name, len(got), len(want))
				}
				for i := range want {
					if !reflect.DeepEqual(got[i], want[i]) {
						g, w := fmt.Sprint(got[i]), fmt.Sprint(want[i])
						if len(g) > 300 {
							g, w = g[:300], w[:300]
						}
						t.Errorf("%s, row %d:\n got %s\nwant %s", name, i, g, w)
					}
				}
			}
			many := rows(t, db, "Many")
			if len(many) != 900 {
				t.Fatalf("%d rows in the long table", len(many))
			}
			for i, r := range many {
				if r["n"] != int64(i) || r["text"] != fmt.Sprintf("row number %d of a table over several pages", i) {
					t.Fatalf("row %d of the long table: %v", i, r)
				}
			}
			if _, err := db.Table("nothing"); err == nil {
				t.Error("a table that is not there was opened")
			}
		})
	}
}

// TestWriteSample writes the sample to the file named by JET_SAMPLE, for
// checking it with another reader of the format.
func TestWriteSample(t *testing.T) {
	path := os.Getenv("JET_SAMPLE")
	if path == "" {
		t.Skip("JET_SAMPLE is not set")
	}
	tables, _ := sample()
	if err := os.WriteFile(path, jettest.Build(tables, 0), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".enc", jettest.Build(tables, 0x5A17C3E9), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRefuses(t *testing.T) {
	tables, _ := sample()
	data := jettest.Build(tables, 0)
	if _, err := jet.Open([]byte("not a database at all")); err == nil {
		t.Error("text was opened as a database")
	}
	if _, err := jet.Open(data[:5000]); err == nil {
		t.Error("a file cut off after its first page was opened")
	}
	old := append([]byte(nil), data...)
	old[0x14] = 0
	if _, err := jet.Open(old); err == nil || !strings.Contains(err.Error(), "Jet 3") {
		t.Errorf("an Access 97 file: %v", err)
	}
	// Damage must give an error or fewer rows, never a crash.
	for at := 2 * 4096; at < len(data); at += 97 {
		bad := append([]byte(nil), data...)
		bad[at] ^= 0xFF
		bad[at+1] ^= 0x55
		db, err := jet.Open(bad)
		if err != nil {
			continue
		}
		for _, name := range db.Tables() {
			if table, err := db.Table(name); err == nil {
				_ = table.Rows(func(jet.Row) error { return nil })
			}
		}
	}
}
