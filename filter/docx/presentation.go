package docx

import (
	"archive/zip"
	"errors"
	"fmt"
	"strings"

	"github.com/4nonX/interlinea/filter"
)

// ErrNotPresentation is returned for files that are not PowerPoint
// presentations.
var ErrNotPresentation = errors.New("pptx: not a PowerPoint presentation")

// ExtractPresentation returns the translatable units of a PowerPoint
// presentation (.pptx): the text of each slide in the order of the show,
// with its tables, charts and SmartArt, followed by the slide's speaker
// notes. lang is the source language; it selects the sentence rules.
//
// Slide masters and layouts are not read: the prompts in their
// placeholders ("Click to add title") are not shown in a slide show.
// Comments are kept as they are.
func ExtractPresentation(data []byte, lang string) (*filter.Extraction, error) {
	d, err := parseAs(data, lang, Options{}, presentation)
	if err != nil {
		return nil, err
	}
	d.ext.Format = "pptx"
	d.ext.SourceLang = lang
	return &d.ext, nil
}

// MergePresentation returns a copy of the presentation with the units
// replaced by their translations. lang must be the language used for
// ExtractPresentation; targetLang is written to the rewritten text, and a
// paragraph translated into a language of the other direction is turned
// around with its alignment.
func MergePresentation(data []byte, lang, targetLang string, tr filter.Translations) ([]byte, error) {
	d, err := parseAs(data, lang, Options{}, presentation)
	if err != nil {
		return nil, err
	}
	changed := map[string][]byte{}
	for _, p := range d.parts {
		if !p.hasUnits() {
			continue
		}
		changed[p.file] = p.render(d, tr, targetLang)
	}
	if err := d.updateChartData(changed, tr); err != nil {
		return nil, err
	}
	return rewriteZip(d.zr, changed)
}

// slideParts lists the parts of a presentation with text: each slide in
// the order of the show, followed by its charts, its diagrams and its
// speaker notes.
func slideParts(zr *zip.Reader, main string, mainData []byte) ([]partRef, error) {
	root, err := parseFragmentDoc(mainData)
	if err != nil {
		return nil, fmt.Errorf("pptx: %s: %w", main, err)
	}
	rels, err := readRels(zr, main)
	if err != nil {
		return nil, fmt.Errorf("pptx: %w", err)
	}
	target := map[string]string{}
	for _, r := range rels {
		if strings.HasSuffix(r.typ, "/slide") {
			target[r.id] = r.target
		}
	}
	var files []partRef
	seen := map[string]bool{}
	add := func(name string, kind filter.Part, cache bool) {
		if name != "" && !seen[name] {
			seen[name] = true
			files = append(files, partRef{name, kind, cache})
		}
	}
	list := root.Child("sldIdLst")
	if list == nil {
		return nil, nil
	}
	for _, s := range list.Children("sldId") {
		id := ""
		for _, a := range s.Attrs {
			if a.Name.Space != "" && a.Name.Space != "xmlns" && a.Name.Local == "id" {
				id = a.Value
			}
		}
		slide := target[id]
		if slide == "" {
			continue
		}
		add(slide, filter.PartBody, false)
		srels, _ := readRels(zr, slide)
		var notes string
		var drawings []string
		for _, r := range srels {
			switch {
			case strings.HasSuffix(r.typ, "/chart"):
				add(r.target, filter.PartChart, false)
			case strings.HasSuffix(r.typ, "/diagramData"):
				add(r.target, filter.PartDiagram, false)
			case strings.HasSuffix(r.typ, "/diagramDrawing"):
				drawings = append(drawings, r.target)
			case strings.HasSuffix(r.typ, "/notesSlide"):
				notes = r.target
			}
		}
		// SmartArt keeps a drawing of the diagram as a cache; it gets the
		// translations of the diagram's data.
		for _, n := range drawings {
			add(n, filter.PartDiagram, true)
		}
		add(notes, filter.PartNotes, false)
	}
	return files, nil
}
