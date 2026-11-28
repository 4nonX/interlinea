// Package filter defines what file filters hand to the rest of Interlinea:
// translatable units with inline tags, plus the document structure they sit
// in. A filter reads an original file, extracts the units and rebuilds the
// file from translations, keeping everything it did not extract unchanged.
package filter

import (
	"strconv"

	"github.com/4nonX/interlinea/content"
)

// Part names the area of a document a block belongs to.
type Part string

const (
	PartBody     Part = "body"
	PartFootnote Part = "footnote"
	PartEndnote  Part = "endnote"
	PartHeader   Part = "header"
	PartFooter   Part = "footer"
	PartComment  Part = "comment"
	PartChart    Part = "chart"
	PartDiagram  Part = "diagram"
	// PartNotes is the speaker notes of a slide.
	PartNotes Part = "notes"
	// PartAttribute is text kept in an attribute of the markup, such as
	// the alternative text of a picture in a web page.
	PartAttribute Part = "attribute"
	// PartMeta is text about the document, such as a web page's title.
	PartMeta Part = "meta"
)

// Cell locates a block inside a table: the table's number in its part (from
// zero, outermost tables only), and the row and column within it.
type Cell struct {
	Table int `json:"table"`
	Row   int `json:"row"`
	Col   int `json:"col"`
}

// Block is a paragraph of the original document. Units refer to their block,
// so previews and checks can show them in context.
type Block struct {
	ID string `json:"id"`
	// Part is the document area; File is the file inside the package.
	Part Part   `json:"part"`
	File string `json:"file"`
	// Style is the paragraph style ID and StyleName its display name.
	Style     string `json:"style,omitempty"`
	StyleName string `json:"styleName,omitempty"`
	// Heading is the outline level from 1, or 0 for body text.
	Heading int `json:"heading,omitempty"`
	// List is true for numbered or bulleted paragraphs.
	List bool `json:"list,omitempty"`
	// Align is the paragraph alignment as written in the file ("center", "right", "both").
	Align string `json:"align,omitempty"`
	Cell  *Cell  `json:"cell,omitempty"`
	// Note is the footnote, endnote or comment number for blocks in those parts.
	Note string `json:"note,omitempty"`
	// Frame is "textbox" for text in a text box or shape.
	Frame string `json:"frame,omitempty"`
	// Revisions is true when the paragraph contains tracked changes.
	Revisions bool `json:"revisions,omitempty"`
	// Lead and Trail list the types of tags kept at the paragraph edges,
	// outside the units (a footnote mark at the end, a link around the
	// whole paragraph), so previews can show them.
	Lead  []string `json:"lead,omitempty"`
	Trail []string `json:"trail,omitempty"`
}

// Unit is one translatable segment. Tag IDs are local to the unit; tag
// Native holds the original markup, so translations only need kind and ID.
type Unit struct {
	ID     string          `json:"id"`
	Block  string          `json:"block"`
	Source content.Content `json:"source"`

	// The fields below come from bilingual files (XLIFF, SDLXLIFF,
	// MQXLIFF), which carry translations and their state.

	// Target is the translation in the file, if any.
	Target content.Content `json:"target,omitempty"`
	// State is how far the translation is ("" for monolingual files).
	State State `json:"state,omitempty"`
	// Locked units must not be changed (locked in Trados or memoQ).
	Locked bool `json:"locked,omitempty"`
	// Match is the match percentage the file records for the translation
	// (100, 101 for a context match), Origin where it came from ("tm",
	// "mt", "interactive").
	Match  int    `json:"match,omitempty"`
	Origin string `json:"origin,omitempty"`
	// Key is the unit's name in the file (resname), Notes its comments.
	Key   string   `json:"key,omitempty"`
	Notes []string `json:"notes,omitempty"`
	// MaxLength is the longest the translation may be, in characters
	// (0 for no limit).
	MaxLength int `json:"maxLength,omitempty"`
	// Timing is when a subtitle is shown; nil for anything else.
	Timing *Timing `json:"timing,omitempty"`
}

// Timing is when a subtitle is shown, in milliseconds from the start. A
// subtitle whose times the file does not say (frames without a frame
// rate) has End no later than Start.
type Timing struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
	// LineLimit is the most characters a line of the format holds (32 in
	// closed captions); 0 where the format sets none.
	LineLimit int `json:"lineLimit,omitempty"`
}

