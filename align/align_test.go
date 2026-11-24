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

func TestRealign(t *testing.T) {
	// The translation has a long passage the document lacks, and after it
	// the lengths alone pair everything one off. One pair set right by
	// hand sets right what follows it.
	var de, en []string
	for i := 0; i < 4; i++ {
		de = append(de, german...)
		en = append(en, english...)
	}
	extra := []string{"This clause was added later.", "It has no counterpart.", "Nor has this one."}
	en = append(append(append([]string{}, en[:7]...), extra...), en[7:]...)
	src, tgt := segs("", de...), segs("", en...)
	// German 9 is English 12; say so, and say that German 7 is English 10.
	ps := Realign(src, tgt, []Fixed{{Source: []int{9}, Target: []int{12}}, {Source: []int{7}, Target: []int{10}}})
	ns, nt, fixed := 0, 0, 0
	for _, p := range ps {
		ns += len(p.SourceSegments)
		nt += len(p.TargetSegments)
		if p.Fixed {
			fixed++
			if p.Score != 100 || len(p.SourceSegments) != 1 || p.TargetSegments[0] != p.SourceSegments[0]+3 {
				t.Errorf("fixed pair %+v", p)
			}
			continue
		}
		// After the fixed pairs every sentence is three on; which of the
		// sentences next to the passage it belongs to, lengths cannot tell.
		if len(p.SourceSegments) == 1 && len(p.TargetSegments) == 1 && (p.SourceSegments[0] < 6 || p.SourceSegments[0] > 7) {
			want := p.SourceSegments[0]
			if want > 7 {
				want += 3
			}
			if p.TargetSegments[0] != want {
				t.Errorf("%v=%v", p.SourceSegments, p.TargetSegments)
			}
		}
	}
	if ns != len(de) || nt != len(en) || fixed != 2 {
		t.Fatalf("%d and %d segments in pairs, %d fixed: %s", ns, nt, fixed, show(ps))
	}
	// A fixed pair of several segments that do not follow each other, and
	// one that crosses another: both stay, and every segment is in one pair.
	ps = Realign(segs("", german...), segs("", english...), []Fixed{{Source: []int{0, 2}, Target: []int{1}}, {Source: []int{4}, Target: []int{0}}})
	seenS, seenT := map[int]int{}, map[int]int{}
	for _, p := range ps {
		for _, i := range p.SourceSegments {
			seenS[i]++
		}
		for _, i := range p.TargetSegments {
			seenT[i]++
		}
	}
	if len(seenS) != 6 || len(seenT) != 6 {
		t.Fatalf("segments in pairs: %v %v: %s", seenS, seenT, show(ps))
	}
	for _, n := range seenS {
		if n != 1 {
			t.Fatalf("a segment twice: %s", show(ps))
		}
	}
	if !strings.Contains(show(ps), "[0 2]=[1]") || !strings.Contains(show(ps), "[4]=[0]") {
		t.Errorf("the fixed pairs: %s", show(ps))
	}
	if got := ps[0].Source.Plain(); !ps[0].Fixed || got != german[0]+" "+german[2] {
		t.Errorf("first pair %q", got)
	}
	// Nothing fixed is the plain alignment.
	if show(Realign(segs("", german...), segs("", english...), nil)) != show(Align(segs("", german...), segs("", english...))) {
		t.Error("nothing fixed differs from Align")
	}
	for _, bad := range [][]Fixed{{{Source: []int{0}}}, {{Source: []int{6}, Target: []int{0}}}, {{Source: []int{0}, Target: []int{1}}, {Source: []int{1}, Target: []int{1}}}} {
		if CheckFixed(bad, 6, 6) == "" {
			t.Errorf("%+v passes", bad)
		}
	}
	if msg := CheckFixed([]Fixed{{Source: []int{0, 2}, Target: []int{1}}}, 6, 6); msg != "" {
		t.Error(msg)
	}
}
