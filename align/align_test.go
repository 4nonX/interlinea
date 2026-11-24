package align

import (
	"fmt"
	"strings"
	"testing"

	"github.com/4nonX/interlinea/content"
)

func segs(block string, texts ...string) []Segment {
	var out []Segment
	for _, t := range texts {
		out = append(out, Segment{Block: block, Content: content.Text(t)})
	}
	return out
}

func show(ps []Pair) string {
	var b strings.Builder
	for _, p := range ps {
		fmt.Fprintf(&b, "%v=%v ", p.SourceSegments, p.TargetSegments)
	}
	return strings.TrimSpace(b.String())
}

var german = []string{
	"Der Mieter zahlt die Miete bis zum dritten Werktag eines jeden Monats.",
	"Die Kaution beträgt 2.400,00 Euro.",
	"Sie wird bei Beginn des Mietverhältnisses fällig und vom Vermieter getrennt von seinem Vermögen angelegt.",
	"Die Betriebskosten werden jährlich abgerechnet.",
	"Das Mietverhältnis beginnt am 1. März 2026 und läuft auf unbestimmte Zeit.",
	"Die Kündigung bedarf der Schriftform.",
}

var english = []string{
	"The tenant pays the rent by the third working day of each month.",
	"The deposit is EUR 2,400.00.",
	"It falls due at the start of the tenancy and is invested by the landlord separately from his assets.",
	"Service charges are settled once a year.",
	"The tenancy begins on 1 March 2026 and runs for an indefinite period.",
	"Notice must be given in writing.",
}

func TestOneToOne(t *testing.T) {
	ps := Align(segs("", german...), segs("", english...))
	if got := show(ps); got != "[0]=[0] [1]=[1] [2]=[2] [3]=[3] [4]=[4] [5]=[5]" {
		t.Fatal(got)
	}
	for i, p := range ps {
		if p.Score < 60 {
			t.Errorf("pair %d scores %d", i, p.Score)
		}
	}
	// The same numbers in other separators lift a pair above a plain one.
	if ps[1].Score <= ps[3].Score {
		t.Errorf("numbers: %d, plain: %d", ps[1].Score, ps[3].Score)
	}
}

func TestSplitAndJoined(t *testing.T) {
	// The translator made two sentences of the third and one of the last two.
	en := []string{
		english[0], english[1],
		"It falls due at the start of the tenancy.", "The landlord invests it separately from his assets.",
		english[3],
		"The tenancy begins on 1 March 2026 and runs for an indefinite period; notice must be given in writing.",
	}
	ps := Align(segs("", german...), segs("", en...))
	if got := show(ps); got != "[0]=[0] [1]=[1] [2]=[2 3] [3]=[4] [4 5]=[5]" {
		t.Fatal(got)
	}
	if got := ps[2].Target.Plain(); got != "It falls due at the start of the tenancy. The landlord invests it separately from his assets." {
		t.Errorf("joined: %q", got)
	}
}

func TestLeftOut(t *testing.T) {
	// A sentence the translation does not have is left alone, and the
	// rest still pairs.
	var de, full []string
	for i := 0; i < 3; i++ {
		de = append(de, german...)
		full = append(full, english...)
	}
	en := append(append([]string{}, full[:10]...), full[11:]...)
	ps := Align(segs("", de...), segs("", en...))
	if len(ps) != 18 || show(ps[9:12]) != "[9]=[9] [10]=[] [11]=[10]" {
		t.Fatal(show(ps))
	}
	if ps[10].Score != 0 || len(ps[10].Target) != 0 {
		t.Errorf("the pair left out: %+v", ps[10])
	}
	// And one only the translation has.
	ps = Align(segs("", en...), segs("", full...))
	if len(ps) != 18 || show(ps[9:12]) != "[9]=[9] []=[10] [10]=[11]" {
		t.Fatal(show(ps))
	}
}

func TestParagraphs(t *testing.T) {
	// With the same paragraphs on both sides a sentence stays in its own,
	// even where the lengths would pair it across.
	src := append(segs("p1", "Kurz.", "Ein deutlich längerer Satz, der im ersten Absatz steht."), segs("p2", "Noch einer.")...)
	tgt := append(segs("a", "Short, and a clearly longer sentence that is in the first paragraph."), segs("b", "Another one.", "And one more that the source does not have at all.")...)
	ps := Align(src, tgt)
	if got := show(ps); got != "[0 1]=[0] [2]=[1] []=[2]" {
		t.Fatal(got)
	}
}

func TestOtherScripts(t *testing.T) {
	// A language written much shorter than the other: the ratio of the
	// whole documents is what a pair is measured against.
	ko := []string{
		"임차인은 매월 셋째 영업일까지 임대료를 지급한다.",
		"보증금은 2,400유로이다.",
		"보증금은 임대차 개시 시에 지급하여야 하며 임대인은 이를 자신의 재산과 분리하여 예치한다.",
		"관리비는 매년 정산한다.",
		"임대차는 2026년 3월 1일에 개시하며 기간의 정함이 없다.",
		"해지는 서면으로 하여야 한다.",
	}
	ps := Align(segs("", german...), segs("", ko...))
	if got := show(ps); got != "[0]=[0] [1]=[1] [2]=[2] [3]=[3] [4]=[4] [5]=[5]" {
		t.Fatal(got)
	}
	// Arabic-Indic digits are the same numbers.
	if !sameSet(numbers("٢٬٤٠٠٫٠٠ يورو في 1 مارس"), numbers("EUR 2,400.00 on 1 March")) {
		t.Errorf("%v / %v", numbers("٢٬٤٠٠٫٠٠ يورو في 1 مارس"), numbers("EUR 2,400.00 on 1 March"))
	}
	// Segments of a language without spaces join without one.
	c, _ := joined(segs("", "第一句。", "第二句。"), 0, 2)
	if c.Plain() != "第一句。第二句。" {
		t.Errorf("joined %q", c.Plain())
	}
}

func TestEmptyAndLong(t *testing.T) {
	if ps := Align(nil, nil); len(ps) != 0 {
		t.Fatal(ps)
	}
	if got := show(Align(segs("", "Nur hier."), nil)); got != "[0]=[]" {
		t.Fatal(got)
	}
	// Long documents stay in reach, and every segment is in one pair.
	var s, e []string
	for i := 0; i < 2500; i++ {
		s = append(s, german[i%len(german)]+fmt.Sprint(" Nr. ", i))
		e = append(e, english[i%len(english)]+fmt.Sprint(" No. ", i))
	}
	e = append(e[:1200], e[1201:]...)
	ps := Align(segs("", s...), segs("", e...))
	ns, nt, good := 0, 0, 0
	for _, p := range ps {
		ns += len(p.SourceSegments)
		nt += len(p.TargetSegments)
		if len(p.SourceSegments) == 1 && len(p.TargetSegments) == 1 && strings.HasSuffix(p.Target.Plain(), strings.TrimPrefix(p.Source.Plain()[strings.LastIndex(p.Source.Plain(), " Nr. "):], " Nr.")) {
			good++
		}
	}
	if ns != 2500 || nt != 2499 || good < 2490 {
		t.Fatalf("%d source and %d target segments in pairs, %d right", ns, nt, good)
	}
}
