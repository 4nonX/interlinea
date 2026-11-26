// Package mqpackage reads memoQ handoff packages (.mqout) and memoQ XLIFF
// archives (.mqxlz), and writes delivery packages (.mqback).
//
// A handoff package is a ZIP file with the documents to translate as
// memoQ XLIFF, each in an archive of its own (.mqxlz) or as it is
// (.mqxliff), with the translation memories (TMX) and term bases (CSV) of
// the job and a description of the package (HandoffPackageInfo.xml). An
// .mqxlz is a ZIP file with the bilingual document (document.mqxliff) and
// what memoQ needs to write the translated file (skeleton.xml).
//
// A delivery package is a ZIP file with the translated documents as memoQ
// XLIFF and DeliveryPackageInfo.xml: the handoff's description under
// another name. memoQ publishes no format for either file. Return follows
// how people build delivery packages by hand for memoQ to take in: the
// HandoffPackageInfo element becomes HandoffDeliveryPackageInfo and its
// UserName becomes User. It has not been tried against memoQ; each
// translated document can also be saved on its own and brought into
// memoQ as a bilingual file.
package mqpackage

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"

	"github.com/4nonX/interlinea/filter/xliff"
	"github.com/4nonX/interlinea/internal/unzip"
	"github.com/4nonX/interlinea/internal/xmlraw"
)

// Errors returned by this package.
var (
	ErrNotPackage = errors.New("mqpackage: not a handoff package (no MQXLIFF documents in the ZIP)")
	ErrNotArchive = errors.New("mqpackage: not a MQXLIFF archive (no .mqxliff in the ZIP)")
	ErrNoFile     = errors.New("mqpackage: the package has no such file")
)

// The names of the files that describe a handoff and a delivery.
const (
	handoffInfo  = "HandoffPackageInfo.xml"
	deliveryInfo = "DeliveryPackageInfo.xml"
)

// File is a document of the package.
type File struct {
	// Path inside the package, with forward slashes.
	Path string `json:"path"`
	// Lang is the document's target language, as the document says.
	Lang string `json:"lang"`
	// Target is true: every document of a handoff is to be translated.
	Target bool `json:"target"`
	Size   int  `json:"size"`
	// Archive is true for a document in an archive of its own (.mqxlz).
	Archive bool `json:"archive,omitempty"`
}

// Package describes a handoff package.
type Package struct {
	Name        string   `json:"name"`
	SourceLang  string   `json:"sourceLang"`
	TargetLangs []string `json:"targetLangs"`
	Files       []File   `json:"files"`
	// Other counts the remaining entries (translation memories, term
	// bases, settings), which stay in the package.
	Other int `json:"other"`
	// Memories lists the translation memories of the package (TMX).
	Memories []string `json:"memories,omitempty"`
}

func readZip(data []byte, notThis error) (*zip.Reader, error) {
	zr, err := unzip.Open(data)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", notThis, err)
	}
	return zr, nil
}

// maxEntry guards against decompression bombs (512 MiB per entry).
const maxEntry = 512 << 20

