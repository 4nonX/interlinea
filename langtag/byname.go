package langtag

import (
	"strings"
	"sync"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// moreLanguages are languages not in the list the apps offer, by the
// English names CAT tools write in their bilingual files.
var moreLanguages = map[string]string{
	"hmong": "hmn", "haitian": "ht", "haitian creole": "ht", "somali": "so", "tigrinya": "ti",
	"pashto": "ps", "dari": "prs", "kurdish": "ku", "central kurdish": "ckb", "yoruba": "yo",
	"igbo": "ig", "hausa": "ha", "zulu": "zu", "isizulu": "zu", "xhosa": "xh", "isixhosa": "xh",
	"kinyarwanda": "rw", "oromo": "om", "samoan": "sm", "tongan": "to", "hawaiian": "haw",
	"navajo": "nv", "cherokee": "chr", "tagalog": "tl", "cebuano": "ceb", "ilocano": "ilo",
	"latin": "la", "esperanto": "eo", "scottish gaelic": "gd", "gaelic": "gd", "breton": "br",
	"faroese": "fo", "uyghur": "ug", "uighur": "ug", "tibetan": "bo", "tajik": "tg", "turkmen": "tk",
	"kyrgyz": "ky", "tatar": "tt", "bashkir": "ba", "yiddish": "yi", "frisian": "fy",
	"western frisian": "fy", "corsican": "co", "occitan": "oc", "romansh": "rm",
	"northern sami": "se", "sami": "se", "greenlandic": "kl", "inuktitut": "iu", "quechua": "qu",
	"guarani": "gn", "malagasy": "mg", "assamese": "as", "odia": "or", "oriya": "or",
	"sindhi": "sd", "divehi": "dv", "tswana": "tn", "setswana": "tn", "sesotho": "st",
	"southern sotho": "st", "northern sotho": "nso", "sesotho sa leboa": "nso", "wolof": "wo",
	"lingala": "ln", "fulah": "ff", "akan": "ak", "twi": "tw", "bambara": "bm", "ewe": "ee",
	"shona": "sn", "chichewa": "ny", "nyanja": "ny", "kirundi": "rn", "rundi": "rn", "ganda": "lg",
	"luganda": "lg", "marshallese": "mh", "chuukese": "chk", "palauan": "pau", "chamorro": "ch",
	"fijian": "fj", "maori": "mi", "tok pisin": "tpi", "cantonese": "yue", "mandarin": "zh",
	"norwegian": "no", "norwegian (bokmal)": "nb", "norwegian (nynorsk)": "nn", "bokmal": "nb",
	"nynorsk": "nn", "valencian": "ca", "flemish": "nl", "moldavian": "ro", "farsi": "fa",
	"slovene": "sl", "kiswahili": "sw", "bangla": "bn", "panjabi": "pa", "sinhalese": "si",
	"burmese": "my", "khmer": "km", "lao": "lo", "dzongkha": "dz", "tigre": "tig", "amharic": "am",
	"montenegrin": "cnr", "konkani": "kok", "manipuri": "mni", "santali": "sat", "kashmiri": "ks",
	"maithili": "mai", "dogri": "doi", "bodo": "brx", "sanskrit": "sa",
}

// scriptNames are the names of scripts as written in brackets.
var scriptNames = map[string]string{
	"latin": "Latn", "cyrillic": "Cyrl", "arabic": "Arab", "simplified": "Hans", "traditional": "Hant",
	"devanagari": "Deva", "hebrew": "Hebr", "greek": "Grek", "mongolian": "Mong",
}

var (
	nameOnce    sync.Once
	byFullName  map[string]string // "german (germany)" -> "de-DE"
	byBaseName  map[string]string // "german" -> "de"
	byRegionNam map[string]string // "germany" -> "DE"
)

// fold lowercases a name and drops its accents and extra spaces, so
// "Norwegian Bokmal" finds "Norwegian Bokmål".
func fold(s string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(strings.ToLower(s)) {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		b.WriteRune(r)
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func loadNames() {
	byFullName, byBaseName, byRegionNam = map[string]string{}, map[string]string{}, map[string]string{}
	for _, l := range languages {
		byFullName[fold(l.Name)] = l.Tag
		base, rest, _ := strings.Cut(l.Name, " (")
		primary := Primary(l.Tag)
		if _, ok := byBaseName[fold(base)]; !ok {
			byBaseName[fold(base)] = primary
		}
		rest = strings.TrimSuffix(rest, ")")
		parts := strings.Split(rest, ", ")
		if r := regionSubtag(l.Tag); r != "" && len(parts) > 0 {
			byRegionNam[fold(parts[len(parts)-1])] = r
		}
	}
	for k, v := range moreLanguages {
		if _, ok := byBaseName[k]; !ok {
			byBaseName[k] = v
		}
	}
}

// ByName returns the tag of a language named in English, as CAT tools
// write language names in bilingual files: "German", "German (Germany)",
// "Serbian (Latin, Serbia)", "Chinese (Simplified)". The country and the
// script in brackets are used where they are known.
func ByName(name string) (string, bool) {
	nameOnce.Do(loadNames)
	f := fold(name)
	if f == "" {
		return "", false
	}
	if t, ok := byFullName[f]; ok {
		return t, true
	}
	if t, ok := byBaseName[f]; ok {
		return t, true
	}
	base, rest, hasRest := strings.Cut(f, " (")
	t, ok := byBaseName[base]
	if !ok {
		return "", false
	}
	if !hasRest {
		return t, true
	}
	rest = strings.TrimSuffix(rest, ")")
	script, region := "", ""
	for _, p := range strings.Split(rest, ",") {
		p = strings.TrimSpace(p)
		if s, ok := scriptNames[p]; ok {
			script = s
			continue
		}
		if r, ok := byRegionNam[p]; ok {
			region = r
			continue
		}
		// "Norwegian (Bokmal)" and the like name the language itself.
		if v, ok := byBaseName[base+" "+p]; ok {
			t = v
		} else if v, ok := moreLanguages[base+" ("+p+")"]; ok {
			t = v
		}
	}
	tag := t
	if script != "" {
		tag += "-" + script
	}
	if region != "" {
		tag += "-" + region
	}
	if n, err := Normalize(tag); err == nil {
		return n, true
	}
	return t, true
}
