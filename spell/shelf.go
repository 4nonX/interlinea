package spell

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// The dictionaries that come with Interlinea are a folder of ZIP files,
// one per dictionary with its .aff and .dic files and its authors' readme
// and licence, and an index of them. spell/shelfgen makes the folder from
// the LibreOffice dictionaries. The server and the desktop app's core read
// it from disk; the browser client fetches the same files.

// ShelfIndex is the name of the index in the folder.
const ShelfIndex = "index.json"

// ShelfEntry is one dictionary in the folder.
type ShelfEntry struct {
	// Name identifies it ("de_DE_frami"), as the base name of its files.
	Name string `json:"name"`
	// Lang is the language it spells (a BCP 47 tag).
	Lang string `json:"lang"`
	// Words counts its words.
	Words int `json:"words"`
	// File is its ZIP file in the folder.
	File string `json:"file"`
	// Size is the ZIP file's size in bytes.
	Size int64 `json:"size"`
	// Notes are the readme and licence files in the ZIP file.
	Notes []string `json:"notes"`
}

// ShelfIndexFile is the index: where the dictionaries come from and what
// they are.
type ShelfIndexFile struct {
	// Source is the repository the dictionaries were taken from.
	Source string `json:"source"`
	// Commit is the version of the repository they were taken from.
	Commit       string       `json:"commit"`
	Dictionaries []ShelfEntry `json:"dictionaries"`
}

// Shelf is the folder of included dictionaries. A nil Shelf has none.
type Shelf struct {
	fsys fs.FS
	idx  ShelfIndexFile
}

// OpenShelf reads the index of a folder of included dictionaries. The
// dictionaries themselves are read when they are loaded.
func OpenShelf(fsys fs.FS) (*Shelf, error) {
	data, err := fs.ReadFile(fsys, ShelfIndex)
	if err != nil {
		return nil, err
	}
	var idx ShelfIndexFile
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("spell: %s can't be read: %w", ShelfIndex, err)
	}
	seen := map[string]bool{}
	for _, e := range idx.Dictionaries {
		key := strings.ToLower(e.Name)
		switch {
		case e.Name == "" || strings.ContainsAny(e.Name, `/\`):
			return nil, fmt.Errorf("spell: %s lists a dictionary without a proper name (%q)", ShelfIndex, e.Name)
		case seen[key]:
			return nil, fmt.Errorf("spell: %s lists %s twice", ShelfIndex, e.Name)
		case e.Lang == "":
			return nil, fmt.Errorf("spell: %s doesn't say which language %s spells", ShelfIndex, e.Name)
		case !fs.ValidPath(e.File) || strings.Contains(e.File, "/"):
			return nil, fmt.Errorf("spell: %s gives %s the file %q, which is not a file in the folder", ShelfIndex, e.Name, e.File)
		}
		seen[key] = true
		if _, err := fs.Stat(fsys, e.File); err != nil {
			return nil, fmt.Errorf("spell: the file of %s is missing: %w", e.Name, err)
		}
	}
	return &Shelf{fsys: fsys, idx: idx}, nil
}

// Index is the folder's index.
func (s *Shelf) Index() ShelfIndexFile {
	if s == nil {
		return ShelfIndexFile{}
	}
	return s.idx
}

// List lists the included dictionaries.
func (s *Shelf) List() []ShelfEntry {
	if s == nil {
		return nil
	}
	return s.idx.Dictionaries
}

// Find finds an included dictionary by name, in any case.
func (s *Shelf) Find(name string) (ShelfEntry, bool) {
	for _, e := range s.List() {
		if strings.EqualFold(e.Name, name) {
			return e, true
		}
	}
	return ShelfEntry{}, false
}

// ErrNotOnShelf is returned for a name no included dictionary has.
var ErrNotOnShelf = errors.New("spell: no included dictionary has that name")

// Load reads an included dictionary.
func (s *Shelf) Load(name string) (*Dictionary, error) {
	e, ok := s.Find(name)
	if !ok {
		return nil, ErrNotOnShelf
	}
	data, err := fs.ReadFile(s.fsys, e.File)
	if err != nil {
		return nil, err
	}
	bundles, err := ReadArchive(data)
	if err != nil {
		return nil, fmt.Errorf("spell: %s: %w", e.File, err)
	}
	for _, b := range bundles {
		if strings.EqualFold(b.Name, e.Name) {
			return Load(b.Aff, b.Dic)
		}
	}
	return nil, fmt.Errorf("spell: %s has no dictionary %s", e.File, e.Name)
}

// FindShelf looks for the folder of included dictionaries where the
// packages put it: next to the program ("dictionaries"), in
// ../share/interlinea/dictionaries (Linux), in ../Resources/dictionaries
// (inside a macOS app), and in the other folders given, in that order. It
// returns "" if none of them has the index.
func FindShelf(more ...string) string {
	var dirs []string
	if exe, err := os.Executable(); err == nil {
		if real, err := filepath.EvalSymlinks(exe); err == nil {
			exe = real
		}
		dir := filepath.Dir(exe)
		dirs = append(dirs,
			filepath.Join(dir, "dictionaries"),
			filepath.Join(dir, "..", "share", "interlinea", "dictionaries"),
			filepath.Join(dir, "..", "Resources", "dictionaries"))
	}
	dirs = append(dirs, more...)
	for _, d := range dirs {
		if d == "" {
			continue
		}
		if st, err := os.Stat(filepath.Join(d, ShelfIndex)); err == nil && st.Mode().IsRegular() {
			return filepath.Clean(d)
		}
	}
	return ""
}
