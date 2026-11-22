package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testTMX = `<?xml version="1.0" encoding="UTF-8"?>
<tmx version="1.4"><header creationtool="t" creationtoolversion="1" segtype="sentence" o-tmf="x" adminlang="en" srclang="de-DE" datatype="plaintext"/>
<body>
<tu><tuv xml:lang="de-DE"><seg>Der Mieter haftet für alle Schäden an der Mietsache.</seg></tuv><tuv xml:lang="en-GB"><seg>The tenant is liable for all damage to the leased property.</seg></tuv></tu>
<tu><tuv xml:lang="de-DE"><seg>Die Kaution beträgt <ph x="1">{amount}</ph>.</seg></tuv><tuv xml:lang="en-GB"><seg>The deposit is <ph x="1">{amount}</ph>.</seg></tuv></tu>
</body></tmx>`

func exec(t *testing.T, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run(args, strings.NewReader(stdin), &out, &errOut)
	return code, out.String(), errOut.String()
}

func writeTMX(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "test.tmx")
	if err := os.WriteFile(p, []byte(testTMX), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestUsageAndUnknown(t *testing.T) {
	if code, _, errOut := exec(t, ""); code != 2 || !strings.Contains(errOut, "Commands:") {
		t.Fatalf("no args: %d %q", code, errOut)
	}
	if code, _, _ := exec(t, "", "help"); code != 0 {
		t.Fatal("help should exit 0")
	}
	if code, _, errOut := exec(t, "", "frobnicate"); code != 2 || !strings.Contains(errOut, "unknown command") {
		t.Fatalf("unknown: %d %q", code, errOut)
	}
	if code, out, _ := exec(t, "", "version"); code != 0 || !strings.HasPrefix(out, "interlinea ") {
		t.Fatalf("version: %d %q", code, out)
	}
	if code, _, _ := exec(t, "", "segment", "-h"); code != 0 {
		t.Fatal("-h should exit 0")
	}
}

func TestSegmentCommand(t *testing.T) {
	in := "Herr Dr. Weber kommt. Er bleibt z.B. zwei Tage.\n\nZweiter Absatz. Ende."
	code, out, errOut := exec(t, in, "segment", "-lang", "de-DE")
	if code != 0 {
		t.Fatal(errOut)
	}
	want := "Herr Dr. Weber kommt.\nEr bleibt z.B. zwei Tage.\n\nZweiter Absatz.\nEnde.\n"
	if out != want {
		t.Fatalf("got %q\nwant %q", out, want)
	}
	code, out, _ = exec(t, in, "segment", "-lang", "de", "-json")
	var parsed [][]string
	if code != 0 || json.Unmarshal([]byte(out), &parsed) != nil || len(parsed) != 2 || len(parsed[0]) != 2 {
		t.Fatalf("json: %d %q", code, out)
	}
	if code, _, _ := exec(t, in, "segment", "-lang", "!!"); code != 2 {
		t.Fatal("invalid language accepted")
	}
	if code, _, _ := exec(t, "", "segment", "/does/not/exist"); code != 1 {
		t.Fatal("missing file should exit 1")
	}
}

func TestTMXInfo(t *testing.T) {
	p := writeTMX(t)
	code, out, errOut := exec(t, "", "tmx-info", p)
	if code != 0 {
		t.Fatal(errOut)
	}
	for _, want := range []string{"Source language: de-DE", "Units:           2 (1 with inline tags)", "en-GB      2"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	if code, _, _ := exec(t, "", "tmx-info"); code != 2 {
		t.Fatal("missing argument accepted")
	}
}

func TestLookupAndConcordance(t *testing.T) {
	p := writeTMX(t)
	code, out, errOut := exec(t, "", "lookup", "-tmx", p, "-tgt", "en-GB", "Der Vermieter haftet für alle Schäden an der Mietsache.")
	if code != 0 {
		t.Fatal(errOut)
	}
	if !strings.Contains(out, "The tenant is liable") || strings.Contains(out, "100%") {
		t.Fatalf("lookup output:\n%s", out)
	}
	code, out, _ = exec(t, "", "lookup", "-tmx", p, "-tgt", "en-GB", "-json", "Der Mieter haftet für alle Schäden an der Mietsache.")
	var ms []jsonMatch
	if code != 0 || json.Unmarshal([]byte(out), &ms) != nil || len(ms) != 1 || ms[0].Score != 100 {
		t.Fatalf("json lookup: %d %q", code, out)
	}
	if code, out, _ := exec(t, "", "lookup", "-tmx", p, "-tgt", "en-GB", "Etwas ganz anderes."); code != 0 || !strings.Contains(out, "No matches.") {
		t.Fatalf("no match: %q", out)
	}
	if code, _, _ := exec(t, "", "lookup", "-tmx", p, "x"); code != 2 {
		t.Fatal("missing -tgt accepted")
	}
	if code, _, _ := exec(t, "", "lookup", "-tmx", p, "-tgt", "en", "-min", "0", "x"); code != 2 {
		t.Fatal("invalid -min accepted")
	}

	code, out, errOut = exec(t, "", "concordance", "-tmx", p, "-tgt", "en-GB", "kaution")
	if code != 0 || !strings.Contains(out, "The deposit is {ph1/}.") {
		t.Fatalf("concordance: %d %q %q", code, out, errOut)
	}
	code, out, _ = exec(t, "", "concordance", "-tmx", p, "-tgt", "en-GB", "-target", "leased property")
	if code != 0 || !strings.Contains(out, "Mietsache") {
		t.Fatalf("target concordance: %q", out)
	}
}

func TestScoreCommand(t *testing.T) {
	code, out, _ := exec(t, "", "score", "Der Mieter haftet.", "Der Mieter haftet.")
	if code != 0 || out != "100%\n" {
		t.Fatalf("score: %d %q", code, out)
	}
	if code, _, _ := exec(t, "", "score", "one"); code != 2 {
		t.Fatal("single argument accepted")
	}
}

func TestTMCommands(t *testing.T) {
	p := writeTMX(t)
	db := filepath.Join(t.TempDir(), "mem.db")
	steps := []struct {
		args []string
		code int
		want string
	}{
		{[]string{"tm"}, 2, ""},
		{[]string{"tm", "nope"}, 2, ""},
		{[]string{"tm", "list", "-db", db}, 0, "No memories"},
		{[]string{"tm", "create", "-db", db, "-name", "Mietrecht", "-src", "de_de", "-tgt", "en-gb"}, 0, `Created "Mietrecht" (de-DE → en-GB)`},
		{[]string{"tm", "create", "-db", db, "-name", "Mietrecht", "-src", "de", "-tgt", "en"}, 1, ""},
		{[]string{"tm", "create", "-db", db, "-name", "x"}, 2, ""},
		{[]string{"tm", "import", "-db", db, "-name", "Mietrecht", p}, 0, "Read 2 units: 2 stored, 0 skipped"},
		{[]string{"tm", "import", "-db", db, "-name", "Fehlt", p}, 1, ""},
		{[]string{"tm", "list", "-db", db}, 0, "2 units"},
		{[]string{"tm", "lookup", "-db", db, "-name", "Mietrecht", "Der Vermieter haftet für alle Schäden an der Mietsache."}, 0, "The tenant is liable"},
		{[]string{"tm", "concordance", "-db", db, "-name", "Mietrecht", "kaution"}, 0, "The deposit is"},
		{[]string{"tm", "export", "-db", db, "-name", "Mietrecht"}, 0, `<tuv xml:lang="en-GB">`},
		{[]string{"tm", "delete", "-db", db, "-name", "Mietrecht"}, 2, ""},
		{[]string{"tm", "delete", "-db", db, "-name", "Mietrecht", "-yes"}, 0, `Deleted "Mietrecht" (2 units)`},
		{[]string{"tm", "list", "-db", db}, 0, "No memories"},
	}
	for _, s := range steps {
		code, out, errOut := exec(t, "", s.args...)
		if code != s.code || !strings.Contains(out, s.want) {
			t.Fatalf("%v: exit %d (want %d)\nstdout: %s\nstderr: %s", s.args, code, s.code, out, errOut)
		}
	}
	code, out, _ := exec(t, "", "tm", "list", "-db", db, "-json")
	if code != 0 || strings.TrimSpace(out) != "[]" {
		t.Fatalf("empty json list: %q", out)
	}
}

func TestTMExportToFile(t *testing.T) {
	p := writeTMX(t)
	dir := t.TempDir()
	db := filepath.Join(dir, "mem.db")
	out := filepath.Join(dir, "out.tmx")
	for _, args := range [][]string{
		{"tm", "create", "-db", db, "-name", "a", "-src", "de-DE", "-tgt", "en-GB"},
		{"tm", "import", "-db", db, "-name", "a", p},
		{"tm", "export", "-db", db, "-name", "a", "-o", out},
	} {
		if code, _, errOut := exec(t, "", args...); code != 0 {
			t.Fatalf("%v: %s", args, errOut)
		}
	}
	code, info, _ := exec(t, "", "tmx-info", out)
	if code != 0 || !strings.Contains(info, "Units:           2 (1 with inline tags)") {
		t.Fatalf("exported file: %s", info)
	}
}

func TestDefaultDBPath(t *testing.T) {
	t.Setenv("INTERLINEA_DB", "/tmp/custom.db")
	if got := defaultDBPath(); got != "/tmp/custom.db" {
		t.Fatalf("env override = %q", got)
	}
	t.Setenv("INTERLINEA_DB", "")
	if got := defaultDBPath(); !strings.HasSuffix(got, filepath.Join("Interlinea", "memories.db")) {
		t.Fatalf("default = %q", got)
	}
}
