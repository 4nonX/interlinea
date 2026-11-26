// Package docx reads and writes Word (.docx) documents.
//
// Extract splits the text of the body, footnotes, endnotes, headers and
// footers into translatable units. Formatting that changes inside a
// paragraph (bold, italic, a different font, links) becomes paired tags;
// tabs, breaks, fields, images and footnote references become standalone
// tags. Formatting that covers the whole paragraph is not tagged at all, so
// translators only see tags where the formatting actually changes.
//
// Merge rebuilds the document from translations. Only the paragraphs that
// contain units are rewritten; every other byte of the package is copied
// unchanged, including styles, numbering, images, section settings and
// paragraphs without text. Rewritten text runs get the target language, so
// Word proofs them in the right language.
//
// Known limits of this version: text inside text boxes, content controls,
// charts and SmartArt is kept but not extracted; tracked changes are
// accepted when a paragraph is rewritten; comments are not translated.
package docx

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/4nonX/interlinea/filter"
	"github.com/4nonX/interlinea/internal/unzip"
	"github.com/4nonX/interlinea/segment"
)

const (
	nsMain       = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
	relOfficeDoc = "/officeDocument"
)

// ErrNotDocx is returned for files that are not Word documents.
var ErrNotDocx = errors.New("docx: not a Word document")

// maxPartSize guards against decompression bombs (256 MiB per part).
const maxPartSize = 256 << 20

// Options change how documents are read and written.
type Options struct {
	// KeepRevisions keeps tracked changes in rewritten paragraphs: tracked
	// insertions become tag pairs around their translation and tracked
	// deletions placeholders. By default they are accepted (inserted text
	// stays, deleted text goes), as in most CAT tools.
	KeepRevisions bool
}

// Extract returns the translatable units of a .docx file. lang is the
// source language (a BCP 47 tag); it selects the sentence rules.
func Extract(data []byte, lang string, opts Options) (*filter.Extraction, error) {
	d, err := parse(data, lang, opts)
	if err != nil {
		return nil, err
	}
	d.ext.Format = "docx"
	d.ext.SourceLang = lang
	return &d.ext, nil
}

// Merge returns a copy of the .docx file with the units replaced by their
// translations. lang and opts must be those used for Extract, so the units
// are found again; targetLang is written to the rewritten text.
func Merge(data []byte, lang, targetLang string, tr filter.Translations, opts Options) ([]byte, error) {
	d, err := parse(data, lang, opts)
	if err != nil {
		return nil, err
	}
	changed := map[string][]byte{}
	for _, p := range d.parts {
		if !p.hasUnits() {
			continue
		}
		changed[p.file] = p.render(d, tr, targetLang)
	}
	if err := d.updateChartData(changed, tr); err != nil {
		return nil, err
	}
	return rewriteZip(d.zr, changed)
}

type document struct {
	zr     *zip.Reader
	opts   Options
	styles map[string]styleInfo
	seg    *segment.Segmenter
	parts  []*part
	ext    filter.Extraction
	sigs   map[string]*para // source signature -> first paragraph with it
	labels []labelRef
	// noLabels leaves the names a chart shows from its data unread.
	noLabels bool
}

// labelRef points at a chart label unit, for updating the chart's data.
type labelRef struct {
	part *part
	unit int
}

type part struct {
	file  string
	kind  filter.Part
	data  []byte
	cache bool // a cached copy of other content: only mirrors
	paras []*para
}

type partRef struct {
	name  string
	kind  filter.Part
	cache bool
}

var (
	chartName   = regexp.MustCompile(`^word/charts/chart\d*\.xml$`)
	diagramData = regexp.MustCompile(`^word/diagrams/data\d*\.xml$`)
	diagramDraw = regexp.MustCompile(`^word/diagrams/drawing\d*\.xml$`)
)

// kind is the family of the package: a Word document or a presentation.
type kind int

const (
	wordDocument kind = iota
	presentation
)

func parse(data []byte, lang string, opts Options) (*document, error) {
	return parseAs(data, lang, opts, wordDocument)
}

