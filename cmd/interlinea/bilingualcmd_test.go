package main

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestXLIFFCommands(t *testing.T) {
	dir := t.TempDir()
	in, err := os.ReadFile(filepath.Join("..", "..", "filter", "xliff", "testdata", "trados.sdlxliff"))
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "a.sdlxliff")
	if err := os.WriteFile(src, in, 0o644); err != nil {
		t.Fatal(err)
	}
	up := filepath.Join(dir, "up.json")
	os.WriteFile(up, []byte(`{"1/u1/2": "The deposit", "1/u2/4": {"target": [{"text": "New "}, {"tag": {"kind": "standalone", "id": "pt2"}}, {"text": "sentence."}], "state": "draft"}, "1/u1/1": {"state": "final"}}`), 0o644)
	out := filepath.Join(dir, "b.sdlxliff")
	steps := []struct {
		args []string
		code int
		want string
	}{
		{[]string{"xliff"}, 2, ""},
		{[]string{"xliff", "info", src}, 0, "sdlxliff"},
		{[]string{"xliff", "info", src}, 0, "Segments:  5 (1 locked)"},
		{[]string{"xliff", "extract", "-state", "new", src}, 0, "1/u2/4 [new]\n  Neuer {pt2/}Satz."},
		{[]string{"xliff", "extract", "-state", "nope", src}, 2, ""},
		{[]string{"xliff", "extract", "-json", src}, 0, `"format": "sdlxliff"`},
		{[]string{"xliff", "merge", src, up}, 2, ""},
		{[]string{"xliff", "merge", "-o", out, src, up}, 0, "3 segments updated"},
		{[]string{"xliff", "extract", out}, 0, "1/u2/4 [draft]\n  Neuer {pt2/}Satz.\n  New {pt2/}sentence."},
		{[]string{"xliff", "extract", out}, 0, "1/u1/1 [final 100%]"},
		{[]string{"xliff", "info", up}, 1, ""},
	}
	for _, s := range steps {
		code, stdout, errOut := exec(t, "", s.args...)
		if code != s.code || !strings.Contains(stdout, s.want) {
			t.Fatalf("%v: exit %d (want %d)\nstdout: %s\nstderr: %s", s.args, code, s.code, stdout, errOut)
		}
	}
	os.WriteFile(up, []byte(`{"1/u1/3": "x"}`), 0o644)
	if code, _, errOut := exec(t, "", "xliff", "merge", "-o", out, src, up); code != 1 || !strings.Contains(errOut, "locked") {
		t.Fatalf("locked segment: %d %s", code, errOut)
	}
}

func TestPackageCommands(t *testing.T) {
	dir := t.TempDir()
	x, err := os.ReadFile(filepath.Join("..", "..", "filter", "xliff", "testdata", "trados.sdlxliff"))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, b := range map[string][]byte{
		"P.sdlproj":             []byte(`<?xml version="1.0" encoding="utf-8"?><PackageProject PackageGuid="g" PackageType="ProjectPackage" Name="P"><LanguageDirections><LanguageDirection TargetLanguageCode="en-GB" SourceLanguageCode="de-DE"/></LanguageDirections></PackageProject>`),
		"de-DE/a.docx.sdlxliff": x,
		"en-GB/a.docx.sdlxliff": x,
		"Tm/x.sdltm":            []byte("tm"),
	} {
		w, _ := zw.Create(name)
		w.Write(b)
	}
	zw.Close()
	ppx := filepath.Join(dir, "p.sdlppx")
	os.WriteFile(ppx, buf.Bytes(), 0o644)
	work := filepath.Join(dir, "work")
	rpx := filepath.Join(dir, "p.sdlrpx")
	up := filepath.Join(dir, "up.json")
	os.WriteFile(up, []byte(`{"1/u1/2": "The deposit"}`), 0o644)
	tgt := filepath.Join(work, "en-GB", "a.docx.sdlxliff")
	steps := []struct {
		args []string
		code int
		want string
	}{
		{[]string{"package"}, 2, ""},
		{[]string{"package", "info", ppx}, 0, "en-GB/a.docx.sdlxliff"},
		{[]string{"package", "info", ppx}, 0, "2 of 5 segments done"},
		{[]string{"package", "return", "-o", rpx, ppx, work}, 1, ""},
		{[]string{"package", "extract", "-o", work, ppx}, 0, "wrote 1 files"},
		{[]string{"xliff", "merge", "-o", tgt, tgt, up}, 0, "1 segments updated"},
		{[]string{"package", "return", "-o", rpx, ppx, work}, 0, "1 translated files"},
		{[]string{"package", "info", rpx}, 0, "(ReturnPackage)"},
		{[]string{"package", "info", rpx}, 0, "3 of 5 segments done"},
	}
	for _, s := range steps {
		code, stdout, errOut := exec(t, "", s.args...)
		if code != s.code || !strings.Contains(stdout, s.want) {
			t.Fatalf("%v: exit %d (want %d)\nstdout: %s\nstderr: %s", s.args, code, s.code, stdout, errOut)
		}
	}
}
