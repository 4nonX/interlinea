// Package fonts holds the fonts documents are typeset in: Noto Sans for
// Latin, Greek and Cyrillic (regular, bold, italic, bold italic), and Noto
// Sans Arabic and Noto Sans Hebrew (regular and bold), under the SIL Open
// Font License (OFL.txt). Other scripts take fonts installed on the
// computer (see typeset.SystemFonts).
package fonts

import "embed"

//go:embed *.ttf OFL.txt
var files embed.FS

// Names are the font files, in the order fonts are tried for a character.
var Names = []string{
	"NotoSans-Regular.ttf",
	"NotoSans-Bold.ttf",
	"NotoSans-Italic.ttf",
	"NotoSans-BoldItalic.ttf",
	"NotoSansArabic-Regular.ttf",
	"NotoSansArabic-Bold.ttf",
	"NotoSansHebrew-Regular.ttf",
	"NotoSansHebrew-Bold.ttf",
}

// All gives the bundled font files.
func All() [][]byte {
	out := make([][]byte, 0, len(Names))
	for _, n := range Names {
		b, err := files.ReadFile(n)
		if err != nil {
			panic(err) // embedded at build time
		}
		out = append(out, b)
	}
	return out
}

// License is the fonts' licence, to go with a program that ships them.
func License() string {
	b, _ := files.ReadFile("OFL.txt")
	return string(b)
}
