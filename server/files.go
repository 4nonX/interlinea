package server

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"unicode"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter"
	"github.com/4nonX/interlinea/filter/docx"
	"github.com/4nonX/interlinea/filter/formats"
	"github.com/4nonX/interlinea/filter/mqpackage"
	"github.com/4nonX/interlinea/filter/sdlpackage"
	"github.com/4nonX/interlinea/filter/xliff"
	"github.com/4nonX/interlinea/langtag"
	"github.com/4nonX/interlinea/project"
)

// The server reads the files of a project itself. An app may send a
// translatable file with its segments (the web app reads files in the
// browser); the server then checks that they are the file's and keeps the
// file's own source text, tags and locks, taking only translations and
// their status from the app. An app that sends a file without segments
// (the desktop app) gets them made here, for every target language.

// fileProblem says what is wrong with a file.
type fileProblem struct{ msg string }

func (p *fileProblem) Error() string { return p.msg }

func problemf(format string, args ...any) error {
	return &fileProblem{fmt.Sprintf(format, args...)}
}

// plainKey compares source text as people read it: tags and spacing aside.
func plainKey(s string) string {
	return strings.Join(strings.FieldsFunc(s, unicode.IsSpace), " ")
}

// formatOf is the format of a file by its name: a document format (docx,
// pptx, xlsx, html, idml), xliff, mqxlz (a memoQ XLIFF in its archive),
// package (a Trados or memoQ package) or txt.
func formatOf(name string) string {
	f := formats.Of(name)
	if formats.Package(f) {
		return "package"
	}
	return f
}

// formatOfFile is formatOf for a file whose content is at hand: a Word
// document that is a bilingual table for review is a bilingual file.
func formatOfFile(name string, data []byte) string {
	if f := formatOf(name); f != formats.Word {
		return f
	}
	return formats.OfFile(name, data)
}

// prepareFiles reads the uploaded files of a project with the given source
// and target languages: a Trados package becomes the package and its
// bilingual files, and each translatable file gets its segments, made or
// checked.
//
// A document whose segments the server makes is cut by the project's own
// segmentation rules; segments the app sent are taken as the app has them.
func prepareFiles(files []fileUpload, sourceLang string, targets []string, rules []project.SegmentationRule) ([]fileUpload, error) {
	var out []fileUpload
	for _, f := range files {
		if f.Role == project.Package {
			fs, err := expandPackage(f, targets)
			if err != nil {
				return nil, err
			}
			out = append(out, fs...)
			continue
		}
		sent := len(f.Segments) > 0
		if err := prepareFile(&f, sourceLang, targets); err != nil {
			return nil, err
		}
		if !sent && formats.Document(f.Format) {
			for lang, segs := range f.Segments {
				f.Segments[lang] = project.ApplySegmentation(segs, rules, sourceLang)
			}
		}
		out = append(out, f)
	}
	return out, nil
}

