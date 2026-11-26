package spell

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func shelfZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(body))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func shelfIndex(t *testing.T, es ...ShelfEntry) []byte {
	t.Helper()
	data, err := json.Marshal(ShelfIndexFile{Source: "test", Commit: "1", Dictionaries: es})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestShelf(t *testing.T) {
	fsys := fstest.MapFS{
		"xx_XX.zip": {Data: shelfZip(t, map[string]string{
			"xx_XX.aff":  "SET UTF-8\nSFX S Y 1\nSFX S 0 s .\n",
			"xx_XX.dic":  "2\nhaus/S\nbaum\n",
			"README.txt": "made for the test",
		})},
	}
	fsys[ShelfIndex] = &fstest.MapFile{Data: shelfIndex(t, ShelfEntry{Name: "xx_XX", Lang: "xx", Words: 2, File: "xx_XX.zip"})}
	s, err := OpenShelf(fsys)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.List(); len(got) != 1 || got[0].Name != "xx_XX" {
		t.Fatalf("List = %+v", got)
	}
	if _, ok := s.Find("XX_xx"); !ok {
		t.Error("Find ignores case")
	}
	d, err := s.Load("xx_xx")
	if err != nil {
		t.Fatal(err)
	}
	for w, want := range map[string]bool{"haus": true, "hauss": true, "hauses": false, "baum": true, "baums": false} {
		if d.Spell(w) != want {
			t.Errorf("Spell(%q) = %v", w, !want)
		}
	}
	if _, err := s.Load("yy"); !errors.Is(err, ErrNotOnShelf) {
		t.Errorf("Load of a name not there: %v", err)
	}

	var none *Shelf
	if none.List() != nil || none.Index().Commit != "" {
		t.Error("a nil shelf has dictionaries")
	}
	if _, ok := none.Find("xx_XX"); ok {
		t.Error("a nil shelf finds a dictionary")
	}
}

func TestShelfRefusesBadIndex(t *testing.T) {
	zipped := &fstest.MapFile{Data: shelfZip(t, map[string]string{"a.aff": "", "a.dic": "0\n"})}
	for name, es := range map[string][]ShelfEntry{
		"no name":      {{Name: "", Lang: "xx", File: "a.zip"}},
		"slash":        {{Name: "../a", Lang: "xx", File: "a.zip"}},
		"twice":        {{Name: "a", Lang: "xx", File: "a.zip"}, {Name: "A", Lang: "xx", File: "a.zip"}},
		"no language":  {{Name: "a", File: "a.zip"}},
		"outside":      {{Name: "a", Lang: "xx", File: "../a.zip"}},
		"in a folder":  {{Name: "a", Lang: "xx", File: "sub/a.zip"}},
		"file missing": {{Name: "a", Lang: "xx", File: "b.zip"}},
	} {
		fsys := fstest.MapFS{"a.zip": zipped, ShelfIndex: {Data: shelfIndex(t, es...)}}
		if _, err := OpenShelf(fsys); err == nil {
			t.Errorf("%s: the index is accepted", name)
		}
	}
	if _, err := OpenShelf(fstest.MapFS{ShelfIndex: {Data: []byte("{")}}); err == nil {
		t.Error("a broken index is accepted")
	}
	if _, err := OpenShelf(fstest.MapFS{}); err == nil {
		t.Error("a folder without an index is accepted")
	}
	// The archive has another dictionary than the index says.
	fsys := fstest.MapFS{"a.zip": zipped, ShelfIndex: {Data: shelfIndex(t, ShelfEntry{Name: "b", Lang: "xx", File: "a.zip"})}}
	s, err := OpenShelf(fsys)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load("b"); err == nil || !strings.Contains(err.Error(), "no dictionary b") {
		t.Errorf("Load = %v", err)
	}
}

func TestFindShelf(t *testing.T) {
	dir := t.TempDir()
	if got := FindShelf(dir); got != "" {
		t.Errorf("a folder without an index is found: %s", got)
	}
	if err := os.WriteFile(filepath.Join(dir, ShelfIndex), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := FindShelf("", filepath.Join(dir, "nothing"), dir); got != filepath.Clean(dir) {
		t.Errorf("FindShelf = %q, want %q", got, dir)
	}
}

// The included dictionaries, when they have been made (make dictionaries),
// all load and spell.
func TestIncludedDictionaries(t *testing.T) {
	dir := filepath.Join("..", "web", "public", "dictionaries")
	if _, err := os.Stat(filepath.Join(dir, ShelfIndex)); err != nil {
		t.Skip("no included dictionaries here (make dictionaries)")
	}
	s, err := OpenShelf(os.DirFS(dir))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.List()) < 20 {
		t.Errorf("only %d dictionaries", len(s.List()))
	}
	for _, e := range s.List() {
		d, err := s.Load(e.Name)
		if err != nil {
			t.Errorf("%s: %v", e.Name, err)
			continue
		}
		if d.Words() != e.Words {
			t.Errorf("%s has %d words, the index says %d", e.Name, d.Words(), e.Words)
		}
	}
}

// The desktop app's tests run their core with a small folder of their own.
func TestDesktopTestDictionaries(t *testing.T) {
	s, err := OpenShelf(os.DirFS(filepath.Join("..", "desktop", "tests", "dictionaries")))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range s.List() {
		d, err := s.Load(e.Name)
		if err != nil {
			t.Fatal(err)
		}
		if d.Words() != e.Words {
			t.Errorf("%s has %d words, the index says %d", e.Name, d.Words(), e.Words)
		}
	}
}
