package project

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/qa"
	"github.com/4nonX/interlinea/tm"
)

var ctx = context.Background()

func valid() Settings {
	s := Defaults()
	s.Name = "Mietvertrag Hofmann"
	s.Client = "Kanzlei Weber"
	s.Due = "2026-12-04T16:00:00Z"
	s.SourceLang = "de-DE"
	s.TargetLangs = []string{"en-GB", "fr-FR"}
	s.Files = []File{
		{Name: "Mietvertrag.docx", Role: Translatable},
		{Name: "Anlage/Hausordnung.docx", Role: Translatable},
		{Name: "Styleguide.pdf", Role: Reference},
		{Name: "return.sdlxliff", Role: Translatable, TargetLang: "en-GB"},
	}
	s.Memories = map[string][]MemorySetting{
		"en-GB": {{Memory: "Weber DE-EN", Lookup: true, Concordance: true, Update: true}, {Memory: "Alt DE-EN", Lookup: true, Penalty: 5}},
		"fr-FR": {{Memory: "Weber DE-FR", Lookup: true, Update: true}},
	}
	s.Terminology = Terminology{Mode: Layered, Masters: []string{"tb-weber"}, Location: Server, AutoUpdate: true}
	return s
}

func catalog() Catalog {
	return Catalog{
		Memories: map[string]tm.Info{
			"Weber DE-EN": {Name: "Weber DE-EN", SourceLang: "de-DE", TargetLang: "en-GB"},
			"Alt DE-EN":   {Name: "Alt DE-EN", SourceLang: "de", TargetLang: "en-US"},
			"Weber DE-FR": {Name: "Weber DE-FR", SourceLang: "de-DE", TargetLang: "fr-FR"},
			"Weber EN-DE": {Name: "Weber EN-DE", SourceLang: "en-GB", TargetLang: "de-DE"},
		},
		Termbases: map[string][]string{
			"tb-weber":   {"de-DE", "en-GB"},
			"tb-medtech": {"de", "en", "fr"},
		},
		Dictionaries: map[string]string{
			"English (GB)": "en-GB",
			"Français":     "fr",
		},
	}
}

func fields(ps []Problem) []string {
	var out []string
	for _, p := range ps {
		out = append(out, p.Field)
	}
	return out
}

func TestValidSetup(t *testing.T) {
	if ps := Validate(valid(), catalog()); len(ps) != 0 {
		t.Fatalf("valid setup has problems: %+v", ps)
	}
	// Without a catalog only the setup itself is checked.
	if ps := Validate(valid(), Catalog{}); len(ps) != 0 {
		t.Fatalf("problems without catalog: %+v", ps)
	}
}

