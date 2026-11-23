package xliff

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/internal/xmlraw"
)

// ErrTags means a translation's tags do not fit the unit: a tag the unit
// does not have, or a span opened but not closed.
var ErrTags = errors.New("xliff: the translation's tags do not match the unit")

// tagDef describes an SDLXLIFF tag from the file header (tag-defs), which
// holds the original code a <g> or <x> stands for.
type tagDef struct {
	desc string
}

// reader turns inline XLIFF markup into content. One reader is used per
// unit, so tag IDs stay unique within it.
type reader struct {
	v2      bool
	data    map[string]string // XLIFF 2: originalData by id
	defs    map[string]tagDef // SDLXLIFF tag-defs
	marks   int               // counter for markers without an ID
	notes   []string          // warnings
	revised bool              // SDL tracked changes seen
}

func truncate(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

// tagType maps XLIFF ctype values to Interlinea's tag types.
func tagType(ctype, fallback string) string {
	switch strings.ToLower(ctype) {
	case "bold", "x-bold":
		return "bold"
	case "italic", "x-italic":
		return "italic"
	case "underlined", "underline", "x-underline":
		return "underline"
	case "link", "x-link", "x-html-a":
		return "link"
	case "lb", "x-lb", "x-html-br":
		return "lb"
	case "x-sup", "superscript":
		return "superscript"
	case "x-sub", "subscript":
		return "subscript"
	case "":
		return fallback
	default:
		return fallback
	}
}

// openTag of an element with content: the start tag (a self-closing
// element gets a separate start tag so the pair can wrap text).
func openNative(n *xmlraw.Node) string {
	if n.SelfClosing() {
		s := strings.TrimRight(strings.TrimSuffix(string(n.Start), "/>"), " \t\r\n")
		return s + ">"
	}
	return string(n.Start)
}

func closeNative(n *xmlraw.Node) string {
	if n.SelfClosing() {
		return "</" + n.Name() + ">"
	}
	return string(n.End)
}

// pairs finds the IDs of paired codes (bpt/ept in 1.2, sc/ec and sm/em in
// 2.0) whose both halves are in the content, so they become an open and a
// close tag; a lone half is a standalone tag.
func (r *reader) pairs(kids []*xmlraw.Node) map[string]bool {
	starts, ends := map[string]bool{}, map[string]bool{}
	var walk func(ns []*xmlraw.Node)
	walk = func(ns []*xmlraw.Node) {
		for _, k := range ns {
			if !k.IsElement() {
				continue
			}
			switch k.Local {
			case "bpt":
				starts[pairKey(k)] = true
			case "ept":
				ends[pairKey(k)] = true
			case "sc", "sm":
				if k.Get("isolated") != "yes" {
					starts[k.Get("id")] = true
				}
			case "ec", "em":
				if k.Get("isolated") != "yes" {
					ends[k.Get("startRef")] = true
				}
			}
			switch k.Local {
			case "g", "mrk", "pc":
				walk(k.Kids)
			}
		}
	}
	walk(kids)
	out := map[string]bool{}
	for id := range starts {
		if ends[id] {
			out[id] = true
		}
	}
	return out
}

// pairKey is the ID a 1.2 bpt/ept pair shares: rid when given, else id.
func pairKey(n *xmlraw.Node) string {
	if v := n.Get("rid"); v != "" {
		return v
	}
	return n.Get("id")
}

// read converts the children of a source, target or segment marker.
func (r *reader) read(kids []*xmlraw.Node) content.Content {
	paired := r.pairs(kids)
	var c content.Content
	r.readInto(&c, kids, paired)
	return c.Normalize()
}

func (r *reader) id(n *xmlraw.Node, prefix string) string {
	if v := n.Get("id"); v != "" {
		return v
	}
	r.marks++
	return prefix + strconv.Itoa(r.marks)
}

func (r *reader) readInto(c *content.Content, kids []*xmlraw.Node, paired map[string]bool) {
	for _, k := range kids {
		if k.CharData {
			*c = c.Append(k.Text)
			continue
		}
		if !k.IsElement() {
			// Comments and processing instructions stay as they are.
			*c = c.AppendTag(content.Tag{Kind: content.TagStandalone, ID: r.id(k, "c"), Type: "inline", Native: string(k.Raw)})
			continue
		}
		if r.v2 {
			r.read2(c, k, paired)
		} else {
			r.read1(c, k, paired)
		}
	}
}

func (r *reader) desc(id, fallback string) string {
	if d, ok := r.defs[id]; ok && d.desc != "" {
		return d.desc
	}
	return fallback
}

func (r *reader) read1(c *content.Content, k *xmlraw.Node, paired map[string]bool) {
	switch k.Local {
	case "g":
		id := r.id(k, "g")
		t := tagType(k.Get("ctype"), "format")
		d := r.desc(id, k.Get("ctype"))
		*c = c.AppendTag(content.Tag{Kind: content.TagOpen, ID: id, Type: t, Desc: d, Native: openNative(k)})
		r.readInto(c, k.Kids, paired)
		*c = c.AppendTag(content.Tag{Kind: content.TagClose, ID: id, Type: t, Desc: d, Native: closeNative(k)})
	case "x", "bx", "ex", "ph", "it":
		id := r.id(k, k.Local)
		fallback := truncate(k.InnerText(), 60)
		if fallback == "" {
			fallback = k.Get("ctype")
		}
		*c = c.AppendTag(content.Tag{Kind: content.TagStandalone, ID: id, Type: tagType(k.Get("ctype"), "inline"), Desc: r.desc(id, fallback), Native: string(k.Raw)})
	case "bpt", "ept":
		key := pairKey(k)
		kind := content.TagStandalone
		if paired[key] {
			kind = content.TagOpen
			if k.Local == "ept" {
				kind = content.TagClose
			}
		} else {
			key = r.id(k, k.Local)
		}
		*c = c.AppendTag(content.Tag{Kind: kind, ID: key, Type: tagType(k.Get("ctype"), "format"), Desc: truncate(k.InnerText(), 60), Native: string(k.Raw)})
	case "mrk":
		r.mark(c, k, k.Get("mtype"), paired)
	default:
		*c = c.AppendTag(content.Tag{Kind: content.TagStandalone, ID: r.id(k, "e"), Type: "inline", Desc: k.Name(), Native: string(k.Raw)})
		r.notes = append(r.notes, fmt.Sprintf("Unknown inline element <%s> kept as a tag.", k.Name()))
	}
}

// mark handles markers: deleted revisions and protected text stay out of
// the translatable text; other markers (comments, terms, added
// revisions) wrap their text.
func (r *reader) mark(c *content.Content, k *xmlraw.Node, mtype string, paired map[string]bool) {
	switch {
	case mtype == "x-sdl-deleted" || mtype == "protected" || k.Get("translate") == "no":
		if mtype == "x-sdl-deleted" {
			r.revised = true
		}
		*c = c.AppendTag(content.Tag{Kind: content.TagStandalone, ID: r.id(k, "m"), Type: "protected", Desc: truncate(k.InnerText(), 60), Native: string(k.Raw)})
	default:
		if mtype == "x-sdl-added" {
			r.revised = true
		}
		id := r.id(k, "m")
		t := "mark"
		if strings.Contains(mtype, "comment") {
			t = "comment"
		}
		*c = c.AppendTag(content.Tag{Kind: content.TagOpen, ID: id, Type: t, Desc: mtype, Native: openNative(k)})
		r.readInto(c, k.Kids, paired)
		*c = c.AppendTag(content.Tag{Kind: content.TagClose, ID: id, Type: t, Desc: mtype, Native: closeNative(k)})
	}
}

func (r *reader) data2(k *xmlraw.Node, attr string) string {
	if ref := k.Get(attr); ref != "" {
		return truncate(r.data[ref], 60)
	}
	return ""
}

func (r *reader) read2(c *content.Content, k *xmlraw.Node, paired map[string]bool) {
	switch k.Local {
	case "pc":
		id := r.id(k, "pc")
		t := tagType(k.Get("type"), "format")
		d := r.data2(k, "dataRefStart")
		*c = c.AppendTag(content.Tag{Kind: content.TagOpen, ID: id, Type: t, Desc: d, Native: openNative(k)})
		r.readInto(c, k.Kids, paired)
		*c = c.AppendTag(content.Tag{Kind: content.TagClose, ID: id, Type: t, Desc: r.data2(k, "dataRefEnd"), Native: closeNative(k)})
	case "ph":
		*c = c.AppendTag(content.Tag{Kind: content.TagStandalone, ID: r.id(k, "ph"), Type: tagType(k.Get("type"), "inline"), Desc: r.data2(k, "dataRef"), Native: string(k.Raw)})
	case "sc", "ec":
		id := k.Get("id")
		if k.Local == "ec" && k.Get("startRef") != "" {
			id = k.Get("startRef")
		}
		kind := content.TagStandalone
		if paired[id] && k.Get("isolated") != "yes" {
			kind = content.TagOpen
			if k.Local == "ec" {
				kind = content.TagClose
			}
		} else {
			id = r.id(k, k.Local)
		}
		*c = c.AppendTag(content.Tag{Kind: kind, ID: id, Type: tagType(k.Get("type"), "format"), Desc: r.data2(k, "dataRef"), Native: string(k.Raw)})
	case "sm", "em":
		id := k.Get("id")
		if k.Local == "em" {
			id = k.Get("startRef")
		}
		kind := content.TagStandalone
		if paired[id] {
			kind = content.TagOpen
			if k.Local == "em" {
				kind = content.TagClose
			}
		} else {
			id = r.id(k, k.Local)
		}
		*c = c.AppendTag(content.Tag{Kind: kind, ID: id, Type: "mark", Desc: k.Get("type"), Native: string(k.Raw)})
	case "mrk":
		r.mark(c, k, k.Get("type"), paired)
	case "cp":
		*c = c.AppendTag(content.Tag{Kind: content.TagStandalone, ID: r.id(k, "cp"), Type: "inline", Desc: "U+" + strings.ToUpper(k.Get("hex")), Native: string(k.Raw)})
	default:
		*c = c.AppendTag(content.Tag{Kind: content.TagStandalone, ID: r.id(k, "e"), Type: "inline", Desc: k.Name(), Native: string(k.Raw)})
		r.notes = append(r.notes, fmt.Sprintf("Unknown inline element <%s> kept as a tag.", k.Name()))
	}
}

// tagKey identifies a tag of a unit for writing.
type tagKey struct {
	kind content.TagKind
	id   string
}

// tagTable collects the original markup of a unit's tags (from its source
// and its existing target).
type tagTable map[tagKey]string

func (t tagTable) add(c content.Content) {
	for _, r := range c {
		if r.IsTag() {
			k := tagKey{r.Tag.Kind, r.Tag.ID}
			if _, ok := t[k]; !ok {
				t[k] = r.Tag.Native
			}
		}
	}
}

// render writes content as inline XLIFF. Tags are written with their
// original markup, from the tag itself or, when a translation names a tag
// by kind and ID only, from the unit's tags. Open tags whose markup wraps
// text (<g>, <pc>, <mrk>) must be closed in the same content.
func render(c content.Content, tags tagTable) (string, error) {
	var b strings.Builder
	var stack []string
	for _, r := range c {
		if !r.IsTag() {
			b.WriteString(xmlraw.EscapeText(r.Text))
			continue
		}
		native := r.Tag.Native
		if native == "" {
			native = tags[tagKey{r.Tag.Kind, r.Tag.ID}]
		}
		if native == "" {
			return "", fmt.Errorf("%w: %s tag %q is not in the unit", ErrTags, r.Tag.Kind, r.Tag.ID)
		}
		wraps := isWrapper(native)
		switch {
		case r.Tag.Kind == content.TagOpen && wraps:
			stack = append(stack, r.Tag.ID)
		case r.Tag.Kind == content.TagClose && strings.HasPrefix(native, "</"):
			if len(stack) == 0 || stack[len(stack)-1] != r.Tag.ID {
				return "", fmt.Errorf("%w: tag %q is closed out of order", ErrTags, r.Tag.ID)
			}
			stack = stack[:len(stack)-1]
		}
		b.WriteString(native)
	}
	if len(stack) > 0 {
		return "", fmt.Errorf("%w: tag %q is opened but not closed", ErrTags, stack[len(stack)-1])
	}
	return b.String(), nil
}

// isWrapper reports whether markup is a bare start tag (<g ...>) rather
// than a complete element (<bpt ...>...</bpt>, <x/>).
func isWrapper(native string) bool {
	if !strings.HasPrefix(native, "<") || strings.HasPrefix(native, "</") || strings.HasPrefix(native, "<!") || strings.HasPrefix(native, "<?") {
		return false
	}
	if strings.HasSuffix(native, "/>") {
		return false
	}
	// A complete element ends with its own end tag.
	end := strings.LastIndex(native, "</")
	return end <= 0
}
