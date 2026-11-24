package html

import (
	"html"
	"strings"
)

type tokenKind int

const (
	textToken  tokenKind = iota // character data, with entities
	rawToken                    // the content of script and style: not text
	startToken                  // <name ...>
	endToken                    // </name>
	otherToken                  // comment, doctype, processing instruction
)

// token is a piece of the page with its byte range.
type token struct {
	kind       tokenKind
	start, end int
	// name is the element's name in lower case, for start and end tags.
	name  string
	attrs []attr
}

// attr is an attribute of a start tag. The value span covers the value
// with its quotes; it is empty, at the end of the name, for an attribute
// without a value.
type attr struct {
	name           string // lower case
	value          string // entities decoded
	valStart, vEnd int
}

func (t token) attr(name string) (string, bool) {
	for _, a := range t.attrs {
		if a.name == name {
			return a.value, true
		}
	}
	return "", false
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'
}

func isLetter(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// tokenize splits a page into tokens as a browser's tokenizer does where
// it matters for text: tags with quoted and unquoted attributes, comments,
// and the content of script, style, title and textarea, in which a "<"
// starts nothing. Markup that is not well formed stays text or is passed
// over; nothing is dropped, so the tokens always add up to the page.
func tokenize(src string) []token {
	var out []token
	textStart := 0
	flush := func(end int) {
		if end > textStart {
			out = append(out, token{kind: textToken, start: textStart, end: end})
		}
	}
	i := 0
	for i < len(src) {
		if src[i] != '<' || i+1 >= len(src) {
			i++
			continue
		}
		switch c := src[i+1]; {
		case strings.HasPrefix(src[i:], "<!--"):
			end := strings.Index(src[i+4:], "-->")
			if end < 0 {
				end = len(src)
			} else {
				end += i + 4 + 3
			}
			flush(i)
			out = append(out, token{kind: otherToken, start: i, end: end})
			i, textStart = end, end
		case c == '!' || c == '?':
			end := strings.IndexByte(src[i:], '>')
			if end < 0 {
				end = len(src)
			} else {
				end += i + 1
			}
			flush(i)
			out = append(out, token{kind: otherToken, start: i, end: end})
			i, textStart = end, end
		case c == '/' && i+2 < len(src) && isLetter(src[i+2]):
			j := i + 2
			for j < len(src) && !isSpace(src[j]) && src[j] != '>' && src[j] != '/' {
				j++
			}
			name := strings.ToLower(src[i+2 : j])
			end := strings.IndexByte(src[j:], '>')
			if end < 0 {
				end = len(src)
			} else {
				end += j + 1
			}
			flush(i)
			out = append(out, token{kind: endToken, start: i, end: end, name: name})
			i, textStart = end, end
		case isLetter(c):
			t, end := startTag(src, i)
			flush(i)
			out = append(out, t)
			i, textStart = end, end
			// Elements whose content is not markup.
			switch t.name {
			case "script", "style", "title", "textarea":
				closing := indexFold(src[i:], "</"+t.name)
				if closing < 0 {
					closing = len(src) - i
				}
				if closing > 0 {
					kind := rawToken
					if t.name == "title" || t.name == "textarea" {
						kind = textToken
					}
					out = append(out, token{kind: kind, start: i, end: i + closing})
				}
				i += closing
				textStart = i
			}
		default:
			i++
		}
	}
	flush(len(src))
	return out
}

// startTag reads the start tag at src[i] and returns it with its end.
func startTag(src string, i int) (token, int) {
	j := i + 1
	for j < len(src) && !isSpace(src[j]) && src[j] != '>' && src[j] != '/' {
		j++
	}
	t := token{kind: startToken, start: i, name: strings.ToLower(src[i+1 : j])}
	for j < len(src) {
		for j < len(src) && (isSpace(src[j]) || src[j] == '/') {
			j++
		}
		if j >= len(src) {
			break
		}
		if src[j] == '>' {
			j++
			t.end = j
			return t, j
		}
		ns := j
		for j < len(src) && !isSpace(src[j]) && src[j] != '=' && src[j] != '>' && (src[j] != '/' || j == ns) {
			j++
		}
		a := attr{name: strings.ToLower(src[ns:j]), valStart: j, vEnd: j}
		k := j
		for k < len(src) && isSpace(src[k]) {
			k++
		}
		if k < len(src) && src[k] == '=' {
			k++
			for k < len(src) && isSpace(src[k]) {
				k++
			}
			a.valStart = k
			switch {
			case k < len(src) && (src[k] == '"' || src[k] == '\''):
				q := strings.IndexByte(src[k+1:], src[k])
				if q < 0 {
					q = len(src) - k - 1
					a.value = html.UnescapeString(src[k+1:])
					k = len(src)
				} else {
					a.value = html.UnescapeString(src[k+1 : k+1+q])
					k += q + 2
				}
			default:
				vs := k
				for k < len(src) && !isSpace(src[k]) && src[k] != '>' {
					k++
				}
				a.value = html.UnescapeString(src[vs:k])
			}
			a.vEnd = k
			j = k
		}
		t.attrs = append(t.attrs, a)
	}
	t.end = len(src)
	return t, len(src)
}

// indexFold finds needle, which is lower case, in s without regard to case.
func indexFold(s, needle string) int {
	for i := 0; i+len(needle) <= len(s); i++ {
		if s[i] == '<' && strings.EqualFold(s[i:i+len(needle)], needle) {
			return i
		}
	}
	return -1
}
