package docx

import (
	"archive/zip"
	"bytes"
	"fmt"

	"github.com/4nonX/interlinea/filter"
	"github.com/4nonX/interlinea/segment"
)

// PartRef names a part of an Office package with DrawingML text: a
// drawing with text boxes and shapes, or a chart.
type PartRef struct {
	Name string
	Kind filter.Part
}

// Parts is the DrawingML text of some parts of an Office package, for
// filters of formats that keep the rest of their text another way (a
// workbook's cells).
type Parts struct {
	d *document
}

// ExtractParts reads the text of the given parts of the package: text
// boxes, shapes and the titles of charts. The names a chart shows from its
// data are not read, since they are the data's.
func ExtractParts(zr *zip.Reader, lang string, refs []PartRef) *Parts {
	d := &document{zr: zr, seg: segment.New(nil, lang), sigs: map[string]*para{}, noLabels: true}
	for _, r := range refs {
		b, err := readFile(zr, r.Name)
		if err != nil {
			continue // a missing part is left as it is
		}
		p, err := d.parsePart(r.Name, r.Kind, b, false)
		if err != nil {
			d.ext.Warnings = append(d.ext.Warnings, fmt.Sprintf("%s could not be read and is kept unchanged: %v", r.Name, err))
			continue
		}
		d.parts = append(d.parts, p)
	}
	d.resolveMirrors()
	return &Parts{d}
}

// Extraction is the units and blocks of the parts, in the order given.
func (p *Parts) Extraction() *filter.Extraction { return &p.d.ext }

// Render returns the parts that have units, rewritten with their
// translations, by name.
func (p *Parts) Render(tr filter.Translations, targetLang string) map[string][]byte {
	changed := map[string][]byte{}
	for _, part := range p.d.parts {
		if part.hasUnits() {
			changed[part.file] = part.render(p.d, tr, targetLang)
		}
	}
	return changed
}

// ReadPart returns a part of the package.
func ReadPart(zr *zip.Reader, name string) ([]byte, error) { return readFile(zr, name) }

// Rel is a relationship of a part.
type Rel struct{ ID, Type, Target string }

// Rels returns the relationships of a part ("" for the package), with
// their targets as package paths. External targets are left out.
func Rels(zr *zip.Reader, part string) ([]Rel, error) {
	rs, err := readRels(zr, part)
	if err != nil {
		return nil, err
	}
	out := make([]Rel, len(rs))
	for i, r := range rs {
		out[i] = Rel{r.id, r.typ, r.target}
	}
	return out, nil
}

// OpenPackage opens an Office package.
func OpenPackage(data []byte) (*zip.Reader, error) {
	return zip.NewReader(bytes.NewReader(data), int64(len(data)))
}

// RewritePackage returns the package with the changed parts replaced.
// Everything else is copied as it is.
func RewritePackage(zr *zip.Reader, changed map[string][]byte) ([]byte, error) {
	return rewriteZip(zr, changed)
}
