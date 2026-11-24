package fuzzy

import "testing"

// A pair of segments and the range its score must fall in.
type pair struct {
	name string
	a, b string
	min  int
	max  int
}

// What a translator expects of a match percentage, language by language:
// a change of ending, spelling, digits or punctuation leaves a close
// match, and another word does not.
func TestLanguages(t *testing.T) {
	for _, p := range []pair{
		// Inflection: one ending changed in a short sentence.
		{"German genitive", "Die Pflichten des Mieters", "Die Pflichten des Mieter", 85, 99},
		{"German other word", "Die Pflichten des Mieters", "Die Pflichten des Hauses", 70, 80},
		{"Finnish case", "Vuokralainen maksaa vuokran talossa", "Vuokralainen maksaa vuokran talosta", 85, 99},
		{"Turkish suffix", "Kiracı evlerinizde oturuyor", "Kiracı evlerinizden oturuyor", 80, 99},
		{"Hungarian case", "A bérlő a házban lakik", "A bérlő a házból lakik", 80, 99},
		{"Russian case", "Арендатор платит арендную плату", "Арендатор платит арендной платы", 60, 90},
		{"Polish case", "Najemca płaci czynsz właścicielowi", "Najemca płaci czynsz właściciela", 80, 99},
		{"Korean particle", "임차인은 차임을 지급한다", "임차인이 차임을 지급한다", 78, 99},
		{"Korean other word", "임차인은 차임을 지급한다", "임대인은 건물을 지급한다", 40, 80},
		// One word of a single-word segment in another form is still a match.
		{"single word, other form", "Mieters", "Mieter", 50, 80},
		{"single word, other word", "Mieter", "Garten", 0, 20},

		// Spelling: marks and letter variants.
		{"French accent left out", "Le résumé du contrat", "Le resume du contrat", 90, 99},
		{"Arabic with short vowels", "العَقْدُ مُلْزِمٌ للطرفين", "العقد ملزم للطرفين", 85, 99},
		{"Arabic alef with hamza", "أحكام الإيجار", "احكام الايجار", 70, 99},
		{"Arabic tatweel", "العقـــد ملزم", "العقد ملزم", 85, 99},
		{"Persian and Arabic yeh", "قرارداد اجاره‌ی خانه", "قرارداد اجاره‌ي خانه", 85, 99},
		{"Hebrew with points", "שָׁלוֹם עוֹלָם טוב", "שלום עולם טוב", 80, 99},
		{"Greek accent", "Ο μισθωτής πληρώνει", "Ο μισθωτης πληρώνει", 90, 99},
		{"German sharp s", "Die Straße ist gesperrt", "Die Strasse ist gesperrt", 90, 99},
		{"Turkish dotted capital", "İstanbul büyük bir şehir", "istanbul büyük bir şehir", 90, 99},
		{"Vietnamese tone", "Hợp đồng thuê nhà", "Hợp đồng thuê nhá", 90, 99},
		{"Hindi matra", "किराया हर महीने देय है", "किराये हर महीने देय है", 80, 99},

		// Scripts without spaces: one character of many.
		{"Chinese one character", "承租人应按月支付租金", "承租人应按年支付租金", 85, 95},
		{"Chinese other sentence", "承租人应按月支付租金", "出租人负责维修房屋", 0, 40},
		{"Japanese one character", "賃借人は毎月賃料を支払う", "賃借人は毎年賃料を支払う", 85, 95},
		{"Thai one character", "ผู้เช่าต้องชำระค่าเช่า", "ผู้เช่าต้องชำระค่าเช้า", 85, 99},
		{"Khmer one character", "អ្នកជួលត្រូវបង់ថ្លៃ", "អ្នកជួលត្រូវបង់ថ្ល", 80, 99},
		{"full-width letters", "ＡＢＣ社との契約", "ABC社との契約", 90, 99},

		// Numbers.
		{"another number", "Die Miete beträgt 1.250,00 EUR", "Die Miete beträgt 980,00 EUR", 90, 99},
		{"Arabic-Indic digits", "الإيجار ١٢٥٠ ريال", "الإيجار 1250 ريال", 95, 99},
		{"Devanagari digits", "किराया १२५० रुपये", "किराया 1250 रुपये", 95, 99},
		{"full-width digits", "賃料は１２５０円", "賃料は1250円", 95, 99},
		{"other separators", "Total: 1\u00a0250,00", "Total: 1.250,00", 90, 99},

		// Punctuation in another form.
		{"curly quotes", `Er sagte "ja" dazu`, "Er sagte „ja“ dazu", 93, 99},
		{"dash", "Miete - Nebenkosten", "Miete – Nebenkosten", 93, 99},
		{"Chinese and ASCII comma", "租金，押金", "租金,押金", 93, 99},
		{"Arabic question mark", "هل العقد ملزم؟", "هل العقد ملزم?", 93, 99},
		{"other punctuation", "Miete: Nebenkosten", "Miete! Nebenkosten", 75, 85},
	} {
		got := ScoreText(p.a, p.b)
		if got < p.min || got > p.max {
			t.Errorf("%s: %q against %q scores %d, want %d to %d", p.name, p.a, p.b, got, p.min, p.max)
		}
		if back := ScoreText(p.b, p.a); back != got {
			t.Errorf("%s: %d one way and %d the other", p.name, got, back)
		}
	}
}

// A changed ending must cost less than another word, another word less
// than two, in every language with word forms.
func TestFormsRankBelowWords(t *testing.T) {
	for _, c := range []struct{ base, form, word string }{
		{"Der Mieter zahlt die Miete pünktlich", "Der Mieter zahlt die Mieten pünktlich", "Der Mieter zahlt die Kaution pünktlich"},
		{"Vuokralainen maksaa vuokran ajoissa", "Vuokralainen maksaa vuokrat ajoissa", "Vuokralainen maksaa takuun ajoissa"},
		{"Kiracı kirayı zamanında öder", "Kiracı kiraları zamanında öder", "Kiracı depozitoyu zamanında öder"},
		{"Арендатор вовремя платит аренду", "Арендатор вовремя платит аренды", "Арендатор вовремя платит залог"},
		{"المستأجر يدفع الإيجار في موعده", "المستأجر يدفع الإيجارات في موعده", "المستأجر يدفع التأمين في موعده"},
		{"임차인은 차임을 제때 지급한다", "임차인은 차임도 제때 지급한다", "임차인은 보증금을 제때 지급한다"},
	} {
		same, form, word := ScoreText(c.base, c.base), ScoreText(c.base, c.form), ScoreText(c.base, c.word)
		if !(same == 100 && form < 100 && form > word) {
			t.Errorf("%q: same %d, other form %d, other word %d", c.base, same, form, word)
		}
	}
}

func TestNumberTokens(t *testing.T) {
	for text, want := range map[string]int{
		"1.250,00":       1,
		"1\u00a0250,00":  1, // no-break space
		"1'250.50":       1,
		"١٬٢٥٠٫٥٠":       1,
		"๑๒๓":            1,
		"3 Monate":       2,
		"12. Januar":     3,
		"Art. 5 Abs. 2.": 7,
	} {
		if got := len(TokenizeText(text)); got != want {
			t.Errorf("%q: %d tokens, want %d: %+v", text, got, want, TokenizeText(text))
		}
	}
}
