// Package project describes translation projects as they are set up:
// their languages, files, translation memories per target language,
// terminology, quality checks and preparation, and the templates new
// projects start from. Validate checks a setup completely and names the
// field of every problem, so the desktop apps, the server and the browser
// client all accept and refuse the same setups. Pretranslate fills a
// project's segments from its memories the way the analysis counts them.
package project

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/4nonX/interlinea/langtag"
	"github.com/4nonX/interlinea/qa"
	"github.com/4nonX/interlinea/tm"
)

// Role says what a file is for.
type Role string

const (
	// Translatable files are segmented and translated.
	Translatable Role = "translatable"
	// Reference files (style guides, earlier versions, PDFs of the
	// layout) travel with the project for the translators to read.
	Reference Role = "reference"
	// Package is the Trados project package (.sdlppx) the project's
	// bilingual files came from, kept to write the return package.
	Package Role = "package"
)

// File is a file of the project.
type File struct {
	// Name is the file name, with the folder it came from if any
	// ("contracts/lease.docx"). Names are unique in a project, ignoring
	// case.
	Name string `json:"name"`
	Role Role   `json:"role"`
	// TargetLang is set for a bilingual file (XLIFF, a file of a Trados
	// package): it is translated into this language only. Empty for a
	// monolingual file, which is translated into every target language.
	TargetLang string `json:"targetLang,omitempty"`
	// Package is the file's path in the project's Trados package, for a
	// file that came from one ("en-GB/Vertrag.docx.sdlxliff").
	Package string `json:"package,omitempty"`
}

// MemorySetting is how a project uses one translation memory for one
// target language.
type MemorySetting struct {
	// Memory is the memory's name.
	Memory string `json:"memory"`
	// Lookup offers its matches while translating, in the analysis and
	// in pre-translation.
	Lookup bool `json:"lookup"`
	// Concordance searches it with the concordance search.
	Concordance bool `json:"concordance"`
	// Update stores confirmed translations in it.
	Update bool `json:"update"`
	// Penalty lowers its match scores by this many points (0 to 100).
	Penalty int `json:"penalty"`
}

// TermMode says where terms found while translating are stored.
type TermMode string

const (
	// Layered: a project termbase on top of the masters; its changes
	// are reviewed and merged into the masters later.
	Layered TermMode = "layered"
	// Direct: new terms go straight into one master.
	Direct TermMode = "direct"
	// Own: the project has its own termbase and uses no master.
	Own TermMode = "own"
)

// Location is where a termbase or memory is kept.
type Location string

const (
	Local  Location = "local"
	Server Location = "server"
)

// Terminology is the project's termbase setup.
type Terminology struct {
	Mode TermMode `json:"mode"`
	// Masters are the master termbases used, by ID.
	Masters []string `json:"masters"`
	// WriteTo is the master new terms go into in Direct mode.
	WriteTo string `json:"writeTo,omitempty"`
	// AddLanguages lists masters that gain the project languages they
	// lack. The masters are changed only when the user chose this.
	AddLanguages []string `json:"addLanguages,omitempty"`
	// Location is where the project termbase is kept (Layered, Own).
	Location Location `json:"location"`
	// Termbase is the project termbase's ID once it exists (Layered,
	// Own); empty until it is made, and in Direct mode.
	Termbase string `json:"termbase,omitempty"`
	// AutoUpdate takes in the masters' changes each time the project
	// opens (Layered). Otherwise they wait until the user updates.
	AutoUpdate bool `json:"autoUpdate"`
}

// Analysis is how the project's analysis counts.
type Analysis struct {
	// Context is the context match rule: "previous" (Trados) or "both"
	// (memoQ).
	Context string `json:"context"`
}

