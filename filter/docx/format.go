package docx

import (
	"sort"
	"strconv"
	"strings"

	"github.com/4nonX/interlinea/i18n"
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
		for _, a := range n.Attrs {
			switch a.Name.Local {
			case "lang", "altLang", "dirty", "err", "noProof", "smtClean", "smtId", "bmk", "kumimoji":
			default:
				out[a.Name.Local] = a.Value
			}
		}
	}
	for _, k := range n.Kids {
		if !k.IsElement() {
			continue
		}
		switch k.Local {
		case "lang", "noProof", "rPrChange", "bCs", "iCs", "szCs", "webHidden":
			continue
		case "solidFill":
			out["color"] = fillColor(k)
		case "latin":
			v, _ := k.AttrAny("typeface")
			out["font"] = v
		case "ea", "cs", "sym":
			continue
		case "rFonts":
			v, ok := k.AttrAny("ascii")
			if !ok {
				v, _ = k.AttrAny("hAnsi")
			}
			if v == "" {
				v, _ = k.AttrAny("asciiTheme")
			}
			out["font"] = v
		default:
			v, ok := k.AttrAny("val")
			if !ok {
				v = "on"
			}
			if hl := k.Child("srgbClr"); hl != nil && k.Local == "highlight" {
				v, _ = hl.AttrAny("val")
			}
			out[k.Local] = v
		}
	}
	return out
}

func fillColor(n *node) string {
	for _, k := range n.Kids {
		if !k.IsElement() {
			continue
		}
		if v, ok := k.AttrAny("val"); ok {
			if k.Local == "srgbClr" {
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
				changes = append(changes, change{"bold", i18n.N("Bold")})
			} else {
				changes = append(changes, change{"format", i18n.N("Not bold")})
			}
		case "i":
			if on {
				changes = append(changes, change{"italic", i18n.N("Italic")})
			} else {
				changes = append(changes, change{"format", i18n.N("Not italic")})
			}
		case "u":
			if inB && bv != "none" {
				changes = append(changes, change{"underline", i18n.N("Underline")})
			} else {
				changes = append(changes, change{"format", i18n.N("Not underlined")})
			}
		case "strike", "dstrike":
			if inB && bv != "noStrike" && (dml || isOn(bv)) {
				changes = append(changes, change{"strike", i18n.N("Strikethrough")})
			} else {
				changes = append(changes, change{"format", i18n.N("Not struck through")})
			}
		case "vertAlign":
			switch bv {
			case "superscript":
				changes = append(changes, change{"superscript", i18n.N("Superscript")})
			case "subscript":
				changes = append(changes, change{"subscript", i18n.N("Subscript")})
			default:
				changes = append(changes, change{"format", i18n.N("Normal position")})
			}
		case "baseline":
			n, _ := strconv.Atoi(bv)
			switch {
			case n > 0:
				changes = append(changes, change{"superscript", i18n.N("Superscript")})
			case n < 0:
				changes = append(changes, change{"subscript", i18n.N("Subscript")})
			default:
				changes = append(changes, change{"format", i18n.N("Normal position")})
			}
		case "color":
			if inB {
				if len(bv) == 6 && strings.Trim(strings.ToUpper(bv), "0123456789ABCDEF") == "" {
					bv = "#" + strings.ToUpper(bv)
				}
				changes = append(changes, change{"color", i18n.Fill(i18n.N("Colour {value}"), "value", bv)})
			} else {
				changes = append(changes, change{"color", i18n.N("Automatic colour")})
			}
		case "highlight":
			if inB && bv != "none" {
				changes = append(changes, change{"highlight", i18n.Fill(i18n.N("Highlight {value}"), "value", bv)})
			} else {
				changes = append(changes, change{"format", i18n.N("No highlight")})
			}
		case "shd":
			changes = append(changes, change{"highlight", i18n.N("Shading")})
		case "font":
			changes = append(changes, change{"font", i18n.Fill(i18n.N("Font {value}"), "value", bv)})
		case "sz":
			if n, err := strconv.ParseFloat(bv, 64); err == nil {
				pt := n / 2
				if dml {
					pt = n / 100
				}
				changes = append(changes, change{"size", i18n.Fill(i18n.N("Size {n} pt"), "n", strconv.FormatFloat(pt, 'f', -1, 64))})
			} else {
				changes = append(changes, change{"size", i18n.N("Size")})
			}
		case "caps":
			changes = append(changes, change{"caps", i18n.N("All caps")})
		case "smallCaps":
			changes = append(changes, change{"caps", i18n.N("Small caps")})
		case "cap":
			changes = append(changes, change{"caps", map[string]string{"all": i18n.N("All caps"), "small": i18n.N("Small caps")}[bv]})
		case "rStyle":
			name := bv
			if styleName != nil && styleName(bv) != "" {
				name = styleName(bv)
			}
			if inB {
				changes = append(changes, change{"style", i18n.Fill(i18n.N("Character style {name}"), "name", name)})
			} else {
				changes = append(changes, change{"style", i18n.N("No character style")})
			}
		case "vanish", "specVanish":
			changes = append(changes, change{"hidden", i18n.N("Hidden text")})
		case "hlinkClick":
			changes = append(changes, change{"link", i18n.N("Link")})
		default:
			changes = append(changes, change{"format", i18n.Fill(i18n.N("Other formatting ({name})"), "name", k)})
		}
	}
	switch len(changes) {
	case 0:
		return "format", i18n.N("Formatting")
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