func readEntry(f *zip.File) ([]byte, error) {
	if f.UncompressedSize64 > maxEntry {
		return nil, fmt.Errorf("mqpackage: %s is too large", f.Name)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(io.LimitReader(rc, maxEntry))
}

func entryName(f *zip.File) string {
	return strings.TrimPrefix(strings.ReplaceAll(f.Name, `\`, "/"), "/")
}

func ext(name string) string { return strings.ToLower(path.Ext(name)) }

// Open lists the documents of a handoff package with their languages.
func Open(data []byte) (*Package, error) {
	zr, err := readZip(data, ErrNotPackage)
	if err != nil {
		return nil, err
	}
	p := &Package{}
	seen := map[string]bool{}
	for _, f := range zr.File {
		name := entryName(f)
		if f.FileInfo().IsDir() {
			continue
		}
		switch ext(name) {
		case ".mqxlz", ".mqxliff":
			doc, err := document(f)
			if err != nil {
				return nil, fmt.Errorf("mqpackage: %s: %w", name, err)
			}
			ex, err := xliff.Extract(doc)
			if err != nil {
				return nil, fmt.Errorf("mqpackage: %s: %w", name, err)
			}
			if p.SourceLang == "" {
				p.SourceLang = ex.SourceLang
			}
			if t := ex.TargetLang; t != "" && !seen[strings.ToLower(t)] {
				seen[strings.ToLower(t)] = true
				p.TargetLangs = append(p.TargetLangs, t)
			}
			p.Files = append(p.Files, File{Path: name, Lang: ex.TargetLang, Target: true, Size: len(doc), Archive: ext(name) == ".mqxlz"})
		case ".tmx":
			p.Memories = append(p.Memories, name)
			p.Other++
		default:
			if path.Base(name) == handoffInfo {
				if raw, err := readEntry(f); err == nil {
					p.Name = projectName(raw)
				}
			}
			p.Other++
		}
	}
	if len(p.Files) == 0 {
		return nil, ErrNotPackage
	}
	sort.SliceStable(p.Files, func(i, j int) bool { return p.Files[i].Path < p.Files[j].Path })
	return p, nil
}

// projectName finds the project's name in the handoff's description, if
// it gives one.
func projectName(raw []byte) string {
	doc, err := xmlraw.Read(raw)
	if err != nil {
		return ""
	}
	name := ""
	doc.Root.Walk(func(n *xmlraw.Node) bool {
		if name == "" && (n.Local == "ProjectName" || n.Local == "Name") {
			if t := strings.TrimSpace(n.InnerText()); t != "" && len(n.Elements()) == 0 {
				name = t
			}
		}
		return name == ""
	})
	return name
}

// document is the memoQ XLIFF of an entry: the entry itself, or the
// document in its archive.
func document(f *zip.File) ([]byte, error) {
	raw, err := readEntry(f)
	if err != nil {
		return nil, err
	}
	if ext(f.Name) == ".mqxlz" {
		return ReadArchive(raw)
	}
	return raw, nil
}

// Read returns a document of the package as memoQ XLIFF.
func Read(data []byte, name string) ([]byte, error) {
	zr, err := readZip(data, ErrNotPackage)
	if err != nil {
		return nil, err
	}
	for _, f := range zr.File {
		if entryName(f) == name {
			return document(f)
		}
	}
	return nil, fmt.Errorf("%w: %q", ErrNoFile, name)
}

// archived finds the bilingual document of an .mqxlz.
func archived(zr *zip.Reader) *zip.File {
	var found *zip.File
	for _, f := range zr.File {
		if ext(f.Name) != ".mqxliff" {
			continue
		}
		if strings.EqualFold(path.Base(entryName(f)), "document.mqxliff") {
			return f
		}
		if found == nil {
			found = f
		}
	}
	return found
}

// ReadArchive returns the memoQ XLIFF document of an .mqxlz archive.
func ReadArchive(data []byte) ([]byte, error) {
	zr, err := readZip(data, ErrNotArchive)
	if err != nil {
		return nil, err
	}
	f := archived(zr)
	if f == nil {
		return nil, ErrNotArchive
	}
	return readEntry(f)
}

// WriteArchive returns the .mqxlz archive with its document replaced.
// Everything else in it is copied as it is.
func WriteArchive(data, doc []byte) ([]byte, error) {
	zr, err := readZip(data, ErrNotArchive)
	if err != nil {
		return nil, err
	}
	target := archived(zr)
	if target == nil {
		return nil, ErrNotArchive
	}
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for _, f := range zr.File {
		if f != target {
			if err := zw.Copy(f); err != nil {
				return nil, err
			}
			continue
		}
		if err := write(zw, f.Name, doc); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func write(zw *zip.Writer, name string, b []byte) error {
	w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate})
	if err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}

// Return builds a delivery package from a handoff package: every document
// as memoQ XLIFF under its own name, those in translated replaced, and
// the description of the delivery. Translation memories, term bases and
// other entries are left out. Documents are named by their path in the
// handoff.
func Return(data []byte, translated map[string][]byte) ([]byte, error) {
	zr, err := readZip(data, ErrNotPackage)
	if err != nil {
		return nil, err
	}
	pkg, err := Open(data)
	if err != nil {
		return nil, err
	}
	known := map[string]bool{}
	for _, f := range pkg.Files {
		known[f.Path] = true
	}
	for name := range translated {
		if !known[name] {
			return nil, fmt.Errorf("%w: %q is not a document of the package", ErrNoFile, name)
		}
	}
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	used := map[string]bool{}
	info := false
	for _, f := range zr.File {
		name := entryName(f)
		switch {
		case known[name]:
			doc, ok := translated[name]
			if !ok {
				if doc, err = document(f); err != nil {
					return nil, err
				}
			}
			// A delivery holds the documents themselves, side by side.
			base := strings.TrimSuffix(path.Base(name), path.Ext(name)) + ".mqxliff"
			for n := 2; used[strings.ToLower(base)]; n++ {
				base = fmt.Sprintf("%s (%d).mqxliff", strings.TrimSuffix(path.Base(name), path.Ext(name)), n)
			}
			used[strings.ToLower(base)] = true
			if err := write(zw, base, doc); err != nil {
				return nil, err
			}
		case path.Base(name) == handoffInfo && !info:
			raw, err := readEntry(f)
			if err != nil {
				return nil, err
			}
			del, err := delivery(raw)
			if err != nil {
				return nil, fmt.Errorf("mqpackage: %s: %w", name, err)
			}
			if err := write(zw, deliveryInfo, del); err != nil {
				return nil, err
			}
			info = true
		}
	}
	if !info {
		return nil, fmt.Errorf("mqpackage: the package has no %s, which a delivery is written from", handoffInfo)
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// delivery turns the description of a handoff into that of a delivery:
// the HandoffPackageInfo element alone, named HandoffDeliveryPackageInfo,
// with UserName elements named User.
func delivery(raw []byte) ([]byte, error) {
	doc, err := xmlraw.Read(raw)
	if err != nil {
		return nil, err
	}
	var info *xmlraw.Node
	doc.Root.Walk(func(n *xmlraw.Node) bool {
		if info == nil && n.Local == "HandoffPackageInfo" {
			info = n
		}
		return info == nil
	})
	if info == nil {
		return nil, errors.New("no HandoffPackageInfo element")
	}
	var edits []xmlraw.Edit
	rename := func(n *xmlraw.Node, to string) {
		q := to
		if n.Prefix != "" {
			q = n.Prefix + ":" + to
		}
		start := n.Off + 1
		edits = append(edits, xmlraw.Edit{Span: xmlraw.Span{start, start + len(n.Name())}, With: []byte(q)})
		if !n.SelfClosing() {
			end := n.EndTag()[0] + 2
			edits = append(edits, xmlraw.Edit{Span: xmlraw.Span{end, end + len(n.Name())}, With: []byte(q)})
		}
	}
	rename(info, "HandoffDeliveryPackageInfo")
	info.Walk(func(n *xmlraw.Node) bool {
		if n.Local == "UserName" {
			rename(n, "User")
		}
		return true
	})
	all, err := xmlraw.Apply(doc.Data, edits)
	if err != nil {
		return nil, err
	}
	// The element alone: its range grew by what the new names add.
	grow := 0
	for _, e := range edits {
		grow += len(e.With) - (e.Span[1] - e.Span[0])
	}
	el := all[info.Off : info.Off+len(info.Raw)+grow]
	// A root element carries the namespaces it uses itself; one taken out
	// of a document needs those declared above it.
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?>` + "\n")
	if info != doc.Root {
		el = withNamespaces(el, doc.Root, info)
	}
	b.Write(el)
	b.WriteString("\n")
	return doc.Encode(b.Bytes()), nil
}

// withNamespaces adds to the element's start tag the namespace
// declarations of root that the element does not make itself.
func withNamespaces(el []byte, root, info *xmlraw.Node) []byte {
	have := map[string]bool{}
	for _, a := range info.Attrs {
		if a.Name.Space == "xmlns" || (a.Name.Space == "" && a.Name.Local == "xmlns") {
			have[a.Name.Space+":"+a.Name.Local] = true
		}
	}
	end := bytes.IndexByte(el, '>')
	if end < 0 {
		return el
	}
	tag := el[:end+1]
	for _, a := range root.Attrs {
		key := a.Name.Space + ":" + a.Name.Local
		isNS := a.Name.Space == "xmlns" || (a.Name.Space == "" && a.Name.Local == "xmlns")
		if !isNS || have[key] {
			continue
		}
		name := "xmlns"
		if a.Name.Space == "xmlns" {
			name = "xmlns:" + a.Name.Local
		}
		tag = xmlraw.SetAttr(tag, name, a.Value)
	}
	return append(append([]byte{}, tag...), el[end+1:]...)
}
