package textfmt

import (
	"regexp"
	"strconv"
	"strings"
)

// parseYAML reads the YAML that programs keep their texts in: mappings
// and lists of strings, one per line or as a block (| or >). A string is
// named by the keys that lead to it. What is not text (numbers, true and
// false, dates, references, lists and mappings written in one line) and
// strings that go on over lines without being a block stay as they are.
func parseYAML(src string) ([]*slot, error) {
	ls := linesOf(src)
	nl := newline(src)
	type level struct {
		indent int
		key    string
	}
	var stack []level
	path := func() string {
		var b strings.Builder
		for _, l := range stack {
			if b.Len() > 0 && !strings.HasPrefix(l.key, "[") {
				b.WriteByte('.')
			}
			b.WriteString(l.key)
		}
		return b.String()
	}
	pop := func(indent int) {
		for len(stack) > 0 && stack[len(stack)-1].indent >= indent {
			stack = stack[:len(stack)-1]
		}
	}
	items := map[string]int{}
	var out []*slot
	var notes []string
	for i := 0; i < len(ls); i++ {
		raw := ls[i].text
		body := strings.TrimLeft(raw, " ")
		indent := len(raw) - len(body)
		if body == "" || body == "---" || body == "..." || body[0] == '%' {
			notes = nil
			continue
		}
		if body[0] == '#' {
			notes = append(notes, strings.TrimSpace(body[1:]))
			continue
		}
		at := indent
		// A list item: "- " and then a key, a value, or nothing.
		for body == "-" || strings.HasPrefix(body, "- ") {
			pop(at)
			k := path() + "@" + strconv.Itoa(at)
			stack = append(stack, level{at, "[" + strconv.Itoa(items[k]) + "]"})
			items[k]++
			rest := strings.TrimLeft(body[1:], " ")
			at += len(body) - len(rest)
			body = rest
		}
		if body == "" {
			notes = nil
			continue
		}
		value, vAt := body, 0
		if key, v, off, ok := splitYAMLKey(body); ok {
			pop(at)
			stack = append(stack, level{at, key})
			value, vAt = v, off
		}
		keyNotes := notes
		notes = nil
		if value == "" {
			continue
		}
		name := path()
		// A block: its lines are those indented deeper than its key.
		if m := blockHead.FindStringSubmatch(value); m != nil {
			first, last, blockIndent := i+1, i, -1
			for j := first; j < len(ls); j++ {
				t := strings.TrimLeft(ls[j].text, " ")
				if t == "" {
					continue
				}
				ind := len(ls[j].text) - len(t)
				if ind <= indent {
					break
				}
				if blockIndent < 0 {
					blockIndent = ind
				}
				last = j
			}
			if blockIndent < 0 {
				continue
			}
			var lines []string
			for j := first; j <= last; j++ {
				if len(ls[j].text) >= blockIndent {
					lines = append(lines, ls[j].text[blockIndent:])
				} else {
					lines = append(lines, "")
				}
			}
			text := strings.Join(lines, "\n")
			if m[1] == ">" {
				text = foldYAML(lines)
			}
			pad := strings.Repeat(" ", blockIndent)
			out = append(out, &slot{start: ls[first].start, end: ls[last].end, id: "y" + strconv.Itoa(len(out)+1), text: text, key: name, notes: keyNotes,
				write: func(t string) string {
					parts := strings.Split(t, "\n")
					for k := range parts {
						if parts[k] != "" {
							parts[k] = pad + parts[k]
						}
					}
					return strings.Join(parts, nl)
				}})
			i = last
			continue
		}
		scalar, quote, length, ok := readYAMLScalar(value)
		if !ok || !isYAMLText(scalar, quote) {
			continue
		}
		start := ls[i].start + at + vAt
		out = append(out, &slot{start: start, end: start + length, id: "y" + strconv.Itoa(len(out)+1), text: scalar, key: name, notes: keyNotes,
			write: func(t string) string { return writeYAMLScalar(t, quote) }})
	}
	return out, nil
}

var blockHead = regexp.MustCompile(`^([|>])[+-]?[0-9]?[+-]?\s*(#.*)?$`)

// splitYAMLKey cuts "key: value" into its key and value, and says where
// in the line the value starts. A line that is no key is not ok.
func splitYAMLKey(s string) (key, value string, at int, ok bool) {
	end := -1
	switch s[0] {
	case '"', '\'':
		q := s[0]
		for i := 1; i < len(s); i++ {
			if s[i] == '\\' && q == '"' {
				i++
			} else if s[i] == q {
				if q == '\'' && i+1 < len(s) && s[i+1] == '\'' {
					i++
					continue
				}
				end = i + 1
				break
			}
		}
		if end < 0 || end >= len(s) || s[end] != ':' {
			return "", "", 0, false
		}
		key = strings.Trim(s[:end], `"'`)
	case '[', '{', '&', '*', '!', '|', '>', '@', '`':
		return "", "", 0, false
	default:
		for i := 0; i < len(s); i++ {
			if s[i] == ':' && (i+1 == len(s) || s[i+1] == ' ') {
				end = i
				break
			}
			if s[i] == '#' && i > 0 && s[i-1] == ' ' {
				break
			}
		}
		if end <= 0 {
			return "", "", 0, false
		}
		key = strings.TrimSpace(s[:end])
	}
	rest := s[end+1:]
	value = strings.TrimLeft(rest, " ")
	return key, strings.TrimRight(value, " "), end + 1 + len(rest) - len(value), true
}

