// Package langdetect guesses the language of a document's text.
//
// A script that one language writes (Korean, Thai, Greek, Georgian and so
// on) decides by its share of the letters. A script that several
// languages share (Latin, Cyrillic, Arabic, Devanagari, Chinese
// characters) decides by frequent function words or by the letters and
// characters only some of them use. This is reliable on a page of running
// text and says so when the text is too short or too mixed to tell: the
// caller then asks the person to confirm.
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
	seen := map[string]bool{}
	var out []string
	add := func(l string) {
		if !seen[l] {
			seen[l] = true
			out = append(out, l)
		}
	}
	for _, s := range scripts {
		add(s.lang)
	}
	for _, l := range []string{"ja-JP", "zh-CN", "zh-TW", "fa-IR", "ur-PK", "mr-IN", "ne-NP"} {
		add(l)
	}
	for l := range stopwords {
		add(l)
	}
	sort.Strings(out)
	return out
}

// scripts are the writing systems that tell the language, or narrow it to
// a few. Latin is what is left.
var scripts = []struct {
	lang  string
	table *unicode.RangeTable
}{
	{"ko-KR", unicode.Hangul},
	{"ja-JP", unicode.Hiragana},
	{"ja-JP", unicode.Katakana},
	{"zh-CN", unicode.Han},
	{"ar-SA", unicode.Arabic},
	{"he-IL", unicode.Hebrew},
	{"el-GR", unicode.Greek},
	{"ru-RU", unicode.Cyrillic},
	{"th-TH", unicode.Thai},
	{"lo-LA", unicode.Lao},
	{"km-KH", unicode.Khmer},
	{"my-MM", unicode.Myanmar},
	{"hi-IN", unicode.Devanagari},
	{"bn-BD", unicode.Bengali},
	{"pa-IN", unicode.Gurmukhi},
	{"gu-IN", unicode.Gujarati},
	{"ta-IN", unicode.Tamil},
	{"te-IN", unicode.Telugu},
	{"kn-IN", unicode.Kannada},
	{"ml-IN", unicode.Malayalam},
	{"si-LK", unicode.Sinhala},
	{"ka-GE", unicode.Georgian},
	{"hy-AM", unicode.Armenian},
	{"am-ET", unicode.Ethiopic},
}

// sharedScript lists the languages of a script that several write; the
// first is the guess when nothing tells them apart.
var sharedScript = map[string][]string{
	"ru-RU": {"ru-RU", "uk-UA", "be-BY", "bg-BG", "mk-MK", "sr-Cyrl-RS", "kk-KZ", "mn-MN"},
	"hi-IN": {"hi-IN", "mr-IN", "ne-NP"},
	"ar-SA": {"ar-SA", "fa-IR", "ur-PK"},
}

// tieBreakers are letters only some languages use.
var tieBreakers = map[string]string{
	"de-DE": "äöüß", "fr-FR": "éèêàçù", "es-ES": "ñ¿¡", "pl-PL": "ąęłńśźż", "pt-PT": "ãõ", "sv-SE": "åä", "da-DK": "æø",
	"nb-NO": "æø", "et-EE": "õ", "cs-CZ": "ěřů", "sk-SK": "ľĺŕô", "hu-HU": "őű", "ro-RO": "țșă", "tr-TR": "ğış", "hr-HR": "đ",
	"lt-LT": "ėįųū", "lv-LV": "ļņķģ", "vi-VN": "ơưạảấầậđệộ", "ca-ES": "·", "is-IS": "ðþ", "mt-MT": "ħġż", "az-AZ": "ə",
	"sq-AL": "ë", "uk-UA": "їєґі", "be-BY": "ў", "mk-MK": "ѓќѕ", "sr-Cyrl-RS": "ђћљњџ", "kk-KZ": "әғқңұһ", "mn-MN": "өү",
	"bg-BG": "ъ", "fa-IR": "پچژگ", "ur-PK": "ٹڈڑےں", "mr-IN": "ळ",
}

// Characters that Simplified and Traditional Chinese write differently,
// among the most frequent of each.
const (
	simplifiedOnly  = "这为国们来时个对说会发经过还后里么无与东车长门问间马见现员区应开关书电话语读写约租赁务权义项条规签订双违责赔偿终协议证业产种当设务实际资认让该应数据两时间组织报进运动实现处总结选择"
	traditionalOnly = "這為國們來時個對說會發經過還後裡麼無與東車長門問間馬見現員區應開關書電話語讀寫約租賃務權義項條規簽訂雙違責賠償終協議證業產種當設務實際資認讓該應數據兩時間組織報進運動實現處總結選擇"
)

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
			if unicode.Is(s.table, r) {
				counts[i]++
			}
		}
	}
	if letters == 0 {
		letters = 1
	}
	byLang := map[string]int{}
	for i, s := range scripts {
		byLang[s.lang] += counts[i]
	}
	kana := byLang["ja-JP"]
	best := ""
	for _, s := range scripts {
		if byLang[s.lang] > 0 && (best == "" || byLang[s.lang] > byLang[best]) {
			best = s.lang
		}
	}
	if best != "" && float64(byLang[best])/float64(letters) > 0.3 {
		n := byLang[best]
		lang := best
		// Japanese is written with Chinese characters too; kana decide it.
		if kana > 0 && (lang == "zh-CN" || lang == "ja-JP") {
			lang = "ja-JP"
			n = byLang["zh-CN"] + kana
		}
		if lang == "zh-CN" {
			simp, trad := 0, 0
			for _, r := range text {
				inSimp, inTrad := strings.ContainsRune(simplifiedOnly, r), strings.ContainsRune(traditionalOnly, r)
				switch {
				case inSimp && !inTrad:
					simp++
				case inTrad && !inSimp:
					trad++
				}
			}
			if trad > simp {
				lang = "zh-TW"
			}
		}
		share := math.Min(1, float64(n)/float64(letters))
		conf := math.Min(0.99, 0.6+share*0.4)
		if family, ok := sharedScript[lang]; ok {
			return amongWritten(text, family, n, conf)
		}
		return Detection{Lang: lang, Confidence: conf, Alternatives: []Guess{}, Basis: n, Unit: "characters", Reason: OK}
	}
	var latin []string
	for l := range stopwords {
		if !inSharedScript(l) {
			latin = append(latin, l)
		}
	}
	return byWords(text, latin, none)
}

