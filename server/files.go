package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter"
	"github.com/4nonX/interlinea/filter/docx"
	"github.com/4nonX/interlinea/filter/formats"
	"github.com/4nonX/interlinea/filter/mqpackage"
	"github.com/4nonX/interlinea/filter/sdlpackage"
	"github.com/4nonX/interlinea/filter/xliff"
	"github.com/4nonX/interlinea/langtag"
	"github.com/4nonX/interlinea/project"
	"github.com/4nonX/interlinea/segment"
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

// prepareFiles reads the uploaded files of a project with the given source
// and target languages: a Trados package becomes the package and its
// bilingual files, and each translatable file gets its segments, made or
// checked.
func prepareFiles(files []fileUpload, sourceLang string, targets []string) ([]fileUpload, error) {
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
		if err := prepareFile(&f, sourceLang, targets); err != nil {
			return nil, err
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
	want := formatOf(f.Name)
	if f.Format != "" && f.Format != want {
		return problemf("%s is sent as a %q file, but its name says %q.", f.Name, f.Format, want)
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
		lang, ok := projectTarget(f.TargetLang, targets)
		if !ok {
			return problemf("%s is a bilingual file for %s, which is not a target language of the project.", f.Name, f.TargetLang)
		}
		f.TargetLang = lang
		return fill(f, ex, []string{lang})
	case "txt":
		return fillText(f, sourceLang, targets)
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
	if len(f.Segments) == 0 {
		if len(ex.Units) == 0 {
			return problemf("%s has no text to translate.", f.Name)
		}
		f.Segments = map[string][]project.Segment{}
		for _, l := range langs {
			f.Segments[l] = unitSegments(ex.Units)
		}
		return nil
	}
	for lang, segs := range f.Segments {
		if len(segs) != len(ex.Units) {
			return problemf("%s has %d segments, not %d as sent for %s.", f.Name, len(ex.Units), len(segs), lang)
		}
		for i := range segs {
			u := ex.Units[i]
			if segs[i].ID != u.ID || plainKey(segs[i].Source.Plain()) != plainKey(u.Source.Plain()) {
				return problemf("Segment %d of %s is not the file's: the file has %q.", i+1, f.Name, u.Source.Plain())
			}
			fromFile := unitSegment(u)
			segs[i].Source = u.Source
			segs[i].Locked = segs[i].Locked || u.Locked
			segs[i].Block, segs[i].MaxLength, segs[i].Notes, segs[i].File = fromFile.Block, fromFile.MaxLength, fromFile.Notes, fromFile.File
		}
	}
	return nil
}

func unitSegments(units []filter.Unit) []project.Segment {
	out := make([]project.Segment, len(units))
	for i, u := range units {
		out[i] = unitSegment(u)
	}
	return out
}

// unitSegment is a unit as a segment: a bilingual file's translation and
// state become the segment's.
func unitSegment(u filter.Unit) project.Segment {
	s := project.Segment{ID: u.ID, Source: u.Source, Target: u.Target, Locked: u.Locked, Block: u.Block, MaxLength: u.MaxLength, Notes: u.Notes, Status: project.StatusNew}
	if s.Target == nil {
		s.Target = content.Content{}
	}
	if u.State != "" {
		js, _ := json.Marshal(s.Target)
		s.File = &project.FileState{State: string(u.State), Target: string(js), Match: u.Match}
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
		if len(ss) != len(ex.Units) {
			return false
		}
		for i := range ss {
			if ss[i].ID != ex.Units[i].ID || plainKey(ss[i].Source.Plain()) != plainKey(ex.Units[i].Source.Plain()) {
				return false
			}
		}
	}
	return true
}

// decodeText reads a text file: UTF-8, or UTF-16 with a byte order mark.
// bom is the mark the file started with, to write it back.
func decodeText(data []byte) (text string, bom []byte, err error) {
	switch {
	case bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}):
		bom, data = data[:3], data[3:]
	case bytes.HasPrefix(data, []byte{0xFF, 0xFE}), bytes.HasPrefix(data, []byte{0xFE, 0xFF}):
		le := data[0] == 0xFF
		body := data[2:]
		if len(body)%2 != 0 {
			return "", nil, fmt.Errorf("the UTF-16 text has an odd number of bytes")
		}
		u := make([]uint16, len(body)/2)
		for i := range u {
			if le {
				u[i] = uint16(body[2*i]) | uint16(body[2*i+1])<<8
			} else {
				u[i] = uint16(body[2*i])<<8 | uint16(body[2*i+1])
			}
		}
		return string(utf16.Decode(u)), data[:2], nil
	}
	if !utf8.Valid(data) {
		return "", nil, fmt.Errorf("the text is not UTF-8 or UTF-16")
	}
	return string(data), bom, nil
}

// encodeText writes text as the file was: UTF-8, or UTF-16 with its mark.
func encodeText(text string, bom []byte) []byte {
	if bytes.Equal(bom, []byte{0xFF, 0xFE}) || bytes.Equal(bom, []byte{0xFE, 0xFF}) {
		out := append([]byte{}, bom...)
		for _, c := range utf16.Encode([]rune(text)) {
			if bom[0] == 0xFF {
				out = append(out, byte(c), byte(c>>8))
			} else {
				out = append(out, byte(c>>8), byte(c))
			}
		}
		return out
	}
	return append(append([]byte{}, bom...), text...)
}

