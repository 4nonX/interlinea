package projectdb

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/project"
	"github.com/4nonX/interlinea/tm"
)

var ctx = context.Background()

func openTemp(t testing.TB) (*DB, string) {
	t.Helper()
	var db *DB
	var key string
	if pgDSN() != "" {
		db, key = openPG(t)
	} else {
		key = filepath.Join(t.TempDir(), "projects.db")
		var err error
		if db, err = Open(key); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
	}
	clock := time.Date(2026, 11, 23, 9, 0, 0, 0, time.UTC)
	var mu sync.Mutex
	db.now = func() time.Time { mu.Lock(); defer mu.Unlock(); clock = clock.Add(time.Second); return clock }
	return db, key
}

func reopen(t testing.TB, key string) *DB {
	t.Helper()
	if isPG(key) {
		return reopenPG(t, key)
	}
	db, err := Open(key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func sqliteOnly(t *testing.T) {
	t.Helper()
	if pgDSN() != "" {
		t.Skip("tests the SQLite file")
	}
}

var cat = project.Catalog{
	Memories: map[string]tm.Info{
		"Weber DE-EN": {Name: "Weber DE-EN", SourceLang: "de-DE", TargetLang: "en-GB"},
		"Weber DE-FR": {Name: "Weber DE-FR", SourceLang: "de-DE", TargetLang: "fr-FR"},
	},
	Termbases: map[string][]string{"tb-weber": {"de-DE", "en-GB", "fr-FR"}},
}

func settings(name string) project.Settings {
	s := project.Defaults()
	s.Name = name
	s.Client = "Kanzlei Weber"
	s.SourceLang = "de_de"
	s.TargetLangs = []string{"en-gb", "fr-FR"}
	s.Memories = map[string][]project.MemorySetting{
		"en-GB": {{Memory: "Weber DE-EN", Lookup: true, Update: true, Penalty: 2}},
	}
	s.Terminology = project.Terminology{Mode: project.Layered, Masters: []string{"tb-weber"}, Location: project.Server, AutoUpdate: true}
	return s
}

func segs(texts ...string) []project.Segment {
	out := make([]project.Segment, len(texts))
	for i, s := range texts {
		out[i] = project.Segment{ID: fmt.Sprintf("s%d", i+1), Source: content.Text(s), Status: project.StatusNew}
	}
	return out
}

func files() []NewFile {
	lease := segs("Mietvertrag.", "Der Mieter zahlt die Miete.", "Die Kaution beträgt drei Monatsmieten.")
	lease[2].Locked = true
	house := segs("Hausordnung.")
	return []NewFile{
		{File: project.File{Name: "Mietvertrag.docx", Role: project.Translatable}, Format: "docx", Data: []byte("PK lease"),
			Segments: map[string][]project.Segment{"en-GB": lease, "fr-FR": lease}},
		{File: project.File{Name: "Anlage/Hausordnung.docx", Role: project.Translatable}, Format: "docx", Data: []byte("PK house"),
			Segments: map[string][]project.Segment{"en-GB": house, "fr-FR": house}},
		{File: project.File{Name: "Styleguide.pdf", Role: project.Reference}, Format: "pdf", Data: []byte("%PDF")},
		{File: project.File{Name: "return.sdlxliff", Role: project.Translatable, TargetLang: "en-gb"}, Format: "sdlxliff", Data: []byte("<xliff/>"),
			Segments: map[string][]project.Segment{"en-GB": segs("Anhang.")}},
	}
}

func create(t *testing.T, db *DB, name string) Project {
	t.Helper()
	p, err := db.Create(ctx, settings(name), files(), cat, "anna")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCreateAndRead(t *testing.T) {
	db, key := openTemp(t)
	p := create(t, db, "Mietvertrag Hofmann")
	if p.ID == 0 || p.Version != 1 || p.CreatedBy != "anna" {
		t.Fatalf("created %+v", p)
	}
	// Languages are stored in canonical form; the file list comes from the files.
	s := p.Settings
	if s.SourceLang != "de-DE" || s.TargetLangs[0] != "en-GB" || len(s.Files) != 4 || s.Files[3].TargetLang != "en-GB" {
		t.Fatalf("settings %+v", s)
	}
	db2 := reopen(t, key)
	got, err := db2.Project(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Settings.Name != "Mietvertrag Hofmann" || got.Settings.Memories["en-GB"][0].Penalty != 2 || !got.Settings.Terminology.AutoUpdate {
		t.Fatalf("reopened %+v", got.Settings)
	}
	byName, err := db2.ProjectByName(ctx, "mietvertrag hofmann")
	if err != nil || byName.ID != p.ID {
		t.Fatalf("by name %+v %v", byName, err)
	}
	f, err := db2.File(ctx, p.ID, "anlage/hausordnung.DOCX")
	if err != nil || string(f.Data) != "PK house" || f.Format != "docx" || f.Role != project.Translatable {
		t.Fatalf("file %+v %v", f, err)
	}
	ref, err := db2.File(ctx, p.ID, "Styleguide.pdf")
	if err != nil || ref.Role != project.Reference || string(ref.Data) != "%PDF" {
		t.Fatalf("reference %+v %v", ref, err)
	}
	en, err := db2.Segments(ctx, p.ID, "Mietvertrag.docx", "en-gb")
	if err != nil || len(en) != 3 || en[1].Source.Plain() != "Der Mieter zahlt die Miete." || !en[2].Locked || en[0].Version != 1 {
		t.Fatalf("segments %+v %v", en, err)
	}
	bi, _ := db2.Segments(ctx, p.ID, "return.sdlxliff", "en-GB")
	none, _ := db2.Segments(ctx, p.ID, "return.sdlxliff", "fr-FR")
	if len(bi) != 1 || len(none) != 0 {
		t.Fatalf("bilingual file: %d en, %d fr", len(bi), len(none))
	}
	if _, err := db2.Project(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing project: %v", err)
	}
	if _, err := db2.File(ctx, p.ID, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing file: %v", err)
	}
	if _, err := db2.Segments(ctx, p.ID, "nope", "en-GB"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing file segments: %v", err)
	}
}

func TestCreateRefusesBadInput(t *testing.T) {
	db, _ := openTemp(t)
	create(t, db, "Eins")
	if _, err := db.Create(ctx, settings("EINS"), files(), cat, "anna"); !errors.Is(err, ErrExists) {
		t.Fatalf("duplicate name: %v", err)
	}
	var inv *InvalidError
	bad := settings("Zwei")
	bad.TargetLangs = nil
	if _, err := db.Create(ctx, bad, files(), cat, "anna"); !errors.As(err, &inv) || len(inv.Problems) == 0 || !strings.Contains(err.Error(), "targetLangs") {
		t.Fatalf("invalid settings: %v", err)
	}
	cases := map[string]func([]NewFile){
		"missing target":          func(f []NewFile) { delete(f[0].Segments, "fr-FR") },
		"extra target":            func(f []NewFile) { f[3].Segments["fr-FR"] = segs("x") },
		"reference with segments": func(f []NewFile) { f[2].Segments = map[string][]project.Segment{"en-GB": segs("x")} },
		"duplicate segment":       func(f []NewFile) { f[1].Segments["en-GB"] = append(segs("a"), segs("b")...) },
		"segment without ID": func(f []NewFile) {
			f[1].Segments["en-GB"] = []project.Segment{{Source: content.Text("a"), Status: project.StatusNew}}
		},
		"unknown status": func(f []NewFile) {
			f[1].Segments["en-GB"] = []project.Segment{{ID: "a", Source: content.Text("a"), Status: "done"}}
		},
	}
	for name, change := range cases {
		fs := files()
		change(fs)
		if _, err := db.Create(ctx, settings("Zwei "+name), fs, cat, "anna"); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	list, _ := db.List(ctx)
	if len(list) != 1 {
		t.Fatalf("refused projects were stored: %d", len(list))
	}
}

func TestSegmentsVersionsAndConflicts(t *testing.T) {
	db, key := openTemp(t)
	p := create(t, db, "P")
	sg, err := db.SaveSegment(ctx, p.ID, "Mietvertrag.docx", "en-GB", "s2", content.Text("The tenant pays the rent."), project.StatusDraft, 1, "anna")
	if err != nil {
		t.Fatal(err)
	}
	if sg.Version != 2 || sg.Status != project.StatusDraft || sg.ChangedBy != "anna" || sg.Target.Plain() != "The tenant pays the rent." {
		t.Fatalf("saved %+v", sg)
	}
	// A second translator saving from version 1 comes too late.
	db2 := reopen(t, key)
	if _, err := db2.SaveSegment(ctx, p.ID, "Mietvertrag.docx", "en-GB", "s2", content.Text("Rent."), project.StatusConfirmed, 1, "ben"); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale save: %v", err)
	}
	if _, err := db.SaveSegment(ctx, p.ID, "Mietvertrag.docx", "en-GB", "s3", content.Text("x"), project.StatusDraft, 1, "anna"); !errors.Is(err, ErrLocked) {
		t.Fatalf("locked segment: %v", err)
	}
	if _, err := db.SaveSegment(ctx, p.ID, "Mietvertrag.docx", "en-GB", "s9", content.Text("x"), project.StatusDraft, 1, "anna"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing segment: %v", err)
	}
	if _, err := db.SaveSegment(ctx, p.ID, "Mietvertrag.docx", "en-GB", "s2", content.Text("x"), "done", 2, "anna"); err == nil {
		t.Fatal("unknown status accepted")
	}
	// The French copy of the same file is separate.
	fr, _ := db.Segments(ctx, p.ID, "Mietvertrag.docx", "fr-FR")
	if fr[1].Version != 1 || fr[1].Target.Plain() != "" {
		t.Fatalf("fr = %+v", fr[1])
	}

	// Concurrent saves of one version: exactly one wins.
	var wg sync.WaitGroup
	errs := make([]error, 6)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			d := db
			if i%2 == 1 {
				d = db2
			}
			_, errs[i] = d.SaveSegment(ctx, p.ID, "Mietvertrag.docx", "en-GB", "s1", content.Text(fmt.Sprintf("Lease %d.", i)), project.StatusConfirmed, 1, "t")
		}(i)
	}
	wg.Wait()
	ok := 0
	for _, err := range errs {
		if err == nil {
			ok++
		} else if !errors.Is(err, ErrConflict) {
			t.Fatalf("unexpected: %v", err)
		}
	}
	if ok != 1 {
		t.Fatalf("%d saves of version 1 won", ok)
	}

	list, err := db.List(ctx)
	if err != nil || len(list) != 1 || list[0].Segments != 7 || list[0].Confirmed != 1 {
		t.Fatalf("list %+v %v", list, err)
	}
}

func TestApplyTranslations(t *testing.T) {
	db, _ := openTemp(t)
	p := create(t, db, "P")
	if _, err := db.SaveSegment(ctx, p.ID, "Mietvertrag.docx", "en-GB", "s1", content.Text("Lease."), project.StatusDraft, 1, "anna"); err != nil {
		t.Fatal(err)
	}
	n, err := db.ApplyTranslations(ctx, p.ID, "en-gb", []project.Translation{
		{File: "Mietvertrag.docx", ID: "s1", Target: content.Text("Tenancy agreement."), Status: project.StatusDraft}, // translated meanwhile
		{File: "Mietvertrag.docx", ID: "s2", Target: content.Text("The tenant pays the rent."), Status: project.StatusConfirmed},
		{File: "Mietvertrag.docx", ID: "s3", Target: content.Text("Locked."), Status: project.StatusDraft}, // locked
		{File: "gone.docx", ID: "s1", Target: content.Text("x"), Status: project.StatusDraft},              // removed file
	}, "pretranslation")
	if err != nil || n != 1 {
		t.Fatalf("applied %d, %v", n, err)
	}
	en, _ := db.Segments(ctx, p.ID, "Mietvertrag.docx", "en-GB")
	if en[0].Target.Plain() != "Lease." || en[1].Status != project.StatusConfirmed || en[1].Version != 2 || en[1].ChangedBy != "pretranslation" || en[2].Target.Plain() != "" {
		t.Fatalf("segments %+v", en)
	}
	if _, err := db.ApplyTranslations(ctx, p.ID, "en-GB", []project.Translation{{File: "Mietvertrag.docx", ID: "s2", Status: "new"}}, "x"); err == nil {
		t.Fatal("status new accepted")
	}
}

func TestUpdateSettings(t *testing.T) {
	db, _ := openTemp(t)
	p := create(t, db, "P")
	create(t, db, "Other")
	if _, err := db.SaveSegment(ctx, p.ID, "Mietvertrag.docx", "en-GB", "s2", content.Text("The tenant pays the rent."), project.StatusConfirmed, 1, "anna"); err != nil {
		t.Fatal(err)
	}
	s := p.Settings
	s.Due = "2026-12-04T16:00:00Z"
	s.TargetLangs = []string{"en-GB", "it-IT"} // French goes, Italian comes
	s.Memories["fr-FR"] = nil
	delete(s.Memories, "fr-FR")
	s.Memories["en-GB"][0].Penalty = 7
	got, err := db.UpdateSettings(ctx, p.ID, s, 1, cat, "ben")
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != 2 || got.ChangedBy != "ben" || got.Settings.Memories["en-GB"][0].Penalty != 7 || got.Settings.Due == "" {
		t.Fatalf("updated %+v", got)
	}
	it, _ := db.Segments(ctx, p.ID, "Mietvertrag.docx", "it-IT")
	if len(it) != 3 || it[1].Source.Plain() != "Der Mieter zahlt die Miete." || it[1].Target.Plain() != "" || it[1].Status != project.StatusNew || !it[2].Locked {
		t.Fatalf("italian copy %+v", it)
	}
	if fr, _ := db.Segments(ctx, p.ID, "Mietvertrag.docx", "fr-FR"); len(fr) != 0 {
		t.Fatalf("french segments left: %d", len(fr))
	}
	if bi, _ := db.Segments(ctx, p.ID, "return.sdlxliff", "it-IT"); len(bi) != 0 {
		t.Fatal("a bilingual file got another language")
	}
	en, _ := db.Segments(ctx, p.ID, "Mietvertrag.docx", "en-GB")
	if en[1].Target.Plain() != "The tenant pays the rent." {
		t.Fatal("English translations were touched")
	}

	if _, err := db.UpdateSettings(ctx, p.ID, s, 1, cat, "carl"); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale settings: %v", err)
	}
	renamed := got.Settings
	renamed.Name = "other"
	if _, err := db.UpdateSettings(ctx, p.ID, renamed, 2, cat, "ben"); !errors.Is(err, ErrExists) {
		t.Fatalf("rename onto another project: %v", err)
	}
	src := got.Settings
	src.SourceLang = "de-AT"
	if _, err := db.UpdateSettings(ctx, p.ID, src, 2, cat, "ben"); err == nil {
		t.Fatal("source language changed")
	}
	fs := got.Settings
	fs.Files = fs.Files[:2]
	if _, err := db.UpdateSettings(ctx, p.ID, fs, 2, cat, "ben"); err == nil || !strings.Contains(err.Error(), "AddFile") {
		t.Fatalf("file list changed: %v", err)
	}
	bad := got.Settings
	bad.Memories["en-GB"][0].Penalty = 200
	var inv *InvalidError
	if _, err := db.UpdateSettings(ctx, p.ID, bad, 2, cat, "ben"); !errors.As(err, &inv) {
		t.Fatalf("invalid settings: %v", err)
	}
	if cur, _ := db.Project(ctx, p.ID); cur.Version != 2 {
		t.Fatalf("version after refused updates = %d", cur.Version)
	}
}

func TestAddAndRemoveFiles(t *testing.T) {
	db, _ := openTemp(t)
	p := create(t, db, "P")
	nf := NewFile{File: project.File{Name: "Nachtrag.docx", Role: project.Translatable}, Format: "docx", Data: []byte("PK"),
		Segments: map[string][]project.Segment{"en-GB": segs("Nachtrag."), "fr-FR": segs("Nachtrag.")}}
	got, err := db.AddFile(ctx, p.ID, nf, 1, cat, "anna")
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != 2 || len(got.Settings.Files) != 5 || got.Settings.Files[4].Name != "Nachtrag.docx" {
		t.Fatalf("added %+v", got.Settings.Files)
	}
	if _, err := db.AddFile(ctx, p.ID, nf, 2, cat, "anna"); err == nil {
		t.Fatal("same file added twice")
	}
	if _, err := db.AddFile(ctx, p.ID, NewFile{File: project.File{Name: "x.docx", Role: project.Translatable}, Segments: map[string][]project.Segment{"en-GB": segs("a")}}, 2, cat, "anna"); err == nil {
		t.Fatal("file without French segments accepted")
	}
	if _, err := db.AddFile(ctx, p.ID, nf, 1, cat, "anna"); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale add: %v", err)
	}
	got, err = db.RemoveFile(ctx, p.ID, "mietvertrag.docx", 2, cat, "anna")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Settings.Files) != 4 || got.Settings.Files[0].Name != "Anlage/Hausordnung.docx" {
		t.Fatalf("after removal %+v", got.Settings.Files)
	}
	if _, err := db.Segments(ctx, p.ID, "Mietvertrag.docx", "en-GB"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("segments of a removed file: %v", err)
	}
	if _, err := db.RemoveFile(ctx, p.ID, "nope", 3, cat, "anna"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("remove missing: %v", err)
	}
	// The last files to translate can't all go.
	for _, n := range []string{"Anlage/Hausordnung.docx", "return.sdlxliff"} {
		cur, _ := db.Project(ctx, p.ID)
		if _, err := db.RemoveFile(ctx, p.ID, n, cur.Version, cat, "anna"); err != nil {
			t.Fatal(err)
		}
	}
	cur, _ := db.Project(ctx, p.ID)
	var inv *InvalidError
	if _, err := db.RemoveFile(ctx, p.ID, "Nachtrag.docx", cur.Version, cat, "anna"); !errors.As(err, &inv) {
		t.Fatalf("removing the last translatable file: %v", err)
	}
}

