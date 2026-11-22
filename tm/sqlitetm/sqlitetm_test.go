package sqlitetm

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/tm"
	"github.com/4nonX/interlinea/tm/tmtest"
)

var ctx = context.Background()

func openTemp(t testing.TB) (*DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sub", "memories.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db, path
}

func TestConformance(t *testing.T) {
	tmtest.Run(t, func(t *testing.T, info tm.Info) tm.Memory {
		db, _ := openTemp(t)
		m, err := db.Create(ctx, info)
		if err != nil {
			t.Fatal(err)
		}
		return m
	})
}

func TestConformanceSharedDatabase(t *testing.T) {
	// All memories of the suite in one file: lookups must stay within their memory.
	db, _ := openTemp(t)
	n := 0
	tmtest.Run(t, func(t *testing.T, info tm.Info) tm.Memory {
		n++
		info.Name = fmt.Sprintf("%s #%d", info.Name, n)
		m, err := db.Create(ctx, info)
		if err != nil {
			t.Fatal(err)
		}
		return m
	})
}

func TestPersistenceAndManagement(t *testing.T) {
	db, path := openTemp(t)
	m, err := db.Create(ctx, tm.Info{Name: "Verträge", SourceLang: "de_de", TargetLang: "en-gb"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Create(ctx, tm.Info{Name: "verträge", SourceLang: "de", TargetLang: "en"}); !errors.Is(err, ErrExists) {
		// NOCASE folds ASCII only, so this name differs; check a pure ASCII duplicate too.
		if _, err := db.Create(ctx, tm.Info{Name: "Verträge", SourceLang: "de", TargetLang: "en"}); !errors.Is(err, ErrExists) {
			t.Fatalf("duplicate name: %v", err)
		}
	}
	if _, err := db.Create(ctx, tm.Info{Name: "", SourceLang: "de", TargetLang: "en"}); !errors.Is(err, tm.ErrInvalidInfo) {
		t.Fatalf("invalid info: %v", err)
	}
	if _, err := m.Add(ctx, tm.Unit{Source: content.Text("Der Mieter haftet."), Target: content.Text("The tenant is liable.")}, tm.KeepBoth); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()
	m2, err := db2.Memory(ctx, "Verträge")
	if err != nil {
		t.Fatal(err)
	}
	if info := m2.Info(); info.SourceLang != "de-DE" || info.TargetLang != "en-GB" {
		t.Fatalf("info = %+v", info)
	}
	ms, err := m2.Lookup(ctx, content.Text("Der Mieter haftet."), tm.LookupOptions{})
	if err != nil || len(ms) != 1 || ms[0].Score != 100 {
		t.Fatalf("lookup after reopen = %+v, %v", ms, err)
	}
	list, err := db2.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range list {
		if s.Name == "Verträge" {
			found = s.Units == 1 && !s.CreatedAt.IsZero()
		}
	}
	if !found {
		t.Fatalf("list = %+v", list)
	}
	if err := db2.Delete(ctx, "Verträge"); err != nil {
		t.Fatal(err)
	}
	if _, err := db2.Memory(ctx, "Verträge"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted memory still there: %v", err)
	}
	if err := db2.Delete(ctx, "Verträge"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete: %v", err)
	}
	var orphans int
	if err := db2.sql.QueryRow(`SELECT count(*) FROM units`).Scan(&orphans); err != nil || orphans != 0 {
		t.Fatalf("units left after delete: %d %v", orphans, err)
	}
	var ftsRows int
	if err := db2.sql.QueryRow(`SELECT count(*) FROM units_fts WHERE units_fts MATCH '"mieter"'`).Scan(&ftsRows); err != nil || ftsRows != 0 {
		t.Fatalf("search index not cleaned: %d %v", ftsRows, err)
	}
}

func TestNewerSchemaRejected(t *testing.T) {
	db, path := openTemp(t)
	if _, err := db.sql.Exec(`PRAGMA user_version = 99`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if _, err := Open(path); err == nil || !strings.Contains(err.Error(), "newer") {
		t.Fatalf("expected schema error, got %v", err)
	}
	if _, err := Open(" "); err == nil {
		t.Fatal("empty path accepted")
	}
}

func TestUpdateKeepsSearchIndexInSync(t *testing.T) {
	db, _ := openTemp(t)
	m, _ := db.Create(ctx, tm.Info{Name: "x", SourceLang: "de", TargetLang: "en"})
	if _, err := m.Add(ctx, tm.Unit{Source: content.Text("Haftung des Mieters."), Target: content.Text("Liability of the tenant.")}, tm.KeepBoth); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Add(ctx, tm.Unit{Source: content.Text("Haftung des Mieters."), Target: content.Text("Lessee's liability.")}, tm.Overwrite); err != nil {
		t.Fatal(err)
	}
	if got, _ := m.Concordance(ctx, "tenant", tm.ConcordanceOptions{InTarget: true}); len(got) != 0 {
		t.Fatalf("old target still indexed: %v", got)
	}
	if got, _ := m.Concordance(ctx, "lessee", tm.ConcordanceOptions{InTarget: true}); len(got) != 1 {
		t.Fatalf("new target not indexed: %v", got)
	}
}

func TestConcurrentWriters(t *testing.T) {
	db, _ := openTemp(t)
	m, _ := db.Create(ctx, tm.Info{Name: "c", SourceLang: "de", TargetLang: "en"})
	var wg sync.WaitGroup
	for g := 0; g < 6; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 40; i++ {
				src := fmt.Sprintf("Satz %d aus Routine %d.", i, g)
				if _, err := m.Add(ctx, tm.Unit{Source: content.Text(src), Target: content.Text("t")}, tm.KeepBoth); err != nil {
					t.Error(err)
					return
				}
				if _, err := m.Lookup(ctx, content.Text(src), tm.LookupOptions{}); err != nil {
					t.Error(err)
					return
				}
			}
		}(g)
	}
	wg.Wait()
	if n, _ := m.Count(ctx); n != 240 {
		t.Fatalf("count = %d", n)
	}
}

func TestFTSQuoting(t *testing.T) {
	db, _ := openTemp(t)
	m, _ := db.Create(ctx, tm.Info{Name: "q", SourceLang: "en", TargetLang: "de"})
	if _, err := m.Add(ctx, tm.Unit{Source: content.Text(`He said "stop" AND left (NOT really).`), Target: content.Text("x")}, tm.KeepBoth); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{`"stop"`, `AND left`, `(NOT`, `really).`} {
		got, err := m.Concordance(ctx, q, tm.ConcordanceOptions{})
		if err != nil || len(got) != 1 {
			t.Errorf("concordance %q = %v, %v", q, got, err)
		}
	}
	if _, err := m.Lookup(ctx, content.Text(`"AND" OR NOT NEAR(*)`), tm.LookupOptions{}); err != nil {
		t.Errorf("lookup with FTS syntax characters: %v", err)
	}
}

func BenchmarkLookup10k(b *testing.B) {
	db, _ := openTemp(b)
	m, _ := db.Create(ctx, tm.Info{Name: "b", SourceLang: "de", TargetLang: "en"})
	words := strings.Fields("Mieter Vermieter haftet Schäden Mietsache Vertrag Kündigung Frist Monate Kaution Zahlung Miete Wohnung Nebenkosten Abrechnung jährlich schriftlich")
	batch := make([]tm.Unit, 0, 10000)
	for i := 0; i < 10000; i++ {
		var sb strings.Builder
		for j := 0; j < 10; j++ {
			sb.WriteString(words[(i*7+j*3)%len(words)])
			sb.WriteByte(' ')
		}
		fmt.Fprintf(&sb, "%d.", i)
		batch = append(batch, tm.Unit{Source: content.Text(sb.String()), Target: content.Text("x")})
	}
	if err := m.AddBatch(ctx, batch, tm.KeepBoth); err != nil {
		b.Fatal(err)
	}
	q := content.Text("Der Mieter haftet für Schäden an der Mietsache laut Vertrag 42.")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := m.Lookup(ctx, q, tm.LookupOptions{}); err != nil {
			b.Fatal(err)
		}
	}
}

