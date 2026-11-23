// Package langdetect guesses the language of a document's text.
//
// Scripts with a writing system of their own (Korean, Japanese, Chinese,
// Arabic) decide by their share of the letters. Languages written in Latin
// letters decide by frequent function words, with the letters only some of
// them use breaking near-ties. This is reliable on a page of running text
// and says so when the text is too short or too mixed to tell: the caller
// then asks the person to confirm.
package langdetect

import (
	"math"
	"sort"
	"strings"
	"unicode"
)

// Confident is the confidence at which a guess needs no confirmation.
const Confident = 0.75

// Reason says why there is a guess or none.
type Reason string

const (
	// OK is a guess the text supports.
	OK Reason = "ok"
	// Empty text gives no guess.
	Empty Reason = "empty"
	// Short text (fewer than eight words) gives no guess.
	Short Reason = "short"
	// Mixed text points at several languages.
	Mixed Reason = "mixed"
)

// Guess is another language the text may be in.
type Guess struct {
	Lang       string  `json:"lang"`
	Confidence float64 `json:"confidence"`
}

// Detection is a guess at a text's language.
type Detection struct {
	// Lang is the best guess (a BCP 47 tag), empty when the text gives no
	// usable evidence.
	Lang string `json:"lang"`
	// Confidence is from 0 to 1; below Confident the person confirms.
	Confidence float64 `json:"confidence"`
	// Alternatives are the runners-up, best first.
	Alternatives []Guess `json:"alternatives"`
	// Basis is the amount of text the guess rests on, in Unit.
	Basis int `json:"basis"`
	// Unit is "words", or "characters" for scripts written without spaces.
	Unit   string `json:"unit"`
	Reason Reason `json:"reason"`
}

// Languages are the languages Detect tells apart.
func Languages() []string {
	out := []string{"ko-KR", "ja-JP", "zh-CN", "ar-SA"}
	for l := range stopwords {
		out = append(out, l)
	}
	sort.Strings(out)
	return out
}

var scripts = []struct {
	lang string
	in   func(rune) bool
}{
	{"ko-KR", func(r rune) bool { return (r >= 0xAC00 && r <= 0xD7AF) || (r >= 0x1100 && r <= 0x11FF) }},
	{"ja-JP", func(r rune) bool { return r >= 0x3040 && r <= 0x30FF }},
	{"zh-CN", func(r rune) bool { return r >= 0x4E00 && r <= 0x9FFF }},
	{"ar-SA", func(r rune) bool { return r >= 0x0600 && r <= 0x06FF }},
}

// tieBreakers are letters only some languages use.
var tieBreakers = map[string]string{
	"de-DE": "äöüß",
	"fr-FR": "éèêàçù",
	"es-ES": "ñ¿¡",
	"pl-PL": "ąęłńśźż",
}

// Detect guesses the language of text.
func Detect(text string) Detection {
	text = strings.TrimSpace(text)
	none := func(basis int, reason Reason) Detection {
		return Detection{Alternatives: []Guess{}, Basis: basis, Unit: "words", Reason: reason}
	}
	if text == "" {
		return none(0, Empty)
	}
	letters := 0
	counts := make([]int, len(scripts))
	for _, r := range text {
		if unicode.IsLetter(r) {
			letters++
		}
		for i, s := range scripts {
			if s.in(r) {
				counts[i]++
			}
		}
	}
	if letters == 0 {
		letters = 1
	}
	kana := counts[1]
	best := -1
	for i, n := range counts {
		if n > 0 && (best < 0 || n > counts[best]) {
			best = i
		}
	}
	if best >= 0 && float64(counts[best])/float64(letters) > 0.3 {
		lang := scripts[best].lang
		// Japanese is written with Han characters too; kana decide it.
		n := counts[best]
		if kana > 0 && lang == "zh-CN" {
			lang = "ja-JP"
		}
		if lang == "ja-JP" {
			n += kana
		}
		share := math.Min(1, float64(n)/float64(letters))
		return Detection{Lang: lang, Confidence: math.Min(0.99, 0.6+share*0.4), Alternatives: []Guess{}, Basis: counts[best], Unit: "characters", Reason: OK}
	}

	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) })
	if len(words) < 8 {
		return none(len(words), Short)
	}
	type score struct {
		lang string
		hits int
	}
	var scores []score
	for lang, list := range stopwords {
		set := make(map[string]bool, len(list))
		for _, w := range list {
			set[w] = true
		}
		n := 0
		for _, w := range words {
			if set[w] {
				n++
			}
		}
		if chars, ok := tieBreakers[lang]; ok && strings.ContainsAny(text, chars) {
			n += max(1, int(math.Round(float64(len(words))*0.01)))
		}
		scores = append(scores, score{lang, n})
	}
	sort.Slice(scores, func(i, j int) bool {
		if scores[i].hits != scores[j].hits {
			return scores[i].hits > scores[j].hits
		}
		return scores[i].lang < scores[j].lang
	})
	total := 0
	for _, s := range scores {
		total += s.hits
	}
	if total == 0 {
		return none(len(words), Mixed)
	}
	top, second := scores[0], scores[1]
	share := float64(top.hits) / float64(total)
	margin := float64(top.hits-second.hits) / float64(top.hits)
	// Confidence grows with the share of function words that point the
	// same way and with the amount of text.
	amount := math.Min(1, float64(len(words))/40)
	conf := math.Max(0, math.Min(0.99, (share*0.6+margin*0.4)*(0.55+0.45*amount)))
	d := Detection{Lang: top.lang, Confidence: conf, Alternatives: []Guess{}, Basis: len(words), Unit: "words", Reason: OK}
	for _, s := range scores[1:3] {
		if s.hits > 0 {
			d.Alternatives = append(d.Alternatives, Guess{Lang: s.lang, Confidence: float64(s.hits) / float64(total)})
		}
	}
	if conf < Confident && share < 0.6 {
		d.Reason = Mixed
	}
	return d
}