func TestTemplates(t *testing.T) {
	db, _ := openTemp(t)
	tpl := project.TemplateFrom("Weber Mietrecht", settings("x"))
	st, err := db.SaveTemplate(ctx, tpl, 0, cat, "anna")
	if err != nil || st.Version != 1 {
		t.Fatalf("saved %+v %v", st, err)
	}
	if _, err := db.SaveTemplate(ctx, tpl, 0, cat, "ben"); !errors.Is(err, ErrConflict) {
		t.Fatalf("second new template of the same name: %v", err)
	}
	tpl.Prepare = project.Prepare{Pretranslate: true, MinScore: 95}
	if st, err = db.SaveTemplate(ctx, tpl, 1, cat, "anna"); err != nil || st.Version != 2 {
		t.Fatalf("update %+v %v", st, err)
	}
	if _, err := db.SaveTemplate(ctx, tpl, 1, cat, "ben"); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale template: %v", err)
	}
	if _, err := db.SaveTemplate(ctx, project.Template{Name: "Fresh"}, 3, cat, "ben"); err == nil {
		t.Fatal("invalid template accepted")
	}
	other := project.TemplateFrom("Allgemein", settings("x"))
	if _, err := db.SaveTemplate(ctx, other, 5, cat, "ben"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("update of a template that doesn't exist: %v", err)
	}
	if _, err := db.SaveTemplate(ctx, other, 0, cat, "ben"); err != nil {
		t.Fatal(err)
	}
	list, err := db.Templates(ctx)
	if err != nil || len(list) != 2 || list[0].Name != "Allgemein" || list[1].Prepare.MinScore != 95 {
		t.Fatalf("templates %+v %v", list, err)
	}
	s := list[1].Settings()
	s.Name = "From template"
	if _, err := db.Create(ctx, s, files(), cat, "anna"); err != nil {
		t.Fatalf("project from template: %v", err)
	}
	if err := db.DeleteTemplate(ctx, "allgemein", "ben"); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteTemplate(ctx, "allgemein", "ben"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete: %v", err)
	}
}

