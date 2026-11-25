package sqlitefile

import (
	"bytes"
	"database/sql"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
)

// build writes a database with SQLite itself and returns its file.
func build(t *testing.T, pragmas []string, stmts []string, args ...[]any) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "t.db")
	db, err := sql.Open("sqlite3", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			t.Fatal(p, err)
		}
	}
	for i, s := range stmts {
		var a []any
		if i < len(args) {
			a = args[i]
		}
		if _, err := db.Exec(s, a...); err != nil {
			t.Fatal(s, err)
		}
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

func rows(t *testing.T, db *DB, name string) []Row {
	t.Helper()
	var out []Row
	if err := db.Rows(name, func(r Row) error { out = append(out, r); return nil }); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestValues(t *testing.T) {
	data := build(t, nil, []string{
		`CREATE TABLE "values" (id INTEGER PRIMARY KEY, i INT, f REAL, s TEXT, b BLOB, "odd name" TEXT, [br] INT)`,
		`INSERT INTO "values" VALUES (NULL, 0, 0.5, 'ä€😀', x'00ff', NULL, 1)`,
		`INSERT INTO "values" VALUES (7, -1, -2.25, '', x'', 'x', 127)`,
		`INSERT INTO "values" VALUES (NULL, 128, 1e300, 'a', NULL, 'y', -32768)`,
		`INSERT INTO "values" VALUES (NULL, 8388607, 0, 'b', NULL, 'z', 2147483647)`,
		`INSERT INTO "values" VALUES (NULL, -140737488355328, 0, 'c', NULL, 'w', 9223372036854775807)`,
		`INSERT INTO "values" VALUES (NULL, -9223372036854775808, 0, 'd', NULL, 'v', 1099511627776)`,
		`ALTER TABLE "values" ADD COLUMN later TEXT`,
		`INSERT INTO "values" VALUES (NULL, 2, 3, 'e', NULL, 'u', 4, 'set')`,
	})
	db, err := Open(data)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(db.Columns("VALUES"), []string{"id", "i", "f", "s", "b", "odd name", "br", "later"}) {
		t.Fatalf("columns: %q", db.Columns("values"))
	}
	got := rows(t, db, "values")
	want := []Row{
		{"id": int64(1), "i": int64(0), "f": 0.5, "s": "ä€😀", "b": []byte{0, 0xff}, "odd name": nil, "br": int64(1), "later": nil},
		{"id": int64(7), "i": int64(-1), "f": -2.25, "s": "", "b": []byte{}, "odd name": "x", "br": int64(127), "later": nil},
		{"id": int64(8), "i": int64(128), "f": 1e300, "s": "a", "b": nil, "odd name": "y", "br": int64(-32768), "later": nil},
		{"id": int64(9), "i": int64(8388607), "f": 0.0, "s": "b", "b": nil, "odd name": "z", "br": int64(2147483647), "later": nil},
		{"id": int64(10), "i": int64(-140737488355328), "f": 0.0, "s": "c", "b": nil, "odd name": "w", "br": int64(math.MaxInt64), "later": nil},
		{"id": int64(11), "i": int64(math.MinInt64), "f": 0.0, "s": "d", "b": nil, "odd name": "v", "br": int64(1099511627776), "later": nil},
		{"id": int64(12), "i": int64(2), "f": 3.0, "s": "e", "b": nil, "odd name": "u", "br": int64(4), "later": "set"},
	}
	if len(got) != len(want) {
		t.Fatalf("%d rows", len(got))
	}
	for i := range want {
		for k, w := range want[i] {
			g := got[i][k]
			// SQLite stores a REAL with no fraction as an integer on disk;
			// either reads as the same number.
			if gi, ok := g.(int64); ok {
				if wf, ok := w.(float64); ok && float64(gi) == wf {
					continue
				}
			}
			if !reflect.DeepEqual(g, w) {
				t.Errorf("row %d %s: %#v, want %#v", i, k, g, w)
			}
		}
	}
	if err := db.Rows("missing", func(Row) error { return nil }); err == nil {
		t.Fatal("missing table")
	}
}

// Rows on many pages, text longer than a page (overflow pages), every page
// size and both UTF-16 byte orders.
func TestPagesAndEncodings(t *testing.T) {
	long := strings.Repeat("Übersetzung 翻訳 ", 3000)
	for _, ps := range []int{512, 1024, 4096, 65536} {
		for _, enc := range []string{"UTF-8", "UTF-16le", "UTF-16be"} {
			t.Run(fmt.Sprintf("%d-%s", ps, enc), func(t *testing.T) {
				stmts := []string{`CREATE TABLE units (id INTEGER PRIMARY KEY AUTOINCREMENT, source TEXT, target TEXT, n INT)`, `CREATE INDEX units_n ON units(n)`}
				var args [][]any
				args = append(args, nil, nil)
				for i := 0; i < 600; i++ {
					stmts = append(stmts, `INSERT INTO units (source, target, n) VALUES (?, ?, ?)`)
					tgt := fmt.Sprintf("Ziel %d", i)
					if i%97 == 0 {
						tgt = long + tgt
					}
					args = append(args, []any{fmt.Sprintf("source %d", i), tgt, i})
				}
				stmts = append(stmts, `DELETE FROM units WHERE n % 50 = 3`)
				data := build(t, []string{fmt.Sprintf("PRAGMA page_size = %d", ps), fmt.Sprintf("PRAGMA encoding = '%s'", enc)}, stmts, args...)
				db, err := Open(data)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(db.Tables(), []string{"units", "sqlite_sequence"}) {
					t.Fatalf("tables: %v", db.Tables())
				}
				got := rows(t, db, "units")
				if len(got) != 588 {
					t.Fatalf("%d rows", len(got))
				}
				j := 0
				for i := 0; i < 600; i++ {
					if i%50 == 3 {
						continue
					}
					r := got[j]
					j++
					tgt := fmt.Sprintf("Ziel %d", i)
					if i%97 == 0 {
						tgt = long + tgt
					}
					if r["id"] != int64(i+1) || r["source"] != fmt.Sprintf("source %d", i) || r["target"] != tgt || r["n"] != int64(i) {
						t.Fatalf("row %d: id %v source %v n %v, target of %d bytes", i, r["id"], r["source"], r["n"], len(fmt.Sprint(r["target"])))
					}
				}
			})
		}
	}
}

func TestNotADatabase(t *testing.T) {
	for _, b := range [][]byte{nil, []byte("PK\x03\x04"), bytes.Repeat([]byte{0}, 4096)} {
		if _, err := Open(b); err == nil {
			t.Fatalf("%q opened", b)
		}
	}
	// A database cut short is refused, not read past its end.
	data := build(t, []string{"PRAGMA page_size = 512"}, []string{`CREATE TABLE t (s TEXT)`, `INSERT INTO t VALUES (?)`}, nil, []any{strings.Repeat("x", 5000)})
	db, err := Open(data[:len(data)-512])
	if err != nil {
		return
	}
	if err := db.Rows("t", func(Row) error { return nil }); err == nil {
		t.Fatal("a cut database read without an error")
	}
}

func TestColumns(t *testing.T) {
	for sql, want := range map[string][]string{
		`CREATE TABLE a (x INTEGER PRIMARY KEY, "y z" TEXT DEFAULT 'a,b', w NUMERIC(10, 2), CONSTRAINT c UNIQUE (y), PRIMARY KEY (w))`: {"x", "y z", "w"},
		"CREATE TABLE `b` ([q] TEXT, r CHECK (r > 0), FOREIGN KEY (q) REFERENCES a(x))":                                                {"q", "r"},
		// As in a Trados Studio memory: a comment with a comma in it.
		"CREATE TABLE attributes(\r\n\tid INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,\r\n\ttm_id INT NOT NULL, -- CONSTRAINT FK_a_t REFERENCES translation_memories(id) ON DELETE CASCADE,\r\nCONSTRAINT CK_a UNIQUE (\r\n\tname, tm_id\r\n)\r\n)": {"id", "tm_id"},
		"CREATE TABLE c (a /* x, y */ TEXT, 'b''c' INT)": {"a", "b'c"},
	} {
		got, _, _ := columns(sql)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: %q", sql, got)
		}
	}
	if _, rowid, _ := columns(`CREATE TABLE a (n TEXT, id integer primary key autoincrement)`); rowid != 1 {
		t.Fatalf("rowid alias: %d", rowid)
	}
	if _, _, without := columns(`CREATE TABLE a (k TEXT PRIMARY KEY) WITHOUT ROWID`); !without {
		t.Fatal("WITHOUT ROWID")
	}
}
