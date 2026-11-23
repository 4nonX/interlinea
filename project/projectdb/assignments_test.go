package projectdb

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func assign(t *testing.T, db *DB, a Assignment) Assignment {
	t.Helper()
	got, err := db.Assign(ctx, a, "anna")
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func refused(t *testing.T, err error, field string) {
	t.Helper()
	var inv *InvalidError
	if !errors.As(err, &inv) {
		t.Fatalf("want a problem with %s, got %v", field, err)
	}
	for _, p := range inv.Problems {
		if p.Field == field {
			return
		}
	}
	t.Fatalf("problems %+v, want one with %s", inv.Problems, field)
}

func TestAssignments(t *testing.T) {
	db, key := openTemp(t)
	p := create(t, db, "P")
	q := create(t, db, "Q")

	ben := assign(t, db, Assignment{ProjectID: p.ID, File: "mietvertrag.docx", TargetLang: "en-gb", FromSegment: "s1", ToSegment: "s2",
		UserID: 2, UserName: " Ben ", Task: TaskTranslate, Due: "2026-12-01T12:00:00Z"})
	if ben.ID == 0 || ben.File != "Mietvertrag.docx" || ben.TargetLang != "en-GB" || ben.UserName != "Ben" || ben.Version != 1 || ben.ChangedBy != "anna" || ben.Done {
		t.Fatalf("assigned %+v", ben)
	}
	// The rest of the file, French, a reviewer on the same segments and a
	// whole-project reviewer for another language don't overlap.
	assign(t, db, Assignment{ProjectID: p.ID, File: "Mietvertrag.docx", TargetLang: "en-GB", FromSegment: "s3", ToSegment: "s3", UserID: 3, UserName: "Carl", Task: TaskTranslate})
	assign(t, db, Assignment{ProjectID: p.ID, File: "Mietvertrag.docx", TargetLang: "fr-FR", UserID: 3, UserName: "Carl", Task: TaskTranslate})
	dora := assign(t, db, Assignment{ProjectID: p.ID, File: "Mietvertrag.docx", TargetLang: "en-GB", UserID: 4, UserName: "Dora", Task: TaskReview})
	assign(t, db, Assignment{ProjectID: q.ID, UserID: 2, UserName: "Ben", Task: TaskTranslate})

	_, err := db.Assign(ctx, Assignment{ProjectID: p.ID, File: "Mietvertrag.docx", TargetLang: "en-GB", FromSegment: "s2", ToSegment: "s3", UserID: 5, UserName: "Emil", Task: TaskTranslate}, "anna")
	refused(t, err, "fromSegment")
	if !strings.Contains(err.Error(), "Ben is translating") {
		t.Fatalf("overlap message: %v", err)
	}
	_, err = db.Assign(ctx, Assignment{ProjectID: p.ID, UserID: 5, UserName: "Emil", Task: TaskReview}, "anna")
	refused(t, err, "fromSegment") // the whole project covers Dora's file
	_, err = db.Assign(ctx, Assignment{ProjectID: p.ID, TargetLang: "fr-FR", UserID: 5, UserName: "Emil", Task: TaskTranslate}, "anna")
	refused(t, err, "fromSegment") // all French files cover Carl's

	for _, c := range []struct {
		a     Assignment
		field string
	}{
		{Assignment{ProjectID: p.ID, Task: TaskTranslate}, "user"},
		{Assignment{ProjectID: p.ID, UserID: 5, Task: "proofread"}, "task"},
		{Assignment{ProjectID: p.ID, UserID: 5, Task: TaskTranslate, Due: "Friday"}, "due"},
		{Assignment{ProjectID: p.ID, UserID: 5, Task: TaskTranslate, TargetLang: "it-IT"}, "targetLang"},
		{Assignment{ProjectID: p.ID, UserID: 5, Task: TaskTranslate, File: "Nope.docx"}, "file"},
		{Assignment{ProjectID: p.ID, UserID: 5, Task: TaskTranslate, File: "Styleguide.pdf"}, "file"},
		{Assignment{ProjectID: p.ID, UserID: 5, Task: TaskTranslate, File: "return.sdlxliff", TargetLang: "fr-FR"}, "targetLang"},
		{Assignment{ProjectID: p.ID, UserID: 5, Task: TaskTranslate, FromSegment: "s1", ToSegment: "s1"}, "fromSegment"},
		{Assignment{ProjectID: p.ID, UserID: 5, Task: TaskTranslate, File: "Anlage/Hausordnung.docx", FromSegment: "s1"}, "toSegment"},
		{Assignment{ProjectID: p.ID, UserID: 5, Task: TaskTranslate, File: "Anlage/Hausordnung.docx", FromSegment: "s9", ToSegment: "s1"}, "fromSegment"},
		{Assignment{ProjectID: p.ID, UserID: 5, Task: TaskTranslate, File: "Anlage/Hausordnung.docx", FromSegment: "s1", ToSegment: "s9"}, "toSegment"},
	} {
		_, err := db.Assign(ctx, c.a, "anna")
		refused(t, err, c.field)
	}
	_, err = db.Assign(ctx, Assignment{ProjectID: p.ID, UserID: 5, UserName: "Emil", Task: TaskTranslate, File: "Mietvertrag.docx", TargetLang: "fr-FR", FromSegment: "s3", ToSegment: "s1"}, "anna")
	refused(t, err, "toSegment")
	if _, err := db.Assign(ctx, Assignment{ProjectID: 999, UserID: 5, Task: TaskTranslate}, "anna"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("assignment in a missing project: %v", err)
	}

	all, err := db.Assignments(ctx, p.ID)
	if err != nil || len(all) != 4 {
		t.Fatalf("project assignments %d, %v", len(all), err)
	}
	mine, err := db.AssignmentsOf(ctx, 2)
	if err != nil || len(mine) != 2 || mine[0].ProjectID != p.ID || mine[1].ProjectID != q.ID {
		t.Fatalf("Ben's assignments %+v, %v", mine, err)
	}

	for _, c := range []struct {
		user       int64
		file, lang string
		seg        string
		task       Task
		want       bool
		why        string
	}{
		{2, "Mietvertrag.docx", "en-GB", "s2", TaskTranslate, true, "Ben's range"},
		{2, "MIETVERTRAG.docx", "en-gb", "s1", TaskTranslate, true, "names and languages in any case"},
		{2, "Mietvertrag.docx", "en-GB", "s3", TaskTranslate, false, "outside Ben's range"},
		{2, "Mietvertrag.docx", "fr-FR", "s1", TaskTranslate, false, "another language"},
		{2, "Mietvertrag.docx", "en-GB", "s1", TaskReview, false, "translators don't review"},
		{4, "Mietvertrag.docx", "en-GB", "s3", TaskReview, true, "Dora's file"},
		{4, "Mietvertrag.docx", "en-GB", "s3", TaskTranslate, true, "reviewers correct"},
		{4, "Anlage/Hausordnung.docx", "en-GB", "s1", TaskReview, false, "another file"},
		{3, "Mietvertrag.docx", "fr-FR", "s9", TaskTranslate, false, "a missing segment"},
	} {
		got, err := db.AssignedTo(ctx, p.ID, c.user, c.file, c.lang, c.seg, c.task)
		if err != nil || got != c.want {
			t.Errorf("%s: %v, %v", c.why, got, err)
		}
	}
	if _, err := db.AssignedTo(ctx, p.ID, 2, "Nope.docx", "en-GB", "s1", TaskTranslate); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing file: %v", err)
	}

	// Ben finishes; Dora's review moves to Emil.
	ben.Done = true
	done, err := db.UpdateAssignment(ctx, ben, 1, "ben")
	if err != nil || !done.Done || done.Version != 2 || done.ChangedBy != "ben" {
		t.Fatalf("done %+v, %v", done, err)
	}
	if _, err := db.UpdateAssignment(ctx, ben, 1, "ben"); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale update: %v", err)
	}
	dora.UserID, dora.UserName = 5, "Emil"
	emil, err := db.UpdateAssignment(ctx, dora, 1, "anna")
	if err != nil || emil.UserName != "Emil" {
		t.Fatalf("moved %+v, %v", emil, err)
	}
	// An assignment doesn't overlap itself, but can't grow onto another.
	grow := done
	grow.ToSegment = "s3"
	_, err = db.UpdateAssignment(ctx, grow, 2, "anna")
	refused(t, err, "fromSegment")
	if _, err := db.UpdateAssignment(ctx, Assignment{ID: 999, UserID: 1, Task: TaskReview}, 1, "anna"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing assignment: %v", err)
	}
	if err := db.Unassign(ctx, emil.ID, "anna"); err != nil {
		t.Fatal(err)
	}
	if err := db.Unassign(ctx, emil.ID, "anna"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second removal: %v", err)
	}
	if ok, _ := db.AssignedTo(ctx, p.ID, 5, "Mietvertrag.docx", "en-GB", "s1", TaskReview); ok {
		t.Fatal("removed assignment still counts")
	}

	log, _, err := db.Changes(ctx, ChangeQuery{Project: p.ID})
	if err != nil {
		t.Fatal(err)
	}
	var set, removed int
	for _, c := range log {
		switch c.Kind {
		case ChangeAssignment:
			set++
			if c.File != "Mietvertrag.docx" {
				t.Errorf("logged %+v", c)
			}
		case ChangeAssignmentRemoved:
			removed++
		}
	}
	if set != 6 || removed != 1 {
		t.Fatalf("logged %d set, %d removed", set, removed)
	}

	// Assignments are kept on disk.
	again := reopen(t, key)
	if all, err := again.Assignments(ctx, p.ID); err != nil || len(all) != 3 || !all[0].Done || all[0].Due != "2026-12-01T12:00:00Z" {
		t.Fatalf("after reopening %+v, %v", all, err)
	}
}

