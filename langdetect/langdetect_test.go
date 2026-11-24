package langdetect

import "testing"

func TestDetect(t *testing.T) {
	for _, c := range []struct {
		text, lang string
		reason     Reason
	}{
		{"Der Mieter zahlt die Miete bis zum dritten Werktag eines jeden Monats auf das Konto des Vermieters. Die Kaution beträgt drei Monatsmieten und wird bei Beginn des Mietverhältnisses fällig.", "de-DE", OK},
		{"The tenant shall pay the rent by the third working day of each month into the landlord's account. The deposit is three months' rent and is due when the tenancy begins.", "en-GB", OK},
		{"Le locataire paie le loyer au plus tard le troisième jour ouvrable de chaque mois sur le compte du bailleur. Le dépôt de garantie est de trois mois de loyer.", "fr-FR", OK},
		{"임차인은 매월 셋째 영업일까지 임대인의 계좌로 월세를 지급한다.", "ko-KR", OK},
		{"借主は毎月第三営業日までに家賃を貸主の口座に支払うものとする。", "ja-JP", OK},
		{"承租人应在每月第三个工作日前将租金支付至出租人账户。", "zh-CN", OK},
		{"Mietvertrag Hofmann", "", Short},
		{"   ", "", Empty},
	} {
		d := Detect(c.text)
		if d.Lang != c.lang || d.Reason != c.reason {
			t.Errorf("%.30q: %+v, want %s %s", c.text, d, c.lang, c.reason)
		}
	}
	// A page of running text is confident; a couple of sentences may ask
	// for confirmation.
	long := "Der Mieter zahlt die Miete bis zum dritten Werktag eines jeden Monats auf das Konto des Vermieters. " +
		"Die Kaution beträgt drei Monatsmieten und wird bei Beginn des Mietverhältnisses fällig. " +
		"Der Vermieter hat die Kaution von seinem Vermögen getrennt anzulegen, und die Zinsen stehen dem Mieter zu. " +
		"Schäden in der Wohnung sind dem Vermieter unverzüglich anzuzeigen, auch wenn sie nicht vom Mieter verursacht wurden."
	if d := Detect(long); d.Lang != "de-DE" || d.Confidence < Confident || len(d.Alternatives) == 0 {
		t.Errorf("long German text: %+v", d)
	}
	// Text with no function words of any language gives no guess.
	if d := Detect("Xylo qwerty zzzap plonk frobnicate grault garply waldo fred plugh"); d.Lang != "" || d.Reason != Mixed {
		t.Errorf("gibberish: %+v", d)
	}
	if len(Languages()) < 60 {
		t.Errorf("languages %v", Languages())
	}
}