// textSegments splits a text into segments: paragraphs (separated by
// line breaks) split into sentences by the source language's rules.
func textSegments(text, lang string) []project.Segment {
	seg := segment.New(nil, lang)
	var out []project.Segment
	para := 0
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		para++
		for _, sp := range seg.Split(line) {
			out = append(out, project.Segment{
				ID:     fmt.Sprintf("s%d", len(out)+1),
				Block:  fmt.Sprintf("p%d", para),
				Source: content.Text(line[sp.Start:sp.End]),
				Target: content.Content{},
				Status: project.StatusNew,
			})
		}
	}
	return out
}

// fillText makes a text file's segments, or checks that the app's are the
// file's text, in order.
func fillText(f *fileUpload, sourceLang string, langs []string) error {
	text, _, err := decodeText(f.Data)
	if err != nil {
		return problemf("%s could not be read: %v", f.Name, err)
	}
	if len(f.Segments) == 0 {
		segs := textSegments(text, sourceLang)
		if len(segs) == 0 {
			return problemf("%s has no text to translate.", f.Name)
		}
		f.Segments = map[string][]project.Segment{}
		for _, l := range langs {
			f.Segments[l] = append([]project.Segment(nil), segs...)
		}
		return nil
	}
	key := plainKey(text)
	for lang, segs := range f.Segments {
		at := 0
		for i, s := range segs {
			src := plainKey(s.Source.Plain())
			if src == "" {
				continue
			}
			j := strings.Index(key[at:], src)
			if j < 0 {
				return problemf("Segment %d of %s (%s) is not in the file's text.", i+1, f.Name, lang)
			}
			at += j + len(src)
		}
	}
	return nil
}

// Delivery: a file with the translations of a target language.

// fileStateOf is the state a segment gets in its bilingual file: a
// reviewer's approval or rejection stays while the translation is the
// file's.
func fileStateOf(s project.Segment) filter.State {
	unchanged := false
	if s.File != nil {
		js, _ := json.Marshal(s.Target)
		unchanged = string(js) == s.File.Target
	}
	switch s.Status {
	case project.StatusConfirmed:
		if unchanged && (s.File.State == string(filter.StateReviewed) || s.File.State == string(filter.StateFinal)) {
			return filter.State(s.File.State)
		}
		return filter.StateTranslated
	case project.StatusDraft:
		if unchanged && s.File.State == string(filter.StateRejected) {
			return filter.StateRejected
		}
		return filter.StateDraft
	}
	if strings.TrimSpace(s.Target.Plain()) != "" {
		return filter.StateDraft
	}
	return filter.StateNew
}

// translatedFile writes a file with its segments' translations: a
// document in the target language, a bilingual file with translations and
// states, or a text with each segment replaced by its translation.
// Segments without a translation keep their source text.
func translatedFile(format string, data []byte, sourceLang, targetLang string, segs []project.Segment) ([]byte, error) {
	if formats.Document(format) {
		tr := filter.Translations{}
		for _, s := range segs {
			if strings.TrimSpace(s.Target.Plain()) != "" {
				tr[s.ID] = s.Target
			}
		}
		return formats.Merge(format, data, sourceLang, targetLang, tr)
	}
	switch format {
	case "xliff", formats.MemoQArchive:
		up := map[string]xliff.Update{}
		for _, s := range segs {
			if !s.Locked {
				up[s.ID] = xliff.Update{Target: s.Target, State: fileStateOf(s)}
			}
		}
		if format == formats.MemoQArchive {
			doc, err := mqpackage.ReadArchive(data)
			if err != nil {
				return nil, err
			}
			merged, err := xliff.Merge(doc, up)
			if err != nil {
				return nil, err
			}
			return mqpackage.WriteArchive(data, merged)
		}
		return xliff.Merge(data, up)
	case "txt":
		text, bom, err := decodeText(data)
		if err != nil {
			return nil, err
		}
		var b strings.Builder
		at := 0
		for _, s := range segs {
			src := s.Source.Plain()
			if src == "" {
				continue
			}
			j := strings.Index(text[at:], src)
			if j < 0 {
				return nil, fmt.Errorf("segment %s is not in the file's text", s.ID)
			}
			b.WriteString(text[at : at+j])
			if t := s.Target.Plain(); strings.TrimSpace(t) != "" {
				b.WriteString(t)
			} else {
				b.WriteString(src)
			}
			at += j + len(src)
		}
		b.WriteString(text[at:])
		return encodeText(b.String(), bom), nil
	}
	return nil, fmt.Errorf("%s files can't be written back", format)
}

// translatedName is the name a translated file is delivered under:
// "Vertrag.docx" in en-GB becomes "Vertrag (en-GB).docx"; bilingual
// files keep their name.
func translatedName(name, format, lang string) string {
	base := path.Base(name)
	if format == "xliff" || format == formats.MemoQArchive {
		return base
	}
	ext := path.Ext(base)
	return strings.TrimSuffix(base, ext) + " (" + lang + ")" + ext
}
