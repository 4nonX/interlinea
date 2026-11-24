package textfmt

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// parseJSON reads a JSON file: every string that is a value is a string
// to translate, named by its path (menu.file.open, items[2].title). The
// names of the members are not translated.
func parseJSON(src string) ([]*slot, error) {
	if !json.Valid([]byte(src)) {
		return nil, fmt.Errorf("the file is not valid JSON")
	}
	type level struct {
		object bool
		key    string
		index  int
		// wantKey: in an object, the next string is a member's name.
		wantKey bool
	}
	var stack []level
	path := func() string {
		var b strings.Builder
		for _, l := range stack {
			if l.object {
				if b.Len() > 0 {
					b.WriteByte('.')
				}
				b.WriteString(l.key)
			} else {
				fmt.Fprintf(&b, "[%d]", l.index)
			}
		}
		return b.String()
	}
	// A value is done: the place after it is the next member or element.
	done := func() {
		if n := len(stack); n > 0 {
			if stack[n-1].object {
				stack[n-1].wantKey = true
			} else {
				stack[n-1].index++
			}
		}
	}
	var out []*slot
	for i := 0; i < len(src); i++ {
		switch c := src[i]; {
		case c == '{':
			stack = append(stack, level{object: true, wantKey: true})
		case c == '[':
			stack = append(stack, level{})
		case c == '}' || c == ']':
			stack = stack[:len(stack)-1]
			done()
		case c == '"':
			end := i + 1
			for src[end] != '"' {
				if src[end] == '\\' {
					end++
				}
				end++
			}
			var text string
			if err := json.Unmarshal([]byte(src[i:end+1]), &text); err != nil {
				return nil, err
			}
			if n := len(stack); n > 0 && stack[n-1].object && stack[n-1].wantKey {
				stack[n-1].key, stack[n-1].wantKey = text, false
			} else {
				key := path()
				out = append(out, &slot{start: i, end: end + 1, id: "j" + strconv.Itoa(len(out)+1), text: text, key: key, write: quoteJSON})
				done()
			}
			i = end
		case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == ',' || c == ':':
		default:
			// A number, true, false or null.
			for i+1 < len(src) && !strings.ContainsRune(",}] \t\n\r", rune(src[i+1])) {
				i++
			}
			done()
		}
	}
	return out, nil
}

// quoteJSON writes a string as JSON, with its letters as they are.
func quoteJSON(s string) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		return `""`
	}
	return strings.TrimRight(b.String(), "\n")
}
