package termdb

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/4nonX/interlinea/internal/sqlitedb"
	"github.com/4nonX/interlinea/term"
)

func master(t *testing.T, db *DB, name string) *Termbase {
	t.Helper()
	tb, err := db.CreateBy(ctx, term.Definition{Name: name, Languages: []string{"de-DE", "en-GB"}, Fields: []term.Field{{Name: "Subject", Level: term.LevelEntry}, {Name: "Note", Level: term.LevelEntry}}}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	return tb
}

func withField(e term.Entry, field, value string) term.Entry {
	e.Fields = append(e.Fields, term.Value{Field: field, Value: value})
	return e
}

func TestChangeLog(t *testing.T) {
	db, _ := open(t)
	m := master(t, db, "Recht")
	s, err := m.Add(ctx, ent("", lng("de-DE", approved("Kaution")), lng("en-GB", approved("deposit"))), "eva", "")
	if err != nil {
		t.Fatal(err)
	}
	s.Languages[1].Terms = append(s.Languages[1].Terms, term.Term{Text: "security deposit"})
	if _, err := m.Update(ctx, s.Entry, s.Version, "jan", ""); err != nil {
		t.Fatal(err)
	}
	if err := m.DeleteEntry(ctx, s.ID, "eva", ""); err != nil {
		t.Fatal(err)
	}
	if err := m.RenameBy(ctx, "Mietrecht", "admin"); err != nil {
		t.Fatal(err)
	}
	other := master(t, db, "Medizin")
	if _, err := other.Add(ctx, ent("", lng("de-DE", approved("Dosis"))), "eva", ""); err != nil {
		t.Fatal(err)
	}
	all, more, err := db.Changes(ctx, ChangeQuery{})
	if err != nil || more {
		t.Fatal(err, more)
	}
	var got []string
	for _, c := range all {
		got = append(got, c.Termbase+":"+c.Kind+":"+c.EntryID+":"+c.By)
	}
	want := "Recht:termbase-created::admin Recht:created:c1:eva Recht:changed:c1:jan Recht:deleted:c1:eva Mietrecht:termbase-changed::admin Medizin:termbase-created::admin Medizin:created:c1:eva"
	if strings.Join(got, " ") != want {
		t.Fatalf("log:\n got %s\nwant %s", strings.Join(got, " "), want)
	}
	for i := 1; i < len(all); i++ {
		if all[i].Seq <= all[i-1].Seq {
			t.Fatal("sequence not increasing")
		}
	}
	// Paging and filtering by the current or an earlier name.
	page, more, _ := db.Changes(ctx, ChangeQuery{Since: all[1].Seq, Limit: 2})
	if len(page) != 2 || !more || page[0].Seq != all[2].Seq {
		t.Fatalf("page: %+v %v", page, more)
	}
	byName, _, _ := db.Changes(ctx, ChangeQuery{Termbases: []string{"mietrecht"}})
	if len(byName) != 5 {
		t.Fatalf("by current name: %d", len(byName))
	}
	last, _ := db.LastChange(ctx)
	if last != all[len(all)-1].Seq {
		t.Fatalf("last: %d", last)
	}
	if err := db.DeleteBy(ctx, "Medizin", "admin"); err != nil {
		t.Fatal(err)
	}
	after, _, _ := db.Changes(ctx, ChangeQuery{Since: last})
	if len(after) != 1 || after[0].Kind != ChangeTermbaseDeleted || after[0].Termbase != "Medizin" {
		t.Fatalf("deletion: %+v", after)
	}
}

// A database written before the change log gets it filled in from the
// entries' histories.
func TestChangeLogBackfill(t *testing.T) {
	if pgDSN() != "" {
		t.Skip("SQLite migration")
	}
	db, path := open(t)
	m := master(t, db, "Recht")
	m.Add(ctx, ent("", lng("de-DE", approved("Kaution"))), "eva", "")
	db.Close()
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	cs, _, _ := reopened.Changes(ctx, ChangeQuery{})
	if len(cs) != 2 {
		t.Fatalf("changes after reopening: %+v", cs)
	}
}

func setup(t *testing.T) (*DB, *Termbase, *Termbase, term.Entry) {
	t.Helper()
	db, _ := open(t)
	m := master(t, db, "Mietrecht")
	s, err := m.Add(ctx, withField(ent("", lng("de-DE", approved("Kaution")), lng("en-GB", approved("deposit"))), "Subject", "Law"), "admin", "")
	if err != nil {
		t.Fatal(err)
	}
	m.Add(ctx, ent("", lng("de-DE", approved("Mieter")), lng("en-GB", approved("tenant"))), "admin", "")
	p, err := db.CreateProject(ctx, term.Definition{Name: "Hofmann 2026"}, []string{"Mietrecht"}, "eva")
	if err != nil {
		t.Fatal(err)
	}
	return db, m, p, s.Entry
}

func TestProjectLayers(t *testing.T) {
	db, m, p, kaution := setup(t)
	info, _ := p.Info(ctx)
	if info.Kind != "project" || strings.Join(info.Upstreams, ",") != "Mietrecht" {
		t.Fatalf("info: %+v", info)
	}
	def, _ := p.Definition(ctx)
	if strings.Join(def.Languages, ",") != "de-DE,en-GB" || len(def.Fields) != 2 {
		t.Fatalf("project definition: %+v", def)
	}
	if _, err := db.CreateProject(ctx, term.Definition{Name: "x"}, []string{"Hofmann 2026"}, ""); err == nil {
		t.Fatal("project over a project accepted")
	}
	if err := db.Delete(ctx, "Mietrecht"); !errors.Is(err, ErrInUse) {
		t.Fatalf("master in use deleted: %v", err)
	}
	ls, _ := p.Layered(ctx)
	if len(ls) != 2 || ls[0].Termbase != "Mietrecht" {
		t.Fatalf("layered: %+v", ls)
	}
	// Changing a master entry in the project copies it.
	cp, err := p.Override(ctx, "Mietrecht", kaution.ID, "eva")
	if err != nil {
		t.Fatal(err)
	}
	again, _ := p.Override(ctx, "Mietrecht", kaution.ID, "eva")
	if again.ID != cp.ID {
		t.Fatal("second copy made")
	}
	ref, ok, _ := p.Base(ctx, cp.ID)
	if !ok || ref.Termbase != "Mietrecht" || ref.EntryID != kaution.ID || ref.Version != 1 {
		t.Fatalf("base: %+v %v", ref, ok)
	}
	p.Add(ctx, ent("", lng("de-DE", approved("Nebenkosten")), lng("en-GB", approved("service charges"))), "eva", "")
	ls, _ = p.Layered(ctx)
	var order []string
	for _, l := range ls {
		order = append(order, l.Termbase+"/"+terms(l.Entry, "de-DE")[0])
	}
	if strings.Join(order, " ") != "Hofmann 2026/Kaution Hofmann 2026/Nebenkosten Mietrecht/Mieter" {
		t.Fatalf("layered order: %v", order)
	}
	// The project's entries win in term recognition.
	x := term.NewIndex()
	if err := p.AddLayeredTo(ctx, x); err != nil {
		t.Fatal(err)
	}
	hits := x.Recognize("Die Kaution und der Mieter.", "de-DE", "en-GB")
	if len(hits) != 2 || hits[0].Source != "Hofmann 2026" || hits[1].Source != "Mietrecht" {
		t.Fatalf("hits: %+v", hits)
	}
	// Removing a master entry hides it and is proposed for deletion.
	if err := p.RemoveFromProject(ctx, "Mietrecht", "c2", "eva"); err != nil {
		t.Fatal(err)
	}
	ls, _ = p.Layered(ctx)
	if len(ls) != 2 {
		t.Fatalf("removed entry still shown: %+v", ls)
	}
	rs, _ := p.Removals(ctx)
	if len(rs) != 1 || rs[0].EntryID != "c2" {
		t.Fatalf("removals: %+v", rs)
	}
	// Taken back, the master's entry shows in the project again.
	if ok, err := p.RestoreToProject(ctx, "Mietrecht", "c2", "eva"); err != nil || !ok {
		t.Fatalf("restore: %v %v", ok, err)
	}
	if rs, _ := p.Removals(ctx); len(rs) != 0 {
		t.Fatalf("removals after restoring: %+v", rs)
	}
	layered, _ := p.Layered(ctx)
	seen := false
	for _, l := range layered {
		seen = seen || (l.Termbase == "Mietrecht" && l.ID == "c2")
	}
	if !seen {
		t.Fatalf("the restored entry is not shown: %+v", layered)
	}
	if ok, err := p.RestoreToProject(ctx, "Mietrecht", "c2", "eva"); err != nil || ok {
		t.Fatalf("restoring twice: %v %v", ok, err)
	}
	if _, err := p.RestoreToProject(ctx, "Nirgends", "c2", "eva"); err == nil {
		t.Fatal("restored from a termbase it is not layered over")
	}
	if err := p.RemoveFromProject(ctx, "Mietrecht", "c2", "eva"); err != nil {
		t.Fatal(err)
	}
	_ = m
}

func open1(t *testing.T, p *Termbase) *MergeRequest {
	t.Helper()
	mr, err := p.OpenMergeRequest(ctx, "Mietrecht", "", "eva", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	return mr
}

func items(t *testing.T, mr *MergeRequest, q ItemQuery) []Item {
	t.Helper()
	is, _, err := mr.Items(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	return is
}

func TestMergeFastForwardAndNew(t *testing.T) {
	_, m, p, kaution := setup(t)
	cp, _ := p.Override(ctx, "Mietrecht", kaution.ID, "eva")
	cp.Languages[1].Terms = append(cp.Languages[1].Terms, term.Term{Text: "security deposit"})
	if _, err := p.Update(ctx, cp.Entry, cp.Version, "eva", ""); err != nil {
		t.Fatal(err)
	}
	p.Add(ctx, ent("", lng("de-DE", approved("Nebenkosten")), lng("en-GB", approved("service charges"))), "eva", "")
	p.RemoveFromProject(ctx, "Mietrecht", "c2", "eva")
	mr := open1(t, p)
	is := items(t, mr, ItemQuery{Sort: "date"})
	var kinds []string
	for _, it := range is {
		kinds = append(kinds, it.Kind+":"+it.State)
	}
	if strings.Join(kinds, " ") != "changed:waiting new:waiting deleted:waiting" {
		t.Fatalf("items: %v", kinds)
	}
	if len(is[0].Changes) != 1 || is[0].Changes[0].Path != "en-GB term security deposit" || len(is[0].Conflicts) != 0 {
		t.Fatalf("changes: %+v", is[0])
	}
	// Approve them all; the master changes, the project follows.
	res, err := mr.Decide(ctx, Filter{}, Decision{State: StateApproved}, "rita")
	if err != nil || res.Decided != 3 {
		t.Fatalf("decide: %+v %v", res, err)
	}
	mk, _ := m.Get(ctx, kaution.ID)
	if mk.Version != 2 || len(terms(mk.Entry, "en-GB")) != 2 {
		t.Fatalf("master not fast-forwarded: %+v", mk)
	}
	if _, err := m.Get(ctx, "c2"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deletion not merged: %v", err)
	}
	ms, _ := m.Entries(ctx)
	if len(ms) != 2 || terms(ms[1].Entry, "de-DE")[0] != "Nebenkosten" {
		t.Fatalf("new entry not merged: %+v", ms)
	}
	// Nothing is pending afterwards.
	mr2 := open1(t, p)
	if is := items(t, mr2, ItemQuery{}); len(is) != 0 {
		t.Fatalf("pending after merge: %+v", is)
	}
	// History records the reviewer.
	h, _ := m.History(ctx, kaution.ID)
	if h[1].By != "rita" || !strings.Contains(h[1].Summary, "merge request") {
		t.Fatalf("history: %+v", h[1])
	}
	// Reverting the merge of the change.
	if err := mr.Revert(ctx, is[0].ID, "rita"); err != nil {
		t.Fatal(err)
	}
	mk, _ = m.Get(ctx, kaution.ID)
	if len(terms(mk.Entry, "en-GB")) != 1 || mk.Version != 3 {
		t.Fatalf("revert: %+v", mk)
	}
	if err := mr.Revert(ctx, is[0].ID, "rita"); err == nil {
		t.Fatal("reverted twice")
	}
	// Reverting the new entry deletes it; reverting the deletion restores.
	if err := mr.Revert(ctx, is[1].ID, "rita"); err != nil {
		t.Fatal(err)
	}
	if err := mr.Revert(ctx, is[2].ID, "rita"); err != nil {
		t.Fatal(err)
	}
	ms, _ = m.Entries(ctx)
	if len(ms) != 2 || terms(ms[1].Entry, "de-DE")[0] != "Mieter" {
		t.Fatalf("after reverts: %+v", ms)
	}
}

func TestMergeThreeWayAndConflicts(t *testing.T) {
	_, m, p, kaution := setup(t)
	cp, _ := p.Override(ctx, "Mietrecht", kaution.ID, "eva")
	// The project adds a term; the master changes the subject: no
	// conflict, both kept.
	cp.Languages[1].Terms = append(cp.Languages[1].Terms, term.Term{Text: "security deposit"})
	p.Update(ctx, cp.Entry, cp.Version, "eva", "")
	mk, _ := m.Get(ctx, kaution.ID)
	mk.Fields = []term.Value{{Field: "Subject", Value: "Tenancy law"}}
	m.Update(ctx, mk.Entry, mk.Version, "admin", "")
	mr := open1(t, p)
	is := items(t, mr, ItemQuery{})
	if len(is) != 1 || len(is[0].Conflicts) != 0 || is[0].Merged == nil {
		t.Fatalf("three-way: %+v", is)
	}
	if _, err := mr.Decide(ctx, Filter{IDs: []int64{is[0].ID}}, Decision{State: StateApproved}, "rita"); err != nil {
		t.Fatal(err)
	}
	mk, _ = m.Get(ctx, kaution.ID)
	if mk.Fields[0].Value != "Tenancy law" || len(terms(mk.Entry, "en-GB")) != 2 {
		t.Fatalf("merged: %+v", mk.Entry)
	}
	pk, _ := p.Get(ctx, cp.ID)
	if pk.Fields[0].Value != "Tenancy law" {
		t.Fatalf("project not brought in step: %+v", pk.Entry)
	}

	// Now both change the subject differently: a conflict.
	pk.Fields = []term.Value{{Field: "Subject", Value: "Contract law"}}
	p.Update(ctx, pk.Entry, pk.Version, "eva", "")
	mk.Fields = []term.Value{{Field: "Subject", Value: "Civil law"}}
	m.Update(ctx, mk.Entry, mk.Version, "admin", "")
	mr2 := open1(t, p)
	yes := true
	is = items(t, mr2, ItemQuery{Filter: Filter{Conflicts: &yes}})
	if len(is) != 1 || is[0].Conflicts[0].Path != "entry field Subject" || is[0].Conflicts[0].Ours != "Contract law" || is[0].Conflicts[0].Theirs != "Civil law" {
		t.Fatalf("conflict: %+v", is)
	}
	// Both ways out are offered: the master's value kept, or the project's taken.
	if is[0].Merged == nil || is[0].MergedOurs == nil || is[0].Merged.Fields[0].Value != "Civil law" || is[0].MergedOurs.Fields[0].Value != "Contract law" ||
		is[0].MergedOurs.ID != is[0].Merged.ID || len(terms(*is[0].MergedOurs, "en-GB")) != len(terms(*is[0].Merged, "en-GB")) {
		t.Fatalf("the two resolutions: %+v / %+v", is[0].Merged, is[0].MergedOurs)
	}
	if _, err := mr2.Decide(ctx, Filter{IDs: []int64{is[0].ID}}, Decision{State: StateApproved}, "rita"); !errors.Is(err, ErrNeedsResolution) {
		t.Fatalf("conflict approved: %v", err)
	}
	// A bulk approval leaves it waiting and says so.
	res, err := mr2.Decide(ctx, Filter{}, Decision{State: StateApproved}, "rita")
	if err != nil || res.Decided != 0 || len(res.NeedResolution) != 1 {
		t.Fatalf("bulk with conflict: %+v %v", res, err)
	}
	resolved := *is[0].Merged
	resolved.Fields = []term.Value{{Field: "Subject", Value: "Civil law"}, {Field: "Note", Value: "Agreed with Eva"}}
	if _, err := mr2.Decide(ctx, Filter{IDs: []int64{is[0].ID}}, Decision{State: StateEdited, Edited: &resolved}, "rita"); err != nil {
		t.Fatal(err)
	}
	mk, _ = m.Get(ctx, kaution.ID)
	if len(mk.Fields) != 2 || mk.Fields[1].Value != "Agreed with Eva" {
		t.Fatalf("resolution: %+v", mk.Entry)
	}
	if _, err := mr2.Decide(ctx, Filter{IDs: []int64{is[0].ID}}, Decision{State: StateApproved}, "rita"); !errors.Is(err, ErrDecided) {
		t.Fatalf("decided twice: %v", err)
	}
}

func TestScopedReview(t *testing.T) {
	db, m, p, _ := setup(t)
	// 40 new entries: 30 legal from students, 10 medical from Eva.
	for i := 0; i < 40; i++ {
		subject, by := "Law", "student"
		if i >= 30 {
			subject, by = "Medicine", "eva"
		}
		e := withField(ent("", lng("de-DE", approved(strings.Repeat("x", 3)+string(rune('a'+i%26))+string(rune('A'+i/26)))), lng("en-GB", approved("t"+string(rune('a'+i%26))+string(rune('A'+i/26))))), "Subject", subject)
		if _, err := p.Add(ctx, e, by, ""); err != nil {
			t.Fatal(err)
		}
	}
	// A forbidden term and a duplicate of a master entry come first.
	p.Add(ctx, ent("", lng("de-DE", forbidden("Mietzins"))), "student", "")
	p.Add(ctx, ent("", lng("de-DE", approved("Mieter")), lng("en-GB", approved("lessee"))), "student", "")
	mr := open1(t, p)
	all := items(t, mr, ItemQuery{Frequency: map[string]int{"c10": 99}})
	if len(all) != 42 || !all[0].Forbidden || len(all[1].Duplicates) != 1 || all[2].EntryID != "c10" {
		t.Fatalf("priority: %v %v %v %s", all[0].Forbidden, all[1].Duplicates, len(all), all[2].EntryID)
	}
	// Scopes: one reviewer per subject.
	mr.Assign(ctx, "rita", Filter{Fields: []term.Value{{Field: "Subject", Value: "law"}}})
	mr.Assign(ctx, "max", Filter{Fields: []term.Value{{Field: "Subject", Value: "Medicine"}}})
	if is := items(t, mr, ItemQuery{Reviewer: "max"}); len(is) != 10 {
		t.Fatalf("max's part: %d", len(is))
	}
	// A spot check of five, then the rest of the scope in bulk.
	law := Filter{Fields: []term.Value{{Field: "Subject", Value: "Law"}}, Authors: []string{"student"}}
	sample, _ := mr.Sample(ctx, law, 5, 1)
	sample2, _ := mr.Sample(ctx, law, 5, 1)
	if len(sample) != 5 || sample[0].ID != sample2[0].ID {
		t.Fatal("sample not repeatable")
	}
	var ids []int64
	for _, s := range sample {
		ids = append(ids, s.ID)
	}
	if res, err := mr.Decide(ctx, Filter{IDs: ids[:1]}, Decision{State: StateChanges, Comment: "Wrong subject"}, "rita"); err != nil || res.Decided != 1 {
		t.Fatalf("send back: %v", err)
	}
	if _, err := mr.Decide(ctx, Filter{IDs: ids[1:2]}, Decision{State: StateChanges}, "rita"); err == nil {
		t.Fatal("request for changes without a comment accepted")
	}
	res, err := mr.Decide(ctx, law, Decision{State: StateApproved}, "rita")
	if err != nil || res.Decided != 29 {
		t.Fatalf("bulk: %+v %v", res, err)
	}
	mr.Decide(ctx, Filter{Fields: []term.Value{{Field: "Subject", Value: "Medicine"}}}, Decision{State: StatePostponed}, "max")
	pr, _ := mr.Progress(ctx)
	if pr.Total != 42 || pr.States[StateApproved] != 29 || pr.States[StateChanges] != 1 || pr.States[StatePostponed] != 10 || pr.States[StateWaiting] != 2 ||
		pr.ByReviewer["rita"][StateApproved] != 29 || pr.ByReviewer["max"][StatePostponed] != 10 {
		t.Fatalf("progress: %+v", pr)
	}
	ms, _ := m.Entries(ctx)
	if len(ms) != 2+29 {
		t.Fatalf("master entries: %d", len(ms))
	}
	// Closing: postponed and waiting items come back in the next request;
	// the one sent back only once changed.
	if err := mr.Close(ctx, "rita"); err != nil {
		t.Fatal(err)
	}
	if _, err := mr.Decide(ctx, Filter{}, Decision{State: StateApproved}, "rita"); !errors.Is(err, ErrClosed) {
		t.Fatalf("decided on a closed request: %v", err)
	}
	next := open1(t, p)
	if is := items(t, next, ItemQuery{}); len(is) != 12 {
		t.Fatalf("next request: %d", len(is))
	}
	sent := sample[0].EntryID
	s, _ := p.Get(ctx, sent)
	s.Fields = []term.Value{{Field: "Subject", Value: "Medicine"}}
	p.Update(ctx, s.Entry, s.Version, "student", "")
	next.Close(ctx, "rita")
	third := open1(t, p)
	if is := items(t, third, ItemQuery{}); len(is) != 13 {
		t.Fatalf("changed entry not back: %d", len(is))
	}
	reqs, _ := m.MergeRequests(ctx)
	if len(reqs) != 3 || reqs[0].ID != third.ID {
		t.Fatalf("requests: %d", len(reqs))
	}
	_ = db
}

func TestReviewRules(t *testing.T) {
	_, m, p, kaution := setup(t)
	if err := m.SetReviewRules(ctx, ReviewRules{MinorFields: []string{"Note"}, Trusted: []TrustedRule{{Author: "rita", Scope: Filter{Langs: []string{"en"}}}}}, "admin"); err != nil {
		t.Fatal(err)
	}
	if err := p.SetReviewRules(ctx, ReviewRules{}, "eva"); err == nil {
		t.Fatal("rules on a project accepted")
	}
	// A note only: merged without review.
	cp, _ := p.Override(ctx, "Mietrecht", kaution.ID, "eva")
	cp.Fields = append(cp.Fields, term.Value{Field: "Note", Value: "See § 551 BGB"})
	p.Update(ctx, cp.Entry, cp.Version, "eva", "")
	// A copy of a master entry, unchanged: no item at all.
	p.Override(ctx, "Mietrecht", "c2", "eva")
	// Identical to the master: linked, not copied.
	p.Add(ctx, ent("", lng("de-DE", approved("Mieter")), lng("en-GB", approved("tenant"))), "eva", "")
	// A trusted author in scope: approved; out of scope: waiting.
	p.Add(ctx, ent("", lng("en-GB", approved("landlord"))), "rita", "")
	p.Add(ctx, ent("", lng("de-DE", approved("Vermieter"))), "rita", "")
	mr := open1(t, p)
	var got []string
	for _, it := range items(t, mr, ItemQuery{Sort: "date"}) {
		got = append(got, it.Kind+":"+it.State+":"+it.Rule)
	}
	if strings.Join(got, " ") != "changed:approved:minor-fields new:approved:identical new:approved:trusted:rita new:waiting:" {
		t.Fatalf("rules: %v", got)
	}
	mk, _ := m.Get(ctx, kaution.ID)
	if len(mk.Fields) != 2 {
		t.Fatalf("minor change not merged: %+v", mk.Entry)
	}
	ms, _ := m.Entries(ctx)
	if len(ms) != 3 {
		t.Fatalf("master entries: %d (identical entry copied?)", len(ms))
	}
	rules, _ := m.ReviewRules(ctx)
	if len(rules.Trusted) != 1 {
		t.Fatalf("rules: %+v", rules)
	}
}

// A database at schema version 2 (columns named "by", masters as JSON)
// moves to version 3 without losing anything.
func TestMigrateToVersion3(t *testing.T) {
	if pgDSN() != "" {
		t.Skip("SQLite migration")
	}
	path := filepath.Join(t.TempDir(), "v2.db")
	sdb, err := sqlitedb.Open(path, "termdb", migrations[:2])
	if err != nil {
		t.Fatal(err)
	}
	stmts := []string{
		`INSERT INTO termbases(id, name, definition_json, created_at, kind) VALUES (1, 'Recht', '{"name":"Recht","languages":["de-DE"]}', '2026-11-23T09:00:00Z', 'master')`,
		`INSERT INTO termbases(id, name, definition_json, created_at, kind, upstreams_json) VALUES (2, 'Projekt', '{"name":"Projekt","languages":["de-DE"]}', '2026-11-23T09:00:00Z', 'project', '[1]')`,
		`INSERT INTO entries(id, termbase_id, entry_id, version, entry_json, created_at, changed_at) VALUES (1, 1, 'c1', 1, '{"id":"c1","languages":[{"lang":"de-DE","terms":[{"text":"Kaution"}]}]}', '2026-11-23T09:00:00Z', '2026-11-23T09:00:00Z')`,
		`INSERT INTO versions(entry_row, version, at, by, summary, entry_json) VALUES (1, 1, '2026-11-23T09:00:00Z', 'eva', 'Created', '{}')`,
		`INSERT INTO changes(termbase_id, termbase, entry_id, version, kind, at, by, summary) VALUES (1, 'Recht', 'c1', 1, 'created', '2026-11-23T09:00:00Z', 'eva', 'Created')`,
	}
	for _, s := range stmts {
		if _, err := sdb.Exec(s); err != nil {
			t.Fatal(err)
		}
	}
	sdb.Close()
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	p, err := db.Termbase(ctx, "projekt")
	if err != nil {
		t.Fatal(err)
	}
	info, _ := p.Info(ctx)
	if strings.Join(info.Upstreams, ",") != "Recht" {
		t.Fatalf("upstreams: %+v", info)
	}
	m, _ := db.Termbase(ctx, "Recht")
	h, err := m.History(ctx, "c1")
	if err != nil || h[0].By != "eva" {
		t.Fatalf("history: %+v %v", h, err)
	}
	cs, _, _ := db.Changes(ctx, ChangeQuery{})
	if len(cs) != 1 || cs[0].By != "eva" {
		t.Fatalf("changes: %+v", cs)
	}
}

func TestRebind(t *testing.T) {
	got := postgresDialect.Rebind(`SELECT a FROM t WHERE x = ? AND y = '?' AND z IN (?, lower(?))`)
	if got != `SELECT a FROM t WHERE x = $1 AND y = '?' AND z IN ($2, lower($3))` {
		t.Fatal(got)
	}
	if sqliteDialect.Rebind("x = ?") != "x = ?" {
		t.Fatal("sqlite rebound")
	}
	cond, arg := searchCondition(postgresDialect, `50% of_x`)
	if cond != `search_body ILIKE ? ESCAPE '\'` || arg != `%50\% of\_x%` {
		t.Fatal(cond, arg)
	}
}
