package pdf

import (
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/unicode/norm"
)

// font is what a page needs of a font to read text shown in it: which
// characters its codes are, and how wide they are.
type font struct {
	composite bool
	// toUnicode maps a code's bytes to text; nil without the map.
	toUnicode map[string]string
	// codeLengths are the lengths of codes, in bytes, a composite font
	// has, the shortest first.
	codeLengths []int
	// utf16 says the codes of a composite font are UTF-16 themselves.
	utf16 bool
	// enc is the text of each code of a simple font.
	enc [256]string
	// widths of a simple font by code, of a composite font by CID, in
	// thousandths of the font size.
	widths       map[int]float64
	defaultWidth float64
	bold, italic bool
}

// glyph is one code of a shown string.
type glyph struct {
	text  string
	width float64 // in thousandths of the font size
	space bool    // the single-byte code 32, which word spacing applies to
	known bool    // whether the font said which character it is
}

func (ft *font) glyphs(s pstring) []glyph {
	var out []glyph
	if !ft.composite {
		for i := 0; i < len(s); i++ {
			c := s[i]
			g := glyph{space: c == 32, width: ft.defaultWidth}
			if w, ok := ft.widths[int(c)]; ok {
				g.width = w
			}
			if t, ok := ft.toUnicode[string(s[i:i+1])]; ok {
				g.text, g.known = t, true
			} else if ft.enc[c] != "" {
				g.text, g.known = ft.enc[c], true
			}
			out = append(out, g)
		}
		return out
	}
	for i := 0; i < len(s); {
		n := 2
		if len(ft.codeLengths) > 0 {
			n = ft.codeLengths[0]
			// The shortest code the map knows; else the shortest there is.
			for _, l := range ft.codeLengths {
				if i+l <= len(s) {
					if _, ok := ft.toUnicode[string(s[i:i+l])]; ok {
						n = l
						break
					}
				}
			}
		}
		if i+n > len(s) {
			n = len(s) - i
		}
		code := string(s[i : i+n])
		cid := 0
		for j := 0; j < n; j++ {
			cid = cid<<8 | int(code[j])
		}
		g := glyph{space: n == 1 && code[0] == 32, width: ft.defaultWidth}
		if w, ok := ft.widths[cid]; ok {
			g.width = w
		}
		if t, ok := ft.toUnicode[code]; ok {
			g.text, g.known = t, true
		} else if ft.utf16 && n == 2 {
			g.text, g.known = string(rune(cid)), true
		}
		out = append(out, g)
		i += n
	}
	if ft.utf16 {
		// Surrogate pairs come as two codes.
		for i := 0; i+1 < len(out); i++ {
			a, b := []rune(out[i].text), []rune(out[i+1].text)
			if len(a) == 1 && len(b) == 1 && utf16.IsSurrogate(a[0]) {
				out[i].text = string(utf16.DecodeRune(a[0], b[0]))
				out[i+1].text = ""
			}
		}
	}
	return out
}