func TestFileDSN(t *testing.T) {
	cases := map[string]string{
		"/home/dan/memories.db":                     "file:/home/dan/memories.db?",
		"C:/Users/Dan/AppData/Roaming/x.db":         "file:C:/Users/Dan/AppData/Roaming/x.db?",
		"C:/Users/Dan/Übersetzungen/Mein TM#1?.db":  "file:C:/Users/Dan/%C3%9Cbersetzungen/Mein%20TM%231%3F.db?",
		"C:/Users/RUNNER~1/AppData/Local/Temp/a.db": "file:C:/Users/RUNNER~1/AppData/Local/Temp/a.db?",
	}
	for in, prefix := range cases {
		got := fileDSN(in)
		if !strings.HasPrefix(got, prefix) || !strings.Contains(got, "_txlock=immediate") || !strings.Contains(got, "journal_mode%28wal%29") {
			t.Errorf("fileDSN(%q) = %q", in, got)
		}
		if strings.Count(got, "?") != 1 {
			t.Errorf("fileDSN(%q) has an unescaped '?': %q", in, got)
		}
	}
}

func TestOpenPathWithSpecialCharacters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Übersetzungen", "Mein TM #1?.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Create(ctx, tm.Info{Name: "x", SourceLang: "de", TargetLang: "en"}); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("database not created at the literal path: %v", err)
	}
}
