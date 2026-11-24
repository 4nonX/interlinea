package docxtest

import (
	"fmt"
	"strings"
)

const slideNS = `xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"`

// Slide describes one slide of a test presentation. Shapes holds the
// shapes of the slide (Shape, SlideTable, ChartFrame); Notes the
// paragraphs of its speaker notes, empty for none.
type Slide struct {
	Shapes string
	Notes  string
	// Chart adds the test chart with its embedded data to the slide.
	Chart bool
}

// AP is a DrawingML paragraph of runs (AR, ARP, ABr).
func AP(runs ...string) string { return `<a:p>` + strings.Join(runs, "") + `</a:p>` }

// APR is a DrawingML paragraph with paragraph properties (attributes of a:pPr).
func APR(attrs string, runs ...string) string {
	return `<a:p><a:pPr ` + attrs + `/>` + strings.Join(runs, "") + `</a:p>`
}

// AR is a DrawingML run of text in the document's language.
func AR(text string) string { return ARP(``, text) }

// ARP is a DrawingML run with extra attributes of its properties (b="1").
func ARP(attrs, text string) string {
	if attrs != "" {
		attrs = " " + attrs
	}
	return `<a:r><a:rPr lang="de-DE"` + attrs + `/><a:t>` + Esc(text) + `</a:t></a:r>`
}

// ABr is a line break inside a DrawingML paragraph.
func ABr() string { return `<a:br><a:rPr lang="de-DE"/></a:br>` }

// Shape is a text shape of a slide: a title, a body or a text box,
// depending on the placeholder type ("title", "body", "" for a text box).
func Shape(id int, placeholder string, paras ...string) string {
	ph := ""
	if placeholder != "" {
		ph = `<p:ph type="` + placeholder + `"/>`
	}
	return fmt.Sprintf(`<p:sp><p:nvSpPr><p:cNvPr id="%d" name="Form %d"/><p:cNvSpPr/><p:nvPr>%s</p:nvPr></p:nvSpPr><p:spPr/><p:txBody><a:bodyPr/><a:lstStyle/>%s</p:txBody></p:sp>`, id, id, ph, strings.Join(paras, ""))
}

// SlideTable is a table on a slide; each cell holds one paragraph of text.
func SlideTable(id int, rows ...[]string) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<p:graphicFrame><p:nvGraphicFramePr><p:cNvPr id="%d" name="Tabelle %d"/><p:cNvGraphicFramePr/><p:nvPr/></p:nvGraphicFramePr><p:xfrm><a:off x="0" y="0"/><a:ext cx="6000000" cy="800000"/></p:xfrm><a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/table"><a:tbl><a:tblPr firstRow="1"/><a:tblGrid>`, id, id)
	if len(rows) > 0 {
		for range rows[0] {
			b.WriteString(`<a:gridCol w="3000000"/>`)
		}
	}
	b.WriteString(`</a:tblGrid>`)
	for _, row := range rows {
		b.WriteString(`<a:tr h="400000">`)
		for _, cell := range row {
			b.WriteString(`<a:tc><a:txBody><a:bodyPr/><a:lstStyle/>` + AP(AR(cell)) + `</a:txBody><a:tcPr/></a:tc>`)
		}
		b.WriteString(`</a:tr>`)
	}
	b.WriteString(`</a:tbl></a:graphicData></a:graphic></p:graphicFrame>`)
	return b.String()
}

