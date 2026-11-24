package textfmt

import (
	"fmt"
	"strconv"
	"strings"
)

// parseProperties reads a Java properties file: key=value, key:value or
// key value, a value going on over lines that end in a backslash, and
// comments from # or ! to the end of their line. The comments right over
// a key are its notes.
func parseProperties(src string, wasLatin1 bool) ([]*slot, error) {
	ls := linesOf(src)
	// A file written in escapes stays in them; one that has its letters
	// as they are keeps them.
	escapes := wasLatin1 || strings.Contains(src, `\u`) || isASCII(src)
	var out []*slot
	var notes []string
	for i := 0; i < len(ls); i++ {
		text := strings.TrimLeft(ls[i].text, " \t\f")
		lead := len(ls[i].text) - len(text)
		if text == "" {
			notes = nil
			continue
		}
		if text[0] == '#' || text[0] == '!' {
			notes = append(notes, strings.TrimSpace(text[1:]))
			continue
		}
		// The key ends at the first =, : or blank that no backslash is before.
		k := 0
		for k < len(text) && !strings.ContainsRune("=: \t\f", rune(text[k])) {
			if text[k] == '\\' {
				k++
			}
			k++
		}
		if k > len(text) {
			k = len(text)
		}
		key := unescapeProperties(text[:k])
		v := k
		for v < len(text) && strings.ContainsRune(" \t\f", rune(text[v])) {
			v++
		}
		if v < len(text) && (text[v] == '=' || text[v] == ':') {
			v++
			for v < len(text) && strings.ContainsRune(" \t\f", rune(text[v])) {
				v++
			}
		}
		start := ls[i].start + lead + v
		// The value goes on while its line ends in a backslash that is not escaped.
		var raw strings.Builder
		part := text[v:]
		for {
			if !continues(part) {
				raw.WriteString(part)
				break
			}
			raw.WriteString(part[:len(part)-1])
			if i+1 >= len(ls) {
				break
			}
			i++
			part = strings.TrimLeft(ls[i].text, " \t\f")
		}
		end := ls[i].end
		keyNotes := notes
		notes = nil
		out = append(out, &slot{
			start: start, end: end, id: "k" + strconv.Itoa(len(out)+1) + ":" + key, text: unescapeProperties(raw.String()), key: key, notes: keyNotes,
			write: func(t string) string { return escapeProperties(t, escapes) },
		})
	}
	return out, nil
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// continues reports whether a line ends in a backslash that is not
// itself escaped.
func continues(s string) bool {
	n := 0
	for i := len(s) - 1; i >= 0 && s[i] == '\\'; i-- {
		n++
	}
	return n%2 == 1
}

func unescapeProperties(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '\\' || i+1 >= len(s) {
			b.WriteByte(c)
			continue
		}
		i++
		switch s[i] {
		case 't':
			b.WriteByte('\t')
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 'f':
			b.WriteByte('\f')
		case 'u':
			if i+4 < len(s) {
				if v, err := strconv.ParseUint(s[i+1:i+5], 16, 16); err == nil {
					r := rune(v)
					// Two escapes make one letter beyond the first plane.
					if r >= 0xD800 && r < 0xDC00 && i+10 < len(s) && s[i+5] == '\\' && s[i+6] == 'u' {
						if lo, err := strconv.ParseUint(s[i+7:i+11], 16, 16); err == nil && lo >= 0xDC00 && lo < 0xE000 {
							r = 0x10000 + (r-0xD800)<<10 + (rune(lo) - 0xDC00)
							i += 6
						}
					}
					b.WriteRune(r)
					i += 4
					continue
				}
			}
			b.WriteByte('u')
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

func escapeProperties(s string, escapes bool) string {
	var b strings.Builder
	for i, r := range s {
		switch {
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r == '\f':
			b.WriteString(`\f`)
		case r == ' ' && i == 0:
			// A blank at the start would be read as the gap after the key.
			b.WriteString(`\ `)
		case r > 0x7E && escapes:
			if r > 0xFFFF {
				r -= 0x10000
				fmt.Fprintf(&b, `\u%04X\u%04X`, 0xD800+(r>>10), 0xDC00+(r&0x3FF))
			} else {
				fmt.Fprintf(&b, `\u%04X`, r)
			}
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
