// Package tmx reads and writes TMX 1.4b (Translation Memory eXchange), the
// interchange format every CAT tool can import and export.
//
// Reading is streaming, so multi-gigabyte memories never have to fit in
// memory. Inline markup (bpt, ept, ph, it, hi, ut) is mapped to the
// content model's tags with the original code kept as native text.
package tmx

import (
	"time"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/langtag"
)

// DateLayout is the TMX date format (ISO 8601 basic, UTC).
const DateLayout = "20060102T150405Z"

// Prop is a TMX <prop> element: a typed, tool-specific property.
type Prop struct {
	Type  string
	Value string
}

// Header holds the TMX header attributes.
type Header struct {
	CreationTool        string
	CreationToolVersion string
	SegType             string
	OTMF                string
	AdminLang           string
	SrcLang             string
	DataType            string
	CreationDate        time.Time
	CreationID          string
	Props               []Prop
	Notes               []string
}

// Variant is one language version of a unit (<tuv>).
type Variant struct {
	Lang         string
	Content      content.Content
	CreationDate time.Time
	CreationID   string
	ChangeDate   time.Time
	ChangeID     string
	Props        []Prop
	Notes        []string
}

// Unit is a translation unit (<tu>).
type Unit struct {
	ID           string
	SrcLang      string
	CreationDate time.Time
	CreationID   string
	ChangeDate   time.Time
	ChangeID     string
	UsageCount   int
	LastUsage    time.Time
	Props        []Prop
	Notes        []string
	Variants     []Variant
}

// Variant returns the variant that best matches lang: an exact match if
// present, otherwise the first variant with the same primary language.
func (u *Unit) Variant(lang string) (*Variant, langtag.Quality) {
	var best *Variant
	bestQ := langtag.NoMatch
	for i := range u.Variants {
		q := langtag.Match(lang, u.Variants[i].Lang)
		if q > bestQ {
			best, bestQ = &u.Variants[i], q
			if q == langtag.ExactMatch {
				break
			}
		}
	}
	return best, bestQ
}

func parseDate(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse(DateLayout, s); err == nil {
		return t
	}
	// Some tools omit the trailing Z or use extended ISO 8601.
	for _, layout := range []string{"20060102T150405", time.RFC3339, "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

func formatDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(DateLayout)
}
