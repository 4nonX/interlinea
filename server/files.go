package server

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/4nonX/interlinea/filter"
	"github.com/4nonX/interlinea/filter/docx"
	"github.com/4nonX/interlinea/filter/xliff"
	"github.com/4nonX/interlinea/project"
)

// The apps read files with the core's filters and send the segments with
// the file. The server reads the file itself and refuses segments that
// are not the file's: it keeps the file's own source text, with its tags,
// and takes only translations and their status from the app.

// fileProblem says what is wrong with a file's segments.
type fileProblem struct{ msg string }

func (p *fileProblem) Error() string { return p.msg }

// plainKey compares source text as people read it: tags and spacing aside.
func plainKey(s string) string {
	return strings.Join(strings.FieldsFunc(s, unicode.IsSpace), " ")
}

// checkFile checks a translatable file's segments against the file, and
// replaces their sources with the file's.
func checkFile(f fileUpload, sourceLang string) error {
	if f.Role != project.Translatable {
		return nil
	}
	if want := formatOf(f.Name); f.Format != want {
		return &fileProblem{fmt.Sprintf("%s is sent as a %q file, but its name says %q.", f.Name, f.Format, want)}
	}
	var ex *filter.Extraction
	var err error
	switch f.Format {
	case "docx":
		// Revisions are accepted when the app reads the file unless it is
		// told to keep them; either reading is the file's.
		ex, err = docx.Extract(f.Data, sourceLang, docx.Options{})
		if err == nil && !matches(ex, f.Segments) {
			if kept, err2 := docx.Extract(f.Data, sourceLang, docx.Options{KeepRevisions: true}); err2 == nil && matches(kept, f.Segments) {
				ex = kept
			}
		}
	case "xliff":
		ex, err = xliff.Extract(f.Data)
	default:
		return checkText(f)
	}
	if err != nil {
		return &fileProblem{fmt.Sprintf("%s could not be read: %v", f.Name, err)}
	}
	for lang, segs := range f.Segments {
		if len(segs) != len(ex.Units) {
			return &fileProblem{fmt.Sprintf("%s has %d segments, not %d as sent for %s.", f.Name, len(ex.Units), len(segs), lang)}
		}
		for i := range segs {
			u := ex.Units[i]
			if segs[i].ID != u.ID || plainKey(segs[i].Source.Plain()) != plainKey(u.Source.Plain()) {
				return &fileProblem{fmt.Sprintf("Segment %d of %s is not the file's: the file has %q.", i+1, f.Name, u.Source.Plain())}
			}
			segs[i].Source = u.Source
			if u.Locked {
				segs[i].Locked = true
			}
		}
	}
	return nil
}

// formatOf is the format the apps give a translatable file by its name.
func formatOf(name string) string {
	n := strings.ToLower(name)
	switch {
	case strings.HasSuffix(n, ".docx"):
		return "docx"
	case strings.HasSuffix(n, ".xlf"), strings.HasSuffix(n, ".xliff"), strings.HasSuffix(n, ".sdlxliff"), strings.HasSuffix(n, ".mqxliff"):
		return "xliff"
	default:
		return "txt"
	}
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

// checkText checks a plain text file's segments: the app splits text
// into sentences itself, so the server checks that each segment is the
// file's text, in order.
func checkText(f fileUpload) error {
	text := plainKey(string(f.Data))
	for lang, segs := range f.Segments {
		at := 0
		for i, s := range segs {
			src := plainKey(s.Source.Plain())
			if src == "" {
				continue
			}
			j := strings.Index(text[at:], src)
			if j < 0 {
				return &fileProblem{fmt.Sprintf("Segment %d of %s (%s) is not in the file's text.", i+1, f.Name, lang)}
			}
			at += j + len(src)
		}
	}
	return nil
}
