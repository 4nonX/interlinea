package tm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/4nonX/interlinea/langtag"
	"github.com/4nonX/interlinea/tmx"
)

// ImportOptions controls TMX import.
type ImportOptions struct {
	// Policy is applied to every imported unit.
	Policy AddPolicy
	// AllowPrimaryMatch accepts variants whose language matches only by
	// primary subtag (de-AT for a de-DE memory) when no exact variant exists.
	AllowPrimaryMatch bool
}

// ImportStats reports the outcome of an import.
type ImportStats struct {
	Read    int // units read from the file
	Stored  int // units added or updated
	Skipped int // units without usable source and target variants
}

const (
	attrTUID        = "x-tmx-tuid"
	importBatchSize = 1000
)

// ImportTMX reads a TMX document into m, picking the variants for the
// memory's source and target languages.
func ImportTMX(ctx context.Context, m Memory, r io.Reader, opts ImportOptions) (ImportStats, error) {
	var st ImportStats
	rd, err := tmx.NewReader(r)
	if err != nil {
		return st, err
	}
	info := m.Info()
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
	accept := func(q langtag.Quality) bool {
		return q == langtag.ExactMatch || (opts.AllowPrimaryMatch && q == langtag.PrimaryMatch)
	}
	for {
		if err := ctx.Err(); err != nil {
			return st, err
		}
		tu, err := rd.Next()
		if errors.Is(err, io.EOF) {
			if canBatch {
				if err := flush(); err != nil {
					return st, fmt.Errorf("tm: import: %w", err)
				}
			}
			return st, nil
		}
		if err != nil {
			return st, err
		}
		st.Read++
		src, qs := tu.Variant(info.SourceLang)
		tgt, qt := tu.Variant(info.TargetLang)
		if src == nil || tgt == nil || !accept(qs) || !accept(qt) || src == tgt {
			st.Skipped++
			continue
		}
		u := Unit{
			Source:     src.Content,
			Target:     tgt.Content,
			CreatedAt:  firstSet(tgt.CreationDate, tu.CreationDate),
			CreatedBy:  firstStr(tgt.CreationID, tu.CreationID),
			ChangedAt:  firstSet(tgt.ChangeDate, tu.ChangeDate),
			ChangedBy:  firstStr(tgt.ChangeID, tu.ChangeID),
			UsageCount: tu.UsageCount,
			Notes:      append(append([]string(nil), tu.Notes...), tgt.Notes...),
		}
		if tu.ID != "" {
			u.Attributes = append(u.Attributes, Attribute{Name: attrTUID, Value: tu.ID})
		}
		for _, p := range tu.Props {
			u.Attributes = append(u.Attributes, Attribute{Name: p.Type, Value: p.Value})
		}
		if canBatch {
			if err := ValidateUnit(u); err != nil {
				if errors.Is(err, ErrEmptySource) || errors.Is(err, ErrEmptyTarget) {
					st.Skipped++
					continue
				}
				return st, fmt.Errorf("tm: import unit %q: %w", tu.ID, err)
			}
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
			if errors.Is(err, ErrEmptySource) || errors.Is(err, ErrEmptyTarget) {
				st.Skipped++
				continue
			}
			return st, fmt.Errorf("tm: import unit %q: %w", tu.ID, err)
		}
		st.Stored++
	}
}

// ExportTMX writes every unit of m as a TMX 1.4b document.
func ExportTMX(ctx context.Context, m Memory, w io.Writer, toolVersion string) error {
	info := m.Info()
	tw, err := tmx.NewWriter(w, tmx.Header{
		CreationTool:        "Interlinea",
		CreationToolVersion: toolVersion,
		SrcLang:             info.SourceLang,
		Props:               []tmx.Prop{{Type: "x-memory-name", Value: info.Name}},
	})
	if err != nil {
		return err
	}
	err = m.Each(ctx, func(u Unit) error {
		tu := &tmx.Unit{
			ID:           strconv.FormatInt(u.ID, 10),
			CreationDate: u.CreatedAt,
			CreationID:   u.CreatedBy,
			ChangeDate:   u.ChangedAt,
			ChangeID:     u.ChangedBy,
			UsageCount:   u.UsageCount,
			Notes:        u.Notes,
			Variants: []tmx.Variant{
				{Lang: info.SourceLang, Content: u.Source},
				{Lang: info.TargetLang, Content: u.Target},
			},
		}
		for _, a := range u.Attributes {
			if a.Name == attrTUID {
				continue
			}
			typ := a.Name
			if !strings.HasPrefix(typ, "x-") {
				typ = "x-" + typ
			}
			tu.Props = append(tu.Props, tmx.Prop{Type: typ, Value: a.Value})
		}
		return tw.Write(tu)
	})
	if err != nil {
		return err
	}
	return tw.Close()
}

func firstSet[T interface{ IsZero() bool }](a, b T) T {
	if a.IsZero() {
		return b
	}
	return a
}

func firstStr(a, b string) string {
	if a == "" {
		return b
	}
	return a
}
