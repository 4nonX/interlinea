package xliff

import (
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter"
	"github.com/4nonX/interlinea/internal/xmlraw"
)

// Phrase (formerly Memsource) writes its bilingual files (.mxliff) as
// XLIFF 1.2 with attributes of its own namespace on each trans-unit:
// m:confirmed ("1" once the segment is confirmed), m:locked, m:score (the
// match, 1.01 for a context match) and m:trans-origin. The document's tags
// are not XLIFF elements but codes in the text: {1} stands alone, {1>
// opens and <1} closes, and {b>, {i>, {u>, {sup>, {sub> with their closing
// codes are formatting. m:tunit-metadata says what each numbered code is.
// The root's m:level is the step of the workflow the file is for: 1
// translation, 2 and on review.

const nsPhrase = "http://www.memsource.com/mxlf/2.0"

// FormatPhrase is a Phrase (Memsource) bilingual file.
const FormatPhrase = "mxliff"

var phraseCode = regexp.MustCompile(`\{(\d+|b|i|u|sup|sub)>|<(\d+|b|i|u|sup|sub)\}|\{(\d+)\}`)

// phraseEscape writes a code as Phrase does: only "&" and "<" escaped.
var phraseEscape = strings.NewReplacer("&", "&amp;", "<", "&lt;")

var phraseTypes = map[string]string{"b": "bold", "i": "italic", "u": "underline", "sup": "superscript", "sub": "subscript"}

// phraseMarks reads what m:tunit-metadata says of a unit's numbered codes.
func phraseMarks(tu *xmlraw.Node) map[string]string {
	out := map[string]string{}
	for _, k := range tu.Elements() {
		if k.Local != "tunit-metadata" {
			continue
		}
		for _, m := range k.Elements() {
			if m.Local != "mark" {
				continue
			}
			var desc string
			for _, d := range m.Elements() {
				switch d.Local {
				case "content":
					desc = strings.TrimSpace(d.InnerText())
				case "type":
					if desc == "" {
						desc = strings.TrimSpace(d.InnerText())
					}
				}
			}
			if id := m.Get("id"); id != "" && desc != "" {
				out[id] = truncate(desc, 120)
			}
		}
	}
	return out
}

// phraseContent turns Phrase's codes in the text runs into tags. A tag's
// markup is the code as the file has it, so a translation is written back
// with the codes Phrase reads.
func phraseContent(c content.Content, marks map[string]string) content.Content {
	out := content.Content{}
	for _, r := range c {
		if r.IsTag() {
			out = out.AppendTag(*r.Tag)
			continue
		}
		last := 0
		for _, m := range phraseCode.FindAllStringSubmatchIndex(r.Text, -1) {
			out = out.Append(r.Text[last:m[0]])
			code := r.Text[m[0]:m[1]]
			t := content.Tag{Native: phraseEscape.Replace(code)}
			var name string
			switch {
			case m[2] >= 0:
				name, t.Kind = r.Text[m[2]:m[3]], content.TagOpen
			case m[4] >= 0:
				name, t.Kind = r.Text[m[4]:m[5]], content.TagClose
			default:
				name, t.Kind = r.Text[m[6]:m[7]], content.TagStandalone
			}
			t.ID = "m" + name
			if typ, ok := phraseTypes[name]; ok {
				t.Type = typ
			} else {
				t.Desc = marks[name]
			}
			out = out.AppendTag(t)
			last = m[1]
		}
		out = out.Append(r.Text[last:])
	}
	return out
}

// phraseUnit reads the state, lock, match and origin of a trans-unit.
func (s *scan) phraseUnit(u *filter.Unit, tu *xmlraw.Node, hasText bool) {
	confirmed, _ := tu.AttrNS(s.phrase, "confirmed")
	locked, _ := tu.AttrNS(s.phrase, "locked")
	u.Locked = locked == "true"
	// In a file for review (level 2 and on) confirming is the reviewer's:
	// a translation not yet confirmed there is the translator's, done.
	review := s.phraseLevel > 1
	switch {
	case confirmed == "1" && review:
		u.State = filter.StateReviewed
	case confirmed == "1":
		u.State = filter.StateTranslated
	case hasText && review:
		u.State = filter.StateTranslated
	case hasText:
		u.State = filter.StateDraft
	default:
		u.State = filter.StateNew
	}
	if score, err := strconv.ParseFloat(attrNS(tu, s.phrase, "score"), 64); err == nil && score > 0 {
		u.Match = int(math.Round(math.Min(score, 1.01) * 100))
	}
	switch o := attrNS(tu, s.phrase, "trans-origin"); o {
	case "tm", "mt":
		u.Origin = o
	}
}

// phraseState writes a state as m:confirmed: in a file for translation
// confirmed from translated on, in a file for review from reviewed on.
func (w *writer) phraseState(tu *xmlraw.Node, st filter.State) {
	v := "0"
	switch st {
	case filter.StateReviewed, filter.StateFinal:
		v = "1"
	case filter.StateTranslated:
		if w.s.phraseLevel <= 1 {
			v = "1"
		}
	}
	w.setAttr(tu, qualified(w.s.phrase, "confirmed"), v)
}

func attrNS(n *xmlraw.Node, prefix, local string) string {
	v, _ := n.AttrNS(prefix, local)
	return v
}
