package project

import (
	"context"
	"fmt"
	"strings"

	"github.com/4nonX/interlinea/analysis"
	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter"
	"github.com/4nonX/interlinea/tm"
)

// Status is a segment's translation status.
type Status string

const (
	StatusNew       Status = "new"
	StatusDraft     Status = "draft"
	StatusConfirmed Status = "confirmed"
)

// Segment is a segment of a file in one target language.
type Segment struct {
	ID     string          `json:"id"`
	Source content.Content `json:"source"`
	Target content.Content `json:"target"`
	Status Status          `json:"status"`
	Locked bool            `json:"locked,omitempty"`

	// What the file says about the segment.

	// Block is the paragraph of the original document the segment is in.
	Block string `json:"block,omitempty"`
	// MaxLength is the longest the translation may be, in characters.
	MaxLength int `json:"maxLength,omitempty"`
	// Timing is when a subtitle is shown; nil for anything else. The parts
	// of a split subtitle keep the time of the whole, which is not theirs
	// alone: CheckTiming is nil for them.
	Timing *filter.Timing `json:"timing,omitempty"`
	// Notes are a bilingual file's comments on the segment.
	Notes []string `json:"notes,omitempty"`
	// File is the segment's state and translation in a bilingual file,
	// so writing the file back keeps a reviewer's approval of a
	// translation that did not change.
	File *FileState `json:"file,omitempty"`

	// Split says the segment is a part of a unit of the file that was
	// split, and Joined lists the units of the file a joined segment was
	// made of; the segment's ID is then the first unit's.
	Split  *SplitPart   `json:"split,omitempty"`
	Joined []JoinedUnit `json:"joined,omitempty"`

	// What the translator's work left on it.

	// Origin is where the translation came from when it was inserted.
	Origin *Origin `json:"origin,omitempty"`
	// Warnings are the checks' results saved when it was confirmed.
	Warnings []Warning `json:"warnings,omitempty"`
	// Ignored are the warnings marked as false alarms, by kind and text.
	Ignored []string `json:"ignored,omitempty"`
	// Comments are what the people working on the project wrote about
	// the segment, oldest first. They do not go into the document.
	Comments []Comment `json:"comments,omitempty"`
	// Issues are what a reviewer marked as wrong with the translation;
	// the file's score comes from them.
	Issues []Issue `json:"issues,omitempty"`
}

// Comment is a remark on a segment by someone working on the project.
type Comment struct {
	ID   string `json:"id"`
	Text string `json:"text"`
	By   string `json:"by"`
	// At is when it was written, in RFC 3339.
	At string `json:"at"`
}

// MaxCommentLength is how long a comment may be, in characters.
const MaxCommentLength = 2000

// FileState is a segment's state in its bilingual file.
type FileState struct {
	// State is "new", "draft", "translated", "reviewed", "final" or
	// "rejected".
	State string `json:"state"`
	// Target is the translation in the file, as content JSON.
	Target string `json:"target"`
	// Match is the match percentage the file records.
	Match int `json:"match,omitempty"`
	// Fixed: the program the file comes from keeps its own record of
	// each segment (its ID, state and history), so the segment cannot
	// be split or joined.
	Fixed bool `json:"fixed,omitempty"`
}

// Origin is where a translation came from.
type Origin struct {
	// Kind is "tm" (a memory match), "mt", "copy" (the source) or
	// "repetition" (the translation of an equal segment).
	Kind string `json:"kind"`
	// Score is a memory match's score; Memory its memory; Context marks
	// a context match.
	Score   int    `json:"score,omitempty"`
	Memory  string `json:"memory,omitempty"`
	Context bool   `json:"context,omitempty"`
	// Inserted is the translation as inserted, as content JSON, to see
	// whether it was edited.
	Inserted string `json:"inserted,omitempty"`
	// From is the segment a repetition was taken from.
	From string `json:"from,omitempty"`
}

// Warning is a check's result on a segment.
type Warning struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

// Edit is what saving a segment changes.
type Edit struct {
	Target   content.Content `json:"target"`
	Status   Status          `json:"status"`
	Origin   *Origin         `json:"origin,omitempty"`
	Warnings []Warning       `json:"warnings,omitempty"`
	Ignored  []string        `json:"ignored,omitempty"`
}

// FileSegments are a file's segments in one target language, in order.
type FileSegments struct {
	Name     string    `json:"name"`
	Segments []Segment `json:"segments"`
}

// Translation is what pre-translation put into a segment.
type Translation struct {
	File   string          `json:"file"`
	ID     string          `json:"id"`
	Target content.Content `json:"target"`
	// Score is the match's score after the memory's penalty.
	Score  int    `json:"score"`
	Memory string `json:"memory"`
	// Context reports a context match.
	Context bool `json:"context"`
	// Status is StatusConfirmed when the settings confirm this kind of
	// match, else StatusDraft.
	Status Status `json:"status"`
}

// Pretranslate fills the segments that are not locked, not translated and
// empty with their best match of at least p.MinScore from memories (with
// penalties already applied, as MemoriesFor gives them). The best match is
// the one the analysis counts: a context match under rule first, then the
// highest score. A segment filled with a 100% match is confirmed with
// p.ConfirmExact (context matches included), one filled with a context
// match with p.ConfirmContext. Segments are not changed; the translations
// are returned in file and segment order.
func Pretranslate(ctx context.Context, files []FileSegments, memories []tm.Memory, p Prepare, rule string) ([]Translation, error) {
	if p.MinScore < 50 || p.MinScore > 100 {
		return nil, fmt.Errorf("project: the lowest match to insert must be between 50 and 100, not %d", p.MinScore)
	}
	if len(memories) == 0 {
		return nil, nil
	}
	var out []Translation
	for _, f := range files {
		segs := make([]analysis.Segment, len(f.Segments))
		for i, s := range f.Segments {
			segs[i] = analysis.Segment{ID: s.ID, Source: s.Source, Locked: s.Locked}
		}
		for i, s := range f.Segments {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if s.Locked || (s.Status != StatusNew && s.Status != "") || !empty(s.Target) || strings.TrimSpace(s.Source.Plain()) == "" {
				continue
			}
			b, ok, err := analysis.BestMatch(ctx, segs, i, memories, p.MinScore, analysis.ContextRule(rule))
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
			t := Translation{File: f.Name, ID: s.ID, Target: b.Match.Unit.Target, Score: b.Match.Score, Memory: b.Memory, Context: b.Context, Status: StatusDraft}
			if (b.Context && p.ConfirmContext) || (b.Match.Score == 100 && p.ConfirmExact) {
				t.Status = StatusConfirmed
			}
			out = append(out, t)
		}
	}
	return out, nil
}

func empty(c content.Content) bool { return strings.TrimSpace(c.Plain()) == "" && !c.HasTags() }

// CheckTiming is the time a subtitle's checks count with: its own, or nil
// for a part of a split subtitle, whose time is that of the whole.
func (s Segment) CheckTiming() *filter.Timing {
	if s.Split != nil {
		return nil
	}
	return s.Timing
}
