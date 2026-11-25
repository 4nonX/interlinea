package xliff

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter"
	"github.com/4nonX/interlinea/internal/xmlraw"
)

// writer collects the changes to one file.
type writer struct {
	s     *scan
	edits []xmlraw.Edit
	// starts holds rewritten start tags of elements whose attributes
	// change; replaced lists elements rewritten as a whole.
	starts   map[*xmlraw.Node][]byte
	replaced map[*xmlraw.Node]bool
}

func (w *writer) start(n *xmlraw.Node) []byte {
	if b, ok := w.starts[n]; ok {
		return b
	}
	return n.Start
}

func (w *writer) setAttr(n *xmlraw.Node, name, value string) {
	w.starts[n] = xmlraw.SetAttr(w.start(n), name, value)
}

func (w *writer) removeAttr(n *xmlraw.Node, name string) {
	cur := w.start(n)
	if out := xmlraw.RemoveAttr(cur, name); !bytes.Equal(out, cur) {
		w.starts[n] = out
	}
}

func qualified(prefix, local string) string {
	if prefix == "" {
		return local
	}
	return prefix + ":" + local
}

// fill writes new content into an element: its inner bytes, or, for an
// element written as <x/>, the whole element.
func (w *writer) fill(n *xmlraw.Node, inner string) {
	if n.SelfClosing() {
		w.replaced[n] = true
		w.edits = append(w.edits, xmlraw.Edit{Span: n.Whole(), With: []byte(openTagOf(w.start(n)) + inner + "</" + n.Name() + ">")})
		return
	}
	w.edits = append(w.edits, xmlraw.Edit{Span: n.Inner(), With: []byte(inner)})
}

// openTagOf turns a self-closing start tag into an opening one.
func openTagOf(start []byte) string {
	s := string(start)
	if len(s) >= 2 && s[len(s)-2:] == "/>" {
		s = s[:len(s)-2]
		for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t' || s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
			s = s[:len(s)-1]
		}
		return s + ">"
	}
	return s
}

// Merge writes translations and states into an XLIFF file. Segments are
// named by the unit IDs Extract returns. A locked segment cannot change.
func Merge(data []byte, updates map[string]Update) ([]byte, error) {
	s, err := read(data)
	if err != nil {
		return nil, err
	}
	w := &writer{s: s, starts: map[*xmlraw.Node][]byte{}, replaced: map[*xmlraw.Node]bool{}}
	byTU := map[*xmlraw.Node][]*segment{}
	var tus []*xmlraw.Node
	for id, u := range updates {
		sg, ok := s.byID[id]
		if !ok {
			return nil, fmt.Errorf("%w: %q", ErrNoSegment, id)
		}
		if u.State != "" && !u.State.Valid() {
			return nil, fmt.Errorf("%w %q for segment %q", ErrBadState, u.State, id)
		}
		changed := u.Target != nil && !sameContent(u.Target, sg.unit.Target)
		restate := u.State != "" && u.State != sg.unit.State
		if !changed && !restate {
			continue
		}
		if sg.unit.Locked {
			return nil, fmt.Errorf("%w: %q", ErrLocked, id)
		}
		if _, ok := byTU[sg.tu]; !ok {
			tus = append(tus, sg.tu)
		}
		byTU[sg.tu] = append(byTU[sg.tu], sg)
	}
	sort.Slice(tus, func(i, j int) bool { return tus[i].Off < tus[j].Off })
	for _, tu := range tus {
		segs := byTU[tu]
		sort.Slice(segs, func(i, j int) bool { return segs[i].srcNode.Off < segs[j].srcNode.Off })
		var err error
		if segs[0].segSrc != nil {
			err = w.segmented(tu, segs, updates)
		} else {
			for _, sg := range segs {
				if err = w.single(sg, updates[sg.unit.ID]); err != nil {
					break
				}
			}
		}
		if err != nil {
			return nil, err
		}
	}
	for n, b := range w.starts {
		if !w.replaced[n] {
			w.edits = append(w.edits, xmlraw.Edit{Span: n.StartTag(), With: b})
		}
	}
	out, err := xmlraw.Apply(s.doc.Data, w.edits)
	if err != nil {
		return nil, err
	}
	// The result must still be a file this filter (and so other tools) can read.
	if _, err := xmlraw.Parse(out); err != nil {
		return nil, fmt.Errorf("xliff: the merged file is not well-formed: %w", err)
	}
	return s.doc.Encode(out), nil
}

// sameContent compares text and tags (kind and ID).
func sameContent(a, b content.Content) bool {
	return a.Equal(b)
}

