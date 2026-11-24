package textfmt

import (
	"regexp"
	"strconv"
	"strings"
)

// parsePO reads a gettext PO file. The msgid of an entry is what is
// translated and its msgstr where the translation goes; a plural's
// msgid_plural goes into every form but the first. The comments the
// programmers left (#.) are the notes and msgctxt the key. An entry
// marked fuzzy loses the mark when it gets its translation, or gettext
// would not use it. The header and what is obsolete (#~) stay.
func parsePO(src string) ([]*slot, error) {
	ls := linesOf(src)
	nl := newline(src)
	var out []*slot
	type field struct {
		name       string
		text       string
		start, end int // of the quoted strings, over its lines
	}
	i := 0
	for i < len(ls) {
		// An entry: comments, then fields, up to a blank line or the next comment after fields.
		var notes []string
		flagLine := -1
		var fields []field
		for i < len(ls) {
			t := strings.TrimSpace(ls[i].text)
			if t == "" {
				i++
				if len(fields) > 0 || len(notes) > 0 || flagLine >= 0 {
					break
				}
				continue
			}
			if strings.HasPrefix(t, "#") {
				if len(fields) > 0 {
					break
				}
				switch {
				case strings.HasPrefix(t, "#."):
					notes = append(notes, strings.TrimSpace(t[2:]))
				case strings.HasPrefix(t, "#,"):
					flagLine = i
				}
				i++
				continue
			}
			if t[0] == '"' {
				// A string that goes on from the line before.
				if n := len(fields); n > 0 {
					fields[n-1].text += unquotePO(t)
					fields[n-1].end = ls[i].end
				}
				i++
				continue
			}
			sp := strings.IndexAny(t, " \t")
			if sp < 0 {
				i++
				continue
			}
			q := strings.IndexByte(ls[i].text, '"')
			if q < 0 {
				i++
				continue
			}
			fields = append(fields, field{name: t[:sp], text: unquotePO(strings.TrimSpace(t[sp:])), start: ls[i].start + q, end: ls[i].end})
			i++
		}
		get := func(name string) *field {
			for k := range fields {
				if fields[k].name == name {
					return &fields[k]
				}
			}
			return nil
		}
		id := get("msgid")
		if id == nil || id.text == "" {
			continue
		}
		key := ""
		if c := get("msgctxt"); c != nil {
			key = c.text
		}
		write := func(t string) string { return quotePO(t, nl) }
		// The mark that an entry is only a guess goes with its translation.
		var unfuzzy *slot
		if flagLine >= 0 && fuzzy.MatchString(ls[flagLine].text) {
			l := ls[flagLine]
			end := l.end
			rest := strings.TrimSpace(fuzzy.ReplaceAllString(strings.TrimPrefix(strings.TrimSpace(l.text), "#,"), ""))
			rest = strings.Trim(rest, ", ")
			unfuzzy = &slot{start: l.start, end: end, write: func(string) string {
				if rest == "" {
					return "#"
				}
				return "#, " + rest
			}}
		}
		n := strconv.Itoa(len(out) + 1)
		if pl := get("msgid_plural"); pl != nil {
			one := get("msgstr[0]")
			if one == nil {
				continue
			}
			s := &slot{start: one.start, end: one.end, id: "p" + n, text: id.text, key: key, notes: notes, write: write}
			if unfuzzy != nil {
				s.also = append(s.also, unfuzzy)
			}
			out = append(out, s)
			var others []*slot
			for k := range fields {
				if strings.HasPrefix(fields[k].name, "msgstr[") && fields[k].name != "msgstr[0]" {
					others = append(others, &slot{start: fields[k].start, end: fields[k].end, write: write})
				}
			}
			if len(others) > 0 {
				p := &slot{start: others[0].start, end: others[0].end, id: "p" + n + "+", text: pl.text, key: key, notes: append(append([]string{}, notes...), "Plural form"), write: write, also: others[1:]}
				out = append(out, p)
			}
			continue
		}
		str := get("msgstr")
		if str == nil {
			continue
		}
		s := &slot{start: str.start, end: str.end, id: "p" + n, text: id.text, key: key, notes: notes, write: write}
		if unfuzzy != nil {
			s.also = append(s.also, unfuzzy)
		}
		out = append(out, s)
	}
	return out, nil
}

var fuzzy = regexp.MustCompile(`\bfuzzy\b`)

func unquotePO(s string) string {
	s = strings.TrimSpace(s)
	if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' {
		return ""
	}
	s = s[1 : len(s)-1]
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 >= len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case 'r':
			b.WriteByte('\r')
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// quotePO writes a text as a PO string: on one line, or, with line
// breaks in it, an empty string and then a line for each of its lines.
func quotePO(s, nl string) string {
	esc := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\t", `\t`, "\r", `\r`)
	if !strings.Contains(strings.TrimSuffix(s, "\n"), "\n") {
		return `"` + strings.ReplaceAll(esc.Replace(s), "\n", `\n`) + `"`
	}
	var b strings.Builder
	b.WriteString(`""`)
	for _, part := range strings.SplitAfter(s, "\n") {
		if part == "" {
			continue
		}
		b.WriteString(nl + `"` + strings.ReplaceAll(esc.Replace(part), "\n", `\n`) + `"`)
	}
	return b.String()
}

var poLanguage = regexp.MustCompile(`"Language: [^"\\]*\\n"`)

// setPOLanguage names the language of the translations in the header.
func setPOLanguage(src, lang string) string {
	return poLanguage.ReplaceAllLiteralString(src, `"Language: `+strings.ReplaceAll(lang, "-", "_")+`\n"`)
}
