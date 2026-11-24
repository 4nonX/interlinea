package langtag

import "strings"

// Language is a language the apps offer by name. Every other valid tag
// works as well; these are the ones listed, named and tested.
type Language struct {
	// Tag is the BCP 47 tag ("de-DE", "sr-Latn-RS").
	Tag string `json:"tag"`
	// Name is the English name, with the country or script in brackets.
	Name string `json:"name"`
	// Native is the language's own name.
	Native string `json:"native"`
	// RTL says the language is written from right to left.
	RTL bool `json:"rtl,omitempty"`
}

// Languages is the list the apps offer, the same in the browser client and
// the desktop app, in the order shown: sorted by English name.
func Languages() []Language {
	out := make([]Language, len(languages))
	copy(out, languages)
	return out
}

// Lookup returns the listed language with the tag.
func Lookup(tag string) (Language, bool) {
	n, err := Normalize(tag)
	if err != nil {
		return Language{}, false
	}
	for _, l := range languages {
		if l.Tag == n {
			return l, true
		}
	}
	return Language{}, false
}

// rtlLanguages are written from right to left, whatever the region.
var rtlLanguages = map[string]bool{
	"ar": true, "he": true, "fa": true, "ur": true, "ps": true, "sd": true, "ug": true, "yi": true, "dv": true, "ckb": true,
}

// rtlScripts are the scripts written from right to left.
var rtlScripts = map[string]bool{"Arab": true, "Hebr": true, "Thaa": true, "Syrc": true, "Nkoo": true, "Adlm": true}

// RTL reports whether a tag's language is written from right to left. A
// script in the tag decides (az-Arab is, ur-Latn is not).
func RTL(tag string) bool {
	if s := scriptSubtag(tag); s != "" {
		return rtlScripts[s]
	}
	return rtlLanguages[Primary(tag)]
}

func scriptSubtag(tag string) string {
	n, err := Normalize(tag)
	if err != nil {
		return ""
	}
	parts := strings.Split(n, "-")
	if len(parts) > 1 && len(parts[1]) == 4 && isAlpha(parts[1]) {
		return parts[1]
	}
	return ""
}

func regionSubtag(tag string) string {
	n, err := Normalize(tag)
	if err != nil {
		return ""
	}
	for i, p := range strings.Split(n, "-") {
		if i > 0 && i <= 2 && ((len(p) == 2 && isAlpha(p)) || (len(p) == 3 && isDigit(p))) {
			return p
		}
	}
	return ""
}

// Script returns the script a tag is written in where a language has more
// than one in use: the tag's own script subtag, or the one its region
// implies (Hant for zh-TW, Hans for zh-CN, Cyrl for sr-RS). It is empty
// where the language alone settles it.
func Script(tag string) string {
	if s := scriptSubtag(tag); s != "" {
		return s
	}
	region := regionSubtag(tag)
	switch Primary(tag) {
	case "zh":
		switch region {
		case "TW", "HK", "MO":
			return "Hant"
		case "CN", "SG", "MY":
			return "Hans"
		}
	case "sr":
		switch region {
		case "RS", "BA", "ME", "XK":
			return "Cyrl"
		}
	}
	return ""
}

