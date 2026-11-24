package term

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/4nonX/interlinea/internal/jet"
)

// IsSDLTB reports whether data is a MultiTerm termbase file (.sdltb), which
// is an Access database.
func IsSDLTB(data []byte) bool { return jet.IsDatabase(data) }

// sdltbTags are the short element names MultiTerm stores its entries
// with, and the names its XML export writes them as.
var sdltbTags = map[string]string{
	"cG": "conceptGrp", "c": "concept",
	"lG": "languageGrp", "l": "language",
	"tG": "termGrp", "t": "term",
	"dG": "descripGrp", "d": "descrip",
	"trG": "transacGrp", "tr": "transac", "dt": "date",
}

var sdltbTag = regexp.MustCompile(`<(/?)(cG|c|lG|l|tG|t|dG|d|trG|tr|dt)([\s/>])`)

// ReadSDLTB reads a MultiTerm termbase file (.sdltb) as MultiTerm itself
// keeps it: an Access database whose table mtConcepts holds one entry a
// row, as MultiTerm XML with shortened element names. The entries are read
// as an export to MultiTerm XML would be, so both give the same termbase.
// A termbase protected with a password is read too: the password guards
// MultiTerm's own sign-in, not the file.
func ReadSDLTB(data []byte) (*Termbase, Report, error) {
	db, err := jet.Open(data)
	if err != nil {
		return nil, Report{}, fmt.Errorf("sdltb: %w", err)
	}
	concepts, err := db.Table("mtConcepts")
	if err != nil {
		return nil, Report{}, errors.New("sdltb: this Access database is not a MultiTerm termbase: it has no table of entries")
	}
	type row struct {
		id   int64
		text string
	}
	var rows []row
	seen := map[int64]bool{}
	err = concepts.Rows(func(r jet.Row) error {
		id, _ := r["conceptid"].(int64)
		text, _ := r["text"].(string)
		if text == "" || seen[id] {
			return nil
		}
		seen[id] = true
		rows = append(rows, row{id, text})
		return nil
	})
	if err != nil {
		return nil, Report{}, fmt.Errorf("sdltb: %w", err)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].id < rows[j].id })

	var doc bytes.Buffer
	doc.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<mtf>\n")
	for _, r := range rows {
		doc.WriteString(sdltbTag.ReplaceAllStringFunc(r.text, func(m string) string {
			p := sdltbTag.FindStringSubmatch(m)
			return "<" + p[1] + sdltbTags[p[2]] + p[3]
		}))
		doc.WriteString("\n")
	}
	doc.WriteString("</mtf>\n")
	tb, rep, err := ReadMultiTerm(doc.Bytes())
	if err != nil {
		return nil, rep, fmt.Errorf("sdltb: %w", err)
	}

	// The termbase's name and description are kept apart from the entries.
	if props, err := db.Table("mtBlobs"); err == nil {
		_ = props.Rows(func(r jet.Row) error {
			name, _ := r["name"].(string)
			var value string
			switch v := r["blob"].(type) {
			case string:
				value = v
			case []byte:
				value = string(ToUTF8(v))
			}
			if strings.EqualFold(name, "FriendlyName") && strings.TrimSpace(value) != "" {
				tb.Name = strings.TrimSpace(value)
			}
			return nil
		})
	}
	return tb, rep, nil
}
