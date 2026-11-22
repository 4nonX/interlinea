package docx

import (
	"sort"
	"strconv"
	"strings"
)

// Run formatting: how it is compared (to find what differs from the
// paragraph's usual formatting) and how a difference is named for people.

// propSet reads run properties into name -> value. WordprocessingML keeps
// properties in child elements (<w:b/>, <w:color w:val=".."/>); DrawingML
// keeps most in attributes (b="1", sz="1200") and colour and font in
// children (<a:solidFill>, <a:latin typeface="..">).
func propSet(rpr []byte, dml bool) map[string]string {
	out := map[string]string{}
	if len(rpr) == 0 {
		return out
	}
	n, err := parseFragment(rpr)
	if err != nil {
		return out
	}
	if dml {
		for _, a := range n.attrs {
			switch a.Name.Local {
			case "lang", "altLang", "dirty", "err", "noProof", "smtClean", "smtId", "bmk", "kumimoji":
			default:
				out[a.Name.Local] = a.Value
			}
		}
	}
	for _, k := range n.kids {
		if k.isText() {
			continue
		}
		switch k.local {
		case "lang", "noProof", "rPrChange", "bCs", "iCs", "szCs", "webHidden":
			continue
		case "solidFill":
			out["color"] = fillColor(k)
		case "latin":
			v, _ := k.attr("typeface")
			out["font"] = v
		case "ea", "cs", "sym":
			continue
		case "rFonts":
			v, ok := k.attr("ascii")
			if !ok {
				v, _ = k.attr("hAnsi")
			}
			if v == "" {
				v, _ = k.attr("asciiTheme")
			}
			out["font"] = v
		default:
			v, ok := k.attr("val")
			if !ok {
				v = "on"
			}
			if hl := k.child("srgbClr"); hl != nil && k.local == "highlight" {
				v, _ = hl.attr("val")
			}
			out[k.local] = v
		}
	}
	return out
}

func fillColor(n *node) string {
	for _, k := range n.kids {
		if k.isText() {
			continue
		}
		if v, ok := k.attr("val"); ok {
			if k.local == "srgbClr" {
				return "#" + v
			}
			return v
		}
	}
	return "set"
}

// propKey is a canonical string of the properties that matter.
func propKey(rpr []byte, dml bool) string {
	set := propSet(rpr, dml)
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k + "=" + set[k] + ";")
	}
	return b.String()
}

func isOn(v string) bool {
	switch v {
	case "", "on", "1", "true":
		return true
	}
	return false
}

// describe names how rpr differs from base. It returns a tag type (bold,
// italic, colour, ...; "format" when several things differ) and a
// description such as "Colour #C00000, Size 9 pt".
func describe(base, rpr []byte, dml bool, styleName func(string) string) (typ, desc string) {
	a, b := propSet(base, dml), propSet(rpr, dml)
	type change struct{ typ, desc string }
	var changes []change
	names := make([]string, 0, len(b))
	for k := range b {
		names = append(names, k)
	}
	for k := range a {
		if _, ok := b[k]; !ok {
			names = append(names, k)
		}
	}
	sort.Strings(names)
	for _, k := range names {
		bv, inB := b[k]
		av, inA := a[k]
		if inA && inB && av == bv {
			continue
		}
		on := inB && isOn(bv)
		switch k {
		case "b":
			if on {
				changes = append(changes, change{"bold", "Bold"})
			} else {
				changes = append(changes, change{"format", "Not bold"})
			}
		case "i":
			if on {
				changes = append(changes, change{"italic", "Italic"})
			} else {
				changes = append(changes, change{"format", "Not italic"})
			}
		case "u":
			if inB && bv != "none" {
				changes = append(changes, change{"underline", "Underline"})
			} else {
				changes = append(changes, change{"format", "Not underlined"})
			}
		case "strike", "dstrike":
			if inB && bv != "noStrike" && (dml || isOn(bv)) {
				changes = append(changes, change{"strike", "Strikethrough"})
			} else {
				changes = append(changes, change{"format", "Not struck through"})
			}
		case "vertAlign":
			switch bv {
			case "superscript":
				changes = append(changes, change{"superscript", "Superscript"})
			case "subscript":
				changes = append(changes, change{"subscript", "Subscript"})
			default:
				changes = append(changes, change{"format", "Normal position"})
			}
		case "baseline":
			n, _ := strconv.Atoi(bv)
			switch {
			case n > 0:
				changes = append(changes, change{"superscript", "Superscript"})
			case n < 0:
				changes = append(changes, change{"subscript", "Subscript"})
			default:
				changes = append(changes, change{"format", "Normal position"})
			}
		case "color":
			if inB {
				if len(bv) == 6 && strings.Trim(strings.ToUpper(bv), "0123456789ABCDEF") == "" {
					bv = "#" + strings.ToUpper(bv)
				}
				changes = append(changes, change{"color", "Colour " + bv})
			} else {
				changes = append(changes, change{"color", "Automatic colour"})
			}
		case "highlight":
			if inB && bv != "none" {
				changes = append(changes, change{"highlight", "Highlight " + bv})
			} else {
				changes = append(changes, change{"format", "No highlight"})
			}
		case "shd":
			changes = append(changes, change{"highlight", "Shading"})
		case "font":
			changes = append(changes, change{"font", "Font " + bv})
		case "sz":
			if n, err := strconv.ParseFloat(bv, 64); err == nil {
				pt := n / 2
				if dml {
					pt = n / 100
				}
				changes = append(changes, change{"size", "Size " + strconv.FormatFloat(pt, 'f', -1, 64) + " pt"})
			} else {
				changes = append(changes, change{"size", "Size"})
			}
		case "caps":
			changes = append(changes, change{"caps", "All caps"})
		case "smallCaps":
			changes = append(changes, change{"caps", "Small caps"})
		case "cap":
			changes = append(changes, change{"caps", map[string]string{"all": "All caps", "small": "Small caps"}[bv]})
		case "rStyle":
			name := bv
			if styleName != nil && styleName(bv) != "" {
				name = styleName(bv)
			}
			if inB {
				changes = append(changes, change{"style", "Character style " + name})
			} else {
				changes = append(changes, change{"style", "No character style"})
			}
		case "vanish", "specVanish":
			changes = append(changes, change{"hidden", "Hidden text"})
		case "hlinkClick":
			changes = append(changes, change{"link", "Link"})
		default:
			changes = append(changes, change{"format", "Other formatting (" + k + ")"})
		}
	}
	switch len(changes) {
	case 0:
		return "format", "Formatting"
	case 1:
		return changes[0].typ, changes[0].desc
	}
	// Bold and italic together read better as one description.
	var descs []string
	for _, c := range changes {
		if c.desc != "" {
			descs = append(descs, c.desc)
		}
	}
	return "format", strings.Join(descs, ", ")
}