// The names follow the Unicode CLDR's English and native names.
var languages = []Language{
	{"af-ZA", "Afrikaans (South Africa)", "Afrikaans", false},
	{"sq-AL", "Albanian (Albania)", "Shqip", false},
	{"am-ET", "Amharic (Ethiopia)", "አማርኛ", false},
	{"ar-EG", "Arabic (Egypt)", "العربية", true},
	{"ar-SA", "Arabic (Saudi Arabia)", "العربية", true},
	{"ar-AE", "Arabic (United Arab Emirates)", "العربية", true},
	{"hy-AM", "Armenian (Armenia)", "Հայերեն", false},
	{"az-AZ", "Azerbaijani (Azerbaijan)", "Azərbaycan", false},
	{"eu-ES", "Basque (Spain)", "Euskara", false},
	{"be-BY", "Belarusian (Belarus)", "Беларуская", false},
	{"bn-BD", "Bengali (Bangladesh)", "বাংলা", false},
	{"bs-BA", "Bosnian (Bosnia and Herzegovina)", "Bosanski", false},
	{"bg-BG", "Bulgarian (Bulgaria)", "Български", false},
	{"my-MM", "Burmese (Myanmar)", "မြန်မာ", false},
	{"ca-ES", "Catalan (Spain)", "Català", false},
	{"zh-CN", "Chinese (Simplified, China)", "简体中文", false},
	{"zh-HK", "Chinese (Traditional, Hong Kong)", "繁體中文", false},
	{"zh-TW", "Chinese (Traditional, Taiwan)", "繁體中文", false},
	{"hr-HR", "Croatian (Croatia)", "Hrvatski", false},
	{"cs-CZ", "Czech (Czechia)", "Čeština", false},
	{"da-DK", "Danish (Denmark)", "Dansk", false},
	{"nl-BE", "Dutch (Belgium)", "Nederlands", false},
	{"nl-NL", "Dutch (Netherlands)", "Nederlands", false},
	{"en-AU", "English (Australia)", "English", false},
	{"en-CA", "English (Canada)", "English", false},
	{"en-IE", "English (Ireland)", "English", false},
	{"en-GB", "English (United Kingdom)", "English", false},
	{"en-US", "English (United States)", "English", false},
	{"et-EE", "Estonian (Estonia)", "Eesti", false},
	{"fil-PH", "Filipino (Philippines)", "Filipino", false},
	{"fi-FI", "Finnish (Finland)", "Suomi", false},
	{"fr-BE", "French (Belgium)", "Français", false},
	{"fr-CA", "French (Canada)", "Français", false},
	{"fr-FR", "French (France)", "Français", false},
	{"fr-CH", "French (Switzerland)", "Français", false},
	{"gl-ES", "Galician (Spain)", "Galego", false},
	{"ka-GE", "Georgian (Georgia)", "ქართული", false},
	{"de-AT", "German (Austria)", "Deutsch", false},
	{"de-DE", "German (Germany)", "Deutsch", false},
	{"de-CH", "German (Switzerland)", "Deutsch", false},
	{"el-GR", "Greek (Greece)", "Ελληνικά", false},
	{"gu-IN", "Gujarati (India)", "ગુજરાતી", false},
	{"he-IL", "Hebrew (Israel)", "עברית", true},
	{"hi-IN", "Hindi (India)", "हिन्दी", false},
	{"hu-HU", "Hungarian (Hungary)", "Magyar", false},
	{"is-IS", "Icelandic (Iceland)", "Íslenska", false},
	{"id-ID", "Indonesian (Indonesia)", "Indonesia", false},
	{"ga-IE", "Irish (Ireland)", "Gaeilge", false},
	{"it-IT", "Italian (Italy)", "Italiano", false},
	{"it-CH", "Italian (Switzerland)", "Italiano", false},
	{"ja-JP", "Japanese (Japan)", "日本語", false},
	{"kn-IN", "Kannada (India)", "ಕನ್ನಡ", false},
	{"kk-KZ", "Kazakh (Kazakhstan)", "Қазақ тілі", false},
	{"km-KH", "Khmer (Cambodia)", "ខ្មែរ", false},
	{"ko-KR", "Korean (South Korea)", "한국어", false},
	{"lo-LA", "Lao (Laos)", "ລາວ", false},
	{"lv-LV", "Latvian (Latvia)", "Latviešu", false},
	{"lt-LT", "Lithuanian (Lithuania)", "Lietuvių", false},
	{"lb-LU", "Luxembourgish (Luxembourg)", "Lëtzebuergesch", false},
	{"mk-MK", "Macedonian (North Macedonia)", "Македонски", false},
	{"ms-MY", "Malay (Malaysia)", "Melayu", false},
	{"ml-IN", "Malayalam (India)", "മലയാളം", false},
	{"mt-MT", "Maltese (Malta)", "Malti", false},
	{"mr-IN", "Marathi (India)", "मराठी", false},
	{"mn-MN", "Mongolian (Mongolia)", "Монгол", false},
	{"ne-NP", "Nepali (Nepal)", "नेपाली", false},
	{"nb-NO", "Norwegian Bokmål (Norway)", "Norsk bokmål", false},
	{"nn-NO", "Norwegian Nynorsk (Norway)", "Norsk nynorsk", false},
	{"fa-IR", "Persian (Iran)", "فارسی", true},
	{"pl-PL", "Polish (Poland)", "Polski", false},
	{"pt-BR", "Portuguese (Brazil)", "Português", false},
	{"pt-PT", "Portuguese (Portugal)", "Português", false},
	{"pa-IN", "Punjabi (India)", "ਪੰਜਾਬੀ", false},
	{"ro-RO", "Romanian (Romania)", "Română", false},
	{"ru-RU", "Russian (Russia)", "Русский", false},
	{"sr-Cyrl-RS", "Serbian (Cyrillic, Serbia)", "Српски", false},
	{"sr-Latn-RS", "Serbian (Latin, Serbia)", "Srpski", false},
	{"si-LK", "Sinhala (Sri Lanka)", "සිංහල", false},
	{"sk-SK", "Slovak (Slovakia)", "Slovenčina", false},
	{"sl-SI", "Slovenian (Slovenia)", "Slovenščina", false},
	{"es-419", "Spanish (Latin America)", "Español", false},
	{"es-MX", "Spanish (Mexico)", "Español", false},
	{"es-ES", "Spanish (Spain)", "Español", false},
	{"sw-KE", "Swahili (Kenya)", "Kiswahili", false},
	{"sv-SE", "Swedish (Sweden)", "Svenska", false},
	{"ta-IN", "Tamil (India)", "தமிழ்", false},
	{"te-IN", "Telugu (India)", "తెలుగు", false},
	{"th-TH", "Thai (Thailand)", "ไทย", false},
	{"tr-TR", "Turkish (Türkiye)", "Türkçe", false},
	{"uk-UA", "Ukrainian (Ukraine)", "Українська", false},
	{"ur-PK", "Urdu (Pakistan)", "اردو", true},
	{"uz-UZ", "Uzbek (Uzbekistan)", "O‘zbek", false},
	{"vi-VN", "Vietnamese (Vietnam)", "Tiếng Việt", false},
	{"cy-GB", "Welsh (United Kingdom)", "Cymraeg", false},
}

