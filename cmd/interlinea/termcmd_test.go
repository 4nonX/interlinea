package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/4nonX/interlinea/term"
)

const termCSV = "ID;Deutsch;Englisch;Status Englisch;Definition\r\n" +
	"1;Mietvertrag;lease agreement;approved;Vertrag über die Gebrauchsüberlassung\r\n" +
	"2;Mieter;tenant;;\r\n" +
	"3;Mieter;lessee;forbidden;\r\n"

func TestTermCommands(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "glossar.csv")
	if err := os.WriteFile(src, []byte(termCSV), 0o644); err != nil {
		t.Fatal(err)
	}

	code, out, errOut := exec(t, "", "term", "info", src)
	if code != 0 || !strings.Contains(out, "Entries:   3") || !strings.Contains(out, "Terms:     6") || !strings.Contains(out, "de ") {
		t.Fatalf("info: %d %q %q", code, out, errOut)
	}

	code, out, errOut = exec(t, "", "term", "mapping", src)
	var m term.Mapping
	if code != 0 || json.Unmarshal([]byte(out), &m) != nil || len(m.Columns) != 5 || m.Columns[0].Role != term.RoleID {
		t.Fatalf("mapping: %d %q %q", code, out, errOut)
	}

	code, out, errOut = exec(t, "", "term", "duplicates", "-lang", "de", src)
	if code != 0 || !strings.Contains(out, "1 groups") || !strings.Contains(out, "de:Mieter") {
		t.Fatalf("duplicates: %d %q %q", code, out, errOut)
	}

	for _, name := range []string{"out.tbx", "out.xml", "out.xlsx", "out.tsv"} {
		dst := filepath.Join(dir, name)
		code, out, errOut = exec(t, "", "term", "convert", src, dst)
		if code != 0 || !strings.Contains(out, "3 entries, 6 terms") {
			t.Fatalf("convert %s: %d %q %q", name, code, out, errOut)
		}
		code, out, errOut = exec(t, "", "term", "info", dst)
		if code != 0 || !strings.Contains(out, "Entries:   3") || !strings.Contains(out, "Terms:     6") {
			t.Fatalf("info %s: %d %q %q", name, code, out, errOut)
		}
	}

	tbx2 := filepath.Join(dir, "out2.tbx")
	if code, _, errOut = exec(t, "", "term", "convert", "-to", "tbx2", src, tbx2); code != 0 {
		t.Fatalf("convert tbx2: %q", errOut)
	}
	if b, _ := os.ReadFile(tbx2); !strings.Contains(string(b), "<martif") {
		t.Fatal("tbx2 output is not TBX 2")
	}

	if code, _, _ = exec(t, "", "term", "convert", src, filepath.Join(dir, "x.xdt")); code != 2 {
		t.Fatalf("writing xdt should be a usage error, got %d", code)
	}
	if code, _, _ = exec(t, "", "term", "convert", src, filepath.Join(dir, "x.bin")); code != 2 {
		t.Fatalf("unknown extension should be a usage error, got %d", code)
	}
	code, out, errOut = exec(t, "", "term", "recognize", "-src", "de-DE", "-tgt", "en-GB", "-target", "The lessee pays the deposit.", src, "Die Mieter zahlen die Kaution.")
	want := "Mieter\tMieter\ttenant\nMieter\tMieter\tlessee (forbidden)\nforbidden: \"lessee\"\nmissing: \"Mieter\" is usually \"tenant\"\n"
	if code != 0 || out != want {
		t.Fatalf("recognize: %d %q %q", code, out, errOut)
	}
	if code, _, _ = exec(t, "", "term", "frob"); code != 2 {
		t.Fatalf("unknown subcommand: %d", code)
	}
	if code, _, _ = exec(t, "", "term", "mapping", filepath.Join(dir, "out.tbx")); code != 2 {
		t.Fatalf("mapping of a TBX file: %d", code)
	}
}
