package server

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter/docx/docxtest"
	"github.com/4nonX/interlinea/filter/mqpackage"
	"github.com/4nonX/interlinea/filter/xliff"
	"github.com/4nonX/interlinea/project"
	"github.com/4nonX/interlinea/project/projectdb"
)

func zipOf(t *testing.T, files [][2]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range files {
		w, err := zw.Create(f[0])
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(f[1]))
	}
	zw.Close()
	return buf.Bytes()
}

func unzip(t *testing.T, data []byte, name string) string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("not a ZIP: %v", err)
	}
	for _, f := range zr.File {
		if f.Name == name {
			rc, _ := f.Open()
			defer rc.Close()
			b, _ := io.ReadAll(rc)
			return string(b)
		}
	}
	t.Fatalf("no %s in the file", name)
	return ""
}

const xmlHead = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`

func workbookFile(t *testing.T) []byte {
	const main = `xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"`
	const rels = `xmlns="http://schemas.openxmlformats.org/package/2006/relationships"`
	const typ = `http://schemas.openxmlformats.org/officeDocument/2006/relationships/`
	return zipOf(t, [][2]string{
		{"[Content_Types].xml", xmlHead + `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/></Types>`},
		{"_rels/.rels", xmlHead + `<Relationships ` + rels + `><Relationship Id="rId1" Type="` + typ + `officeDocument" Target="xl/workbook.xml"/></Relationships>`},
		{"xl/workbook.xml", xmlHead + `<workbook ` + main + `><sheets><sheet name="Fristen" sheetId="1" r:id="rId1"/></sheets></workbook>`},
		{"xl/_rels/workbook.xml.rels", xmlHead + `<Relationships ` + rels + `><Relationship Id="rId1" Type="` + typ + `worksheet" Target="worksheets/sheet1.xml"/><Relationship Id="rId2" Type="` + typ + `sharedStrings" Target="sharedStrings.xml"/></Relationships>`},
		{"xl/sharedStrings.xml", xmlHead + `<sst ` + main + ` count="2" uniqueCount="2"><si><t>Frist</t></si><si><t>Drei Monate</t></si></sst>`},
		{"xl/worksheets/sheet1.xml", xmlHead + `<worksheet ` + main + `><sheetData><row r="1"><c r="A1" t="s"><v>0</v></c><c r="B1" t="s"><v>1</v></c></row></sheetData></worksheet>`},
	})
}

func indesignFile(t *testing.T) []byte {
	const ns = `xmlns:idPkg="http://ns.adobe.com/AdobeInDesign/idml/1.0/packaging" DOMVersion="8.0"`
	return zipOf(t, [][2]string{
		{"mimetype", "application/vnd.adobe.indesign-idml-package"},
		{"designmap.xml", xmlHead + `<Document ` + ns + ` Self="d"><idPkg:Story src="Stories/Story_u1.xml"/></Document>`},
		{"Stories/Story_u1.xml", xmlHead + `<idPkg:Story ` + ns + `><Story Self="u1"><ParagraphStyleRange AppliedParagraphStyle="ParagraphStyle/Titel"><CharacterStyleRange AppliedCharacterStyle="CharacterStyle/$ID/[No character style]"><Content>Mietvertrag</Content><Br/><Content>Die Kaution ist fällig.</Content></CharacterStyleRange></ParagraphStyleRange></Story></idPkg:Story>`},
	})
}

