// Command shelfgen makes the folder of spelling dictionaries that comes
// with Interlinea, from a copy of the LibreOffice dictionaries
// (https://github.com/LibreOffice/dictionaries):
//
//	go run ./spell/shelfgen -src <copy> -commit <its commit> -out <folder>
//	go run ./spell/shelfgen -files  lists the files of the copy it reads
//
// Each dictionary becomes one ZIP file with its .aff and .dic files and its
// authors' readme and licence files, and the folder gets an index
// (index.json) and a README.txt that credits them. Every dictionary is
// loaded and tried on a few words first, so a dictionary the spell checker
// reads wrongly is not shipped. "make dictionaries" fetches the copy at the
// commit the Makefile names and runs this.
package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/4nonX/interlinea/langtag"
	"github.com/4nonX/interlinea/spell"
)

// source is one dictionary in the LibreOffice repository.
type source struct {
	// name is the dictionary's name in Interlinea, and of its files in
	// the ZIP file.
	name string
	lang string
	// files is the path of its .aff and .dic files without the extension.
	files string
	// notes are its readme and licence files.
	notes []string
	// right are words it must accept, wrong words it must not.
	right, wrong []string
}

var frami = []string{"de/README_de_DE_frami.txt", "de/README_extension_owner.txt", "de/COPYING_GPLv2", "de/COPYING_GPLv3", "de/COPYING_OASIS.txt"}
var spanish = []string{"es/README_hunspell_es.txt", "es/LICENSE.md", "es/GPLv3.txt", "es/LGPLv3.txt", "es/LGPLv2.1.txt", "es/MPL-1.1.txt"}

var sources = []source{
	{"en_US", "en-US", "en/en_US", []string{"en/README_en_US.txt"}, []string{"color", "translation", "running"}, []string{"colour", "translaton"}},
	{"en_GB", "en-GB", "en/en_GB", []string{"en/README_en_GB.txt"}, []string{"colour", "translation", "organisation"}, []string{"translaton"}},
	{"de_DE_frami", "de-DE", "de/de_DE_frami", frami, []string{"Straße", "Übersetzung", "Häuser"}, []string{"Strasse", "Übersetzng"}},
	{"de_AT_frami", "de-AT", "de/de_AT_frami", frami, []string{"Straße", "Übersetzung", "Jänner"}, []string{"Übersetzng"}},
	{"de_CH_frami", "de-CH", "de/de_CH_frami", frami, []string{"Strasse", "Übersetzung"}, []string{"Straße", "Übersetzng"}},
	{"fr_FR", "fr-FR", "fr_FR/dictionaries/fr", []string{"fr_FR/dictionaries/README_dict_fr.txt"}, []string{"traduction", "élève", "château"}, []string{"traducton"}},
	{"es_ES", "es-ES", "es/es_ES", spanish, []string{"traducción", "niño", "casas"}, []string{"traducion"}},
	{"es_MX", "es-MX", "es/es_MX", spanish, []string{"traducción", "niño", "casas"}, []string{"traducion"}},
	{"it_IT", "it-IT", "it_IT/it_IT", []string{"it_IT/README_it_IT.txt"}, []string{"traduzione", "città", "case"}, []string{"traduzone"}},
	{"pt_PT", "pt-PT", "pt_PT/pt_PT", []string{"pt_PT/README_pt_PT.txt", "pt_PT/LICENSES.txt"}, []string{"tradução", "casa", "ação"}, []string{"tradusão"}},
	{"pt_BR", "pt-BR", "pt_BR/pt_BR", []string{"pt_BR/README_pt_BR.txt", "pt_BR/README_en.txt"}, []string{"tradução", "casa", "ação"}, []string{"tradusão"}},
	{"nl_NL", "nl-NL", "nl_NL/nl_NL", []string{"nl_NL/README.md", "nl_NL/LICENSE.txt"}, []string{"vertaling", "huizen", "ziekenhuis"}, []string{"vertalnig"}},
	{"pl_PL", "pl-PL", "pl_PL/pl_PL", []string{"pl_PL/README_pl_PL.txt", "pl_PL/README_en.txt"}, []string{"tłumaczenie", "domy", "źródło"}, []string{"tlumaczenie"}},
	{"ru_RU", "ru-RU", "ru_RU/ru_RU", []string{"ru_RU/README_ru_RU.txt"}, []string{"перевод", "дома", "книга"}, []string{"пеервод"}},
	{"sv_SE", "sv-SE", "sv_SE/dictionaries/sv_SE", []string{"sv_SE/LICENSE_en_US.txt", "sv_SE/LICENSE_sv_SE.txt"}, []string{"översättning", "hus", "bilar"}, []string{"översätning"}},
	{"da_DK", "da-DK", "da_DK/da_DK", []string{"da_DK/README_da_DK.txt"}, []string{"oversættelse", "huse", "bøger"}, []string{"oversætelse"}},
	{"nb_NO", "nb-NO", "no/nb_NO", []string{"no/README_NO.txt", "no/README", "no/COPYING"}, []string{"oversettelse", "hus", "bøker"}, []string{"oversetelse"}},
	{"cs_CZ", "cs-CZ", "cs_CZ/cs_CZ", []string{"cs_CZ/README_en.txt", "cs_CZ/README_cs.txt"}, []string{"překlad", "domy", "město"}, []string{"preklad"}},
	{"ko_KR", "ko-KR", "ko_KR/ko_KR", []string{"ko_KR/README_ko_KR.txt"}, []string{"번역", "사람", "학교"}, []string{"벉역"}},
	{"uk_UA", "uk-UA", "uk_UA/uk_UA", []string{"uk_UA/README_uk_UA.txt"}, []string{"переклад", "будинки", "книга"}, []string{"пееркалд"}},
	{"hu_HU", "hu-HU", "hu_HU/hu_HU", []string{"hu_HU/README_hu_HU.txt"}, []string{"fordítás", "házak", "könyv"}, []string{"forditás"}},
	{"ro_RO", "ro-RO", "ro/ro_RO", []string{"ro/README_EN.txt", "ro/COPYING.GPL", "ro/COPYING.LGPL", "ro/COPYING.MPL"}, []string{"traducere", "casă", "țară"}, []string{"traduere"}},
}

