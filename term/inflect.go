package term

import (
	"strings"
	"unicode/utf8"

	"github.com/4nonX/interlinea/internal/textnorm"
)

// inflection says how the words of a language change in running text, as
// far as finding a term needs it. Like the endings above, the rules name
// what may follow or replace the end of the term as written; they do not
// analyse the language, so irregular forms are not found.
type inflection struct {
	// prefixes may stand before the word and are written together with it:
	// the Arabic article and one-letter prepositions, the Hebrew ones.
	prefixes []string
	// endings may follow the word's stem.
	endings map[string]bool
	// drop is how many of the term's last letters an ending may replace
	// (Russian договор-а but also книг-а, книг-и), and tails says which
	// letters those may be; nil allows any.
	drop  int
	tails map[string]bool
	// open takes any ending of up to this many letters, for languages that
	// string suffixes together (Turkish, Finnish, Hungarian): sözleşme,
	// sözleşmelerinde.
	open int
	// min is the shortest term the rules apply to; shorter ones match only
	// as written, since a short stem fits too many other words.
	min int
	// every applies the rules to each word of a term, not only the last:
	// in трудовой договор both words change.
	every bool
}

func set(words string) map[string]bool {
	out := map[string]bool{}
	for _, w := range strings.Fields(words) {
		out[w] = true
	}
	return out
}

var (
	slavicCyrillic = inflection{
		endings: set("а я у ю е и ы о ь й ой ей ёй ом ем ём ам ям ах ях ов ев ёв ей ами ями ого его ому ему ым им ых их ую юю ая яя ое ее ие ые ою ею " +
			"і ї є ів їв ові еві єві ою ею єю ам ям ах ях ами ями ій ої ього ьому им ім их іх " +
			"ът ят та то те ите ове ата ия ии ият ията ито ото " +
			"от та то те ите ови ата ов ови " +
			"ы ў аў яў оў ёй ай яй ой ою аю яю ом ам ым ых ымі амі ямі"),
		drop: 2, tails: set("а я о е ь й ы и і ї у ю ий ый ой ая яя ое ее ие ые ия ие ій а ть ти"), min: 4, every: true,
	}
	slavicLatin = inflection{
		endings: set("a u e y i o ě ą ę ů ou em om ům am ami emi ach ech ích ých ách ovi ové ov ev ów ie iem iami iach ia iu " +
			"ého ému ým ých ými ej ou ú ému ím á é í ý ém ou ho mu " +
			"ama ima ih oj om eg em im og omu ome ega emu ih imi ah ev ov ih ju jo je ja ji jem jih jim jami " +
			"owi owie ego emu ymi imi ą ę ym im ych ich"),
		drop: 2, tails: set("a o e y i ě í ý á é ia ie ek ec ý ij y"), min: 4, every: true,
	}
	baltic = inflection{
		endings: set("as o ui ą u e ai ų ams us ais uose is io iui į iu yje iai ių iams ius iais a os oje oms omis ose ė ės ei ę ėje ėms es ėmis ys ies imi yse ims " +
			"s am ā i iem os im ī ji ju jiem jus jos ēs ē ai ām ās es ei ēm is ij īm īs u us iem um ū"),
		drop: 2, tails: set("as is us ys a ė e s š i o is"), min: 4, every: true,
	}
	greek = inflection{
		endings: set("ος ου ο ε οι ων ους α ας ες η ης ι ια ιου ιων μα ματος ματα ματων ση σης σεις σεων εις εων υ ες αδες ων οσ ασ εσ ησ ουσ σησ εισ ματοσ"),
		drop:    2, tails: set("ος ας ης ες η α ο ι ε οσ ασ ησ εσ ση"), min: 4, every: true,
	}
	romanian = inflection{
		endings: set("ul ului a ei le lor i ii ilor e uri urile urilor ele elor ile ă ea ua lui"),
		drop:    1, tails: set("ă a e u i"), min: 4, every: true,
	}
	scandinavian = inflection{
		endings: set("en et n t ar er or na a e s ens ets erna arna orna ene ne r ns ts ernas arnas ornas enes es ane ar i"),
		drop:    1, tails: set("e a"), min: 4,
	}
	icelandic = inflection{
		endings: set("ar ir ur um s i a u inn ið in ins ina inum inu num na nna"),
		drop:    2, tails: set("ur i a r"), min: 4, every: true,
	}
	agglutinative = inflection{open: 10, drop: 1, min: 4}
	indic         = inflection{
		endings: set("े ों ें ाँ ियाँ ियों एँ ओं यों ी ा ने ला ची चा चे त स ना ाचा ाची ाचे ाला ात ाने को ले लाई मा का की हरू"),
		drop:    1, tails: set("ा ी े"), min: 3, every: true,
	}
	dravidian = inflection{open: 10, drop: 2, min: 3}
	albanian  = inflection{
		endings: set("a i u ja t të ve s n in un ës e et it ave së"),
		drop:    1, tails: set("ë e"), min: 4, every: true,
	}
	amharic = inflection{open: 6, min: 2}
	arabic    = inflection{
		prefixes: []string{"وبال", "فبال", "ولل", "فلل", "وال", "فال", "بال", "كال", "لل", "ال", "و", "ف", "ب", "ك", "ل"},
		endings:  set("ة ات ون ين ان ه ها هم هن نا ي ك كم ية تين تان"),
		drop:     1, tails: set("ة ه"), min: 3, every: true,
	}
	hebrew = inflection{
		prefixes: []string{"וכש", "ושה", "שב", "שה", "של", "וה", "וב", "ול", "ומ", "בה", "לה", "מה", "כש", "ה", "ו", "ב", "כ", "ל", "מ", "ש"},
		endings:  set("ים ות י ה ו נו ם ן ך כם יו יה יהם"),
		drop:     1, tails: set("ה ת"), min: 3, every: true,
	}
	persian = inflection{
		prefixes: []string{},
		endings:  set("ها های هایی ی ای ات ان ام ات اش مان تان شان ٔ"),
		min:      3, every: true,
	}
	urdu  = inflection{endings: set("وں یں ے ی ات یاں یوں"), drop: 1, tails: set("ہ ا ی"), min: 3, every: true}
	malay = inflection{endings: set("nya lah kah ku mu pun"), min: 4}
)

