package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/4nonX/interlinea/analysis"
	"github.com/4nonX/interlinea/auth/authdb"
	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/langtag"
	"github.com/4nonX/interlinea/project"
	"github.com/4nonX/interlinea/project/projectdb"
	"github.com/4nonX/interlinea/qa"
	"github.com/4nonX/interlinea/spell"
	"github.com/4nonX/interlinea/term"
	"github.com/4nonX/interlinea/term/termdb"
)

// Quality checks, spelling and term recognition run on the server, for
// the desktop app and for any app that would rather not do them itself.

func (s *Server) qaRoutes() {
	s.handle("GET /api/qa/kinds", s.signedIn(s.qaKinds))
	s.handle("GET /api/dictionaries", s.signedIn(s.listDictionaries))
	s.handle("POST /api/dictionaries", s.signedIn(s.installDictionaries))
	s.handle("DELETE /api/dictionaries/{name}", s.signedIn(s.deleteDictionary))
	s.handle("POST /api/dictionaries/{name}/check", s.signedIn(s.spellCheck))
	s.handle("GET /api/dictionaries/{name}/suggest", s.signedIn(s.spellSuggest))
	s.handle("POST /api/projects/{id}/qa", s.signedIn(s.projectQA))
	s.handle("GET /api/projects/{id}/qa/report", s.signedIn(s.projectQAReport))
	s.handle("POST /api/projects/{id}/recognize", s.signedIn(s.projectRecognize))
	s.handle("POST /api/projects/{id}/analysis", s.signedIn(s.projectAnalysis))
}

// dictionaries keeps loaded spelling dictionaries, reloaded when one is
// installed again.
type dictionaries struct {
	mu     sync.Mutex
	loaded map[string]loadedDict
}

type loadedDict struct {
	at time.Time
	d  *spell.Dictionary
}

// dictionary loads a dictionary: the installed one of the name, or else
// the included one.
func (s *Server) dictionary(ctx context.Context, name string) (*spell.Dictionary, error) {
	ds, err := s.cfg.Projects.Dictionaries(ctx)
	if err != nil {
		return nil, err
	}
	var info *projectdb.Dictionary
	for i := range ds {
		if strings.EqualFold(ds[i].Name, name) {
			info = &ds[i]
		}
	}
	var at time.Time
	switch {
	case info != nil:
		name, at = info.Name, info.CreatedAt
	default:
		e, ok := s.cfg.Dictionaries.Find(name)
		if !ok {
			return nil, projectdb.ErrNotFound
		}
		// Included dictionaries have no time, installed ones always do.
		name = e.Name
	}
	key := strings.ToLower(name)
	s.dicts.mu.Lock()
	defer s.dicts.mu.Unlock()
	if l, ok := s.dicts.loaded[key]; ok && l.at.Equal(at) {
		return l.d, nil
	}
	var d *spell.Dictionary
	if info != nil {
		_, aff, dic, err := s.cfg.Projects.DictionaryFiles(ctx, name)
		if err != nil {
			return nil, err
		}
		if d, err = spell.Load(aff, dic); err != nil {
			return nil, fmt.Errorf("the dictionary %s can't be loaded: %w", name, err)
		}
	} else if d, err = s.cfg.Dictionaries.Load(name); err != nil {
		return nil, fmt.Errorf("the dictionary %s can't be loaded: %w", name, err)
	}
	if s.dicts.loaded == nil {
		s.dicts.loaded = map[string]loadedDict{}
	}
	s.dicts.loaded[key] = loadedDict{at: at, d: d}
	return d, nil
}

// dictionaryView is a spelling dictionary as the API lists it.
type dictionaryView struct {
	projectdb.Dictionary
	// Included says it comes with Interlinea: it can't be deleted, and
	// installing one of the same name puts that one in its place.
	Included bool `json:"included,omitempty"`
}

// allDictionaries lists the installed dictionaries and the included ones
// no installed one has the name of, by name.
func (s *Server) allDictionaries(ctx context.Context) ([]dictionaryView, error) {
	ds, err := s.cfg.Projects.Dictionaries(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]dictionaryView, 0, len(ds)+len(s.cfg.Dictionaries.List()))
	have := map[string]bool{}
	for _, d := range ds {
		out = append(out, dictionaryView{Dictionary: d})
		have[strings.ToLower(d.Name)] = true
	}
	for _, e := range s.cfg.Dictionaries.List() {
		if have[strings.ToLower(e.Name)] {
			continue
		}
		out = append(out, dictionaryView{
			Dictionary: projectdb.Dictionary{Name: e.Name, Lang: e.Lang, Locales: []string{e.Lang}, Words: e.Words},
			Included:   true,
		})
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out, nil
}

