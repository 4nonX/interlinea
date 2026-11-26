package xliff

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter"
	"github.com/4nonX/interlinea/internal/rtf"
)

// Déjà Vu exports a project as an RTF external view: a table with a row
// for each segment, its number (the segment ID, which Déjà Vu needs to
// read the file back), the source, the target and a comment. Déjà Vu's
// codes are text in braces, {00108}. Rows before the first segment (a
// header with the languages' names) are kept as they are; only target
// cells change.

// FormatDVRTF is a Déjà Vu RTF external view.
const FormatDVRTF = "dvrtf"

var (
	dvTagRE = regexp.MustCompile(`\{\d{1,6}\}`)
	dvID    = regexp.MustCompile(`^\d+$`)
)

var dvTags = tagPattern{re: dvTagRE, kind: func(m string) (content.TagKind, string) {
	return content.TagStandalone, strings.Trim(m, "{}")
}}

// dvRows returns the rows that are segments: three or more cells, a
// number in the first.
func dvRows(doc *rtf.Doc) []int {
	var rows []int
	for i, r := range doc.Rows {
		if len(r.Cells) >= 3 && dvID.MatchString(strings.TrimSpace(r.Cells[0].Text())) {
			rows = append(rows, i)
		}
	}
	return rows
}

// isDVRTF reports whether an RTF table is a Déjà Vu external view.
func isDVRTF(doc *rtf.Doc) bool {
	return !isMQRTF(doc) && len(dvRows(doc)) > 0
}

func readDVRTF(doc *rtf.Doc) (*rtfTable, error) {
	f := &rtfTable{doc: doc, byID: map[string]*rtfSegment{}, tagStyle: -1}
	f.ext.Format = FormatDVRTF
	data := dvRows(doc)
	if len(data) == 0 {
		return nil, fmt.Errorf("xliff: the D\u00e9j\u00e0 Vu external view has no segments")
	}
	srcCol, tgtCol, commentCol := 1, 2, -1
	if len(doc.Rows[data[0]].Cells) >= 4 {
		commentCol = 3
	}
	// A header row just before the segments names the languages.
	srcName, tgtName := "", ""
	if h := data[0] - 1; h >= 0 && len(doc.Rows[h].Cells) == len(doc.Rows[data[0]].Cells) {
		srcName = doc.Rows[h].Cells[srcCol].Text()
		tgtName = doc.Rows[h].Cells[tgtCol].Text()
	}
	f.ext.SourceLang = rtfLang(srcName, cellLang(doc, data, srcCol))
	f.ext.TargetLang = rtfLang(tgtName, cellLang(doc, data, tgtCol))
	if f.ext.SourceLang == "" || f.ext.TargetLang == "" {
		f.ext.Warnings = append(f.ext.Warnings, noLanguagesNote)
	}
	f.ext.Blocks = append(f.ext.Blocks, filter.Block{ID: "b1", Part: filter.PartBody})
	for _, i := range data {
		cells := doc.Rows[i].Cells
		id := strings.TrimSpace(cells[0].Text())
		sg := &rtfSegment{row: i, target: tgtCol, status: -1}
		src, tags := cellContent(cells[srcCol], -1, dvTags, nil)
		// Codes are plain text in the cell; a translation writes them so.
		for k := range tags {
			tags[k].prefix = ""
		}
		sg.tags = tags
		tgt, _ := cellContent(cells[tgtCol], -1, dvTags, sg)
		sg.unit = filter.Unit{ID: id, Block: "b1", Key: id, Source: src, State: filter.StateNew}
		if strings.TrimSpace(tgt.Plain()) != "" || tgt.HasTags() {
			sg.unit.Target = tgt
			sg.unit.State = filter.StateTranslated
		}
		if commentCol >= 0 && commentCol < len(cells) {
			if note := strings.TrimSpace(cells[commentCol].Text()); note != "" {
				sg.unit.Notes = []string{note}
			}
		}
		if _, dup := f.byID[id]; dup {
			return nil, fmt.Errorf("xliff: two rows of the D\u00e9j\u00e0 Vu external view have the segment ID %s", id)
		}
		f.byID[id] = sg
		f.segments = append(f.segments, sg)
		f.ext.Units = append(f.ext.Units, sg.unit)
	}
	return f, nil
}
