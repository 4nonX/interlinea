package xliff

import (
	"strings"

	"github.com/4nonX/interlinea/filter"
)

// XLIFF 1.2: the state attribute of <target>.
func from12(state string, approved, hasText bool) filter.State {
	switch strings.ToLower(state) {
	case "new", "needs-translation":
		if hasText {
			return filter.StateDraft
		}
		return filter.StateNew
	case "needs-l10n", "needs-adaptation":
		return filter.StateDraft
	case "translated", "needs-review-translation", "needs-review-l10n", "needs-review-adaptation":
		return filter.StateTranslated
	case "signed-off":
		return filter.StateReviewed
	case "final":
		return filter.StateFinal
	case "x-rejected":
		return filter.StateRejected
	}
	switch {
	case approved:
		return filter.StateFinal
	case hasText:
		return filter.StateTranslated
	}
	return filter.StateNew
}

func to12(s filter.State) string {
	switch s {
	case filter.StateDraft:
		return "needs-translation"
	case filter.StateTranslated:
		return "translated"
	case filter.StateReviewed:
		return "signed-off"
	case filter.StateFinal:
		return "final"
	case filter.StateRejected:
		return "x-rejected"
	}
	return "new"
}

// SDLXLIFF: the conf attribute of sdl:seg (Trados confirmation level).
// No conf means Not Translated.
func fromSDL(conf string) filter.State {
	switch conf {
	case "Draft":
		return filter.StateDraft
	case "Translated":
		return filter.StateTranslated
	case "ApprovedTranslation":
		return filter.StateReviewed
	case "ApprovedSignOff":
		return filter.StateFinal
	case "RejectedTranslation", "RejectedSignOff":
		return filter.StateRejected
	}
	return filter.StateNew
}

// toSDL returns the conf value for a state ("" for none); a rejection
// keeps its stage (translation or sign-off).
func toSDL(s filter.State, current string) string {
	switch s {
	case filter.StateDraft:
		return "Draft"
	case filter.StateTranslated:
		return "Translated"
	case filter.StateReviewed:
		return "ApprovedTranslation"
	case filter.StateFinal:
		return "ApprovedSignOff"
	case filter.StateRejected:
		if current == "RejectedSignOff" {
			return current
		}
		return "RejectedTranslation"
	}
	return ""
}

// MQXLIFF: the mq:status attribute of <trans-unit>.
func fromMQ(status string, hasText bool) filter.State {
	switch status {
	case "NotStarted":
		return filter.StateNew
	case "PreTranslated", "PartiallyEdited", "Edited":
		return filter.StateDraft
	case "ManuallyConfirmed", "Translated":
		return filter.StateTranslated
	case "Reviewer1Confirmed":
		return filter.StateReviewed
	case "Proofread", "Reviewer2Confirmed":
		return filter.StateFinal
	case "Rejected":
		return filter.StateRejected
	}
	if hasText {
		return filter.StateDraft
	}
	return filter.StateNew
}

// toMQ returns the memoQ status for a state; a pre-translation whose text
// did not change stays pre-translated.
func toMQ(s filter.State, current string, changed bool) string {
	switch s {
	case filter.StateDraft:
		if current == "PreTranslated" && !changed {
			return current
		}
		return "PartiallyEdited"
	case filter.StateTranslated:
		return "ManuallyConfirmed"
	case filter.StateReviewed:
		return "Reviewer1Confirmed"
	case filter.StateFinal:
		return "Proofread"
	case filter.StateRejected:
		return "Rejected"
	}
	return "NotStarted"
}

// XLIFF 2: the state and subState attributes of <segment>. Drafts and
// rejections, which XLIFF 2 counts as initial, are told apart by a
// subState in Interlinea's namespace prefix.
const (
	subDraft    = "interlinea:draft"
	subRejected = "interlinea:rejected"
)

func from20(state, sub string, hasText bool) filter.State {
	switch state {
	case "translated":
		return filter.StateTranslated
	case "reviewed":
		return filter.StateReviewed
	case "final":
		return filter.StateFinal
	}
	switch sub {
	case subDraft:
		return filter.StateDraft
	case subRejected:
		return filter.StateRejected
	}
	if hasText {
		return filter.StateDraft
	}
	return filter.StateNew
}

func to20(s filter.State) (state, sub string) {
	switch s {
	case filter.StateDraft:
		return "initial", subDraft
	case filter.StateRejected:
		return "initial", subRejected
	case filter.StateTranslated:
		return "translated", ""
	case filter.StateReviewed:
		return "reviewed", ""
	case filter.StateFinal:
		return "final", ""
	}
	return "initial", ""
}
