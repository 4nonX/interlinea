// Package sdlpackage reads Trados Studio project packages (.sdlppx) and
// writes return packages (.sdlrpx).
//
// Both are ZIP files with a project file (.sdlproj) at the root whose root
// element, PackageProject, says in its PackageType attribute which of the
// two it is, and a folder per language holding the bilingual SDLXLIFF
// files (the target-language folders are the ones to translate). A return
// package carries the same project file with PackageType="ReturnPackage"
// and the translated target files; translation memories and termbases are
// not sent back.
package sdlpackage

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"

	"github.com/4nonX/interlinea/internal/xmlraw"
)

// Errors returned by this package.
var (
	ErrNotPackage = errors.New("sdlpackage: not a Trados package (no .sdlproj in the ZIP)")
	ErrNoFile     = errors.New("sdlpackage: the package has no such file")
)

// File is a bilingual file of the package.
type File struct {
	// Path inside the package, with forward slashes ("en-GB/Vertrag.docx.sdlxliff").
	Path string `json:"path"`
	// Lang is the language folder it sits in.
	Lang string `json:"lang"`
	// Target is true for files in a target-language folder.
	Target bool `json:"target"`
	Size   int  `json:"size"`
}

// Package describes a package's manifest and files.
type Package struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"` // ProjectPackage or ReturnPackage
	Guid        string   `json:"guid"`
	SourceLang  string   `json:"sourceLang"`
	TargetLangs []string `json:"targetLangs"`
	Project     string   `json:"project"` // path of the .sdlproj
	Files       []File   `json:"files"`
	// Other counts the remaining entries (translation memories,
	// termbases, file type settings), which stay in the package.
	Other int `json:"other"`
}

func readZip(data []byte) (*zip.Reader, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("sdlpackage: %w", err)
	}
	return zr, nil
}

