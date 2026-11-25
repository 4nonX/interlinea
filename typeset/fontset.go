// Package typeset lays out text in pages and writes them as PDF, with the
// fonts it uses embedded (only the glyphs used). It takes care of what a
// glossary needs: several scripts in one document, joining scripts such
// as Arabic, text written from right to left, and line breaking also
// between Chinese and Japanese characters.
package typeset

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/4nonX/interlinea/typeset/sfnt"
)

// Face is a font as a document uses it: the glyphs used, for the subset.
type Face struct {
	Font *sfnt.Font
	used map[uint16]bool
	// What a glyph stands for when it is not one character's (a ligature),
	// for copying text out of the PDF.
	text map[uint16]string
	id   int
}

func (f *Face) use(g uint16, s string) {
	f.used[g] = true
	if f.text == nil {
		f.text = map[uint16]string{}
	}
	if _, ok := f.text[g]; !ok {
		f.text[g] = s
	}
}

// FontSet is the fonts a document can use, tried in order for each
// character, then the computer's own fonts when allowed.
type FontSet struct {
	faces   []*Face
	system  bool
	missing map[rune]bool
	mu      sync.Mutex
}

// NewFontSet makes a set of the font files given (a collection gives all
// its fonts). With system, characters none of them has are looked up in
// the fonts installed on the computer.
func NewFontSet(files [][]byte, system bool) (*FontSet, error) {
	s := &FontSet{system: system, missing: map[rune]bool{}}
	for _, b := range files {
		for i := 0; i < sfnt.Count(b); i++ {
			f, err := sfnt.ParseIndex(b, i)
			if err != nil {
				return nil, err
			}
			s.add(f)
		}
	}
	return s, nil
}

func (s *FontSet) add(f *sfnt.Font) *Face {
	face := &Face{Font: f, used: map[uint16]bool{}, id: len(s.faces) + 1}
	s.faces = append(s.faces, face)
	return face
}

// Missing lists the characters no font had, in order.
func (s *FontSet) Missing() []rune {
	out := make([]rune, 0, len(s.missing))
	for r := range s.missing {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// Style is how text is set.
type Style struct {
	Bold, Italic bool
	// Lang is the language's tag: it picks the right fonts for Chinese,
	// Japanese and Korean, and tells right-to-left text.
	Lang string
}

// face finds the font for a character: one with the glyph, of the style
// asked for if there is one.
func (s *FontSet) face(r rune, st Style) *Face {
	var best *Face
	bestScore := -1
	for _, f := range s.faces {
		if !f.Font.Has(r) {
			continue
		}
		score := styleScore(f.Font, st)
		if score > bestScore {
			best, bestScore = f, score
		}
	}
	if best != nil || !s.system {
		return best
	}
	if f := systemFont(r, st); f != nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		for _, have := range s.faces {
			if have.Font == f {
				return have
			}
		}
		return s.add(f)
	}
	return nil
}

func styleScore(f *sfnt.Font, st Style) int {
	score := 0
	if f.Bold == st.Bold {
		score += 2
	}
	if f.Italic == st.Italic {
		score++
	}
	return score
}

// The fonts on the computer, read once and only when a character needs
// one: where each is and what it covers.
type systemEntry struct {
	path    string
	index   int
	family  string
	bold    bool
	italic  bool
	ranges  [][2]rune
	loaded  *sfnt.Font
	quality int
}

var (
	systemOnce  sync.Once
	systemFonts []*systemEntry
	systemMu    sync.Mutex
)

// SystemFontDirs are the folders fonts are installed in on this system,
// after those INTERLINEA_FONTS names (separated as PATH is).
func SystemFontDirs() []string {
	var extra []string
	if v := os.Getenv("INTERLINEA_FONTS"); v != "" {
		extra = filepath.SplitList(v)
	}
	return append(extra, systemDirs()...)
}

func systemDirs() []string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "windows":
		dirs := []string{filepath.Join(os.Getenv("WINDIR"), "Fonts")}
		if l := os.Getenv("LOCALAPPDATA"); l != "" {
			dirs = append(dirs, filepath.Join(l, "Microsoft", "Windows", "Fonts"))
		}
		return dirs
	case "darwin":
		return []string{"/System/Library/Fonts", "/Library/Fonts", filepath.Join(home, "Library", "Fonts")}
	}
	return []string{"/usr/share/fonts", "/usr/local/share/fonts", filepath.Join(home, ".fonts"), filepath.Join(home, ".local", "share", "fonts")}
}

