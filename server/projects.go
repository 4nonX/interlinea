package server

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/4nonX/interlinea/auth/authdb"
	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/project"
	"github.com/4nonX/interlinea/project/projectdb"
	"github.com/4nonX/interlinea/term"
	"github.com/4nonX/interlinea/term/termdb"
	"github.com/4nonX/interlinea/tm"
	"github.com/4nonX/interlinea/tm/tmdb"
)

func (s *Server) projectRoutes() {
	s.handle("GET /api/catalog", s.signedIn(s.catalogView))
	s.handle("GET /api/projects", s.signedIn(s.listProjects))
	s.handle("POST /api/projects", s.signedIn(s.createProject))
	s.handle("GET /api/projects/{id}", s.signedIn(s.getProject))
	s.handle("PUT /api/projects/{id}/settings", s.signedIn(s.updateProjectSettings))
	s.handle("DELETE /api/projects/{id}", s.signedIn(s.deleteProject))
	s.handle("POST /api/projects/{id}/files", s.signedIn(s.addProjectFile))
	s.handle("DELETE /api/projects/{id}/files", s.signedIn(s.removeProjectFile))
	s.handle("GET /api/projects/{id}/files/content", s.signedIn(s.projectFileContent))
	s.handle("GET /api/projects/{id}/segments", s.signedIn(s.projectSegments))
	s.handle("PUT /api/projects/{id}/segments", s.signedIn(s.saveProjectSegment))
	s.handle("POST /api/projects/{id}/pretranslate", s.signedIn(s.pretranslateProject))
	s.handle("POST /api/projects/{id}/lookup", s.signedIn(s.projectLookup))
	s.handle("POST /api/projects/{id}/concordance", s.signedIn(s.projectConcordance))
	s.handle("GET /api/projects/{id}/memories/{name}/export", s.signedIn(s.projectMemoryExport))
	s.handle("GET /api/projects/{id}/terminology", s.signedIn(s.projectTerminology))
	s.handle("POST /api/projects/{id}/terms", s.signedIn(s.addProjectTerm))
	s.handle("GET /api/projects/{id}/assignments", s.signedIn(s.projectAssignments))
	s.handle("POST /api/projects/{id}/assignments", s.signedIn(s.assign))
	s.handle("PUT /api/projects/{id}/assignments/{aid}", s.signedIn(s.updateAssignment))
	s.handle("DELETE /api/projects/{id}/assignments/{aid}", s.signedIn(s.unassign))
	s.handle("GET /api/projects/{id}/presence", s.signedIn(s.projectPresence))
	s.handle("PUT /api/projects/{id}/presence", s.signedIn(s.touchPresence))
	s.handle("DELETE /api/projects/{id}/presence", s.signedIn(s.leavePresence))
	s.handle("GET /api/projects/{id}/changes", s.signedIn(s.projectChanges))
	s.handle("GET /api/templates", s.signedIn(s.listTemplates))
	s.handle("PUT /api/templates", s.signedIn(s.saveTemplate))
	s.handle("DELETE /api/templates", s.signedIn(s.deleteTemplate))
}

// catalog lists every memory and termbase, to check settings against.
func (s *Server) catalog(ctx context.Context) (project.Catalog, error) {
	cat := project.Catalog{Memories: map[string]tm.Info{}, Termbases: map[string][]string{}}
	ms, err := s.cfg.Memories.List(ctx)
	if err != nil {
		return cat, err
	}
	for _, m := range ms {
		cat.Memories[m.Name] = m.Info
	}
	tbs, err := s.cfg.Termbases.List(ctx)
	if err != nil {
		return cat, err
	}
	for _, t := range tbs {
		cat.Termbases[t.Name] = t.Languages
	}
	return cat, nil
}

// mayCreate reports whether someone may make projects (manage on all
// projects), memories or termbases.
func (s *Server) mayCreate(ctx context.Context, me session, kind authdb.Kind) (bool, error) {
	return s.cfg.Auth.Can(ctx, me.User, kind, authdb.AnyResource, authdb.RoleManage)
}

type catalogMemory struct {
	Name       string `json:"name"`
	SourceLang string `json:"sourceLang"`
	TargetLang string `json:"targetLang"`
	Units      int    `json:"units"`
	Role       string `json:"role,omitempty"`
}

type catalogTermbase struct {
	Name      string   `json:"name"`
	Languages []string `json:"languages"`
	Entries   int      `json:"entries"`
	Role      string   `json:"role,omitempty"`
}

