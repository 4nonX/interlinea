package docxtest

import "strings"

// TextBox is an inline text box as Word 2010 and later write it: the
// DrawingML shape in mc:Choice and a VML copy in mc:Fallback, both with
// the same paragraphs.
func TextBox(paras ...string) string {
	content := `<w:txbxContent>` + strings.Join(paras, "") + `</w:txbxContent>`
	return `<w:r><mc:AlternateContent><mc:Choice Requires="wps"><w:drawing><wp:inline distT="0" distB="0" distL="0" distR="0"><wp:extent cx="3600000" cy="540000"/><wp:docPr id="1" name="Textfeld 1"/><a:graphic><a:graphicData uri="http://schemas.microsoft.com/office/word/2010/wordprocessingShape"><wps:wsp><wps:cNvSpPr txBox="1"/><wps:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="3600000" cy="540000"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom><a:ln w="6350"><a:solidFill><a:srgbClr val="1F5BD6"/></a:solidFill></a:ln></wps:spPr><wps:txbx>` +
		content + `</wps:txbx><wps:bodyPr rot="0" vert="horz" wrap="square" lIns="91440" tIns="45720" rIns="91440" bIns="45720" anchor="t"><a:noAutofit/></wps:bodyPr></wps:wsp></a:graphicData></a:graphic></wp:inline></w:drawing></mc:Choice><mc:Fallback><w:pict><v:shape id="Textfeld1" type="#_x0000_t202" style="width:283.5pt;height:42.5pt" strokecolor="#1f5bd6"><v:textbox>` +
		content + `</v:textbox></v:shape></w:pict></mc:Fallback></mc:AlternateContent></w:r>`
}

// Chart is an inline chart referencing the chart part with rID.
func Chart(rID string) string {
	return `<w:r><w:drawing><wp:inline distT="0" distB="0" distL="0" distR="0"><wp:extent cx="4320000" cy="2160000"/><wp:docPr id="2" name="Diagramm 1"/><a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/chart"><c:chart r:id="` + rID + `"/></a:graphicData></a:graphic></wp:inline></w:drawing></w:r>`
}