func scanSystem() {
	for _, dir := range SystemFontDirs() {
		filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(p))
			if ext != ".ttf" && ext != ".otf" && ext != ".ttc" && ext != ".otc" {
				return nil
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return nil
			}
			for i := 0; i < sfnt.Count(b); i++ {
				f, err := sfnt.ParseIndex(b, i)
				if err != nil {
					continue
				}
				e := &systemEntry{path: p, index: i, family: f.Family, bold: f.Bold, italic: f.Italic, ranges: rangesOf(f.Runes())}
				low := strings.ToLower(f.Family + " " + f.Name)
				switch {
				case strings.Contains(low, "emoji") || strings.Contains(low, "symbol") || strings.Contains(low, "mono"):
					e.quality = -2
				case strings.Contains(low, "serif") || strings.Contains(low, "mincho") || strings.Contains(low, "song") || strings.Contains(low, "ming"):
					e.quality = -1
				case strings.Contains(low, "sans") || strings.Contains(low, "gothic") || strings.Contains(low, "hei"):
					e.quality = 1
				}
				systemFonts = append(systemFonts, e)
			}
			return nil
		})
	}
}

func rangesOf(m map[rune]uint16) [][2]rune {
	rs := make([]rune, 0, len(m))
	for r := range m {
		rs = append(rs, r)
	}
	sort.Slice(rs, func(i, j int) bool { return rs[i] < rs[j] })
	var out [][2]rune
	for _, r := range rs {
		if n := len(out); n > 0 && out[n-1][1]+1 == r {
			out[n-1][1] = r
			continue
		}
		out = append(out, [2]rune{r, r})
	}
	return out
}

func (e *systemEntry) has(r rune) bool {
	i := sort.Search(len(e.ranges), func(i int) bool { return e.ranges[i][1] >= r })
	return i < len(e.ranges) && e.ranges[i][0] <= r
}

// The family names that mark a font for one of the CJK languages.
func langHint(lang string) []string {
	l := strings.ToLower(lang)
	switch {
	case strings.HasPrefix(l, "ja"):
		return []string{" jp", "japan", "gothic", "mincho"}
	case strings.HasPrefix(l, "ko"):
		return []string{" kr", "korea", "gothic", "malgun"}
	case strings.HasPrefix(l, "zh") && (strings.Contains(l, "tw") || strings.Contains(l, "hk") || strings.Contains(l, "mo") || strings.Contains(l, "hant")):
		return []string{" tc", " hk", "jhenghei", "traditional"}
	case strings.HasPrefix(l, "zh"):
		return []string{" sc", "yahei", "simplified", "simhei"}
	}
	return nil
}

func systemFont(r rune, st Style) *sfnt.Font {
	systemOnce.Do(scanSystem)
	systemMu.Lock()
	defer systemMu.Unlock()
	var best *systemEntry
	bestScore := -1 << 30
	hints := langHint(st.Lang)
	for _, e := range systemFonts {
		if !e.has(r) {
			continue
		}
		score := e.quality * 4
		if e.bold == st.Bold {
			score += 2
		}
		if e.italic == st.Italic {
			score++
		}
		fam := " " + strings.ToLower(e.family)
		for _, h := range hints {
			if strings.Contains(fam, h) {
				score += 8
				break
			}
		}
		if score > bestScore {
			best, bestScore = e, score
		}
	}
	if best == nil {
		return nil
	}
	if best.loaded == nil {
		b, err := os.ReadFile(best.path)
		if err != nil {
			return nil
		}
		f, err := sfnt.ParseIndex(b, best.index)
		if err != nil {
			return nil
		}
		best.loaded = f
	}
	return best.loaded
}