// qaKinds lists the quality checks, which of them run by default, and what
// each finds.
//
// Access: signed in.
func (s *Server) qaKinds(w http.ResponseWriter, r *http.Request, me session) {
	writeJSON(w, http.StatusOK, qa.Kinds)
}

// listDictionaries lists the spelling dictionaries on the server: those
// installed and those that come with Interlinea (included), by name. An
// installed dictionary takes the place of an included one of its name.
//
// Access: signed in.
func (s *Server) listDictionaries(w http.ResponseWriter, r *http.Request, me session) {
	ds, err := s.allDictionaries(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ds)
}

// looseDictionary is a Hunspell dictionary sent as its two files.
type looseDictionary struct {
	// Name identifies it; the files' base name ("de_DE_frami").
	Name string `json:"name"`
	// Lang is the language it spells. Left out, the .aff file's LANG line
	// or the name says it.
	Lang string `json:"lang,omitempty"`
	// Aff and Dic are the bytes of the .aff and the .dic file.
	Aff []byte `json:"aff"`
	Dic []byte `json:"dic"`
}

// installDictionaries installs the Hunspell dictionaries in a LibreOffice
// extension (.oxt) or ZIP file (the request body), replacing those of the
// same names, and lists them. With Content-Type application/json the body
// is one dictionary as its .aff and .dic files, with its name and, if the
// files do not say it, its language.
//
// Access: those who make projects (manage on all projects) and
// administrators.
func (s *Server) installDictionaries(w http.ResponseWriter, r *http.Request, me session) {
	ctx := r.Context()
	ok, err := s.mayCreate(ctx, me, authdb.KindProject)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !ok {
		forbidden(w, "Only project managers install dictionaries.")
		return
	}
	var bs []spell.Bundled
	said := ""
	if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt == "application/json" {
		var loose looseDictionary
		if !readJSON(w, r, &loose, maxFileBody) {
			return
		}
		name := strings.TrimSpace(loose.Name)
		if name == "" || strings.ContainsAny(name, `/\`) || hasControlRune(name) {
			invalid(w, "name", "Give the dictionary a name without slashes.")
			return
		}
		if len(loose.Aff) == 0 || len(loose.Dic) == 0 {
			invalid(w, "file", "A dictionary is two files: its .aff and its .dic.")
			return
		}
		if loose.Lang != "" {
			n, err := langtag.Normalize(strings.ReplaceAll(loose.Lang, "_", "-"))
			if err != nil {
				invalid(w, "lang", fmt.Sprintf("%s is not a language tag.", loose.Lang))
				return
			}
			said = n
		}
		bs = []spell.Bundled{{Name: name, Locales: []string{name}, Aff: loose.Aff, Dic: loose.Dic}}
	} else {
		data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxFileBody))
		if err != nil {
			writeError(w, http.StatusRequestEntityTooLarge, "tooLarge", "The file is too large.")
			return
		}
		if bs, err = spell.ReadArchive(data); err != nil {
			invalid(w, "file", "The file is not a dictionary: "+err.Error())
			return
		}
	}
	var out []projectdb.Dictionary
	for _, b := range bs {
		d, err := spell.Load(b.Aff, b.Dic)
		if err != nil {
			invalid(w, "file", fmt.Sprintf("The dictionary %s can't be read: %v", b.Name, err))
			return
		}
		lang := said
		if lang == "" {
			for _, l := range append([]string{d.Lang()}, b.Locales...) {
				if n, err := langtag.Normalize(strings.ReplaceAll(l, "_", "-")); err == nil && l != "" {
					lang = n
					break
				}
			}
		}
		if lang == "" {
			invalid(w, "lang", fmt.Sprintf("The dictionary %s doesn't say which language it spells.", b.Name))
			return
		}
		locales := b.Locales
		if said != "" {
			locales = []string{said}
		}
		x, err := s.cfg.Projects.SaveDictionary(ctx, projectdb.Dictionary{Name: b.Name, Lang: lang, Locales: locales, Words: d.Words()}, b.Aff, b.Dic, me.by())
		if err != nil {
			s.fail(w, r, err)
			return
		}
		s.dicts.mu.Lock()
		delete(s.dicts.loaded, strings.ToLower(b.Name))
		s.dicts.mu.Unlock()
		out = append(out, x)
	}
	writeJSON(w, http.StatusOK, out)
}

func hasControlRune(s string) bool {
	return strings.IndexFunc(s, unicode.IsControl) >= 0
}

// deleteDictionary removes a spelling dictionary. One a project or
// template spells with is not removed; the answer names them. Included
// dictionaries can't be removed; an installed one of the same name can, and
// the included one is then used again.
//
// Access: those who make projects and administrators.
func (s *Server) deleteDictionary(w http.ResponseWriter, r *http.Request, me session) {
	ctx := r.Context()
	ok, err := s.mayCreate(ctx, me, authdb.KindProject)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !ok {
		forbidden(w, "Only project managers remove dictionaries.")
		return
	}
	name := r.PathValue("name")
	installed, err := s.cfg.Projects.Dictionaries(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !slices.ContainsFunc(installed, func(d projectdb.Dictionary) bool { return strings.EqualFold(d.Name, name) }) {
		if e, ok := s.cfg.Dictionaries.Find(name); ok {
			writeError(w, http.StatusConflict, "included", fmt.Sprintf("%s comes with Interlinea and can't be deleted. Projects choose whether to spell with it.", e.Name))
			return
		}
	}
	// With an included one of the name, the projects keep a dictionary.
	_, included := s.cfg.Dictionaries.Find(name)
	users, err := s.usedBy(ctx, func(set project.Settings) bool {
		if included {
			return false
		}
		for _, d := range set.QA.Dictionaries {
			if strings.EqualFold(d, name) {
				return true
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
	if err := s.cfg.Projects.DeleteDictionary(ctx, name); err != nil {
		s.fail(w, r, err)
		return
	}
	s.dicts.mu.Lock()
	delete(s.dicts.loaded, strings.ToLower(name))
	s.dicts.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

type spellRequest struct {
	Words []string `json:"words"`
}

type spellResult struct {
	// Correct says, for each word, whether it is spelled right.
	Correct []bool `json:"correct"`
}

// spellCheck checks the spelling of words with a dictionary.
//
// Access: signed in.
func (s *Server) spellCheck(w http.ResponseWriter, r *http.Request, me session) {
	var req spellRequest
	if !readJSON(w, r, &req, maxBody) {
		return
	}
	d, err := s.dictionary(r.Context(), r.PathValue("name"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := spellResult{Correct: make([]bool, len(req.Words))}
	for i, word := range req.Words {
		out.Correct[i] = d.Spell(word)
	}
	writeJSON(w, http.StatusOK, out)
}

// spellSuggest suggests corrections for a word (?word=), at most ?max= (5).
//
// Access: signed in.
func (s *Server) spellSuggest(w http.ResponseWriter, r *http.Request, me session) {
	max, ok := queryInt(w, r, "max", 5, 1, 50)
	if !ok {
		return
	}
	d, err := s.dictionary(r.Context(), r.PathValue("name"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, nonNil(d.Suggest(r.URL.Query().Get("word"), max)))
}

// termIndexes keeps each project's term index until a termbase or the
// project changes.
type termIndexes struct {
	mu sync.Mutex
	by map[int64]cachedIndex
}

type cachedIndex struct {
	version int
	last    int64
	x       *term.Index
}

// termIndex is the index of the terms a project's segments are checked
// and annotated with: its termbases on the server, the masters layered
// under its own.
func (s *Server) termIndex(ctx context.Context, p projectdb.Project) (*term.Index, error) {
	last, err := s.cfg.Termbases.LastChange(ctx)
	if err != nil {
		return nil, err
	}
	s.terms.mu.Lock()
	c, ok := s.terms.by[p.ID]
	s.terms.mu.Unlock()
	if ok && c.version == p.Version && c.last == last {
		return c.x, nil
	}
	x := term.NewIndex()
	for _, n := range projectTermbases(p.Settings.Terminology) {
		tb, err := s.cfg.Termbases.Termbase(ctx, n)
		if errors.Is(err, termdb.ErrNotFound) {
			continue // kept on someone's computer
		}
		if err != nil {
			return nil, err
		}
		ls, err := tb.Layered(ctx)
		if err != nil {
			return nil, err
		}
		es := make([]term.Entry, len(ls))
		for i, l := range ls {
			es[i] = l.Entry
		}
		x.Add(tb.Name(), es)
	}
	s.terms.mu.Lock()
	if s.terms.by == nil {
		s.terms.by = map[int64]cachedIndex{}
	}
	s.terms.by[p.ID] = cachedIndex{version: p.Version, last: last, x: x}
	s.terms.mu.Unlock()
	return x, nil
}

// projectTermbases are the termbases a project works with: its own, and
// the masters it uses directly.
func projectTermbases(t project.Terminology) []string {
	var names []string
	if t.Termbase != "" && t.Location == project.Server {
		names = append(names, t.Termbase)
	}
	if t.Mode == project.Direct || t.Termbase == "" || t.Location != project.Server {
		names = append(names, t.Masters...)
	}
	return names
}

// qaOptions are the project's checks for a target language, with its
// terms and its dictionary for the language.
func (s *Server) qaOptions(ctx context.Context, p projectdb.Project, lang string) (qa.Options, error) {
	o := p.Settings.QAOptions(lang)
	x, err := s.termIndex(ctx, p)
	if err != nil {
		return o, err
	}
	o.Terms = x
	if name := p.Settings.QA.Dictionaries[lang]; name != "" {
		d, err := s.dictionary(ctx, name)
		switch {
		case errors.Is(err, projectdb.ErrNotFound):
			// Removed since; the language is not spell-checked.
		case err != nil:
			return o, err
		default:
			o.Spell = d
		}
	}
	return o, nil
}

type qaRequest struct {
	File string `json:"file"`
	Lang string `json:"lang"`
	// Segments are the segments to check, by ID; empty for all.
	Segments []string `json:"segments,omitempty"`
	// Targets check translations not saved yet, by segment ID.
	Targets map[string]content.Content `json:"targets,omitempty"`
}

// qaSegments are a file's segments as the checks read them, with
// unsaved translations put in.
func (s *Server) qaSegments(ctx context.Context, p projectdb.Project, file, lang string, only []string, targets map[string]content.Content) ([]qa.Segment, error) {
	segs, err := s.cfg.Projects.Segments(ctx, p.ID, file, lang)
	if err != nil {
		return nil, err
	}
	want := map[string]bool{}
	for _, id := range only {
		want[id] = true
	}
	var out []qa.Segment
	for _, sg := range segs {
		if len(want) > 0 && !want[sg.ID] {
			continue
		}
		t := sg.Target
		if x, ok := targets[sg.ID]; ok {
			t = x
		}
		out = append(out, qa.Segment{ID: sg.ID, Source: sg.Source, Target: t, MaxLength: sg.MaxLength, Skip: sg.Locked})
	}
	return out, nil
}

// projectQA runs the project's quality checks on a file's segments in a
// target language (or some of them), with translations not saved yet if
// given, and lists the issues found.
//
// Access: read on the project, or work in it.
func (s *Server) projectQA(w http.ResponseWriter, r *http.Request, me session) {
	p, _, ok := s.openProject(w, r, me)
	if !ok {
		return
	}
	var req qaRequest
	if !readJSON(w, r, &req, maxBody) {
		return
	}
	ctx := r.Context()
	lang := canonicalTarget(p, req.Lang)
	segs, err := s.qaSegments(ctx, p, req.File, lang, req.Segments, req.Targets)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	o, err := s.qaOptions(ctx, p, lang)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	issues, err := qa.Check(segs, o)
	if err != nil {
		invalid(w, "qa", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, nonNil(issues))
}

// projectQAReport writes the quality report of a file in a target language
// (?file=&lang=) as HTML, CSV or JSON (?format=, html by default). In a
// project with a grammar service the translations are sent to it, and
// what it finds is in the report.
//
// Access: read on the project, or work in it.
func (s *Server) projectQAReport(w http.ResponseWriter, r *http.Request, me session) {
	p, _, ok := s.openProject(w, r, me)
	if !ok {
		return
	}
	q := r.URL.Query()
	ctx := r.Context()
	lang := canonicalTarget(p, q.Get("lang"))
	segs, err := s.qaSegments(ctx, p, q.Get("file"), lang, nil, nil)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	o, err := s.qaOptions(ctx, p, lang)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	issues, err := qa.Check(segs, o)
	if err != nil {
		invalid(w, "qa", err.Error())
		return
	}
	found, err := s.grammarIssues(ctx, p, lang, segs)
	if err != nil {
		writeError(w, http.StatusBadGateway, "provider", err.Error()+".")
		return
	}
	issues = append(issues, found...)
	rep := qa.NewReport(q.Get("file"), segs, issues, o, s.cfg.Now())
	var data []byte
	var ct, ext string
	switch q.Get("format") {
	case "", "html":
		data, ct, ext = rep.HTML(), "text/html; charset=utf-8", ".html"
	case "csv":
		data, err = rep.CSV()
		ct, ext = "text/csv; charset=utf-8", ".csv"
	case "json":
		data, err = rep.JSON()
		ct, ext = "application/json", ".json"
	default:
		invalid(w, "format", "The report is html, csv or json.")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": "QA " + lang + ext}))
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Write(data)
}

type recognizeRequest struct {
	// Lang is the target language the entries' translations are for.
	Lang string `json:"lang"`
	Text string `json:"text"`
}

// termHit is a term found in a text, with its place in UTF-16 code units
// as well, for apps whose strings count those.
type termHit struct {
	term.Hit
	Start16 int `json:"start16"`
	End16   int `json:"end16"`
}

// projectRecognize finds the terms of the project's termbases in a source
// text, with their translations into a target language, as the editor
// shows them for a segment.
//
// Access: read on the project, or work in it.
func (s *Server) projectRecognize(w http.ResponseWriter, r *http.Request, me session) {
	p, _, ok := s.openProject(w, r, me)
	if !ok {
		return
	}
	var req recognizeRequest
	if !readJSON(w, r, &req, maxBody) {
		return
	}
	x, err := s.termIndex(r.Context(), p)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	hits := x.Recognize(req.Text, p.Settings.SourceLang, canonicalTarget(p, req.Lang))
	out := make([]termHit, len(hits))
	for i, h := range hits {
		out[i] = termHit{Hit: h, Start16: utf16Len(req.Text[:h.Start]), End16: utf16Len(req.Text[:h.End])}
	}
	writeJSON(w, http.StatusOK, out)
}

func utf16Len(s string) int {
	n := 0
	for len(s) > 0 {
		r, size := utf8.DecodeRuneInString(s)
		n += len(utf16.Encode([]rune{r}))
		s = s[size:]
	}
	return n
}

type analysisRequest struct {
	// Lang is the target language; empty for all.
	Lang string `json:"lang"`
	// File is the one file to analyse; empty for all. The editor asks for
	// the file it has open, to show each segment's best match.
	File string `json:"file,omitempty"`
	// Weights are the share of the full rate charged per band, in
	// percent; with them the report has weighted words.
	Weights map[analysis.Band]float64 `json:"weights,omitempty"`
}

// projectAnalysis analyses a project's files against the memories it looks
// up in: words by match band, repetitions, and with weights the weighted
// words, for each target language, or for one language and one
// file. Each report lists how every segment was counted, with the score
// and the memory of its best match.
//
// Access: read on the project, or work in it.
func (s *Server) projectAnalysis(w http.ResponseWriter, r *http.Request, me session) {
	p, _, ok := s.openProject(w, r, me)
	if !ok {
		return
	}
	var req analysisRequest
	if !readJSON(w, r, &req, maxBody) {
		return
	}
	ctx := r.Context()
	langs := p.Settings.TargetLangs
	if req.Lang != "" {
		langs = []string{canonicalTarget(p, req.Lang)}
	}
	out := []analysis.Report{}
	for _, lang := range langs {
		mems, err := p.Settings.MemoriesFor(lang, project.UseLookup, s.openMemory(ctx))
		if err != nil {
			s.fail(w, r, err)
			return
		}
		var files []analysis.File
		for _, f := range p.Settings.Files {
			if f.Role != project.Translatable || (f.TargetLang != "" && !strings.EqualFold(f.TargetLang, lang)) {
				continue
			}
			if req.File != "" && f.Name != req.File {
				continue
			}
			segs, err := s.cfg.Projects.Segments(ctx, p.ID, f.Name, lang)
			if err != nil {
				s.fail(w, r, err)
				return
			}
			af := analysis.File{Name: f.Name}
			for _, sg := range segs {
				af.Segments = append(af.Segments, analysis.Segment{ID: sg.ID, Source: sg.Source, Locked: sg.Locked})
			}
			files = append(files, af)
		}
		o := analysis.Options{SourceLang: p.Settings.SourceLang, TargetLang: lang, Context: analysis.ContextRule(p.Settings.Analysis.Context), Weights: req.Weights}
		rep, err := analysis.Analyze(ctx, files, mems, o, s.cfg.Now())
		if err != nil {
			invalid(w, "analysis", err.Error())
			return
		}
		out = append(out, rep)
	}
	writeJSON(w, http.StatusOK, out)
}