func TestValidateProblems(t *testing.T) {
	cases := []struct {
		name   string
		change func(*Settings)
		want   []string
	}{
		{"no name", func(s *Settings) { s.Name = "  " }, []string{"name"}},
		{"name with a tab", func(s *Settings) { s.Name = "a\tb" }, []string{"name"}},
		{"client with a line break", func(s *Settings) { s.Client = "a\nb" }, []string{"client"}},
		{"bad due date", func(s *Settings) { s.Due = "next Friday" }, []string{"due"}},
		{"no source", func(s *Settings) { s.SourceLang = "" }, []string{"sourceLang"}},
		{"no targets", func(s *Settings) {
			s.TargetLangs = nil
			s.Memories = nil
			s.Files = s.Files[:3]
		}, []string{"targetLangs"}},
		{"target equals source", func(s *Settings) { s.TargetLangs = []string{"en-GB", "de_de"} }, []string{"targetLangs[1]", "memories.fr-FR"}},
		{"target twice", func(s *Settings) { s.TargetLangs = []string{"en-GB", "fr-FR", "EN-gb"} }, []string{"targetLangs[2]"}},
		{"bad target", func(s *Settings) { s.TargetLangs = append(s.TargetLangs, "123") }, []string{"targetLangs[2]"}},
		{"no files", func(s *Settings) { s.Files = nil }, []string{"files"}},
		{"only reference files", func(s *Settings) { s.Files = s.Files[2:3] }, []string{"files"}},
		{"duplicate file", func(s *Settings) { s.Files = append(s.Files, File{Name: "mietvertrag.DOCX", Role: Translatable}) }, []string{"files[4].name"}},
		{"unnamed file", func(s *Settings) { s.Files[0].Name = "" }, []string{"files[0].name"}},
		{"bad role", func(s *Settings) { s.Files[0].Role = "other" }, []string{"files[0].role"}},
		{"reference with target", func(s *Settings) { s.Files[2].TargetLang = "en-GB" }, []string{"files[2].targetLang"}},
		{"bilingual for another language", func(s *Settings) { s.Files[3].TargetLang = "it-IT" }, []string{"files[3].targetLang"}},
		{"memories for a language not in the project", func(s *Settings) {
			s.Memories["it-IT"] = []MemorySetting{{Memory: "Weber DE-EN", Lookup: true}}
		}, []string{"memories.it-IT"}},
		{"memory twice", func(s *Settings) {
			s.Memories["en-GB"] = append(s.Memories["en-GB"], MemorySetting{Memory: "Weber DE-EN", Lookup: true})
		}, []string{"memories.en-GB[2].memory"}},
		{"penalty too high", func(s *Settings) { s.Memories["en-GB"][1].Penalty = 101 }, []string{"memories.en-GB[1].penalty"}},
		{"negative penalty", func(s *Settings) { s.Memories["en-GB"][1].Penalty = -1 }, []string{"memories.en-GB[1].penalty"}},
		{"memory used for nothing", func(s *Settings) { s.Memories["fr-FR"][0] = MemorySetting{Memory: "Weber DE-FR"} }, []string{"memories.fr-FR[0]"}},
		{"unknown memory", func(s *Settings) { s.Memories["fr-FR"][0].Memory = "Nope" }, []string{"memories.fr-FR[0].memory"}},
		{"memory of the wrong pair", func(s *Settings) { s.Memories["fr-FR"][0].Memory = "Weber EN-DE" }, []string{"memories.fr-FR[0].memory"}},
		{"memory of another target", func(s *Settings) { s.Memories["fr-FR"][0].Memory = "Weber DE-EN" }, []string{"memories.fr-FR[0].memory"}},
		{"layered without masters", func(s *Settings) { s.Terminology.Masters = nil }, []string{"terminology.masters"}},
		{"unknown master", func(s *Settings) { s.Terminology.Masters = []string{"tb-x"} }, []string{"terminology.masters[0]"}},
		{"master twice", func(s *Settings) { s.Terminology.Masters = []string{"tb-weber", "tb-weber"} }, []string{"terminology.masters[1]"}},
		{"languages added to a master not used", func(s *Settings) { s.Terminology.AddLanguages = []string{"tb-medtech"} }, []string{"terminology.addLanguages"}},
		{"direct without a master to write to", func(s *Settings) {
			s.Terminology = Terminology{Mode: Direct, Masters: []string{"tb-medtech"}}
		}, []string{"terminology.writeTo"}},
		{"direct into a master that lacks a language", func(s *Settings) {
			s.Terminology = Terminology{Mode: Direct, Masters: []string{"tb-weber"}, WriteTo: "tb-weber"}
		}, []string{"terminology.writeTo"}},
		{"own with masters", func(s *Settings) { s.Terminology.Mode = Own }, []string{"terminology.masters"}},
		{"no mode", func(s *Settings) { s.Terminology.Mode = "" }, []string{"terminology.mode"}},
		{"no location", func(s *Settings) { s.Terminology.Location = "" }, []string{"terminology.location"}},
		{"bad context rule", func(s *Settings) { s.Analysis.Context = "next" }, []string{"analysis.context"}},
		{"unknown disabled check", func(s *Settings) { s.QA.Disabled = []qa.Kind{"vibes"} }, []string{"qa.disabled"}},
		{"unknown enabled check", func(s *Settings) { s.QA.Enabled = []qa.Kind{"vibes"} }, []string{"qa.enabled"}},
		{"rule without a name", func(s *Settings) {
			s.QA.Rules = []qa.UserRule{{Target: "x", Mode: "must"}}
		}, []string{"qa.rules[0].name"}},
		{"two rules of one name", func(s *Settings) {
			s.QA.Rules = []qa.UserRule{{Name: "A", Target: "x", Mode: "must"}, {Name: "a", Target: "y", Mode: "must"}}
		}, []string{"qa.rules[1].name"}},
		{"rule that doesn't compile", func(s *Settings) {
			s.QA.Rules = []qa.UserRule{{Name: "A", Target: "(", Mode: "must"}}
		}, []string{"qa.rules[0]"}},
		{"count rule without a source pattern", func(s *Settings) {
			s.QA.Rules = []qa.UserRule{{Name: "A", Target: "x", Mode: "count"}}
		}, []string{"qa.rules[0]"}},
		{"dictionary for a language not in the project", func(s *Settings) {
			s.QA.Dictionaries = map[string]string{"it-IT": "Français"}
		}, []string{"qa.dictionaries.it-IT"}},
		{"unknown dictionary", func(s *Settings) {
			s.QA.Dictionaries = map[string]string{"en-GB": "Nope"}
		}, []string{"qa.dictionaries.en-GB"}},
		{"dictionary of another language", func(s *Settings) {
			s.QA.Dictionaries = map[string]string{"en-GB": "Français"}
		}, []string{"qa.dictionaries.en-GB"}},
		{"empty project word", func(s *Settings) { s.QA.Words = []string{"Hofmann", " "} }, []string{"qa.words"}},
		{"pre-translation threshold too low", func(s *Settings) {
			s.Prepare.Pretranslate = true
			s.Prepare.MinScore = 40
		}, []string{"prepare.minScore"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := valid()
			c.change(&s)
			got := fields(Validate(s, catalog()))
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("problems %v, want %v: %+v", got, c.want, Validate(s, catalog()))
			}
		})
	}
}

