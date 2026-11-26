package mqpackage

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter"
	"github.com/4nonX/interlinea/filter/xliff"
)

func pack(t testing.TB, files [][2]string) []byte {
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

func entries(t *testing.T, data []byte) map[string]string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, f := range zr.File {
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		rc.Close()
		out[f.Name] = string(b)
	}
	return out
}

const info = `<?xml version="1.0" encoding="utf-8"?>
<HandoffPackageInfo xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <PackageId>5d0a8a7e-1111-2222-3333-444455556666</PackageId>
  <ProjectName>Mietvertrag Hofmann</ProjectName>
  <UserName>Dan</UserName>
  <Role>0</Role>
</HandoffPackageInfo>
`

func handoff(t testing.TB) ([]byte, string) {
	t.Helper()
	doc, err := os.ReadFile("../xliff/testdata/memoq.mqxliff")
	if err != nil {
		t.Fatal(err)
	}
	archive := pack(t, [][2]string{{"document.mqxliff", string(doc)}, {"skeleton.xml", "<skeleton/>"}})
	return pack(t, [][2]string{
		{"HandoffPackageInfo.xml", info},
		{"Documents/Vertrag.docx.mqxlz", string(archive)},
		{"Documents/Anlage.mqxliff", string(doc)},
		{"TM/Kanzlei.tmx", "<tmx/>"},
		{"TB/Begriffe.csv", "a;b"},
	}), string(doc)
}

func TestOpen(t *testing.T) {
	data, _ := handoff(t)
	p, err := Open(data)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "Mietvertrag Hofmann" || p.SourceLang == "" || len(p.TargetLangs) != 1 || p.Other != 3 {
		t.Errorf("package: %+v", p)
	}
	if len(p.Files) != 2 || p.Files[0].Path != "Documents/Anlage.mqxliff" || p.Files[1].Path != "Documents/Vertrag.docx.mqxlz" || !p.Files[1].Archive || p.Files[1].Lang != p.TargetLangs[0] {
		t.Errorf("files: %+v", p.Files)
	}
	if len(p.Memories) != 1 || p.Memories[0] != "TM/Kanzlei.tmx" {
		t.Errorf("memories: %v", p.Memories)
	}
}

func TestReadAndReturn(t *testing.T) {
	data, doc := handoff(t)
	got, err := Read(data, "Documents/Vertrag.docx.mqxlz")
	if err != nil || string(got) != doc {
		t.Fatalf("the document in its archive: %v", err)
	}
	if _, err := Read(data, "Documents/Fehlt.mqxlz"); !errors.Is(err, ErrNoFile) {
		t.Errorf("a missing document: %v", err)
	}
	ex, err := xliff.Extract(got)
	if err != nil {
		t.Fatal(err)
	}
	u := ex.Units[0]
	translated, err := xliff.Merge(got, map[string]xliff.Update{u.ID: {Target: content.Text("Übersetzt in Interlinea"), State: filter.StateTranslated}})
	if err != nil {
		t.Fatal(err)
	}
	back, err := Return(data, map[string][]byte{"Documents/Vertrag.docx.mqxlz": translated})
	if err != nil {
		t.Fatal(err)
	}
	e := entries(t, back)
	if len(e) != 3 {
		t.Fatalf("delivery holds %d entries: %v", len(e), e)
	}
	if e["Vertrag.docx.mqxliff"] != string(translated) || e["Anlage.mqxliff"] != doc {
		t.Error("the documents are not the translated one and the other as it was")
	}
	want := `<?xml version="1.0" encoding="utf-8"?>
<HandoffDeliveryPackageInfo xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <PackageId>5d0a8a7e-1111-2222-3333-444455556666</PackageId>
  <ProjectName>Mietvertrag Hofmann</ProjectName>
  <User>Dan</User>
  <Role>0</Role>
</HandoffDeliveryPackageInfo>
`
	if e["DeliveryPackageInfo.xml"] != want {
		t.Errorf("delivery description:\n%s", e["DeliveryPackageInfo.xml"])
	}
	if _, err := Return(data, map[string][]byte{"TM/Kanzlei.tmx": nil}); !errors.Is(err, ErrNoFile) {
		t.Errorf("a file that is no document: %v", err)
	}
}

func TestDescriptionInsideAnotherElement(t *testing.T) {
	raw := `<?xml version="1.0" encoding="utf-8"?><Package xmlns:m="urn:m"><Created>x</Created><m:HandoffPackageInfo><m:UserName>Dan</m:UserName><Docs/></m:HandoffPackageInfo><More/></Package>`
	got, err := delivery([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	want := `<?xml version="1.0" encoding="utf-8"?>` + "\n" + `<m:HandoffDeliveryPackageInfo xmlns:m="urn:m"><m:User>Dan</m:User><Docs/></m:HandoffDeliveryPackageInfo>` + "\n"
	if string(got) != want {
		t.Errorf("got %s", got)
	}
}

func TestArchive(t *testing.T) {
	doc := `<xliff/>`
	archive := pack(t, [][2]string{{"skeleton.xml", "<skeleton/>"}, {"document.mqxliff", doc}})
	if got, err := ReadArchive(archive); err != nil || string(got) != doc {
		t.Fatalf("read: %q %v", got, err)
	}
	out, err := WriteArchive(archive, []byte("<xliff>neu</xliff>"))
	if err != nil {
		t.Fatal(err)
	}
	e := entries(t, out)
	if e["document.mqxliff"] != "<xliff>neu</xliff>" || e["skeleton.xml"] != "<skeleton/>" || len(e) != 2 {
		t.Errorf("written: %v", e)
	}
	if _, err := ReadArchive(pack(t, [][2]string{{"a.txt", "x"}})); !errors.Is(err, ErrNotArchive) {
		t.Errorf("a ZIP without a document: %v", err)
	}
}

func TestNotAPackage(t *testing.T) {
	if _, err := Open([]byte("no")); !errors.Is(err, ErrNotPackage) {
		t.Errorf("plain bytes: %v", err)
	}
	if _, err := Open(pack(t, [][2]string{{"a.txt", "x"}})); !errors.Is(err, ErrNotPackage) {
		t.Errorf("a ZIP without documents: %v", err)
	}
	data := pack(t, [][2]string{{"Documents/x.mqxliff", strings.Repeat(" ", 3)}})
	if _, err := Open(data); err == nil {
		t.Error("a document that is not XLIFF was accepted")
	}
}
