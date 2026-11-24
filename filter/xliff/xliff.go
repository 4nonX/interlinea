// Package xliff reads and writes bilingual XLIFF files: XLIFF 1.2 and its
// Trados (SDLXLIFF) and memoQ (MQXLIFF) variants, and XLIFF 2.0, 2.1 and
// 2.2.
//
// Extract lists the segments with their source, existing translation,
// state, lock, match value and notes. Merge writes translations and states
// back. It changes only the target of each updated segment and the
// attributes that hold its state (the Trados confirmation level, the memoQ
// status, the XLIFF state); every other byte of the file stays as it was,
// so Trados, memoQ and other tools open the result as their own file.
package xliff

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter"
	"github.com/4nonX/interlinea/internal/xmlraw"
)

// Namespaces of the variants.
const (
	nsXLIFF12 = "urn:oasis:names:tc:xliff:document:1.2"
	nsXLIFF20 = "urn:oasis:names:tc:xliff:document:2.0"
	nsSDL     = "http://sdl.com/FileTypes/SdlXliff/1.0"
	nsMQ      = "MQXliff"
)

// Flavors as reported in Extraction.Format.
const (
	FormatXLIFF12 = "xliff-1.2"
	FormatXLIFF2  = "xliff-2"
	FormatSDL     = "sdlxliff"
	FormatMQ      = "mqxliff"
)

// Errors returned by Merge.
var (
	ErrLocked     = errors.New("xliff: the segment is locked")
	ErrNoSegment  = errors.New("xliff: no segment with this ID")
	ErrNotXLIFF   = errors.New("xliff: not an XLIFF file")
	ErrBadState   = errors.New("xliff: unknown state")
	errNoLanguage = errors.New("xliff: the file declares no source language")
)

// Update is the new translation and state of a segment. A nil Target
// leaves the translation as it is; an empty State leaves the state.
type Update struct {
	Target content.Content `json:"target,omitempty"`
	State  filter.State    `json:"state,omitempty"`
}

// segment is one unit of the file with the nodes Merge needs.
type segment struct {
	unit    filter.Unit
	tu      *xmlraw.Node // trans-unit (1.2) or unit (2.0)
	seg2    *xmlraw.Node // segment (2.0)
	srcNode *xmlraw.Node // source, or the segment marker in seg-source
	segSrc  *xmlraw.Node // seg-source (1.2 segmented)
	tgtEl   *xmlraw.Node // target element, if any
	tgtNode *xmlraw.Node // target content: target, or the target's marker
	mid     string       // segment marker ID (1.2 segmented)
	sdlSeg  *xmlraw.Node // sdl:seg of the segment
	segDefs *xmlraw.Node // sdl:seg-defs of the trans-unit
	conf    string       // SDL confirmation level as read
	mqStat  string       // memoQ status as read
	tags    tagTable
}

type scan struct {
	doc      *xmlraw.Document
	format   string
	sdl, mq  string // namespace prefixes
	segments []*segment
	byID     map[string]*segment
	ext      filter.Extraction
}

// Detect reports whether data is XLIFF and which variant.
func Detect(data []byte) (string, bool) {
	doc, err := xmlraw.Read(data)
	if err != nil {
		return "", false
	}
	return format(doc.Root)
}

func format(root *xmlraw.Node) (string, bool) {
	if root.Local != "xliff" {
		return "", false
	}
	ns := root.Namespaces()
	v := root.Get("version")
	switch {
	case strings.HasPrefix(v, "2.") || hasNS(ns, nsXLIFF20) || hasNS(ns, "urn:oasis:names:tc:xliff:document:2.1") || hasNS(ns, "urn:oasis:names:tc:xliff:document:2.2"):
		return FormatXLIFF2, true
	case hasNS(ns, nsSDL):
		return FormatSDL, true
	case hasNS(ns, nsMQ):
		return FormatMQ, true
	case v == "1.2" || v == "1.1" || v == "1.0" || hasNS(ns, nsXLIFF12):
		return FormatXLIFF12, true
	}
	return "", false
}

func hasNS(ns map[string]string, uri string) bool {
	_, ok := ns[uri]
	return ok
}

