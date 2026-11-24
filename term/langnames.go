package term

import (
	"strings"

	"github.com/4nonX/interlinea/langtag"
)

// Language names as they appear in MultiTerm files and spreadsheet
// headers, in English, German and the language itself.
var langNames = []struct {
	code  string
	names []string
}{
	{"de", []string{"german", "deutsch"}},
	{"de-DE", []string{"german (germany)", "deutsch (deutschland)"}},
	{"de-AT", []string{"german (austria)", "deutsch (österreich)"}},
	{"de-CH", []string{"german (switzerland)", "deutsch (schweiz)"}},
	{"en", []string{"english", "englisch"}},
	{"en-GB", []string{"english (united kingdom)", "english (uk)", "english (great britain)", "englisch (großbritannien)", "british english"}},
	{"en-US", []string{"english (united states)", "english (us)", "english (usa)", "englisch (usa)", "american english"}},
	{"fr", []string{"french", "französisch", "français", "francais"}},
	{"fr-FR", []string{"french (france)", "französisch (frankreich)"}},
	{"es", []string{"spanish", "spanisch", "español", "espanol"}},
	{"es-ES", []string{"spanish (spain)", "spanisch (spanien)"}},
	{"it", []string{"italian", "italienisch", "italiano"}},
	{"nl", []string{"dutch", "niederländisch", "nederlands"}},
	{"pl", []string{"polish", "polnisch", "polski"}},
	{"pt", []string{"portuguese", "portugiesisch", "português", "portugues"}},
	{"pt-BR", []string{"portuguese (brazil)", "portugiesisch (brasilien)"}},
	{"ru", []string{"russian", "russisch", "русский"}},
	{"uk", []string{"ukrainian", "ukrainisch", "українська"}},
	{"cs", []string{"czech", "tschechisch", "čeština"}},
	{"sk", []string{"slovak", "slowakisch", "slovenčina"}},
	{"sl", []string{"slovenian", "slowenisch", "slovenščina"}},
	{"hr", []string{"croatian", "kroatisch", "hrvatski"}},
	{"hu", []string{"hungarian", "ungarisch", "magyar"}},
	{"ro", []string{"romanian", "rumänisch", "română"}},
	{"bg", []string{"bulgarian", "bulgarisch", "български"}},
	{"el", []string{"greek", "griechisch", "ελληνικά"}},
	{"tr", []string{"turkish", "türkisch", "türkçe"}},
	{"da", []string{"danish", "dänisch", "dansk"}},
	{"sv", []string{"swedish", "schwedisch", "svenska"}},
	{"nb", []string{"norwegian", "norwegisch", "norsk", "norwegian (bokmål)"}},
	{"fi", []string{"finnish", "finnisch", "suomi"}},
	{"ja", []string{"japanese", "japanisch", "日本語"}},
	{"zh", []string{"chinese", "chinesisch", "中文"}},
	{"zh-CN", []string{"chinese (simplified)", "chinese (prc)", "chinesisch (vereinfacht)"}},
	{"zh-TW", []string{"chinese (traditional)", "chinese (taiwan)", "chinesisch (traditionell)"}},
	{"ko", []string{"korean", "koreanisch", "한국어"}},
	{"ar", []string{"arabic", "arabisch", "العربية"}},
	{"he", []string{"hebrew", "hebräisch", "עברית"}},
	{"lt", []string{"lithuanian", "litauisch", "lietuvių"}},
	{"lv", []string{"latvian", "lettisch", "latviešu"}},
	{"et", []string{"estonian", "estnisch", "eesti"}},
}

// LangFromName recognises a language name or a language tag ("German",
// "Deutsch", "EN-GB", "de_de") and returns its BCP 47 tag.
func LangFromName(s string) (string, bool) {
	k := strings.ToLower(strings.TrimSpace(s))
	if k == "" {
		return "", false
	}
	for _, l := range langNames {
		for _, n := range l.names {
			if k == n {
				return l.code, true
			}
		}
	}
	// The names of the languages the apps list: "Thai (Thailand)", "Thai",
	// "ไทย".
	for _, l := range langtag.Languages() {
		if k == strings.ToLower(l.Name) {
			return l.Tag, true
		}
	}
	for _, l := range langtag.Languages() {
		bare, _, _ := strings.Cut(strings.ToLower(l.Name), " (")
		if k == bare || k == strings.ToLower(l.Native) {
			return langtag.Primary(l.Tag), true
		}
	}
	// A tag on its own: a known language, optionally with a script and a
	// region. A field named "Nl-term" has the shape of a tag but is none.
	if langtag.WellFormed(k) {
		t := langtag.MustNormalize(k)
		p := langtag.Primary(t)
		for _, l := range langNames {
			if langtag.Primary(l.code) == p {
				return t, true
			}
		}
		for _, l := range langtag.Languages() {
			if langtag.Primary(l.Tag) == p {
				return t, true
			}
		}
	}
	return "", false
}

// LangName is the English name of a language tag ("German (Germany)").
func LangName(code string) string {
	c := NormalizeLang(code)
	if l, ok := langtag.Lookup(c); ok {
		return l.Name
	}
	for _, l := range langNames {
		if strings.EqualFold(l.code, c) {
			return titleCase(l.names[0])
		}
	}
	p := langtag.Primary(c)
	for _, l := range langNames {
		if strings.EqualFold(l.code, p) {
			return titleCase(l.names[0])
		}
	}
	// A language listed only with its regions: its name without one.
	for _, l := range langtag.Languages() {
		if langtag.Primary(l.Tag) == p {
			bare, _, _ := strings.Cut(l.Name, " (")
			return bare
		}
	}
	return code
}

func titleCase(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		if strings.HasPrefix(w, "(") && len(w) > 1 {
			words[i] = "(" + strings.ToUpper(w[1:2]) + w[2:]
			continue
		}
		if w == "uk" || w == "us" || w == "usa" || w == "prc" {
			words[i] = strings.ToUpper(w)
			continue
		}
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}
