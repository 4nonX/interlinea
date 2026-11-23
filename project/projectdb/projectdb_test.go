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
		"segment without ID":      func(f []NewFile) { f[1].Segments["en-GB"] = []project.Segment{{Source: content.Text("a"), Status: project.StatusNew}} },
		"unknown status":          func(f []NewFile) { f[1].Segments["en-GB"] = []project.Segment{{ID: "a", Source: content.Text("a"), Status: "done"}} },
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
