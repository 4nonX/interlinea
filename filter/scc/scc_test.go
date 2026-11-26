package scc

import (
	"strings"
	"testing"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter"
)

// hexOf writes text as the byte pairs of basic characters.
func hexOf(s string) string {
	var out []string
	b := []byte(s)
	for i := 0; i < len(b); i += 2 {
		hi, lo := b[i], byte(0)
		if i+1 < len(b) {
			lo = b[i+1]
		}
		out = append(out, strings.ToLower(strings.TrimSpace(func() string {
			return string("0123456789abcdef"[parity(hi)>>4]) + string("0123456789abcdef"[parity(hi)&15]) + string("0123456789abcdef"[parity(lo)>>4]) + string("0123456789abcdef"[parity(lo)&15])
		}())))
	}
	return strings.Join(out, " ")
}

// A pop-on caption of two rows (14 and 15), centred, the second with a
// word in italics and an é; erased two seconds later. Then a second
// caption on one row.
var sample = "Scenarist_SCC V1.0\n\n" +
	"00:00:01:00\t9420 9420 94ae 94ae 9454 9454 97a2 97a2 " + hexOf("Good evening,") + " 94f4 94f4 97a1 97a1 " + hexOf("welcome to ") + " 91ae 91ae " + hexOf("the show") + " 9120 9120 " + hexOf("caf") + " 805c 942f 942f\n\n" +
	"00:00:04:00\t942c 942c\n\n" +
	"00:00:05:00\t9420 9420 94ae 94ae 94f4 94f4 " + hexOf("Goodbye.") + " 942f 942f\n\n" +
	"00:00:07:00\t942c 942c\n"

func TestExtract(t *testing.T) {
	ex, err := Extract([]byte(sample), "en-US")
	if err != nil {
		t.Fatal(err)
	}
	if len(ex.Units) != 2 {
		t.Fatalf("units %+v", ex.Units)
	}
	u := ex.Units[0]
	if u.Source.Plain() != "Good evening,welcome to the showcafé" {
		t.Errorf("text %q", u.Source.Plain())
	}
	tags := u.Source.Tags()
	if len(tags) != 3 || tags[0].Type != "break" || tags[1].Desc != "italics" || tags[2].Desc != "white" {
		t.Errorf("tags %+v", tags)
	}
	// Shown at End of Caption: line 1:00 plus the pairs before it.
	n := int64(strings.Count(strings.Split(sample, "\n")[2], " "))
	if tm := u.Timing; tm.Start != ms(30+n-1) || tm.End != ms(4*30) || tm.LineLimit != 32 {
		t.Errorf("timing %+v", tm)
	}
	if !strings.HasSuffix(u.Key, " --> 00:00:04:00") {
		t.Errorf("key %q", u.Key)
	}
}

func TestMerge(t *testing.T) {
	ex, err := Extract([]byte(sample), "en-US")
	if err != nil {
		t.Fatal(err)
	}
	same, err := Merge([]byte(sample), "en-US", "", filter.Translations{})
	if err != nil || string(same) != sample {
		t.Fatalf("without translations: %v", err)
	}
	src := ex.Units[0].Source
	var tags []content.Tag
	for _, r := range src {
		if r.Tag != nil {
			tags = append(tags, *r.Tag)
		}
	}
	tr := filter.Translations{
		ex.Units[0].ID: content.Text("Guten Abend,").AppendTag(tags[0]).Append("willkommen zur ").AppendTag(tags[1]).Append("Sendung").AppendTag(tags[2]).Append(" – Grüße"),
		ex.Units[1].ID: content.Text("Auf Wiedersehen, bis zum nächsten Mal und danke fürs Zuschauen."),
	}
	if _, err := Merge([]byte(sample), "en-US", "de-DE", tr); err == nil || !strings.Contains(err.Error(), "can't show") {
		t.Errorf("a dash CEA-608 lacks: %v", err)
	}
	tr[ex.Units[0].ID] = content.Text("Guten Abend,").AppendTag(tags[0]).Append("willkommen zur ").AppendTag(tags[1]).Append("Sendung").AppendTag(tags[2]).Append(" - Grüße")
	out, err := Merge([]byte(sample), "en-US", "de-DE", tr)
	if err != nil {
		t.Fatal(err)
	}
	back, err := Extract(out, "de-DE")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if len(back.Units) != 2 {
		t.Fatalf("units %+v\n%s", back.Units, out)
	}
	if got := back.Units[0].Source.Plain(); got != "Guten Abend,willkommen zur Sendung - Grüße" {
		t.Errorf("first %q\n%s", got, out)
	}
	// The long row is wrapped: three rows, the caption moved up.
	if got := back.Units[1].Source.Plain(); got != "Auf Wiedersehen, bis zumnächsten Mal und danke fürsZuschauen." {
		t.Errorf("second %q\n%s", got, out)
	}
	// Each caption is shown when it was.
	for i := range ex.Units {
		if back.Units[i].Timing.Start != ex.Units[i].Timing.Start || back.Units[i].Timing.End != ex.Units[i].Timing.End {
			t.Errorf("caption %d: %+v, was %+v\n%s", i+1, back.Units[i].Timing, ex.Units[i].Timing, out)
		}
	}
	// The second caption's load is longer than the time between the
	// erase and its showing: the erase is sent within it.
	if strings.Contains(string(out), "\n00:00:04:00\t942c 942c\n") || strings.Count(string(out), "942c 942c") != 2 {
		t.Errorf("the erase is not in the load:\n%s", out)
	}
}

