package sdlpackage

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter"
	"github.com/4nonX/interlinea/filter/xliff"
)

const manifest = "\ufeff" + `<?xml version="1.0" encoding="utf-8"?>
<PackageProject xmlns:xsd="http://www.w3.org/2001/XMLSchema" Guid="11111111-1111-1111-1111-111111111111" PackageCreatedAt="2026-11-20T10:00:00Z" PackageCreatedBy="PM" Comment="" PackageGuid="22222222-2222-2222-2222-222222222222" PackageType="ProjectPackage" Version="4.0.0.0" Name="Mietvertrag Hofmann">
  <LanguageDirections>
    <LanguageDirection Guid="33333333-3333-3333-3333-333333333333" TargetLanguageCode="en-GB" SourceLanguageCode="de-DE" />
  </LanguageDirections>
  <ProjectFiles />
</PackageProject>`

func sdlxliff(t testing.TB) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "xliff", "testdata", "trados.sdlxliff"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func makePackage(t testing.TB, project string, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name string, b []byte) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write(b)
	}
	if project != "" {
		add("Mietvertrag Hofmann.sdlproj", []byte(project))
	}
	for _, name := range []string{"de-DE/Mietvertrag.docx.sdlxliff", "en-GB/Mietvertrag.docx.sdlxliff", "en-GB/Anlage.docx.sdlxliff", "Tm/Weber.sdltm", "Termbases/Weber.sdltb"} {
		if b, ok := files[name]; ok {
			add(name, b)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func entries(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]byte{}
	for _, f := range zr.File {
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		rc.Close()
		out[f.Name] = b
	}
	return out
}

func TestPackage(t *testing.T) {
	x := sdlxliff(t)
	pkg := makePackage(t, manifest, map[string][]byte{
		"de-DE/Mietvertrag.docx.sdlxliff": x,
		"en-GB/Mietvertrag.docx.sdlxliff": x,
		"en-GB/Anlage.docx.sdlxliff":      x,
		"Tm/Weber.sdltm":                  []byte("SQLite format 3"),
		"Termbases/Weber.sdltb":           []byte("access"),
	})
	p, err := Open(pkg)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "Mietvertrag Hofmann" || p.Type != "ProjectPackage" || p.Guid != "22222222-2222-2222-2222-222222222222" || p.SourceLang != "de-DE" || strings.Join(p.TargetLangs, ",") != "en-GB" || p.Other != 2 {
		t.Fatalf("package: %+v", p)
	}
	if len(p.Files) != 3 || !p.Files[0].Target || p.Files[0].Path != "en-GB/Anlage.docx.sdlxliff" || p.Files[2].Target || p.Files[2].Lang != "de-DE" {
		t.Fatalf("files: %+v", p.Files)
	}
	b, err := Read(pkg, "en-GB/Mietvertrag.docx.sdlxliff")
	if err != nil || !bytes.Equal(b, x) {
		t.Fatalf("read: %v", err)
	}
	if _, err := Read(pkg, "nope"); !errors.Is(err, ErrNoFile) {
		t.Fatalf("missing file: %v", err)
	}

	translated, err := xliff.Merge(x, map[string]xliff.Update{"1/u1/2": {Target: content.Text("The deposit"), State: filter.StateTranslated}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Return(pkg, map[string][]byte{"de-DE/Mietvertrag.docx.sdlxliff": translated}); !errors.Is(err, ErrNoFile) {
		t.Fatalf("source file accepted as translation: %v", err)
	}
	rpx, err := Return(pkg, map[string][]byte{"en-GB/Mietvertrag.docx.sdlxliff": translated})
	if err != nil {
		t.Fatal(err)
	}
	got := entries(t, rpx)
	if len(got) != 4 || got["Tm/Weber.sdltm"] != nil || got["Termbases/Weber.sdltb"] != nil {
		t.Fatalf("return package entries: %v", keys(got))
	}
	if !bytes.Equal(got["en-GB/Mietvertrag.docx.sdlxliff"], translated) || !bytes.Equal(got["en-GB/Anlage.docx.sdlxliff"], x) || !bytes.Equal(got["de-DE/Mietvertrag.docx.sdlxliff"], x) {
		t.Fatal("return package files differ")
	}
	wantProj := strings.Replace(manifest, `PackageType="ProjectPackage"`, `PackageType="ReturnPackage"`, 1)
	if string(got["Mietvertrag Hofmann.sdlproj"]) != wantProj {
		t.Fatalf("project file:\n%s", got["Mietvertrag Hofmann.sdlproj"])
	}
	rp, err := Open(rpx)
	if err != nil || rp.Type != "ReturnPackage" || rp.Guid != p.Guid {
		t.Fatalf("return package reads back: %+v %v", rp, err)
	}
}

func keys(m map[string][]byte) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestInferredLanguagesAndErrors(t *testing.T) {
	x := sdlxliff(t)
	bare := strings.Replace(manifest, "<LanguageDirections>\n    <LanguageDirection Guid=\"33333333-3333-3333-3333-333333333333\" TargetLanguageCode=\"en-GB\" SourceLanguageCode=\"de-DE\" />\n  </LanguageDirections>", "", 1)
	pkg := makePackage(t, bare, map[string][]byte{"de-DE/Mietvertrag.docx.sdlxliff": x, "en-GB/Mietvertrag.docx.sdlxliff": x})
	p, err := Open(pkg)
	if err != nil {
		t.Fatal(err)
	}
	if p.SourceLang != "de-DE" || strings.Join(p.TargetLangs, ",") != "en-GB" || !p.Files[0].Target || p.Files[1].Target {
		t.Fatalf("inferred: %+v", p)
	}
	if _, err := Open(makePackage(t, "", map[string][]byte{"en-GB/Mietvertrag.docx.sdlxliff": x})); !errors.Is(err, ErrNotPackage) {
		t.Fatalf("no project file: %v", err)
	}
	if _, err := Open([]byte("not a zip")); err == nil {
		t.Fatal("not a zip accepted")
	}
	plain := strings.Replace(manifest, "PackageProject", "Project", 2)
	if _, err := Return(makePackage(t, plain, map[string][]byte{"en-GB/Mietvertrag.docx.sdlxliff": x}), nil); err == nil {
		t.Fatal("a plain project file accepted for a return package")
	}
}
