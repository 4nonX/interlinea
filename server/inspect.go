package server

import (
	"io"
	"net/http"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/4nonX/interlinea/analysis"
	"github.com/4nonX/interlinea/filter"
	"github.com/4nonX/interlinea/filter/formats"
	"github.com/4nonX/interlinea/filter/mqpackage"
	"github.com/4nonX/interlinea/filter/sdlpackage"
	"github.com/4nonX/interlinea/filter/xliff"
	"github.com/4nonX/interlinea/langdetect"
)

// detectionLimit is how much text the language guess reads.
const detectionLimit = 20000

// inspection is what the server finds in a document before a project is
// made from it.
type inspection struct {
	Name string `json:"name"`
	// Format is docx, pdf, xliff, package (a Trados project package), txt
	// and so on, or other for a file that is not translated (a reference
	// file).
	Format string `json:"format"`
	// Translatable is false for other files and files that can't be read.
	Translatable bool `json:"translatable"`
	// Problem says why a file that looks translatable can't be read.
	Problem  string `json:"problem,omitempty"`
	Words    int    `json:"words"`
	Segments int    `json:"segments"`
	// Detection guesses the source language from the text.
	Detection *langdetect.Detection `json:"detection,omitempty"`
	// Bilingual is set for an XLIFF file: its variant and languages.
	Bilingual *bilingualInfo `json:"bilingual,omitempty"`
	// Package describes a Trados project package and its files.
	Package  *packageInfo `json:"package,omitempty"`
	Warnings []string     `json:"warnings"`
}

type bilingualInfo struct {
	// Format is xliff-1.2, xliff-2, sdlxliff, mqxliff, mxliff (Phrase),
	// txml (Wordfast Pro) or ttx (Trados TagEditor).
	Format     string `json:"format"`
	SourceLang string `json:"sourceLang"`
	TargetLang string `json:"targetLang"`
	// Translated counts the segments the file has translations for.
	Translated int `json:"translated"`
}

type packageInfo struct {
	Name        string        `json:"name"`
	SourceLang  string        `json:"sourceLang"`
	TargetLangs []string      `json:"targetLangs"`
	Files       []packageFile `json:"files"`
	// Other counts the package's other entries (memories, termbases),
	// which stay in it.
	Other int `json:"other"`
}

type packageFile struct {
	Path     string `json:"path"`
	Lang     string `json:"lang"`
	Words    int    `json:"words"`
	Segments int    `json:"segments"`
	Problem  string `json:"problem,omitempty"`
}

// inspectFile reads a document (the request body, named by ?name=) and
// says what it is: its format, words and segments, the source language it
// seems to be in, and for bilingual files and Trados packages their
// languages and files. Nothing is kept.
//
// Access: signed in.
func (s *Server) inspectFile(w http.ResponseWriter, r *http.Request, me session) {
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if name == "" {
		invalid(w, "name", "Say the file's name (?name=), which tells its format.")
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxFileBody))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "tooLarge", "The file is too large.")
		return
	}
	writeJSON(w, http.StatusOK, inspect(name, data))
}

func inspect(name string, data []byte) inspection {
	out := inspection{Name: name, Format: formatOf(name), Warnings: []string{}}
	if out.Format == formats.Text && !formats.TextName(name) {
		out.Format = "other"
		return out
	}
	switch out.Format {
	case "package":
		memoQ := formats.Of(name) == formats.MemoQPackage
		type member struct{ path, lang string }
		var members []member
		info := &packageInfo{Files: []packageFile{}}
		read := func(p string) ([]byte, error) { return sdlpackage.Read(data, p) }
		if memoQ {
			pkg, err := mqpackage.Open(data)
			if err != nil {
				out.Problem = "The file could not be read as a handoff package: " + err.Error()
				return out
			}
			info.Name, info.SourceLang, info.TargetLangs, info.Other = pkg.Name, pkg.SourceLang, pkg.TargetLangs, pkg.Other
			if info.Name == "" {
				info.Name = strings.TrimSuffix(path.Base(name), path.Ext(name))
			}
			for _, f := range pkg.Files {
				members = append(members, member{f.Path, f.Lang})
			}
			read = func(p string) ([]byte, error) { return mqpackage.Read(data, p) }
		} else {
			pkg, err := sdlpackage.Open(data)
			if err != nil {
				out.Problem = "The file could not be read as a project package: " + err.Error()
				return out
			}
			info.Name, info.SourceLang, info.TargetLangs, info.Other = pkg.Name, pkg.SourceLang, pkg.TargetLangs, pkg.Other
			for _, f := range pkg.Files {
				if f.Target {
					members = append(members, member{f.Path, f.Lang})
				}
			}
		}
		var text strings.Builder
		for _, f := range members {
			pf := packageFile{Path: f.path, Lang: f.lang}
			b, err := read(f.path)
			if err == nil {
				var ex *filter.Extraction
				if ex, err = xliff.Extract(b); err == nil {
					pf.Words, pf.Segments = count(ex.Units, &text)
					out.Words += pf.Words
					out.Segments += pf.Segments
					out.Warnings = append(out.Warnings, ex.Warnings...)
				}
			}
			if err != nil {
				pf.Problem = err.Error()
			}
			info.Files = append(info.Files, pf)
		}
		out.Package, out.Translatable = info, len(info.Files) > 0
		if !out.Translatable {
			out.Problem = "The package has no files to translate."
		}
		out.Detection = detect(text.String())
	case "xliff", formats.MemoQArchive:
		doc := data
		if out.Format == formats.MemoQArchive {
			var err error
			if doc, err = mqpackage.ReadArchive(data); err != nil {
				out.Problem = "The file could not be read: " + err.Error()
				return out
			}
		}
		ex, err := xliff.Extract(doc)
		if err != nil {
			out.Problem = "The file could not be read: " + err.Error()
			return out
		}
		var text strings.Builder
		out.Words, out.Segments = count(ex.Units, &text)
		b := &bilingualInfo{Format: ex.Format, SourceLang: ex.SourceLang, TargetLang: ex.TargetLang}
		for _, u := range ex.Units {
			if strings.TrimSpace(u.Target.Plain()) != "" {
				b.Translated++
			}
		}
		out.Bilingual, out.Translatable, out.Warnings = b, true, append(out.Warnings, ex.Warnings...)
		out.Detection = detect(text.String())
	default:
		if formats.Document(out.Format) {
			// Sentence rules barely matter here; the file is read again with
			// the project's source language.
			ex, err := formats.Extract(out.Format, data, "en")
			if err != nil {
				out.Problem = "The file could not be read: " + err.Error()
				return out
			}
			var text strings.Builder
			out.Words, out.Segments = count(ex.Units, &text)
			out.Translatable, out.Warnings = out.Segments > 0, append(out.Warnings, ex.Warnings...)
			if !out.Translatable {
				out.Problem = "The file has no text to translate."
			}
			out.Detection = detect(text.String())
		}
	}
	return out
}

// count counts the words and segments of units and collects their text
// for the language guess.
func count(units []filter.Unit, text *strings.Builder) (words, segments int) {
	for _, u := range units {
		words += analysis.Count(u.Source).Words
		if text.Len() < detectionLimit {
			text.WriteString(u.Source.Plain())
			text.WriteByte('\n')
		}
	}
	return words, len(units)
}

func detect(text string) *langdetect.Detection {
	if len(text) > detectionLimit {
		text = text[:detectionLimit]
		for !utf8.ValidString(text) {
			text = text[:len(text)-1]
		}
	}
	d := langdetect.Detect(text)
	return &d
}
