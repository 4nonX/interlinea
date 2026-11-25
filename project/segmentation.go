package project

import (
	"fmt"

	"github.com/4nonX/interlinea/segment"
)

// SegmentationRule is a rule of the project's own for where sentences
// end: at a place where the text before it ends with Before and the text
// after it begins with After (regular expressions, either may be empty),
// a segment ends (Break) or does not. The project's rules come before
// the ones Interlinea has for the language, and the first rule that
// matches a place decides it.
type SegmentationRule struct {
	Break  bool   `json:"break"`
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
}

// MaxSegmentationRules is how many rules of its own a project may have.
const MaxSegmentationRules = 100

// Check reports what is wrong with the rule.
func (r SegmentationRule) Check() error {
	if r.Before == "" && r.After == "" {
		return fmt.Errorf("Say what stands before the place, after it, or both.")
	}
	if _, err := segment.NewRule(r.Break, r.Before, r.After); err != nil {
		return fmt.Errorf("The rule is not a regular expression that can be used: %v", err)
	}
	return nil
}

// ApplySegmentation cuts a file's segments as the project's rules say,
// by joining and splitting the ones its filter read: where a rule says a
// sentence does not end, the two segments of the paragraph are joined,
// and where it says one ends inside a segment, the segment is split.
// Segments that are locked, whose file keeps its own record of them, or
// that were split or joined already stay as they are.
func ApplySegmentation(segs []Segment, rules []SegmentationRule, lang string) []Segment {
	var own []segment.Rule
	for _, r := range rules {
		if c, err := segment.NewRule(r.Break, r.Before, r.After); err == nil {
			own = append(own, c)
		}
	}
	if len(own) == 0 {
		return segs
	}
	usual := segment.Default().For(lang)
	seg := segment.FromRules(append(own, usual...))
	without := segment.FromRules(usual)
	plain := func(s Segment) bool {
		return s.Block != "" && !s.Locked && !fixed(s) && s.Split == nil && len(s.Joined) == 0
	}
	var out []Segment
	for i := 0; i < len(segs); {
		if !plain(segs[i]) {
			out = append(out, segs[i])
			i++
			continue
		}
		j := i + 1
		for j < len(segs) && plain(segs[j]) && segs[j].Block == segs[i].Block {
			j++
		}
		out = append(out, recut(segs[i:j], seg, without)...)
		i = j
	}
	return out
}

// recut cuts the segments of one paragraph as the project's rules change
// it: seg has them, without is how the language is cut without them.
// Only what the rules change is changed, so a boundary the file's filter
// set for a reason of its own stays.
func recut(group []Segment, seg, without *segment.Segmenter) []Segment {
	// The paragraph as its segments joined would read, and where each
	// segment starts and ends in it.
	units := make([]JoinedUnit, len(group))
	starts, ends := make([]int, len(group)), make([]int, len(group))
	for k, s := range group {
		units[k] = JoinedUnit{ID: s.ID, Source: s.Source}
		whole, _ := fold(units[:k+1])
		ends[k] = len(whole.Plain())
		starts[k] = ends[k] - len(s.Source.Plain())
	}
	whole, _ := fold(units)
	text := whole.Plain()
	usual := map[int]bool{}
	for _, b := range without.Breaks(text) {
		usual[b] = true
	}
	ruled := map[int]bool{}
	for _, b := range seg.Breaks(text) {
		ruled[b] = true
	}
	// kept: a sentence still ends between segment k and the next.
	// inside: a sentence now ends inside segment k, at these places.
	kept := make([]bool, len(group))
	inside := make([][]int, len(group))
	for k := range group {
		kept[k] = true
		if k+1 < len(group) {
			was, is := false, false
			for b := ends[k]; b <= starts[k+1]; b++ {
				was = was || usual[b]
				is = is || ruled[b]
			}
			kept[k] = is || !was
		}
		for b := starts[k] + 1; b < ends[k]; b++ {
			if ruled[b] && !usual[b] {
				inside[k] = append(inside[k], b-starts[k])
			}
		}
	}
	var out []Segment
	for k := 0; k < len(group); {
		// Segments no sentence ends between are one.
		n := k
		for n+1 < len(group) && !kept[n] {
			n++
		}
		parts := append([]Segment{}, group[k:n+1]...)
		for len(parts) > 1 {
			joined, err := Join(parts, 0)
			if err != nil {
				break
			}
			parts = joined
		}
		if len(parts) == 1 {
			// Cut from the end, so the places before stay where they are.
			var places []int
			for m := k; m <= n; m++ {
				for _, at := range inside[m] {
					places = append(places, starts[m]+at-starts[k])
				}
			}
			for c := len(places) - 1; c >= 0; c-- {
				if cutUp, err := Split(parts, 0, parts[0].Source.Slice(0, places[c])); err == nil {
					parts = cutUp
				}
			}
		}
		out = append(out, parts...)
		k = n + 1
	}
	return out
}
