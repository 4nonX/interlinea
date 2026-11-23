package spell

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
)

// Bundled is a dictionary found in an archive: a LibreOffice extension
// (.oxt), a Firefox add-on (.xpi) or a plain zip of .aff and .dic files.
type Bundled struct {
	// Name is the base name of the files ("de_DE_frami").
	Name string `json:"name"`
	// Locales are the languages the archive declares for it (from
	// dictionaries.xcu), or the one its name suggests ("de-DE").
	Locales []string `json:"locales"`
	Aff     []byte   `json:"-"`
	Dic     []byte   `json:"-"`
}

// ErrNoDictionary is returned for an archive without .aff and .dic.
var ErrNoDictionary = errors.New("spell: the archive has no Hunspell dictionary (.aff and .dic)")

const maxArchiveFile = 200 << 20

// ReadArchive lists the Hunspell dictionaries in a zip archive.
func ReadArchive(data []byte) ([]Bundled, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("spell: not a zip archive: %w", err)
	}
	files := map[string]*zip.File{}
	for _, f := range zr.File {
		files[strings.ToLower(f.Name)] = f
	}
	read := func(name string) ([]byte, error) {
		f, ok := files[strings.ToLower(name)]
		if !ok {
			return nil, fmt.Errorf("spell: %s is not in the archive", name)
		}
		if f.UncompressedSize64 > maxArchiveFile {
			return nil, fmt.Errorf("spell: %s is too large", name)
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		return io.ReadAll(io.LimitReader(rc, maxArchiveFile+1))
	}
	locales := map[string][]string{} // base path (lower case) to locales
	for name, f := range files {
		if path.Base(name) != "dictionaries.xcu" {
			continue
		}
		x, err := read(f.Name)
		if err != nil {
			return nil, err
		}
		for base, ls := range parseXCU(x, path.Dir(f.Name)) {
			locales[strings.ToLower(base)] = ls
		}
	}
	var out []Bundled
	for name, f := range files {
		if !strings.HasSuffix(name, ".aff") {
			continue
		}
		base := strings.TrimSuffix(f.Name, f.Name[len(f.Name)-4:])
		dic, ok := files[strings.ToLower(base)+".dic"]
		if !ok {
			continue
		}
		aff, err := read(f.Name)
		if err != nil {
			return nil, err
		}
		dd, err := read(dic.Name)
		if err != nil {
			return nil, err
		}
		b := Bundled{Name: path.Base(base), Aff: aff, Dic: dd, Locales: locales[strings.ToLower(base)]}
		if len(b.Locales) == 0 {
			if l := localeFromName(b.Name); l != "" {
				b.Locales = []string{l}
			}
		}
		out = append(out, b)
	}
	if len(out) == 0 {
		return nil, ErrNoDictionary
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// localeFromName reads "de_DE_frami" as de-DE and "fr" as fr.
func localeFromName(name string) string {
	parts := strings.Split(strings.ReplaceAll(name, "-", "_"), "_")
	if len(parts[0]) < 2 || len(parts[0]) > 3 {
		return ""
	}
	for _, r := range parts[0] {
		if r < 'a' || r > 'z' {
			return ""
		}
	}
	if len(parts) > 1 && len(parts[1]) == 2 && strings.ToUpper(parts[1]) == parts[1] {
		return parts[0] + "-" + parts[1]
	}
	return parts[0]
}

// parseXCU reads the spelling dictionaries a LibreOffice extension
// declares: per dictionary, its locations (%origin%/de_DE_frami.aff) and
// locales.
func parseXCU(data []byte, dir string) map[string][]string {
	type prop struct {
		Name  string `xml:"name,attr"`
		Value string `xml:"value"`
	}
	type node struct {
		Name  string `xml:"name,attr"`
		Props []prop `xml:"prop"`
		Nodes []node `xml:"node"`
	}
	var root node
	if err := xml.Unmarshal(data, &root); err != nil {
		return nil
	}
	out := map[string][]string{}
	var walk func(n node)
	walk = func(n node) {
		var format, locations, locales string
		for _, p := range n.Props {
			switch p.Name {
			case "Format":
				format = strings.TrimSpace(p.Value)
			case "Locations":
				locations = p.Value
			case "Locales":
				locales = p.Value
			}
		}
		if format == "DICT_SPELL" {
			for _, loc := range strings.Fields(locations) {
				loc = strings.TrimPrefix(loc, "%origin%/")
				if strings.HasSuffix(strings.ToLower(loc), ".aff") {
					base := path.Join(dir, loc[:len(loc)-4])
					out[base] = strings.Fields(locales)
				}
			}
		}
		for _, c := range n.Nodes {
			walk(c)
		}
	}
	walk(root)
	return out
}