// loadFont reads a font dictionary.
func loadFont(f *file, d dict) *font {
	ft := &font{widths: map[int]float64{}, defaultWidth: 500}
	base := string(f.name(d["BaseFont"]))
	if i := strings.IndexByte(base, '+'); i == 6 {
		base = base[7:] // a subset's tag
	}
	low := strings.ToLower(base)
	ft.bold = strings.Contains(low, "bold") || strings.Contains(low, "black") || strings.Contains(low, "heavy")
	ft.italic = strings.Contains(low, "italic") || strings.Contains(low, "oblique")
	if s, ok := f.resolve(d["ToUnicode"]).(*stream); ok {
		if data, err := decodeStream(f, s); err == nil {
			ft.toUnicode, ft.codeLengths = parseCMap(data)
		}
	}
	desc := f.dict(d["FontDescriptor"])
	if f.name(d["Subtype"]) == "Type0" {
		ft.composite = true
		ft.defaultWidth = 1000
		var cid dict
		if kids := f.array(d["DescendantFonts"]); len(kids) > 0 {
			cid = f.dict(kids[0])
		}
		if desc == nil {
			desc = f.dict(cid["FontDescriptor"])
		}
		if dw, ok := f.number(cid["DW"]); ok {
			ft.defaultWidth = dw
		}
		// W: c [w1 w2 …] or cfirst clast w.
		w := f.array(cid["W"])
		for i := 0; i < len(w); {
			c, ok := f.number(w[i])
			if !ok || i+1 >= len(w) {
				break
			}
			if list, ok := f.resolve(w[i+1]).([]any); ok {
				for j, v := range list {
					if x, ok := f.number(v); ok {
						ft.widths[int(c)+j] = x
					}
				}
				i += 2
				continue
			}
			last, ok1 := f.number(w[i+1])
			if i+2 >= len(w) || !ok1 {
				break
			}
			x, _ := f.number(w[i+2])
			for k := int(c); k <= int(last) && k-int(c) < 65536; k++ {
				ft.widths[k] = x
			}
			i += 3
		}
		switch enc := f.resolve(d["Encoding"]).(type) {
		case name:
			e := string(enc)
			ft.utf16 = strings.Contains(e, "UCS2") || strings.Contains(e, "UTF16")
			if strings.HasPrefix(e, "Identity") || ft.utf16 {
				// Two bytes a code, whatever the map to text says of it.
				ft.codeLengths = []int{2}
			}
		case *stream:
			// The font's own map from codes to CIDs: its code lengths.
			if data, err := decodeStream(f, enc); err == nil {
				if _, lengths := parseCMap(data); len(lengths) > 0 {
					ft.codeLengths = lengths
				}
			}
		}
		if len(ft.codeLengths) == 0 {
			ft.codeLengths = []int{2}
		}
	} else {
		ft.simpleEncoding(f, d, desc, base)
		first, _ := f.number(d["FirstChar"])
		scale := 1.0
		if f.name(d["Subtype"]) == "Type3" {
			// Widths in glyph space: the font matrix says its size.
			if m := f.array(d["FontMatrix"]); len(m) == 6 {
				if a, ok := f.number(m[0]); ok && a != 0 {
					scale = a * 1000
				}
			}
		}
		ws := f.array(d["Widths"])
		for i, v := range ws {
			if x, ok := f.number(v); ok {
				ft.widths[int(first)+i] = x * scale
			}
		}
		if mw, ok := f.number(desc["MissingWidth"]); ok && mw > 0 {
			ft.defaultWidth = mw
		}
		if len(ws) == 0 {
			// One of the fonts every reader has, named without its widths.
			table := &helveticaWidths
			switch {
			case strings.Contains(low, "courier"):
				table = nil
				ft.defaultWidth = 600
			case strings.Contains(low, "times"):
				table = &timesWidths
			}
			if table != nil {
				for c := 32; c < 127; c++ {
					ft.widths[c] = float64(table[c-32])
				}
				ft.defaultWidth = 556
			}
		}
	}
	if flags, ok := f.number(desc["Flags"]); ok {
		ft.bold = ft.bold || int(flags)&(1<<18) != 0
		ft.italic = ft.italic || int(flags)&(1<<6) != 0
	}
	if w, ok := f.number(desc["FontWeight"]); ok && w >= 600 {
		ft.bold = true
	}
	return ft
}

// simpleEncoding fills in the characters of a simple font's codes.
func (ft *font) simpleEncoding(f *file, d, desc dict, base string) {
	flags, _ := f.number(desc["Flags"])
	symbolic := int(flags)&4 != 0
	baseEnc := name("")
	var diffs []any
	switch enc := f.resolve(d["Encoding"]).(type) {
	case name:
		baseEnc = enc
	case dict:
		baseEnc = f.name(enc["BaseEncoding"])
		diffs = f.array(enc["Differences"])
	}
	if baseEnc == "" {
		switch {
		case base == "Symbol":
			baseEnc = "Symbol"
		case base == "ZapfDingbats":
			baseEnc = "None"
		case f.name(d["Subtype"]) == "TrueType" || (!symbolic && desc != nil):
			baseEnc = "WinAnsiEncoding"
		default:
			baseEnc = "StandardEncoding"
		}
	}
	for c := 0; c < 256; c++ {
		ft.enc[c] = baseChar(baseEnc, byte(c))
	}
	code := 0
	for _, v := range diffs {
		switch x := f.resolve(v).(type) {
		case float64:
			code = int(x)
		case name:
			if code >= 0 && code < 256 {
				ft.enc[code] = glyphText(string(x))
			}
			code++
		}
	}
}

