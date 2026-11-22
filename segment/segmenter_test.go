package segment

import (
	"reflect"
	"strings"
	"testing"

	"github.com/4nonX/interlinea/content"
)

func seg(t *testing.T, lang, text string) []string {
	t.Helper()
	return New(nil, lang).Strings(text)
}

func TestGerman(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{
			"Der Vertrag beginnt am 1. Januar 2027. Er endet automatisch.",
			[]string{"Der Vertrag beginnt am 1. Januar 2027.", "Er endet automatisch."},
		},
		{
			"Es gelten z.B. die AGB. Siehe auch § 5 Abs. 2 BGB. Fertig!",
			[]string{"Es gelten z.B. die AGB.", "Siehe auch § 5 Abs. 2 BGB.", "Fertig!"},
		},
		{
			"Herr Dr. Müller unterschreibt. Frau Prof. Weber prüft. Ist das klar? Ja.",
			[]string{"Herr Dr. Müller unterschreibt.", "Frau Prof. Weber prüft.", "Ist das klar?", "Ja."},
		},
		{
			"„Wir kommen.“ Dann ging er.",
			[]string{"„Wir kommen.“", "Dann ging er."},
		},
		{
			"Der Preis beträgt 3.50 EUR. Danke.",
			[]string{"Der Preis beträgt 3.50 EUR.", "Danke."},
		},
	}
	for _, c := range cases {
		if got := seg(t, "de-DE", c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%q\n got  %q\n want %q", c.in, got, c.want)
		}
	}
}

func TestEnglish(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{
			"Mr. Smith signed the lease. The tenant, i.e. Ms. Jones, agreed.",
			[]string{"Mr. Smith signed the lease.", "The tenant, i.e. Ms. Jones, agreed."},
		},
		{
			"See J. R. R. Tolkien. It works! Does it? Yes...",
			[]string{"See J. R. R. Tolkien.", "It works!", "Does it?", "Yes..."},
		},
		{
			"He waited... and then left. Done.",
			[]string{"He waited... and then left.", "Done."},
		},
	}
	for _, c := range cases {
		if got := seg(t, "en-GB", c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%q\n got  %q\n want %q", c.in, got, c.want)
		}
	}
}

func TestCJKAndKorean(t *testing.T) {
	got := seg(t, "ja-JP", "今日は晴れです。明日は雨です！本当？")
	want := []string{"今日は晴れです。", "明日は雨です！", "本当？"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ja got %q", got)
	}
	got = seg(t, "ko-KR", "계약은 1월에 시작합니다. 자동으로 종료됩니다.")
	want = []string{"계약은 1월에 시작합니다.", "자동으로 종료됩니다."}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ko got %q", got)
	}
}

func TestLineBreakAndWhitespace(t *testing.T) {
	got := seg(t, "fr-FR", "  Première ligne\nDeuxième ligne.  ")
	want := []string{"Première ligne", "Deuxième ligne."}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q", got)
	}
	if out := seg(t, "de", ""); len(out) != 0 {
		t.Errorf("empty input gave %q", out)
	}
	if out := seg(t, "de", "   "); len(out) != 0 {
		t.Errorf("blank input gave %q", out)
	}
}

func TestSpansCoverText(t *testing.T) {
	text := "Eins. Zwei! Drei? Vier."
	spans := New(nil, "de").Split(text)
	if len(spans) != 4 {
		t.Fatalf("spans = %v", spans)
	}
	prev := 0
	for _, sp := range spans {
		if sp.Start < prev || sp.End <= sp.Start {
			t.Fatalf("bad span order %v", spans)
		}
		if strings.TrimSpace(text[prev:sp.Start]) != "" {
			t.Fatalf("non-space gap before %v", sp)
		}
		prev = sp.End
	}
}

func TestSplitContentKeepsTags(t *testing.T) {
	var c content.Content
	c = c.Append("Erster Satz. ")
	c = c.AppendTag(content.Tag{Kind: content.TagOpen, ID: "1", Type: "bold"})
	c = c.Append("Zweiter")
	c = c.AppendTag(content.Tag{Kind: content.TagClose, ID: "1", Type: "bold"})
	c = c.Append(" Satz.")
	parts := New(nil, "de").SplitContent(c)
	if len(parts) != 2 {
		t.Fatalf("parts = %v", parts)
	}
	if parts[0].String() != "Erster Satz." || parts[1].String() != "{1}Zweiter{/1} Satz." {
		t.Fatalf("got %q / %q", parts[0].String(), parts[1].String())
	}
}

func TestParseCustomAndCascade(t *testing.T) {
	doc := `<srx version="2.0"><header cascade="no"/><body>
<languagerules>
 <languagerule languagerulename="A"><rule break="yes"><beforebreak>;</beforebreak><afterbreak>\s</afterbreak></rule></languagerule>
 <languagerule languagerulename="B"><rule break="yes"><beforebreak>\.</beforebreak><afterbreak>\s</afterbreak></rule></languagerule>
</languagerules>
<maprules>
 <languagemap languagepattern="xx.*" languagerulename="A"/>
 <languagemap languagepattern=".*" languagerulename="B"/>
</maprules></body></srx>`
	rs, err := Parse(strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	got := New(rs, "XX-yy").Strings("a; b. c")
	if !reflect.DeepEqual(got, []string{"a;", "b. c"}) {
		t.Fatalf("no-cascade got %q", got)
	}
	rs.Cascade = true
	got = New(rs, "xx").Strings("a; b. c")
	if !reflect.DeepEqual(got, []string{"a;", "b.", "c"}) {
		t.Fatalf("cascade got %q", got)
	}
}

func TestParseErrors(t *testing.T) {
	bad := []string{
		`<srx version="1.0"><body/></srx>`,
		`<srx version="2.0"><body><languagerules><languagerule languagerulename="A"><rule><beforebreak>(?=x)</beforebreak></rule></languagerule></languagerules></body></srx>`,
		`<srx version="2.0"><body><languagerules><languagerule languagerulename="A"><rule/></languagerule></languagerules></body></srx>`,
		`<srx version="2.0"><body><maprules><languagemap languagepattern=".*" languagerulename="Missing"/></maprules></body></srx>`,
		`not xml`,
	}
	for i, d := range bad {
		if _, err := Parse(strings.NewReader(d)); err == nil {
			t.Errorf("case %d: expected error", i)
		}
	}
	if _, err := NewRule(true, "", ""); err == nil {
		t.Error("NewRule without patterns must fail")
	}
}

func TestFromRulesAfterOnly(t *testing.T) {
	r, err := NewRule(true, "", `\|`)
	if err != nil {
		t.Fatal(err)
	}
	got := FromRules([]Rule{r}).Strings("a|b|c")
	if !reflect.DeepEqual(got, []string{"a", "|b", "|c"}) {
		t.Fatalf("got %q", got)
	}
}

func BenchmarkGermanParagraph(b *testing.B) {
	para := strings.Repeat("Der Mieter haftet z.B. für Schäden gem. § 5 Abs. 2 des Vertrags vom 1. Januar 2027. Ist das klar? ", 50)
	s := New(nil, "de-DE")
	b.SetBytes(int64(len(para)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Breaks(para)
	}
}

func TestReverseString(t *testing.T) {
	cases := map[string]string{"": "", "abc": "cba", "äöü": "üöä", "a\xffb": "b\xffa", "日本": "本日"}
	for in, want := range cases {
		if got := reverseString(in); got != want {
			t.Errorf("reverseString(%q) = %q, want %q", in, got, want)
		}
	}
}
