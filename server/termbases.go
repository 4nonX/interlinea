package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/4nonX/interlinea/auth/authdb"
	"github.com/4nonX/interlinea/langtag"
	"github.com/4nonX/interlinea/project"
	"github.com/4nonX/interlinea/project/projectdb"
	"github.com/4nonX/interlinea/term"
	"github.com/4nonX/interlinea/term/termdb"
)

func (s *Server) termbaseRoutes() {
	s.handle("GET /api/termbases", s.signedIn(s.listTermbases))
	s.handle("POST /api/termbases", s.signedIn(s.createTermbase))
	s.handle("GET /api/termbases/{name}", s.signedIn(s.getTermbase))
	s.handle("DELETE /api/termbases/{name}", s.signedIn(s.deleteTermbase))
	s.handle("PUT /api/termbases/{name}/name", s.signedIn(s.renameTermbase))
	s.handle("PUT /api/termbases/{name}/definition", s.signedIn(s.setDefinition))
	s.handle("PUT /api/termbases/{name}/upstreams", s.signedIn(s.setUpstreams))
	s.handle("DELETE /api/termbases/{name}/languages/{lang}", s.signedIn(s.removeLanguage))
	s.handle("DELETE /api/termbases/{name}/fields/{field}", s.signedIn(s.removeField))
	s.handle("PUT /api/termbases/{name}/fields/{field}", s.signedIn(s.renameField))
	s.handle("GET /api/termbases/{name}/entries", s.signedIn(s.searchEntries))
	s.handle("POST /api/termbases/{name}/entries", s.signedIn(s.addEntry))
	s.handle("GET /api/termbases/{name}/entries/{eid}", s.signedIn(s.getEntry))
	s.handle("PUT /api/termbases/{name}/entries/{eid}", s.signedIn(s.updateEntry))
	s.handle("DELETE /api/termbases/{name}/entries/{eid}", s.signedIn(s.deleteEntry))
	s.handle("GET /api/termbases/{name}/entries/{eid}/history", s.signedIn(s.entryHistory))
	s.handle("POST /api/termbases/{name}/entries/{eid}/revert", s.signedIn(s.revertEntry))
	s.handle("POST /api/termbases/{name}/merge", s.signedIn(s.mergeEntries))
	s.handle("GET /api/termbases/{name}/duplicates", s.signedIn(s.duplicates))
	s.handle("GET /api/termbases/{name}/deleted", s.signedIn(s.deletedEntries))
	s.handle("GET /api/termbases/{name}/lookup", s.signedIn(s.termLookup))
	s.handle("GET /api/termbases/{name}/layered", s.signedIn(s.layeredEntries))
	s.handle("POST /api/termbases/{name}/overrides", s.signedIn(s.overrideEntry))
	s.handle("POST /api/termbases/{name}/removals", s.signedIn(s.removeFromProject))
	s.handle("GET /api/termbases/{name}/removals", s.signedIn(s.removals))
	s.handle("GET /api/termbases/{name}/export", s.signedIn(s.exportTermbase))
	s.handle("POST /api/termbases/{name}/export", s.signedIn(s.exportEntries))
	s.handle("POST /api/termbases/{name}/import", s.signedIn(s.importTermbase))
	s.handle("POST /api/termfiles/inspect", s.signedIn(s.inspectTermFile))
	s.handle("GET /api/termbases/{name}/changes", s.signedIn(s.termbaseChanges))
	s.handle("GET /api/termbases/{name}/review-rules", s.signedIn(s.reviewRules))
	s.handle("PUT /api/termbases/{name}/review-rules", s.signedIn(s.setReviewRules))
	s.handle("GET /api/termbases/{name}/merge-requests", s.signedIn(s.mergeRequests))
	s.handle("POST /api/termbases/{name}/merge-requests", s.signedIn(s.openMergeRequest))
	s.handle("GET /api/merge-requests/{id}", s.signedIn(s.getMergeRequest))
	s.handle("GET /api/merge-requests/{id}/items", s.signedIn(s.mergeItems))
	s.handle("POST /api/merge-requests/{id}/items", s.signedIn(s.queryMergeItems))
	s.handle("POST /api/merge-requests/{id}/sample", s.signedIn(s.sampleMergeItems))
	s.handle("GET /api/merge-requests/{id}/progress", s.signedIn(s.mergeProgress))
	s.handle("POST /api/merge-requests/{id}/decide", s.signedIn(s.decide))
	s.handle("GET /api/merge-requests/{id}/reviewers", s.signedIn(s.mergeReviewers))
	s.handle("PUT /api/merge-requests/{id}/reviewers", s.signedIn(s.assignReviewer))
	s.handle("DELETE /api/merge-requests/{id}/reviewers", s.signedIn(s.unassignReviewer))
	s.handle("POST /api/merge-requests/{id}/close", s.signedIn(s.closeMergeRequest))
	s.handle("POST /api/merge-requests/{id}/items/{iid}/revert", s.signedIn(s.revertMergeItem))
}

// termbaseRole is what someone may do with a termbase: their grant on
// it, or, for a project's own termbase, what they may do in the project
// (managers manage it, editors and assigned people edit it, readers read
// it), whichever is more.
func (s *Server) termbaseRole(ctx context.Context, me session, name string) (authdb.Role, error) {
	return s.termbaseRoleIn(ctx, me, name, func() ([]projectdb.Summary, error) { return s.cfg.Projects.List(ctx) })
}

// termbaseRoleIn is termbaseRole with the projects to look in.
func (s *Server) termbaseRoleIn(ctx context.Context, me session, name string, projects func() ([]projectdb.Summary, error)) (authdb.Role, error) {
	role, err := s.cfg.Auth.RoleOn(ctx, me.User, authdb.KindTermbase, name)
	if err != nil || role == authdb.RoleManage {
		return role, err
	}
	ps, err := projects()
	if err != nil {
		return role, err
	}
	rank := map[authdb.Role]int{authdb.RoleRead: 1, authdb.RoleSuggest: 2, authdb.RoleEdit: 3, authdb.RoleApprove: 4, authdb.RoleManage: 5}
	for _, p := range ps {
		t := p.Settings.Terminology
		if t.Location != project.Server || !strings.EqualFold(t.Termbase, name) {
			continue
		}
		a, err := s.accessTo(ctx, me, p.ID)
		if err != nil {
			return role, err
		}
		var via authdb.Role
		switch {
		case a.manage():
			via = authdb.RoleManage
		case a.edit() || a.Assigned:
			via = authdb.RoleEdit
		case a.read():
			via = authdb.RoleRead
		}
		if rank[via] > rank[role] {
			role = via
		}
	}
	return role, nil
}

