// Package filter defines what file filters hand to the rest of Interlinea:
// translatable units with inline tags, plus the document structure they sit
// in. A filter reads an original file, extracts the units and rebuilds the
// file from translations, keeping everything it did not extract unchanged.
package filter

import "github.com/4nonX/interlinea/content"

// Part names the area of a document a block belongs to.
type Part string

const (
	PartBody     Part = "body"
	PartFootnote Part = "footnote"
	PartEndnote  Part = "endnote"
	PartHeader   Part = "header"
	PartFooter   Part = "footer"
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
	// Note is the footnote or endnote number for blocks in those parts.
	Note string `json:"note,omitempty"`
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
}

// Extraction is the translatable content of a file in document order.
type Extraction struct {
	Units  []Unit  `json:"units"`
	Blocks []Block `json:"blocks"`
}

// Translations maps unit IDs to target content. Units without an entry keep
// their source text, as in other CAT tools.
type Translations map[string]content.Content
