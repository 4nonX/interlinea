package qa

import (
	"testing"

	"github.com/4nonX/interlinea/content"
)

func TestNumbersInEveryScript(t *testing.T) {
	for _, c := range []struct{ text, lang, value, written string }{
		{"12,50,000.5", "hi-IN", "1250000.5", "12,50,000.5"},
		{"१२,५०,०००.५", "hi-IN", "1250000.5", "१२,५०,०००.५"},
		{"12,50,000", "en-IN", "1250000", "12,50,000"},
		{"١٬٢٥٠٫٥", "ar-EG", "1250.5", "١٬٢٥٠٫٥"},
		{"۱٬۲۵۰٫۵", "fa-IR", "1250.5", "۱٬۲۵۰٫۵"},
		{"๑,๒๕๐.๕", "th-TH", "1250.5", "๑,๒๕๐.๕"},
		{"1,250.5", "es-MX", "1250.5", "1,250.5"},
		{"1.250,5", "es-ES", "1250.5", "1.250,5"},
		{"1’250.5", "de-CH", "1250.5", "1’250.5"},
		{"1.250,5", "km-KH", "1250.5", "1.250,5"},
		{"1,250.5", "mn-MN", "1250.5", "1,250.5"},
	} {
		got := numbersIn("x "+c.text+" y", c.lang)
		if len(got) != 1 || got[0].value != c.value || got[0].text != c.written {
			t.Errorf("%s in %s: %+v, want %s written %s", c.text, c.lang, got, c.value, c.written)
		}
	}
}

func TestFrenchSpacingByRegion(t *testing.T) {
	kinds := func(lang, tgt string) []Kind {
		issues, err := Check([]Segment{{ID: "1", Source: content.Text("Hello! Why: this."), Target: content.Text(tgt)}}, Options{SourceLang: "en-GB", TargetLang: lang})
		if err != nil {
			t.Fatal(err)
		}
		var out []Kind
		for _, i := range issues {
			if i.Kind == SpaceBeforePunct {
				out = append(out, i.Kind)
			}
		}
		return out
	}
	// France sets a space before ! and : ; Canada only before the colon.
	if got := kinds("fr-FR", "Bonjour! Pourquoi: ceci."); len(got) != 2 {
		t.Errorf("fr-FR: %d spacing issues", len(got))
	}
	if got := kinds("fr-CA", "Bonjour! Pourquoi: ceci."); len(got) != 1 {
		t.Errorf("fr-CA: %d spacing issues", len(got))
	}
	if got := kinds("fr-CA", "Bonjour! Pourquoi : ceci."); len(got) != 0 {
		t.Errorf("fr-CA, spaced as it should be: %d spacing issues", len(got))
	}
	if got := kinds("fr-CA", "Bonjour ! Pourquoi : ceci."); len(got) != 1 {
		t.Errorf("fr-CA with a space before the exclamation mark: %d spacing issues", len(got))
	}
}

func TestSentenceEndsAcrossScripts(t *testing.T) {
	for _, c := range []struct{ src, tgt, lang string }{
		{"It begins today.", "今天开始。", "zh-CN"},
		{"It begins today.", "आज शुरू होता है।", "hi-IN"},
		{"It begins today.", "Այսօր է սկսվում։", "hy-AM"},
		{"It begins today.", "ዛሬ ይጀምራል።", "am-ET"},
		{"It begins today.", "ថ្ងៃនេះចាប់ផ្តើម។", "km-KH"},
		{"It begins today.", "เริ่มวันนี้", "th-TH"},
		{"Do you agree?", "Συμφωνείτε;", "el-GR"},
		{"Do you agree?", "هل توافق؟", "ar-SA"},
	} {
		issues, err := Check([]Segment{{ID: "1", Source: content.Text(c.src), Target: content.Text(c.tgt)}}, Options{SourceLang: "en-GB", TargetLang: c.lang})
		if err != nil {
			t.Fatal(err)
		}
		for _, i := range issues {
			if i.Kind == EndPunctuation {
				t.Errorf("%s: %s", c.lang, i.Message)
			}
		}
	}
	// A question answered with a statement is still noticed.
	issues, _ := Check([]Segment{{ID: "1", Source: content.Text("Do you agree?"), Target: content.Text("Συμφωνείτε.")}}, Options{SourceLang: "en-GB", TargetLang: "el-GR"})
	found := false
	for _, i := range issues {
		found = found || i.Kind == EndPunctuation
	}
	if !found {
		t.Error("a full stop for a question mark passes in Greek")
	}
}