// stamp is the time on every file in the ZIP files, so the same sources
// make the same files.
var stamp = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func main() {
	src := flag.String("src", "", "copy of github.com/LibreOffice/dictionaries")
	commit := flag.String("commit", "", "the copy's commit, recorded in the index")
	out := flag.String("out", "", "folder to write (made if missing; its ZIP files and index are replaced)")
	files := flag.Bool("files", false, "list the files of the repository it reads, as sparse-checkout patterns, and stop")
	flag.Parse()
	if *files {
		seen := map[string]bool{}
		for _, s := range sources {
			for _, f := range append([]string{s.files + ".aff", s.files + ".dic"}, s.notes...) {
				if !seen[f] {
					seen[f] = true
					fmt.Println("/" + f)
				}
			}
		}
		return
	}
	if *src == "" || *out == "" || *commit == "" {
		fmt.Fprintln(os.Stderr, "usage: shelfgen -src <copy> -commit <commit> -out <folder>")
		os.Exit(2)
	}
	if err := run(*src, *commit, *out); err != nil {
		fmt.Fprintln(os.Stderr, "shelfgen:", err)
		os.Exit(1)
	}
}

func run(src, commit, out string) error {
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	idx := spell.ShelfIndexFile{Source: "https://github.com/LibreOffice/dictionaries", Commit: commit}
	var credits strings.Builder
	credits.WriteString(`Spelling dictionaries included with Interlinea

These Hunspell dictionaries are taken unchanged from the LibreOffice
dictionaries, ` + idx.Source + `, commit ` + commit + `.
Each ZIP file holds one dictionary with its authors' readme and licence
files, which say who made it and under which licence it may be used and
passed on. They are separate works with licences of their own, not part of
Interlinea's source code.

`)
	for _, s := range sources {
		e, err := pack(src, out, s)
		if err != nil {
			return fmt.Errorf("%s: %w", s.name, err)
		}
		idx.Dictionaries = append(idx.Dictionaries, e)
		fmt.Fprintf(&credits, "%-16s %-6s %s\n", e.File, e.Lang, strings.Join(e.Notes, ", "))
		fmt.Printf("%-12s %-6s %8d words %6d KB\n", e.Name, e.Lang, e.Words, e.Size/1024)
	}
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(out, spell.ShelfIndex), append(data, '\n'), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(out, "README.txt"), []byte(credits.String()), 0o644); err != nil {
		return err
	}
	// What was written reads back as the apps read it.
	shelf, err := spell.OpenShelf(os.DirFS(out))
	if err != nil {
		return err
	}
	for _, e := range shelf.List() {
		if _, err := shelf.Load(e.Name); err != nil {
			return err
		}
	}
	return nil
}

func pack(src, out string, s source) (spell.ShelfEntry, error) {
	e := spell.ShelfEntry{Name: s.name, File: s.name + ".zip"}
	lang, err := langtag.Normalize(s.lang)
	if err != nil || lang != s.lang {
		return e, fmt.Errorf("%s is not a language tag the apps know", s.lang)
	}
	e.Lang = lang
	aff, err := os.ReadFile(filepath.Join(src, filepath.FromSlash(s.files+".aff")))
	if err != nil {
		return e, err
	}
	dic, err := os.ReadFile(filepath.Join(src, filepath.FromSlash(s.files+".dic")))
	if err != nil {
		return e, err
	}
	d, err := spell.Load(aff, dic)
	if err != nil {
		return e, err
	}
	var bad []string
	for _, w := range s.right {
		if !d.Spell(w) {
			bad = append(bad, w+" (marked wrong)")
		}
	}
	for _, w := range s.wrong {
		if d.Spell(w) {
			bad = append(bad, w+" (accepted)")
		}
	}
	if len(bad) > 0 {
		return e, errors.New("spells wrongly: " + strings.Join(bad, ", "))
	}
	e.Words = d.Words()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name string, data []byte) error {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: stamp})
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	}
	if err := add(s.name+".aff", aff); err != nil {
		return e, err
	}
	if err := add(s.name+".dic", dic); err != nil {
		return e, err
	}
	for _, n := range s.notes {
		data, err := os.ReadFile(filepath.Join(src, filepath.FromSlash(n)))
		if err != nil {
			return e, err
		}
		if err := add(path.Base(n), data); err != nil {
			return e, err
		}
		e.Notes = append(e.Notes, path.Base(n))
	}
	if err := zw.Close(); err != nil {
		return e, err
	}
	e.Size = int64(buf.Len())
	return e, os.WriteFile(filepath.Join(out, e.File), buf.Bytes(), 0o644)
}