func read(data []byte) (*scan, error) {
	doc, err := xmlraw.Read(data)
	if err != nil {
		return nil, fmt.Errorf("xliff: %w", err)
	}
	f, ok := format(doc.Root)
	if !ok {
		return nil, ErrNotXLIFF
	}
	ns := doc.Root.Namespaces()
	s := &scan{doc: doc, format: f, sdl: ns[nsSDL], mq: ns[nsMQ], byID: map[string]*segment{}}
	if s.sdl == "" && f == FormatSDL {
		s.sdl = "sdl"
	}
	s.ext.Format = f
	if f == FormatXLIFF2 {
		err = s.scan2()
	} else {
		err = s.scan1()
	}
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (s *scan) add(sg *segment) error {
	if _, dup := s.byID[sg.unit.ID]; dup {
		return fmt.Errorf("xliff: two segments have the ID %q", sg.unit.ID)
	}
	s.byID[sg.unit.ID] = sg
	s.segments = append(s.segments, sg)
	s.ext.Units = append(s.ext.Units, sg.unit)
	return nil
}

func (s *scan) warn(msg string) {
	for _, w := range s.ext.Warnings {
		if w == msg {
			return
		}
	}
	s.ext.Warnings = append(s.ext.Warnings, msg)
}

// sdlTagDefs reads the tag definitions of an SDLXLIFF file header.
func sdlTagDefs(file *xmlraw.Node) map[string]tagDef {
	defs := map[string]tagDef{}
	h := file.Child("header")
	if h == nil {
		return defs
	}
	td := h.Child("tag-defs")
	if td == nil {
		return defs
	}
	for _, t := range td.Children("tag") {
		var d tagDef
		for _, k := range t.Elements() {
			switch k.Local {
			case "bpt", "ph", "st", "x":
				if txt := truncate(k.InnerText(), 60); txt != "" {
					d.desc = txt
				} else if n := k.Get("name"); n != "" {
					d.desc = n
				}
			}
			if d.desc != "" {
				break
			}
		}
		defs[t.Get("id")] = d
	}
	return defs
}

func (s *scan) scan1() error {
	files := s.doc.Root.Children("file")
	if len(files) == 0 {
		return fmt.Errorf("%w: no <file> element", ErrNotXLIFF)
	}
	for fi, file := range files {
		if s.ext.SourceLang == "" {
			s.ext.SourceLang = file.Get("source-language")
		}
		if s.ext.TargetLang == "" {
			s.ext.TargetLang = file.Get("target-language")
		} else if t := file.Get("target-language"); t != "" && !strings.EqualFold(t, s.ext.TargetLang) {
			s.warn("The file has parts for more than one target language; all are listed.")
		}
		var defs map[string]tagDef
		if s.format == FormatSDL {
			defs = sdlTagDefs(file)
		}
		body := file.Child("body")
		if body == nil {
			continue
		}
		if err := s.walk1(fi, file, body, false, defs); err != nil {
			return err
		}
	}
	if s.ext.SourceLang == "" {
		return errNoLanguage
	}
	return nil
}

func (s *scan) walk1(fi int, file, n *xmlraw.Node, skip bool, defs map[string]tagDef) error {
	for _, k := range n.Elements() {
		switch k.Local {
		case "group":
			if err := s.walk1(fi, file, k, skip || k.Get("translate") == "no", defs); err != nil {
				return err
			}
		case "trans-unit":
			if skip || k.Get("translate") == "no" {
				continue
			}
			if err := s.unit1(fi, file, k, defs); err != nil {
				return err
			}
		}
	}
	return nil
}

func notes1(tu *xmlraw.Node) []string {
	var out []string
	for _, n := range tu.Children("note") {
		if t := strings.TrimSpace(n.InnerText()); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func (s *scan) unit1(fi int, file, tu *xmlraw.Node, defs map[string]tagDef) error {
	tuID := tu.Get("id")
	if tuID == "" {
		return fmt.Errorf("xliff: a trans-unit in file %d has no id", fi+1)
	}
	block := fmt.Sprintf("%d/%s", fi+1, tuID)
	s.ext.Blocks = append(s.ext.Blocks, filter.Block{ID: block, Part: filter.PartBody, File: file.Get("original")})
	src := tu.Child("source")
	if src == nil {
		return fmt.Errorf("xliff: trans-unit %q has no source", tuID)
	}
	tgt := tu.Child("target")
	base := filter.Unit{Block: block, Key: tu.Get("resname"), Notes: notes1(tu)}
	if tu.Get("size-unit") == "char" {
		if n, err := strconv.Atoi(tu.Get("maxwidth")); err == nil && n > 0 {
			base.MaxLength = n
		}
	}
	var segDefs *xmlraw.Node
	for _, k := range tu.Elements() {
		if k.Local == "seg-defs" {
			segDefs = k
		}
	}
	mqStatus, _ := tu.AttrNS(s.mq, "status")
	mqLocked, _ := tu.AttrNS(s.mq, "locked")
	mqPercent, _ := tu.AttrNS(s.mq, "percent")

	segSrc := tu.Child("seg-source")
	if segSrc == nil || s.format == FormatMQ {
		r := &reader{defs: defs}
		sg := &segment{tu: tu, srcNode: src, tgtEl: tgt, tgtNode: tgt, tags: tagTable{}}
		sg.unit = base
		sg.unit.ID = block
		sg.unit.Source = r.read(src.Kids)
		if tgt != nil {
			sg.unit.Target = r.read(tgt.Kids)
		}
		sg.tags.add(sg.unit.Source)
		sg.tags.add(sg.unit.Target)
		hasText := tgt != nil && strings.TrimSpace(sg.unit.Target.Plain()) != ""
		switch s.format {
		case FormatMQ:
			sg.mqStat = mqStatus
			sg.unit.State = fromMQ(mqStatus, hasText)
			sg.unit.Locked = mqLocked == "locked"
			sg.unit.Match, _ = strconv.Atoi(mqPercent)
		default:
			st := ""
			if tgt != nil {
				st = tgt.Get("state")
			}
			sg.unit.State = from12(st, tu.Get("approved") == "yes", hasText)
		}
		s.noteReader(r)
		return s.add(sg)
	}

	// Segmented: one unit per <mrk mtype="seg"> of seg-source; the target
	// holds markers with the same mid.
	targets := map[string]*xmlraw.Node{}
	if tgt != nil {
		for _, m := range segMarkers(tgt) {
			targets[m.Get("mid")] = m
		}
	}
	sdlSegs := map[string]*xmlraw.Node{}
	if segDefs != nil {
		for _, k := range segDefs.Elements() {
			if k.Local == "seg" {
				sdlSegs[k.Get("id")] = k
			}
		}
	}
	markers := segMarkers(segSrc)
	if len(markers) == 0 {
		return nil // nothing translatable (formatting only)
	}
	tuState := ""
	if tgt != nil {
		tuState = tgt.Get("state")
	}
	for _, m := range markers {
		mid := m.Get("mid")
		r := &reader{defs: defs}
		sg := &segment{tu: tu, srcNode: m, segSrc: segSrc, tgtEl: tgt, tgtNode: targets[mid], mid: mid, segDefs: segDefs, sdlSeg: sdlSegs[mid], tags: tagTable{}}
		sg.unit = base
		sg.unit.ID = block + "/" + mid
		sg.unit.Source = r.read(m.Kids)
		if sg.tgtNode != nil {
			sg.unit.Target = r.read(sg.tgtNode.Kids)
		}
		sg.tags.add(sg.unit.Source)
		sg.tags.add(sg.unit.Target)
		hasText := sg.tgtNode != nil && strings.TrimSpace(sg.unit.Target.Plain()) != ""
		if s.format == FormatSDL {
			if sd := sg.sdlSeg; sd != nil {
				sg.conf = sd.Get("conf")
				sg.unit.Locked = sd.Get("locked") == "true"
				sg.unit.Match, _ = strconv.Atoi(sd.Get("percent"))
				sg.unit.Origin = sd.Get("origin")
			}
			sg.unit.State = fromSDL(sg.conf)
		} else {
			sg.unit.State = from12(tuState, tu.Get("approved") == "yes", hasText)
		}
		s.noteReader(r)
		if err := s.add(sg); err != nil {
			return err
		}
	}
	return nil
}

func (s *scan) noteReader(r *reader) {
	for _, n := range r.notes {
		s.warn(n)
	}
	if r.revised {
		s.warn("Some segments contain tracked changes. Deleted text is kept as a tag and not counted; inserted text is translatable.")
	}
}

// segMarkers returns the <mrk mtype="seg"> markers under n in document
// order; they may sit inside formatting (<g>).
func segMarkers(n *xmlraw.Node) []*xmlraw.Node {
	var out []*xmlraw.Node
	n.Walk(func(k *xmlraw.Node) bool {
		if k == n {
			return true
		}
		if k.Local == "mrk" && k.Get("mtype") == "seg" {
			out = append(out, k)
			return false
		}
		return k.Local == "g" || k.Local == "mrk"
	})
	return out
}

func (s *scan) scan2() error {
	root := s.doc.Root
	s.ext.SourceLang = root.Get("srcLang")
	s.ext.TargetLang = root.Get("trgLang")
	if s.ext.SourceLang == "" {
		return errNoLanguage
	}
	files := root.Children("file")
	if len(files) == 0 {
		return fmt.Errorf("%w: no <file> element", ErrNotXLIFF)
	}
	for fi, file := range files {
		if err := s.walk2(fi, file, file, file.Get("translate") == "no"); err != nil {
			return err
		}
	}
	return nil
}

func (s *scan) walk2(fi int, file, n *xmlraw.Node, skip bool) error {
	for _, k := range n.Elements() {
		switch k.Local {
		case "group":
			if err := s.walk2(fi, file, k, skip || k.Get("translate") == "no"); err != nil {
				return err
			}
		case "unit":
			if skip || k.Get("translate") == "no" {
				continue
			}
			if err := s.unit2(fi, file, k); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *scan) unit2(fi int, file, u *xmlraw.Node) error {
	uid := u.Get("id")
	if uid == "" {
		return fmt.Errorf("xliff: a unit in file %d has no id", fi+1)
	}
	block := fmt.Sprintf("%d/%s", fi+1, uid)
	s.ext.Blocks = append(s.ext.Blocks, filter.Block{ID: block, Part: filter.PartBody, File: file.Get("original")})
	data := map[string]string{}
	if od := u.Child("originalData"); od != nil {
		for _, d := range od.Children("data") {
			data[d.Get("id")] = d.InnerText()
		}
	}
	var notes []string
	if ns := u.Child("notes"); ns != nil {
		for _, n := range ns.Children("note") {
			if t := strings.TrimSpace(n.InnerText()); t != "" {
				notes = append(notes, t)
			}
		}
	}
	n := 0
	for _, seg := range u.Elements() {
		if seg.Local != "segment" {
			continue
		}
		n++
		src := seg.Child("source")
		if src == nil {
			return fmt.Errorf("xliff: a segment of unit %q has no source", uid)
		}
		tgt := seg.Child("target")
		sid := seg.Get("id")
		if sid == "" {
			sid = "#" + strconv.Itoa(n)
		}
		r := &reader{v2: true, data: data}
		sg := &segment{tu: u, seg2: seg, srcNode: src, tgtEl: tgt, tgtNode: tgt, tags: tagTable{}}
		sg.unit = filter.Unit{ID: block + "/" + sid, Block: block, Key: u.Get("name"), Notes: notes}
		sg.unit.Source = r.read(src.Kids)
		if tgt != nil {
			sg.unit.Target = r.read(tgt.Kids)
		}
		sg.tags.add(sg.unit.Source)
		sg.tags.add(sg.unit.Target)
		hasText := tgt != nil && strings.TrimSpace(sg.unit.Target.Plain()) != ""
		sg.unit.State = from20(seg.Get("state"), seg.Get("subState"), hasText)
		s.noteReader(r)
		if err := s.add(sg); err != nil {
			return err
		}
	}
	return nil
}

// Extract lists the segments of an XLIFF file.
func Extract(data []byte) (*filter.Extraction, error) {
	s, err := read(data)
	if err != nil {
		return nil, err
	}
	return &s.ext, nil
}
