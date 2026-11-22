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
	"io"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter"
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

// Extract returns the translatable units of a .docx file. lang is the
// source language (a BCP 47 tag); it selects the sentence rules.
func Extract(data []byte, lang string) (*filter.Extraction, error) {
	d, err := parse(data, lang)
	if err != nil {
		return nil, err
	}
	return &d.ext, nil
}

// Merge returns a copy of the .docx file with the units replaced by their
// translations. lang must be the source language used for Extract, so the
// units are found again; targetLang is written to the rewritten runs.
func Merge(data []byte, lang, targetLang string, tr filter.Translations) ([]byte, error) {
	d, err := parse(data, lang)
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
	return rewriteZip(d.zr, changed)
}

type document struct {
	zr     *zip.Reader
	styles map[string]styleInfo
	seg    *segment.Segmenter
	parts  []*part
	ext    filter.Extraction
}

type part struct {
	file  string
	kind  filter.Part
	data  []byte
	w     string // prefix bound to the main namespace
	paras []*para
}

func (p *part) hasUnits() bool {
	for _, pa := range p.paras {
		if pa.hasUnits() {
			return true
		}
	}
	return false
}

func parse(data []byte, lang string) (*document, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotDocx, err)
	}
	d := &document{zr: zr, seg: segment.New(nil, lang)}
	main, err := mainPart(zr)
	if err != nil {
		return nil, err
	}
	if b, err := readFile(zr, path.Join(path.Dir(main), "styles.xml")); err == nil {
		d.styles = parseStyles(b)
	}
	files := []struct {
		name string
		kind filter.Part
	}{{main, filter.PartBody}}
	rels, _ := readRels(zr, main)
	var notes, heads, foots []string
	var endnotes []string
	for _, r := range rels {
		switch {
		case strings.HasSuffix(r.typ, "/footnotes"):
			notes = append(notes, r.target)
		case strings.HasSuffix(r.typ, "/endnotes"):
			endnotes = append(endnotes, r.target)
		case strings.HasSuffix(r.typ, "/header"):
			heads = append(heads, r.target)
		case strings.HasSuffix(r.typ, "/footer"):
			foots = append(foots, r.target)
		}
	}
	for _, group := range []struct {
		names []string
		kind  filter.Part
	}{{notes, filter.PartFootnote}, {endnotes, filter.PartEndnote}, {heads, filter.PartHeader}, {foots, filter.PartFooter}} {
		sort.Slice(group.names, func(i, j int) bool { return naturalLess(group.names[i], group.names[j]) })
		for _, n := range group.names {
			files = append(files, struct {
				name string
				kind filter.Part
			}{n, group.kind})
		}
	}
	for _, f := range files {
		b, err := readFile(zr, f.name)
		if err != nil {
			if f.kind == filter.PartBody {
				return nil, err
			}
			continue // a broken optional part is left as it is
		}
		p, err := d.parsePart(f.name, f.kind, b)
		if err != nil {
			return nil, fmt.Errorf("docx: %s: %w", f.name, err)
		}
		d.parts = append(d.parts, p)
	}
	return d, nil
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

type rel struct{ typ, target string }

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
		out = append(out, rel{typ: r.Type, target: t})
	}
	return out, nil
}

func readFile(zr *zip.Reader, name string) ([]byte, error) {
	for _, f := range zr.File {
		if f.Name != name {
			continue
		}
		if f.UncompressedSize64 > maxPartSize {
			return nil, fmt.Errorf("docx: %s is too large", name)
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		b, err := io.ReadAll(io.LimitReader(rc, maxPartSize+1))
		if err != nil {
			return nil, err
		}
		if len(b) > maxPartSize {
			return nil, fmt.Errorf("docx: %s is too large", name)
		}
		return b, nil
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
		if s.Type != "" && s.Type != "paragraph" {
			continue
		}
		si := styleInfo{name: s.Name.Val, basedOn: s.BasedOn.Val, numbered: s.PPr.NumPr != nil}
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

// shortName turns "word/footnotes.xml" into "footnotes" for readable IDs.
func shortName(file string) string {
	return strings.TrimSuffix(path.Base(file), path.Ext(file))
}

// unitTag looks up the source tag for a tag in a translation by kind and ID.
func unitTag(src content.Content, t content.Tag) (content.Tag, bool) {
	for _, r := range src {
		if r.IsTag() && r.Tag.ID == t.ID && r.Tag.Kind == t.Kind {
			return *r.Tag, true
		}
	}
	return content.Tag{}, false
}
