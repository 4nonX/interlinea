package xliff

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter"
	"github.com/4nonX/interlinea/internal/xmlraw"
	"github.com/4nonX/interlinea/langtag"
)

// Wordfast Pro writes its bilingual files (.txml) as XML of its own: the
// root <txml> names the languages (locale, targetlocale); each
// <translatable> block holds <segment>s, each with a <source>, a <target>
// once translated, and <ws> for the space and codes between segments.
// The document's codes are <ut x="1" type="…">, with the original markup
// as their text. A segment with unconfirmed="true" is a draft, one with
// gtmt="true" came from machine translation, and a target's score is the
// match it came from.

// FormatTXML is a Wordfast Pro bilingual file.
const FormatTXML = "txml"

type txmlSegment struct {
	unit             filter.Unit
	seg, src, target *xmlraw.Node
	// after is where a new target goes: after the <ws> that follows the
	// source, else after the source.
	after int
	tags  tagTable
}

type txmlFile struct {
	doc      *xmlraw.Document
	segments []*txmlSegment
	byID     map[string]*txmlSegment
	ext      filter.Extraction
}

func txmlLang(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if n, err := langtag.Normalize(s); err == nil {
		return n
	}
	return s
}

func readTXML(doc *xmlraw.Document) (*txmlFile, error) {
	root := doc.Root
	f := &txmlFile{doc: doc, byID: map[string]*txmlSegment{}}
	f.ext.Format = FormatTXML
	f.ext.SourceLang = txmlLang(root.Get("locale"))
	f.ext.TargetLang = txmlLang(root.Get("targetlocale"))
	if f.ext.SourceLang == "" {
		return nil, errNoLanguage
	}
	blocks := 0
	for _, tr := range root.Elements() {
		if tr.Local != "translatable" {
			continue
		}
		blocks++
		blockID := tr.Get("blockId")
		if blockID == "" {
			blockID = strconv.Itoa(blocks)
		}
		block := "b" + blockID
		f.ext.Blocks = append(f.ext.Blocks, filter.Block{ID: block, Part: filter.PartBody, File: root.Get("file_name")})
		for _, sg := range tr.Elements() {
			if sg.Local != "segment" {
				continue
			}
			src := sg.Child("source")
			if src == nil {
				continue
			}
			t := &txmlSegment{seg: sg, src: src, target: sg.Child("target"), after: src.Whole()[1], tags: tagTable{}}
			for _, k := range sg.Elements() {
				if k.Local == "ws" && k.Off > src.Off && (t.target == nil || k.Off < t.target.Off) {
					t.after = k.Whole()[1]
				}
			}
			id := sg.Get("segmentId")
			if id == "" {
				id = strconv.Itoa(len(f.segments) + 1)
			}
			t.unit = filter.Unit{ID: block + "/" + id, Block: block}
			t.unit.Source = txmlContent(src)
			if t.target != nil {
				t.unit.Target = txmlContent(t.target)
			}
			t.tags.add(t.unit.Source)
			t.tags.add(t.unit.Target)
			hasText := t.target != nil && strings.TrimSpace(t.unit.Target.Plain()) != ""
			switch {
			case !hasText:
				t.unit.State = filter.StateNew
			case sg.Get("unconfirmed") == "true":
				t.unit.State = filter.StateDraft
			default:
				t.unit.State = filter.StateTranslated
			}
			t.unit.Locked = sg.Get("locked") == "true"
			if t.target != nil {
				t.unit.Match, _ = strconv.Atoi(t.target.Get("score"))
			}
			if sg.Get("gtmt") == "true" && hasText {
				t.unit.Origin = "mt"
			}
			if _, dup := f.byID[t.unit.ID]; dup {
				return nil, fmt.Errorf("xliff: two segments have the ID %q", t.unit.ID)
			}
			f.byID[t.unit.ID] = t
			f.segments = append(f.segments, t)
			f.ext.Units = append(f.ext.Units, t.unit)
		}
	}
	return f, nil
}

// txmlContent reads a source or target: text, and each <ut> a placeholder
// named by its x, with its markup kept.
func txmlContent(n *xmlraw.Node) content.Content {
	c := content.Content{}
	for _, k := range n.Kids {
		switch {
		case k.CharData:
			c = c.Append(k.Text)
		case k.Local == "ut":
			id := k.Get("x")
			if id == "" {
				id = strconv.Itoa(len(c.Tags()) + 1)
			}
			c = c.AppendTag(content.Tag{Kind: content.TagStandalone, ID: "x" + id, Type: k.Get("type"), Desc: truncate(k.InnerText(), 80), Native: string(k.Raw)})
		case k.IsElement():
			// Anything else Wordfast puts in a segment stays as it is.
			c = c.AppendTag(content.Tag{Kind: content.TagStandalone, ID: "e" + strconv.Itoa(len(c.Tags())+1), Native: string(k.Raw)})
		}
	}
	return c
}

func (f *txmlFile) merge(updates map[string]Update) ([]byte, error) {
	w := &writer{starts: map[*xmlraw.Node][]byte{}, replaced: map[*xmlraw.Node]bool{}}
	for id, u := range updates {
		t, ok := f.byID[id]
		if !ok {
			return nil, fmt.Errorf("%w: %q", ErrNoSegment, id)
		}
		if u.State != "" && !u.State.Valid() {
			return nil, fmt.Errorf("%w %q for segment %q", ErrBadState, u.State, id)
		}
		changed := u.Target != nil && !sameContent(u.Target, t.unit.Target)
		st := u.State
		if st == "" {
			st = t.unit.State
		}
		if !changed && st == t.unit.State {
			continue
		}
		if t.unit.Locked {
			return nil, fmt.Errorf("%w: %q", ErrLocked, id)
		}
		if changed {
			inner, err := render(u.Target, t.tags)
			if err != nil {
				return nil, fmt.Errorf("segment %q: %w", id, err)
			}
			if t.target != nil {
				w.fill(t.target, inner)
			} else {
				w.edits = append(w.edits, xmlraw.Edit{Span: xmlraw.Span{t.after, t.after}, With: []byte("<target>" + inner + "</target>")})
			}
			w.setAttr(t.seg, "modified", "true")
		}
		if st == filter.StateDraft || st == filter.StateNew || st == filter.StateRejected {
			if len(u.Target.Plain()) > 0 || (!changed && t.target != nil) {
				w.setAttr(t.seg, "unconfirmed", "true")
			}
		} else {
			w.removeAttr(t.seg, "unconfirmed")
		}
	}
	for n, b := range w.starts {
		if !w.replaced[n] {
			w.edits = append(w.edits, xmlraw.Edit{Span: n.StartTag(), With: b})
		}
	}
	out, err := xmlraw.Apply(f.doc.Data, w.edits)
	if err != nil {
		return nil, err
	}
	if _, err := xmlraw.Parse(out); err != nil {
		return nil, fmt.Errorf("xliff: the merged file is not well-formed: %w", err)
	}
	return f.doc.Encode(out), nil
}