func (w *writer) target(sg *segment, u Update) (string, bool, error) {
	if u.Target == nil || sameContent(u.Target, sg.unit.Target) {
		return "", false, nil
	}
	tags := sg.tags
	if len(u.Joined) > 0 {
		// The tags of the segments translated with this one, numbered on.
		tags = tagTable{}
		for k, v := range sg.tags {
			tags[k] = v
		}
		src := sg.unit.Source
		for _, id := range u.Joined {
			other, ok := w.s.byID[id]
			if !ok {
				return "", false, fmt.Errorf("%w: %q", ErrNoSegment, id)
			}
			if other.tu != sg.tu {
				return "", false, fmt.Errorf("xliff: segments %q and %q are not in the same unit and cannot be joined", sg.unit.ID, id)
			}
			more, back := filter.JoinTags(src, other.unit.Source)
			for now, own := range back {
				for _, kind := range []content.TagKind{content.TagOpen, content.TagClose, content.TagStandalone} {
					if native, ok := other.tags[tagKey{kind, own}]; ok {
						tags[tagKey{kind, now}] = native
					}
				}
			}
			src = append(src.Clone(), more...)
		}
	}
	inner, err := render(u.Target, tags)
	if err != nil {
		return "", false, fmt.Errorf("segment %q: %w", sg.unit.ID, err)
	}
	return inner, true, nil
}

func (w *writer) newState(sg *segment, u Update) filter.State {
	if u.State != "" {
		return u.State
	}
	return sg.unit.State
}

// single handles a segment that is a whole trans-unit (XLIFF 1.2 without
// seg-source, MQXLIFF) or an XLIFF 2 segment.
func (w *writer) single(sg *segment, u Update) error {
	inner, changed, err := w.target(sg, u)
	if err != nil {
		return err
	}
	st := w.newState(sg, u)
	restate := st != sg.unit.State
	v2 := w.s.format == FormatXLIFF2
	mq := w.s.format == FormatMQ

	switch {
	case changed && sg.tgtEl != nil:
		if !v2 && !mq {
			w.setAttr(sg.tgtEl, "state", to12(st))
		}
		w.fill(sg.tgtEl, inner)
	case changed:
		open := "<target>"
		if !v2 && !mq {
			open = `<target state="` + to12(st) + `">`
		}
		w.edits = append(w.edits, xmlraw.Edit{Span: xmlraw.Span{sg.srcNode.Whole()[1], sg.srcNode.Whole()[1]}, With: []byte(open + inner + "</target>")})
	case restate && sg.tgtEl != nil && !v2 && !mq:
		w.setAttr(sg.tgtEl, "state", to12(st))
	}

	switch {
	case v2:
		if restate || changed {
			state, sub := to20(st)
			w.setAttr(sg.seg2, "state", state)
			if sub != "" {
				w.setAttr(sg.seg2, "subState", sub)
			} else if _, has := sg.seg2.Attr("subState"); has && restate {
				w.removeAttr(sg.seg2, "subState")
			}
		}
	case mq:
		if restate || changed {
			w.setAttr(sg.tu, qualified(w.s.mq, "status"), toMQ(st, sg.mqStat, changed))
		}
	default:
		if restate {
			w.approved(sg.tu, st)
		}
	}
	return nil
}

// approved keeps the 1.2 approved attribute in line with the state.
func (w *writer) approved(tu *xmlraw.Node, st filter.State) {
	_, has := tu.Attr("approved")
	switch {
	case st == filter.StateFinal:
		w.setAttr(tu, "approved", "yes")
	case has:
		w.setAttr(tu, "approved", "no")
	}
}