const chartPart = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<c:chartSpace xmlns:c="http://schemas.openxmlformats.org/drawingml/2006/chart" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><c:lang val="de-DE"/><c:chart><c:title><c:tx><c:rich><a:bodyPr/><a:lstStyle/><a:p><a:pPr><a:defRPr sz="1400" b="1"/></a:pPr><a:r><a:rPr lang="de-DE" sz="1400" b="1"/><a:t>Miete und Nebenkosten 2027</a:t></a:r></a:p></c:rich></c:tx><c:overlay val="0"/></c:title><c:autoTitleDeleted val="0"/><c:plotArea><c:layout/><c:barChart><c:barDir val="col"/><c:grouping val="clustered"/><c:varyColors val="0"/>` +
	`<c:ser><c:idx val="0"/><c:order val="0"/><c:tx><c:strRef><c:f>Tabelle1!$B$1</c:f><c:strCache><c:ptCount val="1"/><c:pt idx="0"><c:v>Nettokaltmiete</c:v></c:pt></c:strCache></c:strRef></c:tx>` +
	`<c:cat><c:strRef><c:f>Tabelle1!$A$2:$A$3</c:f><c:strCache><c:ptCount val="2"/><c:pt idx="0"><c:v>Januar</c:v></c:pt><c:pt idx="1"><c:v>Februar</c:v></c:pt></c:strCache></c:strRef></c:cat>` +
	`<c:val><c:numRef><c:f>Tabelle1!$B$2:$B$3</c:f><c:numCache><c:formatCode>General</c:formatCode><c:ptCount val="2"/><c:pt idx="0"><c:v>1250</c:v></c:pt><c:pt idx="1"><c:v>1250</c:v></c:pt></c:numCache></c:numRef></c:val></c:ser>` +
	`<c:axId val="1"/><c:axId val="2"/></c:barChart><c:catAx><c:axId val="1"/><c:scaling><c:orientation val="minMax"/></c:scaling><c:delete val="0"/><c:axPos val="b"/><c:crossAx val="2"/></c:catAx><c:valAx><c:axId val="2"/><c:scaling><c:orientation val="minMax"/></c:scaling><c:delete val="0"/><c:axPos val="l"/><c:numFmt formatCode="General" sourceLinked="1"/><c:crossAx val="1"/></c:valAx></c:plotArea><c:plotVisOnly val="1"/></c:chart><c:externalData r:id="rIdData"><c:autoUpdate val="0"/></c:externalData></c:chartSpace>`

// workbook is the chart's embedded data.
func workbook() string {
	return string(Build(map[string]string{
		"[Content_Types].xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/><Override PartName="/xl/sharedStrings.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sharedStrings+xml"/></Types>`,
		"_rels/.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`,
		"xl/workbook.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Tabelle1" sheetId="1" r:id="rId1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/sharedStrings" Target="sharedStrings.xml"/></Relationships>`,
		"xl/worksheets/sheet1.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData><row r="1"><c r="B1" t="s"><v>0</v></c></row><row r="2"><c r="A2" t="s"><v>1</v></c><c r="B2"><v>1250</v></c></row><row r="3"><c r="A3" t="s"><v>2</v></c><c r="B3"><v>1250</v></c></row></sheetData></worksheet>`,
		"xl/sharedStrings.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" count="3" uniqueCount="3"><si><t>Nettokaltmiete</t></si><si><t>Januar</t></si><si><t>Februar</t></si></sst>`,
	}))
}

const diagramData = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<dgm:dataModel xmlns:dgm="http://schemas.openxmlformats.org/drawingml/2006/diagram" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><dgm:ptLst><dgm:pt modelId="{0}" type="doc"><dgm:prSet/><dgm:spPr/><dgm:t><a:bodyPr/><a:lstStyle/><a:p><a:endParaRPr lang="de-DE"/></a:p></dgm:t></dgm:pt><dgm:pt modelId="{1}"><dgm:prSet/><dgm:spPr/><dgm:t><a:bodyPr/><a:lstStyle/><a:p><a:r><a:rPr lang="de-DE"/><a:t>Antrag stellen</a:t></a:r></a:p></dgm:t></dgm:pt><dgm:pt modelId="{2}"><dgm:prSet/><dgm:spPr/><dgm:t><a:bodyPr/><a:lstStyle/><a:p><a:r><a:rPr lang="de-DE"/><a:t>Unterlagen </a:t></a:r><a:r><a:rPr lang="de-DE" b="1"/><a:t>prüfen</a:t></a:r></a:p></dgm:t></dgm:pt></dgm:ptLst><dgm:cxnLst/><dgm:bg/><dgm:whole/></dgm:dataModel>`

