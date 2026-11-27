// Command gen keeps the translations of the apps' interface in step with
// their text: it collects the English strings the browser client and the
// desktop app show, and lists them in the catalogs of i18n, one per
// interface language.
//
//	go run ./i18n/gen          adds the new strings to the catalogs (with no
//	                           translation yet) and takes out those gone
//	go run ./i18n/gen -check   fails if a string has no translation, a
//	                           translation is left over, or a translation
//	                           lacks a placeholder of its string
//	go run ./i18n/gen -todo de lists the strings German has no translation for
//
// The browser client marks its strings with tl("…"), tlx("…") and
// plural(n, "…", "…"); the desktop app with tr("…"), QT_TR_NOOP("…"),
// QT_TRANSLATE_NOOP("Context", "…") and QCoreApplication::translate. Both
// apps look a string up by its English text, so one catalog serves both.
// The names of the languages the apps list (package langtag) are strings
// of both as well.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/4nonX/interlinea/langtag"
)

// Languages are the interface languages besides English.
var Languages = []string{"de", "ko"}

func main() {
	check := flag.Bool("check", false, "fail if the catalogs are out of date")
	todo := flag.String("todo", "", "list the strings a language has no translation for")
	flag.Parse()
	root, err := repoRoot()
	if err != nil {
		fail(err)
	}
	keys, err := collect(root)
	if err != nil {
		fail(err)
	}
	if *todo != "" {
		cat, err := load(root, *todo)
		if err != nil {
			fail(err)
		}
		var out []string
		for _, k := range sortedKeys(keys) {
			if cat[k] == "" {
				out = append(out, k)
			}
		}
		b, _ := json.MarshalIndent(out, "", "  ")
		os.Stdout.Write(append(b, '\n'))
		return
	}
	bad := 0
	if *check {
		for _, k := range sortedKeys(keys) {
			if why := pieceOfSentence(k); why != "" && !desktopOnly[k] {
				fmt.Fprintf(os.Stderr, "%q (%s) %s: write the whole sentence, with named placeholders\n", k, keys[k], why)
				bad++
			}
		}
	}
	for _, lang := range Languages {
		cat, err := load(root, lang)
		if err != nil {
			fail(err)
		}
		next := map[string]string{}
		for k := range keys {
			next[k] = cat[k]
		}
		if *check {
			for _, k := range sortedKeys(keys) {
				v := cat[k]
				switch {
				case v == "":
					fmt.Fprintf(os.Stderr, "%s: no translation for %q (%s)\n", lang, k, keys[k])
					bad++
				case !samePlaceholders(k, v):
					fmt.Fprintf(os.Stderr, "%s: the translation of %q has other placeholders: %q\n", lang, k, v)
					bad++
				}
			}
			for k := range cat {
				if _, ok := keys[k]; !ok {
					fmt.Fprintf(os.Stderr, "%s: %q is no longer in the apps\n", lang, k)
					bad++
				}
			}
			continue
		}
		if err := os.WriteFile(catalogPath(root, lang), encode(next), 0o644); err != nil {
			fail(err)
		}
	}
	if bad > 0 {
		fmt.Fprintf(os.Stderr, "%d problems; run go run ./i18n/gen and translate what it adds\n", bad)
		os.Exit(1)
	}
}

// desktopOnly holds the strings only the desktop app has: its pages are
// being made again as a desktop application, with whole sentences, and
// are held to pieceOfSentence once they are.
var desktopOnly = map[string]bool{}

var (
	opaquePlaceholder = regexp.MustCompile(`\{x\d*\}`)
	leadingPiece      = regexp.MustCompile(`^(?:[,;:]|\. | [a-z{]| and )`)
	trailingPiece     = regexp.MustCompile(`[ ,;(]$`)
)

// pieceOfSentence says why a string reads as part of a sentence that is
// glued to others, which no other language can translate: one that starts
// with a comma or a lower-case word after a space, ends open, or has a
// placeholder that says nothing of what goes in it ({x}). Whole sentences
// after a space (" The rules below.") and remarks in brackets are whole.
func pieceOfSentence(k string) string {
	switch {
	case opaquePlaceholder.MatchString(k):
		return "has a placeholder that does not say what goes in it"
	case leadingPiece.MatchString(k):
		return "starts in the middle of a sentence"
	case trailingPiece.MatchString(k):
		return "ends in the middle of a sentence"
	}
	return ""
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		up := filepath.Dir(dir)
		if up == dir {
			return "", fmt.Errorf("not in the repository")
		}
		dir = up
	}
}

func catalogPath(root, lang string) string {
	return filepath.Join(root, "i18n", lang+".json")
}

