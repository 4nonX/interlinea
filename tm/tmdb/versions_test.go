package tmdb

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/internal/sqlitedb"
	"github.com/4nonX/interlinea/tm"
)

func unit(src, tgt string) tm.Unit {
	return tm.Unit{Source: content.Text(src), Target: content.Text(tgt)}
}

func TestUnitVersions(t *testing.T) {
	db, _ := openTemp(t)
	m, err := db.Create(ctx, tm.Info{Name: "v", SourceLang: "de", TargetLang: "en"})
	if err != nil {
		t.Fatal(err)
	}
	u, err := m.Add(ctx, unit("Die Frist beträgt zwei Wochen.", "The period is two weeks."), tm.KeepBoth)
	if err != nil {
		t.Fatal(err)
	}
	if u.Version != 1 || u.ID == 0 {
		t.Fatalf("new unit = id %d, version %d", u.ID, u.Version)
	}
	// Saving the same pair again updates the unit and counts a revision.
	again, err := m.Add(ctx, unit("Die Frist beträgt zwei Wochen.", "The period is two weeks."), tm.KeepBoth)
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != u.ID || again.Version != 2 {
		t.Fatalf("re-added unit = id %d, version %d", again.ID, again.Version)
	}

	edited := again
	edited.Target = content.Text("The deadline is two weeks.")
	edited.Notes = []string{"checked against the contract"}
	got, err := m.Update(ctx, edited, 2, "anna")
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != 3 || got.ChangedBy != "anna" || got.Target.Plain() != "The deadline is two weeks." {
		t.Fatalf("updated = %+v", got)
	}
	stored, err := m.Unit(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Version != 3 || len(stored.Notes) != 1 || !stored.CreatedAt.Equal(u.CreatedAt) {
		t.Fatalf("stored = %+v", stored)
	}

	// An edit made on version 2 comes too late.
	stale := edited
	stale.Target = content.Text("Two weeks.")
	if _, err := m.Update(ctx, stale, 2, "ben"); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale update: %v", err)
	}
	if s, _ := m.Unit(ctx, u.ID); s.Target.Plain() != "The deadline is two weeks." {
		t.Fatalf("stale update was written: %q", s.Target.Plain())
	}

	// The edited source is what lookups and concordance find.
	src := got
	src.Source = content.Text("Die Kündigungsfrist beträgt zwei Wochen.")
	if _, err := m.Update(ctx, src, 3, "anna"); err != nil {
		t.Fatal(err)
	}
	ms, err := m.Lookup(ctx, content.Text("Die Kündigungsfrist beträgt zwei Wochen."), tm.LookupOptions{})
	if err != nil || len(ms) != 1 || ms[0].Score != 100 || ms[0].Unit.Version != 4 {
		t.Fatalf("lookup after source edit = %+v, %v", ms, err)
	}
	if c, _ := m.Concordance(ctx, "kündigungsfrist", tm.ConcordanceOptions{}); len(c) != 1 {
		t.Fatalf("concordance after source edit = %v", c)
	}
	if _, err := m.Update(ctx, tm.Unit{ID: u.ID, Source: content.Text(" "), Target: content.Text("x")}, 4, "anna"); err == nil {
		t.Fatal("empty source accepted")
	}

	if err := m.DeleteUnit(ctx, u.ID, "anna"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Unit(ctx, u.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted unit: %v", err)
	}
	if err := m.DeleteUnit(ctx, u.ID, "anna"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete: %v", err)
	}
	if _, err := m.Update(ctx, got, 3, "anna"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("update of deleted unit: %v", err)
	}

	// A unit of another memory is not reachable through this one.
	other, _ := db.Create(ctx, tm.Info{Name: "other", SourceLang: "de", TargetLang: "en"})
	ou, _ := other.Add(ctx, unit("Anderer Satz.", "Other sentence."), tm.KeepBoth)
	if _, err := m.Unit(ctx, ou.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unit of another memory: %v", err)
	}
	if err := m.DeleteUnit(ctx, ou.ID, "anna"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete through another memory: %v", err)
	}
}

func TestConcurrentEditsOneWins(t *testing.T) {
	db, key := openTemp(t)
	m, _ := db.Create(ctx, tm.Info{Name: "c", SourceLang: "de", TargetLang: "en"})
	u, _ := m.Add(ctx, unit("Satz.", "Sentence."), tm.KeepBoth)
	// Two connections, as two translators' apps would have.
	m2, err := reopen(t, key).Memory(ctx, "c")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make([]error, 8)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			mm := m
			if i%2 == 1 {
				mm = m2
			}
			e := u
			e.Target = content.Text("Version " + string(rune('A'+i)))
			_, errs[i] = mm.Update(ctx, e, 1, "t")
		}(i)
	}
	wg.Wait()
	ok := 0
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, ErrConflict):
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if ok != 1 {
		t.Fatalf("%d edits of version 1 succeeded", ok)
	}
	if s, _ := m.Unit(ctx, u.ID); s.Version != 2 {
		t.Fatalf("version = %d", s.Version)
	}
}

