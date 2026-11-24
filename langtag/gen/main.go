// Command gen writes the list of languages the apps offer, from the one
// in package langtag, into the browser client and the desktop app:
//
//	go run ./langtag/gen          rewrites the two files
//	go run ./langtag/gen -check   fails if they are out of date
//
// Both apps then name and order the languages alike, and the checks of
// package langmatrix cover every language either of them lists.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"strconv"

	"github.com/4nonX/interlinea/langtag"
)

const note = "Written by go run ./langtag/gen from langtag/languages.go. Do not edit; change the list there."

func typescript() []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "// %s\n\n", note)
	b.WriteString("export interface ListedLanguage {\n  code: string;\n  name: string;\n  /** The language's own name. */\n  native: string;\n  dir?: \"rtl\";\n}\n\n")
	b.WriteString("export const LISTED_LANGUAGES: ListedLanguage[] = [\n")
	for _, l := range langtag.Languages() {
		fmt.Fprintf(&b, "  { code: %s, name: %s, native: %s", strconv.Quote(l.Tag), strconv.Quote(l.Name), strconv.Quote(l.Native))
		if l.RTL {
			b.WriteString(`, dir: "rtl"`)
		}
		b.WriteString(" },\n")
	}
	b.WriteString("];\n")
	return b.Bytes()
}

func cpp() []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "// %s\n#pragma once\n\n", note)
	b.WriteString("struct ListedLanguage {\n    const char *tag;\n    const char *name;\n    const char *native;\n    bool rtl;\n};\n\n")
	b.WriteString("inline constexpr ListedLanguage listedLanguages[] = {\n")
	for _, l := range langtag.Languages() {
		fmt.Fprintf(&b, "    {%s, %s, %s, %t},\n", cString(l.Tag), cString(l.Name), cString(l.Native), l.RTL)
	}
	b.WriteString("};\n")
	return b.Bytes()
}

// cString writes s as a UTF-8 string literal that every compiler reads the
// same whatever its source encoding: other than ASCII as escapes.
func cString(s string) string {
	var b bytes.Buffer
	b.WriteByte('"')
	escaped := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"' || c == '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
			escaped = false
		case c < 0x20 || c >= 0x7f:
			fmt.Fprintf(&b, "\\x%02x", c)
			escaped = true
		default:
			// A hex escape takes every hex digit after it; break the
			// literal so a following letter is not read into it.
			if escaped && (c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
				b.WriteString(`" "`)
			}
			b.WriteByte(c)
			escaped = false
		}
	}
	b.WriteByte('"')
	return b.String()
}

func main() {
	check := flag.Bool("check", false, "fail if the files are out of date instead of writing them")
	flag.Parse()
	files := map[string][]byte{
		"web/src/languages.gen.ts":   typescript(),
		"desktop/src/languagelist.h": cpp(),
	}
	stale := false
	for path, want := range files {
		got, _ := os.ReadFile(path)
		if bytes.Equal(got, want) {
			continue
		}
		if *check {
			fmt.Fprintf(os.Stderr, "%s is out of date; run: go run ./langtag/gen\n", path)
			stale = true
			continue
		}
		if err := os.WriteFile(path, want, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("wrote", path)
	}
	if stale {
		os.Exit(1)
	}
}