func parseAs(data []byte, lang string, opts Options, k kind) (*document, error) {
	notThis := ErrNotDocx
	if k == presentation {
		notThis = ErrNotPresentation
	}
	zr, err := unzip.Open(data)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", notThis, err)
	}
	d := &document{zr: zr, opts: opts, seg: segment.New(nil, lang), sigs: map[string]*para{}}
	main, err := mainPart(zr)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", notThis, err)
	}
	mainData, err := readFile(zr, main)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", notThis, err)
	}
	want := "document"
	if k == presentation {
		want = "presentation"
	}
	if rootName(mainData) != want {
		return nil, notThis
	}
	var files []partRef
	if k == presentation {
		files, err = slideParts(zr, main, mainData)
		if err != nil {
			return nil, err
		}
	} else {
		if b, err := readFile(zr, path.Join(path.Dir(main), "styles.xml")); err == nil {
			d.styles = parseStyles(b)
		}
		files = wordParts(zr, main)
	}
	for _, f := range files {
		b, err := readFile(zr, f.name)
		if err != nil {
			if f.kind == filter.PartBody {
				return nil, err
			}
			continue // a missing optional part is left as it is
		}
		p, err := d.parsePart(f.name, f.kind, b, f.cache)
		if err != nil {
			if f.kind == filter.PartBody {
				return nil, fmt.Errorf("%s could not be read: %w", f.name, err)
			}
			d.ext.Warnings = append(d.ext.Warnings, fmt.Sprintf("%s could not be read and is kept unchanged: %v", f.name, err))
			continue
		}
		d.parts = append(d.parts, p)
	}
	d.resolveMirrors()
	revised := 0
	for _, b := range d.ext.Blocks {
		if b.Revisions {
			revised++
		}
	}
	if revised > 0 {
		if opts.KeepRevisions {
			d.ext.Warnings = append(d.ext.Warnings, fmt.Sprintf("%d paragraphs contain tracked changes. They are kept as tags: keep the insertion tags around the translation of the inserted text.", revised))
		} else {
			d.ext.Warnings = append(d.ext.Warnings, fmt.Sprintf("%d paragraphs contain tracked changes. Translated paragraphs keep them accepted: inserted text stays, deleted text is removed.", revised))
		}
	}
	return d, nil
}

// rootName is the local name of a part's root element.
func rootName(data []byte) string {
	dec := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := dec.Token()
		if err != nil {
			return ""
		}
		if se, ok := tok.(xml.StartElement); ok {
			return se.Name.Local
		}
	}
}

// wordParts lists the parts of a Word document with text: the body, then
// the notes, comments, headers and footers, then charts and diagrams.
func wordParts(zr *zip.Reader, main string) []partRef {
	files := []partRef{{main, filter.PartBody, false}}
	rels, _ := readRels(zr, main)
	groups := map[filter.Part][]string{}
	for _, r := range rels {
		for _, g := range []struct {
			suffix string
			kind   filter.Part
		}{{"/footnotes", filter.PartFootnote}, {"/endnotes", filter.PartEndnote}, {"/comments", filter.PartComment}, {"/header", filter.PartHeader}, {"/footer", filter.PartFooter}} {
			if strings.HasSuffix(r.typ, g.suffix) {
				groups[g.kind] = append(groups[g.kind], r.target)
			}
		}
	}
	var charts, diagrams, drawings []string
	for _, f := range zr.File {
		switch {
		case chartName.MatchString(f.Name):
			charts = append(charts, f.Name)
		case diagramData.MatchString(f.Name):
			diagrams = append(diagrams, f.Name)
		case diagramDraw.MatchString(f.Name):
			drawings = append(drawings, f.Name)
		}
	}
	groups[filter.PartChart] = charts
	groups[filter.PartDiagram] = diagrams
	for _, kind := range []filter.Part{filter.PartFootnote, filter.PartEndnote, filter.PartComment, filter.PartHeader, filter.PartFooter, filter.PartChart, filter.PartDiagram} {
		names := groups[kind]
		sort.Slice(names, func(i, j int) bool { return naturalLess(names[i], names[j]) })
		for _, n := range names {
			files = append(files, partRef{n, kind, false})
		}
	}
	// SmartArt keeps a drawing of the diagram as a cache; it is rewritten
	// with the translations of the diagram's data.
	sort.Slice(drawings, func(i, j int) bool { return naturalLess(drawings[i], drawings[j]) })
	for _, n := range drawings {
		files = append(files, partRef{n, filter.PartDiagram, true})
	}
	return files
}

// mainPart finds the main document part through the package relationships.
func mainPart(zr *zip.Reader) (string, error) {
	rels, err := readRels(zr, "")
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrNotDocx, err)
	}
	for _, r := range rels {
		if strings.HasSuffix(r.typ, relOfficeDoc) {
			return r.target, nil
		}
	}
	return "", ErrNotDocx
}

type rel struct{ id, typ, target string }