var inflections = map[string]inflection{
	"ru": slavicCyrillic, "uk": slavicCyrillic, "be": slavicCyrillic, "bg": slavicCyrillic, "mk": slavicCyrillic,
	"cs": slavicLatin, "sk": slavicLatin, "sl": slavicLatin, "hr": slavicLatin, "bs": slavicLatin,
	"lt": baltic, "lv": baltic,
	"el": greek,
	"ro": romanian,
	"sv": scandinavian, "da": scandinavian, "nb": scandinavian, "nn": scandinavian, "no": scandinavian,
	"is": icelandic,
	"tr": agglutinative, "az": agglutinative, "fi": agglutinative, "et": agglutinative, "hu": agglutinative,
	"kk": agglutinative, "uz": agglutinative, "mn": agglutinative, "ka": agglutinative, "hy": agglutinative,
	"eu": agglutinative,
	"hi": indic, "mr": indic, "ne": indic,
	"ta": dravidian, "te": dravidian, "kn": dravidian, "ml": dravidian, "bn": dravidian, "si": dravidian,
	"gu": dravidian, "pa": dravidian,
	"sq": albanian, "am": amharic, "pl": slavicLatin,
	"ar": arabic, "he": hebrew, "fa": persian, "ur": urdu,
	"id": malay, "ms": malay,
}

// rules returns the inflection of the matcher's language. Serbian is
// written in both alphabets; the text's own letters decide.
func (m matcher) rules(word string) (inflection, bool) {
	if m.lang == "sr" {
		for _, r := range word {
			if r >= 0x0400 && r <= 0x04FF {
				return slavicCyrillic, true
			}
		}
		return slavicLatin, true
	}
	r, ok := inflections[m.lang]
	return r, ok
}

// inflected reports whether text is a form of term under the language's
// rules. Both are folded.
func (m matcher) inflected(term, text string, last bool) bool {
	r, ok := m.rules(term)
	if !ok || (!last && !r.every) {
		return false
	}
	forms := []string{text}
	for _, p := range r.prefixes {
		if rest, ok := strings.CutPrefix(text, p); ok && utf8.RuneCountInString(rest) >= 2 {
			forms = append(forms, rest)
		}
	}
	n := utf8.RuneCountInString(term)
	for _, f := range forms {
		if f == term {
			return true
		}
		if n < r.min {
			continue
		}
		stem := term
		for d := 0; d <= r.drop; d++ {
			if d > 0 {
				_, size := utf8.DecodeLastRuneInString(stem)
				stem = stem[:len(stem)-size]
				if utf8.RuneCountInString(stem) < 3 {
					break
				}
				if r.tails != nil && !r.tails[term[len(stem):]] {
					continue
				}
			}
			rest, ok := strings.CutPrefix(f, stem)
			if !ok {
				continue
			}
			switch {
			case rest == "":
				// The text has the bare stem: книг for книга.
				return d > 0
			case r.endings[rest]:
				return true
			case r.open > 0 && utf8.RuneCountInString(rest) <= r.open:
				return true
			}
		}
	}
	return false
}

// foldFor folds a word for comparing in the matcher's language: lower
// case and composed accents everywhere, and what each writing system
// leaves optional or writes in two ways.
func (m matcher) fold(s string) string {
	switch m.lang {
	case "tr", "az":
		// The dotless ı and the dotted İ are letters of their own.
		s = strings.NewReplacer("I", "ı", "İ", "i").Replace(s)
	}
	s = strings.ReplaceAll(textnorm.Fold(s), "’", "'")
	switch m.lang {
	case "de":
		// Swiss texts write ss for ß.
		s = strings.ReplaceAll(s, "ß", "ss")
	case "el":
		// The accent moves with the ending: σύμβαση, συμβάσεις.
		s = textnorm.StripMarks(s)
	case "ar", "fa", "ur", "ps":
		// Short vowels are optional, and the alif is written with or
		// without its hamza.
		s = textnorm.StripMarks(s)
		s = strings.NewReplacer("أ", "ا", "إ", "ا", "آ", "ا", "ٱ", "ا", "ى", "ي", "ك", "ک", "ي", "ی").Replace(s)
	case "he", "yi":
		// The vowel points are optional.
		s = textnorm.StripMarks(s)
	}
	return s
}