// openTermbaseFor loads the termbase in the path if someone has the role
// on it. Someone who may not read it is told it doesn't exist.
func (s *Server) openTermbaseFor(w http.ResponseWriter, r *http.Request, me session, need authdb.Role) (*termdb.Termbase, authdb.Role, bool) {
	return s.termbaseFor(w, r, me, r.PathValue("name"), need)
}

func (s *Server) termbaseFor(w http.ResponseWriter, r *http.Request, me session, name string, need authdb.Role) (*termdb.Termbase, authdb.Role, bool) {
	tb, err := s.cfg.Termbases.Termbase(r.Context(), name)
	if err != nil {
		s.fail(w, r, err)
		return nil, "", false
	}
	role, err := s.termbaseRole(r.Context(), me, tb.Name())
	if err != nil {
		s.fail(w, r, err)
		return nil, "", false
	}
	if !role.Includes(authdb.RoleRead) {
		writeError(w, http.StatusNotFound, "notFound", "It doesn't exist, or it was deleted.")
		return nil, role, false
	}
	if !role.Includes(need) {
		forbidden(w, fmt.Sprintf("You may %s this termbase, not %s it.", role, need))
		return nil, role, false
	}
	return tb, role, true
}

type termbaseSummary struct {
	termdb.Summary
	Kind      string   `json:"kind"`
	Upstreams []string `json:"upstreams,omitempty"`
	Role      string   `json:"role"`
}