// expandPackage turns a Trados or memoQ package into the package file and
// the bilingual files of its target languages that are the project's.
func expandPackage(f fileUpload, targets []string) ([]fileUpload, error) {
	if formatOf(f.Name) != "package" {
		return nil, problemf("%s is not a project package (.sdlppx) or a handoff package (.mqout).", f.Name)
	}
	type member struct{ path, lang string }
	var members []member
	var sourceLang string
	var read func(path string) ([]byte, error)
	kind := formats.Of(f.Name)
	if kind == formats.MemoQPackage {
		pkg, err := mqpackage.Open(f.Data)
		if err != nil {
			return nil, problemf("%s could not be read as a handoff package: %v", f.Name, err)
		}
		sourceLang = pkg.SourceLang
		for _, pf := range pkg.Files {
			members = append(members, member{pf.Path, pf.Lang})
		}
		read = func(path string) ([]byte, error) { return mqpackage.Read(f.Data, path) }
	} else {
		pkg, err := sdlpackage.Open(f.Data)
		if err != nil {
			return nil, problemf("%s could not be read as a project package: %v", f.Name, err)
		}
		sourceLang = pkg.SourceLang
		for _, pf := range pkg.Files {
			if pf.Target && strings.HasSuffix(strings.ToLower(pf.Path), ".sdlxliff") {
				members = append(members, member{pf.Path, pf.Lang})
			}
		}
		read = func(path string) ([]byte, error) { return sdlpackage.Read(f.Data, path) }
	}
	f.Format, f.TargetLang, f.Segments = kind, "", nil
	out := []fileUpload{f}
	for _, m := range members {
		lang, ok := projectTarget(m.lang, targets)
		if !ok {
			continue
		}
		data, err := read(m.path)
		if err != nil {
			return nil, problemf("%s in %s could not be read: %v", m.path, f.Name, err)
		}
		// A memoQ document is stored as the memoQ XLIFF it is, whether or
		// not the package kept it in an archive.
		name := m.path
		if strings.HasSuffix(strings.ToLower(name), ".mqxlz") {
			name = name[:len(name)-len(".mqxlz")] + ".mqxliff"
		}
		b := fileUpload{Name: name, Role: project.Translatable, TargetLang: lang, Format: "xliff", Data: data, Package: m.path}
		if err := prepareFile(&b, sourceLang, targets); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	if len(out) == 1 {
		return nil, problemf("%s has no files to translate into the project's target languages.", f.Name)
	}
	return out, nil
}

// projectTarget finds lang among the project's target languages.
func projectTarget(lang string, targets []string) (string, bool) {
	n, err := langtag.Normalize(lang)
	if err != nil {
		return "", false
	}
	for _, t := range targets {
		if m, err := langtag.Normalize(t); err == nil && m == n {
			return t, true
		}
	}
	return "", false
}

// prepareFile makes or checks a translatable file's segments.
func prepareFile(f *fileUpload, sourceLang string, targets []string) error {
	if f.Role != project.Translatable {
		return nil
	}
	want := formatOfFile(f.Name, f.Data)
	if f.Format != "" && f.Format != want {
		return problemf("%s is sent as a %q file, but it is a %q file.", f.Name, f.Format, want)
	}
	f.Format = want
	if f.Format == "package" {
		return problemf("%s is a package: send it as a package, not as a file to translate.", f.Name)
	}
	switch f.Format {
	case "xliff", formats.MemoQArchive:
		doc := f.Data
		if f.Format == formats.MemoQArchive {
			var err error
			if doc, err = mqpackage.ReadArchive(f.Data); err != nil {
				return problemf("%s could not be read: %v", f.Name, err)
			}
		}
		ex, err := xliff.Extract(doc)
		if err != nil {
			return problemf("%s could not be read: %v", f.Name, err)
		}
		if f.TargetLang == "" {
			f.TargetLang = ex.TargetLang
		}
		if f.TargetLang == "" && len(targets) > 0 {
			// A file that names no target language yet (a TagEditor file
			// never segmented) is for the project's first.
			f.TargetLang = targets[0]
		}
		lang, ok := projectTarget(f.TargetLang, targets)
		if !ok {
			return problemf("%s is a bilingual file for %s, which is not a target language of the project.", f.Name, f.TargetLang)
		}
		f.TargetLang = lang
		return fill(f, ex, []string{lang})
	}
	ex, err := formats.Extract(f.Format, f.Data, sourceLang)
	if err != nil {
		return problemf("%s could not be read: %v", f.Name, err)
	}
	if f.Format == formats.Word && len(f.Segments) > 0 && !matches(ex, f.Segments) {
		// The app may have read the file with tracked changes kept.
		if kept, err := docx.Extract(f.Data, sourceLang, docx.Options{KeepRevisions: true}); err == nil && matches(kept, f.Segments) {
			ex = kept
		}
	}
	return fill(f, ex, targets)
}

// fill gives a file the extraction's segments for each language, or checks
// and completes the ones the app sent.
func fill(f *fileUpload, ex *filter.Extraction, langs []string) error {
	// Trados, memoQ, Phrase, Wordfast, TagEditor and Déjà Vu keep their
	// own record of every segment.
	fixed := xliff.Fixed(ex.Format)
	if len(f.Segments) == 0 {
		if len(ex.Units) == 0 {
			return problemf("%s has no text to translate.", f.Name)
		}
		f.Segments = map[string][]project.Segment{}
		for _, l := range langs {
			f.Segments[l] = unitSegments(ex.Units, fixed)
		}
		return nil
	}
	for lang, segs := range f.Segments {
		// Segments the translator split or joined stand for the file's
		// units all the same.
		units := project.Units(segs)
		if len(units) != len(ex.Units) {
			return problemf("%s has %d segments, not %d as sent for %s.", f.Name, len(ex.Units), len(units), lang)
		}
		first := 0 // the first unit of the stretch the unit is in
		for i, got := range units {
			u := ex.Units[i]
			if got.ID != u.ID {
				return problemf("Segment %d of %s (%s) is not the file's: the file has %q.", i+1, f.Name, lang, u.Source.Plain())
			}
			fromFile := unitSegment(u, fixed)
			if i == 0 || got.Segments[0] != units[i-1].Segments[0] {
				first = i
			}
			if sg := &segs[got.Segments[0]]; len(got.Segments) == 1 && sg.Split == nil && len(sg.Joined) == 0 {
				if plainKey(got.Source.Plain()) != plainKey(u.Source.Plain()) {
					return problemf("Segment %d of %s (%s) is not the file's: the file has %q.", i+1, f.Name, lang, u.Source.Plain())
				}
				sg.Source = u.Source
				sg.Locked = sg.Locked || u.Locked
				sg.Block, sg.MaxLength, sg.Notes, sg.File = fromFile.Block, fromFile.MaxLength, fromFile.Notes, fromFile.File
				sg.Timing = fromFile.Timing
				continue
			}
			if fixed || u.Locked || u.Block != ex.Units[first].Block {
				return problemf("Segment %d of %s is sent split or joined, but its file keeps it as it is.", i+1, f.Name)
			}
			for _, k := range got.Segments {
				sg := &segs[k]
				for j := range sg.Joined {
					if sg.Joined[j].ID == u.ID {
						sg.Joined[j].Source = u.Source
					}
				}
				if len(sg.Joined) == 0 && plainKey(got.Source.Plain()) != plainKey(u.Source.Plain()) {
					return problemf("Segment %d of %s (%s) is not the file's: the file has %q.", i+1, f.Name, lang, u.Source.Plain())
				}
				sg.Block, sg.File, sg.Locked = fromFile.Block, nil, false
			}
		}
		if bad := project.CheckRuns(segs); bad >= 0 {
			return problemf("Segment %d of %s (%s) does not read as the file's text.", bad+1, f.Name, lang)
		}
		for k := range segs {
			if segs[k].Split == nil && len(segs[k].Joined) > 0 {
				segs[k].Source = project.JoinedSource(segs[k].Joined)
			}
		}
	}
	return nil
}

func unitSegments(units []filter.Unit, fixed bool) []project.Segment {
	out := make([]project.Segment, len(units))
	for i, u := range units {
		out[i] = unitSegment(u, fixed)
	}
	return out
}

// unitSegment is a unit as a segment: a bilingual file's translation and
// state become the segment's. fixed says the file's program keeps its
// own record of each segment, so they cannot be split or joined.
func unitSegment(u filter.Unit, fixed bool) project.Segment {
	s := project.Segment{ID: u.ID, Source: u.Source, Target: u.Target, Locked: u.Locked, Block: u.Block, MaxLength: u.MaxLength, Timing: u.Timing, Notes: u.Notes, Status: project.StatusNew}
	if s.Target == nil {
		s.Target = content.Content{}
	}
	if u.State != "" {
		js, _ := json.Marshal(s.Target)
		s.File = &project.FileState{State: string(u.State), Target: string(js), Match: u.Match, Fixed: fixed}
	}
	switch u.State {
	case filter.StateTranslated, filter.StateReviewed, filter.StateFinal:
		s.Status = project.StatusConfirmed
	case filter.StateDraft, filter.StateRejected:
		s.Status = project.StatusDraft
	default:
		if strings.TrimSpace(s.Target.Plain()) != "" {
			s.Status = project.StatusDraft
		}
	}
	return s
}

// matches reports whether segments are a reading of the file.
func matches(ex *filter.Extraction, segs map[string][]project.Segment) bool {
	for _, ss := range segs {
		units := project.Units(ss)
		if len(units) != len(ex.Units) {
			return false
		}
		for i := range units {
			if units[i].ID != ex.Units[i].ID || plainKey(units[i].Source.Plain()) != plainKey(ex.Units[i].Source.Plain()) {
				return false
			}
		}
	}
	return true
}

// Delivery: a file with the translations of a target language.

// fileStateOf is the state a segment gets in its bilingual file: a
// reviewer's approval or rejection stays while the translation is the
// file's.
func fileStateOf(s project.Segment) filter.State { return project.FileStateOf(s) }

// translatedFile writes a file with its segments' translations: a
// document in the target language, a bilingual file with translations and
// states, or a text with each segment replaced by its translation.
// Segments without a translation keep their source text.
func translatedFile(format string, data []byte, sourceLang, targetLang string, segs []project.Segment) ([]byte, error) {
	if formats.Document(format) {
		return formats.Merge(format, data, sourceLang, targetLang, project.Translations(segs))
	}
	switch format {
	case "xliff", formats.MemoQArchive:
		up := project.XLIFFUpdates(segs)
		if format == formats.MemoQArchive {
			doc, err := mqpackage.ReadArchive(data)
			if err != nil {
				return nil, err
			}
			merged, err := xliff.MergeTo(doc, up, targetLang)
			if err != nil {
				return nil, err
			}
			return mqpackage.WriteArchive(data, merged)
		}
		return xliff.MergeTo(data, up, targetLang)
	}
	return nil, fmt.Errorf("%s files can't be written back", format)
}

// translatedName is the name a translated file is delivered under:
// "Vertrag.docx" in en-GB becomes "Vertrag (en-GB).docx"; bilingual
// files keep their name. A PDF's translation is the Word document its
// text was read into: "Vertrag.pdf" becomes "Vertrag (en-GB).docx".
func translatedName(name, format, lang string) string {
	base := path.Base(name)
	if format == "xliff" || format == formats.MemoQArchive {
		return base
	}
	ext := path.Ext(base)
	as := ext
	if other := formats.TranslatedExt(format); other != "" {
		as = other
	}
	return strings.TrimSuffix(base, ext) + " (" + lang + ")" + as
}