// Presentation builds a .pptx with the slides in the given order. The
// slide files are numbered against that order (the first slide of the
// show is slide2.xml when there are two), as happens when slides are
// moved in PowerPoint, so readers must follow the presentation's list.
func Presentation(slides ...Slide) []byte {
	files := map[string]string{
		"_rels/.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="ppt/presentation.xml"/></Relationships>`,
		"ppt/slideMasters/slideMaster1.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:sldMaster ` + slideNS + `><p:cSld><p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/>` + Shape(2, "title", AP(AR("Titelmasterformat durch Klicken bearbeiten"))) + `</p:spTree></p:cSld><p:clrMap bg1="lt1" tx1="dk1" bg2="lt2" tx2="dk2" accent1="accent1" accent2="accent2" accent3="accent3" accent4="accent4" accent5="accent5" accent6="accent6" hlink="hlink" folHlink="folHlink"/></p:sldMaster>`,
	}
	types := []string{
		`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>`,
		`<Default Extension="xml" ContentType="application/xml"/>`,
		`<Default Extension="xlsx" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"/>`,
		`<Override PartName="/ppt/presentation.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"/>`,
		`<Override PartName="/ppt/slideMasters/slideMaster1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slideMaster+xml"/>`,
	}
	var ids, rels strings.Builder
	rels.WriteString(`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideMaster" Target="slideMasters/slideMaster1.xml"/>`)
	for i, s := range slides {
		file := len(slides) - i // the files run against the order of the show
		name := fmt.Sprintf("ppt/slides/slide%d.xml", file)
		shapes := s.Shapes
		srels := ""
		if s.Chart {
			chart := fmt.Sprintf("chart%d", file)
			shapes += `<p:graphicFrame><p:nvGraphicFramePr><p:cNvPr id="90" name="Diagramm"/><p:cNvGraphicFramePr/><p:nvPr/></p:nvGraphicFramePr><p:xfrm><a:off x="0" y="0"/><a:ext cx="4000000" cy="2000000"/></p:xfrm><a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/chart"><c:chart xmlns:c="http://schemas.openxmlformats.org/drawingml/2006/chart" r:id="rIdChart"/></a:graphicData></a:graphic></p:graphicFrame>`
			srels += `<Relationship Id="rIdChart" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/chart" Target="../charts/` + chart + `.xml"/>`
			files["ppt/charts/"+chart+".xml"] = chartPart
			files["ppt/charts/_rels/"+chart+".xml.rels"] = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rIdData" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/package" Target="../embeddings/` + chart + `.xlsx"/></Relationships>`
			files["ppt/embeddings/"+chart+".xlsx"] = workbook()
			types = append(types, `<Override PartName="/ppt/charts/`+chart+`.xml" ContentType="application/vnd.openxmlformats-officedocument.drawingml.chart+xml"/>`)
		}
		if s.Notes != "" {
			notes := fmt.Sprintf("notesSlide%d", file)
			srels += `<Relationship Id="rIdNotes" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/notesSlide" Target="../notesSlides/` + notes + `.xml"/>`
			files["ppt/notesSlides/"+notes+".xml"] = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:notes ` + slideNS + `><p:cSld><p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/>` +
				Shape(2, "body", s.Notes) +
				Shape(3, "sldNum", `<a:p><a:fld id="{B5CEABB6-07DC-46E8-9B57-56EC44A396E5}" type="slidenum"><a:rPr lang="de-DE"/><a:t>‹Nr.›</a:t></a:fld><a:endParaRPr lang="de-DE"/></a:p>`) +
				`</p:spTree></p:cSld></p:notes>`
			types = append(types, `<Override PartName="/ppt/notesSlides/`+notes+`.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.notesSlide+xml"/>`)
		}
		files[name] = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:sld ` + slideNS + `><p:cSld><p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/>` + shapes + `</p:spTree></p:cSld></p:sld>`
		if srels != "" {
			files[fmt.Sprintf("ppt/slides/_rels/slide%d.xml.rels", file)] = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` + srels + `</Relationships>`
		}
		types = append(types, fmt.Sprintf(`<Override PartName="/%s" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slide+xml"/>`, name))
		fmt.Fprintf(&ids, `<p:sldId id="%d" r:id="rId%d"/>`, 256+i, i+2)
		fmt.Fprintf(&rels, `<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slide" Target="slides/slide%d.xml"/>`, i+2, file)
	}
	files["ppt/presentation.xml"] = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:presentation ` + slideNS + `><p:sldMasterIdLst><p:sldMasterId id="2147483648" r:id="rId1"/></p:sldMasterIdLst><p:sldIdLst>` + ids.String() + `</p:sldIdLst><p:sldSz cx="12192000" cy="6858000"/><p:notesSz cx="6858000" cy="9144000"/></p:presentation>`
	files["ppt/_rels/presentation.xml.rels"] = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` + rels.String() + `</Relationships>`
	files["[Content_Types].xml"] = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` + strings.Join(types, "") + `</Types>`
	return Build(files)
}

// Deck is the test presentation: a title slide, a slide with a list, a
// table and a chart, each with speaker notes.
func Deck() []byte {
	return Presentation(
		Slide{
			Shapes: Shape(2, "title", AP(AR("Mietrecht 2027"))) +
				Shape(3, "body", AP(AR("Was sich für Vermieter ändert. "), ARP(`b="1"`, "Stand: November 2026"), AR("."))),
			Notes: AP(AR("Begrüßung. Kurz die Agenda nennen.")),
		},
		Slide{
			Shapes: Shape(2, "title", AP(AR("Fristen"))) +
				Shape(3, "body", AP(AR("Kündigung: drei Monate")), AP(AR("Erste Zeile"), ABr(), AR("zweite Zeile"))) +
				SlideTable(4, []string{"Frist", "Dauer"}, []string{"Kündigung", "3 Monate"}) +
				Shape(5, "", AP(AR("Quelle: BGB"))),
			Notes: AP(AR("Auf die Tabelle hinweisen.")),
			Chart: true,
		},
	)
}