// catalogView lists the memories and termbases someone may set up
// projects with: all of them for those who make projects, otherwise the
// ones they may read. Role is what they may do with each directly.
func (s *Server) catalogView(w http.ResponseWriter, r *http.Request, me session) {
	ctx := r.Context()
	all, err := s.mayCreate(ctx, me, authdb.KindProject)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := struct {
		Memories  []catalogMemory   `json:"memories"`
		Termbases []catalogTermbase `json:"termbases"`
	}{Memories: []catalogMemory{}, Termbases: []catalogTermbase{}}
	ms, err := s.cfg.Memories.List(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	for _, m := range ms {
		role, err := s.cfg.Auth.RoleOn(ctx, me.User, authdb.KindMemory, m.Name)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if all || role.Includes(authdb.RoleRead) {
			out.Memories = append(out.Memories, catalogMemory{Name: m.Name, SourceLang: m.SourceLang, TargetLang: m.TargetLang, Units: m.Units, Role: string(role)})
		}
	}
	tbs, err := s.cfg.Termbases.List(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	for _, t := range tbs {
		role, err := s.cfg.Auth.RoleOn(ctx, me.User, authdb.KindTermbase, t.Name)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if all || role.Includes(authdb.RoleRead) {
			out.Termbases = append(out.Termbases, catalogTermbase{Name: t.Name, Languages: nonNil(t.Languages), Entries: t.Entries, Role: string(role)})
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// access is what someone may do in a project.
type access struct {
	Role authdb.Role `json:"role"`
	// Assigned is set when they have work in the project, which lets them
	// see it without a grant.
	Assigned bool `json:"assigned"`
}

func (a access) read() bool   { return a.Role.Includes(authdb.RoleRead) || a.Assigned }
func (a access) edit() bool   { return a.Role.Includes(authdb.RoleEdit) }
func (a access) manage() bool { return a.Role.Includes(authdb.RoleManage) }

func (s *Server) accessTo(ctx context.Context, me session, projectID int64) (access, error) {
	role, err := s.cfg.Auth.RoleOn(ctx, me.User, authdb.KindProject, strconv.FormatInt(projectID, 10))
	if err != nil {
		return access{}, err
	}
	a := access{Role: role}
	if me.User.Disabled {
		return a, nil
	}
	mine, err := s.cfg.Projects.AssignmentsOf(ctx, me.User.ID)
	if err != nil {
		return a, err
	}
	for _, x := range mine {
		if x.ProjectID == projectID {
			a.Assigned = true
			break
		}
	}
	return a, nil
}

// openProject loads a project someone may at least see. Someone who may
// not see it is told it doesn't exist.
func (s *Server) openProject(w http.ResponseWriter, r *http.Request, me session) (projectdb.Project, access, bool) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return projectdb.Project{}, access{}, false
	}
	p, err := s.cfg.Projects.Project(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return p, access{}, false
	}
	a, err := s.accessTo(r.Context(), me, id)
	if err != nil {
		s.fail(w, r, err)
		return p, a, false
	}
	if !a.read() {
		writeError(w, http.StatusNotFound, "notFound", "It doesn't exist, or it was deleted.")
		return p, a, false
	}
	return p, a, true
}

type projectView struct {
	projectdb.Project
	Access access `json:"access"`
}

func (s *Server) listProjects(w http.ResponseWriter, r *http.Request, me session) {
	ctx := r.Context()
	ps, err := s.cfg.Projects.List(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	type listed struct {
		projectdb.Summary
		Access access `json:"access"`
	}
	out := []listed{}
	for _, p := range ps {
		a, err := s.accessTo(ctx, me, p.ID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if a.read() {
			out = append(out, listed{Summary: p, Access: a})
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// fileUpload is a file as the apps send it: the original bytes (base64
// in JSON) and, for a file to translate, its segments per target
// language as the app's filters read them.
type fileUpload struct {
	Name       string                       `json:"name"`
	Role       project.Role                 `json:"role"`
	TargetLang string                       `json:"targetLang,omitempty"`
	Format     string                       `json:"format"`
	Data       []byte                       `json:"data"`
	Segments   map[string][]project.Segment `json:"segments,omitempty"`
}

func (f fileUpload) newFile() projectdb.NewFile {
	return projectdb.NewFile{File: project.File{Name: f.Name, Role: f.Role, TargetLang: f.TargetLang}, Format: f.Format, Data: f.Data, Segments: f.Segments}
}

type createProjectRequest struct {
	Settings project.Settings `json:"settings"`
	Files    []fileUpload     `json:"files"`
	// Memories are new memories to make with the project (its own
	// memories); the settings refer to them by name.
	Memories []tm.Info `json:"memories,omitempty"`
	// Termbase makes the project termbase on the server (Layered or Own
	// mode with location server): layered over the settings' masters, or
	// on its own. Its name goes into the settings.
	Termbase *term.Definition `json:"termbase,omitempty"`
}

// createProject makes a project with its own memories and termbase. If a
// step fails, what was made before it is removed again.
func (s *Server) createProject(w http.ResponseWriter, r *http.Request, me session) {
	ctx := r.Context()
	ok, err := s.mayCreate(ctx, me, authdb.KindProject)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !ok {
		forbidden(w, "Only project managers make projects.")
		return
	}
	var req createProjectRequest
	if !readJSON(w, r, &req, maxFileBody) {
		return
	}
	set := req.Settings
	if req.Termbase != nil {
		t := set.Terminology
		switch {
		case t.Mode == project.Direct:
			invalid(w, "termbase", "New terms go into a master, so the project gets no termbase of its own.")
			return
		case t.Location != project.Server:
			invalid(w, "termbase", "The project termbase is kept on this computer, not on the server.")
			return
		case strings.TrimSpace(req.Termbase.Name) == "":
			invalid(w, "termbase.name", "Give the project termbase a name.")
			return
		}
	}
	// Problems with the setup are reported before anything is made. The
	// memories and the termbase to make count as existing.
	cat, err := s.catalog(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	for i, m := range req.Memories {
		info, err := tm.ValidateInfo(m)
		if err != nil {
			invalid(w, fmt.Sprintf("memories[%d]", i), "A new memory needs a name and its source and target languages.")
			return
		}
		if _, taken := lookupFold(cat.Memories, info.Name); taken {
			writeError(w, http.StatusConflict, "exists", fmt.Sprintf("A memory called %s exists already.", info.Name))
			return
		}
		cat.Memories[info.Name] = info
		req.Memories[i] = info
	}
	if req.Termbase != nil {
		name := strings.TrimSpace(req.Termbase.Name)
		if _, taken := lookupFold(cat.Termbases, name); taken {
			writeError(w, http.StatusConflict, "exists", fmt.Sprintf("A termbase called %s exists already.", name))
			return
		}
		langs := req.Termbase.Languages
		if len(langs) == 0 {
			langs = append([]string{set.SourceLang}, set.TargetLangs...)
		}
		cat.Termbases[name] = langs
		set.Terminology.Termbase = name
	}
	files := make([]projectdb.NewFile, len(req.Files))
	for i, f := range req.Files {
		files[i] = f.newFile()
	}
	set.Files = nil
	for _, f := range files {
		set.Files = append(set.Files, f.File)
	}
	if ps := project.Validate(set, cat); len(ps) > 0 {
		s.fail(w, r, &projectdb.InvalidError{Problems: ps})
		return
	}

	var undo []func()
	rollback := func() {
		for i := len(undo) - 1; i >= 0; i-- {
			undo[i]()
		}
	}
	bg := context.WithoutCancel(ctx)
	for _, m := range req.Memories {
		if _, err := s.cfg.Memories.CreateBy(ctx, m, me.by()); err != nil {
			rollback()
			s.fail(w, r, err)
			return
		}
		name := m.Name
		undo = append(undo, func() { s.cfg.Memories.DeleteBy(bg, name, me.by()) })
	}
	if req.Termbase != nil {
		def := *req.Termbase
		def.Name = set.Terminology.Termbase
		if len(def.Languages) == 0 && set.Terminology.Mode == project.Own {
			def.Languages = append([]string{set.SourceLang}, set.TargetLangs...)
		}
		var err error
		if set.Terminology.Mode == project.Layered {
			_, err = s.cfg.Termbases.CreateProject(ctx, def, set.Terminology.Masters, me.by())
		} else {
			_, err = s.cfg.Termbases.CreateBy(ctx, def, me.by())
		}
		if err != nil {
			rollback()
			s.fail(w, r, err)
			return
		}
		name := def.Name
		undo = append(undo, func() { s.cfg.Termbases.DeleteBy(bg, name, me.by()) })
	}
	p, err := s.cfg.Projects.Create(ctx, set, files, cat, me.by())
	if err != nil {
		rollback()
		s.fail(w, r, err)
		return
	}
	// The project's maker manages it and what was made with it.
	grants := []authdb.Grant{{UserID: me.User.ID, Kind: authdb.KindProject, Resource: strconv.FormatInt(p.ID, 10), Role: authdb.RoleManage}}
	for _, m := range req.Memories {
		grants = append(grants, authdb.Grant{UserID: me.User.ID, Kind: authdb.KindMemory, Resource: m.Name, Role: authdb.RoleManage})
	}
	if req.Termbase != nil {
		grants = append(grants, authdb.Grant{UserID: me.User.ID, Kind: authdb.KindTermbase, Resource: set.Terminology.Termbase, Role: authdb.RoleManage})
	}
	if !me.User.Admin {
		for _, g := range grants {
			if err := s.cfg.Auth.SetGrant(ctx, g, me.by()); err != nil {
				s.fail(w, r, err)
				return
			}
		}
	}
	a, err := s.accessTo(ctx, me, p.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, projectView{Project: p, Access: a})
}

// lookupFold finds a name in a map ignoring case, as the stores compare
// memory and termbase names.
func lookupFold[V any](m map[string]V, name string) (V, bool) {
	for k, v := range m {
		if strings.EqualFold(k, name) {
			return v, true
		}
	}
	var zero V
	return zero, false
}

func (s *Server) getProject(w http.ResponseWriter, r *http.Request, me session) {
	p, a, ok := s.openProject(w, r, me)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, projectView{Project: p, Access: a})
}

func (s *Server) updateProjectSettings(w http.ResponseWriter, r *http.Request, me session) {
	p, a, ok := s.openProject(w, r, me)
	if !ok {
		return
	}
	if !a.manage() {
		forbidden(w, "Only the project's managers change its settings.")
		return
	}
	v, ok := version(w, r)
	if !ok {
		return
	}
	var set project.Settings
	if !readJSON(w, r, &set, maxBody) {
		return
	}
	cat, err := s.catalog(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	got, err := s.cfg.Projects.UpdateSettings(r.Context(), p.ID, set, v, cat, me.by())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, projectView{Project: got, Access: a})
}

// deleteProject removes a project. Its memories and termbase stay: they
// may be used by other projects, and are deleted on their own.
func (s *Server) deleteProject(w http.ResponseWriter, r *http.Request, me session) {
	p, a, ok := s.openProject(w, r, me)
	if !ok {
		return
	}
	if !a.manage() {
		forbidden(w, "Only the project's managers delete it.")
		return
	}
	if err := s.cfg.Projects.Delete(r.Context(), p.ID, me.by()); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.cfg.Auth.RevokeResource(r.Context(), authdb.KindProject, strconv.FormatInt(p.ID, 10), me.by()); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) addProjectFile(w http.ResponseWriter, r *http.Request, me session) {
	p, a, ok := s.openProject(w, r, me)
	if !ok {
		return
	}
	if !a.manage() {
		forbidden(w, "Only the project's managers add files.")
		return
	}
	v, ok := version(w, r)
	if !ok {
		return
	}
	var f fileUpload
	if !readJSON(w, r, &f, maxFileBody) {
		return
	}
	cat, err := s.catalog(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	got, err := s.cfg.Projects.AddFile(r.Context(), p.ID, f.newFile(), v, cat, me.by())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, projectView{Project: got, Access: a})
}

func (s *Server) removeProjectFile(w http.ResponseWriter, r *http.Request, me session) {
	p, a, ok := s.openProject(w, r, me)
	if !ok {
		return
	}
	if !a.manage() {
		forbidden(w, "Only the project's managers remove files.")
		return
	}
	v, ok := version(w, r)
	if !ok {
		return
	}
	cat, err := s.catalog(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	got, err := s.cfg.Projects.RemoveFile(r.Context(), p.ID, r.URL.Query().Get("name"), v, cat, me.by())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, projectView{Project: got, Access: a})
}

// contentTypes are the types of the formats the apps store.
var contentTypes = map[string]string{
	"docx":     "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	"xliff":    "application/xliff+xml",
	"sdlxliff": "application/xliff+xml",
	"pdf":      "application/pdf",
	"txt":      "text/plain; charset=utf-8",
}

func (s *Server) projectFileContent(w http.ResponseWriter, r *http.Request, me session) {
	p, _, ok := s.openProject(w, r, me)
	if !ok {
		return
	}
	f, err := s.cfg.Projects.File(r.Context(), p.ID, r.URL.Query().Get("name"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	ct := contentTypes[strings.ToLower(f.Format)]
	if ct == "" {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": path.Base(f.Name)}))
	w.Header().Set("Content-Length", strconv.Itoa(len(f.Data)))
	w.Write(f.Data)
}

func (s *Server) projectSegments(w http.ResponseWriter, r *http.Request, me session) {
	p, _, ok := s.openProject(w, r, me)
	if !ok {
		return
	}
	q := r.URL.Query()
	segs, err := s.cfg.Projects.Segments(r.Context(), p.ID, q.Get("file"), q.Get("lang"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, nonNil(segs))
}

type segmentSave struct {
	Target content.Content `json:"target"`
	Status project.Status  `json:"status"`
}

// saveProjectSegment saves a translation. Editors of the project may
// save any segment; translators and reviewers those assigned to them. A
// confirmed translation goes into the memories the project updates.
func (s *Server) saveProjectSegment(w http.ResponseWriter, r *http.Request, me session) {
	p, a, ok := s.openProject(w, r, me)
	if !ok {
		return
	}
	q := r.URL.Query()
	file, lang, seg := q.Get("file"), q.Get("lang"), q.Get("segment")
	v, ok := version(w, r)
	if !ok {
		return
	}
	var req segmentSave
	if !readJSON(w, r, &req, maxBody) {
		return
	}
	ctx := r.Context()
	if !a.edit() {
		mine, err := s.cfg.Projects.AssignedTo(ctx, p.ID, me.User.ID, file, lang, seg, projectdb.TaskTranslate)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if !mine {
			forbidden(w, "This segment is not assigned to you.")
			return
		}
	}
	got, err := s.cfg.Projects.SaveSegment(ctx, p.ID, file, lang, seg, req.Target, req.Status, v, me.by())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if got.Status == project.StatusConfirmed {
		if err := s.storeConfirmed(ctx, p, got.Segment, lang, me); err != nil {
			// The translation is saved; the memories missing it is
			// reported in the log and to the app.
			s.cfg.Log.Error("updating the project's memories failed", "project", p.ID, "segment", seg, "err", err)
			w.Header().Set("Interlinea-Warning", "memories-not-updated")
		}
	}
	writeJSON(w, http.StatusOK, got)
}

func (s *Server) openMemory(ctx context.Context) func(name string) (tm.Memory, error) {
	return func(name string) (tm.Memory, error) {
		m, err := s.cfg.Memories.Memory(ctx, name)
		if err != nil {
			return nil, err
		}
		return m, nil
	}
}

func (s *Server) storeConfirmed(ctx context.Context, p projectdb.Project, seg project.Segment, lang string, me session) error {
	if strings.TrimSpace(seg.Source.Plain()) == "" || strings.TrimSpace(seg.Target.Plain()) == "" {
		return nil
	}
	lang = canonicalTarget(p, lang)
	mems, err := p.Settings.MemoriesFor(lang, project.UseUpdate, s.openMemory(ctx))
	if err != nil {
		return err
	}
	for _, m := range mems {
		if _, err := m.Add(ctx, tm.Unit{Source: seg.Source, Target: seg.Target, CreatedBy: me.by(), ChangedBy: me.by()}, tm.KeepBoth); err != nil {
			return fmt.Errorf("memory %s: %w", m.Info().Name, err)
		}
	}
	return nil
}

// canonicalTarget is the project's spelling of a target language, the
// key of its memory settings.
func canonicalTarget(p projectdb.Project, lang string) string {
	for _, t := range p.Settings.TargetLangs {
		if strings.EqualFold(strings.ReplaceAll(t, "_", "-"), strings.ReplaceAll(lang, "_", "-")) {
			return t
		}
	}
	return lang
}

// pretranslateProject fills a target language's empty segments (or all
// languages') from the project's memories, as its settings say.
func (s *Server) pretranslateProject(w http.ResponseWriter, r *http.Request, me session) {
	p, a, ok := s.openProject(w, r, me)
	if !ok {
		return
	}
	if !a.manage() {
		forbidden(w, "Only the project's managers pre-translate it.")
		return
	}
	var req struct {
		// Lang is the target language; empty for all.
		Lang    string           `json:"lang"`
		Prepare *project.Prepare `json:"prepare"`
	}
	if !readJSON(w, r, &req, maxBody) {
		return
	}
	prep := p.Settings.Prepare
	if req.Prepare != nil {
		prep = *req.Prepare
	}
	langs := p.Settings.TargetLangs
	if req.Lang != "" {
		langs = []string{canonicalTarget(p, req.Lang)}
	}
	ctx := r.Context()
	type result struct {
		Lang   string `json:"lang"`
		Filled int    `json:"filled"`
	}
	out := []result{}
	for _, lang := range langs {
		mems, err := p.Settings.MemoriesFor(lang, project.UseLookup, s.openMemory(ctx))
		if err != nil {
			s.fail(w, r, err)
			return
		}
		var files []project.FileSegments
		for _, f := range p.Settings.Files {
			if f.Role != project.Translatable || (f.TargetLang != "" && !strings.EqualFold(f.TargetLang, lang)) {
				continue
			}
			segs, err := s.cfg.Projects.Segments(ctx, p.ID, f.Name, lang)
			if err != nil {
				s.fail(w, r, err)
				return
			}
			fs := project.FileSegments{Name: f.Name}
			for _, sg := range segs {
				fs.Segments = append(fs.Segments, sg.Segment)
			}
			files = append(files, fs)
		}
		ts, err := project.Pretranslate(ctx, files, mems, prep, p.Settings.Analysis.Context)
		if err != nil {
			invalid(w, "prepare.minScore", err.Error())
			return
		}
		n, err := s.cfg.Projects.ApplyTranslations(ctx, p.ID, lang, ts, me.by())
		if err != nil {
			s.fail(w, r, err)
			return
		}
		out = append(out, result{Lang: lang, Filled: n})
	}
	writeJSON(w, http.StatusOK, out)
}

// unitView is a memory unit as the API shows it.
type unitView struct {
	ID         int64             `json:"id,omitempty"`
	Source     content.Content   `json:"source"`
	Target     content.Content   `json:"target"`
	CreatedAt  string            `json:"createdAt,omitempty"`
	CreatedBy  string            `json:"createdBy,omitempty"`
	ChangedAt  string            `json:"changedAt,omitempty"`
	ChangedBy  string            `json:"changedBy,omitempty"`
	UsageCount int               `json:"usageCount,omitempty"`
	Attributes map[string]string `json:"attributes,omitempty"`
	Notes      []string          `json:"notes,omitempty"`
	Version    int               `json:"version,omitempty"`
}

const apiTime = "2006-01-02T15:04:05Z07:00"

func viewUnit(u tm.Unit) unitView {
	v := unitView{ID: u.ID, Source: u.Source, Target: u.Target, CreatedBy: u.CreatedBy, ChangedBy: u.ChangedBy,
		UsageCount: u.UsageCount, Notes: u.Notes, Version: u.Version}
	if !u.CreatedAt.IsZero() {
		v.CreatedAt = u.CreatedAt.UTC().Format(apiTime)
	}
	if !u.ChangedAt.IsZero() {
		v.ChangedAt = u.ChangedAt.UTC().Format(apiTime)
	}
	if len(u.Attributes) > 0 {
		v.Attributes = map[string]string{}
		for _, a := range u.Attributes {
			v.Attributes[a.Name] = a.Value
		}
	}
	return v
}

type matchView struct {
	Score  int      `json:"score"`
	Memory string   `json:"memory"`
	Unit   unitView `json:"unit"`
}

type lookupRequest struct {
	Lang     string          `json:"lang"`
	Source   content.Content `json:"source"`
	MinScore int             `json:"minScore"`
	Limit    int             `json:"limit"`
}

// lookup finds matches in memories, best first; on equal scores the
// earlier memory wins.
func lookup(ctx context.Context, mems []tm.Memory, req lookupRequest) ([]matchView, error) {
	if req.MinScore <= 0 {
		req.MinScore = 70
	}
	if req.Limit <= 0 || req.Limit > 50 {
		req.Limit = 5
	}
	type ranked struct {
		matchView
		order int
	}
	var all []ranked
	for i, m := range mems {
		ms, err := m.Lookup(ctx, req.Source, tm.LookupOptions{MinScore: req.MinScore, Limit: req.Limit})
		if err != nil {
			return nil, err
		}
		for _, x := range ms {
			all = append(all, ranked{matchView{Score: x.Score, Memory: m.Info().Name, Unit: viewUnit(x.Unit)}, i})
		}
	}
	// Stable insertion sort: few matches.
	for i := 1; i < len(all); i++ {
		for j := i; j > 0 && (all[j].Score > all[j-1].Score || (all[j].Score == all[j-1].Score && all[j].order < all[j-1].order)); j-- {
			all[j], all[j-1] = all[j-1], all[j]
		}
	}
	out := []matchView{}
	for i := 0; i < len(all) && i < req.Limit; i++ {
		out = append(out, all[i].matchView)
	}
	return out, nil
}

func (s *Server) projectLookup(w http.ResponseWriter, r *http.Request, me session) {
	p, _, ok := s.openProject(w, r, me)
	if !ok {
		return
	}
	var req lookupRequest
	if !readJSON(w, r, &req, maxBody) {
		return
	}
	mems, err := p.Settings.MemoriesFor(canonicalTarget(p, req.Lang), project.UseLookup, s.openMemory(r.Context()))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out, err := lookup(r.Context(), mems, req)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

type concordanceRequest struct {
	Lang     string `json:"lang"`
	Query    string `json:"query"`
	InTarget bool   `json:"inTarget"`
	Limit    int    `json:"limit"`
}

type concordanceHit struct {
	Memory string   `json:"memory"`
	Unit   unitView `json:"unit"`
}

func concordance(ctx context.Context, mems []tm.Memory, req concordanceRequest) ([]concordanceHit, error) {
	if req.Limit <= 0 || req.Limit > 500 {
		req.Limit = 50
	}
	out := []concordanceHit{}
	for _, m := range mems {
		us, err := m.Concordance(ctx, req.Query, tm.ConcordanceOptions{InTarget: req.InTarget, Limit: req.Limit - len(out)})
		if err != nil {
			return nil, err
		}
		for _, u := range us {
			out = append(out, concordanceHit{Memory: m.Info().Name, Unit: viewUnit(u)})
		}
		if len(out) >= req.Limit {
			break
		}
	}
	return out, nil
}

func (s *Server) projectConcordance(w http.ResponseWriter, r *http.Request, me session) {
	p, _, ok := s.openProject(w, r, me)
	if !ok {
		return
	}
	var req concordanceRequest
	if !readJSON(w, r, &req, maxBody) {
		return
	}
	mems, err := p.Settings.MemoriesFor(canonicalTarget(p, req.Lang), project.UseConcordance, s.openMemory(r.Context()))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out, err := concordance(r.Context(), mems, req)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// projectMemoryExport sends a memory the project looks up in, or searches
// with the concordance, as TMX, to the project's people: the apps match
// against it while translating. Memories the project only updates are
// not sent.
func (s *Server) projectMemoryExport(w http.ResponseWriter, r *http.Request, me session) {
	p, _, ok := s.openProject(w, r, me)
	if !ok {
		return
	}
	name := r.PathValue("name")
	used := false
	for _, ms := range p.Settings.Memories {
		for _, m := range ms {
			if strings.EqualFold(m.Memory, name) && (m.Lookup || m.Concordance) {
				used = true
			}
		}
	}
	if !used {
		writeError(w, http.StatusNotFound, "notFound", "The project doesn't look up in this memory.")
		return
	}
	m, err := s.cfg.Memories.Memory(r.Context(), name)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/x-tmx+xml; charset=utf-8")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": m.Info().Name + ".tmx"}))
	if err := tm.ExportTMX(r.Context(), m, w, "Interlinea server "+s.cfg.Version); err != nil {
		s.cfg.Log.Error("TMX export failed", "memory", m.Info().Name, "err", err)
	}
}

// termbaseView is a termbase with the entries a project sees in it.
type termbaseView struct {
	Name       string           `json:"name"`
	Kind       string           `json:"kind"`
	Upstreams  []string         `json:"upstreams,omitempty"`
	Definition term.Definition  `json:"definition"`
	Entries    []termdb.Layered `json:"entries"`
}

// projectTerminology returns the termbases a project works with that are
// on the server: its own (with the masters' entries layered under it)
// and the masters it uses directly. The apps find terms in segments with
// them.
func (s *Server) projectTerminology(w http.ResponseWriter, r *http.Request, me session) {
	p, _, ok := s.openProject(w, r, me)
	if !ok {
		return
	}
	ctx := r.Context()
	t := p.Settings.Terminology
	var names []string
	if t.Termbase != "" && t.Location == project.Server {
		names = append(names, t.Termbase)
	}
	if t.Mode == project.Direct || t.Termbase == "" || t.Location != project.Server {
		names = append(names, t.Masters...)
	}
	out := []termbaseView{}
	for _, n := range names {
		tb, err := s.cfg.Termbases.Termbase(ctx, n)
		if errors.Is(err, termdb.ErrNotFound) {
			// A master kept elsewhere (on someone's computer).
			continue
		}
		if err != nil {
			s.fail(w, r, err)
			return
		}
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
		es, err := tb.Layered(ctx)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		out = append(out, termbaseView{Name: info.Name, Kind: info.Kind, Upstreams: info.Upstreams, Definition: def, Entries: nonNil(es)})
	}
	writeJSON(w, http.StatusOK, out)
}

// addProjectTerm adds a term found while translating where the project's
// setup puts new terms: its own termbase, or the master in Direct mode.
func (s *Server) addProjectTerm(w http.ResponseWriter, r *http.Request, me session) {
	p, a, ok := s.openProject(w, r, me)
	if !ok {
		return
	}
	if !a.edit() && !a.Assigned {
		forbidden(w, "Only the project's translators, reviewers and editors add terms.")
		return
	}
	var e term.Entry
	if !readJSON(w, r, &e, maxBody) {
		return
	}
	t := p.Settings.Terminology
	target := t.Termbase
	if t.Mode == project.Direct {
		target = t.WriteTo
	}
	if target == "" || (t.Mode != project.Direct && t.Location != project.Server) {
		writeError(w, http.StatusConflict, "notOnServer", "The project keeps new terms on a computer, not on the server.")
		return
	}
	tb, err := s.cfg.Termbases.Termbase(r.Context(), target)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	e.ID = ""
	got, err := tb.Add(r.Context(), e, me.by(), "Added in project "+p.Settings.Name)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, struct {
		Termbase string        `json:"termbase"`
		Entry    termdb.Stored `json:"entry"`
	}{tb.Name(), got})
}

func (s *Server) projectAssignments(w http.ResponseWriter, r *http.Request, me session) {
	p, _, ok := s.openProject(w, r, me)
	if !ok {
		return
	}
	as, err := s.cfg.Projects.Assignments(r.Context(), p.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, nonNil(as))
}

func (s *Server) ownAssignments(w http.ResponseWriter, r *http.Request, me session) {
	as, err := s.cfg.Projects.AssignmentsOf(r.Context(), me.User.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, nonNil(as))
}

// assignee checks that work goes to an active account, and takes the
// name the work is listed under from it.
func (s *Server) assignee(w http.ResponseWriter, r *http.Request, a *projectdb.Assignment) bool {
	u, err := s.cfg.Auth.User(r.Context(), a.UserID)
	if errors.Is(err, authdb.ErrNotFound) || (err == nil && u.Disabled) {
		invalid(w, "user", "Choose someone with an active account.")
		return false
	}
	if err != nil {
		s.fail(w, r, err)
		return false
	}
	a.UserName = u.Name
	return true
}

func (s *Server) assign(w http.ResponseWriter, r *http.Request, me session) {
	p, acc, ok := s.openProject(w, r, me)
	if !ok {
		return
	}
	if !acc.manage() {
		forbidden(w, "Only the project's managers give out work.")
		return
	}
	var a projectdb.Assignment
	if !readJSON(w, r, &a, maxBody) {
		return
	}
	a.ProjectID = p.ID
	if !s.assignee(w, r, &a) {
		return
	}
	got, err := s.cfg.Projects.Assign(r.Context(), a, me.by())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, got)
}

// assignmentOf loads an assignment of the project in the path.
func (s *Server) assignmentOf(w http.ResponseWriter, r *http.Request, projectID int64) (projectdb.Assignment, bool) {
	aid, ok := pathID(w, r, "aid")
	if !ok {
		return projectdb.Assignment{}, false
	}
	as, err := s.cfg.Projects.Assignments(r.Context(), projectID)
	if err != nil {
		s.fail(w, r, err)
		return projectdb.Assignment{}, false
	}
	for _, a := range as {
		if a.ID == aid {
			return a, true
		}
	}
	writeError(w, http.StatusNotFound, "notFound", "It doesn't exist, or it was deleted.")
	return projectdb.Assignment{}, false
}

// updateAssignment changes an assignment. Managers change anything; the
// person it is for only reports it done or not done.
func (s *Server) updateAssignment(w http.ResponseWriter, r *http.Request, me session) {
	p, acc, ok := s.openProject(w, r, me)
	if !ok {
		return
	}
	cur, ok := s.assignmentOf(w, r, p.ID)
	if !ok {
		return
	}
	v, ok := version(w, r)
	if !ok {
		return
	}
	var a projectdb.Assignment
	if !readJSON(w, r, &a, maxBody) {
		return
	}
	switch {
	case acc.manage():
		if a.UserID != cur.UserID && !s.assignee(w, r, &a) {
			return
		}
		if a.UserID == cur.UserID {
			a.UserName = cur.UserName
		}
	case cur.UserID == me.User.ID:
		done := a.Done
		a = cur
		a.Done = done
	default:
		forbidden(w, "Only the project's managers change work given to others.")
		return
	}
	a.ID, a.ProjectID = cur.ID, p.ID
	got, err := s.cfg.Projects.UpdateAssignment(r.Context(), a, v, me.by())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, got)
}

func (s *Server) unassign(w http.ResponseWriter, r *http.Request, me session) {
	p, acc, ok := s.openProject(w, r, me)
	if !ok {
		return
	}
	if !acc.manage() {
		forbidden(w, "Only the project's managers take work back.")
		return
	}
	cur, ok := s.assignmentOf(w, r, p.ID)
	if !ok {
		return
	}
	if err := s.cfg.Projects.Unassign(r.Context(), cur.ID, me.by()); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) projectPresence(w http.ResponseWriter, r *http.Request, me session) {
	p, _, ok := s.openProject(w, r, me)
	if !ok {
		return
	}
	q := r.URL.Query()
	ps, err := s.cfg.Projects.Present(r.Context(), p.ID, q.Get("file"), q.Get("lang"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, nonNil(ps))
}

func (s *Server) touchPresence(w http.ResponseWriter, r *http.Request, me session) {
	p, _, ok := s.openProject(w, r, me)
	if !ok {
		return
	}
	var req struct {
		File    string `json:"file"`
		Lang    string `json:"lang"`
		Segment string `json:"segment"`
	}
	if !readJSON(w, r, &req, maxBody) {
		return
	}
	got, err := s.cfg.Projects.Touch(r.Context(), p.ID, req.File, req.Lang, req.Segment, me.User.ID, me.User.Name)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.feed.presence(p.ID)
	writeJSON(w, http.StatusOK, got)
}

func (s *Server) leavePresence(w http.ResponseWriter, r *http.Request, me session) {
	p, _, ok := s.openProject(w, r, me)
	if !ok {
		return
	}
	if err := s.cfg.Projects.Leave(r.Context(), p.ID, me.User.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	s.feed.presence(p.ID)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) projectChanges(w http.ResponseWriter, r *http.Request, me session) {
	p, _, ok := s.openProject(w, r, me)
	if !ok {
		return
	}
	since, err := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64)
	if err != nil && r.URL.Query().Get("since") != "" {
		invalid(w, "since", "since is the number of the last change seen.")
		return
	}
	limit, ok := queryInt(w, r, "limit", 1000, 1, 10000)
	if !ok {
		return
	}
	cs, more, err := s.cfg.Projects.Changes(r.Context(), projectdb.ChangeQuery{Since: since, Project: p.ID, Limit: limit})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Changes []projectdb.Change `json:"changes"`
		More    bool               `json:"more"`
	}{nonNil(cs), more})
}

func (s *Server) listTemplates(w http.ResponseWriter, r *http.Request, me session) {
	ts, err := s.cfg.Projects.Templates(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, nonNil(ts))
}

func (s *Server) saveTemplate(w http.ResponseWriter, r *http.Request, me session) {
	ok, err := s.mayCreate(r.Context(), me, authdb.KindProject)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !ok {
		forbidden(w, "Only project managers keep templates.")
		return
	}
	v, ok := version(w, r)
	if !ok {
		return
	}
	var t project.Template
	if !readJSON(w, r, &t, maxBody) {
		return
	}
	cat, err := s.catalog(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	got, err := s.cfg.Projects.SaveTemplate(r.Context(), t, v, cat, me.by())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, got)
}

func (s *Server) deleteTemplate(w http.ResponseWriter, r *http.Request, me session) {
	ok, err := s.mayCreate(r.Context(), me, authdb.KindProject)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !ok {
		forbidden(w, "Only project managers keep templates.")
		return
	}
	if err := s.cfg.Projects.DeleteTemplate(r.Context(), r.URL.Query().Get("name"), me.by()); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// tmdbMemory is the memory interface the server uses from the store.
var _ tm.Memory = (*tmdb.Memory)(nil)