func inSharedScript(lang string) bool {
	for _, family := range sharedScript {
		for _, l := range family {
			if l == lang {
				return true
			}
		}
	}
	return false
}

// amongWritten tells the languages of one script apart: by their function
// words where there is enough text, else by the letters only one of them
// writes, else it is the script's most written language.
func amongWritten(text string, family []string, basis int, scriptConfidence float64) Detection {
	d := byWords(text, family, func(int, Reason) Detection { return Detection{} })
	if d.Lang != "" && d.Reason == OK {
		return d
	}
	// The languages whose letters no other of the family writes come
	// first; і is Ukrainian, Belarusian and Kazakh, so it decides last.
	lower := strings.ToLower(text)
	for _, l := range []string{"be-BY", "mk-MK", "sr-Cyrl-RS", "kk-KZ", "mn-MN", "uk-UA", "fa-IR", "ur-PK", "mr-IN", "bg-BG"} {
		in := false
		for _, f := range family {
			in = in || f == l
		}
		chars, ok := tieBreakers[l]
		if !in || !ok || !strings.ContainsAny(lower, chars) {
			continue
		}
		if l == "bg-BG" && (strings.Count(lower, "ъ") < 2 || strings.ContainsAny(lower, "ыэё")) {
			// Russian writes ъ too, but rarely, and Bulgarian has no ы, э or ё.
			continue
		}
		return Detection{Lang: l, Confidence: math.Min(scriptConfidence, 0.7), Alternatives: []Guess{{Lang: family[0], Confidence: 0.3}}, Basis: basis, Unit: "characters", Reason: OK}
	}
	// The script is certain, the language within it is not.
	alt := []Guess{}
	for _, l := range family[1:] {
		alt = append(alt, Guess{Lang: l, Confidence: 0.1})
		if len(alt) == 2 {
			break
		}
	}
	return Detection{Lang: family[0], Confidence: math.Min(scriptConfidence, 0.6), Alternatives: alt, Basis: basis, Unit: "characters", Reason: OK}
}

var stopSets = func() map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for lang, list := range stopwords {
		set := make(map[string]bool, len(list))
		for _, w := range list {
			set[w] = true
		}
		out[lang] = set
	}
	return out
}()

// byWords scores the languages by how many of the text's words are
// function words of each.
func byWords(text string, langs []string, none func(int, Reason) Detection) Detection {
	lower := strings.ToLower(text)
	words := strings.FieldsFunc(lower, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsMark(r) && r != '\'' && r != '’' })
	if len(words) < 8 {
		return none(len(words), Short)
	}
	type score struct {
		lang string
		hits float64
	}
	// A word counts for less the more of the languages have it: "de" and
	// "a" say little, "und" and "the" a lot.
	shared := map[string]int{}
	for _, w := range words {
		if _, done := shared[w]; done {
			continue
		}
		n := 0
		for _, lang := range langs {
			if stopSets[lang][w] {
				n++
			}
		}
		shared[w] = n
	}
	var scores []score
	for _, lang := range langs {
		set := stopSets[lang]
		n := 0.0
		for _, w := range words {
			if set[w] {
				n += 1 / float64(shared[w])
			}
		}
		if chars, ok := tieBreakers[lang]; ok && strings.ContainsAny(lower, chars) {
			n += math.Max(0.5, float64(len(words))*0.01)
		}
		scores = append(scores, score{lang, n})
	}
	sort.Slice(scores, func(i, j int) bool {
		if scores[i].hits != scores[j].hits {
			return scores[i].hits > scores[j].hits
		}
		return scores[i].lang < scores[j].lang
	})
	// The share is taken among the few best: with many languages in the
	// running, the stray hits of the rest would otherwise drown the leader.
	total := 0.0
	for i, s := range scores {
		if i < 4 {
			total += s.hits
		}
	}
	if total == 0 || len(scores) < 2 {
		return none(len(words), Mixed)
	}
	top, second := scores[0], scores[1]
	share := top.hits / total
	margin := (top.hits - second.hits) / top.hits
	// Confidence grows with the share of function words that point the
	// same way and with the amount of text.
	amount := math.Min(1, float64(len(words))/40)
	conf := math.Max(0, math.Min(0.99, (share*0.6+margin*0.4)*(0.55+0.45*amount)))
	d := Detection{Lang: top.lang, Confidence: conf, Alternatives: []Guess{}, Basis: len(words), Unit: "words", Reason: OK}
	for _, s := range scores[1:3] {
		if s.hits > 0 {
			d.Alternatives = append(d.Alternatives, Guess{Lang: s.lang, Confidence: s.hits / total})
		}
	}
	if conf < Confident && share < 0.6 {
		d.Reason = Mixed
	}
	return d
}
