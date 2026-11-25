package server

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/4nonX/interlinea/auth/authdb"
	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/project"
	"github.com/4nonX/interlinea/tm"
	"github.com/4nonX/interlinea/tm/tmdb"
)

func (s *Server) memoryRoutes() {
	s.handle("GET /api/memories", s.signedIn(s.listMemories))
	s.handle("POST /api/memories", s.signedIn(s.createMemory))
	s.handle("GET /api/memories/{name}", s.signedIn(s.getMemory))
	s.handle("DELETE /api/memories/{name}", s.signedIn(s.deleteMemory))
	s.handle("POST /api/memories/{name}/lookup", s.signedIn(s.memoryLookup))
	s.handle("POST /api/memories/{name}/concordance", s.signedIn(s.memoryConcordance))
	s.handle("POST /api/memories/{name}/units", s.signedIn(s.addUnit))
	s.handle("GET /api/memories/{name}/units/{uid}", s.signedIn(s.getUnit))
	s.handle("PUT /api/memories/{name}/units/{uid}", s.signedIn(s.updateUnit))
	s.handle("DELETE /api/memories/{name}/units/{uid}", s.signedIn(s.deleteUnit))
	s.handle("POST /api/memories/{name}/import", s.signedIn(s.importMemoryFile))
	s.handle("POST /api/memoryfiles/inspect", s.signedIn(s.inspectMemoryFile))
	s.handle("GET /api/memories/{name}/export", s.signedIn(s.exportTMX))
	s.handle("GET /api/memories/{name}/changes", s.signedIn(s.memoryChanges))
}

type memoryView struct {
	Name       string `json:"name"`
	SourceLang string `json:"sourceLang"`
	TargetLang string `json:"targetLang"`
	// Client is who the memory is kept for, if anyone.
	Client    string    `json:"client,omitempty"`
	Units     int       `json:"units"`
	CreatedAt time.Time `json:"createdAt"`
	Role      string    `json:"role"`
}

