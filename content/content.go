// Package content defines the segment content model shared by every
// Interlinea component: plain text interleaved with inline tags.
//
// Inline tags stand for formatting or structure in the original file
// (bold runs, links, fields, line breaks). They are kept as atomic
// elements so translation memory, QA and the editor can treat them
// independently of the translatable text.
package content

import (
	"encoding/json"
	"fmt"
	"strings"
)

// TagKind tells whether a tag opens a span, closes it, or stands alone.
type TagKind uint8

const (
	// TagOpen starts a span, for example the start of a bold run.
	TagOpen TagKind = iota + 1
	// TagClose ends a span opened by a TagOpen with the same ID.
	TagClose
	// TagStandalone is a self-contained placeholder such as a line break or field.
	TagStandalone
)

// String returns the lower-case name of the kind.
func (k TagKind) String() string {
	switch k {
	case TagOpen:
		return "open"
	case TagClose:
		return "close"
	case TagStandalone:
		return "standalone"
	default:
		return fmt.Sprintf("TagKind(%d)", uint8(k))
	}
}

// MarshalText implements encoding.TextMarshaler.
func (k TagKind) MarshalText() ([]byte, error) {
	switch k {
	case TagOpen, TagClose, TagStandalone:
		return []byte(k.String()), nil
	default:
		return nil, fmt.Errorf("content: invalid tag kind %d", uint8(k))
	}
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (k *TagKind) UnmarshalText(b []byte) error {
	switch string(b) {
	case "open":
		*k = TagOpen
	case "close":
		*k = TagClose
	case "standalone":
		*k = TagStandalone
	default:
		return fmt.Errorf("content: unknown tag kind %q", string(b))
	}
	return nil
}

// Tag is an inline element. ID pairs open and close tags inside a segment.
// Type is a format-neutral hint ("bold", "link", "lb"). Desc describes the
// tag for people ("Colour #C00000, Bold"). Native holds the original markup
// so the file can be rebuilt exactly.
type Tag struct {
	Kind   TagKind `json:"kind"`
	ID     string  `json:"id"`
	Type   string  `json:"type,omitempty"`
	Desc   string  `json:"desc,omitempty"`
	Native string  `json:"native,omitempty"`
}

// Run is one element of a segment: either text or a tag, never both.
type Run struct {
	Text string `json:"text,omitempty"`
	Tag  *Tag   `json:"tag,omitempty"`
}

// IsTag reports whether the run is a tag.
func (r Run) IsTag() bool { return r.Tag != nil }

// Content is an ordered sequence of runs.
type Content []Run

// Text returns content holding a single text run. Empty input yields empty content.
func Text(s string) Content {
	if s == "" {
		return Content{}
	}
	return Content{{Text: s}}
}

// Append adds text, merging it into a trailing text run.
func (c Content) Append(s string) Content {
	if s == "" {
		return c
	}
	if n := len(c); n > 0 && !c[n-1].IsTag() {
		c[n-1].Text += s
		return c
	}
	return append(c, Run{Text: s})
}

// AppendTag adds a tag run. The tag is copied.
func (c Content) AppendTag(t Tag) Content {
	tc := t
	return append(c, Run{Tag: &tc})
}

// Plain returns the text without tags.
func (c Content) Plain() string {
	var b strings.Builder
	for _, r := range c {
		if !r.IsTag() {
			b.WriteString(r.Text)
		}
	}
	return b.String()
}

// Tags returns copies of all tags in order.
func (c Content) Tags() []Tag {
	var out []Tag
	for _, r := range c {
		if r.IsTag() {
			out = append(out, *r.Tag)
		}
	}
	return out
}

// HasTags reports whether the content contains at least one tag.
func (c Content) HasTags() bool {
	for _, r := range c {
		if r.IsTag() {
			return true
		}
	}
	return false
}

// Normalize merges adjacent text runs and removes empty ones.
func (c Content) Normalize() Content {
	out := make(Content, 0, len(c))
	for _, r := range c {
		if r.IsTag() {
			t := *r.Tag
			out = append(out, Run{Tag: &t})
			continue
		}
		out = out.Append(r.Text)
	}
	return out
}

// Equal reports whether two contents have the same text and the same tags
// (compared by kind, ID and type; native code is ignored).
func (c Content) Equal(o Content) bool {
	a, b := c.Normalize(), o.Normalize()
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		ra, rb := a[i], b[i]
		if ra.IsTag() != rb.IsTag() {
			return false
		}
		if !ra.IsTag() {
			if ra.Text != rb.Text {
				return false
			}
			continue
		}
		if ra.Tag.Kind != rb.Tag.Kind || ra.Tag.ID != rb.Tag.ID || ra.Tag.Type != rb.Tag.Type {
			return false
		}
	}
	return true
}