const diagramDrawing = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<dsp:drawing xmlns:dsp="http://schemas.microsoft.com/office/drawing/2008/diagram" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><dsp:spTree><dsp:nvGrpSpPr><dsp:cNvPr id="0" name=""/><dsp:cNvGrpSpPr/></dsp:nvGrpSpPr><dsp:grpSpPr/><dsp:sp modelId="{1}"><dsp:nvSpPr><dsp:cNvPr id="1" name=""/><dsp:cNvSpPr/></dsp:nvSpPr><dsp:spPr/><dsp:txBody><a:bodyPr/><a:lstStyle/><a:p><a:r><a:rPr lang="de-DE"/><a:t>Antrag stellen</a:t></a:r></a:p></dsp:txBody></dsp:sp><dsp:sp modelId="{2}"><dsp:nvSpPr><dsp:cNvPr id="2" name=""/><dsp:cNvSpPr/></dsp:nvSpPr><dsp:spPr/><dsp:txBody><a:bodyPr/><a:lstStyle/><a:p><a:r><a:rPr lang="de-DE"/><a:t>Unterlagen </a:t></a:r><a:r><a:rPr lang="de-DE" b="1"/><a:t>prüfen</a:t></a:r></a:p></dsp:txBody></dsp:sp></dsp:spTree></dsp:drawing>`

// Features builds a document with everything the sample lease does not
// have: a text box, a content control, tracked changes, a comment,
// coloured and resized text, a character style, a chart with embedded
// data and SmartArt with its cached drawing.
func Features() []byte {
	body := P("Heading1", R("Weitere Inhalte")) +
		P("", R("Siehe den Hinweis im Kasten:")) +
		P("", TextBox(P("", R("Hinweis: Die Kaution wird verzinst. Zinsen stehen dem Mieter zu.")))) +
		P("", R("Vertragsbeginn: "), `<w:sdt><w:sdtPr><w:alias w:val="Datum"/><w:tag w:val="start"/><w:date w:fullDate="2027-01-01T00:00:00Z"><w:dateFormat w:val="d. MMMM yyyy"/><w:lid w:val="de-DE"/></w:date></w:sdtPr><w:sdtContent>`+R("1. Januar 2027")+`</w:sdtContent></w:sdt>`, R(" (verbindlich).")) +
		`<w:p>` + R("Die Frist beträgt ") + `<w:del w:id="10" w:author="Eva" w:date="2026-11-01T09:00:00Z"><w:r><w:delText>zwei</w:delText></w:r></w:del><w:ins w:id="11" w:author="Eva" w:date="2026-11-01T09:00:00Z">` + R("drei") + `</w:ins>` + R(" Monate.") + `</w:p>` +
		`<w:p><w:commentRangeStart w:id="0"/>` + R("Die Nebenkosten werden ") + RP(`<w:color w:val="C00000"/>`, "monatlich") + R(" im Voraus gezahlt.") + `<w:commentRangeEnd w:id="0"/><w:r><w:rPr><w:rStyle w:val="CommentReference"/></w:rPr><w:commentReference w:id="0"/></w:r></w:p>` +
		P("", R("Wichtig sind "), RP(`<w:sz w:val="28"/><w:szCs w:val="28"/>`, "Fristen"), R(", "), RP(`<w:rFonts w:ascii="Courier New" w:hAnsi="Courier New"/>`, "Beträge"), R(" und "), RP(`<w:rStyle w:val="Strong"/>`, "Unterschriften"), R(".")) +
		P("", Chart("rIdChart1"))
	return Document(Parts{
		Body:     body,
		Lang:     "de-DE",
		Comments: []string{`<w:p><w:r><w:annotationRef/></w:r>` + R("Bitte mit dem Mandanten klären.") + `</w:p>`},
		Extra: map[string]string{
			"word/charts/chart1.xml": chartPart,
			"word/charts/_rels/chart1.xml.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rIdData" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/package" Target="../embeddings/Microsoft_Excel_Worksheet.xlsx"/></Relationships>`,
			"word/embeddings/Microsoft_Excel_Worksheet.xlsx": workbook(),
			"word/diagrams/data1.xml":                        diagramData,
			"word/diagrams/drawing1.xml":                     diagramDrawing,
		},
		Rels: []string{`<Relationship Id="rIdChart1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/chart" Target="charts/chart1.xml"/>`},
		Types: []string{
			`<Override PartName="/word/charts/chart1.xml" ContentType="application/vnd.openxmlformats-officedocument.drawingml.chart+xml"/>`,
			`<Override PartName="/word/diagrams/data1.xml" ContentType="application/vnd.openxmlformats-officedocument.drawingml.diagramData+xml"/>`,
			`<Override PartName="/word/diagrams/drawing1.xml" ContentType="application/vnd.ms-office.drawingml.diagramDrawing+xml"/>`,
		},
	})
}