// scripts are the ISO 15924 codes of the scripts in use for the languages
// people translate.
var scripts = map[string]bool{
	"Latn": true, "Cyrl": true, "Grek": true, "Arab": true, "Hebr": true, "Hans": true, "Hant": true, "Hani": true, "Jpan": true,
	"Kore": true, "Hang": true, "Hira": true, "Kana": true, "Deva": true, "Beng": true, "Guru": true, "Gujr": true, "Orya": true,
	"Taml": true, "Telu": true, "Knda": true, "Mlym": true, "Sinh": true, "Thai": true, "Laoo": true, "Khmr": true, "Mymr": true,
	"Tibt": true, "Geor": true, "Armn": true, "Ethi": true, "Mong": true, "Thaa": true, "Syrc": true, "Nkoo": true, "Adlm": true,
	"Tfng": true, "Cher": true, "Cans": true, "Vaii": true, "Yiii": true,
}

// WellFormed reports whether a tag is one a file or a column heading would
// carry for a language: a language of two or three letters, then at most a
// known script and a region. A heading such as "Nl-term" or "Note" has the
// shape of a tag but is not one.
func WellFormed(tag string) bool {
	n, err := Normalize(tag)
	if err != nil {
		return false
	}
	parts := strings.Split(n, "-")
	if len(parts[0]) > 3 || len(parts) > 3 {
		return false
	}
	rest := parts[1:]
	if len(rest) > 0 && len(rest[0]) == 4 {
		if !scripts[rest[0]] {
			return false
		}
		rest = rest[1:]
	}
	if len(rest) > 0 {
		if !((len(rest[0]) == 2 && isAlpha(rest[0])) || (len(rest[0]) == 3 && isDigit(rest[0]))) {
			return false
		}
		rest = rest[1:]
	}
	return len(rest) == 0
}
