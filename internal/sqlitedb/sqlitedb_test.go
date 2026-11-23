package sqlitedb

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestFileDSN(t *testing.T) {
	cases := map[string]string{
		"/home/dan/memories.db":                     "file:/home/dan/memories.db?",
		"C:/Users/Dan/AppData/Roaming/x.db":         "file:C:/Users/Dan/AppData/Roaming/x.db?",
		"C:/Users/Dan/Übersetzungen/Mein TM#1?.db":  "file:C:/Users/Dan/%C3%9Cbersetzungen/Mein%20TM%231%3F.db?",
		"C:/Users/RUNNER~1/AppData/Local/Temp/a.db": "file:C:/Users/RUNNER~1/AppData/Local/Temp/a.db?",
	}
	for in, prefix := range cases {
		got := FileDSN(in)
		if !strings.HasPrefix(got, prefix) || !strings.Contains(got, "_txlock=immediate") || !strings.Contains(got, "busy_timeout%2810000%29") {
			t.Errorf("FileDSN(%q) = %q", in, got)
		}
		if strings.Count(got, "?") != 1 {
			t.Errorf("FileDSN(%q) has an unescaped '?': %q", in, got)
		}
	}
}

func TestMigrations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.db")
	m := []string{`CREATE TABLE a (x INTEGER)`}
	db, err := Open(path, "test", m)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	// A second version is applied on the next open; the first is not rerun.
	db, err = Open(path, "test", append(m, `CREATE TABLE b (y INTEGER)`))
	if err != nil {
		t.Fatal(err)
	}
	var v int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil || v != 2 {
		t.Fatalf("version %d, %v", v, err)
	}
	db.Close()
	// An older program refuses a newer database.
	if _, err := Open(path, "test", m); err == nil || !strings.Contains(err.Error(), "newer") {
		t.Fatalf("expected a newer-schema error, got %v", err)
	}
	if _, err := Open("  ", "test", m); err == nil {
		t.Fatal("empty path accepted")
	}
}