// baseChar is the character of a code in one of the named encodings.
func baseChar(enc name, c byte) string {
	switch enc {
	case "None":
		return ""
	case "Symbol":
		if r, ok := symbolEncoding[c]; ok {
			return string(r)
		}
		if c >= 0x20 && c < 0x7F && !(c >= 'A' && c <= 'Z') && !(c >= 'a' && c <= 'z') {
			return string(rune(c))
		}
		return ""
	case "MacRomanEncoding":
		if c < 0x20 {
			return ""
		}
		return string(charmap.Macintosh.DecodeByte(c))
	case "StandardEncoding", "MacExpertEncoding":
		if r, ok := standardEncoding[c]; ok {
			return string(r)
		}
		if c >= 0x20 && c < 0x7F {
			return string(rune(c))
		}
		return ""
	}
	// WinAnsiEncoding.
	switch {
	case c < 0x20 || c == 0x7F:
		return ""
	case c == 0xAD:
		return "-"
	}
	r := charmap.Windows1252.DecodeByte(c)
	if r == utf8.RuneError || (r >= 0x80 && r < 0xA0) {
		if c >= 0x80 && c < 0xA0 {
			return "•" // the codes the encoding leaves open show a bullet
		}
		return ""
	}
	return string(r)
}

// standardEncoding is where Adobe's standard encoding differs from ASCII.
var standardEncoding = map[byte]rune{
	0x27: '’', 0x60: '‘',
	0xA1: '¡', 0xA2: '¢', 0xA3: '£', 0xA4: '⁄', 0xA5: '¥', 0xA6: 'ƒ', 0xA7: '§', 0xA8: '¤', 0xA9: '\'', 0xAA: '“', 0xAB: '«', 0xAC: '‹', 0xAD: '›', 0xAE: 'ﬁ', 0xAF: 'ﬂ',
	0xB1: '–', 0xB2: '†', 0xB3: '‡', 0xB4: '·', 0xB6: '¶', 0xB7: '•', 0xB8: '‚', 0xB9: '„', 0xBA: '”', 0xBB: '»', 0xBC: '…', 0xBD: '‰', 0xBF: '¿',
	0xC1: '`', 0xC2: '´', 0xC3: 'ˆ', 0xC4: '˜', 0xC5: '¯', 0xC6: '˘', 0xC7: '˙', 0xC8: '¨', 0xCA: '˚', 0xCB: '¸', 0xCD: '˝', 0xCE: '˛', 0xCF: 'ˇ',
	0xD0: '—', 0xE1: 'Æ', 0xE3: 'ª', 0xE8: 'Ł', 0xE9: 'Ø', 0xEA: 'Œ', 0xEB: 'º', 0xF1: 'æ', 0xF5: 'ı', 0xF8: 'ł', 0xF9: 'ø', 0xFA: 'œ', 0xFB: 'ß',
}

// symbolEncoding is the Greek of the Symbol font, and its commonest signs.
var symbolEncoding = map[byte]rune{
	'A': 'Α', 'B': 'Β', 'C': 'Χ', 'D': 'Δ', 'E': 'Ε', 'F': 'Φ', 'G': 'Γ', 'H': 'Η', 'I': 'Ι', 'J': 'ϑ', 'K': 'Κ', 'L': 'Λ', 'M': 'Μ',
	'N': 'Ν', 'O': 'Ο', 'P': 'Π', 'Q': 'Θ', 'R': 'Ρ', 'S': 'Σ', 'T': 'Τ', 'U': 'Υ', 'V': 'ς', 'W': 'Ω', 'X': 'Ξ', 'Y': 'Ψ', 'Z': 'Ζ',
	'a': 'α', 'b': 'β', 'c': 'χ', 'd': 'δ', 'e': 'ε', 'f': 'φ', 'g': 'γ', 'h': 'η', 'i': 'ι', 'j': 'ϕ', 'k': 'κ', 'l': 'λ', 'm': 'μ',
	'n': 'ν', 'o': 'ο', 'p': 'π', 'q': 'θ', 'r': 'ρ', 's': 'σ', 't': 'τ', 'u': 'υ', 'v': 'ϖ', 'w': 'ω', 'x': 'ξ', 'y': 'ψ', 'z': 'ζ',
	0xA3: '≤', 0xA5: '∞', 0xB0: '°', 0xB1: '±', 0xB3: '≥', 0xB4: '×', 0xB5: '∝', 0xB7: '•', 0xB8: '÷', 0xB9: '≠', 0xBA: '≡', 0xBB: '≈', 0xBC: '…',
	0xAB: '↔', 0xAC: '←', 0xAD: '↑', 0xAE: '→', 0xAF: '↓', 0xD7: '⋅', 0xE5: '∑', 0xF2: '∫', 0xD6: '√', 0xB6: '∂', 0xD1: '∇', 0xCE: '∈',
}

