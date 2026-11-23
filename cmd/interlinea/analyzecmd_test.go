package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAnalyzeCommand(t *testing.T) {
	dir := t.TempDir()
	in, err := os.ReadFile(filepath.Join("..", "..", "filter", "xliff", "testdata", "trados.sdlxliff"))
	if err != nil {
		t.Fatal(err)
	}
	a, b := filepath.Join(dir, "a.sdlxliff"), filepath.Join(dir, "b.sdlxliff")
	os.WriteFile(a, in, 0o644)
	os.WriteFile(b, in, 0o644)
	tmx := filepath.Join(dir, "m.tmx")
	os.WriteFile(tmx, []byte(`<?xml version="1.0" encoding="UTF-8"?>
<tmx version="1.4"><header creationtool="t" creationtoolversion="1" segtype="sentence" o-tmf="x" adminlang="en" srclang="de-DE" datatype="plaintext"/><body>
<tu><tuv xml:lang="de-DE"><seg>Der Mieter zahlt.</seg></tuv><tuv xml:lang="en-GB"><seg>The tenant pays.</seg></tuv></tu>
</body></tmx>`), 0o644)
	html := filepath.Join(dir, "a.html")
	steps := []struct {
		args []string
		code int
		want string
	}{
		{[]string{"analyze"}, 2, ""},
		{[]string{"analyze", "-format", "pdf", a}, 2, ""},
		{[]string{"analyze", "-weights", "nope=1", a}, 2, ""},
		{[]string{"analyze", "-context", "next", a}, 1, ""},
		{[]string{"analyze", filepath.Join(dir, "missing.docx")}, 1, ""},
		{[]string{"analyze", a}, 0, "Total         5     10"},
		{[]string{"analyze", "-tmx", tmx, a}, 0, "100%         1      3"},
		{[]string{"analyze", "-tmx", tmx, a, b}, 0, "Cross-file repetitions         4"},
		{[]string{"analyze", "-weights", "100=30", "-tmx", tmx, a}, 0, "Weighted"},
		{[]string{"analyze", "-format", "csv", a}, 0, "File,Band,Segments"},
		{[]string{"analyze", "-format", "json", a}, 0, `"band": "locked"`},
		{[]string{"analyze", "-format", "html", "-o", html, a}, 0, ""},
	}
	for _, s := range steps {
		code, stdout, errOut := exec(t, "", s.args...)
		if code != s.code || !strings.Contains(stdout, s.want) {
			t.Fatalf("%v: exit %d (want %d)\nstdout: %s\nstderr: %s", s.args, code, s.code, stdout, errOut)
		}
	}
	if h, err := os.ReadFile(html); err != nil || !strings.Contains(string(h), "<h1>Analysis</h1>") {
		t.Fatalf("html: %v", err)
	}
}
