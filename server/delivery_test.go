package server

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter/docx"
	"github.com/4nonX/interlinea/filter/docx/docxtest"
	"github.com/4nonX/interlinea/filter/sdlpackage"
	"github.com/4nonX/interlinea/filter/xliff"
	"github.com/4nonX/interlinea/project"
	"github.com/4nonX/interlinea/project/projectdb"
)

func utf16LE(s string) []byte {
	out := []byte{0xFF, 0xFE}
	for _, c := range utf16.Encode([]rune(s)) {
		out = append(out, byte(c), byte(c>>8))
	}
	return out
}

func tradosFile(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "filter", "xliff", "testdata", "trados.sdlxliff"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

const packageManifest = "\ufeff" + `<?xml version="1.0" encoding="utf-8"?>
<PackageProject Guid="11111111-1111-1111-1111-111111111111" PackageGuid="22222222-2222-2222-2222-222222222222" PackageType="ProjectPackage" Version="4.0.0.0" Name="Mietvertrag Hofmann">
  <LanguageDirections>
    <LanguageDirection Guid="33333333-3333-3333-3333-333333333333" TargetLanguageCode="en-GB" SourceLanguageCode="de-DE" />
  </LanguageDirections>
  <ProjectFiles />
</PackageProject>`

func tradosPackage(t *testing.T, x []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, b := range map[string][]byte{
		"Mietvertrag Hofmann.sdlproj":     []byte(packageManifest),
		"de-DE/Mietvertrag.docx.sdlxliff": x,
		"en-GB/Mietvertrag.docx.sdlxliff": x,
	} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write(b)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func (te *team) download(path, token string) reply {
	te.t.Helper()
	r := te.do("GET", path, token, nil)
	if r.status != 200 {
		te.t.Fatalf("GET %s: %d %s", path, r.status, r.body)
	}
	return r
}

// The desktop app sends files without segments: the server makes them,
// and writes the translated files back.
func TestServerReadsAndWritesFiles(t *testing.T) {
	te := newTeam(t)
	lease := docxtest.Document(docxtest.Parts{Lang: "de-DE", Body: docxtest.P("", docxtest.R("Der Mieter zahlt "), docxtest.B("pünktlich"), docxtest.R(" die Miete. Die Kaution beträgt drei Monatsmieten.")) +
		docxtest.P("", docxtest.R("Der Vermieter haftet nicht."))})
	notes := utf16LE("Der Schlüssel liegt beim Hausmeister. Die Heizung läuft.\r\n\r\nDer Müll wird montags abgeholt.\r\n")
	req := leaseRequest()
	req.Files = []fileUpload{
		{Name: "Mietvertrag.docx", Role: project.Translatable, Data: lease},
		{Name: "Hinweise.txt", Role: project.Translatable, Data: notes},
		{Name: "Brief.sdlxliff", Role: project.Translatable, Data: tradosFile(t)},
	}
	var p projectView
	te.call("POST", "/api/projects", te.pm, req, 201, &p)
	base := fmt.Sprintf("/api/projects/%d", p.ID)
	if n := len(p.Settings.Files); n != 3 || p.Settings.Files[2].TargetLang != "en-GB" {
		t.Fatalf("files %+v", p.Settings.Files)
	}

	var word []projectdb.StoredSegment
	te.call("GET", base+"/segments?file=Mietvertrag.docx&lang=fr-FR", te.pm, nil, 200, &word)
	ex, _ := docx.Extract(lease, "de-DE", docx.Options{})
	if len(word) != 3 || word[0].ID != ex.Units[0].ID || word[0].Block == "" || !hasTag(word[0].Source) {
		t.Fatalf("Word segments %+v", word)
	}
	var text []projectdb.StoredSegment
	te.call("GET", base+"/segments?file=Hinweise.txt&lang=en-GB", te.pm, nil, 200, &text)
	if len(text) != 3 || text[0].Source.Plain() != "Der Schlüssel liegt beim Hausmeister." || text[1].Block != "p1" || text[2].Block != "p2" {
		t.Fatalf("text segments %+v", text)
	}
	var bi []projectdb.StoredSegment
	te.call("GET", base+"/segments?file=Brief.sdlxliff&lang=en-GB", te.pm, nil, 200, &bi)
	xex, _ := xliff.Extract(tradosFile(t))
	if len(bi) != len(xex.Units) || bi[0].Target.Plain() != "The tenant pays." || bi[0].Status != project.StatusConfirmed || bi[0].File == nil || bi[0].File.State != "translated" || bi[0].File.Match != 100 {
		t.Fatalf("bilingual segments %+v", bi[0])
	}
	if !bi[2].Locked || bi[2].File.State != "final" {
		t.Fatalf("locked segment %+v", bi[2])
	}

	// Translate and write the files back.
	te.call("PUT", segPath(p.ID, "Mietvertrag.docx", "fr-FR", word[2].ID, 1), te.pm, project.Edit{Target: content.Text("Le bailleur n'est pas responsable."), Status: project.StatusConfirmed}, 200, nil)
	te.call("PUT", segPath(p.ID, "Hinweise.txt", "en-GB", "s3", 1), te.pm, project.Edit{Target: content.Text("Rubbish is collected on Mondays."), Status: project.StatusConfirmed}, 200, nil)
	te.call("PUT", segPath(p.ID, "Brief.sdlxliff", "en-GB", bi[1].ID, bi[1].Version), te.pm, project.Edit{Target: content.Text("The deposit"), Status: project.StatusConfirmed}, 200, nil)

	r := te.download(base+"/files/translated?name=Mietvertrag.docx&lang=fr-FR", te.pm)
	if !strings.Contains(r.header.Get("Content-Disposition"), "Mietvertrag (fr-FR).docx") {
		t.Fatalf("disposition %q", r.header.Get("Content-Disposition"))
	}
	back, err := docx.Extract(r.body, "fr-FR", docx.Options{})
	if err != nil {
		t.Fatal(err)
	}
	all := ""
	for _, u := range back.Units {
		all += u.Source.Plain() + "|"
	}
	if !strings.Contains(all, "Le bailleur n'est pas responsable.") || !strings.Contains(all, "Der Mieter zahlt") {
		t.Fatalf("translated document: %s", all)
	}

	r = te.download(base+"/files/translated?name=Hinweise.txt&lang=en-GB", te.pm)
	got, bom, err := decodeText(r.body)
	if err != nil || len(bom) != 2 || r.header.Get("Content-Type") != "text/plain; charset=utf-16" {
		t.Fatalf("text: %v %x %s", err, bom, r.header.Get("Content-Type"))
	}
	if got != "Der Schlüssel liegt beim Hausmeister. Die Heizung läuft.\r\n\r\nRubbish is collected on Mondays.\r\n" {
		t.Fatalf("translated text %q", got)
	}

	r = te.download(base+"/files/translated?name=Brief.sdlxliff", te.pm)
	bx, err := xliff.Extract(r.body)
	if err != nil {
		t.Fatal(err)
	}
	if bx.Units[1].Target.Plain() != "The deposit" || bx.Units[1].State != "translated" || bx.Units[0].State != "translated" || bx.Units[2].State != "final" {
		t.Fatalf("bilingual file back %+v", bx.Units)
	}
	if r := te.do("GET", base+"/return-package", te.pm, nil); r.status != 404 {
		t.Fatalf("return package of a project without one: %d", r.status)
	}
}

func hasTag(c content.Content) bool {
	for _, r := range c {
		if r.IsTag() {
			return true
		}
	}
	return false
}

// A Trados package starts a project with its files; the project returns
// the return package.
func TestTradosPackageProject(t *testing.T) {
	te := newTeam(t)
	x := tradosFile(t)
	req := leaseRequest()
	req.Settings.TargetLangs = []string{"en-GB"}
	delete(req.Settings.Memories, "fr-FR")
	req.Memories = req.Memories[:1]
	req.Files = []fileUpload{{Name: "Hofmann.sdlppx", Role: project.Package, Data: tradosPackage(t, x)}}
	var p projectView
	te.call("POST", "/api/projects", te.pm, req, 201, &p)
	fs := p.Settings.Files
	if len(fs) != 2 || fs[0].Role != project.Package || fs[1].Name != "en-GB/Mietvertrag.docx.sdlxliff" || fs[1].Package != "en-GB/Mietvertrag.docx.sdlxliff" || fs[1].TargetLang != "en-GB" {
		t.Fatalf("files %+v", fs)
	}
	base := fmt.Sprintf("/api/projects/%d", p.ID)
	var segs []projectdb.StoredSegment
	te.call("GET", base+"/segments?file="+strings.ReplaceAll(fs[1].Name, "/", "%2F")+"&lang=en-GB", te.pm, nil, 200, &segs)
	te.call("PUT", segPath(p.ID, fs[1].Name, "en-GB", segs[1].ID, segs[1].Version), te.pm, project.Edit{Target: content.Text("The deposit"), Status: project.StatusConfirmed}, 200, nil)

	// Settings sent without the package paths keep them.
	set := p.Settings
	for i := range set.Files {
		set.Files[i].Package = ""
	}
	set.Description = "From Trados"
	var got projectView
	te.call("PUT", base+"/settings?version="+fmt.Sprint(p.Version), te.pm, set, 200, &got)
	if got.Settings.Files[1].Package == "" {
		t.Fatalf("package path lost: %+v", got.Settings.Files)
	}

	r := te.download(base+"/return-package", te.pm)
	if !strings.Contains(r.header.Get("Content-Disposition"), "Hofmann.sdlrpx") {
		t.Fatalf("disposition %q", r.header.Get("Content-Disposition"))
	}
	rp, err := sdlpackage.Open(r.body)
	if err != nil || rp.Type != "ReturnPackage" {
		t.Fatalf("return package %+v %v", rp, err)
	}
	b, err := sdlpackage.Read(r.body, "en-GB/Mietvertrag.docx.sdlxliff")
	if err != nil {
		t.Fatal(err)
	}
	bx, _ := xliff.Extract(b)
	if bx.Units[1].Target.Plain() != "The deposit" {
		t.Fatalf("returned file %+v", bx.Units[1])
	}
	zr, _ := zip.NewReader(bytes.NewReader(r.body), int64(len(r.body)))
	for _, f := range zr.File {
		if f.Name == "de-DE/Mietvertrag.docx.sdlxliff" {
			rc, _ := f.Open()
			src, _ := io.ReadAll(rc)
			rc.Close()
			if !bytes.Equal(src, x) {
				t.Fatal("the source-language file changed")
			}
		}
	}

	// A package doesn't join a project later.
	nf := fileUpload{Name: "Other.sdlppx", Role: project.Package, Data: tradosPackage(t, x)}
	if r := te.do("POST", base+"/files?version="+fmt.Sprint(got.Version), te.pm, nf); r.status != 422 {
		t.Fatalf("package added later: %d %s", r.status, r.body)
	}
}
