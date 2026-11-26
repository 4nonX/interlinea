package xliff

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter"
	"github.com/4nonX/interlinea/internal/rtf"
)

// memoQ exports a document as a two-column RTF table for review in Word
// (Export bilingual, Two-column RTF). Its first row names the document
// and the memoQ project ("MQ920910" and a GUID, after the memoQ version);
// the second is the header: ID, the source and target languages by
// name, and Comment and Status when the export has them. Every further
// row is a segment: its number and GUID, source, target, comment and
// status ("Not started", "Pre-translated (99%)"). memoQ's tags are text
// in the character style mqInternal: [1] stands alone, [2} opens, {3]
// closes. memoQ reads the file back by the segment GUIDs, so only the
// target, comment and status cells may change.

// FormatMQRTF is a memoQ two-column RTF file.
const FormatMQRTF = "mqrtf"

var (
	mqHeaderLine = regexp.MustCompile(`(?m)^\s*(?:V[\d.]+\s+)?MQ\d+\s+[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}\s*$`)
	mqGUID       = regexp.MustCompile(`[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}`)
	mqTagRE      = regexp.MustCompile(`\[\d+\]|\[\d+\}|\{\d+\]`)
	mqPercent    = regexp.MustCompile(`\((\d{1,3})%\)\s*$`)
)

var mqTags = tagPattern{re: mqTagRE, kind: func(m string) (content.TagKind, string) {
	num := strings.Trim(m, "[]{}")
	switch {
	case strings.HasPrefix(m, "[") && strings.HasSuffix(m, "}"):
		return content.TagOpen, num
	case strings.HasPrefix(m, "{"):
		return content.TagClose, num
	}
	return content.TagStandalone, num
}}

// isMQRTF reports whether an RTF table is memoQ's.
func isMQRTF(doc *rtf.Doc) bool {
	if len(doc.Rows) == 0 {
		return false
	}
	if mqHeaderLine.MatchString(doc.Rows[0].Cells[0].Text()) {
		return true
	}
	_, styled := doc.Styles["mqInternal"]
	return styled && mqHeaderRow(doc) >= 0
}

// mqHeaderRow is the index of the header row, -1 if there is none.
func mqHeaderRow(doc *rtf.Doc) int {
	for i, r := range doc.Rows {
		if i > 3 {
			break
		}
		if len(r.Cells) >= 3 && strings.EqualFold(strings.TrimSpace(r.Cells[0].Text()), "ID") {
			return i
		}
	}
	return -1
}

// mqState reads a status cell: the state, the match it shows and whether
// the segment is locked.
func mqState(status string, hasText bool) (filter.State, int, string, bool) {
	s := strings.TrimSpace(status)
	match := 0
	if m := mqPercent.FindStringSubmatch(s); m != nil {
		match, _ = strconv.Atoi(m[1])
		s = strings.TrimSpace(s[:len(s)-len(m[0])])
	}
	low := strings.ToLower(s)
	locked := strings.Contains(low, "locked")
	origin := ""
	var st filter.State
	switch {
	case strings.Contains(low, "reviewer 2"), strings.Contains(low, "proofread"):
		st = filter.StateFinal
	case strings.Contains(low, "reviewer 1"), strings.Contains(low, "reviewer confirmed"):
		st = filter.StateReviewed
	case strings.Contains(low, "rejected"):
		st = filter.StateRejected
	case strings.Contains(low, "confirmed"):
		st = filter.StateTranslated
	case strings.Contains(low, "pre-translated"), strings.Contains(low, "pretranslated"):
		st, origin = filter.StateDraft, "tm"
	case strings.Contains(low, "machine"):
		st, origin = filter.StateDraft, "mt"
	case strings.Contains(low, "edited"), strings.Contains(low, "assembled"), strings.Contains(low, "fragment"):
		st = filter.StateDraft
	case strings.Contains(low, "not started"):
		st = filter.StateNew
	case hasText:
		st = filter.StateDraft
	default:
		st = filter.StateNew
	}
	if st == filter.StateNew && hasText {
		st = filter.StateDraft
	}
	return st, match, origin, locked
}

// mqStatus is the status memoQ shows for a state.
func mqStatus(s filter.State) string {
	switch s {
	case filter.StateDraft:
		return "Edited"
	case filter.StateTranslated:
		return "Translator confirmed"
	case filter.StateReviewed:
		return "Reviewer 1 confirmed"
	case filter.StateFinal:
		return "Reviewer 2 confirmed"
	case filter.StateRejected:
		return "Rejected"
	}
	return "Not started"
}