func load(root, lang string) (map[string]string, error) {
	data, err := os.ReadFile(catalogPath(root, lang))
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	cat := map[string]string{}
	if err := json.Unmarshal(data, &cat); err != nil {
		return nil, fmt.Errorf("%s: %v", catalogPath(root, lang), err)
	}
	return cat, nil
}

// encode writes a catalog sorted by its English strings, one to a line,
// without escaping < > & so it reads as it is shown.
func encode(cat map[string]string) []byte {
	var b bytes.Buffer
	b.WriteString("{\n")
	ks := make([]string, 0, len(cat))
	for k := range cat {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	for i, k := range ks {
		b.WriteString("  ")
		b.Write(quote(k))
		b.WriteString(": ")
		b.Write(quote(cat[k]))
		if i < len(ks)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString("}\n")
	return b.Bytes()
}

func quote(s string) []byte {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.Encode(s)
	return bytes.TrimRight(b.Bytes(), "\n")
}

func sortedKeys(m map[string]string) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

var placeholder = regexp.MustCompile(`\{[A-Za-z0-9_]+\}|%[0-9n]`)

// samePlaceholders reports whether a translation has the placeholders of
// its string, each as often, in any order.
func samePlaceholders(key, tr string) bool {
	count := func(s string) map[string]int {
		m := map[string]int{}
		for _, p := range placeholder.FindAllString(s, -1) {
			m[p]++
		}
		return m
	}
	a, b := count(key), count(tr)
	if len(a) != len(b) {
		return false
	}
	for k, n := range a {
		if b[k] != n {
			return false
		}
	}
	return true
}

// collect returns the strings of both apps, each with where it is first
// found.
func collect(root string) (map[string]string, error) {
	keys := map[string]string{}
	add := func(k, where string) {
		if strings.TrimSpace(k) == "" {
			return
		}
		if _, ok := keys[k]; !ok {
			keys[k] = where
		}
	}
	walk := func(dir string, exts []string, scan func(src string, add func(string, int))) error {
		return filepath.WalkDir(filepath.Join(root, dir), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "node_modules" || d.Name() == "e2e" || d.Name() == "tests" {
					return filepath.SkipDir
				}
				return nil
			}
			ok := false
			for _, e := range exts {
				ok = ok || strings.HasSuffix(p, e)
			}
			if !ok || strings.Contains(p, ".gen.") || strings.HasSuffix(p, ".test.ts") {
				return nil
			}
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(root, p)
			src := string(data)
			scan(src, func(k string, at int) {
				add(k, fmt.Sprintf("%s:%d", filepath.ToSlash(rel), strings.Count(src[:at], "\n")+1))
			})
			return nil
		})
	}
	if err := walk("web/src", []string{".ts", ".tsx"}, scanTS); err != nil {
		return nil, err
	}
	others := map[string]bool{}
	for k := range keys {
		others[k] = true
	}
	if err := walk("desktop/src", []string{".cpp", ".h"}, scanCPP); err != nil {
		return nil, err
	}
	for k := range keys {
		if !others[k] {
			desktopOnly[k] = true
		}
	}
	// The core's texts the apps show, marked with i18n.N.
	if err := walkGo(root, func(k, where string) { add(k, where) }); err != nil {
		return nil, err
	}
	// The names of the languages both apps list.
	for _, l := range langtag.Languages() {
		add(l.Name, "langtag/languages.go")
	}
	return keys, nil
}

// walkGo finds i18n.N("…") in the Go sources of the core, not in tests
// or in this command.
func walkGo(root string, add func(k, where string)) error {
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "node_modules", "web", "desktop", "testdata", ".git", "gen":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		src := string(data)
		rel, _ := filepath.Rel(root, p)
		for _, m := range goCall.FindAllStringIndex(src, -1) {
			i := skipSpace(src, m[1])
			lit := goLiteral.FindString(src[i:])
			if lit == "" {
				continue
			}
			if s, err := strconv.Unquote(lit); err == nil {
				add(s, fmt.Sprintf("%s:%d", filepath.ToSlash(rel), strings.Count(src[:m[0]], "\n")+1))
			}
		}
		return nil
	})
}

var (
	goCall    = regexp.MustCompile(`(?:^|[^A-Za-z0-9_])i18n\.N\(`)
	goLiteral = regexp.MustCompile("^(?:\"(?:[^\"\\\\\n]|\\\\.)*\"|`[^`]*`)")
	tsCall    = regexp.MustCompile(`(?:^|[^A-Za-z0-9_$.])(tl|tlx|plural)\(`)
	cppCall   = regexp.MustCompile(`(?:^|[^A-Za-z0-9_])(tr|QT_TR_NOOP|QT_TRANSLATE_NOOP|QCoreApplication::translate)\(`)
)