// Duration is how long the subtitle is shown, in milliseconds; 0 where
// the file does not say.
func (t *Timing) Duration() int64 {
	if t == nil || t.End <= t.Start {
		return 0
	}
	return t.End - t.Start
}

// State is how far the translation of a unit is, in the terms Interlinea
// shows; bilingual filters map their format's states to these and back.
type State string

const (
	// StateNew: not translated yet.
	StateNew State = "new"
	// StateDraft: a translation exists but is not confirmed (Trados Draft,
	// memoQ pre-translated or edited).
	StateDraft State = "draft"
	// StateTranslated: confirmed by the translator.
	StateTranslated State = "translated"
	// StateReviewed: approved by a reviewer (Trados Approved Translation,
	// memoQ Reviewer 1 confirmed).
	StateReviewed State = "reviewed"
	// StateFinal: signed off (Trados Approved Sign-off, memoQ proofread).
	StateFinal State = "final"
	// StateRejected: rejected by a reviewer.
	StateRejected State = "rejected"
)

// Rank orders states from new to final; rejected counts as a draft.
func (s State) Rank() int {
	switch s {
	case StateDraft, StateRejected:
		return 1
	case StateTranslated:
		return 2
	case StateReviewed:
		return 3
	case StateFinal:
		return 4
	default:
		return 0
	}
}

// Valid reports whether s is one of the states.
func (s State) Valid() bool {
	switch s {
	case StateNew, StateDraft, StateTranslated, StateReviewed, StateFinal, StateRejected:
		return true
	}
	return false
}

// Extraction is the translatable content of a file in document order.
type Extraction struct {
	Units  []Unit  `json:"units"`
	Blocks []Block `json:"blocks"`
	// Warnings tell the user about things the filter changes or cannot
	// translate (tracked changes that will be accepted, for example).
	Warnings []string `json:"warnings,omitempty"`
	// Format names the file format and its variant ("docx", "xliff-1.2",
	// "sdlxliff", "mqxliff", "mxliff", "txml", "ttx", "xliff-2.0"); SourceLang and TargetLang are
	// the languages a bilingual file declares.
	Format     string `json:"format,omitempty"`
	SourceLang string `json:"sourceLang,omitempty"`
	TargetLang string `json:"targetLang,omitempty"`
}

// Translations maps unit IDs to target content. Units without an entry keep
// their source text, as in other CAT tools.
type Translations map[string]content.Content

// joinedType is the type of the tag that marks a unit as joined to the
// one before it.
const joinedType = "interlinea:joined"

// Joined is the translation of a unit that was joined to the unit before
// it: the two are translated as one, and the translation stands under the
// first unit. The first unit's translation uses the tag IDs JoinTags
// gives. The unit before may be in an earlier paragraph, as where the
// translation puts in one paragraph what the source has in two: a unit
// that starts its paragraph joined is written without its text there,
// and so is what stands between it and the next unit that is not joined.
// A filter that can write the unit's tags where the translation is (Word)
// does so; others keep the unit's placeholders (a picture, a field) in
// its own paragraph.
func Joined() content.Content {
	return content.Content{{Tag: &content.Tag{Kind: content.TagStandalone, ID: "0", Type: joinedType}}}
}

// IsJoined reports whether c is the mark Joined returns.
func IsJoined(c content.Content) bool {
	return len(c) == 1 && c[0].IsTag() && c[0].Tag.Type == joinedType
}

// JoinTags gives the tags of b, a unit joined to a, the IDs they have in
// the joined unit: the whole numbers after the highest of a's, in the
// order b's tags first appear. It returns b with those IDs and the map
// from them to b's own.
func JoinTags(a, b content.Content) (content.Content, map[string]string) {
	top := 0
	for _, r := range a {
		if r.IsTag() {
			if n, err := strconv.Atoi(r.Tag.ID); err == nil && n > top {
				top = n
			}
		}
	}
	ids, back := map[string]string{}, map[string]string{}
	out := b.Clone()
	for i := range out {
		if !out[i].IsTag() {
			continue
		}
		id, ok := ids[out[i].Tag.ID]
		if !ok {
			top++
			id = strconv.Itoa(top)
			ids[out[i].Tag.ID] = id
			back[id] = out[i].Tag.ID
		}
		out[i].Tag.ID = id
	}
	return out, back
}
