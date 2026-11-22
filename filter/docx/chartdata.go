package docx

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"strings"

	"github.com/4nonX/interlinea/filter"
)

// updateChartData writes translated chart labels into the workbook each
// chart embeds, so "Edit data" in Word shows the translation instead of
// bringing back the source labels. Only shared strings that are exactly a
// translated label are changed.
func (d *document) updateChartData(changed map[string][]byte, tr filter.Translations) error {
	byChart := map[*part]map[string]string{}
	for _, l := range d.labels {
		u := d.ext.Units[l.unit]
		t, ok := tr[u.ID]
		if !ok || len(t) == 0 {
			continue
		}
		m := byChart[l.part]
		if m == nil {
			m = map[string]string{}
			byChart[l.part] = m
		}
		m[u.Source.Plain()] = t.Plain()
	}
	for p, labels := range byChart {
		rels, err := readRels(d.zr, p.file)
		if err != nil {
			continue // no embedded data
		}
		for _, r := range rels {
			if !strings.HasSuffix(r.typ, "/package") || !strings.HasSuffix(strings.ToLower(r.target), ".xlsx") {
				continue
			}
			wb, err := readFile(d.zr, r.target)
			if err != nil {
				continue
			}
			out, ok, err := translateSharedStrings(wb, labels)
			if err != nil {
				return err
			}
			if ok {
				changed[r.target] = out
			}
		}
	}
	return nil
}

// translateSharedStrings replaces shared strings of a workbook that equal a
// key of labels. It reports whether anything changed.
func translateSharedStrings(wb []byte, labels map[string]string) ([]byte, bool, error) {
	zr, err := zip.NewReader(bytes.NewReader(wb), int64(len(wb)))
	if err != nil {
		return nil, false, nil // not a workbook we understand; leave it
	}
	data, err := readFile(zr, "xl/sharedStrings.xml")
	if err != nil {
		return nil, false, nil
	}
	toks, err := tokenize(data)
	if err != nil {
		return nil, false, nil
	}
	var out bytes.Buffer
	last, n := 0, 0
	for i := 0; i < len(toks); i++ {
		st, ok := toks[i].tok.(xml.StartElement)
		if !ok || st.Name.Local != "si" {
			continue
		}
		si, next, err := build(data, toks, i)
		if err != nil {
			return nil, false, nil
		}
		i = next - 1
		var t *node
		for _, k := range si.kids {
			if k.isText() {
				continue
			}
			if k.local != "t" || t != nil {
				t = nil
				break
			}
			t = k
		}
		if t == nil || len(t.end) == 0 {
			continue // rich text or empty: left as it is
		}
		target, ok := labels[t.innerText()]
		if !ok {
			continue
		}
		textStart := t.off + len(t.start)
		textEnd := t.off + len(t.raw) - len(t.end)
		out.Write(data[last:textStart])
		out.WriteString(escapeText(target))
		last = textEnd
		n++
	}
	if n == 0 {
		return nil, false, nil
	}
	out.Write(data[last:])
	res, err := rewriteZip(zr, map[string][]byte{"xl/sharedStrings.xml": out.Bytes()})
	return res, err == nil, err
}