// scanTS finds tl("…"), tlx("…") and the two forms of plural(n, "…", "…"),
// keyed "{n} …" as plural shows them.
func scanTS(src string, add func(string, int)) {
	for _, m := range tsCall.FindAllStringSubmatchIndex(src, -1) {
		name := src[m[2]:m[3]]
		i := m[1]
		if name == "plural" {
			// Past the count, to the two forms.
			j := skipArg(src, i)
			for f := 0; f < 2 && j < len(src) && src[j] == ','; f++ {
				s, end, ok := jsString(src, skipSpace(src, j+1))
				if !ok {
					break
				}
				add("{n} "+s, m[0])
				j = skipSpace(src, end)
			}
			continue
		}
		if s, _, ok := jsString(src, skipSpace(src, i)); ok {
			add(s, m[0])
		}
	}
}

func skipSpace(src string, i int) int {
	for i < len(src) && strings.ContainsRune(" \t\r\n", rune(src[i])) {
		i++
	}
	return i
}

// skipArg goes past one argument of a call, to the comma or bracket after
// it.
func skipArg(src string, i int) int {
	depth := 0
	for i < len(src) {
		switch c := src[i]; c {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			if depth == 0 {
				return i
			}
			depth--
		case ',':
			if depth == 0 {
				return i
			}
		case '"', '\'', '`':
			if _, end, ok := jsString(src, i); ok {
				i = end
				continue
			}
		}
		i++
	}
	return i
}

// jsString reads a JavaScript string literal at i: quoted, or a template
// without substitutions.
func jsString(src string, i int) (string, int, bool) {
	if i >= len(src) || !strings.ContainsRune(`"'`+"`", rune(src[i])) {
		return "", i, false
	}
	q := src[i]
	var b strings.Builder
	for j := i + 1; j < len(src); j++ {
		c := src[j]
		switch {
		case c == q:
			return b.String(), j + 1, true
		case c == '$' && q == '`' && j+1 < len(src) && src[j+1] == '{':
			return "", i, false
		case c == '\n' && q != '`':
			return "", i, false
		case c == '\\' && j+1 < len(src):
			j++
			switch e := src[j]; e {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'u':
				if j+4 < len(src) && src[j+1] != '{' {
					if v, err := strconv.ParseUint(src[j+1:j+5], 16, 32); err == nil {
						b.WriteRune(rune(v))
						j += 4
						continue
					}
				}
				return "", i, false
			case '\n':
			default:
				b.WriteByte(e)
			}
		default:
			b.WriteByte(c)
		}
	}
	return "", i, false
}

// scanCPP finds the strings of tr, QT_TR_NOOP, QT_TRANSLATE_NOOP and
// QCoreApplication::translate: string literals side by side are one.
func scanCPP(src string, add func(string, int)) {
	for _, m := range cppCall.FindAllStringSubmatchIndex(src, -1) {
		name := src[m[2]:m[3]]
		i := skipSpace(src, m[1])
		if name == "QT_TRANSLATE_NOOP" || name == "QCoreApplication::translate" {
			// Past the context.
			if _, end, ok := cppString(src, i); ok {
				i = skipSpace(src, end)
				if i >= len(src) || src[i] != ',' {
					continue
				}
				i = skipSpace(src, i+1)
			} else {
				continue
			}
		}
		if s, _, ok := cppString(src, i); ok {
			add(s, m[0])
		}
	}
}

// cppString reads one or more C++ string literals side by side.
func cppString(src string, i int) (string, int, bool) {
	var b strings.Builder
	found := false
	for {
		i = skipSpace(src, i)
		j := i
		if strings.HasPrefix(src[j:], "u8\"") {
			j += 2
		}
		if j >= len(src) || src[j] != '"' {
			break
		}
		closed := false
		for j++; j < len(src); j++ {
			c := src[j]
			if c == '"' {
				closed = true
				j++
				break
			}
			if c == '\n' {
				return "", i, false
			}
			if c == '\\' && j+1 < len(src) {
				j++
				switch e := src[j]; e {
				case 'n':
					b.WriteByte('\n')
				case 't':
					b.WriteByte('\t')
				case 'x':
					k := j + 1
					for k < len(src) && k < j+3 && strings.ContainsRune("0123456789abcdefABCDEF", rune(src[k])) {
						k++
					}
					v, _ := strconv.ParseUint(src[j+1:k], 16, 8)
					b.WriteByte(byte(v))
					j = k - 1
				default:
					b.WriteByte(e)
				}
				continue
			}
			b.WriteByte(c)
		}
		if !closed {
			return "", i, false
		}
		found = true
		i = j
	}
	if !found || !utf8.ValidString(b.String()) {
		return "", i, false
	}
	return b.String(), i, true
}
