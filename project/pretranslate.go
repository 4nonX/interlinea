package project

import (
	"context"
	"fmt"
	"strings"

	"github.com/4nonX/interlinea/analysis"
	"github.com/4nonX/interlinea/content"
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