// segmented handles the segments of one trans-unit with seg-source
// (SDLXLIFF, and XLIFF 1.2 from tools that segment the same way).
func (w *writer) segmented(tu *xmlraw.Node, segs []*segment, updates map[string]Update) error {
	sdl := w.s.format == FormatSDL
	all := w.s.segmentsOf(tu)
	type change struct {
		inner   string
		changed bool
		state   filter.State
	}
	ch := map[*segment]change{}
	for _, sg := range segs {
		u := updates[sg.unit.ID]
		inner, changed, err := w.target(sg, u)
		if err != nil {
			return err
		}
		ch[sg] = change{inner, changed, w.newState(sg, u)}
	}
	tgt := segs[0].tgtEl

	// The trans-unit's state in plain XLIFF 1.2 is the least advanced of
	// its segments.
	tuState := filter.StateFinal
	for _, sg := range all {
		st := sg.unit.State
		if c, ok := ch[sg]; ok {
			st = c.state
		}
		if st.Rank() < tuState.Rank() || st == filter.StateRejected {
			tuState = st
		}
	}

	if tgt == nil {
		// Build the target from seg-source: the same structure, with each
		// segment's translation (or nothing) in its marker.
		src := segs[0].segSrc
		inner := src.InnerRaw()
		base := src.Inner()[0]
		var local []xmlraw.Edit
		for _, sg := range all {
			c := ch[sg]
			m := sg.srcNode
			span := xmlraw.Span{m.Off - base, m.Off - base + len(m.Raw)}
			local = append(local, xmlraw.Edit{Span: span, With: []byte(openTagOf(m.Start) + c.inner + "</" + m.Name() + ">")})
		}
		built, err := xmlraw.Apply(inner, local)
		if err != nil {
			return err
		}
		open := "<target>"
		if !sdl {
			open = `<target state="` + to12(tuState) + `">`
		}
		w.edits = append(w.edits, xmlraw.Edit{Span: xmlraw.Span{src.Whole()[1], src.Whole()[1]}, With: append(append([]byte(open), built...), "</target>"...)})
	} else {
		for _, sg := range segs {
			c := ch[sg]
			if !c.changed {
				continue
			}
			if sg.tgtNode != nil {
				w.fill(sg.tgtNode, c.inner)
				continue
			}
			// The target lacks this segment's marker: add it after the
			// marker of the segment before it, or at the start.
			at := tgt.Inner()[0]
			for _, prev := range all {
				if prev == sg {
					break
				}
				if prev.tgtNode != nil {
					at = prev.tgtNode.Whole()[1]
				}
			}
			w.edits = append(w.edits, xmlraw.Edit{Span: xmlraw.Span{at, at}, With: []byte(openTagOf(sg.srcNode.Start) + c.inner + "</" + sg.srcNode.Name() + ">")})
		}
		if !sdl {
			w.setAttr(tgt, "state", to12(tuState))
		}
	}
	if !sdl {
		w.approved(tu, tuState)
		return nil
	}

	// Trados keeps each segment's confirmation level in sdl:seg-defs.
	var created []byte
	for _, sg := range segs {
		c := ch[sg]
		conf := toSDL(c.state, sg.conf)
		if sg.sdlSeg == nil {
			if conf == "" && !c.changed {
				continue
			}
			el := "<" + qualified(w.s.sdl, "seg") + ` id="` + xmlraw.EscapeAttr(sg.mid) + `"`
			if conf != "" {
				el += ` conf="` + conf + `"`
			}
			if c.changed {
				el += ` origin="interactive"`
			}
			created = append(created, el+"/>"...)
			continue
		}
		if conf == "" {
			w.removeAttr(sg.sdlSeg, "conf")
		} else {
			w.setAttr(sg.sdlSeg, "conf", conf)
		}
		if c.changed && sg.sdlSeg.Get("origin") != "interactive" {
			w.setAttr(sg.sdlSeg, "origin", "interactive")
		}
	}
	if len(created) > 0 {
		defs := segs[0].segDefs
		switch {
		case defs != nil && defs.SelfClosing():
			w.replaced[defs] = true
			w.edits = append(w.edits, xmlraw.Edit{Span: defs.Whole(), With: append(append([]byte(openTagOf(w.start(defs))), created...), "</"+defs.Name()+">"...)})
		case defs != nil:
			w.edits = append(w.edits, xmlraw.Edit{Span: xmlraw.Span{defs.Inner()[1], defs.Inner()[1]}, With: created})
		default:
			at := segs[0].segSrc.Whole()[1]
			if tgt != nil {
				at = tgt.Whole()[1]
			}
			name := qualified(w.s.sdl, "seg-defs")
			w.edits = append(w.edits, xmlraw.Edit{Span: xmlraw.Span{at, at}, With: append(append([]byte("<"+name+">"), created...), "</"+name+">"...)})
		}
	}
	return nil
}

func (s *scan) segmentsOf(tu *xmlraw.Node) []*segment {
	var out []*segment
	for _, sg := range s.segments {
		if sg.tu == tu {
			out = append(out, sg)
		}
	}
	return out
}

// Translations writes plain translations (as filter.Translations) with the
// state translated, for callers that do not track states.
func Translations(data []byte, tr filter.Translations) ([]byte, error) {
	up := make(map[string]Update, len(tr))
	for id, c := range tr {
		up[id] = Update{Target: c, State: filter.StateTranslated}
	}
	return Merge(data, up)
}