// readRels reads the relationships of a part ("" for the package) and
// returns targets resolved to package paths. External targets are skipped.
func readRels(zr *zip.Reader, partName string) ([]rel, error) {
	relsName := "_rels/.rels"
	base := ""
	if partName != "" {
		base = path.Dir(partName)
		relsName = path.Join(base, "_rels", path.Base(partName)+".rels")
	}
	b, err := readFile(zr, relsName)
	if err != nil {
		return nil, err
	}
	var x struct {
		Rels []struct {
			ID     string `xml:"Id,attr"`
			Type   string `xml:"Type,attr"`
			Target string `xml:"Target,attr"`
			Mode   string `xml:"TargetMode,attr"`
		} `xml:"Relationship"`
	}
	if err := xml.Unmarshal(b, &x); err != nil {
		return nil, err
	}
	var out []rel
	for _, r := range x.Rels {
		if r.Mode == "External" {
			continue
		}
		t := r.Target
		if strings.HasPrefix(t, "/") {
			t = strings.TrimPrefix(t, "/")
		} else {
			t = path.Join(base, t)
		}
		out = append(out, rel{id: r.ID, typ: r.Type, target: t})
	}
	return out, nil
}

func readFile(zr *zip.Reader, name string) ([]byte, error) {
	for _, f := range zr.File {
		if f.Name != name {
			continue
		}
		return unzip.Read(f, maxPartSize)
	}
	return nil, fmt.Errorf("docx: missing %s", name)
}

// rewriteZip copies the package, replacing the changed parts. Unchanged
// entries are copied without recompressing.
func rewriteZip(zr *zip.Reader, changed map[string][]byte) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range zr.File {
		b, ok := changed[f.Name]
		if !ok {
			if err := zw.Copy(f); err != nil {
				return nil, err
			}
			continue
		}
		w, err := zw.CreateHeader(&zip.FileHeader{Name: f.Name, Method: zip.Deflate, Modified: f.Modified})
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(b); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// naturalLess orders header2.xml before header10.xml.
func naturalLess(a, b string) bool {
	na, pa := trailingNumber(a)
	nb, pb := trailingNumber(b)
	if pa == pb {
		return na < nb
	}
	return a < b
}

func trailingNumber(s string) (int, string) {
	s = strings.TrimSuffix(s, path.Ext(s))
	i := len(s)
	for i > 0 && s[i-1] >= '0' && s[i-1] <= '9' {
		i--
	}
	n, _ := strconv.Atoi(s[i:])
	return n, s[:i]
}

// styleInfo is what the filter needs from styles.xml.
type styleInfo struct {
	typ      string
	name     string
	basedOn  string
	outline  int // 1-based heading level, 0 if none
	numbered bool
}

func parseStyles(b []byte) map[string]styleInfo {
	var x struct {
		Styles []struct {
			ID      string  `xml:"styleId,attr"`
			Type    string  `xml:"type,attr"`
			Name    valAttr `xml:"name"`
			BasedOn valAttr `xml:"basedOn"`
			PPr     struct {
				Outline *valAttr  `xml:"outlineLvl"`
				NumPr   *struct{} `xml:"numPr"`
			} `xml:"pPr"`
		} `xml:"style"`
	}
	if err := xml.Unmarshal(b, &x); err != nil {
		return nil
	}
	out := map[string]styleInfo{}
	for _, s := range x.Styles {
		si := styleInfo{typ: s.Type, name: s.Name.Val, basedOn: s.BasedOn.Val, numbered: s.PPr.NumPr != nil}
		if s.PPr.Outline != nil {
			if n, err := strconv.Atoi(s.PPr.Outline.Val); err == nil && n < 9 {
				si.outline = n + 1
			}
		}
		out[s.ID] = si
	}
	return out
}

type valAttr struct {
	Val string `xml:"val,attr"`
}

// heading resolves the outline level of a style through its basedOn chain,
// falling back to the built-in names "heading 1" to "heading 9" and "Title".
func (d *document) heading(styleID string) (level int, name string, numbered bool) {
	seen := map[string]bool{}
	for id := styleID; id != "" && !seen[id]; {
		seen[id] = true
		s, ok := d.styles[id]
		if !ok {
			break
		}
		if name == "" {
			name = s.name
		}
		numbered = numbered || s.numbered
		if level == 0 && s.outline > 0 {
			level = s.outline
		}
		lower := strings.ToLower(s.name)
		if level == 0 && strings.HasPrefix(lower, "heading ") {
			if n, err := strconv.Atoi(strings.TrimPrefix(lower, "heading ")); err == nil {
				level = n
			}
		}
		if level == 0 && lower == "title" {
			level = 1
		}
		id = s.basedOn
	}
	return level, name, numbered
}

// styleName returns the display name of a style ID, or "".
func (d *document) styleName(id string) string {
	return d.styles[id].name
}

// shortName turns "word/footnotes.xml" into "footnotes" for readable IDs.
func shortName(file string) string {
	return strings.TrimSuffix(path.Base(file), path.Ext(file))
}
