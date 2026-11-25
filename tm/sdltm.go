package tm

import (
	"encoding/xml"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/internal/sqlitefile"
)

// A Trados Studio translation memory (.sdltm) is an SQLite database. Its
// table translation_memories describes the memory (name, languages), and
// translation_units holds each unit's source and target as a small XML
// document: the segment's text runs and tags, in order. The fields a memory
// defines (Client, Project and the like) are in attributes, and their
// values in string_attributes, numeric_attributes, date_attributes and,
// for lists, picklist_attributes with picklist_values.

// sdltmTime is how a Trados memory writes its dates, in UTC.
const sdltmTime = "2006-01-02 15:04:05"

// sdltmSystemFields are fields Trados keeps for itself, not ones a user
// defined: where in the document a unit came from ("x-tm-heading").
var sdltmSystemFields = map[string]bool{"structurecontext": true}

// ReadSDLTM reads a Trados Studio translation memory (.sdltm).
func ReadSDLTM(data []byte) (*File, error) {
	db, err := sqlitefile.Open(data)
	if err != nil {
		return nil, fmt.Errorf("sdltm: %w", err)
	}
	if db.Columns("translation_units") == nil || db.Columns("translation_memories") == nil {
		return nil, errors.New("sdltm: this SQLite database is not a Trados translation memory: it has no table of translation units")
	}
	f := &File{Format: FormatSDLTM}
	type memory struct{ name, src, tgt string }
	memories := map[int64]memory{}
	var order []int64
	err = db.Rows("translation_memories", func(r sqlitefile.Row) error {
		id, _ := r["id"].(int64)
		m := memory{name: str(r["name"]), src: str(r["source_language"]), tgt: str(r["target_language"])}
		memories[id] = m
		order = append(order, id)
		if len(order) == 1 {
			f.Name, f.SourceLang, f.TargetLang = m.name, m.src, m.tgt
			f.Description = strings.TrimSpace(str(r["description"]))
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("sdltm: %w", err)
	}
	if len(order) == 0 {
		return nil, errors.New("sdltm: the file describes no translation memory")
	}
	if len(order) > 1 {
		f.Warnings = append(f.Warnings, fmt.Sprintf("The file holds %d memories; their units are read together, each with its own languages.", len(order)))
	}

	fields, err := sdltmFields(db)
	if err != nil {
		return nil, fmt.Errorf("sdltm: %w", err)
	}
	badSegments := 0
	err = db.Rows("translation_units", func(r sqlitefile.Row) error {
		id, _ := r["id"].(int64)
		m, ok := memories[toInt(r["translation_memory_id"])]
		if !ok {
			m = memories[order[0]]
		}
		src, err1 := sdltmSegment(str(r["source_segment"]), 's')
		tgt, err2 := sdltmSegment(str(r["target_segment"]), 't')
		if err1 != nil || err2 != nil {
			badSegments++
			f.Skipped++
			return nil
		}
		u := Unit{
			Source:     src,
			Target:     tgt,
			CreatedAt:  sdltmDate(r["creation_date"]),
			CreatedBy:  str(r["creation_user"]),
			ChangedAt:  sdltmDate(r["change_date"]),
			ChangedBy:  str(r["change_user"]),
			UsageCount: int(toInt(r["usage_counter"])),
			Attributes: fields[id],
		}
		f.Units = append(f.Units, FileUnit{Unit: u, SourceLang: m.src, TargetLang: m.tgt})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("sdltm: %w", err)
	}
	if badSegments > 0 {
		f.Warnings = append(f.Warnings, fmt.Sprintf("%d units could not be read and were left out.", badSegments))
	}
	return f, nil
}

// sdltmFields reads the values of the memory's own fields for every unit,
// by unit ID, as attributes named after the field.
func sdltmFields(db *sqlitefile.DB) (map[int64][]Attribute, error) {
	out := map[int64][]Attribute{}
	if db.Columns("attributes") == nil {
		return out, nil
	}
	names := map[int64]string{}
	err := db.Rows("attributes", func(r sqlitefile.Row) error {
		name := strings.TrimSpace(str(r["name"]))
		if name != "" && !sdltmSystemFields[strings.ToLower(name)] {
			names[toInt(r["id"])] = name
		}
		return nil
	})
	if err != nil || len(names) == 0 {
		return out, err
	}
	add := func(unit, attr int64, value string) {
		if name, ok := names[attr]; ok && strings.TrimSpace(value) != "" {
			out[unit] = append(out[unit], Attribute{Name: name, Value: value})
		}
	}
	for _, t := range []string{"string_attributes", "numeric_attributes", "date_attributes"} {
		if db.Columns(t) == nil {
			continue
		}
		err := db.Rows(t, func(r sqlitefile.Row) error {
			v := r["value"]
			var s string
			switch x := v.(type) {
			case int64:
				s = strconv.FormatInt(x, 10)
			case float64:
				s = strconv.FormatFloat(x, 'f', -1, 64)
			default:
				s = str(x)
			}
			if t == "date_attributes" {
				if d := sdltmDate(v); !d.IsZero() {
					s = d.Format(time.RFC3339)
				}
			}
			add(toInt(r["translation_unit_id"]), toInt(r["attribute_id"]), s)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	if db.Columns("picklist_attributes") != nil && db.Columns("picklist_values") != nil {
		type value struct {
			attr int64
			text string
		}
		values := map[int64]value{}
		err := db.Rows("picklist_values", func(r sqlitefile.Row) error {
			values[toInt(r["id"])] = value{toInt(r["attribute_id"]), str(r["value"])}
			return nil
		})
		if err != nil {
			return nil, err
		}
		err = db.Rows("picklist_attributes", func(r sqlitefile.Row) error {
			if v, ok := values[toInt(r["picklist_value_id"])]; ok {
				add(toInt(r["translation_unit_id"]), v.attr, v.text)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	for id := range out {
		sort.SliceStable(out[id], func(i, j int) bool { return out[id][i].Name < out[id][j].Name })
	}
	return out, nil
}

// sdltmXML is a segment as Trados writes it in a memory.
type sdltmXML struct {
	Elements struct {
		Items []sdltmElement `xml:",any"`
	} `xml:"Elements"`
}

type sdltmElement struct {
	XMLName         xml.Name
	Value           string `xml:"Value"`
	Type            string `xml:"Type"`
	Anchor          int    `xml:"Anchor"`
	AlignmentAnchor int    `xml:"AlignmentAnchor"`
	TagID           string `xml:"TagID"`
	TextEquivalent  string `xml:"TextEquivalent"`
}

// sdltmSegment turns a segment into content. Tags are paired by Anchor
// within the segment; AlignmentAnchor says which tag of the source a tag
// of the target is, so both get the same ID. A tag with no counterpart
// gets an ID of its side (side 's' or 't') and its anchor.
func sdltmSegment(s string, side byte) (content.Content, error) {
	var seg sdltmXML
	if err := xml.Unmarshal([]byte(s), &seg); err != nil {
		return nil, err
	}
	c := content.Content{}
	open := map[int]string{}  // anchor to ID of an open tag
	var opened []int          // anchors in the order their tags opened
	used := map[string]bool{} // IDs of open tags so far
	id := func(e sdltmElement) string {
		if e.AlignmentAnchor > 0 {
			return strconv.Itoa(e.AlignmentAnchor)
		}
		return string(side) + strconv.Itoa(e.Anchor)
	}
	for _, e := range seg.Elements.Items {
		switch e.XMLName.Local {
		case "Text":
			c = c.Append(e.Value)
		case "Tag":
			native := "tag " + e.TagID
			switch e.Type {
			case "Start":
				tid := id(e)
				if used[tid] {
					tid = string(side) + strconv.Itoa(e.Anchor)
				}
				used[tid] = true
				open[e.Anchor] = tid
				opened = append(opened, e.Anchor)
				c = c.AppendTag(content.Tag{Kind: content.TagOpen, ID: tid, Native: native})
			case "End":
				tid, ok := open[e.Anchor]
				if !ok {
					// An end without its start: kept as a placeholder, so
					// the content stays well formed.
					c = c.AppendTag(content.Tag{Kind: content.TagStandalone, ID: id(e), Native: native})
					continue
				}
				delete(open, e.Anchor)
				c = c.AppendTag(content.Tag{Kind: content.TagClose, ID: tid, Native: native})
			default:
				// Standalone, TextPlaceholder, LockedContent and others.
				c = c.AppendTag(content.Tag{Kind: content.TagStandalone, ID: id(e), Desc: e.TextEquivalent, Native: native})
			}
		default:
			// Recognized tokens (numbers, dates, measures) keep their text.
			if e.Value != "" {
				c = c.Append(e.Value)
			}
		}
	}
	// A start without its end is closed at the end of the segment, the
	// last opened first.
	for i := len(opened) - 1; i >= 0; i-- {
		if tid, ok := open[opened[i]]; ok {
			c = c.AppendTag(content.Tag{Kind: content.TagClose, ID: tid})
		}
	}
	return c, c.Validate()
}

func sdltmDate(v any) time.Time {
	s, ok := v.(string)
	if !ok {
		return time.Time{}
	}
	t, err := time.Parse(sdltmTime, strings.TrimSpace(s))
	if err != nil || t.Year() >= 9999 {
		return time.Time{}
	}
	return t
}

func str(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []byte:
		return string(x)
	case int64:
		return strconv.FormatInt(x, 10)
	}
	return ""
}

func toInt(v any) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case float64:
		return int64(x)
	case string:
		n, _ := strconv.ParseInt(strings.TrimSpace(x), 10, 64)
		return n
	}
	return 0
}
