package project

import (
	"encoding/json"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter"
	"github.com/4nonX/interlinea/filter/xliff"
)

// FileStateOf is the state a segment has in its bilingual file when it is
// written back: its status here, and the file's own finer state where
// the translation is still the file's.
func FileStateOf(s Segment) filter.State {
	unchanged := false
	if s.File != nil {
		js, _ := json.Marshal(s.Target)
		unchanged = string(js) == s.File.Target
	}
	switch s.Status {
	case StatusConfirmed:
		if unchanged && (s.File.State == string(filter.StateReviewed) || s.File.State == string(filter.StateFinal)) {
			return filter.State(s.File.State)
		}
		return filter.StateTranslated
	case StatusDraft:
		if unchanged && s.File.State == string(filter.StateRejected) {
			return filter.StateRejected
		}
		return filter.StateDraft
	}
	if !blank(s.Target) {
		return filter.StateDraft
	}
	return filter.StateNew
}

// XLIFFUpdates is what the XLIFF filter takes for a file's segments:
// each segment's translation and state under its ID. A stretch the
// translator cut differently is written as the file has its segments:
// the translations of its parts one after the other under its first
// segment, in the least advanced state of the parts, and the segments
// joined to that one empty, in the same state. Locked segments are left
// out and stay as they are.
func XLIFFUpdates(segs []Segment) map[string]xliff.Update {
	return XLIFFUpdatesBy(segs, FileStateOf)
}

// XLIFFUpdatesBy is XLIFFUpdates with the state of each segment as the
// caller knows it.
func XLIFFUpdatesBy(segs []Segment, stateOf func(Segment) filter.State) map[string]xliff.Update {
	up := map[string]xliff.Update{}
	for _, st := range Stretches(segs) {
		if len(st.Segments) == 1 && len(st.Units) == 1 {
			if s := st.Segments[0]; !s.Locked {
				up[s.ID] = xliff.Update{Target: s.Target, State: stateOf(s)}
			}
			continue
		}
		state := filter.StateFinal
		for _, s := range st.Segments {
			if now := stateOf(s); now.Rank() < state.Rank() {
				state = now
			}
		}
		target := content.Content{}
		if st.Translated() {
			target = st.Target(false)
			// Translated in part, the stretch is a draft, not untouched.
			if state.Rank() == 0 {
				state = filter.StateDraft
			}
		}
		up[st.Units[0]] = xliff.Update{Target: target, State: state, Joined: st.Units[1:]}
		for _, id := range st.Units[1:] {
			up[id] = xliff.Update{Target: content.Content{}, State: state}
		}
	}
	return up
}