func TestChangeLogAndDelete(t *testing.T) {
	db, _ := openTemp(t)
	p := create(t, db, "P")
	q := create(t, db, "Q")
	if _, err := db.SaveSegment(ctx, p.ID, "Mietvertrag.docx", "fr-FR", "s1", content.Text("Bail."), project.StatusConfirmed, 1, "carl"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SaveTemplate(ctx, project.TemplateFrom("T", settings("x")), 0, cat, "dora"); err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(ctx, q.ID, "anna"); err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(ctx, q.ID, "anna"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete: %v", err)
	}
	all, more, err := db.Changes(ctx, ChangeQuery{})
	if err != nil || more {
		t.Fatal(err, more)
	}
	var kinds []string
	for _, c := range all {
		kinds = append(kinds, c.Kind+":"+c.Project)
	}
	want := "project-created:P project-created:Q segment-changed:P template-saved:T project-deleted:Q"
	if strings.Join(kinds, " ") != want {
		t.Fatalf("log %v", kinds)
	}
	seg := all[2]
	if seg.File != "Mietvertrag.docx" || seg.TargetLang != "fr-FR" || seg.Segment != "s1" || seg.Version != 2 || seg.By != "carl" || seg.ProjectID != p.ID {
		t.Fatalf("segment change %+v", seg)
	}
	mine, _, _ := db.Changes(ctx, ChangeQuery{Project: p.ID})
	if len(mine) != 2 {
		t.Fatalf("project log %+v", mine)
	}
	page, more, _ := db.Changes(ctx, ChangeQuery{Since: all[0].Seq, Limit: 2})
	if len(page) != 2 || !more || page[0].Seq != all[1].Seq {
		t.Fatalf("page %+v %v", page, more)
	}
	last, _ := db.LastChange(ctx)
	if last != all[len(all)-1].Seq {
		t.Fatalf("last %d", last)
	}
	// Deleting a project takes its files and segments with it.
	if _, err := db.Segments(ctx, q.ID, "Mietvertrag.docx", "en-GB"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("segments of a deleted project: %v", err)
	}
	var n int
	if err := db.sql.QueryRowContext(ctx, `SELECT count(*) FROM segments`).Scan(&n); err != nil || n != 9 {
		t.Fatalf("segments left: %d %v", n, err)
	}
}

func TestListOrder(t *testing.T) {
	db, _ := openTemp(t)
	for _, x := range []struct{ name, due string }{{"b later", "2026-12-10T10:00:00Z"}, {"a none", ""}, {"c sooner", "2026-12-01T10:00:00+02:00"}} {
		s := settings(x.name)
		s.Due = x.due
		if _, err := db.Create(ctx, s, files(), cat, "anna"); err != nil {
			t.Fatal(err)
		}
	}
	list, _ := db.List(ctx)
	var names []string
	for _, p := range list {
		names = append(names, p.Settings.Name)
	}
	if strings.Join(names, ",") != "c sooner,b later,a none" {
		t.Fatalf("order %v", names)
	}
}

func TestNewerSchemaRejected(t *testing.T) {
	sqliteOnly(t)
	db, path := openTemp(t)
	if _, err := db.sql.ExecContext(ctx, `PRAGMA user_version = 99`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if _, err := Open(path); err == nil || !strings.Contains(err.Error(), "newer") {
		t.Fatalf("expected schema error, got %v", err)
	}
}

func TestNULInNames(t *testing.T) {
	db, _ := openTemp(t)
	p, err := db.Create(ctx, settings("P"), files(), cat, "an\x00na")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.SaveSegment(ctx, p.ID, "Mietvertrag.docx", "en-GB", "s1", content.Text("x"), project.StatusDraft, 1, "b\x00en"); err != nil {
		t.Fatal(err)
	}
	cs, _, _ := db.Changes(ctx, ChangeQuery{Project: p.ID})
	if cs[0].By != "anna" || cs[1].By != "ben" {
		t.Fatalf("authors %+v", cs)
	}
}

// What the file says about a segment stays through saving; what the work
// left on it is replaced by each save; pre-translation records its match.
func TestSegmentDetails(t *testing.T) {
	db, key := openTemp(t)
	fs := files()
	brief := segs("Die Miete ist fällig.", "Kanzlei Weber")
	brief[0].Block, brief[0].MaxLength, brief[0].Notes = "u1", 40, []string{"Keep it short."}
	brief[0].File = &project.FileState{State: "reviewed", Target: `[{"text":"The rent is due."}]`, Match: 100}
	brief[0].Target, brief[0].Status = content.Text("The rent is due."), project.StatusConfirmed
	fs[3].Segments["en-GB"] = brief
	p, err := db.Create(ctx, settings("P"), fs, cat, "anna")
	if err != nil {
		t.Fatal(err)
	}
	got, err := reopen(t, key).Segments(ctx, p.ID, "return.sdlxliff", "en-GB")
	if err != nil {
		t.Fatal(err)
	}
	g := got[0]
	if g.Block != "u1" || g.MaxLength != 40 || len(g.Notes) != 1 || g.File == nil || g.File.State != "reviewed" || g.File.Match != 100 {
		t.Fatalf("details %+v", g)
	}
	e := project.Edit{Target: content.Text("Rent is due."), Status: project.StatusConfirmed,
		Origin:   &project.Origin{Kind: "tm", Score: 87, Memory: "Weber DE-EN", Inserted: `[{"text":"The rent is due."}]`},
		Warnings: []project.Warning{{Kind: "length", Text: "Longer than the source."}}, Ignored: []string{"length:Longer than the source."}}
	s, err := db.SaveEdit(ctx, p.ID, "return.sdlxliff", "en-GB", g.ID, e, g.Version, "ben")
	if err != nil {
		t.Fatal(err)
	}
	if s.Block != "u1" || s.File == nil || s.Origin == nil || s.Origin.Score != 87 || len(s.Warnings) != 1 || len(s.Ignored) != 1 || s.Target.Plain() != "Rent is due." {
		t.Fatalf("saved %+v", s)
	}
	// A plain save clears the origin and warnings, not the file's details.
	s, err = db.SaveSegment(ctx, p.ID, "return.sdlxliff", "en-GB", g.ID, content.Text("Rent due."), project.StatusDraft, s.Version, "ben")
	if err != nil {
		t.Fatal(err)
	}
	if s.Origin != nil || s.Warnings != nil || s.Block != "u1" || s.MaxLength != 40 {
		t.Fatalf("plain save %+v", s)
	}
	if n, err := db.ApplyTranslations(ctx, p.ID, "en-GB", []project.Translation{{File: "Mietvertrag.docx", ID: "s2", Target: content.Text("The tenant pays the rent."), Score: 95, Memory: "Weber DE-EN", Status: project.StatusDraft}}, "pre"); err != nil || n != 1 {
		t.Fatalf("applied %d %v", n, err)
	}
	en, _ := db.Segments(ctx, p.ID, "Mietvertrag.docx", "en-GB")
	if o := en[1].Origin; o == nil || o.Kind != "tm" || o.Score != 95 || o.Memory != "Weber DE-EN" || o.Inserted != `[{"text":"The tenant pays the rent."}]` {
		t.Fatalf("origin %+v", en[1].Origin)
	}
}

func TestDictionaries(t *testing.T) {
	db, key := openTemp(t)
	if ds, err := db.Dictionaries(ctx); err != nil || len(ds) != 0 {
		t.Fatalf("none yet: %v %v", ds, err)
	}
	x, err := db.SaveDictionary(ctx, Dictionary{Name: "de_DE_frami", Lang: "de-DE", Locales: []string{"de-DE", "de-AT"}, Words: 2}, []byte("SET UTF-8\n"), []byte("2\nMiete\nMieter\n"), "anna")
	if err != nil || x.CreatedBy != "anna" {
		t.Fatalf("saved %+v %v", x, err)
	}
	if _, err := db.SaveDictionary(ctx, Dictionary{Name: " "}, nil, nil, "anna"); err == nil {
		t.Fatal("a dictionary without a name")
	}
	db2 := reopen(t, key)
	ds, err := db2.Dictionaries(ctx)
	if err != nil || len(ds) != 1 || ds[0].Lang != "de-DE" || len(ds[0].Locales) != 2 || ds[0].Words != 2 {
		t.Fatalf("listed %+v %v", ds, err)
	}
	got, aff, dic, err := db2.DictionaryFiles(ctx, "DE_de_FRAMI")
	if err != nil || got.Name != "de_DE_frami" || string(aff) != "SET UTF-8\n" || !strings.Contains(string(dic), "Mieter") {
		t.Fatalf("files %+v %q %q %v", got, aff, dic, err)
	}
	// Saving again replaces it.
	if _, err := db2.SaveDictionary(ctx, Dictionary{Name: "de_DE_frami", Lang: "de-DE", Words: 3}, []byte("SET UTF-8\n"), []byte("3\nMiete\nMieter\nKaution\n"), "ben"); err != nil {
		t.Fatal(err)
	}
	if ds, _ := db2.Dictionaries(ctx); len(ds) != 1 || ds[0].Words != 3 || ds[0].CreatedBy != "ben" {
		t.Fatalf("replaced %+v", ds)
	}
	if err := db2.DeleteDictionary(ctx, "de_de_frami"); err != nil {
		t.Fatal(err)
	}
	if err := db2.DeleteDictionary(ctx, "de_DE_frami"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted twice: %v", err)
	}
}

func TestResegment(t *testing.T) {
	db, _ := openTemp(t)
	fs := files()
	lease := []project.Segment{
		{ID: "p1/s1", Block: "p1", Source: content.Text("Der Mieter zahlt die Miete und die Kaution."), Target: content.Text("The tenant pays."), Status: project.StatusConfirmed},
		{ID: "p1/s2", Block: "p1", Source: content.Text("Sie ist fällig."), Target: content.Text("It is due."), Status: project.StatusDraft},
		{ID: "p2/s1", Block: "p2", Source: content.Text("Schluss."), Status: project.StatusNew},
	}
	fs[0].Segments = map[string][]project.Segment{"en-GB": lease, "fr-FR": lease}
	p, err := db.Create(ctx, settings("Umschnitt"), fs, cat, "anna")
	if err != nil {
		t.Fatal(err)
	}
	const file = "Mietvertrag.docx"
	if _, err := db.Assign(ctx, Assignment{ProjectID: p.ID, File: file, TargetLang: "en-GB", FromSegment: "p1/s1", ToSegment: "p1/s2", UserID: 7, UserName: "ben", Task: TaskTranslate}, "anna"); err != nil {
		t.Fatal(err)
	}
	ids := func(ss []StoredSegment) string {
		var out []string
		for _, s := range ss {
			out = append(out, s.ID)
		}
		return strings.Join(out, " ")
	}
	// A stale copy is refused, and so is what the segments do not allow.
	if _, err := db.Resegment(ctx, p.ID, file, "en-gb", "p1/s1", ResegmentSplit, content.Text("Der Mieter zahlt die Miete"), 9, "ben"); !errors.Is(err, ErrConflict) {
		t.Errorf("stale: %v", err)
	}
	var no *project.ErrResegment
	if _, err := db.Resegment(ctx, p.ID, file, "en-GB", "p1/s2", ResegmentJoin, nil, 1, "ben"); !errors.As(err, &no) {
		t.Errorf("across paragraphs: %v", err)
	}
	if _, err := db.Resegment(ctx, p.ID, file, "en-GB", "nope", ResegmentJoin, nil, 1, "ben"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown: %v", err)
	}
	// Split: two parts where one was, the rest moves on.
	got, err := db.Resegment(ctx, p.ID, file, "en-gb", "p1/s1", ResegmentSplit, content.Text("Der Mieter zahlt die Miete"), 1, "ben")
	if err != nil {
		t.Fatal(err)
	}
	if ids(got) != "p1/s1#1 p1/s1#2 p1/s2 p2/s1" || got[0].Version != 2 || got[1].Version != 2 || got[2].Version != 1 || got[0].ChangedBy != "ben" || got[1].Split == nil || got[1].Split.Space != " " {
		t.Fatalf("split: %s %+v", ids(got), got[1])
	}
	if again, _ := db.Segments(ctx, p.ID, file, "en-GB"); ids(again) != ids(got) || again[1].Source.Plain() != "und die Kaution." || again[0].Target.Plain() != "The tenant pays." || again[0].Status != project.StatusDraft {
		t.Errorf("read again: %s %+v", ids(again), again)
	}
	// The other language keeps its segments.
	if fr, _ := db.Segments(ctx, p.ID, file, "fr-FR"); ids(fr) != "p1/s1 p1/s2 p2/s1" {
		t.Errorf("fr: %s", ids(fr))
	}
	// The work given out begins at the first part now.
	as, _ := db.Assignments(ctx, p.ID)
	if len(as) != 1 || as[0].FromSegment != "p1/s1#1" || as[0].ToSegment != "p1/s2" {
		t.Errorf("assignment after the split: %+v", as)
	}
	for _, id := range []string{"p1/s1#1", "p1/s1#2", "p1/s2"} {
		if mine, err := db.AssignedTo(ctx, p.ID, 7, file, "en-GB", id, TaskTranslate); err != nil || !mine {
			t.Errorf("%s is ben's: %v %v", id, mine, err)
		}
	}
	if mine, _ := db.AssignedTo(ctx, p.ID, 7, file, "en-GB", "p2/s1", TaskTranslate); mine {
		t.Error("the last segment is not ben's")
	}
	// The parts are saved like any segment, then joined back.
	if _, err := db.SaveSegment(ctx, p.ID, file, "en-GB", "p1/s1#2", content.Text("and the deposit."), project.StatusConfirmed, 2, "ben"); err != nil {
		t.Fatal(err)
	}
	got, err = db.Resegment(ctx, p.ID, file, "en-GB", "p1/s1#1", ResegmentJoin, nil, 2, "ben")
	if err != nil {
		t.Fatal(err)
	}
	if ids(got) != "p1/s1 p1/s2 p2/s1" || got[0].Target.Plain() != "The tenant pays. and the deposit." || got[0].Status != project.StatusDraft || got[0].Version != 4 || got[0].Split != nil {
		t.Fatalf("joined back: %s %+v", ids(got), got[0])
	}
	// Two sentences of a paragraph are joined, and the work ends at what they are now.
	got, err = db.Resegment(ctx, p.ID, file, "en-GB", "p1/s1", ResegmentJoin, nil, 4, "ben")
	if err != nil {
		t.Fatal(err)
	}
	if ids(got) != "p1/s1 p2/s1" || len(got[0].Joined) != 2 || got[0].Source.Plain() != "Der Mieter zahlt die Miete und die Kaution. Sie ist fällig." {
		t.Fatalf("joined: %s %+v", ids(got), got[0])
	}
	if as, _ := db.Assignments(ctx, p.ID); as[0].FromSegment != "p1/s1" || as[0].ToSegment != "p1/s1" {
		t.Errorf("assignment after the join: %+v", as[0])
	}
	// Split where it was joined, the two stand apart again; then a part is joined to the next sentence.
	got, err = db.Resegment(ctx, p.ID, file, "en-GB", "p1/s1", ResegmentSplit, content.Text("Der Mieter zahlt die Miete und die Kaution."), 5, "ben")
	if err != nil {
		t.Fatal(err)
	}
	if ids(got) != "p1/s1 p1/s2 p2/s1" || got[1].Status != project.StatusNew || got[1].Version != 6 {
		t.Fatalf("separated: %s", ids(got))
	}
	if as, _ := db.Assignments(ctx, p.ID); as[0].FromSegment != "p1/s1" || as[0].ToSegment != "p1/s2" {
		t.Errorf("assignment after separating: %+v", as[0])
	}
	got, err = db.Resegment(ctx, p.ID, file, "en-GB", "p1/s1", ResegmentSplit, content.Text("Der Mieter"), 6, "ben")
	if err != nil || ids(got) != "p1/s1#1 p1/s1#2 p1/s2 p2/s1" {
		t.Fatalf("split again: %v %s", err, ids(got))
	}
	got, err = db.Resegment(ctx, p.ID, file, "en-GB", "p1/s1#2", ResegmentJoin, nil, 7, "ben")
	if err != nil {
		t.Fatal(err)
	}
	if ids(got) != "p1/s1#1 p1/s1#2 p2/s1" || len(got[0].Joined) != 2 || len(got[1].Joined) != 2 || got[1].Source.Plain() != "zahlt die Miete und die Kaution. Sie ist fällig." || got[1].Version != 8 {
		t.Fatalf("a part joined with the next sentence: %s %+v", ids(got), got)
	}
	if as, _ := db.Assignments(ctx, p.ID); as[0].FromSegment != "p1/s1#1" || as[0].ToSegment != "p1/s1#2" {
		t.Errorf("assignment after that: %+v", as[0])
	}
	if again, _ := db.Segments(ctx, p.ID, file, "en-GB"); ids(again) != ids(got) || project.CheckRuns(storedOf(again)) != -1 {
		t.Errorf("read again: %s", ids(again))
	}
	// Each is in the change log.
	changes, _, err := db.Changes(ctx, ChangeQuery{})
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, c := range changes {
		if c.Kind == ChangeResegmented {
			n++
		}
	}
	if n != 6 {
		t.Errorf("%d changes logged, want 6", n)
	}
	// A range given out for every language at once is given out for each, to the same person.
	if _, err := db.Assign(ctx, Assignment{ProjectID: p.ID, File: "Anlage/Hausordnung.docx", FromSegment: "s1", ToSegment: "s1", UserID: 8, UserName: "cem", Task: TaskTranslate, Due: "2026-12-01T10:00:00Z"}, "anna"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Resegment(ctx, p.ID, "Anlage/Hausordnung.docx", "en-GB", "s1", ResegmentSplit, content.Text("Haus"), 1, "cem"); err != nil {
		t.Fatal(err)
	}
	as, _ = db.Assignments(ctx, p.ID)
	found := map[string]string{}
	for _, a := range as {
		if a.UserID == 8 {
			found[a.TargetLang] = a.FromSegment + " " + a.ToSegment + " " + a.Due
		}
	}
	if len(found) != 2 || found["en-GB"] != "s1#1 s1#2 2026-12-01T10:00:00Z" || found["fr-FR"] != "s1 s1 2026-12-01T10:00:00Z" {
		t.Errorf("given out per language: %+v", found)
	}
	for lang, id := range map[string]string{"en-GB": "s1#2", "fr-FR": "s1"} {
		if mine, err := db.AssignedTo(ctx, p.ID, 8, "Anlage/Hausordnung.docx", lang, id, TaskTranslate); err != nil || !mine {
			t.Errorf("%s in %s is cem's: %v %v", id, lang, mine, err)
		}
	}
}

func storedOf(ss []StoredSegment) []project.Segment {
	out := make([]project.Segment, len(ss))
	for i, s := range ss {
		out[i] = s.Segment
	}
	return out
}