func TestDirectAllowedWhenLanguagesAreAdded(t *testing.T) {
	s := valid()
	s.Terminology = Terminology{Mode: Direct, Masters: []string{"tb-weber"}, WriteTo: "tb-weber", AddLanguages: []string{"tb-weber"}}
	if ps := Validate(s, catalog()); len(ps) != 0 {
		t.Fatalf("%+v", ps)
	}
	// A master without regions covers every region of its languages.
	s.Terminology = Terminology{Mode: Direct, Masters: []string{"tb-medtech"}, WriteTo: "tb-medtech"}
	if ps := Validate(s, catalog()); len(ps) != 0 {
		t.Fatalf("%+v", ps)
	}
	// In Direct mode there is no project termbase, so no location.
	s.Terminology.Location = ""
	if ps := Validate(s, catalog()); len(ps) != 0 {
		t.Fatalf("%+v", ps)
	}
}

func TestSettingsJSON(t *testing.T) {
	b, err := json.Marshal(valid())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"targetLangs":["en-GB","fr-FR"]`, `"penalty":5`, `"mode":"layered"`, `"autoUpdate":true`, `"targetLang":"en-GB"`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("JSON lacks %s: %s", want, b)
		}
	}
	var back Settings
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(back, valid()) {
		t.Fatalf("round trip changed the settings:\n%+v\n%+v", back, valid())
	}
}

func TestTemplates(t *testing.T) {
	s := valid()
	s.QA = QA{Disabled: []qa.Kind{qa.Capitalization}, Enabled: []qa.Kind{qa.LengthRatio}}
	s.Prepare = Prepare{Pretranslate: true, MinScore: 95, ConfirmContext: true}
	tpl := TemplateFrom("  Weber leases  ", s)
	if tpl.Name != "Weber leases" {
		t.Fatalf("name %q", tpl.Name)
	}
	if ps := ValidateTemplate(tpl, catalog()); len(ps) != 0 {
		t.Fatalf("%+v", ps)
	}
	got := tpl.Settings()
	if got.Name != "" || got.Due != "" || len(got.Files) != 0 {
		t.Fatalf("template carries project data: %+v", got)
	}
	want := s
	want.Name, want.Due, want.Files, want.Description = "", "", nil, ""
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("template settings:\n%+v\nwant\n%+v", got, want)
	}
	// The template is a copy: changing the project afterwards leaves it alone.
	s.Memories["en-GB"][0].Penalty = 9
	s.Terminology.Masters[0] = "tb-medtech"
	s.QA.Disabled[0] = qa.Spelling
	if tpl.Memories["en-GB"][0].Penalty != 0 || tpl.Terminology.Masters[0] != "tb-weber" || tpl.QA.Disabled[0] != qa.Capitalization {
		t.Fatal("template shares data with the project")
	}
	if ps := ValidateTemplate(Template{Name: "", SourceLang: "de", TargetLangs: []string{"en"}, Terminology: Terminology{Mode: Own, Location: Local}, Analysis: Analysis{Context: "previous"}}, catalog()); !reflect.DeepEqual(fields(ps), []string{"templateName"}) {
		t.Fatalf("%+v", ps)
	}
}

func TestValidQASettings(t *testing.T) {
	s := valid()
	s.QA.Rules = []qa.UserRule{{Name: "Euro sign", Source: `EUR`, Target: `€`, Mode: "count", Severity: qa.Warning}}
	s.QA.Dictionaries = map[string]string{"en-GB": "English (GB)", "fr-FR": "Français"}
	s.QA.Words = []string{"Hofmann"}
	if ps := Validate(s, catalog()); len(ps) != 0 {
		t.Fatalf("%+v", ps)
	}
	s.QA.Rules[0].Target = "("
	ps := Validate(s, catalog())
	if len(ps) != 1 || !strings.HasPrefix(ps[0].Message, "Rule \"Euro sign\": error parsing regexp") {
		t.Fatalf("rule message: %+v", ps)
	}
}

func TestQAOptions(t *testing.T) {
	s := valid()
	s.QA = QA{
		Disabled: []qa.Kind{qa.Capitalization},
		Enabled:  []qa.Kind{qa.LengthRatio},
		Rules:    []qa.UserRule{{Name: "no Sie", Target: `\bSie\b`, Mode: "must-not"}},
		Words:    []string{"Hofmann"},
	}
	o := s.QAOptions("fr-FR")
	if o.SourceLang != "de-DE" || o.TargetLang != "fr-FR" || !reflect.DeepEqual(o.Disabled, s.QA.Disabled) || !reflect.DeepEqual(o.Enabled, s.QA.Enabled) ||
		!reflect.DeepEqual(o.Rules, s.QA.Rules) || !reflect.DeepEqual(o.SpellIgnore, s.QA.Words) {
		t.Fatalf("%+v", o)
	}
	o.Rules[0].Name = "changed"
	if s.QA.Rules[0].Name != "no Sie" {
		t.Fatal("options share the project's rules")
	}
	issues, err := qa.Check([]qa.Segment{{ID: "1", Source: content.Text("Der Mieter zahlt."), Target: content.Text("the tenant pays.")}}, o)
	if err != nil {
		t.Fatal(err)
	}
	for _, i := range issues {
		if i.Kind == qa.Capitalization {
			t.Fatal("a disabled check ran")
		}
	}
}

func mem(t *testing.T, name, src, tgt string, units ...tm.Unit) *tm.InMemory {
	t.Helper()
	m, err := tm.NewInMemory(tm.Info{Name: name, SourceLang: src, TargetLang: tgt})
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range units {
		if _, err := m.Add(ctx, u, tm.KeepBoth); err != nil {
			t.Fatal(err)
		}
	}
	return m
}

func unit(src, tgt string, attrs ...tm.Attribute) tm.Unit {
	return tm.Unit{Source: content.Text(src), Target: content.Text(tgt), Attributes: attrs}
}

func TestMemoriesFor(t *testing.T) {
	s := valid()
	opened := map[string]tm.Memory{
		"Weber DE-EN": mem(t, "Weber DE-EN", "de-DE", "en-GB", unit("Die Miete ist fällig.", "The rent is due.")),
		"Alt DE-EN":   mem(t, "Alt DE-EN", "de-DE", "en-US", unit("Die Miete ist fällig.", "Rent is due.")),
	}
	open := func(n string) (tm.Memory, error) {
		m, ok := opened[n]
		if !ok {
			return nil, errors.New("not open")
		}
		return m, nil
	}
	look, err := s.MemoriesFor("en-GB", UseLookup, open)
	if err != nil || len(look) != 2 || tm.Penalty(look[0]) != 0 || tm.Penalty(look[1]) != 5 {
		t.Fatalf("lookup memories %v %v", look, err)
	}
	conc, _ := s.MemoriesFor("en-GB", UseConcordance, open)
	upd, _ := s.MemoriesFor("en-GB", UseUpdate, open)
	if len(conc) != 1 || len(upd) != 1 || conc[0].Info().Name != "Weber DE-EN" || tm.Penalty(upd[0]) != 0 {
		t.Fatalf("concordance %v, update %v", conc, upd)
	}
	if _, err := s.MemoriesFor("fr-FR", UseLookup, open); err == nil || !strings.Contains(err.Error(), "Weber DE-FR") {
		t.Fatalf("missing memory: %v", err)
	}
	if none, err := s.MemoriesFor("it-IT", UseLookup, open); err != nil || len(none) != 0 {
		t.Fatalf("language without memories: %v %v", none, err)
	}
}

func TestPretranslate(t *testing.T) {
	ctxAttr := func(prev string) tm.Attribute { return tm.Attribute{Name: "x-context-pre", Value: prev} }
	main := mem(t, "Weber", "de-DE", "en-GB",
		unit("Der Mieter zahlt die Miete.", "The tenant pays the rent.", ctxAttr("Mietvertrag.")),
		unit("Die Kaution beträgt drei Monatsmieten.", "The deposit is three months' rent."),
		unit("Der Vermieter haftet nicht für Schäden.", "The landlord is not liable for damage."),
	)
	old := mem(t, "Alt", "de-DE", "en-GB",
		unit("Die Wohnung wird renoviert übergeben.", "The flat is handed over renovated."),
	)
	penalized, _ := tm.Penalized(old, 3)
	file := FileSegments{Name: "Mietvertrag.docx", Segments: []Segment{
		{ID: "1", Source: content.Text("Mietvertrag."), Status: StatusNew},
		{ID: "2", Source: content.Text("Der Mieter zahlt die Miete."), Status: StatusNew},
		{ID: "3", Source: content.Text("Die Kaution beträgt drei Monatsmieten."), Status: StatusNew},
		{ID: "4", Source: content.Text("Der Vermieter haftet nicht für Schäden am Auto."), Status: StatusNew},
		{ID: "5", Source: content.Text("Die Wohnung wird renoviert übergeben."), Status: StatusNew},
		{ID: "6", Source: content.Text("Die Kaution beträgt drei Monatsmieten."), Status: StatusDraft, Target: content.Text("Mine.")},
		{ID: "7", Source: content.Text("Die Kaution beträgt drei Monatsmieten."), Status: StatusNew, Locked: true},
	}}
	memories := []tm.Memory{main, penalized}

	got, err := Pretranslate(ctx, []FileSegments{file}, memories, Prepare{MinScore: 75, ConfirmContext: true}, "previous")
	if err != nil {
		t.Fatal(err)
	}
	type row struct {
		id      string
		score   int
		context bool
		status  Status
		memory  string
	}
	var rows []row
	for _, x := range got {
		rows = append(rows, row{x.ID, x.Score, x.Context, x.Status, x.Memory})
	}
	want := []row{
		{"2", 100, true, StatusConfirmed, "Weber"},
		{"3", 100, false, StatusDraft, "Weber"},
		{"5", 97, false, StatusDraft, "Alt"},
	}
	if len(rows) < 3 || !reflect.DeepEqual(rows[:2], want[:2]) {
		t.Fatalf("got %+v", rows)
	}
	// Segment 4 is a fuzzy match somewhere between 75 and 99.
	var fuzzy *Translation
	for i := range got {
		if got[i].ID == "4" {
			fuzzy = &got[i]
		}
	}
	if fuzzy == nil || fuzzy.Score < 75 || fuzzy.Score >= 100 || fuzzy.Status != StatusDraft || fuzzy.Target.Plain() != "The landlord is not liable for damage." {
		t.Fatalf("fuzzy = %+v", fuzzy)
	}
	if last := rows[len(rows)-1]; last != want[2] {
		t.Fatalf("penalized match = %+v", last)
	}
	for _, r := range rows {
		if r.id == "1" || r.id == "6" || r.id == "7" {
			t.Fatalf("segment %s should be left alone", r.id)
		}
	}

	// Confirming exact matches confirms context matches too; a higher
	// threshold leaves the fuzzy and the penalized match out.
	got, _ = Pretranslate(ctx, []FileSegments{file}, memories, Prepare{MinScore: 98, ConfirmExact: true}, "previous")
	if len(got) != 2 || got[0].Status != StatusConfirmed || got[1].Status != StatusConfirmed || got[1].ID != "3" {
		t.Fatalf("exact = %+v", got)
	}
	// Under the memoQ rule the context match also needs the next segment.
	got, _ = Pretranslate(ctx, []FileSegments{file}, memories, Prepare{MinScore: 100, ConfirmContext: true}, "both")
	if len(got) != 2 || got[0].Context || got[0].Status != StatusDraft {
		t.Fatalf("both = %+v", got)
	}
	if _, err := Pretranslate(ctx, []FileSegments{file}, memories, Prepare{MinScore: 49}, "previous"); err == nil {
		t.Fatal("threshold 49 accepted")
	}
	if _, err := Pretranslate(ctx, []FileSegments{file}, memories, Prepare{MinScore: 80}, "next"); err == nil {
		t.Fatal("bad rule accepted")
	}
	if none, err := Pretranslate(ctx, []FileSegments{file}, nil, Prepare{MinScore: 80}, "previous"); err != nil || none != nil {
		t.Fatalf("no memories: %v %v", none, err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := Pretranslate(cancelled, []FileSegments{file}, memories, Prepare{MinScore: 80}, "previous"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled: %v", err)
	}
}
