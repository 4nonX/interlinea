package tmx

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/langtag"
)

const sample = `<?xml version="1.0" encoding="UTF-8"?>
<tmx version="1.4">
  <header creationtool="TestTool" creationtoolversion="1" segtype="sentence" o-tmf="x"
          adminlang="en-US" srclang="de-DE" datatype="plaintext" creationdate="20260101T120000Z">
    <prop type="x-client">Kanzlei</prop>
    <note>Header note</note>
  </header>
  <body>
    <tu tuid="1" creationdate="20260102T080000Z" creationid="dan" usagecount="3">
      <prop type="x-domain">Recht</prop>
      <tuv xml:lang="de-DE"><seg>Der Mieter haftet für <bpt i="1" type="bold">&lt;b&gt;</bpt>Schäden<ept i="1">&lt;/b&gt;</ept>.<ph x="2" type="lb">&lt;br/&gt;</ph></seg></tuv>
      <tuv xml:lang="en-GB"><seg>The tenant is liable for <bpt i="1" type="bold">&lt;b&gt;</bpt>damage<ept i="1">&lt;/b&gt;</ept>.<ph x="2" type="lb">&lt;br/&gt;</ph></seg></tuv>
    </tu>
    <tu tuid="2">
      <tuv lang="DE"><seg>Siehe <hi type="italic">Anlage</hi> <it pos="begin" x="5">&lt;u&gt;</it>1</seg></tuv>
      <tuv xml:lang="EN-us"><seg>See <hi type="italic">annex</hi> <it pos="begin" x="5">&lt;u&gt;</it>1</seg></tuv>
      <tuv xml:lang="fr-FR"><seg>Voir annexe 1</seg></tuv>
    </tu>
  </body>
</tmx>`

func readAll(t *testing.T, r io.Reader) (*Reader, []*Unit) {
	t.Helper()
	rd, err := NewReader(r)
	if err != nil {
		t.Fatal(err)
	}
	var units []*Unit
	for {
		u, err := rd.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		units = append(units, u)
	}
	return rd, units
}

func TestRead(t *testing.T) {
	rd, units := readAll(t, strings.NewReader(sample))
	if rd.Header.SrcLang != "de-DE" || rd.Header.CreationTool != "TestTool" {
		t.Fatalf("header = %+v", rd.Header)
	}
	if len(rd.Header.Props) != 1 || rd.Header.Props[0].Value != "Kanzlei" || len(rd.Header.Notes) != 1 {
		t.Fatalf("header props/notes = %+v", rd.Header)
	}
	if !rd.Header.CreationDate.Equal(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("header date = %v", rd.Header.CreationDate)
	}
	if len(units) != 2 {
		t.Fatalf("units = %d", len(units))
	}
	u := units[0]
	if u.ID != "1" || u.CreationID != "dan" || u.UsageCount != 3 || len(u.Props) != 1 {
		t.Fatalf("unit = %+v", u)
	}
	de, q := u.Variant("de-DE")
	if q != langtag.ExactMatch {
		t.Fatal("de variant")
	}
	if got := de.Content.String(); got != "Der Mieter haftet für {1}Schäden{/1}.{ph2/}" {
		t.Fatalf("de content = %q", got)
	}
	if de.Content[1].Tag.Native != "<b>" || de.Content[1].Tag.Type != "bold" {
		t.Fatalf("native/type lost: %+v", de.Content[1].Tag)
	}

	u2 := units[1]
	de2, q := u2.Variant("de-DE")
	if q != langtag.PrimaryMatch {
		t.Fatalf("expected primary match for TMX 1.1 lang attribute, got %v", q)
	}
	if got := de2.Content.String(); got != "Siehe {hi1}Anlage{/hi1} {it5}1" {
		t.Fatalf("hi/it content = %q", got)
	}
	if de2.Content[1].Tag.Type != "italic" || de2.Content[3].Tag.Type != "italic" {
		t.Fatal("hi type lost")
	}
	if _, q := u2.Variant("es"); q != langtag.NoMatch {
		t.Fatal("es should not match")
	}
}

