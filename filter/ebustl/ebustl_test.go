package ebustl

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter"
)

type cue struct {
	sn      int
	in, out [4]byte
	vp      byte
	comment bool
	ebn     byte // 0 for FFh
	text    []byte
}

// makeSTL writes an EBU STL file: dsc is the display standard ('0' open
// subtitles, '1' teletext).
func makeSTL(dsc byte, cues ...cue) []byte {
	gsi := bytes.Repeat([]byte(" "), gsiSize)
	copy(gsi, "850STL25.01")
	gsi[11] = dsc
	copy(gsi[12:], "00")
	copy(gsi[14:], "08")
	copy(gsi[16:], "Testfilm")
	copy(gsi[238:], fmt.Sprintf("%05d", len(cues)))
	copy(gsi[243:], fmt.Sprintf("%05d", len(cues)))
	copy(gsi[251:], "40")
	copy(gsi[253:], "23")
	out := append([]byte(nil), gsi...)
	for _, c := range cues {
		b := make([]byte, ttiSize)
		b[1], b[2] = byte(c.sn), byte(c.sn>>8)
		b[3] = 0xFF
		if c.ebn != 0 {
			b[3] = c.ebn
		}
		copy(b[5:9], c.in[:])
		copy(b[9:13], c.out[:])
		b[13] = c.vp
		b[14] = 2
		if c.comment {
			b[15] = 1
		}
		field := bytes.Repeat([]byte{unused}, textSize)
		copy(field, c.text)
		copy(b[16:], field)
		out = append(out, b...)
	}
	return out
}

func upper(ex *filter.Extraction) filter.Translations {
	tr := filter.Translations{}
	for _, u := range ex.Units {
		var c content.Content
		for _, r := range u.Source {
			if r.Tag != nil {
				c = c.AppendTag(content.Tag{Kind: r.Tag.Kind, ID: r.Tag.ID})
			} else {
				c = c.Append(strings.ToUpper(r.Text))
			}
		}
		tr[u.ID] = c
	}
	return tr
}

func TestOpenSubtitles(t *testing.T) {
	// "Grüße, \x80mein\x81 Freund" / "bis bald" in table 00: ü is C8h u, ß FBh.
	data := makeSTL('0',
		cue{sn: 1, in: [4]byte{0, 0, 1, 0}, out: [4]byte{0, 0, 3, 12}, vp: 20, text: []byte("Gr\xC8u\xFBe, \x80mein\x81 Freund\x8Abis bald")},
		cue{sn: 2, in: [4]byte{0, 0, 4, 0}, out: [4]byte{0, 0, 5, 0}, vp: 20, comment: true, text: []byte("Kommentar")},
		cue{sn: 3, ebn: 0xFE, text: []byte("user data")},
		cue{sn: 3, in: [4]byte{0, 0, 6, 0}, out: [4]byte{0, 0, 7, 0}, vp: 21, text: []byte("Preis: 5 \xA4")},
	)
	ex, err := Extract(data, "de-DE")
	if err != nil {
		t.Fatal(err)
	}
	if len(ex.Units) != 2 {
		t.Fatalf("units %+v", ex.Units)
	}
	u := ex.Units[0]
	if u.Source.Plain() != "Grüße, mein Freundbis bald" || u.Key != "00:00:01:00 --> 00:00:03:12" {
		t.Errorf("unit %+v", u)
	}
	tags := u.Source.Tags()
	if len(tags) != 3 || tags[0].Type != "italics" || tags[0].Kind != content.TagOpen || tags[1].Kind != content.TagClose || tags[2].Type != "break" {
		t.Errorf("tags %+v", tags)
	}
	if ex.Units[1].Source.Plain() != "Preis: 5 $" {
		t.Errorf("dollar %+v", ex.Units[1])
	}

	same, err := Merge(data, "de-DE", "", filter.Translations{})
	if err != nil || !bytes.Equal(same, data) {
		t.Fatalf("without translations: %v", err)
	}
	out, err := Merge(data, "de-DE", "de-AT", upper(ex))
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != len(data) {
		t.Fatalf("%d bytes, want %d", len(out), len(data))
	}
	first := out[gsiSize+16 : gsiSize+ttiSize]
	if want := "GR\xC8U\xFBE, \x80MEIN\x81 FREUND\x8ABIS BALD\x8F"; !bytes.HasPrefix(first, []byte(want)) {
		t.Errorf("first text % X", first[:40])
	}
	// The comment and the user data are as they were.
	if !bytes.Equal(out[gsiSize+ttiSize:gsiSize+3*ttiSize], data[gsiSize+ttiSize:gsiSize+3*ttiSize]) {
		t.Error("comment or user data changed")
	}
	back, err := Extract(out, "de-AT")
	if err != nil || len(back.Units) != 2 || back.Units[0].Source.Plain() != "GRÜSSE, MEIN FREUNDBIS BALD" && back.Units[0].Source.Plain() != "GRÜßE, MEIN FREUNDBIS BALD" {
		t.Fatalf("read back %v %+v", err, back)
	}
}

