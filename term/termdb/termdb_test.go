package termdb

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/4nonX/interlinea/term"
)

var ctx = context.Background()

// open returns an empty database and a key to open it again with
// reopen: a SQLite file, or a PostgreSQL schema (see postgres_test.go).
func open(t *testing.T) (*DB, string) {
	t.Helper()
	if pgDSN() != "" {
		return openPG(t)
	}
	path := filepath.Join(t.TempDir(), "termbases.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	clock := time.Date(2026, 11, 23, 9, 0, 0, 0, time.UTC)
	db.now = func() time.Time { clock = clock.Add(time.Second); return clock }
	return db, path
}

// reopen opens a second handle on the database, as another process
// would.
func reopen(t *testing.T, key string) *DB {
	t.Helper()
	if isPG(key) {
		return reopenPG(t, key)
	}
	db, err := Open(key)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func ent(id string, langs ...term.Language) term.Entry { return term.Entry{ID: id, Languages: langs} }

func lng(code string, terms ...term.Term) term.Language {
	return term.Language{Lang: code, Terms: terms}
}

func approved(s string) term.Term  { return term.Term{Text: s, Status: term.StatusApproved} }
func forbidden(s string) term.Term { return term.Term{Text: s, Status: term.StatusForbidden} }

func create(t *testing.T, db *DB) *Termbase {
	t.Helper()
	tb, err := db.Create(ctx, term.Definition{Name: "Kanzlei Weber", Languages: []string{"de_de", "en-GB"}, Fields: []term.Field{{Name: "Definition", Level: term.LevelEntry}}})
	if err != nil {
		t.Fatal(err)
	}
	return tb
}

func terms(e term.Entry, lang string) []string {
	var out []string
	for _, l := range e.Languages {
		if l.Lang == lang {
			for _, x := range l.Terms {
				out = append(out, x.Text)
			}
		}
	}
	return out
}

func TestTermbases(t *testing.T) {
	db, path := open(t)
	tb := create(t, db)
	if _, err := db.Create(ctx, term.Definition{Name: "kanzlei weber"}); !errors.Is(err, ErrExists) {
		t.Fatalf("duplicate name: %v", err)
	}
	if _, err := db.Create(ctx, term.Definition{Name: "  "}); err == nil {
		t.Fatal("empty name accepted")
	}
	if _, err := db.Create(ctx, term.Definition{Name: "X", Fields: []term.Field{{Name: "F", Level: "page"}}}); err == nil {
		t.Fatal("invalid field level accepted")
	}
	def, err := tb.Definition(ctx)
	if err != nil || !reflect.DeepEqual(def.Languages, []string{"de-DE", "en-GB"}) || def.Fields[0].Kind != term.KindText {
		t.Fatalf("definition: %+v %v", def, err)
	}
	if _, err := db.Create(ctx, term.Definition{Name: "Andere"}); err != nil {
		t.Fatal(err)
	}
	if err := tb.Rename(ctx, "Andere"); !errors.Is(err, ErrExists) {
		t.Fatalf("rename onto an existing name: %v", err)
	}
	if err := tb.Rename(ctx, "Weber Mietrecht"); err != nil || tb.Name() != "Weber Mietrecht" {
		t.Fatal(err)
	}
	if _, err := db.Termbase(ctx, "weber mietrecht"); err != nil {
		t.Fatal(err)
	}
	if _, err := tb.Add(ctx, ent("", lng("de", approved("Kaution"))), "Dan", ""); err != nil {
		t.Fatal(err)
	}
	list, err := db.List(ctx)
	if err != nil || len(list) != 2 || list[0].Name != "Andere" || list[1].Entries != 1 || list[1].Terms != 1 {
		t.Fatalf("list: %+v %v", list, err)
	}
	// Persistence across opening the file again.
	db.Close()
	db2 := reopen(t, path)
	defer db2.Close()
	tb2, err := db2.Termbase(ctx, "Weber Mietrecht")
	if err != nil {
		t.Fatal(err)
	}
	if n, _, _ := tb2.Count(ctx); n != 1 {
		t.Fatalf("after reopening: %d entries", n)
	}
	if err := db2.Delete(ctx, "Weber Mietrecht"); err != nil {
		t.Fatal(err)
	}
	if _, err := db2.Termbase(ctx, "Weber Mietrecht"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted termbase: %v", err)
	}
	if err := db2.Delete(ctx, "Weber Mietrecht"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete twice: %v", err)
	}
}

func TestEntriesAndHistory(t *testing.T) {
	db, _ := open(t)
	tb := create(t, db)
	s, err := tb.Add(ctx, ent("", lng("de", approved(" Kaution "), approved("kaution")), lng("en", approved("deposit"))), "Dan", "")
	if err != nil {
		t.Fatal(err)
	}
	// Languages resolve to the termbase's variants; duplicates and spaces go.
	if s.ID != "c1" || s.Version != 1 || !reflect.DeepEqual(terms(s.Entry, "de-DE"), []string{"Kaution"}) || !reflect.DeepEqual(terms(s.Entry, "en-GB"), []string{"deposit"}) {
		t.Fatalf("added: %+v", s)
	}
	if s.CreatedBy != "Dan" || s.CreatedAt.IsZero() {
		t.Fatalf("created: %+v", s.Entry)
	}
	// A free file ID is kept; a used one is replaced.
	a, _ := tb.Add(ctx, ent("K-7", lng("de-DE", approved("Miete"))), "Dan", "")
	b, _ := tb.Add(ctx, ent("K-7", lng("de-DE", approved("Mieter"))), "Dan", "")
	if a.ID != "K-7" || b.ID != "c2" {
		t.Fatalf("IDs: %q %q", a.ID, b.ID)
	}
	if _, err := tb.Add(ctx, ent("", lng("de-DE", approved("  "))), "Dan", ""); !errors.Is(err, ErrNoTerms) {
		t.Fatalf("entry without terms: %v", err)
	}
	// New languages and fields join the definition.
	c, err := tb.Add(ctx, term.Entry{Fields: []term.Value{{Field: "Domain", Value: "Mietrecht"}}, Languages: []term.Language{lng("fr", approved("loyer"))}}, "Dan", "")
	if err != nil {
		t.Fatal(err)
	}
	def, _ := tb.Definition(ctx)
	if !reflect.DeepEqual(def.Languages, []string{"de-DE", "en-GB", "fr"}) || len(def.Fields) != 2 || def.Fields[1].Name != "Domain" || def.Fields[1].Level != term.LevelEntry {
		t.Fatalf("definition grew: %+v", def)
	}

	// Update with version checks.
	e := s.Entry
	e.Languages[1].Terms = append(e.Languages[1].Terms, approved("security deposit"))
	u, err := tb.Update(ctx, e, 1, "Eva", "Added a synonym")
	if err != nil || u.Version != 2 || u.ChangedBy != "Eva" || u.CreatedBy != "Dan" {
		t.Fatalf("update: %+v %v", u, err)
	}
	if _, err := tb.Update(ctx, e, 1, "Max", "stale"); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale update: %v", err)
	}
	if _, err := tb.Update(ctx, ent("404", lng("de", approved("x"))), 1, "Max", ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing entry: %v", err)
	}
	h, err := tb.History(ctx, "c1")
	if err != nil || len(h) != 2 || h[0].Summary != "Created" || h[1].Summary != "Added a synonym" || h[1].By != "Eva" || len(terms(h[0].Entry, "en-GB")) != 1 {
		t.Fatalf("history: %+v %v", h, err)
	}

	// Delete keeps a record.
	if err := tb.DeleteEntry(ctx, c.ID, "Dan", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := tb.Get(ctx, c.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("deleted entry still there")
	}
	del, _ := tb.Deleted(ctx)
	if len(del) != 1 || del[0].EntryID != c.ID || del[0].Summary != "Deleted" || terms(del[0].Entry, "fr")[0] != "loyer" {
		t.Fatalf("deleted: %+v", del)
	}
	if res, _, _ := tb.Search(ctx, Query{Text: "loyer"}); len(res) != 0 {
		t.Fatal("deleted entry still found by search")
	}

	// A deleted entry comes back as it was, under its ID, and is found.
	if _, err := tb.Restore(ctx, "nothing", "Dan"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("restoring what was never deleted: %v", err)
	}
	back, err := tb.Restore(ctx, c.ID, "Eva")
	if err != nil || back.ID != c.ID || back.Version != 1 || terms(back.Entry, "fr")[0] != "loyer" {
		t.Fatalf("restored: %+v %v", back, err)
	}
	if back.CreatedBy != "Dan" || back.ChangedBy != "Eva" {
		t.Fatalf("restored by: %+v", back.Entry)
	}
	if del, _ := tb.Deleted(ctx); len(del) != 0 {
		t.Fatalf("still listed as deleted: %+v", del)
	}
	if res, _, _ := tb.Search(ctx, Query{Text: "loyer"}); len(res) != 1 {
		t.Fatal("the restored entry is not found by search")
	}
	if h, _ := tb.History(ctx, c.ID); len(h) != 1 || h[0].Summary != "Restored after it was deleted" {
		t.Fatalf("history of the restored entry: %+v", h)
	}
	if _, err := tb.Restore(ctx, c.ID, "Eva"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("restoring twice: %v", err)
	}
	if err := tb.DeleteEntry(ctx, c.ID, "Dan", ""); err != nil {
		t.Fatal(err)
	}
}

func TestLookupAndSearch(t *testing.T) {
	db, _ := open(t)
	tb := create(t, db)
	add := func(e term.Entry) {
		if _, err := tb.Add(ctx, e, "Dan", ""); err != nil {
			t.Fatal(err)
		}
	}
	add(ent("", lng("de-DE", approved("Betriebskosten")), lng("en-GB", approved("service charges"), forbidden("operating costs"))))
	add(term.Entry{Fields: []term.Value{{Field: "Definition", Value: "Geldbetrag zur Sicherung des Vermieters"}}, Languages: []term.Language{lng("de-DE", approved("Kaution")), lng("en-GB", approved("deposit"))}})
	add(ent("", lng("de-DE", approved("Mietsache")), lng("en-GB", term.Term{Text: "rented property", Status: term.StatusProposed})))
	add(ent("", lng("de-AT", approved("Mietzins")), lng("en-GB", approved("rent"))))

	if r, _ := tb.Lookup(ctx, "  BETRIEBSKOSTEN ", "de-DE"); len(r) != 1 {
		t.Fatalf("lookup ignores case and space: %v", r)
	}
	if r, _ := tb.Lookup(ctx, "Mietzins", "de-DE"); len(r) != 1 {
		t.Fatalf("lookup across variants: %v", r)
	}
	if r, _ := tb.Lookup(ctx, "deposit", ""); len(r) != 1 {
		t.Fatalf("lookup in any language: %v", r)
	}
	if r, _ := tb.Lookup(ctx, "Kaution", "en"); len(r) != 0 {
		t.Fatalf("lookup in the wrong language: %v", r)
	}

	cases := []struct {
		q    Query
		want int
	}{
		{Query{Text: "kosten"}, 1},
		{Query{Text: "SICHERUNG"}, 1},             // field values
		{Query{Text: "sicherung", Lang: "de"}, 0}, // with a language, only terms
		{Query{Text: "rent"}, 2},                  // rented property, rent
		{Query{Text: "rent", Lang: "de"}, 0},
		{Query{Text: "ie"}, 4}, // short text: substring match in Go
		{Query{Text: "au", Lang: "de"}, 1},
		{Query{Lang: "de-AT"}, 4},
		{Query{Status: "forbidden"}, 1},
		{Query{Status: "forbidden", Lang: "de"}, 0},
		{Query{Status: "proposed", Lang: "en"}, 1},
		{Query{Text: "„quoted\""}, 0},
	}
	for _, c := range cases {
		got, total, err := tb.Search(ctx, c.q)
		if err != nil {
			t.Fatalf("%+v: %v", c.q, err)
		}
		if total != c.want || len(got) != c.want {
			t.Errorf("%+v: %d (%d), want %d", c.q, total, len(got), c.want)
		}
	}
	page, total, _ := tb.Search(ctx, Query{Limit: 2, Offset: 1})
	if total != 4 || len(page) != 2 || page[0].ID != "c2" {
		t.Fatalf("paging: %d %+v", total, page)
	}
	if page, _, _ := tb.Search(ctx, Query{Offset: 10}); len(page) != 0 {
		t.Fatal("offset past the end")
	}
}

func TestDefinitionChanges(t *testing.T) {
	db, _ := open(t)
	tb := create(t, db)
	tb.Add(ctx, term.Entry{Fields: []term.Value{{Field: "Definition", Value: "x"}}, Languages: []term.Language{lng("de", approved("Kaution")), lng("en", approved("deposit"))}}, "Dan", "")
	tb.Add(ctx, ent("", lng("en", approved("tenant"))), "Dan", "")

	if err := tb.SetDefinition(ctx, term.Definition{Languages: []string{"de-DE"}}); !errors.Is(err, ErrInUse) {
		t.Fatalf("dropping a used language: %v", err)
	}
	if err := tb.SetDefinition(ctx, term.Definition{Languages: []string{"de-DE", "en-GB", "fr-FR"}, Fields: []term.Field{{Name: "Definition", Level: term.LevelEntry}, {Name: "Sachgebiet", Level: term.LevelEntry, Kind: term.KindPicklist, Values: []string{"Miete"}}}}); err != nil {
		t.Fatal(err)
	}
	def, _ := tb.Definition(ctx)
	if def.Name != "Kanzlei Weber" || len(def.Languages) != 3 || def.Fields[1].Values[0] != "Miete" {
		t.Fatalf("definition: %+v", def)
	}
	n, err := tb.RemoveLanguage(ctx, "en-GB", "Dan")
	if err != nil || n != 2 {
		t.Fatalf("remove language: %d %v", n, err)
	}
	if c, _, _ := tb.Count(ctx); c != 1 {
		t.Fatalf("entry left without terms should be deleted, %d entries", c)
	}
	h, _ := tb.History(ctx, "c1")
	if h[len(h)-1].Summary != "Removed the language en-GB" {
		t.Fatalf("history: %+v", h)
	}
	n, err = tb.RenameField(ctx, "definition", "Meaning", "Dan")
	if err != nil || n != 1 {
		t.Fatalf("rename field: %d %v", n, err)
	}
	if e, _ := tb.Get(ctx, "c1"); len(e.Fields) != 1 || e.Fields[0].Field != "Meaning" {
		t.Fatalf("renamed field in the entry: %+v", e.Fields)
	}
	if def, _ := tb.Definition(ctx); !hasField(def.Fields, "Meaning") || hasField(def.Fields, "definition") {
		t.Fatalf("renamed field in the definition: %+v", def.Fields)
	}
	if h, _ := tb.History(ctx, "c1"); !strings.Contains(h[len(h)-1].Summary, "Renamed the field") {
		t.Fatalf("history after renaming: %+v", h[len(h)-1])
	}
	if _, err := tb.RenameField(ctx, "Meaning", " ", "Dan"); err == nil {
		t.Fatal("a field was renamed to nothing")
	}
	if _, err := tb.RenameField(ctx, "nothing", "Other", "Dan"); err == nil {
		t.Fatal("a field that is not there was renamed")
	}
	if _, err := tb.RenameField(ctx, "meaning", "definition", "Dan"); err != nil {
		t.Fatalf("rename back: %v", err)
	}
	n, err = tb.RemoveField(ctx, "definition", "Dan")
	if err != nil || n != 1 {
		t.Fatalf("remove field: %d %v", n, err)
	}
	e, _ := tb.Get(ctx, "c1")
	def, _ = tb.Definition(ctx)
	if len(e.Fields) != 0 || len(def.Fields) != 1 || reflect.DeepEqual(def.Languages, []string{"de-DE", "en-GB", "fr-FR"}) {
		t.Fatalf("after removals: %+v %+v", e, def)
	}
}

func TestImportExportAndMerge(t *testing.T) {
	db, _ := open(t)
	tb := create(t, db)
	tb.Add(ctx, ent("", lng("de-DE", approved("Kaution")), lng("en-GB", approved("deposit"))), "Dan", "")
	tb.Add(ctx, ent("", lng("de-DE", approved("Miete")), lng("en-GB", approved("rent"))), "Dan", "")

	file := &term.Termbase{
		Definition: term.Definition{Languages: []string{"de", "en", "fr"}, Fields: []term.Field{{Name: "Quelle", Level: term.LevelEntry, Kind: term.KindPicklist, Values: []string{"Mandant"}}}},
		Entries: []term.Entry{
			{ID: "K-1", Fields: []term.Value{{Field: "Quelle", Value: "Mandant"}}, Languages: []term.Language{lng("de", approved("Kaution")), lng("en", approved("security deposit"))}},
			ent("K-2", lng("de", approved("Kaution")), lng("fr", approved("caution"))),
			ent("K-3", lng("de", approved("Untermiete")), lng("en", approved("subletting"))),
			ent("K-4", lng("de", approved(" "))),
		},
	}
	actions, prep, err := tb.Plan(ctx, file, term.Policy{Match: term.MatchTerm, OnMatch: term.Merge})
	if err != nil || prep.Merged != 2 || prep.Added != 1 || prep.Invalid != 1 || len(actions) != 3 {
		t.Fatalf("plan: %+v %+v %v", prep, actions, err)
	}
	if n, _, _ := tb.Count(ctx); n != 2 {
		t.Fatal("plan wrote to the database")
	}
	rep, err := tb.Import(ctx, file, term.Policy{Match: term.MatchTerm, OnMatch: term.Merge}, "Dan", "glossar.tbx")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rep.Languages, []string{"fr"}) || !reflect.DeepEqual(rep.Fields, []string{"Quelle"}) {
		t.Fatalf("report: %+v", rep)
	}
	k, _ := tb.Get(ctx, "c1")
	if !reflect.DeepEqual(terms(k.Entry, "en-GB"), []string{"deposit", "security deposit"}) || !reflect.DeepEqual(terms(k.Entry, "fr"), []string{"caution"}) || k.Version != 2 {
		t.Fatalf("merged twice into one entry, one version: %+v", k)
	}
	h, _ := tb.History(ctx, "c1")
	if h[1].Summary != "Merged with an entry imported from glossar.tbx" {
		t.Fatalf("history: %q", h[1].Summary)
	}
	if s, err := tb.Get(ctx, "K-3"); err != nil || terms(s.Entry, "de-DE")[0] != "Untermiete" {
		t.Fatalf("added with the file's ID: %+v %v", s, err)
	}
	def, _ := tb.Definition(ctx)
	if def.Fields[1].Kind != term.KindPicklist {
		t.Fatalf("declared field kind kept: %+v", def.Fields)
	}

	// Export and import back by ID with overwrite: nothing changes but versions.
	out, err := tb.Export(ctx)
	if err != nil || len(out.Entries) != 3 {
		t.Fatalf("export: %v", err)
	}
	data, err := term.Write(out, term.FormatTBX)
	if err != nil {
		t.Fatal(err)
	}
	back, _, err := term.Read(term.FormatTBX, data, nil)
	if err != nil {
		t.Fatal(err)
	}
	back.Entries[2].Languages[1].Terms[0].Text = "sublease"
	rep, err = tb.Import(ctx, back, term.Policy{Match: term.MatchID, OnMatch: term.Overwrite}, "Dan", "")
	if err != nil || rep.Overwritten != 3 || rep.Added != 0 {
		t.Fatalf("reimport: %+v %v", rep, err)
	}
	s, _ := tb.Get(ctx, "K-3")
	if terms(s.Entry, "en-GB")[0] != "sublease" {
		t.Fatalf("overwrite: %+v", s)
	}

	// Duplicates and merge.
	tb.Add(ctx, ent("", lng("de-DE", approved("Miete")), lng("en-GB", approved("rental payment"))), "Eva", "")
	groups, err := tb.Duplicates(ctx, "de")
	if err != nil || len(groups) != 1 || !reflect.DeepEqual(groups[0], []string{"c2", "c3"}) {
		t.Fatalf("duplicates: %v %v", groups, err)
	}
	m, err := tb.Merge(ctx, "c2", []string{"c3", "c2"}, "Dan")
	if err != nil || !reflect.DeepEqual(terms(m.Entry, "en-GB"), []string{"rent", "rental payment"}) {
		t.Fatalf("merge: %+v %v", m, err)
	}
	if _, err := tb.Get(ctx, "c3"); !errors.Is(err, ErrNotFound) {
		t.Fatal("merged entry not deleted")
	}
	if _, err := tb.Merge(ctx, "c2", []string{"404"}, "Dan"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("merge with a missing entry: %v", err)
	}
	if groups, _ := tb.Duplicates(ctx, ""); len(groups) != 0 {
		t.Fatalf("no duplicates left: %v", groups)
	}

	// Recognition from the store.
	x := term.NewIndex()
	if err := tb.AddTo(ctx, x); err != nil {
		t.Fatal(err)
	}
	hits := x.Recognize("Die Kautionen und die Miete", "de-DE", "en-GB")
	if len(hits) != 2 || hits[0].Source != "Kanzlei Weber" || hits[0].Targets[0].Text != "deposit" {
		t.Fatalf("recognize: %+v", hits)
	}
}

func TestConcurrentWriters(t *testing.T) {
	db, path := open(t)
	tb := create(t, db)
	db2 := reopen(t, path)
	defer db2.Close()
	tb2, _ := db2.Termbase(ctx, "Kanzlei Weber")
	var wg sync.WaitGroup
	errs := make(chan error, 40)
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			_, err := tb.Add(ctx, ent("", lng("de", approved("a"+string(rune('a'+i))))), "A", "")
			errs <- err
		}(i)
		go func(i int) {
			defer wg.Done()
			_, err := tb2.Add(ctx, ent("", lng("de", approved("b"+string(rune('a'+i))))), "B", "")
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if n, _, _ := tb.Count(ctx); n != 40 {
		t.Fatalf("%d entries", n)
	}
}