// QA is which quality checks run while translating, as qa.Options
// switches them: a check runs if it is on by default and not disabled, or
// enabled.
type QA struct {
	Disabled []qa.Kind `json:"disabled,omitempty"`
	Enabled  []qa.Kind `json:"enabled,omitempty"`
	// Rules are checks of the project's own.
	Rules []qa.UserRule `json:"rules,omitempty"`
	// Dictionaries names the spelling dictionary for each target
	// language; a language without one is not spell-checked.
	Dictionaries map[string]string `json:"dictionaries,omitempty"`
	// Words are always spelled right in this project (names, terms).
	Words []string `json:"words,omitempty"`
	// UneditedFuzzy warns when a fuzzy match is confirmed unchanged.
	UneditedFuzzy bool `json:"uneditedFuzzy"`
}

// Prepare is what happens to the files when the project is created.
type Prepare struct {
	// Pretranslate fills empty segments with the best memory match.
	Pretranslate bool `json:"pretranslate"`
	// MinScore is the lowest match inserted (50 to 100).
	MinScore int `json:"minScore"`
	// ConfirmExact confirms segments filled with a 100% match (context
	// matches included).
	ConfirmExact bool `json:"confirmExact"`
	// ConfirmContext confirms segments filled with a context match.
	ConfirmContext bool `json:"confirmContext"`
}

// Settings is a project as it is set up.
type Settings struct {
	Name        string `json:"name"`
	Client      string `json:"client,omitempty"`
	Description string `json:"description,omitempty"`
	// Due is the deadline, RFC 3339 ("2026-12-04T16:00:00Z").
	Due         string   `json:"due,omitempty"`
	SourceLang  string   `json:"sourceLang"`
	TargetLangs []string `json:"targetLangs"`
	Files       []File   `json:"files"`
	// Memories are the memories per target language, in priority order
	// (the first wins a tie between equal matches).
	Memories map[string][]MemorySetting `json:"memories"`
	// ProjectMemory gives the project its own memory per target
	// language, which collects its confirmed translations.
	ProjectMemory bool        `json:"projectMemory"`
	Terminology   Terminology `json:"terminology"`
	Analysis      Analysis    `json:"analysis"`
	QA            QA          `json:"qa"`
	Prepare       Prepare     `json:"prepare"`
	// Segmentation are the project's own rules for where sentences end,
	// applied to the files added to it.
	Segmentation []SegmentationRule `json:"segmentation,omitempty"`
}

// Catalog describes the memories and termbases a setup may refer to.
// Validate skips existence checks for a nil map.
type Catalog struct {
	// Memories by name.
	Memories map[string]tm.Info `json:"memories"`
	// Termbases by ID, with their languages.
	Termbases map[string][]string `json:"termbases"`
	// Dictionaries by name, with the language each spells.
	Dictionaries map[string]string `json:"dictionaries"`
}

// Problem is one thing wrong with a setup.
type Problem struct {
	// Field names the setting: "name", "targetLangs", "files[1].name",
	// "memories.en-GB[0].penalty", "terminology.writeTo".
	Field   string `json:"field"`
	Message string `json:"message"`
}

// Defaults are the settings a new project starts with.
func Defaults() Settings {
	return Settings{
		Memories:      map[string][]MemorySetting{},
		ProjectMemory: true,
		Terminology:   Terminology{Mode: Own, Location: Local},
		Analysis:      Analysis{Context: "previous"},
		QA:            QA{UneditedFuzzy: true},
		Prepare:       Prepare{MinScore: 100, ConfirmContext: true},
	}
}

func hasControl(s string) bool { return strings.IndexFunc(s, unicode.IsControl) >= 0 }

func primary(tag string) string {
	p, _, _ := strings.Cut(tag, "-")
	return strings.ToLower(p)
}