func TestChangeLog(t *testing.T) {
	db, _ := openTemp(t)
	if seq, err := db.LastChange(ctx); err != nil || seq != 0 {
		t.Fatalf("empty log: %d %v", seq, err)
	}
	a, _ := db.CreateBy(ctx, tm.Info{Name: "Recht", SourceLang: "de", TargetLang: "en"}, "anna")
	b, _ := db.CreateBy(ctx, tm.Info{Name: "Technik", SourceLang: "de", TargetLang: "en"}, "ben")
	u1, _ := a.Add(ctx, tm.Unit{Source: content.Text("Eins."), Target: content.Text("One."), ChangedBy: "anna"}, tm.KeepBoth)
	if _, err := b.Add(ctx, tm.Unit{Source: content.Text("Zwei."), Target: content.Text("Two."), ChangedBy: "ben"}, tm.KeepBoth); err != nil {
		t.Fatal(err)
	}
	e := u1
	e.Target = content.Text("One!")
	if _, err := a.Update(ctx, e, 1, "carl"); err != nil {
		t.Fatal(err)
	}
	if err := a.DeleteUnit(ctx, u1.ID, "anna"); err != nil {
		t.Fatal(err)
	}
	if err := b.AddBatch(ctx, []tm.Unit{unit("Drei.", "Three."), unit("Vier.", "Four.")}, tm.KeepBoth); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteBy(ctx, "Technik", "ben"); err != nil {
		t.Fatal(err)
	}

	all, more, err := db.Changes(ctx, ChangeQuery{})
	if err != nil || more {
		t.Fatalf("changes: %v %v", more, err)
	}
	want := []struct{ memory, kind, by string }{
		{"Recht", ChangeMemoryCreated, "anna"},
		{"Technik", ChangeMemoryCreated, "ben"},
		{"Recht", ChangeCreated, "anna"},
		{"Technik", ChangeCreated, "ben"},
		{"Recht", ChangeChanged, "carl"},
		{"Recht", ChangeDeleted, "anna"},
		{"Technik", ChangeCreated, ""},
		{"Technik", ChangeCreated, ""},
		{"Technik", ChangeMemoryDeleted, "ben"},
	}
	if len(all) != len(want) {
		t.Fatalf("got %d changes: %+v", len(all), all)
	}
	for i, w := range want {
		c := all[i]
		if c.Memory != w.memory || c.Kind != w.kind || c.By != w.by {
			t.Errorf("change %d = %+v, want %+v", i, c, w)
		}
		if i > 0 && c.Seq <= all[i-1].Seq {
			t.Errorf("seq not increasing at %d", i)
		}
		if c.At.IsZero() {
			t.Errorf("change %d has no time", i)
		}
	}
	if all[4].UnitID != u1.ID || all[4].Version != 2 || all[5].Version != 2 {
		t.Errorf("unit change = %+v / %+v", all[4], all[5])
	}
	last, _ := db.LastChange(ctx)
	if last != all[len(all)-1].Seq {
		t.Errorf("last = %d", last)
	}

	// Paging.
	page, more, err := db.Changes(ctx, ChangeQuery{Since: all[1].Seq, Limit: 3})
	if err != nil || !more || len(page) != 3 || page[0].Seq != all[2].Seq {
		t.Fatalf("page = %+v %v %v", page, more, err)
	}
	// By memory, including one that no longer exists (case ignored).
	tech, _, err := db.Changes(ctx, ChangeQuery{Memories: []string{"technik"}})
	if err != nil || len(tech) != 5 {
		t.Fatalf("technik changes = %+v %v", tech, err)
	}
	for _, c := range tech {
		if c.Memory != "Technik" {
			t.Fatalf("other memory's change: %+v", c)
		}
	}
	if none, _, _ := db.Changes(ctx, ChangeQuery{Since: last}); len(none) != 0 {
		t.Fatalf("changes after last = %v", none)
	}
}

func TestMigrateFromVersion1(t *testing.T) {
	sqliteOnly(t)
	path := filepath.Join(t.TempDir(), "old.db")
	old, err := sqlitedb.Open(path, "tmdb", migrations[:1])
	if err != nil {
		t.Fatal(err)
	}
	created := time.Date(2025, 3, 1, 8, 0, 0, 0, time.UTC).Format(timeLayout)
	src, _ := json.Marshal(content.Text("Alte Einheit."))
	tgt, _ := json.Marshal(content.Text("Old unit."))
	if _, err := old.Exec(`INSERT INTO memories(name, source_lang, target_lang, created_at) VALUES ('Alt', 'de', 'en', ?)`, created); err != nil {
		t.Fatal(err)
	}
	if _, err := old.Exec(`INSERT INTO units(memory_id, source_key, source_plain, source_json, target_plain, target_json, created_at, created_by, changed_at)
		VALUES (1, ?, 'Alte Einheit.', ?, 'Old unit.', ?, ?, 'dora', ?)`,
		tm.SourceKey(content.Text("Alte Einheit.")), string(src), string(tgt), created, created); err != nil {
		t.Fatal(err)
	}
	old.Close()

	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	m, err := db.Memory(ctx, "alt")
	if err != nil {
		t.Fatal(err)
	}
	us, err := m.Lookup(ctx, content.Text("Alte Einheit."), tm.LookupOptions{})
	if err != nil || len(us) != 1 || us[0].Unit.Version != 1 {
		t.Fatalf("migrated unit = %+v %v", us, err)
	}
	cs, _, err := db.Changes(ctx, ChangeQuery{})
	if err != nil || len(cs) != 2 {
		t.Fatalf("backfilled log = %+v %v", cs, err)
	}
	if cs[0].Kind != ChangeMemoryCreated || cs[1].Kind != ChangeCreated || cs[1].By != "dora" || cs[1].UnitID != us[0].Unit.ID || !cs[1].At.Equal(parseTime(created)) {
		t.Fatalf("backfilled log = %+v", cs)
	}
	// Editing a migrated unit works with its version.
	e := us[0].Unit
	e.Target = content.Text("Old unit, edited.")
	if _, err := m.Update(ctx, e, 1, "dora"); err != nil {
		t.Fatal(err)
	}
}