func TestAssignmentsFollowTheProject(t *testing.T) {
	db, _ := openTemp(t)
	p := create(t, db, "P")
	assign(t, db, Assignment{ProjectID: p.ID, File: "Mietvertrag.docx", TargetLang: "fr-FR", UserID: 2, UserName: "Ben", Task: TaskTranslate})
	assign(t, db, Assignment{ProjectID: p.ID, File: "Mietvertrag.docx", TargetLang: "en-GB", UserID: 3, UserName: "Carl", Task: TaskTranslate})
	assign(t, db, Assignment{ProjectID: p.ID, File: "Anlage/Hausordnung.docx", UserID: 4, UserName: "Dora", Task: TaskTranslate})
	assign(t, db, Assignment{ProjectID: p.ID, TargetLang: "fr-FR", UserID: 5, UserName: "Emil", Task: TaskReview})
	if _, err := db.Touch(ctx, p.ID, "Mietvertrag.docx", "fr-FR", "s1", 2, "Ben"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Touch(ctx, p.ID, "Anlage/Hausordnung.docx", "en-GB", "s1", 4, "Dora"); err != nil {
		t.Fatal(err)
	}

	// French goes: its assignments and marks go with it.
	s := p.Settings
	s.TargetLangs = []string{"en-GB"}
	got, err := db.UpdateSettings(ctx, p.ID, s, 1, cat, "anna")
	if err != nil {
		t.Fatal(err)
	}
	left, _ := db.Assignments(ctx, p.ID)
	if len(left) != 2 || left[0].UserName != "Carl" || left[1].UserName != "Dora" {
		t.Fatalf("after dropping French %+v", left)
	}
	if marks, _ := db.Present(ctx, p.ID, "", ""); len(marks) != 1 || marks[0].UserName != "Dora" {
		t.Fatalf("marks after dropping French %+v", marks)
	}

	// A file goes: its assignments and marks go with it.
	if _, err := db.RemoveFile(ctx, p.ID, "anlage/hausordnung.docx", got.Version, cat, "anna"); err != nil {
		t.Fatal(err)
	}
	left, _ = db.Assignments(ctx, p.ID)
	if len(left) != 1 || left[0].UserName != "Carl" {
		t.Fatalf("after removing the house rules %+v", left)
	}
	if marks, _ := db.Present(ctx, p.ID, "", ""); len(marks) != 0 {
		t.Fatalf("marks after removing the file %+v", marks)
	}

	// The project goes: everything goes.
	if err := db.Delete(ctx, p.ID, "anna"); err != nil {
		t.Fatal(err)
	}
	if left, _ := db.AssignmentsOf(ctx, 3); len(left) != 0 {
		t.Fatalf("after deleting the project %+v", left)
	}
}

func TestPresence(t *testing.T) {
	db, _ := openTemp(t)
	p := create(t, db, "P")
	q := create(t, db, "Q")
	var mu sync.Mutex
	clock := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	db.now = func() time.Time { mu.Lock(); defer mu.Unlock(); return clock }
	wait := func(d time.Duration) { mu.Lock(); clock = clock.Add(d); mu.Unlock() }

	ben, err := db.Touch(ctx, p.ID, "mietvertrag.docx", "en-gb", "s1", 2, "Ben")
	if err != nil || ben.File != "Mietvertrag.docx" || ben.TargetLang != "en-GB" || !ben.Since.Equal(clock) || !ben.Expires.Equal(clock.Add(PresenceTTL)) {
		t.Fatalf("touched %+v, %v", ben, err)
	}
	wait(500 * time.Millisecond)
	if _, err := db.Touch(ctx, p.ID, "Mietvertrag.docx", "fr-FR", "s2", 3, "Carl"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Touch(ctx, q.ID, "Mietvertrag.docx", "en-GB", "s3", 2, "Ben"); err != nil {
		t.Fatal(err) // one mark per project: Ben can have Q open too
	}
	wait(time.Minute)
	// Renewing keeps when Ben opened the segment.
	renewed, err := db.Touch(ctx, p.ID, "Mietvertrag.docx", "en-GB", "s1", 2, "Ben")
	if err != nil || !renewed.Since.Equal(ben.Since) || !renewed.Expires.Equal(clock.Add(PresenceTTL)) {
		t.Fatalf("renewed %+v, %v", renewed, err)
	}
	en, _ := db.Present(ctx, p.ID, "MIETVERTRAG.docx", "en-gb")
	if len(en) != 1 || en[0].UserName != "Ben" || en[0].Segment != "s1" {
		t.Fatalf("English marks %+v", en)
	}
	all, _ := db.Present(ctx, p.ID, "", "")
	if len(all) != 2 || all[0].UserName != "Ben" || all[1].UserName != "Carl" {
		t.Fatalf("project marks %+v", all)
	}
	// Moving on replaces the mark and starts it again.
	wait(time.Second)
	moved, err := db.Touch(ctx, p.ID, "Mietvertrag.docx", "en-GB", "s2", 2, "Ben")
	if err != nil || !moved.Since.Equal(clock) {
		t.Fatalf("moved %+v, %v", moved, err)
	}
	if en, _ := db.Present(ctx, p.ID, "Mietvertrag.docx", "en-GB"); len(en) != 1 || en[0].Segment != "s2" {
		t.Fatalf("after moving %+v", en)
	}
	// Carl's mark runs out (sub-second times compare in order too).
	wait(PresenceTTL - 62*time.Second) // 10:01:59.5; Carl's runs to 10:02:00.5
	if all, _ := db.Present(ctx, p.ID, "", ""); len(all) != 2 {
		t.Fatalf("Carl's mark ran out early: %+v", all)
	}
	wait(time.Second)
	if all, _ := db.Present(ctx, p.ID, "", ""); len(all) != 1 || all[0].UserName != "Ben" {
		t.Fatalf("after Carl's ran out %+v", all)
	}
	if _, err := db.Touch(ctx, q.ID, "Mietvertrag.docx", "en-GB", "s3", 2, "Ben"); err != nil {
		t.Fatal(err)
	}
	if err := db.Leave(ctx, p.ID, 2); err != nil {
		t.Fatal(err)
	}
	if all, _ := db.Present(ctx, p.ID, "", ""); len(all) != 0 {
		t.Fatalf("after leaving %+v", all)
	}
	if other, _ := db.Present(ctx, q.ID, "", ""); len(other) != 1 {
		t.Fatalf("leaving P left Q too: %+v", other)
	}
	if _, err := db.Touch(ctx, p.ID, "Mietvertrag.docx", "en-GB", "s9", 2, "Ben"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing segment: %v", err)
	}
	if _, err := db.Touch(ctx, p.ID, "Nope.docx", "en-GB", "s1", 2, "Ben"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing file: %v", err)
	}
	// Marks are not changes.
	log, _, _ := db.Changes(ctx, ChangeQuery{Project: p.ID})
	for _, c := range log {
		if c.Kind != ChangeProjectCreated {
			t.Fatalf("presence logged %+v", c)
		}
	}
}
