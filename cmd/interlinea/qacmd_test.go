package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestQACommand(t *testing.T) {
	dir := t.TempDir()
	in, err := os.ReadFile(filepath.Join("..", "..", "filter", "xliff", "testdata", "trados.sdlxliff"))
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "a.sdlxliff")
	os.WriteFile(src, in, 0o644)
	// An English dictionary for the translations, as a zip like a
	// LibreOffice extension.
	oxt := filepath.Join(dir, "en.oxt")
	f, _ := os.Create(oxt)
	zw := zip.NewWriter(f)
	w, _ := zw.Create("en_GB.aff")
	w.Write([]byte("SET UTF-8\nSFX S Y 1\nSFX S 0 s .\n"))
	w, _ = zw.Create("en_GB.dic")
	w.Write([]byte("4\nthe\ntenant/S\npay/S\nchange\n"))
	zw.Close()
	f.Close()
	rules := filepath.Join(dir, "rules.json")
	os.WriteFile(rules, []byte(`[{"name": "No pays", "target": "pays", "mode": "must-not"}]`), 0o644)
	report := filepath.Join(dir, "qa.html")
	steps := []struct {
		args []string
		code int
		want string
	}{
		{[]string{"qa"}, 2, ""},
		{[]string{"qa", "-checks"}, 0, "spelling"},
		{[]string{"qa", "-format", "pdf", src}, 2, ""},
		{[]string{"qa", "-disable", "nope", src}, 2, ""},
		{[]string{"qa", src}, 1, "4 of 5 segments checked"},
		{[]string{"qa", src}, 1, "Empty translation"},
		{[]string{"qa", "-dict", oxt, src}, 1, "“Changed” is not in the dictionary. Suggestions: Change"},
		{[]string{"qa", "-dict", oxt, "-rules", rules, src}, 1, "No pays"},
		{[]string{"qa", "-disable", "empty,tags-missing", src}, 0, "0 errors"},
		{[]string{"qa", "-format", "csv", src}, 1, "Segment,Severity,Check"},
		{[]string{"qa", "-format", "json", src}, 1, `"kind": "empty"`},
		{[]string{"qa", "-format", "html", "-o", report, src}, 1, ""},
	}
	for _, s := range steps {
		code, stdout, errOut := exec(t, "", s.args...)
		if code != s.code || !strings.Contains(stdout, s.want) {
			t.Fatalf("%v: exit %d (want %d)\nstdout: %s\nstderr: %s", s.args, code, s.code, stdout, errOut)
		}
	}
	h, err := os.ReadFile(report)
	if err != nil || !strings.Contains(string(h), "QA report: a.sdlxliff") {
		t.Fatalf("html report: %v", err)
	}
}

func TestSpellCommand(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "de_DE")
	os.WriteFile(base+".aff", []byte("SET UTF-8\nTRY aeinrst\nSFX N Y 1\nSFX N 0 en .\n"), 0o644)
	os.WriteFile(base+".dic", []byte("2\nKaution/N\nMiete/N\n"), 0o644)
	code, out, _ := exec(t, "", "spell", "-dict", base+".dic", "Kautionen", "Kauton", "Miete.")
	if code != 1 || out != "wrong\tKauton\tKaution\n" {
		t.Fatalf("spell: %d %q", code, out)
	}
	code, out, _ = exec(t, "Kaution Kautionen", "spell", "-dict", base, "-all")
	if code != 0 || out != "ok\tKaution\nok\tKautionen\n" {
		t.Fatalf("spell stdin: %d %q", code, out)
	}
	if code, _, _ := exec(t, "", "spell", "x"); code != 2 {
		t.Fatal("missing -dict accepted")
	}
}