func TestExtensionBlocksAndRows(t *testing.T) {
	data := makeSTL('0', cue{sn: 1, in: [4]byte{0, 0, 1, 0}, out: [4]byte{0, 0, 2, 0}, vp: 20, text: []byte("Kurz\x8Azwei")})
	ex, err := Extract(data, "de-DE")
	if err != nil {
		t.Fatal(err)
	}
	long := strings.Repeat("Ein sehr langer Untertitel mit Umlauten: äöü. ", 4)
	// Three rows: the break of the source, and a line break typed in.
	tr := filter.Translations{ex.Units[0].ID: content.Text("Eins").AppendTag(content.Tag{Kind: content.TagStandalone, ID: "1"}).Append("Zwei\n" + long)}
	out, err := Merge(data, "de-DE", "de-DE", tr)
	if err != nil {
		t.Fatal(err)
	}
	n := (len(out) - gsiSize) / ttiSize
	if n < 2 || string(out[238:243]) != fmt.Sprintf("%05d", n) {
		t.Fatalf("%d blocks, TNB %q", n, out[238:243])
	}
	for i := 0; i < n; i++ {
		b := block(out[gsiSize+i*ttiSize : gsiSize+(i+1)*ttiSize])
		if want := byte(i); i == n-1 && b.ebn() != 0xFF || i < n-1 && b.ebn() != want {
			t.Errorf("block %d EBN %02X", i, b.ebn())
		}
		// One row more: the subtitle moves up a row.
		if b.vp() != 19 {
			t.Errorf("block %d VP %d", i, b.vp())
		}
	}
	back, err := Extract(out, "de-DE")
	if err != nil || len(back.Units) != 1 {
		t.Fatalf("read back %v %+v", err, back)
	}
	if got := strings.TrimSpace(back.Units[0].Source.Plain()); got != "EinsZwei"+strings.TrimSpace(long) {
		t.Errorf("read back %q", got)
	}
	if mnc := string(out[251:253]); mnc == "40" {
		t.Errorf("the longest row is not counted: %s", mnc)
	}
}

func TestTeletext(t *testing.T) {
	// Double height and boxing at each row's start, the box ended at its
	// end, and two CR/LF between rows of double height.
	data := makeSTL('1', cue{sn: 1, in: [4]byte{10, 0, 0, 0}, out: [4]byte{10, 0, 2, 0}, vp: 20,
		text: []byte("\x0D\x0B\x0BHallo\x0A\x0A\x8A\x8A\x0D\x0B\x0B\x03Welt\x0A\x0A")})
	ex, err := Extract(data, "de-DE")
	if err != nil {
		t.Fatal(err)
	}
	u := ex.Units[0]
	if u.Source.Plain() != "HalloWelt" {
		t.Fatalf("unit %+v", u)
	}
	if tags := u.Source.Tags(); len(tags) != 1 || tags[0].Type != "break" || !strings.Contains(tags[0].Desc, "yellow") {
		t.Errorf("tags %+v", tags)
	}
	// One row: the break is left out, and the subtitle moves down.
	out, err := Merge(data, "de-DE", "en-GB", filter.Translations{u.ID: content.Text("Hello world")})
	if err != nil {
		t.Fatal(err)
	}
	b := block(out[gsiSize:])
	if want := "\x0D\x0B\x0BHello world\x0A\x0A\x8F"; !bytes.HasPrefix(b.text(), []byte(want)) || b.vp() != 22 {
		t.Errorf("text % X, VP %d", b.text()[:20], b.vp())
	}
	if lc := string(out[14:16]); lc != "09" {
		t.Errorf("language %s", lc)
	}
}

func TestCodeTables(t *testing.T) {
	data := makeSTL('0', cue{sn: 1, in: [4]byte{0, 0, 1, 0}, out: [4]byte{0, 0, 2, 0}, vp: 20, text: []byte("Guten Tag")})
	ex, _ := Extract(data, "de-DE")
	id := ex.Units[0].ID
	out, err := Merge(data, "de-DE", "ru-RU", filter.Translations{id: content.Text("Добрый день")})
	if err != nil {
		t.Fatal(err)
	}
	if string(out[12:14]) != "01" || string(out[14:16]) != "56" {
		t.Errorf("table %s, language %s", out[12:14], out[14:16])
	}
	back, err := Extract(out, "ru-RU")
	if err != nil || back.Units[0].Source.Plain() != "Добрый день" {
		t.Fatalf("read back %v %+v", err, back)
	}
	if _, err := Merge(data, "de-DE", "zh-CN", filter.Translations{id: content.Text("你好")}); err == nil || !strings.Contains(err.Error(), "can't hold") {
		t.Errorf("Chinese: %v", err)
	}
	for _, r := range "ÀÉÎÕŪĂĖÄŮŐŲŠÇçñøłßœ¡¿€" {
		b, err := encodeText(0, string(r))
		if r == '€' {
			if err == nil {
				t.Error("the euro sign is not in table 00")
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: %v", r, err)
			continue
		}
		if s, err := decodeText(0, b); err != nil || s != string(r) {
			t.Errorf("%q comes back as %q (%v)", r, s, err)
		}
	}
}

func TestNotSTL(t *testing.T) {
	for name, data := range map[string][]byte{
		"short":        []byte("STL"),
		"no STL":       bytes.Repeat([]byte(" "), gsiSize),
		"broken block": append(makeSTL('0'), 1, 2, 3),
		"bad table":    func() []byte { d := makeSTL('0'); copy(d[12:], "09"); return d }(),
	} {
		if _, err := Extract(data, "de"); err == nil {
			t.Errorf("%s is read", name)
		}
	}
	// A lone accent is no text.
	if _, err := Extract(makeSTL('0', cue{sn: 1, vp: 20, text: []byte("a\xC8")}), "de"); err == nil {
		t.Error("an accent without its letter is read")
	}
}