// listMemories lists the memories the person may read, with their role on each.
//
// Access: signed in; each memory needs read.
func (s *Server) listMemories(w http.ResponseWriter, r *http.Request, me session) {
	ctx := r.Context()
	ms, err := s.cfg.Memories.List(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := []memoryView{}
	for _, m := range ms {
		role, err := s.cfg.Auth.RoleOn(ctx, me.User, authdb.KindMemory, m.Name)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if role.Includes(authdb.RoleRead) {
			out = append(out, memoryView{Name: m.Name, SourceLang: m.SourceLang, TargetLang: m.TargetLang, Client: m.Client, Units: m.Units, CreatedAt: m.CreatedAt, Role: string(role)})
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// createMemory makes a memory for one language pair. The person who makes it
// manages it.
//
// Access: those who manage memories (a manage grant on all memories) and
// administrators.
func (s *Server) createMemory(w http.ResponseWriter, r *http.Request, me session) {
	ctx := r.Context()
	ok, err := s.mayCreate(ctx, me, authdb.KindMemory)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !ok {
		forbidden(w, "Only managers of memories make new ones.")
		return
	}
	var req struct {
		Name       string `json:"name"`
		SourceLang string `json:"sourceLang"`
		TargetLang string `json:"targetLang"`
		// Client is who the memory is kept for; it may be left out.
		Client string `json:"client"`
	}
	if !readJSON(w, r, &req, maxBody) {
		return
	}
	info, err := tm.ValidateInfo(tm.Info{Name: req.Name, SourceLang: req.SourceLang, TargetLang: req.TargetLang, Client: req.Client})
	if err != nil {
		invalid(w, "name", "A memory needs a name and its source and target languages.")
		return
	}
	m, err := s.cfg.Memories.CreateBy(ctx, info, me.by())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !me.User.Admin {
		if err := s.cfg.Auth.SetGrant(ctx, authdb.Grant{UserID: me.User.ID, Kind: authdb.KindMemory, Resource: m.Info().Name, Role: authdb.RoleManage}, me.by()); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	writeJSON(w, http.StatusCreated, memoryView{Name: m.Info().Name, SourceLang: m.Info().SourceLang, TargetLang: m.Info().TargetLang, Client: m.Info().Client, CreatedAt: s.cfg.Now().UTC(), Role: string(authdb.RoleManage)})
}

// openMemoryFor loads the memory in the path if someone has the role on
// it. Someone who may not read it is told it doesn't exist.
func (s *Server) openMemoryFor(w http.ResponseWriter, r *http.Request, me session, need authdb.Role) (*tmdb.Memory, authdb.Role, bool) {
	m, err := s.cfg.Memories.Memory(r.Context(), r.PathValue("name"))
	if err != nil {
		s.fail(w, r, err)
		return nil, "", false
	}
	role, err := s.cfg.Auth.RoleOn(r.Context(), me.User, authdb.KindMemory, m.Info().Name)
	if err != nil {
		s.fail(w, r, err)
		return nil, "", false
	}
	if !role.Includes(authdb.RoleRead) {
		writeError(w, http.StatusNotFound, "notFound", "It doesn't exist, or it was deleted.")
		return nil, role, false
	}
	if !role.Includes(need) {
		forbidden(w, fmt.Sprintf("You may %s this memory, not %s it.", role, need))
		return nil, role, false
	}
	return m, role, true
}

// getMemory returns a memory's languages, size and the person's role on it.
//
// Access: read on the memory.
func (s *Server) getMemory(w http.ResponseWriter, r *http.Request, me session) {
	m, role, ok := s.openMemoryFor(w, r, me, authdb.RoleRead)
	if !ok {
		return
	}
	n, err := m.Count(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	info := m.Info()
	ms, err := s.cfg.Memories.List(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	v := memoryView{Name: info.Name, SourceLang: info.SourceLang, TargetLang: info.TargetLang, Client: info.Client, Units: n, Role: string(role)}
	for _, x := range ms {
		if x.Name == info.Name {
			v.CreatedAt = x.CreatedAt
		}
	}
	writeJSON(w, http.StatusOK, v)
}

// usedBy lists the projects and templates whose settings use a memory or
// termbase, so it isn't deleted under them.
func (s *Server) usedBy(ctx context.Context, uses func(project.Settings) bool) ([]string, error) {
	ps, err := s.cfg.Projects.List(ctx)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, p := range ps {
		if uses(p.Settings) {
			out = append(out, p.Settings.Name)
		}
	}
	ts, err := s.cfg.Projects.Templates(ctx)
	if err != nil {
		return nil, err
	}
	for _, t := range ts {
		if uses(t.Settings()) {
			out = append(out, "template "+t.Name)
		}
	}
	return out, nil
}

func inUse(w http.ResponseWriter, what string, users []string) {
	writeError(w, http.StatusConflict, "inUse", fmt.Sprintf("%s is used by %s. Take it out of their settings first.", what, strings.Join(users, ", ")))
}

// deleteMemory deletes a memory and the grants on it. A memory a project or
// template uses is not deleted; the answer names them.
//
// Access: manage on the memory.
func (s *Server) deleteMemory(w http.ResponseWriter, r *http.Request, me session) {
	m, _, ok := s.openMemoryFor(w, r, me, authdb.RoleManage)
	if !ok {
		return
	}
	name := m.Info().Name
	users, err := s.usedBy(r.Context(), func(set project.Settings) bool {
		for _, ms := range set.Memories {
			for _, x := range ms {
				if strings.EqualFold(x.Memory, name) {
					return true
				}
			}
		}
		return false
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if len(users) > 0 {
		inUse(w, name, users)
		return
	}
	if err := s.cfg.Memories.DeleteBy(r.Context(), name, me.by()); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.cfg.Auth.RevokeResource(r.Context(), authdb.KindMemory, name, me.by()); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// memoryLookup finds the memory's matches for a segment, best first.
//
// Access: read on the memory.
func (s *Server) memoryLookup(w http.ResponseWriter, r *http.Request, me session) {
	m, _, ok := s.openMemoryFor(w, r, me, authdb.RoleRead)
	if !ok {
		return
	}
	var req lookupRequest
	if !readJSON(w, r, &req, maxBody) {
		return
	}
	out, err := lookup(r.Context(), []tm.Memory{m}, req)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// memoryConcordance searches the memory's sources (or targets) for a word or
// phrase.
//
// Access: read on the memory.
func (s *Server) memoryConcordance(w http.ResponseWriter, r *http.Request, me session) {
	m, _, ok := s.openMemoryFor(w, r, me, authdb.RoleRead)
	if !ok {
		return
	}
	var req concordanceRequest
	if !readJSON(w, r, &req, maxBody) {
		return
	}
	out, err := concordance(r.Context(), []tm.Memory{m}, req)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// unitInput is a unit as the apps send it.
type unitInput struct {
	Source     content.Content   `json:"source"`
	Target     content.Content   `json:"target"`
	Attributes map[string]string `json:"attributes,omitempty"`
	Notes      []string          `json:"notes,omitempty"`
}

func (u unitInput) unit(by string) tm.Unit {
	out := tm.Unit{Source: u.Source, Target: u.Target, CreatedBy: by, ChangedBy: by, Notes: u.Notes}
	for k, v := range u.Attributes {
		out.Attributes = append(out.Attributes, tm.Attribute{Name: k, Value: v})
	}
	return out
}

func unitProblem(w http.ResponseWriter, err error) bool {
	switch err {
	case tm.ErrEmptySource:
		invalid(w, "source", "The source is empty.")
	case tm.ErrEmptyTarget:
		invalid(w, "target", "The translation is empty.")
	default:
		return false
	}
	return true
}

// addUnit adds a translation unit. ?policy=keep (the default) keeps other
// translations of the same source; ?policy=overwrite replaces them.
//
// Access: edit on the memory.
func (s *Server) addUnit(w http.ResponseWriter, r *http.Request, me session) {
	m, _, ok := s.openMemoryFor(w, r, me, authdb.RoleEdit)
	if !ok {
		return
	}
	policy := tm.KeepBoth
	switch r.URL.Query().Get("policy") {
	case "", "keep":
	case "overwrite":
		policy = tm.Overwrite
	default:
		invalid(w, "policy", "The policy is keep (another translation of the same source is kept) or overwrite.")
		return
	}
	var req unitInput
	if !readJSON(w, r, &req, maxBody) {
		return
	}
	u, err := m.Add(r.Context(), req.unit(me.by()), policy)
	if err != nil {
		if !unitProblem(w, err) {
			s.fail(w, r, err)
		}
		return
	}
	writeJSON(w, http.StatusCreated, viewUnit(u))
}

func unitID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	return pathID(w, r, "uid")
}

// getUnit returns a translation unit.
//
// Access: read on the memory.
func (s *Server) getUnit(w http.ResponseWriter, r *http.Request, me session) {
	m, _, ok := s.openMemoryFor(w, r, me, authdb.RoleRead)
	if !ok {
		return
	}
	id, ok := unitID(w, r)
	if !ok {
		return
	}
	u, err := m.Unit(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, viewUnit(u))
}

// updateUnit changes a translation unit. Send the version read (If-Match or
// ?version=).
//
// Access: edit on the memory.
func (s *Server) updateUnit(w http.ResponseWriter, r *http.Request, me session) {
	m, _, ok := s.openMemoryFor(w, r, me, authdb.RoleEdit)
	if !ok {
		return
	}
	id, ok := unitID(w, r)
	if !ok {
		return
	}
	v, ok := version(w, r)
	if !ok {
		return
	}
	var req unitInput
	if !readJSON(w, r, &req, maxBody) {
		return
	}
	u := req.unit(me.by())
	u.ID = id
	got, err := m.Update(r.Context(), u, v, me.by())
	if err != nil {
		if !unitProblem(w, err) {
			s.fail(w, r, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, viewUnit(got))
}

// deleteUnit deletes a translation unit.
//
// Access: edit on the memory.
func (s *Server) deleteUnit(w http.ResponseWriter, r *http.Request, me session) {
	m, _, ok := s.openMemoryFor(w, r, me, authdb.RoleEdit)
	if !ok {
		return
	}
	id, ok := unitID(w, r)
	if !ok {
		return
	}
	if err := m.DeleteUnit(r.Context(), id, me.by()); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// importMemoryFile reads a memory file (the request body) into a memory:
// TMX, a Trados Studio memory (.sdltm) or a Wordfast memory (.txt), told
// by its content. ?policy=overwrite replaces translations of sources that
// are there; ?variants=exact takes only the memory's exact languages, not
// their variants (en-US for en-GB). Units in the memory's languages the
// other way round are read the other way round.
//
// Access: edit on the memory.
func (s *Server) importMemoryFile(w http.ResponseWriter, r *http.Request, me session) {
	m, _, ok := s.openMemoryFor(w, r, me, authdb.RoleEdit)
	if !ok {
		return
	}
	opts := tm.ImportOptions{AllowPrimaryMatch: r.URL.Query().Get("variants") != "exact"}
	if r.URL.Query().Get("policy") == "overwrite" {
		opts.Policy = tm.Overwrite
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxFileBody))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "tooLarge", fmt.Sprintf("The file is larger than %d MB.", maxFileBody>>20))
		return
	}
	format := tm.FormatOf(data)
	var st tm.ImportStats
	var note string
	if format == tm.FormatTMX {
		st, err = tm.ImportTMX(r.Context(), m, bytes.NewReader(data), opts)
	} else {
		var f *tm.File
		if f, err = tm.ReadFile(data); err == nil {
			st, err = tm.ImportUnits(r.Context(), m, f, opts)
			if st.Stored == 0 {
				note = tm.LanguagesNote(f, m.Info())
			}
		}
	}
	if err != nil {
		invalid(w, "file", "The file could not be read as a "+tm.FormatName(format)+": "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Format  string `json:"format"`
		Read    int    `json:"read"`
		Stored  int    `json:"stored"`
		Skipped int    `json:"skipped"`
		// Note says why nothing was stored, when the file is for other
		// languages.
		Note string `json:"note,omitempty"`
	}{format, st.Read, st.Stored, st.Skipped, note})
}

// inspectMemoryFile reads a memory file (the request body) without
// storing it: its format, the name and languages it gives and how many
// units it holds, for making a memory from it. Nothing is kept.
//
// Access: signed in.
func (s *Server) inspectMemoryFile(w http.ResponseWriter, r *http.Request, me session) {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxFileBody))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "tooLarge", fmt.Sprintf("The file is larger than %d MB.", maxFileBody>>20))
		return
	}
	f, err := tm.ReadFile(data)
	if err != nil {
		invalid(w, "file", "The file could not be read as a "+tm.FormatName(tm.FormatOf(data))+": "+err.Error())
		return
	}
	out := memoryFileInspection{
		Format: f.Format, FormatName: tm.FormatName(f.Format), Name: f.Name, SourceLang: f.SourceLang, TargetLang: f.TargetLang,
		Description: f.Description, Units: f.Count, Skipped: f.Skipped, Warnings: f.Warnings,
	}
	if out.Warnings == nil {
		out.Warnings = []string{}
	}
	writeJSON(w, http.StatusOK, out)
}

// memoryFileInspection is what a memory file holds.
type memoryFileInspection struct {
	// Format is tmx, sdltm (Trados Studio) or wordfast.
	Format     string `json:"format"`
	FormatName string `json:"formatName"`
	// Name, languages and description as the file gives them: a Trados
	// memory names itself, a TMX or Wordfast file only its languages.
	Name        string `json:"name,omitempty"`
	SourceLang  string `json:"sourceLang,omitempty"`
	TargetLang  string `json:"targetLang,omitempty"`
	Description string `json:"description,omitempty"`
	// Units is how many units the file holds; Skipped, how many of them
	// cannot be read.
	Units    int      `json:"units"`
	Skipped  int      `json:"skipped"`
	Warnings []string `json:"warnings"`
}

// exportTMX sends the whole memory as TMX.
//
// Access: read on the memory.
func (s *Server) exportTMX(w http.ResponseWriter, r *http.Request, me session) {
	m, _, ok := s.openMemoryFor(w, r, me, authdb.RoleRead)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "application/x-tmx+xml; charset=utf-8")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": m.Info().Name + ".tmx"}))
	if err := tm.ExportTMX(r.Context(), m, w, "Interlinea server "+s.cfg.Version); err != nil {
		// The answer has started; the log says what went wrong.
		s.cfg.Log.Error("TMX export failed", "memory", m.Info().Name, "err", err)
	}
}

func parseSince(w http.ResponseWriter, r *http.Request) (int64, bool) {
	v := r.URL.Query().Get("since")
	if v == "" {
		return 0, true
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 0 {
		invalid(w, "since", "since is the number of the last change seen.")
		return 0, false
	}
	return n, true
}

// memoryChanges lists the memory's changes after ?since= (a change number),
// oldest first, at most ?limit=.
//
// Access: read on the memory.
func (s *Server) memoryChanges(w http.ResponseWriter, r *http.Request, me session) {
	m, _, ok := s.openMemoryFor(w, r, me, authdb.RoleRead)
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
	cs, more, err := s.cfg.Memories.Changes(r.Context(), tmdb.ChangeQuery{Since: since, Memories: []string{m.Info().Name}, Limit: limit})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Changes []tmdb.Change `json:"changes"`
		More    bool          `json:"more"`
	}{nonNil(cs), more})
}