// glyphNames are the names of glyphs that are not a letter with an accent
// named after it.
var glyphNames = map[string]rune{
	"space": ' ', "exclam": '!', "quotedbl": '"', "numbersign": '#', "dollar": '$', "percent": '%', "ampersand": '&', "quotesingle": '\'',
	"parenleft": '(', "parenright": ')', "asterisk": '*', "plus": '+', "comma": ',', "hyphen": '-', "minus": '−', "period": '.', "slash": '/',
	"zero": '0', "one": '1', "two": '2', "three": '3', "four": '4', "five": '5', "six": '6', "seven": '7', "eight": '8', "nine": '9',
	"colon": ':', "semicolon": ';', "less": '<', "equal": '=', "greater": '>', "question": '?', "at": '@', "bracketleft": '[', "backslash": '\\',
	"bracketright": ']', "asciicircum": '^', "underscore": '_', "grave": '`', "braceleft": '{', "bar": '|', "braceright": '}', "asciitilde": '~',
	"exclamdown": '¡', "cent": '¢', "sterling": '£', "currency": '¤', "yen": '¥', "brokenbar": '¦', "section": '§', "dieresis": '¨', "copyright": '©',
	"ordfeminine": 'ª', "guillemotleft": '«', "logicalnot": '¬', "registered": '®', "macron": '¯', "degree": '°', "plusminus": '±',
	"twosuperior": '²', "threesuperior": '³', "acute": '´', "mu": 'µ', "paragraph": '¶', "periodcentered": '·', "cedilla": '¸', "onesuperior": '¹',
	"ordmasculine": 'º', "guillemotright": '»', "onequarter": '¼', "onehalf": '½', "threequarters": '¾', "questiondown": '¿',
	"AE": 'Æ', "Eth": 'Ð', "multiply": '×', "Oslash": 'Ø', "Thorn": 'Þ', "germandbls": 'ß', "ae": 'æ', "eth": 'ð', "divide": '÷', "oslash": 'ø', "thorn": 'þ',
	"dotlessi": 'ı', "Lslash": 'Ł', "lslash": 'ł', "OE": 'Œ', "oe": 'œ', "florin": 'ƒ', "Dcroat": 'Đ', "dcroat": 'đ', "Eng": 'Ŋ', "eng": 'ŋ',
	"circumflex": 'ˆ', "caron": 'ˇ', "breve": '˘', "dotaccent": '˙', "ring": '˚', "ogonek": '˛', "tilde": '˜', "hungarumlaut": '˝',
	"endash": '–', "emdash": '—', "quoteleft": '‘', "quoteright": '’', "quotesinglbase": '‚', "quotedblleft": '“', "quotedblright": '”', "quotedblbase": '„',
	"dagger": '†', "daggerdbl": '‡', "bullet": '•', "ellipsis": '…', "perthousand": '‰', "guilsinglleft": '‹', "guilsinglright": '›', "fraction": '⁄',
	"Euro": '€', "trademark": '™', "fi": 'ﬁ', "fl": 'ﬂ', "ff": 'ﬀ', "ffi": 'ﬃ', "ffl": 'ﬄ', "nbspace": ' ', "nonbreakingspace": ' ',
	"sfthyphen": '­', "softhyphen": '­', "partialdiff": '∂', "Delta": 'Δ', "summation": '∑', "product": '∏', "pi": 'π', "integral": '∫',
	"Omega": 'Ω', "radical": '√', "approxequal": '≈', "notequal": '≠', "lessequal": '≤', "greaterequal": '≥', "infinity": '∞', "lozenge": '◊',
	"arrowleft": '←', "arrowright": '→', "arrowup": '↑', "arrowdown": '↓',
	"Alpha": 'Α', "Beta": 'Β', "Gamma": 'Γ', "Epsilon": 'Ε', "Zeta": 'Ζ', "Eta": 'Η', "Theta": 'Θ', "Iota": 'Ι', "Kappa": 'Κ', "Lambda": 'Λ', "Mu": 'Μ',
	"Nu": 'Ν', "Xi": 'Ξ', "Omicron": 'Ο', "Pi": 'Π', "Rho": 'Ρ', "Sigma": 'Σ', "Tau": 'Τ', "Upsilon": 'Υ', "Phi": 'Φ', "Chi": 'Χ', "Psi": 'Ψ',
	"alpha": 'α', "beta": 'β', "gamma": 'γ', "delta": 'δ', "epsilon": 'ε', "zeta": 'ζ', "eta": 'η', "theta": 'θ', "iota": 'ι', "kappa": 'κ', "lambda": 'λ',
	"nu": 'ν', "xi": 'ξ', "omicron": 'ο', "rho": 'ρ', "sigma": 'σ', "tau": 'τ', "upsilon": 'υ', "phi": 'φ', "chi": 'χ', "psi": 'ψ', "omega": 'ω',
}

