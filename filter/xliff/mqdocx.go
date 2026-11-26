package xliff

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/4nonX/interlinea/filter"
	"github.com/4nonX/interlinea/internal/rtf"
	"github.com/4nonX/interlinea/internal/xmlraw"
)

// memoQ writes its two-column export as a Word document too: the same
// table as the two-column RTF (the document and project in the first
// row, the header ID, the languages, Comment, Status in the second, a
// segment in each further row), with tags in the character style
// mqInternal.

// FormatMQDOCX is a memoQ two-column DOCX file.
const FormatMQDOCX = "mqdocx"

// mqDOCXTable finds memoQ's table in a document and its header row.
func mqDOCXTable(root *xmlraw.Node, styled bool) (*xmlraw.Node, int) {
	body := root.Child("body")
	if body == nil {
		return nil, -1
	}
	for _, t := range body.Children("tbl") {
		rows := t.Children("tr")
		if len(rows) < 2 {
			continue
		}
		header := -1
		for i, tr := range rows {
			if i > 3 {
				break
			}
			cells := tableCells(tr)
			if len(cells) >= 3 && strings.EqualFold(strings.TrimSpace(cellText(cells[0])), "ID") {
				header = i
				break
			}
		}
		first := tableCells(rows[0])
		marked := len(first) > 0 && mqHeaderLine.MatchString(cellText(first[0]))
		if header >= 0 && (marked || styled) {
			return t, header
		}
	}
	return nil, -1
}

// mqCell is a cell's runs as an RTF cell, for the tag reader the RTF
// tables use: runs in the tag style have style 1.
func (f *reviewFile) mqCell(tc *xmlraw.Node) rtf.Cell {
	var c rtf.Cell
	for _, r := range cellRuns(tc) {
		st := 0
		if r.style == f.mqStyle && f.mqStyle != "" {
			st = 1
		}
		c.Runs = append(c.Runs, rtf.Run{Text: r.text, Style: st, Group: -1})
	}
	return c
}

func (f *reviewFile) readMQ(table *xmlraw.Node, header int) (*reviewFile, error) {
	f.ext.Format = FormatMQDOCX
	rows := table.Children("tr")
	head := tableCells(rows[header])
	srcCol, tgtCol, commentCol, statusCol := 1, 2, -1, -1
	for i, c := range head {
		switch strings.ToLower(strings.TrimSpace(cellText(c))) {
		case "comment", "comments":
			commentCol = i
		case "status":
			statusCol = i
		}
	}
	if len(head) >= 5 {
		if commentCol < 0 {
			commentCol = 3
		}
		if statusCol < 0 {
			statusCol = 4
		}
	}
	name := ""
	if header > 0 {
		if cells := tableCells(rows[0]); len(cells) > 0 {
			name = strings.TrimSpace(strings.Split(cellText(cells[0]), "\n")[0])
		}
	}
	f.ext.Blocks = append(f.ext.Blocks, filter.Block{ID: "b1", Part: filter.PartBody, File: name})
	srcLang, tgtLang := "", ""
	for _, tr := range rows[header+1:] {
		cells := tableCells(tr)
		if len(cells) <= tgtCol {
			continue
		}
		idText := cellText(cells[0])
		fields := strings.Fields(idText)
		if len(fields) == 0 {
			continue
		}
		if _, err := strconv.Atoi(fields[0]); err != nil {
			continue
		}
		num := fields[0]
		id := mqGUID.FindString(idText)
		if id == "" {
			id = num
		}
		sg := &reviewSegment{target: cells[tgtCol], mq: &rtfSegment{target: tgtCol, status: -1}}
		for _, p := range cells[tgtCol].Children("p") {
			if sg.lang = paraLang(p); sg.lang != "" {
				break
			}
		}
		if srcLang == "" {
			for _, p := range cells[srcCol].Children("p") {
				if srcLang = paraLang(p); srcLang != "" {
					break
				}
			}
		}
		if tgtLang == "" {
			tgtLang = sg.lang
		}
		src, tags := cellContent(f.mqCell(cells[srcCol]), 1, mqTags, nil)
		sg.mq.tags = tags
		tgt, _ := cellContent(f.mqCell(cells[tgtCol]), 1, mqTags, sg.mq)
		sg.unit = filter.Unit{ID: id, Block: "b1", Key: num, Source: src}
		hasText := strings.TrimSpace(tgt.Plain()) != "" || tgt.HasTags()
		if hasText {
			sg.unit.Target = tgt
		}
		status := ""
		if statusCol >= 0 && statusCol < len(cells) {
			sg.status = cells[statusCol]
			status = cellText(cells[statusCol])
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
			if note := strings.TrimSpace(cellText(cells[commentCol])); note != "" {
				sg.unit.Notes = []string{note}
			}
		}
		if _, dup := f.byID[id]; dup {
			return nil, fmt.Errorf("xliff: two segments of the two-column document have the ID %q", id)
		}
		f.byID[id] = sg
		f.segments = append(f.segments, sg)
		f.ext.Units = append(f.ext.Units, sg.unit)
	}
	if len(f.segments) == 0 {
		return nil, fmt.Errorf("xliff: the two-column document has no segments")
	}
	f.ext.SourceLang = rtfLang(cellText(head[srcCol]), 0)
	f.ext.TargetLang = rtfLang(cellText(head[tgtCol]), 0)
	if f.ext.SourceLang == "" {
		f.ext.SourceLang = ttxLang(srcLang)
	}
	if f.ext.TargetLang == "" {
		f.ext.TargetLang = ttxLang(tgtLang)
	}
	if f.ext.SourceLang == "" || f.ext.TargetLang == "" {
		f.ext.Warnings = append(f.ext.Warnings, noLanguagesNote)
	}
	return f, nil
}