// Clone returns a deep copy.
func (c Content) Clone() Content {
	out := make(Content, len(c))
	for i, r := range c {
		if r.IsTag() {
			t := *r.Tag
			out[i] = Run{Tag: &t}
		} else {
			out[i] = Run{Text: r.Text}
		}
	}
	return out
}

// Validate checks structural rules: runs carry text or a tag (not both, not
// neither), tags have an ID and a valid kind, and no two open tags share an ID.
// Close tags without a matching open tag are allowed, because a segment may
// end or start inside a span that crosses segment boundaries.
func (c Content) Validate() error {
	open := map[string]bool{}
	for i, r := range c {
		switch {
		case r.IsTag() && r.Text != "":
			return fmt.Errorf("content: run %d has both text and tag", i)
		case !r.IsTag() && r.Text == "":
			return fmt.Errorf("content: run %d is empty", i)
		case r.IsTag():
			t := r.Tag
			if t.ID == "" {
				return fmt.Errorf("content: tag at run %d has no id", i)
			}
			switch t.Kind {
			case TagOpen:
				if open[t.ID] {
					return fmt.Errorf("content: duplicate open tag id %q", t.ID)
				}
				open[t.ID] = true
			case TagClose, TagStandalone:
			default:
				return fmt.Errorf("content: tag %q has invalid kind %d", t.ID, uint8(t.Kind))
			}
		}
	}
	return nil
}

// MarshalJSON encodes the content as a JSON array of runs.
func (c Content) MarshalJSON() ([]byte, error) {
	if c == nil {
		return []byte("[]"), nil
	}
	return json.Marshal([]Run(c))
}

// UnmarshalJSON decodes a JSON array of runs and validates the result.
func (c *Content) UnmarshalJSON(b []byte) error {
	var runs []Run
	if err := json.Unmarshal(b, &runs); err != nil {
		return err
	}
	out := Content(runs)
	if err := out.Validate(); err != nil {
		return err
	}
	*c = out
	return nil
}

// Signature returns a compact, comparable key of the tag structure, used to
// tell segments apart that have equal text but different tags.
func (c Content) Signature() string {
	var b strings.Builder
	for _, r := range c {
		if !r.IsTag() {
			continue
		}
		switch r.Tag.Kind {
		case TagOpen:
			b.WriteByte('<')
		case TagClose:
			b.WriteByte('>')
		case TagStandalone:
			b.WriteByte('|')
		}
		b.WriteString(r.Tag.ID)
	}
	return b.String()
}

// String renders the content for debugging: tags appear as {id}, {/id} and {id/}.
func (c Content) String() string {
	var b strings.Builder
	for _, r := range c {
		if !r.IsTag() {
			b.WriteString(r.Text)
			continue
		}
		switch r.Tag.Kind {
		case TagOpen:
			fmt.Fprintf(&b, "{%s}", r.Tag.ID)
		case TagClose:
			fmt.Fprintf(&b, "{/%s}", r.Tag.ID)
		default:
			fmt.Fprintf(&b, "{%s/}", r.Tag.ID)
		}
	}
	return b.String()
}

// Slice returns the part of c between the plain-text byte offsets start and
// end (as returned by Plain). Tags located exactly at a boundary are assigned
// by kind: close tags at start and open tags at end stay outside the slice
// (they belong to the neighbouring part); standalone tags at start belong to
// the slice, at end they stay outside.
func (c Content) Slice(start, end int) Content {
	if start < 0 {
		start = 0
	}
	var out Content
	pos := 0
	for _, r := range c {
		if r.IsTag() {
			switch {
			case pos > start && pos < end:
				out = out.AppendTag(*r.Tag)
			case pos == start && pos < end && r.Tag.Kind != TagClose:
				out = out.AppendTag(*r.Tag)
			case pos == end && pos > start && r.Tag.Kind == TagClose:
				out = out.AppendTag(*r.Tag)
			}
			continue
		}
		rs, re := pos, pos+len(r.Text)
		pos = re
		lo, hi := max(rs, start), min(re, end)
		if lo < hi {
			out = out.Append(r.Text[lo-rs : hi-rs])
		}
	}
	if out == nil {
		out = Content{}
	}
	return out
}