// accents are the names of marks after a letter's name, as combining marks.
var accents = []struct {
	name string
	mark rune
}{
	{"hungarumlaut", 0x30B}, {"circumflex", 0x302}, {"commaaccent", 0x327}, {"dotaccent", 0x307}, {"dieresis", 0x308}, {"cedilla", 0x327},
	{"ogonek", 0x328}, {"macron", 0x304}, {"grave", 0x300}, {"acute", 0x301}, {"tilde", 0x303}, {"breve", 0x306}, {"caron", 0x30C}, {"ring", 0x30A},
}

// glyphText is the text of a glyph by its name, or nothing for a name
// that does not say.
func glyphText(n string) string {
	if r, ok := glyphNames[n]; ok {
		return string(r)
	}
	if i := strings.IndexByte(n, '.'); i > 0 {
		return glyphText(n[:i]) // "a.sc", "one.oldstyle"
	}
	if strings.Contains(n, "_") {
		var b strings.Builder
		for _, part := range strings.Split(n, "_") {
			t := glyphText(part)
			if t == "" {
				return ""
			}
			b.WriteString(t)
		}
		return b.String()
	}
	if utf8.RuneCountInString(n) == 1 {
		return n
	}
	if strings.HasPrefix(n, "uni") && len(n) >= 7 && (len(n)-3)%4 == 0 {
		var units []uint16
		for i := 3; i < len(n); i += 4 {
			v, err := strconv.ParseUint(n[i:i+4], 16, 16)
			if err != nil {
				return ""
			}
			units = append(units, uint16(v))
		}
		return string(utf16.Decode(units))
	}
	if strings.HasPrefix(n, "u") && len(n) >= 5 && len(n) <= 7 {
		if v, err := strconv.ParseUint(n[1:], 16, 32); err == nil && v <= 0x10FFFF {
			return string(rune(v))
		}
	}
	for _, a := range accents {
		if strings.HasSuffix(n, a.name) && len(n) == len(a.name)+1 {
			letter := n[:1]
			mark := a.mark
			if a.name == "commaaccent" && strings.ContainsAny(letter, "SsTt") {
				mark = 0x326
			}
			if composed := norm.NFC.String(letter + string(mark)); utf8.RuneCountInString(composed) == 1 {
				return composed
			}
		}
	}
	return ""
}

