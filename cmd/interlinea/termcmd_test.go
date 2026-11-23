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

func TestTermbaseCommands(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "glossar.csv")
	if err := os.WriteFile(src, []byte(termCSV), 0o644); err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(dir, "tb.db")
	out := filepath.Join(dir, "out.tbx")
	steps := []struct {
		args []string
		code int
		want string
	}{
		{[]string{"termbase"}, 2, ""},
		{[]string{"termbase", "nope"}, 2, ""},
		{[]string{"termbase", "list", "-db", db}, 0, "No termbases"},
		{[]string{"termbase", "import", "-db", db, "-name", "Weber", src}, 1, ""},
		{[]string{"termbase", "import", "-db", db, "-name", "Weber", "-create", "-dry-run", src}, 0, "Would add 3 entries (6 terms)"},
		{[]string{"termbase", "list", "-db", db}, 0, "No termbases"},
		{[]string{"termbase", "create", "-db", db, "-name", "Weber", "-langs", "de-DE,en-GB", "-by", "x"}, 2, ""},
		{[]string{"termbase", "create", "-db", db, "-name", "Weber", "-langs", "de-DE,en-GB"}, 0, `Created "Weber" (de-DE, en-GB)`},
		{[]string{"termbase", "create", "-db", db, "-name", "weber"}, 1, ""},
		{[]string{"termbase", "import", "-db", db, "-name", "Weber", "-match", "nope", src}, 2, ""},
		{[]string{"termbase", "import", "-db", db, "-name", "Weber", "-by", "Dan", src}, 0, "2 added, 1 merged"},
		{[]string{"termbase", "import", "-db", db, "-name", "Weber", "-dry-run", src}, 0, "Would import glossar.csv into \"Weber\": 0 added, 0 merged, 0 replaced, 3 left as they were"},
		{[]string{"termbase", "import", "-db", db, "-name", "Weber", "-match", "id", "-on", "add", src}, 0, "3 added"},
		{[]string{"termbase", "list", "-db", db}, 0, "5 entries"},
		{[]string{"termbase", "lookup", "-db", db, "-name", "Weber", "-lang", "de", "mieter"}, 0, "de-DE:Mieter"},
		{[]string{"termbase", "lookup", "-db", db, "-name", "Weber", "Mond"}, 0, "No entry has"},
		{[]string{"termbase", "search", "-db", db, "-name", "Weber", "-status", "forbidden"}, 0, "2 of 2 matching entries"},
		{[]string{"termbase", "search", "-db", db, "-name", "Weber", "-status", "maybe"}, 2, ""},
		{[]string{"termbase", "search", "-db", db, "-name", "Weber", "Gebrauch"}, 0, "2 of 2"},
		{[]string{"termbase", "show", "-db", db, "-name", "Weber", "1"}, 0, "lease agreement (approved)"},
		{[]string{"termbase", "show", "-db", db, "-name", "Weber", "404"}, 1, ""},
		{[]string{"termbase", "duplicates", "-db", db, "-name", "Weber", "-lang", "de"}, 0, "2 groups"},
		{[]string{"termbase", "merge", "-db", db, "-name", "Weber", "-by", "Dan", "1", "c1"}, 0, "Merged into 1"},
		{[]string{"termbase", "history", "-db", db, "-name", "Weber", "1"}, 0, "Merged with entries c1"},
		{[]string{"termbase", "delete-entry", "-db", db, "-name", "Weber", "c2"}, 2, ""},
		{[]string{"termbase", "delete-entry", "-db", db, "-name", "Weber", "-yes", "c2"}, 0, "Deleted entry c2"},
		{[]string{"termbase", "recognize", "-db", db, "-name", "Weber", "-src", "de-DE", "-tgt", "en-GB", "-target", "The lease.", "Der Mietvertrag."}, 0, "Mietvertrag\tMietvertrag\tlease agreement"},
		{[]string{"termbase", "remove-field", "-db", db, "-name", "Weber", "-field", "Definition"}, 2, ""},
		{[]string{"termbase", "remove-field", "-db", db, "-name", "Weber", "-field", "Definition", "-yes"}, 0, "1 entries changed"},
		{[]string{"termbase", "export", "-db", db, "-name", "Weber", out}, 0, "3 entries"},
		{[]string{"termbase", "export", "-db", db, "-name", "Weber", filepath.Join(dir, "x.xdt")}, 2, ""},
		{[]string{"termbase", "rename", "-db", db, "-name", "Weber", "-to", "Weber Miete"}, 0, `Renamed "Weber" to "Weber Miete"`},
		{[]string{"termbase", "remove-language", "-db", db, "-name", "Weber Miete", "-lang", "en-GB", "-yes"}, 0, "Removed en-GB"},
		{[]string{"termbase", "delete", "-db", db, "-name", "Weber Miete"}, 2, ""},
		{[]string{"termbase", "delete", "-db", db, "-name", "Weber Miete", "-yes"}, 0, `Deleted "Weber Miete"`},
		{[]string{"termbase", "list", "-db", db, "-json"}, 0, "[]"},
	}
	for _, s := range steps {
		code, stdout, errOut := exec(t, "", s.args...)
		if code != s.code || !strings.Contains(stdout, s.want) {
			t.Fatalf("%v: exit %d (want %d)\nstdout: %s\nstderr: %s", s.args, code, s.code, stdout, errOut)
		}
	}
	if code, stdout, _ := exec(t, "", "term", "info", out); code != 0 || !strings.Contains(stdout, "Entries:   3") {
		t.Fatalf("exported TBX: %s", stdout)
	}
}