// Validate checks a setup and returns every problem, in field order. An
// empty result means the project can be created.
func Validate(s Settings, cat Catalog) []Problem {
	var ps []Problem
	add := func(field, format string, args ...any) {
		ps = append(ps, Problem{Field: field, Message: fmt.Sprintf(format, args...)})
	}
	if strings.TrimSpace(s.Name) == "" {
		add("name", "Give the project a name.")
	} else if hasControl(s.Name) {
		add("name", "The name can't contain line breaks or tabs.")
	}
	if hasControl(s.Client) {
		add("client", "The client can't contain line breaks or tabs.")
	}
	if s.Due != "" {
		if _, err := time.Parse(time.RFC3339, s.Due); err != nil {
			add("due", "The due date is not a valid date and time.")
		}
	}

	src, err := langtag.Normalize(s.SourceLang)
	if err != nil {
		add("sourceLang", "Choose the source language.")
	}
	targets := map[string]bool{}
	if len(s.TargetLangs) == 0 {
		add("targetLangs", "Choose at least one target language.")
	}
	for i, t := range s.TargetLangs {
		n, err := langtag.Normalize(t)
		switch {
		case err != nil:
			add(fmt.Sprintf("targetLangs[%d]", i), "%q is not a language.", t)
		case src != "" && n == src:
			add(fmt.Sprintf("targetLangs[%d]", i), "A target language can't be the same as the source language.")
		case targets[n]:
			add(fmt.Sprintf("targetLangs[%d]", i), "%s is chosen twice.", n)
		default:
			targets[n] = true
		}
	}

	names := map[string]bool{}
	translatable, packages := 0, 0
	for _, f := range s.Files {
		if f.Role == Package {
			packages++
		}
	}
	for i, f := range s.Files {
		field := fmt.Sprintf("files[%d]", i)
		name := strings.TrimSpace(f.Name)
		switch {
		case name == "":
			add(field+".name", "A file has no name.")
		case hasControl(name):
			add(field+".name", "The file name %q contains control characters.", name)
		case names[strings.ToLower(name)]:
			add(field+".name", "%s is in the project twice.", name)
		default:
			names[strings.ToLower(name)] = true
		}
		switch f.Role {
		case Translatable:
			translatable++
		case Reference:
			if f.TargetLang != "" {
				add(field+".targetLang", "A reference file is not translated, so it has no target language.")
			}
		case Package:
			if f.TargetLang != "" {
				add(field+".targetLang", "A project package is not translated itself, so it has no target language.")
			}
			if packages > 1 {
				add(field+".role", "A project holds the files of one project package, so it can be returned as one return package.")
			}
		default:
			add(field+".role", "%s must be a file to translate, a reference file or a project package.", name)
		}
		if f.Package != "" {
			switch {
			case f.Role != Translatable || f.TargetLang == "":
				add(field+".package", "%s is from a project package, so it is a bilingual file to translate.", name)
			case packages == 0:
				add(field+".package", "%s is from a project package, which is not in the project.", name)
			}
		}
		if f.Role == Translatable && f.TargetLang != "" {
			n, err := langtag.Normalize(f.TargetLang)
			if err != nil || !targets[n] {
				add(field+".targetLang", "%s is a bilingual file for %s, which is not a target language of the project.", name, f.TargetLang)
			}
		}
	}
	if translatable == 0 {
		add("files", "Add at least one file to translate.")
	}

	langs := make([]string, 0, len(s.Memories))
	for l := range s.Memories {
		langs = append(langs, l)
	}
	sort.Strings(langs)
	for _, l := range langs {
		n, err := langtag.Normalize(l)
		if err != nil || !targets[n] {
			add("memories."+l, "Memories are set for %s, which is not a target language of the project.", l)
			continue
		}
		seen := map[string]bool{}
		for i, m := range s.Memories[l] {
			field := fmt.Sprintf("memories.%s[%d]", l, i)
			if seen[m.Memory] {
				add(field+".memory", "%s is listed twice for %s.", m.Memory, n)
				continue
			}
			seen[m.Memory] = true
			if m.Penalty < 0 || m.Penalty > tm.MaxPenalty {
				add(field+".penalty", "The penalty must be between 0 and %d points.", tm.MaxPenalty)
			}
			if !m.Lookup && !m.Concordance && !m.Update {
				add(field, "%s is used for nothing: turn on lookup, concordance or updating, or remove it.", m.Memory)
			}
			if cat.Memories == nil {
				continue
			}
			info, ok := cat.Memories[m.Memory]
			if !ok {
				add(field+".memory", "There is no memory called %s.", m.Memory)
				continue
			}
			if (src != "" && primary(info.SourceLang) != primary(src)) || primary(info.TargetLang) != primary(n) {
				add(field+".memory", "%s is %s to %s, which doesn't fit %s to %s.", m.Memory, info.SourceLang, info.TargetLang, src, n)
			}
		}
	}

	t := s.Terminology
	masters := map[string]bool{}
	for i, id := range t.Masters {
		field := fmt.Sprintf("terminology.masters[%d]", i)
		if masters[id] {
			add(field, "A master is chosen twice.")
			continue
		}
		masters[id] = true
		if cat.Termbases != nil {
			if _, ok := cat.Termbases[id]; !ok {
				add(field, "There is no termbase %s.", id)
			}
		}
	}
	for _, id := range t.AddLanguages {
		if !masters[id] {
			add("terminology.addLanguages", "Languages can only be added to a master the project uses.")
			break
		}
	}
	switch t.Mode {
	case Layered:
		if len(t.Masters) == 0 {
			add("terminology.masters", "Choose the masters the project termbase is based on.")
		}
	case Direct:
		switch {
		case len(t.Masters) == 0:
			add("terminology.masters", "Choose the master new terms go into.")
		case !masters[t.WriteTo]:
			add("terminology.writeTo", "Choose which of the masters new terms go into.")
		case cat.Termbases != nil && !contains(t.AddLanguages, t.WriteTo):
			if missing := missingLangs(cat.Termbases[t.WriteTo], src, s.TargetLangs); len(missing) > 0 {
				add("terminology.writeTo", "New terms go into this master, but it has no %s. Add the languages to it or keep new terms in a project termbase.", strings.Join(missing, ", "))
			}
		}
	case Own:
		if len(t.Masters) > 0 {
			add("terminology.masters", "A project with its own termbase only uses no masters.")
		}
	default:
		add("terminology.mode", "Choose where new terms go.")
	}
	if t.Mode != Direct && t.Location != Local && t.Location != Server {
		add("terminology.location", "Choose where the project termbase is kept.")
	}
	switch {
	case t.Termbase == "":
	case t.Mode == Direct:
		add("terminology.termbase", "New terms go into a master, so the project has no termbase of its own.")
	case masters[t.Termbase]:
		add("terminology.termbase", "The project termbase can't be one of its masters.")
	case t.Location == Server && cat.Termbases != nil:
		if _, ok := cat.Termbases[t.Termbase]; !ok {
			add("terminology.termbase", "There is no termbase %s.", t.Termbase)
		}
	}

	if s.Analysis.Context != "previous" && s.Analysis.Context != "both" {
		add("analysis.context", "Choose which neighbouring segments make a context match.")
	}
	known := map[qa.Kind]bool{}
	for _, k := range qa.Kinds {
		known[k.Kind] = true
	}
	for _, k := range s.QA.Disabled {
		if !known[k] {
			add("qa.disabled", "%q is not a quality check.", k)
		}
	}
	for _, k := range s.QA.Enabled {
		if !known[k] {
			add("qa.enabled", "%q is not a quality check.", k)
		}
	}
	if len(s.Segmentation) > MaxSegmentationRules {
		add("segmentation", "A project has at most %d segmentation rules of its own.", MaxSegmentationRules)
	}
	for i, r := range s.Segmentation {
		if err := r.Check(); err != nil {
			add(fmt.Sprintf("segmentation[%d]", i), "%s", err.Error())
		}
	}
	ruleNames := map[string]bool{}
	for i, r := range s.QA.Rules {
		field := fmt.Sprintf("qa.rules[%d]", i)
		name := strings.TrimSpace(r.Name)
		switch {
		case name == "":
			add(field+".name", "Give the rule a name.")
		case ruleNames[strings.ToLower(name)]:
			add(field+".name", "Two rules are called %s.", name)
		default:
			ruleNames[strings.ToLower(name)] = true
		}
		if err := r.Check(); err != nil {
			add(field, "%s", ruleProblem(err))
		}
	}
	dictLangs := make([]string, 0, len(s.QA.Dictionaries))
	for l := range s.QA.Dictionaries {
		dictLangs = append(dictLangs, l)
	}
	sort.Strings(dictLangs)
	for _, l := range dictLangs {
		field := "qa.dictionaries." + l
		n, err := langtag.Normalize(l)
		if err != nil || !targets[n] {
			add(field, "A dictionary is set for %s, which is not a target language of the project.", l)
			continue
		}
		name := s.QA.Dictionaries[l]
		if cat.Dictionaries == nil {
			continue
		}
		dl, ok := cat.Dictionaries[name]
		if !ok {
			add(field, "There is no dictionary called %s.", name)
		} else if primary(dl) != primary(n) {
			add(field, "%s spells %s, not %s.", name, dl, n)
		}
	}
	for _, w := range s.QA.Words {
		if strings.TrimSpace(w) == "" || hasControl(w) {
			add("qa.words", "The project's words can't be empty or contain line breaks or tabs.")
			break
		}
	}
	if s.Prepare.Pretranslate && (s.Prepare.MinScore < 50 || s.Prepare.MinScore > 100) {
		add("prepare.minScore", "The lowest match to insert must be between 50%% and 100%%.")
	}
	return ps
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

// missingLangs are the project languages a termbase lacks, compared by
// language and region where the termbase has a region, by language only
// where it has none.
func missingLangs(have []string, source string, targets []string) []string {
	var out []string
	for _, l := range append([]string{source}, targets...) {
		if l == "" {
			continue
		}
		found := false
		for _, h := range have {
			if strings.EqualFold(h, l) || (!strings.Contains(h, "-") && primary(h) == primary(l)) {
				found = true
				break
			}
		}
		if !found {
			out = append(out, l)
		}
	}
	return out
}

// Use is what a memory is opened for.
type Use int

const (
	UseLookup Use = iota
	UseConcordance
	UseUpdate
)

// MemoriesFor returns the target language's memories used for u, in
// priority order, with their penalties applied for lookups. open opens a
// memory by name.
func (s Settings) MemoriesFor(target string, u Use, open func(name string) (tm.Memory, error)) ([]tm.Memory, error) {
	var out []tm.Memory
	for _, m := range s.Memories[target] {
		if (u == UseLookup && !m.Lookup) || (u == UseConcordance && !m.Concordance) || (u == UseUpdate && !m.Update) {
			continue
		}
		mem, err := open(m.Memory)
		if err != nil {
			return nil, fmt.Errorf("project: memory %s: %w", m.Memory, err)
		}
		if u == UseLookup {
			if mem, err = tm.Penalized(mem, m.Penalty); err != nil {
				return nil, err
			}
		}
		out = append(out, mem)
	}
	return out, nil
}

// Template is what a new project can start from: everything but the
// project's name, files and dates.
type Template struct {
	Name          string                     `json:"name"`
	Description   string                     `json:"description,omitempty"`
	Client        string                     `json:"client,omitempty"`
	SourceLang    string                     `json:"sourceLang"`
	TargetLangs   []string                   `json:"targetLangs"`
	Memories      map[string][]MemorySetting `json:"memories"`
	ProjectMemory bool                       `json:"projectMemory"`
	Terminology   Terminology                `json:"terminology"`
	Analysis      Analysis                   `json:"analysis"`
	QA            QA                         `json:"qa"`
	Prepare       Prepare                    `json:"prepare"`
	Segmentation  []SegmentationRule         `json:"segmentation,omitempty"`
}

// TemplateFrom makes a template of a project's settings.
func TemplateFrom(name string, s Settings) Template {
	// The project termbase belongs to the project; a new project makes
	// its own.
	t := cloneTerminology(s.Terminology)
	t.Termbase = ""
	return Template{
		Name:          strings.TrimSpace(name),
		Client:        s.Client,
		SourceLang:    s.SourceLang,
		TargetLangs:   append([]string(nil), s.TargetLangs...),
		Memories:      cloneMemories(s.Memories),
		ProjectMemory: s.ProjectMemory,
		Terminology:   t,
		Analysis:      s.Analysis,
		QA:            cloneQA(s.QA),
		Prepare:       s.Prepare,
		Segmentation:  append([]SegmentationRule(nil), s.Segmentation...),
	}
}

// Settings starts a project from the template; name, files and due date
// are still to be set.
func (t Template) Settings() Settings {
	return Settings{
		Client:        t.Client,
		SourceLang:    t.SourceLang,
		TargetLangs:   append([]string(nil), t.TargetLangs...),
		Memories:      cloneMemories(t.Memories),
		ProjectMemory: t.ProjectMemory,
		Terminology:   cloneTerminology(t.Terminology),
		Analysis:      t.Analysis,
		QA:            cloneQA(t.QA),
		Prepare:       t.Prepare,
		Segmentation:  append([]SegmentationRule(nil), t.Segmentation...),
	}
}

// ValidateTemplate checks a template as a project setup without files.
func ValidateTemplate(t Template, cat Catalog) []Problem {
	var ps []Problem
	if strings.TrimSpace(t.Name) == "" {
		ps = append(ps, Problem{Field: "templateName", Message: "Give the template a name."})
	} else if hasControl(t.Name) {
		ps = append(ps, Problem{Field: "templateName", Message: "The template name can't contain line breaks or tabs."})
	}
	s := t.Settings()
	s.Name = "template"
	s.Files = []File{{Name: "file", Role: Translatable}}
	return append(ps, Validate(s, cat)...)
}

func cloneMemories(m map[string][]MemorySetting) map[string][]MemorySetting {
	out := make(map[string][]MemorySetting, len(m))
	for k, v := range m {
		out[k] = append([]MemorySetting(nil), v...)
	}
	return out
}

func cloneQA(q QA) QA {
	q.Disabled = append([]qa.Kind(nil), q.Disabled...)
	q.Enabled = append([]qa.Kind(nil), q.Enabled...)
	q.Rules = append([]qa.UserRule(nil), q.Rules...)
	q.Words = append([]string(nil), q.Words...)
	if q.Dictionaries != nil {
		d := make(map[string]string, len(q.Dictionaries))
		for k, v := range q.Dictionaries {
			d[k] = v
		}
		q.Dictionaries = d
	}
	return q
}

// ruleProblem is a rule's error without the package prefix, for people.
func ruleProblem(err error) string {
	msg := strings.TrimPrefix(err.Error(), "qa: ")
	if !strings.HasSuffix(msg, ".") {
		msg += "."
	}
	return strings.ToUpper(msg[:1]) + msg[1:]
}

// QAOptions are the qa package's options for checking a target language
// of the project. The spelling dictionary, s.QA.Dictionaries[target], is
// for the caller to open and set as Spell.
func (s Settings) QAOptions(target string) qa.Options {
	q := cloneQA(s.QA)
	return qa.Options{
		SourceLang:  s.SourceLang,
		TargetLang:  target,
		Disabled:    q.Disabled,
		Enabled:     q.Enabled,
		Rules:       q.Rules,
		SpellIgnore: q.Words,
	}
}

func cloneTerminology(t Terminology) Terminology {
	t.Masters = append([]string(nil), t.Masters...)
	t.AddLanguages = append([]string(nil), t.AddLanguages...)
	return t
}