func TestRoundTrip(t *testing.T) {
	rd, units := readAll(t, strings.NewReader(sample))
	var buf bytes.Buffer
	w, err := NewWriter(&buf, rd.Header)
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range units {
		if err := w.Write(u); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	rd2, units2 := readAll(t, bytes.NewReader(buf.Bytes()))
	if rd2.Header.SrcLang != "de-DE" || len(units2) != len(units) {
		t.Fatalf("round trip lost data:\n%s", buf.String())
	}
	for i := range units {
		for j := range units[i].Variants {
			a, b := units[i].Variants[j].Content, units2[i].Variants[j].Content
			if a.Plain() != b.Plain() || len(a.Tags()) != len(b.Tags()) {
				t.Errorf("unit %d variant %d: %q != %q", i, j, a, b)
			}
			for k, tg := range a.Tags() {
				tb := b.Tags()[k]
				if tg.Kind != tb.Kind || tg.Native != tb.Native {
					t.Errorf("tag %d changed: %+v -> %+v", k, tg, tb)
				}
			}
		}
	}
	if units2[0].UsageCount != 3 || !units2[0].CreationDate.Equal(units[0].CreationDate) {
		t.Errorf("unit attributes lost: %+v", units2[0])
	}
}

func TestWriterSegEncoding(t *testing.T) {
	var c content.Content
	c = c.AppendTag(content.Tag{Kind: content.TagClose, ID: "a", Native: "</i>"})
	c = c.Append("x < y & ")
	c = c.AppendTag(content.Tag{Kind: content.TagOpen, ID: "b", Type: "bold", Native: "<b>"})
	c = c.Append("z")
	c = c.AppendTag(content.Tag{Kind: content.TagClose, ID: "b", Native: "</b>"})
	c = c.AppendTag(content.Tag{Kind: content.TagOpen, ID: "c", Native: "<u>"})
	got, err := SegString(c)
	if err != nil {
		t.Fatal(err)
	}
	want := `<it pos="end" x="1">&lt;/i&gt;</it>x &lt; y &amp; <bpt i="1" type="bold">&lt;b&gt;</bpt>z<ept i="1">&lt;/b&gt;</ept><it pos="begin" x="2">&lt;u&gt;</it>`
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func encodeUTF16LE(s string) []byte {
	out := []byte{0xFF, 0xFE}
	for _, u := range utf16.Encode([]rune(s)) {
		out = append(out, byte(u), byte(u>>8))
	}
	return out
}

func TestUTF16AndLatin1(t *testing.T) {
	doc := strings.Replace(sample, `encoding="UTF-8"`, `encoding="UTF-16"`, 1)
	_, units := readAll(t, bytes.NewReader(encodeUTF16LE(doc)))
	if len(units) != 2 {
		t.Fatalf("utf16 units = %d", len(units))
	}
	v, _ := units[0].Variant("de")
	if !strings.Contains(v.Content.Plain(), "Schäden") {
		t.Fatalf("utf16 text = %q", v.Content.Plain())
	}

	latin := "<?xml version=\"1.0\" encoding=\"windows-1252\"?><tmx version=\"1.4\"><header srclang=\"de\"/><body>" +
		"<tu><tuv xml:lang=\"de\"><seg>Gr\xfc\xdfe \x80 \x84Zitat\x93</seg></tuv></tu></body></tmx>"
	_, units = readAll(t, strings.NewReader(latin))
	v, _ = units[0].Variant("de")
	if got := v.Content.Plain(); got != "Grüße € „Zitat“" {
		t.Fatalf("cp1252 text = %q", got)
	}
}

func TestErrors(t *testing.T) {
	if _, err := NewReader(strings.NewReader(`<html><body/></html>`)); err == nil {
		t.Error("non-TMX root accepted")
	}
	if _, err := NewReader(strings.NewReader(`<tmx version="1.4"><header srclang="de"/></tmx>`)); err == nil {
		t.Error("missing body accepted")
	}
	rd, err := NewReader(strings.NewReader(`<tmx version="1.4"><header srclang="de"/><body><tu><tuv><seg>x</seg></tuv></tu></body></tmx>`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rd.Next(); err == nil {
		t.Error("tuv without language accepted")
	}
	rd, err = NewReader(strings.NewReader(`<tmx version="1.4"><header srclang="de"/><body><tu><tuv xml:lang="de"><seg>x</seg></tuv></tu>`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rd.Next(); err != nil {
		t.Fatal(err)
	}
	if _, err := rd.Next(); err == nil || errors.Is(err, io.EOF) {
		t.Errorf("truncated document must fail, got %v", err)
	}
	if _, err := NewWriter(io.Discard, Header{}); err == nil {
		t.Error("writer without srclang accepted")
	}
	w, _ := NewWriter(io.Discard, Header{SrcLang: "de"})
	if err := w.Write(&Unit{ID: "x"}); err == nil {
		t.Error("unit without variants accepted")
	}
	w.Close()
	if err := w.Write(&Unit{Variants: []Variant{{Lang: "de"}}}); err == nil {
		t.Error("write after close accepted")
	}
}

func TestParseDateVariants(t *testing.T) {
	want := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	for _, s := range []string{"20260304T050607Z", "20260304T050607", "2026-03-04T05:06:07Z"} {
		if got := parseDate(s); !got.Equal(want) {
			t.Errorf("parseDate(%q) = %v", s, got)
		}
	}
	if !parseDate("garbage").IsZero() || formatDate(time.Time{}) != "" {
		t.Error("zero handling")
	}
}