func readEntry(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// projectFile finds the package's .sdlproj (at the root, else the first).
func projectFile(zr *zip.Reader) *zip.File {
	var found *zip.File
	for _, f := range zr.File {
		if strings.EqualFold(path.Ext(f.Name), ".sdlproj") {
			if !strings.Contains(strings.TrimPrefix(f.Name, "/"), "/") {
				return f
			}
			if found == nil {
				found = f
			}
		}
	}
	return found
}

// Open reads a package's manifest and lists its bilingual files.
func Open(data []byte) (*Package, error) {
	zr, err := readZip(data)
	if err != nil {
		return nil, err
	}
	pf := projectFile(zr)
	if pf == nil {
		return nil, ErrNotPackage
	}
	raw, err := readEntry(pf)
	if err != nil {
		return nil, fmt.Errorf("sdlpackage: %s: %w", pf.Name, err)
	}
	doc, err := xmlraw.Read(raw)
	if err != nil {
		return nil, fmt.Errorf("sdlpackage: %s: %w", pf.Name, err)
	}
	root := doc.Root
	p := &Package{Project: pf.Name, Name: root.Get("Name"), Type: root.Get("PackageType"), Guid: root.Get("PackageGuid")}
	if p.Name == "" {
		p.Name = strings.TrimSuffix(path.Base(pf.Name), path.Ext(pf.Name))
	}
	if ld := root.Child("LanguageDirections"); ld != nil {
		for _, d := range ld.Children("LanguageDirection") {
			if p.SourceLang == "" {
				p.SourceLang = d.Get("SourceLanguageCode")
			}
			if t := d.Get("TargetLanguageCode"); t != "" {
				p.TargetLangs = append(p.TargetLangs, t)
			}
		}
	}
	if p.SourceLang == "" {
		p.SourceLang = root.Get("SourceLanguageCode")
	}
	isTarget := map[string]bool{}
	for _, t := range p.TargetLangs {
		isTarget[strings.ToLower(t)] = true
	}
	if len(p.TargetLangs) == 0 {
		// A manifest without language directions: the file's own languages
		// tell which folders are targets.
		inferLanguages(zr, p, isTarget)
	}
	for _, f := range zr.File {
		name := strings.TrimPrefix(f.Name, "/")
		if f.FileInfo().IsDir() || f == pf {
			continue
		}
		if !strings.EqualFold(path.Ext(name), ".sdlxliff") {
			p.Other++
			continue
		}
		lang, _, _ := strings.Cut(name, "/")
		if !strings.Contains(name, "/") {
			lang = ""
		}
		p.Files = append(p.Files, File{Path: name, Lang: lang, Target: isTarget[strings.ToLower(lang)], Size: int(f.UncompressedSize64)})
	}
	sort.SliceStable(p.Files, func(i, j int) bool {
		if p.Files[i].Target != p.Files[j].Target {
			return p.Files[i].Target
		}
		return p.Files[i].Path < p.Files[j].Path
	})
	return p, nil
}

// inferLanguages reads the languages a package's SDLXLIFF files declare
// (from their first bytes) when the manifest names none.
func inferLanguages(zr *zip.Reader, p *Package, isTarget map[string]bool) {
	seen := map[string]bool{}
	for _, f := range zr.File {
		name := strings.TrimPrefix(f.Name, "/")
		if !strings.EqualFold(path.Ext(name), ".sdlxliff") || !strings.Contains(name, "/") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		head := make([]byte, 64<<10)
		n, _ := io.ReadFull(rc, head)
		rc.Close()
		src, tgt := attrValue(head[:n], "source-language"), attrValue(head[:n], "target-language")
		folder, _, _ := strings.Cut(name, "/")
		if p.SourceLang == "" && src != "" {
			p.SourceLang = src
		}
		if tgt != "" && strings.EqualFold(folder, tgt) && !seen[strings.ToLower(tgt)] {
			seen[strings.ToLower(tgt)] = true
			p.TargetLangs = append(p.TargetLangs, folder)
			isTarget[strings.ToLower(folder)] = true
		}
	}
}

func attrValue(b []byte, name string) string {
	i := bytes.Index(b, []byte(name+`="`))
	if i < 0 {
		return ""
	}
	rest := b[i+len(name)+2:]
	j := bytes.IndexByte(rest, '"')
	if j < 0 {
		return ""
	}
	return string(rest[:j])
}

// Read returns the bytes of a file of the package.
func Read(data []byte, name string) ([]byte, error) {
	zr, err := readZip(data)
	if err != nil {
		return nil, err
	}
	for _, f := range zr.File {
		if strings.TrimPrefix(f.Name, "/") == name {
			return readEntry(f)
		}
	}
	return nil, fmt.Errorf("%w: %q", ErrNoFile, name)
}

// Return builds a return package from a project package: its project file
// with PackageType="ReturnPackage" (every other byte unchanged) and its
// SDLXLIFF files, the target files in translated replaced, as Trados
// writes return packages. Translation memories, termbases and other
// entries are left out. Files translated by path must be target files of
// the package.
func Return(data []byte, translated map[string][]byte) ([]byte, error) {
	zr, err := readZip(data)
	if err != nil {
		return nil, err
	}
	pkg, err := Open(data)
	if err != nil {
		return nil, err
	}
	targets := map[string]bool{}
	bilingual := map[string]bool{}
	for _, f := range pkg.Files {
		bilingual[f.Path] = true
		if f.Target {
			targets[f.Path] = true
		}
	}
	for name := range translated {
		if !targets[name] {
			return nil, fmt.Errorf("%w: %q is not a target file of the package", ErrNoFile, name)
		}
	}
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for _, f := range zr.File {
		name := strings.TrimPrefix(f.Name, "/")
		switch {
		case name == strings.TrimPrefix(pkg.Project, "/"):
			raw, err := readEntry(f)
			if err != nil {
				return nil, err
			}
			flipped, err := flip(raw)
			if err != nil {
				return nil, err
			}
			if err := write(zw, f, flipped); err != nil {
				return nil, err
			}
		case bilingual[name]:
			if b, ok := translated[name]; ok {
				if err := write(zw, f, b); err != nil {
					return nil, err
				}
			} else if err := zw.Copy(f); err != nil {
				return nil, err
			}
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func write(zw *zip.Writer, f *zip.File, b []byte) error {
	h := f.FileHeader
	h.CompressedSize64, h.UncompressedSize64, h.CRC32 = 0, 0, 0
	h.Method = zip.Deflate
	w, err := zw.CreateHeader(&h)
	if err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}

// flip sets PackageType="ReturnPackage" on the project file's root element.
func flip(raw []byte) ([]byte, error) {
	doc, err := xmlraw.Read(raw)
	if err != nil {
		return nil, fmt.Errorf("sdlpackage: project file: %w", err)
	}
	root := doc.Root
	if root.Local != "PackageProject" {
		return nil, fmt.Errorf("sdlpackage: the project file's root is <%s>, not <PackageProject>", root.Name())
	}
	out, err := xmlraw.Apply(doc.Data, []xmlraw.Edit{{Span: root.StartTag(), With: xmlraw.SetAttr(root.Start, "PackageType", "ReturnPackage")}})
	if err != nil {
		return nil, err
	}
	return doc.Encode(out), nil
}