func readMQRTF(doc *rtf.Doc) (*rtfTable, error) {
	f := &rtfTable{doc: doc, byID: map[string]*rtfSegment{}, tagStyle: -1, status: mqStatus}
	f.ext.Format = FormatMQRTF
	if n, ok := doc.Styles["mqInternal"]; ok {
		f.tagStyle = n
		f.defaultPrefix = `\noproof\cs` + strconv.Itoa(n) + ` `
	}
	header := mqHeaderRow(doc)
	srcCol, tgtCol, commentCol, statusCol := 1, 2, -1, -1
	first := 1
	if header >= 0 {
		first = header + 1
		cells := doc.Rows[header].Cells
		for i, c := range cells {
			switch strings.ToLower(strings.TrimSpace(c.Text())) {
			case "comment", "comments":
				commentCol = i
			case "status":
				statusCol = i
			}
		}
		if len(cells) >= 5 {
			if commentCol < 0 {
				commentCol = 3
			}
			if statusCol < 0 {
				statusCol = 4
			}
		}
	}
	var data []int
	for i := first; i < len(doc.Rows); i++ {
		cells := doc.Rows[i].Cells
		if len(cells) <= tgtCol {
			continue
		}
		id := strings.Fields(cells[0].Text())
		if len(id) == 0 {
			continue
		}
		if _, err := strconv.Atoi(id[0]); err != nil {
			continue
		}
		data = append(data, i)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("xliff: the memoQ RTF file has no segments")
	}
	srcName, tgtName := "", ""
	if header >= 0 {
		srcName = doc.Rows[header].Cells[srcCol].Text()
		tgtName = doc.Rows[header].Cells[tgtCol].Text()
	}
	f.ext.SourceLang = rtfLang(srcName, cellLang(doc, data, srcCol))
	f.ext.TargetLang = rtfLang(tgtName, cellLang(doc, data, tgtCol))
	if f.ext.SourceLang == "" || f.ext.TargetLang == "" {
		f.ext.Warnings = append(f.ext.Warnings, noLanguagesNote)
	}
	name := ""
	if lines := strings.Split(doc.Rows[0].Cells[0].Text(), "\n"); header > 0 && len(lines) > 0 {
		name = strings.TrimSpace(lines[0])
	}
	f.ext.Blocks = append(f.ext.Blocks, filter.Block{ID: "b1", Part: filter.PartBody, File: name})
	for _, i := range data {
		cells := doc.Rows[i].Cells
		idText := cells[0].Text()
		num := strings.Fields(idText)[0]
		id := mqGUID.FindString(idText)
		if id == "" {
			id = num
		}
		sg := &rtfSegment{row: i, target: tgtCol, status: -1}
		src, tags := cellContent(cells[srcCol], f.tagStyle, mqTags, nil)
		sg.tags = tags
		tgt, _ := cellContent(cells[tgtCol], f.tagStyle, mqTags, sg)
		sg.unit = filter.Unit{ID: id, Block: "b1", Key: num, Source: src}
		hasText := strings.TrimSpace(tgt.Plain()) != "" || tgt.HasTags()
		if hasText {
			sg.unit.Target = tgt
		}
		status := ""
		if statusCol >= 0 && statusCol < len(cells) {
			sg.status = statusCol
			status = cells[statusCol].Text()
			if m := mqPercent.FindString(strings.TrimSpace(status)); m != "" {
				sg.statusSuffix = m
			}
		}
		st, match, origin, locked := mqState(status, hasText)
		sg.unit.State, sg.unit.Locked = st, locked
		if hasText {
			sg.unit.Match, sg.unit.Origin = match, origin
		}
		if commentCol >= 0 && commentCol < len(cells) {
			if note := strings.TrimSpace(cells[commentCol].Text()); note != "" {
				sg.unit.Notes = []string{note}
			}
		}
		if _, dup := f.byID[id]; dup {
			return nil, fmt.Errorf("xliff: two segments of the memoQ RTF file have the ID %q", id)
		}
		f.byID[id] = sg
		f.segments = append(f.segments, sg)
		f.ext.Units = append(f.ext.Units, sg.unit)
	}
	return f, nil
}
