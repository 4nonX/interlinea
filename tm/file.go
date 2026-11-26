package tm

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/4nonX/interlinea/internal/sqlitefile"
	"github.com/4nonX/interlinea/langtag"
	"github.com/4nonX/interlinea/tmx"
)

// The formats of translation memory files that are imported.
const (
	FormatTMX      = "tmx"
	FormatSDLTM    = "sdltm"
	FormatWordfast = "wordfast"
)

// File is a translation memory read from the file of another tool.
type File struct {
	Format string `json:"format"`
	// Name, languages and description as the file gives them, where it
	// does: a Trados memory names itself, a TMX file only its languages.
	Name        string     `json:"name,omitempty"`
	SourceLang  string     `json:"sourceLang,omitempty"`
	TargetLang  string     `json:"targetLang,omitempty"`
	Description string     `json:"description,omitempty"`
	Units       []FileUnit `json:"-"`
	// Count is the number of units in the file, read or not.
	Count int `json:"units"`
	// Skipped counts units that could not be read at all.
	Skipped  int      `json:"skipped"`
	Warnings []string `json:"warnings,omitempty"`
}

// FileUnit is a unit with the languages of its source and target.
type FileUnit struct {
	Unit
	SourceLang, TargetLang string
}

// FormatOf tells a memory file by its content: a Trados Studio memory, a
// Wordfast memory, or else TMX.
func FormatOf(data []byte) string {
	switch {
	case sqlitefile.Is(data):
		return FormatSDLTM
	case IsWordfastTM(data):
		return FormatWordfast
	}
	return FormatTMX
}

// FormatName names a memory format for people.
func FormatName(format string) string {
	switch format {
	case FormatSDLTM:
		return ".sdltm memory"
	case FormatWordfast:
		return ".txt memory"
	}
	return "TMX"
}

// ReadFile reads a memory file of any of the formats. Of a TMX file it
// reads the languages (of its header and first units) and counts the
// units, which is what a new memory needs; Import reads its units.
func ReadFile(data []byte) (*File, error) {
	var f *File
	var err error
	switch FormatOf(data) {
	case FormatSDLTM:
		f, err = ReadSDLTM(data)
	case FormatWordfast:
		f, err = ReadWordfast(data)
	default:
		return readTMXFile(data)
	}
	if err != nil {
		return nil, err
	}
	f.Count = len(f.Units) + f.Skipped
	return f, nil
}

func readTMXFile(data []byte) (*File, error) {
	rd, err := tmx.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	f := &File{Format: FormatTMX}
	if l := rd.Header.SrcLang; !strings.EqualFold(l, "*all*") {
		f.SourceLang = l
	}
	for {
		tu, err := rd.Next()
		if errors.Is(err, io.EOF) {
			return f, nil
		}
		if err != nil {
			return nil, err
		}
		f.Count++
		if f.TargetLang != "" {
			continue
		}
		for _, v := range tu.Variants {
			if f.SourceLang == "" {
				f.SourceLang = v.Lang
			} else if langtag.Match(v.Lang, f.SourceLang) == langtag.NoMatch {
				f.TargetLang = v.Lang
				break
			}
		}
	}
}

// Import reads a memory file of any of the formats into m. A Trados or
// Wordfast unit whose languages are the memory's the other way round is
// read the other way round too, as a TMX unit is.
func Import(ctx context.Context, m Memory, data []byte, opts ImportOptions) (ImportStats, error) {
	if FormatOf(data) == FormatTMX {
		return ImportTMX(ctx, m, bytes.NewReader(data), opts)
	}
	f, err := ReadFile(data)
	if err != nil {
		return ImportStats{}, err
	}
	return ImportUnits(ctx, m, f, opts)
}

// ImportUnits stores the units of a file read by ReadFile.
func ImportUnits(ctx context.Context, m Memory, f *File, opts ImportOptions) (ImportStats, error) {
	st := ImportStats{Read: len(f.Units) + f.Skipped, Skipped: f.Skipped}
	info := m.Info()
	accept := func(a, b string) bool {
		q := langtag.Match(a, b)
		return q == langtag.ExactMatch || (opts.AllowPrimaryMatch && q == langtag.PrimaryMatch)
	}
	batcher, canBatch := m.(BatchAdder)
	var batch []Unit
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		err := batcher.AddBatch(ctx, batch, opts.Policy)
		batch = batch[:0]
		return err
	}
	for _, fu := range f.Units {
		if err := ctx.Err(); err != nil {
			return st, err
		}
		u := fu.Unit
		switch {
		case accept(fu.SourceLang, info.SourceLang) && accept(fu.TargetLang, info.TargetLang):
		case accept(fu.SourceLang, info.TargetLang) && accept(fu.TargetLang, info.SourceLang):
			u.Source, u.Target = u.Target, u.Source
		default:
			st.Skipped++
			continue
		}
		if err := ValidateUnit(u); err != nil {
			if errors.Is(err, ErrEmptySource) || errors.Is(err, ErrEmptyTarget) {
				st.Skipped++
				continue
			}
			return st, fmt.Errorf("tm: import: %w", err)
		}
		if canBatch {
			batch = append(batch, u)
			st.Stored++
			if len(batch) >= importBatchSize {
				if err := flush(); err != nil {
					return st, fmt.Errorf("tm: import: %w", err)
				}
			}
			continue
		}
		if _, err := m.Add(ctx, u, opts.Policy); err != nil {
			return st, fmt.Errorf("tm: import: %w", err)
		}
		st.Stored++
	}
	if canBatch {
		if err := flush(); err != nil {
			return st, fmt.Errorf("tm: import: %w", err)
		}
	}
	return st, nil
}

// LanguagesNote says why no unit of f was stored in a memory of the
// languages info gives, when its languages are others; "" otherwise.
func LanguagesNote(f *File, info Info) string {
	if f.SourceLang == "" || f.TargetLang == "" {
		return ""
	}
	ok := func(a, b string) bool { return langtag.Match(a, b) != langtag.NoMatch }
	if (ok(f.SourceLang, info.SourceLang) && ok(f.TargetLang, info.TargetLang)) || (ok(f.SourceLang, info.TargetLang) && ok(f.TargetLang, info.SourceLang)) {
		return ""
	}
	return fmt.Sprintf("The file is for %s to %s; the memory is for %s to %s.", f.SourceLang, f.TargetLang, info.SourceLang, info.TargetLang)
}