func TestTimecodes(t *testing.T) {
	for _, tc := range []string{"00:00:00;00", "00:00:59;29", "00:01:00;02", "00:09:59;29", "00:10:00;00", "01:23:45;12", "10:00:00:00", "00:00:01:15"} {
		f, drop, err := parseTimecode(tc)
		if err != nil {
			t.Fatal(err)
		}
		if got := timecode(f, drop); got != tc {
			t.Errorf("%s comes back as %s (frame %d)", tc, got, f)
		}
	}
	if f, _, _ := parseTimecode("00:01:00;02"); f != 1800 {
		t.Errorf("a minute in drop-frame is frame %d", f)
	}
	for _, bad := range []string{"1:2:3", "00:00:60:00", "00:00:00:30", "aa:00:00:00"} {
		if _, _, err := parseTimecode(bad); err == nil {
			t.Errorf("%s is read", bad)
		}
	}
}

func TestRefused(t *testing.T) {
	for name, src := range map[string]string{
		"no header": "00:00:01:00\t9420\n",
		"roll-up":   "Scenarist_SCC V1.0\n\n00:00:01:00\t9425 9425 94ad 94ad " + hexOf("Hi") + "\n",
		"paint-on":  "Scenarist_SCC V1.0\n\n00:00:01:00\t9429 9429 " + hexOf("Hi") + "\n",
		"bad pair":  "Scenarist_SCC V1.0\n\n00:00:01:00\t94\n",
		"bad time":  "Scenarist_SCC V1.0\n\nsoon\t9420\n",
		"binary":    "Scenarist_SCC V1.0\n\xff\xfe",
	} {
		if _, err := Extract([]byte(src), "en"); err == nil {
			t.Errorf("%s is read", name)
		}
	}
}

func TestCharacters(t *testing.T) {
	for _, r := range "aZ0 !?áéíóúçñÑ®°½¿™¢£♪àèâêîôûÁÉÓÚÜü¡©•“”ÀÇÈÊËëÎÏïÔÙùÛ«»ÃãÍÌìÒòÕõ{}\\^_|~ÄäÖößÅåØø'’" {
		if _, err := encode(r); err != nil {
			t.Errorf("%q: %v", r, err)
		}
	}
	for _, r := range "Łł€–ąő" {
		if _, err := encode(r); err == nil {
			t.Errorf("%q is encoded", r)
		}
	}
}

func TestChannel2(t *testing.T) {
	src := "Scenarist_SCC V1.0\n\n00:00:01:00\t1c20 1c20 1cae 1cae 1c70 1c70 " + hexOf("Two") + " 1c2f 1c2f\n"
	ex, err := Extract([]byte(src), "en")
	if err != nil || len(ex.Units) != 1 || ex.Units[0].Source.Plain() != "Two" {
		t.Fatalf("%v %+v", err, ex)
	}
	out, err := Merge([]byte(src), "en", "es", filter.Translations{ex.Units[0].ID: content.Text("Dos")})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "1c20 1c20 1cae 1cae 1c70 1c70 c4ef 7380 1c2f 1c2f") {
		t.Errorf("written:\n%s", out)
	}
}