func memoQFile(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "filter", "xliff", "testdata", "memoq.mqxliff"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// Every document format goes the whole way: the server reads the file,
// a translation is confirmed, and the translated file has it.
func TestDocumentFormats(t *testing.T) {
	te := newTeam(t)
	req := leaseRequest()
	req.Settings.TargetLangs = []string{"en-GB"}
	delete(req.Settings.Memories, "fr-FR")
	req.Memories = req.Memories[:1]
	page := `<html lang="de"><head><title>Fristen</title></head><body><p>Die Kaution ist <b>fällig</b>.</p></body></html>`
	manual, err := os.ReadFile("../filter/pdf/testdata/manual.pdf")
	if err != nil {
		t.Fatal(err)
	}
	req.Files = []fileUpload{
		{Name: "Folien.pptx", Role: project.Translatable, Data: docxtest.Deck()},
		{Name: "Tabelle.xlsx", Role: project.Translatable, Data: workbookFile(t)},
		{Name: "Seite.html", Role: project.Translatable, Data: []byte(page)},
		{Name: "Layout.idml", Role: project.Translatable, Data: indesignFile(t)},
		{Name: "Manual.pdf", Role: project.Translatable, Data: manual},
	}
	var p projectView
	te.call("POST", "/api/projects", te.pm, req, 201, &p)
	base := fmt.Sprintf("/api/projects/%d", p.ID)
	bold := content.Text("The deposit is ").AppendTag(content.Tag{Kind: content.TagOpen, ID: "1"}).Append("due").AppendTag(content.Tag{Kind: content.TagClose, ID: "1"}).Append(".")
	for _, c := range []struct {
		file, format, segment string
		count                 int
		target                content.Content
		check                 func(out []byte) bool
	}{
		{"Folien.pptx", "pptx", "slide2/p1/s1", 18, content.Text("Tenancy law 2027"), func(out []byte) bool {
			return strings.Contains(unzip(t, out, "ppt/slides/slide2.xml"), `<a:t>Tenancy law 2027</a:t>`)
		}},
		{"Tabelle.xlsx", "xlsx", "sheet1/B1/s1", 2, content.Text("Three months"), func(out []byte) bool {
			return strings.Contains(unzip(t, out, "xl/sharedStrings.xml"), `<si><t>Frist</t></si><si><t>Three months</t></si>`)
		}},
		{"Seite.html", "html", "b2/s1", 2, bold, func(out []byte) bool {
			return string(out) == `<html lang="en-GB"><head><title>Fristen</title></head><body><p>The deposit is <b>due</b>.</p></body></html>`
		}},
		{"Layout.idml", "idml", "Story_u1/p2/s1", 2, content.Text("The deposit is due."), func(out []byte) bool {
			return strings.Contains(unzip(t, out, "Stories/Story_u1.xml"), `<Content>Mietvertrag</Content><Br/><Content>The deposit is due.</Content></CharacterStyleRange>`)
		}},
		// The translation of a PDF is the Word document its text was read into.
		{"Manual.pdf", "pdf", "document/p3/s1", 58, content.Text("1 Sicherheit"), func(out []byte) bool {
			doc := unzip(t, out, "word/document.xml")
			return strings.Contains(doc, `>1 Sicherheit</w:t>`) && strings.Contains(doc, `>Operating manual</w:t>`) && strings.Contains(doc, `<w:b/>`)
		}},
	} {
		var segs []projectdb.StoredSegment
		te.call("GET", base+"/segments?file="+url.QueryEscape(c.file)+"&lang=en-GB", te.pm, nil, 200, &segs)
		if len(segs) != c.count {
			t.Fatalf("%s: %d segments, want %d", c.file, len(segs), c.count)
		}
		found := false
		for _, sg := range segs {
			if sg.ID == c.segment {
				found = true
				te.call("PUT", segPath(p.ID, c.file, "en-GB", sg.ID, sg.Version), te.pm, project.Edit{Target: c.target, Status: project.StatusConfirmed}, 200, nil)
			}
		}
		if !found {
			t.Fatalf("%s has no segment %s", c.file, c.segment)
		}
		r := te.download(base+"/files/translated?name="+url.QueryEscape(c.file)+"&lang=en-GB", te.pm)
		if !c.check(r.body) {
			t.Errorf("%s: the translated file lacks the translation", c.file)
		}
		ext := filepath.Ext(c.file)
		if c.format == "pdf" {
			ext = ".docx"
			if ct := r.header.Get("Content-Type"); !strings.Contains(ct, "wordprocessingml") {
				t.Errorf("the translation of %s is sent as %s", c.file, ct)
			}
		}
		want := strings.TrimSuffix(c.file, filepath.Ext(c.file)) + " (en-GB)" + ext
		if !strings.Contains(r.header.Get("Content-Disposition"), want) {
			t.Errorf("%s is sent as %q", c.file, r.header.Get("Content-Disposition"))
		}
	}

	// What a file is, before a project is made of it.
	for name, data := range map[string][]byte{"Folien.pptx": docxtest.Deck(), "Tabelle.xlsx": workbookFile(t), "Seite.html": []byte(page), "Layout.idml": indesignFile(t)} {
		var in inspection
		te.call("POST", "/api/files/inspect?name="+name, te.pm, data, 200, &in, "Content-Type", "application/octet-stream")
		if !in.Translatable || in.Segments == 0 || in.Words == 0 || in.Problem != "" {
			t.Errorf("%s inspected: %+v", name, in)
		}
	}
	// A file that is not what its name says is refused with a reason.
	var in inspection
	te.call("POST", "/api/files/inspect?name="+"Falsch.pptx", te.pm, workbookFile(t), 200, &in, "Content-Type", "application/octet-stream")
	if in.Translatable || !strings.Contains(in.Problem, "not a PowerPoint presentation") {
		t.Errorf("a workbook named .pptx: %+v", in)
	}
}

func TestMemoQPackageProject(t *testing.T) {
	te := newTeam(t)
	doc := memoQFile(t)
	ex, err := xliff.Extract(doc)
	if err != nil {
		t.Fatal(err)
	}
	archive := zipOf(t, [][2]string{{"document.mqxliff", string(doc)}, {"skeleton.xml", "<skeleton/>"}})
	handoff := zipOf(t, [][2]string{
		{"HandoffPackageInfo.xml", `<?xml version="1.0" encoding="utf-8"?><HandoffPackageInfo><ProjectName>Hofmann</ProjectName><UserName>pm</UserName></HandoffPackageInfo>`},
		{"Documents/Vertrag.docx.mqxlz", string(archive)},
		{"TM/Kanzlei.tmx", "<tmx/>"},
	})
	req := leaseRequest()
	req.Settings.SourceLang = ex.SourceLang
	req.Settings.TargetLangs = []string{ex.TargetLang}
	req.Settings.Memories = nil
	req.Memories = nil
	req.Settings.ProjectMemory = false
	req.Files = []fileUpload{{Name: "Hofmann.mqout", Role: project.Package, Data: handoff}}
	var p projectView
	te.call("POST", "/api/projects", te.pm, req, 201, &p)
	fs := p.Settings.Files
	if len(fs) != 2 || fs[0].Role != project.Package || fs[1].Name != "Documents/Vertrag.docx.mqxliff" || fs[1].Package != "Documents/Vertrag.docx.mqxlz" {
		t.Fatalf("files %+v", fs)
	}
	base := fmt.Sprintf("/api/projects/%d", p.ID)
	var segs []projectdb.StoredSegment
	te.call("GET", base+"/segments?file="+url.QueryEscape(fs[1].Name)+"&lang="+ex.TargetLang, te.pm, nil, 200, &segs)
	var open projectdb.StoredSegment
	for _, sg := range segs {
		if !sg.Locked {
			open = sg
			break
		}
	}
	te.call("PUT", segPath(p.ID, fs[1].Name, ex.TargetLang, open.ID, open.Version), te.pm, project.Edit{Target: content.Text("Made in Interlinea"), Status: project.StatusConfirmed}, 200, nil)

	r := te.download(base+"/return-package", te.pm)
	if !strings.Contains(r.header.Get("Content-Disposition"), "Hofmann.mqback") {
		t.Fatalf("disposition %q", r.header.Get("Content-Disposition"))
	}
	if info := unzip(t, r.body, "DeliveryPackageInfo.xml"); !strings.Contains(info, "<HandoffDeliveryPackageInfo><ProjectName>Hofmann</ProjectName><User>pm</User></HandoffDeliveryPackageInfo>") {
		t.Errorf("delivery description: %s", info)
	}
	back, err := xliff.Extract([]byte(unzip(t, r.body, "Vertrag.docx.mqxliff")))
	if err != nil {
		t.Fatal(err)
	}
	done := false
	for _, u := range back.Units {
		if u.ID == open.ID && u.Target.Plain() == "Made in Interlinea" {
			done = true
		}
	}
	if !done {
		t.Error("the delivered document lacks the translation")
	}

	// The package is inspected like a Trados package, and a memoQ XLIFF
	// archive on its own is a bilingual file.
	var in inspection
	te.call("POST", "/api/files/inspect?name="+"Hofmann.mqout", te.pm, handoff, 200, &in, "Content-Type", "application/octet-stream")
	if in.Package == nil || in.Package.Name != "Hofmann" || len(in.Package.Files) != 1 || in.Package.Files[0].Segments != len(ex.Units) {
		t.Errorf("package inspected: %+v", in.Package)
	}
	te.call("POST", "/api/files/inspect?name="+"Vertrag.mqxlz", te.pm, archive, 200, &in, "Content-Type", "application/octet-stream")
	if in.Bilingual == nil || in.Bilingual.TargetLang != ex.TargetLang || in.Segments != len(ex.Units) {
		t.Errorf("archive inspected: %+v", in)
	}
	if _, err := mqpackage.Open(handoff); err != nil {
		t.Fatal(err)
	}
}

func TestMemoQArchiveFile(t *testing.T) {
	te := newTeam(t)
	doc := memoQFile(t)
	ex, _ := xliff.Extract(doc)
	archive := zipOf(t, [][2]string{{"document.mqxliff", string(doc)}, {"skeleton.xml", "<skeleton/>"}})
	req := leaseRequest()
	req.Settings.SourceLang = ex.SourceLang
	req.Settings.TargetLangs = []string{ex.TargetLang}
	req.Settings.Memories = nil
	req.Memories = nil
	req.Settings.ProjectMemory = false
	req.Files = []fileUpload{{Name: "Vertrag.mqxlz", Role: project.Translatable, Data: archive}}
	var p projectView
	te.call("POST", "/api/projects", te.pm, req, 201, &p)
	base := fmt.Sprintf("/api/projects/%d", p.ID)
	var segs []projectdb.StoredSegment
	te.call("GET", base+"/segments?file=Vertrag.mqxlz&lang="+ex.TargetLang, te.pm, nil, 200, &segs)
	if len(segs) != len(ex.Units) {
		t.Fatalf("%d segments, want %d", len(segs), len(ex.Units))
	}
	var open projectdb.StoredSegment
	for _, sg := range segs {
		if !sg.Locked {
			open = sg
			break
		}
	}
	te.call("PUT", segPath(p.ID, "Vertrag.mqxlz", ex.TargetLang, open.ID, open.Version), te.pm, project.Edit{Target: content.Text("In the archive"), Status: project.StatusConfirmed}, 200, nil)
	r := te.download(base+"/files/translated?name=Vertrag.mqxlz&lang="+ex.TargetLang, te.pm)
	if !strings.Contains(r.header.Get("Content-Disposition"), `Vertrag.mqxlz`) || unzip(t, r.body, "skeleton.xml") != "<skeleton/>" {
		t.Fatalf("the archive is not sent back as it came: %q", r.header.Get("Content-Disposition"))
	}
	if !strings.Contains(unzip(t, r.body, "document.mqxliff"), "In the archive") {
		t.Error("the document in the archive lacks the translation")
	}
}

// The paragraphs of a project's document, for a preview of it.
func TestFileLayout(t *testing.T) {
	te := newTeam(t)
	req := leaseRequest()
	req.Settings.Name = "Layout"
	plain, err := os.ReadFile("../filter/xliff/testdata/xliff2.xlf")
	if err != nil {
		t.Fatal(err)
	}
	req.Files = []fileUpload{
		{Name: "Seite.html", Role: project.Translatable, Data: []byte(`<html lang="de"><body><h1>Fristen</h1><p>Die Kaution ist <b>fällig</b>.</p><table><tr><td>Miete</td></tr></table></body></html>`)},
		{Name: "Plain.xlf", Role: project.Translatable, Data: plain},
	}
	var p projectView
	te.call("POST", "/api/projects", te.pm, req, 201, &p)
	var got fileLayout
	te.call("GET", fmt.Sprintf("/api/projects/%d/files/layout?name=Seite.html", p.ID), te.pm, nil, 200, &got)
	if len(got.Blocks) != 3 || got.Blocks[0].Heading != 1 || got.Blocks[1].Heading != 0 || got.Blocks[2].Cell == nil {
		t.Fatalf("blocks %+v", got.Blocks)
	}
	// Every segment of the file is in one of the blocks.
	var segs []project.Segment
	te.call("GET", fmt.Sprintf("/api/projects/%d/segments?file=Seite.html&lang=en-GB", p.ID), te.pm, nil, 200, &segs)
	ids := map[string]bool{}
	for _, b := range got.Blocks {
		ids[b.ID] = true
	}
	for _, s := range segs {
		if !ids[s.Block] {
			t.Fatalf("segment %s is in block %q, which the layout does not have", s.ID, s.Block)
		}
	}
	// A bilingual file has none, and a file that is not there is not found.
	var none fileLayout
	te.call("GET", fmt.Sprintf("/api/projects/%d/files/layout?name=Plain.xlf", p.ID), te.pm, nil, 200, &none)
	if none.Blocks == nil || len(none.Blocks) != 0 {
		t.Fatalf("a bilingual file: %+v", none.Blocks)
	}
	if r := te.do("GET", fmt.Sprintf("/api/projects/%d/files/layout?name=Nichts.docx", p.ID), te.pm, nil); r.status != 404 {
		t.Fatalf("a file that is not there: %d", r.status)
	}
}