// listTermbases lists the termbases the person may read, with their kind,
// upstreams and the person's role.
//
// Access: signed in; each termbase needs read (directly or through a project
// that uses it).
func (s *Server) listTermbases(w http.ResponseWriter, r *http.Request, me session) {
	ctx := r.Context()
	tbs, err := s.cfg.Termbases.List(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := []termbaseSummary{}
	for _, t := range tbs {
		role, err := s.termbaseRole(ctx, me, t.Name)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if !role.Includes(authdb.RoleRead) {
			continue
		}
		tb, err := s.cfg.Termbases.Termbase(ctx, t.Name)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		info, err := tb.Info(ctx)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		out = append(out, termbaseSummary{Summary: t, Kind: info.Kind, Upstreams: info.Upstreams, Role: string(role)})
	}
	writeJSON(w, http.StatusOK, out)
}

type newTermbase struct {
	Definition term.Definition `json:"definition"`
	// Upstreams make it a project termbase layered over these masters.
	Upstreams []string `json:"upstreams,omitempty"`
}

// createTermbase makes a master termbase, or a project termbase layered over
// upstream masters (whose languages it gets if none are given). The person who
// makes it manages it.
//
// Access: those who manage termbases (manage on all termbases) and
// administrators; read on each upstream.
func (s *Server) createTermbase(w http.ResponseWriter, r *http.Request, me session) {
	ctx := r.Context()
	ok, err := s.mayCreate(ctx, me, authdb.KindTermbase)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !ok {
		forbidden(w, "Only managers of termbases make new ones.")
		return
	}
	var req newTermbase
	if !readJSON(w, r, &req, maxBody) {
		return
	}
	if strings.TrimSpace(req.Definition.Name) == "" {
		invalid(w, "definition.name", "Give the termbase a name.")
		return
	}
	var tb *termdb.Termbase
	if len(req.Upstreams) > 0 {
		for _, u := range req.Upstreams {
			if _, _, ok := s.termbaseFor(w, r, me, u, authdb.RoleRead); !ok {
				return
			}
		}
		if len(req.Definition.Languages) == 0 {
			cat, err := s.catalog(ctx)
			if err != nil {
				s.fail(w, r, err)
				return
			}
			req.Definition.Languages = mastersLanguages(cat, req.Upstreams)
		}
		tb, err = s.cfg.Termbases.CreateProject(ctx, req.Definition, req.Upstreams, me.by())
	} else {
		// A master may start without languages: they are added as terms
		// are, or by an import.
		tb, err = s.cfg.Termbases.CreateBy(ctx, req.Definition, me.by())
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !me.User.Admin {
		if err := s.cfg.Auth.SetGrant(ctx, authdb.Grant{UserID: me.User.ID, Kind: authdb.KindTermbase, Resource: tb.Name(), Role: authdb.RoleManage}, me.by()); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	s.writeTermbase(w, r, tb, authdb.RoleManage, http.StatusCreated)
}

type termbaseDetail struct {
	termdb.Info
	Definition term.Definition `json:"definition"`
	Entries    int             `json:"entries"`
	Terms      int             `json:"terms"`
	Role       string          `json:"role"`
}

func (s *Server) writeTermbase(w http.ResponseWriter, r *http.Request, tb *termdb.Termbase, role authdb.Role, status int) {
	ctx := r.Context()
	info, err := tb.Info(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	def, err := tb.Definition(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	es, ts, err := tb.Count(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, status, termbaseDetail{Info: info, Definition: def, Entries: es, Terms: ts, Role: string(role)})
}

// getTermbase returns a termbase's definition (languages, fields, picklists),
// size and the person's role.
//
// Access: read on the termbase.
func (s *Server) getTermbase(w http.ResponseWriter, r *http.Request, me session) {
	tb, role, ok := s.openTermbaseFor(w, r, me, authdb.RoleRead)
	if !ok {
		return
	}
	s.writeTermbase(w, r, tb, role, http.StatusOK)
}

func usesTermbase(name string) func(project.Settings) bool {
	return func(set project.Settings) bool {
		t := set.Terminology
		if strings.EqualFold(t.WriteTo, name) || (t.Location == project.Server && strings.EqualFold(t.Termbase, name)) {
			return true
		}
		for _, m := range t.Masters {
			if strings.EqualFold(m, name) {
				return true
			}
		}
		return false
	}
}

// deleteTermbase deletes a termbase and the grants on it. A termbase a project
// or template uses is not deleted; the answer names them.
//
// Access: manage on the termbase.
func (s *Server) deleteTermbase(w http.ResponseWriter, r *http.Request, me session) {
	tb, _, ok := s.openTermbaseFor(w, r, me, authdb.RoleManage)
	if !ok {
		return
	}
	name := tb.Name()
	users, err := s.usedBy(r.Context(), usesTermbase(name))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if len(users) > 0 {
		inUse(w, name, users)
		return
	}
	if err := s.cfg.Termbases.DeleteBy(r.Context(), name, me.by()); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.cfg.Auth.RevokeResource(r.Context(), authdb.KindTermbase, name, me.by()); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// renameTermbase renames a termbase, and the grants, projects and
// templates that refer to it.
//
// Access: manage on the termbase.
func (s *Server) renameTermbase(w http.ResponseWriter, r *http.Request, me session) {
	tb, role, ok := s.openTermbaseFor(w, r, me, authdb.RoleManage)
	if !ok {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if !readJSON(w, r, &req, maxBody) {
		return
	}
	ctx := r.Context()
	old, name := tb.Name(), strings.TrimSpace(req.Name)
	if name == "" {
		invalid(w, "name", "Give the termbase a name.")
		return
	}
	if err := tb.RenameBy(ctx, name, me.by()); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.cfg.Auth.RenameResource(ctx, authdb.KindTermbase, old, name, me.by()); err != nil {
		s.fail(w, r, err)
		return
	}
	rename := func(t project.Terminology) (project.Terminology, bool) {
		changed := false
		swap := func(x string) string {
			if strings.EqualFold(x, old) {
				changed = true
				return name
			}
			return x
		}
		for i := range t.Masters {
			t.Masters[i] = swap(t.Masters[i])
		}
		for i := range t.AddLanguages {
			t.AddLanguages[i] = swap(t.AddLanguages[i])
		}
		t.WriteTo = swap(t.WriteTo)
		if t.Location == project.Server {
			t.Termbase = swap(t.Termbase)
		}
		return t, changed
	}
	cat, err := s.catalog(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	ps, err := s.cfg.Projects.List(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	for _, p := range ps {
		set := p.Settings
		t, changed := rename(set.Terminology)
		if !changed {
			continue
		}
		set.Terminology = t
		if _, err := s.cfg.Projects.UpdateSettings(ctx, p.ID, set, p.Version, cat, me.by()); err != nil {
			s.fail(w, r, fmt.Errorf("renaming the termbase in project %s: %w", p.Settings.Name, err))
			return
		}
	}
	ts, err := s.cfg.Projects.Templates(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	for _, st := range ts {
		t, changed := rename(st.Template.Terminology)
		if !changed {
			continue
		}
		tpl := st.Template
		tpl.Terminology = t
		if _, err := s.cfg.Projects.SaveTemplate(ctx, tpl, st.Version, cat, me.by()); err != nil {
			s.fail(w, r, fmt.Errorf("renaming the termbase in template %s: %w", st.Name, err))
			return
		}
	}
	renamed, err := s.cfg.Termbases.Termbase(ctx, name)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeTermbase(w, r, renamed, role, http.StatusOK)
}

// setDefinition changes a termbase's languages, fields and picklists. Removing
// a language or field that entries use is refused: use the languages and
// fields routes for that.
//
// Access: manage on the termbase.
func (s *Server) setDefinition(w http.ResponseWriter, r *http.Request, me session) {
	tb, role, ok := s.openTermbaseFor(w, r, me, authdb.RoleManage)
	if !ok {
		return
	}
	var def term.Definition
	if !readJSON(w, r, &def, maxBody) {
		return
	}
	def.Name = tb.Name()
	if err := tb.SetDefinitionBy(r.Context(), def, me.by()); err != nil {
		if !s.termProblem(w, err) {
			s.fail(w, r, err)
		}
		return
	}
	s.writeTermbase(w, r, tb, role, http.StatusOK)
}

// setUpstreams sets the masters a project termbase is layered over.
//
// Access: manage on the termbase; read on each upstream.
func (s *Server) setUpstreams(w http.ResponseWriter, r *http.Request, me session) {
	tb, role, ok := s.openTermbaseFor(w, r, me, authdb.RoleManage)
	if !ok {
		return
	}
	var req struct {
		Upstreams []string `json:"upstreams"`
	}
	if !readJSON(w, r, &req, maxBody) {
		return
	}
	for _, u := range req.Upstreams {
		if _, _, ok := s.termbaseFor(w, r, me, u, authdb.RoleRead); !ok {
			return
		}
	}
	if err := tb.SetUpstreams(r.Context(), req.Upstreams, me.by()); err != nil {
		if !s.termProblem(w, err) {
			s.fail(w, r, err)
		}
		return
	}
	s.writeTermbase(w, r, tb, role, http.StatusOK)
}

// termProblem answers the store's refusals that are about the input
// rather than the server.
func (s *Server) termProblem(w http.ResponseWriter, err error) bool {
	switch {
	case errors.Is(err, termdb.ErrNotFound), errors.Is(err, termdb.ErrConflict), errors.Is(err, termdb.ErrExists),
		errors.Is(err, termdb.ErrNoTerms), errors.Is(err, termdb.ErrInUse), errors.Is(err, termdb.ErrNotProject),
		errors.Is(err, termdb.ErrClosed), errors.Is(err, termdb.ErrDecided), errors.Is(err, termdb.ErrNeedsResolution):
		return false
	}
	msg := strings.TrimPrefix(err.Error(), "termdb: ")
	if strings.HasPrefix(err.Error(), "termdb: ") {
		invalid(w, "termbase", strings.ToUpper(msg[:1])+msg[1:]+".")
		return true
	}
	return false
}

// renameField renames a field in the definition and in every entry, and
// answers how many entries changed.
//
// Access: manage on the termbase.
func (s *Server) renameField(w http.ResponseWriter, r *http.Request, me session) {
	tb, _, ok := s.openTermbaseFor(w, r, me, authdb.RoleManage)
	if !ok {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if !readJSON(w, r, &req, maxBody) {
		return
	}
	n, err := tb.RenameField(r.Context(), r.PathValue("field"), req.Name, me.by())
	if err != nil {
		if !s.termProblem(w, err) {
			s.fail(w, r, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"entriesChanged": n})
}

// removeLanguage removes a language from the definition and from every entry,
// and answers how many entries changed.
//
// Access: manage on the termbase.
func (s *Server) removeLanguage(w http.ResponseWriter, r *http.Request, me session) {
	tb, _, ok := s.openTermbaseFor(w, r, me, authdb.RoleManage)
	if !ok {
		return
	}
	n, err := tb.RemoveLanguage(r.Context(), r.PathValue("lang"), me.by())
	if err != nil {
		if !s.termProblem(w, err) {
			s.fail(w, r, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"entriesChanged": n})
}

// removeField removes a field from the definition and from every entry, and
// answers how many entries changed.
//
// Access: manage on the termbase.
func (s *Server) removeField(w http.ResponseWriter, r *http.Request, me session) {
	tb, _, ok := s.openTermbaseFor(w, r, me, authdb.RoleManage)
	if !ok {
		return
	}
	n, err := tb.RemoveField(r.Context(), r.PathValue("field"), me.by())
	if err != nil {
		if !s.termProblem(w, err) {
			s.fail(w, r, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"entriesChanged": n})
}

// searchEntries finds entries by text in a language (?text=&lang=) and term
// status (?status=), a page at a time (?limit=&offset=), with the number
// found.
//
// Access: read on the termbase.
func (s *Server) searchEntries(w http.ResponseWriter, r *http.Request, me session) {
	tb, _, ok := s.openTermbaseFor(w, r, me, authdb.RoleRead)
	if !ok {
		return
	}
	q := r.URL.Query()
	limit, ok := queryInt(w, r, "limit", 100, 1, 1000)
	if !ok {
		return
	}
	offset, ok := queryInt(w, r, "offset", 0, 0, 1<<30)
	if !ok {
		return
	}
	es, total, err := tb.Search(r.Context(), termdb.Query{Text: q.Get("text"), Lang: q.Get("lang"), Status: q.Get("status"), Limit: limit, Offset: offset})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Entries []termdb.Stored `json:"entries"`
		Total   int             `json:"total"`
	}{nonNil(es), total})
}

type entryWrite struct {
	Entry   term.Entry `json:"entry"`
	Summary string     `json:"summary"`
}

// addEntry adds an entry. Who made it and when is the server's to say.
//
// Access: edit on the termbase.
func (s *Server) addEntry(w http.ResponseWriter, r *http.Request, me session) {
	tb, _, ok := s.openTermbaseFor(w, r, me, authdb.RoleEdit)
	if !ok {
		return
	}
	var req entryWrite
	if !readJSON(w, r, &req, maxBody) {
		return
	}
	// Who made it and when is the server's to say.
	req.Entry.ID = ""
	req.Entry.CreatedAt, req.Entry.CreatedBy = time.Time{}, ""
	got, err := tb.Add(r.Context(), req.Entry, me.by(), req.Summary)
	if err != nil {
		if !s.termProblem(w, err) {
			s.fail(w, r, err)
		}
		return
	}
	writeJSON(w, http.StatusCreated, got)
}

// getEntry returns an entry with its version.
//
// Access: read on the termbase.
func (s *Server) getEntry(w http.ResponseWriter, r *http.Request, me session) {
	tb, _, ok := s.openTermbaseFor(w, r, me, authdb.RoleRead)
	if !ok {
		return
	}
	e, err := tb.Get(r.Context(), r.PathValue("eid"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, e)
}

// updateEntry replaces an entry. Send the version read (If-Match or ?version=).
//
// Access: edit on the termbase.
func (s *Server) updateEntry(w http.ResponseWriter, r *http.Request, me session) {
	tb, _, ok := s.openTermbaseFor(w, r, me, authdb.RoleEdit)
	if !ok {
		return
	}
	v, ok := version(w, r)
	if !ok {
		return
	}
	var req entryWrite
	if !readJSON(w, r, &req, maxBody) {
		return
	}
	req.Entry.ID = r.PathValue("eid")
	got, err := tb.Update(r.Context(), req.Entry, v, me.by(), req.Summary)
	if err != nil {
		if !s.termProblem(w, err) {
			s.fail(w, r, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, got)
}

// deleteEntry deletes an entry, with an optional ?summary= for its history.
// Deleted entries are listed at /deleted.
//
// Access: edit on the termbase.
func (s *Server) deleteEntry(w http.ResponseWriter, r *http.Request, me session) {
	tb, _, ok := s.openTermbaseFor(w, r, me, authdb.RoleEdit)
	if !ok {
		return
	}
	if err := tb.DeleteEntry(r.Context(), r.PathValue("eid"), me.by(), r.URL.Query().Get("summary")); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// entryHistory lists every version of an entry, oldest first.
//
// Access: read on the termbase.
func (s *Server) entryHistory(w http.ResponseWriter, r *http.Request, me session) {
	tb, _, ok := s.openTermbaseFor(w, r, me, authdb.RoleRead)
	if !ok {
		return
	}
	vs, err := tb.History(r.Context(), r.PathValue("eid"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, nonNil(vs))
}

// revertEntry sets an entry back to an earlier version, as a new version.
//
// Access: edit on the termbase.
func (s *Server) revertEntry(w http.ResponseWriter, r *http.Request, me session) {
	tb, _, ok := s.openTermbaseFor(w, r, me, authdb.RoleEdit)
	if !ok {
		return
	}
	var req struct {
		Version int `json:"version"`
	}
	if !readJSON(w, r, &req, maxBody) {
		return
	}
	got, err := tb.Revert(r.Context(), r.PathValue("eid"), req.Version, me.by())
	if err != nil {
		if !s.termProblem(w, err) {
			s.fail(w, r, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, got)
}

// mergeEntries merges entries into the one kept, and deletes the others.
//
// Access: edit on the termbase.
func (s *Server) mergeEntries(w http.ResponseWriter, r *http.Request, me session) {
	tb, _, ok := s.openTermbaseFor(w, r, me, authdb.RoleEdit)
	if !ok {
		return
	}
	var req struct {
		Keep   string   `json:"keep"`
		Others []string `json:"others"`
	}
	if !readJSON(w, r, &req, maxBody) {
		return
	}
	got, err := tb.Merge(r.Context(), req.Keep, req.Others, me.by())
	if err != nil {
		if !s.termProblem(w, err) {
			s.fail(w, r, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, got)
}

// duplicates lists groups of entry IDs that share a term in a language
// (?lang=), or in any language.
//
// Access: read on the termbase.
func (s *Server) duplicates(w http.ResponseWriter, r *http.Request, me session) {
	tb, _, ok := s.openTermbaseFor(w, r, me, authdb.RoleRead)
	if !ok {
		return
	}
	groups, err := tb.Duplicates(r.Context(), r.URL.Query().Get("lang"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, nonNil(groups))
}

// deletedEntries lists deleted entries.
//
// Access: read on the termbase.
func (s *Server) deletedEntries(w http.ResponseWriter, r *http.Request, me session) {
	tb, _, ok := s.openTermbaseFor(w, r, me, authdb.RoleRead)
	if !ok {
		return
	}
	ds, err := tb.Deleted(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, nonNil(ds))
}

// termLookup finds the entries that have a term (?text=), case and spacing
// aside, in a language or its variants (?lang=; empty for all).
//
// Access: read on the termbase.
func (s *Server) termLookup(w http.ResponseWriter, r *http.Request, me session) {
	tb, _, ok := s.openTermbaseFor(w, r, me, authdb.RoleRead)
	if !ok {
		return
	}
	q := r.URL.Query()
	es, err := tb.Lookup(r.Context(), q.Get("text"), q.Get("lang"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, nonNil(es))
}

// layeredEntries returns a project termbase's entries with its masters'
// entries layered under them: overrides replace, removals hide.
//
// Access: read on the termbase.
func (s *Server) layeredEntries(w http.ResponseWriter, r *http.Request, me session) {
	tb, _, ok := s.openTermbaseFor(w, r, me, authdb.RoleRead)
	if !ok {
		return
	}
	es, err := tb.Layered(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, nonNil(es))
}

type masterEntry struct {
	Master string `json:"master"`
	Entry  string `json:"entry"`
}

// overrideEntry copies a master entry into the project termbase, to
// change it there.
//
// Access: edit on the project termbase.
func (s *Server) overrideEntry(w http.ResponseWriter, r *http.Request, me session) {
	tb, _, ok := s.openTermbaseFor(w, r, me, authdb.RoleEdit)
	if !ok {
		return
	}
	var req masterEntry
	if !readJSON(w, r, &req, maxBody) {
		return
	}
	got, err := tb.Override(r.Context(), req.Master, req.Entry, me.by())
	if err != nil {
		if !s.termProblem(w, err) {
			s.fail(w, r, err)
		}
		return
	}
	writeJSON(w, http.StatusCreated, got)
}

// removeFromProject hides a master entry in the project; merging asks the
// master's reviewers to delete it there.
//
// Access: edit on the project termbase.
func (s *Server) removeFromProject(w http.ResponseWriter, r *http.Request, me session) {
	tb, _, ok := s.openTermbaseFor(w, r, me, authdb.RoleEdit)
	if !ok {
		return
	}
	var req masterEntry
	if !readJSON(w, r, &req, maxBody) {
		return
	}
	if err := tb.RemoveFromProject(r.Context(), req.Master, req.Entry, me.by()); err != nil {
		if !s.termProblem(w, err) {
			s.fail(w, r, err)
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// removals lists the master entries hidden in a project termbase.
//
// Access: read on the termbase.
func (s *Server) removals(w http.ResponseWriter, r *http.Request, me session) {
	tb, _, ok := s.openTermbaseFor(w, r, me, authdb.RoleRead)
	if !ok {
		return
	}
	rs, err := tb.Removals(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, nonNil(rs))
}

var termFormats = map[term.Format]struct{ ext, ct string }{
	term.FormatTBX:       {".tbx", "application/x-tbx+xml; charset=utf-8"},
	term.FormatTBX2:      {".tbx", "application/x-tbx+xml; charset=utf-8"},
	term.FormatMultiTerm: {".xml", "application/xml; charset=utf-8"},
	term.FormatCSV:       {".csv", "text/csv; charset=utf-8"},
	term.FormatTSV:       {".tsv", "text/tab-separated-values; charset=utf-8"},
	term.FormatXLSX:      {".xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
}

// exportTermbase sends the termbase as a file: ?format=tbx (the default),
// tbx2, multiterm, csv, tsv or xlsx.
//
// Access: read on the termbase.
func (s *Server) exportTermbase(w http.ResponseWriter, r *http.Request, me session) {
	s.sendTermbase(w, r, me, nil)
}

// exportEntries sends chosen entries of the termbase as a file, in the
// formats of the export: those a search shows, for one.
//
// Access: read on the termbase.
func (s *Server) exportEntries(w http.ResponseWriter, r *http.Request, me session) {
	var req struct {
		// Entries are the IDs of the entries to send.
		Entries []string `json:"entries"`
	}
	if !readJSON(w, r, &req, maxBody) {
		return
	}
	if len(req.Entries) == 0 {
		invalid(w, "entries", "Say which entries to export.")
		return
	}
	s.sendTermbase(w, r, me, req.Entries)
}

// sendTermbase writes the termbase, or the entries named, as a file.
func (s *Server) sendTermbase(w http.ResponseWriter, r *http.Request, me session, only []string) {
	tb, _, ok := s.openTermbaseFor(w, r, me, authdb.RoleRead)
	if !ok {
		return
	}
	f := term.Format(r.URL.Query().Get("format"))
	if f == "" {
		f = term.FormatTBX
	}
	kind, known := termFormats[f]
	if !known {
		invalid(w, "format", "The format is tbx, tbx2, multiterm, csv, tsv or xlsx.")
		return
	}
	full, err := tb.Export(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if only != nil {
		want := map[string]bool{}
		for _, id := range only {
			want[id] = true
		}
		var kept []term.Entry
		for _, e := range full.Entries {
			if want[e.ID] {
				kept = append(kept, e)
			}
		}
		full.Entries = kept
	}
	data, err := term.Write(full, f)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", kind.ct)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": tb.Name() + kind.ext}))
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Write(data)
}

// termFileInspection says what a termbase file holds.
type termFileInspection struct {
	Format term.Format `json:"format"`
	// Tabular is set for spreadsheets, whose columns need a mapping.
	Tabular bool `json:"tabular"`
	// Rows are the first rows of a spreadsheet, Total how many it has, and
	// Mapping what each column was read as: the guess, or the one sent.
	Rows    [][]string    `json:"rows,omitempty"`
	Total   int           `json:"total,omitempty"`
	Mapping *term.Mapping `json:"mapping,omitempty"`
	// Name is the termbase's name in the file, if it has one.
	Name      string       `json:"name,omitempty"`
	Read      term.Report  `json:"read"`
	Languages []string     `json:"languages"`
	Fields    []term.Field `json:"fields"`
	// Dropped counts the entries ?languages= leaves without a term, and
	// Sample names the first entries that stay, by their first terms.
	Dropped int      `json:"dropped,omitempty"`
	Sample  []string `json:"sample"`
}

// termLanguages reads ?languages= (a JSON object: which language each
// language of a file goes under, "" to leave one out) and applies it.
func termLanguages(w http.ResponseWriter, r *http.Request, in *term.Termbase) (dropped int, ok bool) {
	v := r.URL.Query().Get("languages")
	if v == "" {
		return 0, true
	}
	to := map[string]string{}
	if err := json.Unmarshal([]byte(v), &to); err != nil {
		invalid(w, "languages", "The languages are not a JSON object: "+err.Error())
		return 0, false
	}
	for from, code := range to {
		if code == "" {
			continue
		}
		norm, err := langtag.Normalize(code)
		if err != nil {
			invalid(w, "languages", fmt.Sprintf("%q is not a language tag.", code))
			return 0, false
		}
		to[from] = norm
	}
	in.Entries, dropped = term.Remap(in.Entries, to)
	in.Definition = term.InferDefinition(in.Name, in.Entries)
	return dropped, true
}

// termFilePreviewRows is how many rows of a spreadsheet an inspection shows.
const termFilePreviewRows = 12

// termMapping reads ?mapping= (a column mapping as JSON) for spreadsheets.
func termMapping(w http.ResponseWriter, r *http.Request) (*term.Mapping, bool) {
	v := r.URL.Query().Get("mapping")
	if v == "" {
		return nil, true
	}
	m := new(term.Mapping)
	if err := json.Unmarshal([]byte(v), m); err != nil {
		invalid(w, "mapping", "The column mapping is not valid JSON: "+err.Error())
		return nil, false
	}
	return m, true
}

// inspectTermFile reads a termbase file (the request body, named by
// ?filename=) and says what it holds: its format, entries, terms, languages
// and fields, and for spreadsheets the first rows with what each column is
// taken for. ?mapping= (JSON) reads a spreadsheet with a column mapping of
// your own, and ?languages= (a JSON object) says which language each of
// the file's goes under, as for an import. Nothing is kept.
//
// Access: signed in.
func (s *Server) inspectTermFile(w http.ResponseWriter, r *http.Request, me session) {
	q := r.URL.Query()
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxFileBody))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "tooLarge", fmt.Sprintf("The file is larger than %d MB.", maxFileBody>>20))
		return
	}
	f := term.Format(q.Get("format"))
	if f == "" {
		if f, err = term.Detect(q.Get("filename"), data); err != nil {
			invalid(w, "file", err.Error())
			return
		}
	}
	m, ok := termMapping(w, r)
	if !ok {
		return
	}
	out := termFileInspection{Format: f, Tabular: f.IsTabular(), Languages: []string{}, Fields: []term.Field{}, Sample: []string{}}
	if out.Tabular {
		rows, err := term.Rows(f, data)
		if err != nil {
			invalid(w, "file", "The file could not be read: "+err.Error())
			return
		}
		if m == nil {
			g := term.GuessMapping(rows)
			m = &g
		}
		out.Mapping, out.Total = m, len(rows)
		if len(rows) > termFilePreviewRows {
			rows = rows[:termFilePreviewRows]
		}
		out.Rows = rows
	}
	in, rep, err := term.Read(f, data, m)
	if err != nil {
		// A spreadsheet whose columns were guessed wrong is still shown,
		// so the mapping can be set right.
		if out.Tabular && q.Get("mapping") == "" {
			out.Read.Warnings = []string{err.Error()}
			writeJSON(w, http.StatusOK, out)
			return
		}
		invalid(w, "file", "The file could not be read: "+err.Error())
		return
	}
	out.Name, out.Read = in.Name, rep
	// The languages listed are the file's own, whatever they are mapped to.
	out.Languages = nonNil(in.FileLanguages())
	if out.Dropped, ok = termLanguages(w, r, in); !ok {
		return
	}
	out.Fields = nonNil(in.Fields)
	for _, e := range in.Entries {
		if len(out.Sample) == importPlanRows {
			break
		}
		out.Sample = append(out.Sample, e.Label())
	}
	writeJSON(w, http.StatusOK, out)
}

// plannedEntry is a line of an import's plan.
type plannedEntry struct {
	// Kind is add, merge, overwrite or skip.
	Kind string `json:"kind"`
	// Entry names the file's entry by its first terms, Existing the entry
	// it meets in the termbase.
	Entry    string `json:"entry"`
	Existing string `json:"existing,omitempty"`
}

// importPlanRows is how many lines of the plan a dry run lists.
const importPlanRows = 40

// importTermbase reads a termbase file (the request body: TBX, MultiTerm
// XML, a MultiTerm termbase file (.sdltb), CSV, TSV or Excel) into the
// termbase. ?match=term|id|none and ?onMatch=merge|overwrite|skip|add say
// what happens to entries that are there already; ?dryRun=1 reports
// without changing anything, with the first lines of what would happen.
// ?mapping= (JSON) says what a spreadsheet's columns hold, ?languages= (a
// JSON object) which termbase language each language of the file goes
// under, with "" to leave one out, and ?status=approved|proposed which
// status terms without one get.
//
// Access: edit on the termbase.
func (s *Server) importTermbase(w http.ResponseWriter, r *http.Request, me session) {
	tb, _, ok := s.openTermbaseFor(w, r, me, authdb.RoleEdit)
	if !ok {
		return
	}
	q := r.URL.Query()
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxFileBody))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "tooLarge", fmt.Sprintf("The file is larger than %d MB.", maxFileBody>>20))
		return
	}
	name := q.Get("filename")
	f := term.Format(q.Get("format"))
	if f == "" {
		if f, err = term.Detect(name, data); err != nil {
			invalid(w, "file", err.Error())
			return
		}
	}
	m, ok := termMapping(w, r)
	if !ok {
		return
	}
	in, rep, err := term.Read(f, data, m)
	if err != nil {
		invalid(w, "file", "The file could not be read: "+err.Error())
		return
	}
	dropped, ok := termLanguages(w, r, in)
	if !ok {
		return
	}
	switch st := q.Get("status"); st {
	case "":
	case term.StatusApproved, term.StatusProposed:
		term.SetDefaultStatus(in.Entries, st)
	default:
		invalid(w, "status", "status is approved or proposed.")
		return
	}
	p := term.Policy{Match: term.Match(q.Get("match")), Lang: q.Get("lang"), OnMatch: term.OnMatch(q.Get("onMatch"))}
	if p.Match == "" {
		p.Match = term.MatchTerm
	}
	if p.OnMatch == "" {
		p.OnMatch = term.Merge
	}
	switch p.Match {
	case term.MatchNone, term.MatchTerm, term.MatchID:
	default:
		invalid(w, "match", "match is term, id or none.")
		return
	}
	switch p.OnMatch {
	case term.Merge, term.Overwrite, term.Skip, term.AddNew:
	default:
		invalid(w, "onMatch", "onMatch is merge, overwrite, skip or add.")
		return
	}
	type result struct {
		termdb.ImportReport
		Read     term.Report `json:"read"`
		Actions  int         `json:"actions,omitempty"`
		Imported bool        `json:"imported"`
		// Dropped counts entries left without a term by ?languages=.
		Dropped int `json:"dropped,omitempty"`
		// Plan lists the first lines of what a dry run would do.
		Plan []plannedEntry `json:"plan,omitempty"`
	}
	if q.Get("dryRun") == "1" {
		acts, ir, err := tb.Plan(r.Context(), in, p)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		var plan []plannedEntry
		for _, a := range acts {
			if len(plan) == importPlanRows {
				break
			}
			line := plannedEntry{Kind: a.Kind}
			if a.Incoming >= 0 && a.Incoming < len(in.Entries) {
				line.Entry = in.Entries[a.Incoming].Label()
			}
			if a.Existing != "" {
				line.Existing = a.Existing
				if old, err := tb.Get(r.Context(), a.Existing); err == nil {
					if l := old.Entry.Label(); l != "" {
						line.Existing = l
					}
				}
			}
			plan = append(plan, line)
		}
		writeJSON(w, http.StatusOK, result{ImportReport: ir, Read: rep, Actions: len(acts), Dropped: dropped, Plan: plan})
		return
	}
	source := name
	if source == "" {
		source = string(f) + " import"
	}
	ir, err := tb.Import(r.Context(), in, p, me.by(), source)
	if err != nil {
		if !s.termProblem(w, err) {
			s.fail(w, r, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, result{ImportReport: ir, Read: rep, Imported: true, Dropped: dropped})
}

// termbaseChanges lists the termbase's changes after ?since= (a change
// number), oldest first, at most ?limit=.
//
// Access: read on the termbase.
func (s *Server) termbaseChanges(w http.ResponseWriter, r *http.Request, me session) {
	tb, _, ok := s.openTermbaseFor(w, r, me, authdb.RoleRead)
	if !ok {
		return
	}
	since, ok := parseSince(w, r)
	if !ok {
		return
	}
	limit, ok := queryInt(w, r, "limit", 1000, 1, 10000)
	if !ok {
		return
	}
	cs, more, err := s.cfg.Termbases.Changes(r.Context(), termdb.ChangeQuery{Since: since, Termbases: []string{tb.Name()}, Limit: limit})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Changes []termdb.Change `json:"changes"`
		More    bool            `json:"more"`
	}{nonNil(cs), more})
}

// reviewRules returns how merge requests into the termbase are reviewed.
//
// Access: read on the termbase.
func (s *Server) reviewRules(w http.ResponseWriter, r *http.Request, me session) {
	tb, _, ok := s.openTermbaseFor(w, r, me, authdb.RoleRead)
	if !ok {
		return
	}
	rr, err := tb.ReviewRules(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, rr)
}

// setReviewRules sets how merge requests into the termbase are reviewed.
//
// Access: manage on the termbase.
func (s *Server) setReviewRules(w http.ResponseWriter, r *http.Request, me session) {
	tb, _, ok := s.openTermbaseFor(w, r, me, authdb.RoleManage)
	if !ok {
		return
	}
	var rr termdb.ReviewRules
	if !readJSON(w, r, &rr, maxBody) {
		return
	}
	if err := tb.SetReviewRules(r.Context(), rr, me.by()); err != nil {
		if !s.termProblem(w, err) {
			s.fail(w, r, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, rr)
}

// mergeRequests lists a project termbase's merge requests, or a master's
// incoming ones, newest first.
//
// Access: read on the termbase.
func (s *Server) mergeRequests(w http.ResponseWriter, r *http.Request, me session) {
	tb, _, ok := s.openTermbaseFor(w, r, me, authdb.RoleRead)
	if !ok {
		return
	}
	mrs, err := tb.MergeRequests(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, nonNil(mrs))
}

// openMergeRequest submits a project termbase's changes to one of its
// masters for review. It takes editing the project termbase and
// suggesting in the master.
//
// Access: edit on the project termbase and suggest on the master.
func (s *Server) openMergeRequest(w http.ResponseWriter, r *http.Request, me session) {
	tb, _, ok := s.openTermbaseFor(w, r, me, authdb.RoleEdit)
	if !ok {
		return
	}
	var req struct {
		Master   string    `json:"master"`
		Title    string    `json:"title"`
		Deadline time.Time `json:"deadline,omitzero"`
	}
	if !readJSON(w, r, &req, maxBody) {
		return
	}
	if _, _, ok := s.termbaseFor(w, r, me, req.Master, authdb.RoleSuggest); !ok {
		return
	}
	mr, err := tb.OpenMergeRequest(r.Context(), req.Master, req.Title, me.by(), req.Deadline)
	if err != nil {
		if !s.termProblem(w, err) {
			s.fail(w, r, err)
		}
		return
	}
	writeJSON(w, http.StatusCreated, mr)
}

// openMR loads a merge request someone may see (they read its master or
// its project termbase) with their role on its master.
func (s *Server) openMR(w http.ResponseWriter, r *http.Request, me session, need authdb.Role) (*termdb.MergeRequest, bool) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return nil, false
	}
	mr, err := s.cfg.Termbases.MergeRequest(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return nil, false
	}
	onMaster, err := s.termbaseRole(r.Context(), me, mr.Target)
	if err != nil {
		s.fail(w, r, err)
		return nil, false
	}
	onProject, err := s.termbaseRole(r.Context(), me, mr.Project)
	if err != nil {
		s.fail(w, r, err)
		return nil, false
	}
	if !onMaster.Includes(authdb.RoleRead) && !onProject.Includes(authdb.RoleRead) {
		writeError(w, http.StatusNotFound, "notFound", "It doesn't exist, or it was deleted.")
		return nil, false
	}
	if need != authdb.RoleRead && !onMaster.Includes(need) {
		forbidden(w, fmt.Sprintf("Only those who may %s in %s do this.", need, mr.Target))
		return nil, false
	}
	return mr, true
}

// getMergeRequest returns a merge request.
//
// Access: read on its master.
func (s *Server) getMergeRequest(w http.ResponseWriter, r *http.Request, me session) {
	mr, ok := s.openMR(w, r, me, authdb.RoleRead)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, mr)
}

type itemQuery struct {
	Filter    termdb.Filter  `json:"filter"`
	Reviewer  string         `json:"reviewer,omitempty"`
	Sort      string         `json:"sort,omitempty"`
	Frequency map[string]int `json:"frequency,omitempty"`
	Limit     int            `json:"limit,omitempty"`
	Offset    int            `json:"offset,omitempty"`
}

func (s *Server) writeItems(w http.ResponseWriter, r *http.Request, mr *termdb.MergeRequest, q itemQuery) {
	switch q.Sort {
	case "", "priority", "date", "author":
	default:
		invalid(w, "sort", "sort is priority, date or author.")
		return
	}
	if q.Limit <= 0 || q.Limit > 1000 {
		q.Limit = 100
	}
	items, total, err := mr.Items(r.Context(), termdb.ItemQuery{Filter: q.Filter, Reviewer: q.Reviewer, Sort: q.Sort, Frequency: q.Frequency, Limit: q.Limit, Offset: q.Offset})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Items []termdb.Item `json:"items"`
		Total int           `json:"total"`
	}{nonNil(items), total})
}

// mergeItems lists the items with a simple query: ?state=, ?reviewer=,
// ?sort=, ?limit=, ?offset=.
//
// Access: read on the master.
func (s *Server) mergeItems(w http.ResponseWriter, r *http.Request, me session) {
	mr, ok := s.openMR(w, r, me, authdb.RoleRead)
	if !ok {
		return
	}
	q := r.URL.Query()
	iq := itemQuery{Reviewer: q.Get("reviewer"), Sort: q.Get("sort")}
	if st := q.Get("state"); st != "" {
		iq.Filter.States = strings.Split(st, ",")
	}
	var lok bool
	if iq.Limit, lok = queryInt(w, r, "limit", 100, 1, 1000); !lok {
		return
	}
	if iq.Offset, lok = queryInt(w, r, "offset", 0, 0, 1<<30); !lok {
		return
	}
	s.writeItems(w, r, mr, iq)
}

// queryMergeItems lists the items a full query (filter, frequencies)
// selects.
//
// Access: read on the master.
func (s *Server) queryMergeItems(w http.ResponseWriter, r *http.Request, me session) {
	mr, ok := s.openMR(w, r, me, authdb.RoleRead)
	if !ok {
		return
	}
	var q itemQuery
	if !readJSON(w, r, &q, maxBody) {
		return
	}
	s.writeItems(w, r, mr, q)
}

// sampleMergeItems picks up to n waiting items at random from those the
// filter selects, for a spot check of a large request. The same seed gives
// the same sample.
//
// Access: read on the master.
func (s *Server) sampleMergeItems(w http.ResponseWriter, r *http.Request, me session) {
	mr, ok := s.openMR(w, r, me, authdb.RoleRead)
	if !ok {
		return
	}
	var req struct {
		Filter termdb.Filter `json:"filter"`
		N      int           `json:"n"`
		Seed   int64         `json:"seed"`
	}
	if !readJSON(w, r, &req, maxBody) {
		return
	}
	if req.N <= 0 || req.N > 1000 {
		invalid(w, "n", "Take a sample of 1 to 1000 entries.")
		return
	}
	items, err := mr.Sample(r.Context(), req.Filter, req.N, req.Seed)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, nonNil(items))
}

// mergeProgress counts a merge request's items by state and by reviewer.
//
// Access: read on the master.
func (s *Server) mergeProgress(w http.ResponseWriter, r *http.Request, me session) {
	mr, ok := s.openMR(w, r, me, authdb.RoleRead)
	if !ok {
		return
	}
	p, err := mr.Progress(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// decide records a decision on the items a filter selects: approved,
// approved-with-edits, changes-requested, rejected or postponed.
//
// Access: approve on the master.
func (s *Server) decide(w http.ResponseWriter, r *http.Request, me session) {
	mr, ok := s.openMR(w, r, me, authdb.RoleApprove)
	if !ok {
		return
	}
	var req struct {
		Filter   termdb.Filter   `json:"filter"`
		Decision termdb.Decision `json:"decision"`
	}
	if !readJSON(w, r, &req, maxBody) {
		return
	}
	switch req.Decision.State {
	case termdb.StateApproved, termdb.StateEdited, termdb.StateChanges, termdb.StateRejected, termdb.StatePostponed:
	default:
		invalid(w, "decision.state", "The decision is approved, approved-with-edits, changes-requested, rejected or postponed.")
		return
	}
	res, err := mr.Decide(r.Context(), req.Filter, req.Decision, me.by())
	if err != nil {
		if !s.termProblem(w, err) {
			s.fail(w, r, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// mergeReviewers lists the reviewers given part of a merge request.
//
// Access: read on the master.
func (s *Server) mergeReviewers(w http.ResponseWriter, r *http.Request, me session) {
	mr, ok := s.openMR(w, r, me, authdb.RoleRead)
	if !ok {
		return
	}
	as, err := mr.Assignments(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, nonNil(as))
}

// assignReviewer gives a reviewer (a username) part of the request. The
// reviewer must be able to approve in the master.
//
// Access: manage on the master.
func (s *Server) assignReviewer(w http.ResponseWriter, r *http.Request, me session) {
	mr, ok := s.openMR(w, r, me, authdb.RoleManage)
	if !ok {
		return
	}
	var req termdb.Assignment
	if !readJSON(w, r, &req, maxBody) {
		return
	}
	u, err := s.cfg.Auth.UserByName(r.Context(), req.Reviewer)
	if errors.Is(err, authdb.ErrNotFound) {
		invalid(w, "reviewer", "There is no account "+req.Reviewer+".")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	can, err := s.termbaseRole(r.Context(), session{User: u}, mr.Target)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !can.Includes(authdb.RoleApprove) {
		invalid(w, "reviewer", fmt.Sprintf("%s may not approve changes to %s. Give them the approve role first.", u.Name, mr.Target))
		return
	}
	if err := mr.Assign(r.Context(), u.Username, req.Scope); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// unassignReviewer takes a reviewer (?reviewer=, a username) off a merge
// request.
//
// Access: manage on the master.
func (s *Server) unassignReviewer(w http.ResponseWriter, r *http.Request, me session) {
	mr, ok := s.openMR(w, r, me, authdb.RoleManage)
	if !ok {
		return
	}
	if err := mr.Unassign(r.Context(), r.URL.Query().Get("reviewer")); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// closeMergeRequest closes a merge request; items not decided come back in the
// next one.
//
// Access: approve on the master.
func (s *Server) closeMergeRequest(w http.ResponseWriter, r *http.Request, me session) {
	mr, ok := s.openMR(w, r, me, authdb.RoleApprove)
	if !ok {
		return
	}
	if err := mr.Close(r.Context(), me.by()); err != nil {
		if !s.termProblem(w, err) {
			s.fail(w, r, err)
		}
		return
	}
	got, err := s.cfg.Termbases.MergeRequest(r.Context(), mr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, got)
}

// revertMergeItem undoes the decision on an item of an open merge request.
//
// Access: approve on the master.
func (s *Server) revertMergeItem(w http.ResponseWriter, r *http.Request, me session) {
	mr, ok := s.openMR(w, r, me, authdb.RoleApprove)
	if !ok {
		return
	}
	iid, ok := pathID(w, r, "iid")
	if !ok {
		return
	}
	if err := mr.Revert(r.Context(), iid, me.by()); err != nil {
		if !s.termProblem(w, err) {
			s.fail(w, r, err)
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