// parseCMap reads a ToUnicode map: the text of each code, and the lengths
// codes have.
func parseCMap(data []byte) (map[string]string, []int) {
	m := map[string]string{}
	lengths := map[int]bool{}
	l := &lexer{b: data}
	var stack []any
	text := func(v any) (string, bool) {
		switch x := v.(type) {
		case pstring:
			return utf16Text(string(x)), true
		case name:
			return glyphText(string(x)), true
		}
		return "", false
	}
	for {
		v, err := l.next()
		if err != nil {
			break
		}
		o, isOp := v.(op)
		if !isOp {
			if len(stack) < 1<<16 {
				stack = append(stack, v)
			}
			continue
		}
		if o == "" {
			break
		}
		switch o {
		case "endcodespacerange":
			for i := 0; i+1 < len(stack); i += 2 {
				if lo, ok := stack[i].(pstring); ok && len(lo) > 0 && len(lo) <= 4 {
					lengths[len(lo)] = true
				}
			}
		case "endbfchar":
			for i := 0; i+1 < len(stack); i += 2 {
				src, ok := stack[i].(pstring)
				if !ok {
					continue
				}
				if t, ok := text(stack[i+1]); ok {
					m[string(src)] = t
				}
			}
		case "endbfrange":
			for i := 0; i+2 < len(stack); i += 3 {
				lo, ok1 := stack[i].(pstring)
				hi, ok2 := stack[i+1].(pstring)
				if !ok1 || !ok2 || len(lo) != len(hi) || len(lo) == 0 || len(lo) > 4 {
					continue
				}
				a, b := codeValue(string(lo)), codeValue(string(hi))
				if b < a || b-a > 0xFFFF {
					continue
				}
				if list, ok := stack[i+2].([]any); ok {
					for j := 0; j < len(list) && a+j <= b; j++ {
						if t, ok := text(list[j]); ok {
							m[codeBytes(a+j, len(lo))] = t
						}
					}
					continue
				}
				dst, ok := stack[i+2].(pstring)
				if !ok || len(dst) < 2 {
					continue
				}
				units := make([]uint16, len(dst)/2)
				for j := range units {
					units[j] = uint16(dst[2*j])<<8 | uint16(dst[2*j+1])
				}
				for c := a; c <= b; c++ {
					m[codeBytes(c, len(lo))] = string(utf16.Decode(units))
					units[len(units)-1]++
				}
			}
		}
		stack = stack[:0]
	}
	var out []int
	for n := 1; n <= 4; n++ {
		if lengths[n] {
			out = append(out, n)
		}
	}
	if len(m) == 0 {
		m = nil
	}
	return m, out
}

func codeValue(s string) int {
	v := 0
	for i := 0; i < len(s); i++ {
		v = v<<8 | int(s[i])
	}
	return v
}

func codeBytes(v, n int) string {
	b := make([]byte, n)
	for i := n - 1; i >= 0; i-- {
		b[i] = byte(v)
		v >>= 8
	}
	return string(b)
}

// utf16Text reads big-endian UTF-16.
func utf16Text(s string) string {
	units := make([]uint16, len(s)/2)
	for i := range units {
		units[i] = uint16(s[2*i])<<8 | uint16(s[2*i+1])
	}
	return string(utf16.Decode(units))
}

// textString reads a string of a dictionary: UTF-16 with a byte order
// mark, UTF-8 with one, or the document encoding, which is Latin-1 here.
func textString(s pstring) string {
	switch {
	case len(s) >= 2 && s[0] == 0xFE && s[1] == 0xFF:
		return utf16Text(string(s[2:]))
	case len(s) >= 3 && s[0] == 0xEF && s[1] == 0xBB && s[2] == 0xBF:
		return string(s[3:])
	}
	r := make([]rune, len(s))
	for i := 0; i < len(s); i++ {
		r[i] = rune(s[i])
	}
	return string(r)
}

// The widths of Helvetica and Times Roman from the space to the tilde,
// for files that name these fonts without their widths.
var helveticaWidths = [95]int16{
	278, 278, 355, 556, 556, 889, 667, 191, 333, 333, 389, 584, 278, 333, 278, 278,
	556, 556, 556, 556, 556, 556, 556, 556, 556, 556, 278, 278, 584, 584, 584, 556,
	1015, 667, 667, 722, 722, 667, 611, 778, 722, 278, 500, 667, 556, 833, 722, 778,
	667, 778, 722, 667, 611, 722, 667, 944, 667, 667, 611, 278, 278, 278, 469, 556,
	333, 556, 556, 500, 556, 556, 278, 556, 556, 222, 222, 500, 222, 833, 556, 556,
	556, 556, 333, 500, 278, 556, 500, 722, 500, 500, 500, 334, 260, 334, 584,
}

var timesWidths = [95]int16{
	250, 333, 408, 500, 500, 833, 778, 180, 333, 333, 500, 564, 250, 333, 250, 278,
	500, 500, 500, 500, 500, 500, 500, 500, 500, 500, 278, 278, 564, 564, 564, 444,
	921, 722, 667, 667, 722, 611, 556, 722, 722, 333, 389, 722, 611, 889, 722, 722,
	556, 722, 667, 556, 611, 722, 722, 944, 722, 722, 611, 333, 278, 333, 469, 500,
	333, 444, 500, 444, 500, 444, 333, 500, 500, 278, 278, 500, 278, 778, 500, 500,
	500, 500, 333, 389, 278, 500, 500, 722, 500, 500, 444, 480, 200, 480, 541,
}