// readYAMLScalar reads the string a value starts with: quoted, or plain
// up to a comment. It gives the string, its quote (0 for plain) and how
// many bytes of the line it takes; a string that does not end on its
// line, or is followed by more than a comment, is not ok.
func readYAMLScalar(v string) (text string, quote byte, length int, ok bool) {
	switch v[0] {
	case '"':
		var b strings.Builder
		for i := 1; i < len(v); i++ {
			switch c := v[i]; c {
			case '\\':
				i++
				if i >= len(v) {
					return "", 0, 0, false
				}
				switch v[i] {
				case 'n':
					b.WriteByte('\n')
				case 't':
					b.WriteByte('\t')
				case 'r':
					b.WriteByte('\r')
				case 'u', 'x', 'U':
					n := map[byte]int{'x': 2, 'u': 4, 'U': 8}[v[i]]
					if i+n >= len(v) {
						return "", 0, 0, false
					}
					r, err := strconv.ParseUint(v[i+1:i+1+n], 16, 32)
					if err != nil {
						return "", 0, 0, false
					}
					b.WriteRune(rune(r))
					i += n
				default:
					b.WriteByte(v[i])
				}
			case '"':
				if rest := strings.TrimSpace(v[i+1:]); rest != "" && rest[0] != '#' {
					return "", 0, 0, false
				}
				return b.String(), '"', i + 1, true
			default:
				b.WriteByte(c)
			}
		}
		return "", 0, 0, false
	case '\'':
		var b strings.Builder
		for i := 1; i < len(v); i++ {
			if v[i] != '\'' {
				b.WriteByte(v[i])
				continue
			}
			if i+1 < len(v) && v[i+1] == '\'' {
				b.WriteByte('\'')
				i++
				continue
			}
			if rest := strings.TrimSpace(v[i+1:]); rest != "" && rest[0] != '#' {
				return "", 0, 0, false
			}
			return b.String(), '\'', i + 1, true
		}
		return "", 0, 0, false
	case '[', '{', '&', '*', '!', '|', '>', '@', '`', '%':
		return "", 0, 0, false
	}
	end := len(v)
	if c := strings.Index(v, " #"); c >= 0 {
		end = c
	}
	text = strings.TrimRight(v[:end], " ")
	return text, 0, len(text), text != ""
}

var notYAMLText = regexp.MustCompile(`^(?i:true|false|yes|no|on|off|null|~|[-+]?(\d[\d_]*)?(\.\d+)?([eE][-+]?\d+)?|0x[0-9a-f]+|0o[0-7]+|[-+]?\.(inf|nan)|\d{4}-\d{2}-\d{2}([Tt ].*)?)$`)

// isYAMLText reports whether a value is text: anything in quotes, and
// what is plain but neither a number, a truth value, nothing, nor a date.
func isYAMLText(s string, quote byte) bool {
	if strings.TrimSpace(s) == "" {
		return false
	}
	return quote != 0 || !notYAMLText.MatchString(s)
}

// foldYAML joins the lines of a folded block as YAML reads them: lines
// that follow each other with a blank, an empty line as a line break.
func foldYAML(lines []string) string {
	var b strings.Builder
	for i, l := range lines {
		switch {
		case i == 0:
		case l == "" || lines[i-1] == "":
			if l == "" {
				b.WriteByte('\n')
			}
		default:
			b.WriteByte(' ')
		}
		b.WriteString(l)
	}
	return b.String()
}

// writeYAMLScalar writes a string the way it was written, where that can
// hold it: plain if nothing in it would be read as something else, in
// single quotes if it has no line break, else in double quotes.
func writeYAMLScalar(s string, quote byte) string {
	plainSafe := s != "" && s == strings.TrimSpace(s) && !strings.ContainsAny(s, "\n\t\"") && !strings.Contains(s, ": ") && !strings.Contains(s, " #") &&
		!strings.HasSuffix(s, ":") && !strings.ContainsRune("-?:,[]{}#&*!|>'%@`", rune(s[0])) && !notYAMLText.MatchString(s)
	switch {
	case quote == 0 && plainSafe:
		return s
	case quote == '\'' && !strings.ContainsAny(s, "\n\t"):
		return "'" + strings.ReplaceAll(s, "'", "''") + "'"
	}
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\t", `\t`, "\r", `\r`).Replace(s) + `"`
}
