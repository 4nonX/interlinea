package xmlraw

import (
	"bytes"
	"strings"
	"testing"
)

const sample = `<?xml version="1.0" encoding="utf-8"?>
<!-- c -->
<root xmlns="urn:x" xmlns:sdl="urn:sdl" a='1'><sdl:seg id="1" conf="Draft"/><p>Text &amp; <b>bold</b> &lt;x&gt;</p><e></e></root>
`

func TestParse(t *testing.T) {
	doc, err := Read([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	r := doc.Root
	if r.Local != "root" || r.Get("a") != "1" || r.Namespaces()["urn:sdl"] != "sdl" || r.Namespaces()["urn:x"] != "" {
		t.Fatalf("root: %+v", r)
	}
	seg := r.ChildNS("sdl", "seg")
	if seg == nil || !seg.SelfClosing() || seg.Name() != "sdl:seg" || string(seg.Raw) != `<sdl:seg id="1" conf="Draft"/>` {
		t.Fatalf("seg: %+v", seg)
	}
	p := r.Child("p")
	if p.InnerText() != "Text & bold <x>" || string(p.InnerRaw()) != "Text &amp; <b>bold</b> &lt;x&gt;" {
		t.Fatalf("p: %q %q", p.InnerText(), p.InnerRaw())
	}
	e := r.Child("e")
	if e.SelfClosing() || len(e.InnerRaw()) != 0 || e.Inner()[0] != e.Inner()[1] {
		t.Fatalf("empty element: %+v", e)
	}
	if string(doc.Data[p.Inner()[0]:p.Inner()[1]]) != string(p.InnerRaw()) || string(doc.Data[seg.Whole()[0]:seg.Whole()[1]]) != string(seg.Raw) {
		t.Fatal("offsets do not match the data")
	}
	var names []string
	r.Walk(func(n *Node) bool {
		if n.IsElement() {
			names = append(names, n.Name())
		}
		return n.Local != "p"
	})
	if strings.Join(names, " ") != "root sdl:seg p e" {
		t.Fatalf("walk: %v", names)
	}
}

func TestEditsAndAttributes(t *testing.T) {
	doc, _ := Read([]byte(sample))
	seg := doc.Root.ChildNS("sdl", "seg")
	p := doc.Root.Child("p")
	out, err := Apply(doc.Data, []Edit{
		{Span: p.Inner(), With: []byte("neu")},
		{Span: seg.StartTag(), With: SetAttr(SetAttr(seg.Start, "conf", "Translated"), "percent", "100")},
		{Span: Span{p.Off, p.Off}, With: []byte("<x/>")},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `<sdl:seg id="1" conf="Translated" percent="100"/><x/><p>neu</p>`
	if !strings.Contains(string(out), want) {
		t.Fatalf("got %s", out)
	}
	if _, err := Apply(doc.Data, []Edit{{Span: Span{5, 10}}, {Span: Span{8, 12}}}); err == nil {
		t.Fatal("overlap accepted")
	}
	cases := []struct{ in, name, val, want string }{
		{`<target>`, "state", "final", `<target state="final">`},
		{`<target xml:lang="de" state='new'>`, "state", "a&b", `<target xml:lang="de" state="a&amp;b">`},
		{`<trans-unit id="1"  />`, "mq:status", "NotStarted", `<trans-unit id="1" mq:status="NotStarted"  />`},
		{`<seg id="state">`, "state", "x", `<seg id="state" state="x">`},
	}
	for _, c := range cases {
		if got := string(SetAttr([]byte(c.in), c.name, c.val)); got != c.want {
			t.Errorf("SetAttr(%s): %s", c.in, got)
		}
	}
	if got := string(RemoveAttr([]byte(`<seg id="1" conf="Draft" origin="tm">`), "conf")); got != `<seg id="1" origin="tm">` {
		t.Errorf("RemoveAttr: %s", got)
	}
	if got := string(RemoveAttr([]byte(`<seg id="1">`), "conf")); got != `<seg id="1">` {
		t.Errorf("RemoveAttr of a missing attribute: %s", got)
	}
}

func TestEncodings(t *testing.T) {
	plain := []byte(`<?xml version="1.0" encoding="utf-16"?><a>Grüße 😀</a>`)
	for _, enc := range []Encoding{UTF8, UTF8BOM, UTF16LE, UTF16BE} {
		d := &Document{Encoding: enc}
		file := d.Encode(plain)
		doc, err := Read(file)
		if err != nil {
			t.Fatalf("%d: %v", enc, err)
		}
		if doc.Encoding != enc || doc.Root.InnerText() != "Grüße 😀" || !bytes.Equal(doc.Encode(doc.Data), file) {
			t.Fatalf("%d: round trip failed", enc)
		}
	}
	if _, err := Read([]byte{0xff, 0x41}); err == nil {
		t.Fatal("invalid input accepted")
	}
	if _, err := Read([]byte(`<a></a><b/>`)); err == nil {
		t.Fatal("two roots accepted")
	}
	if got := EscapeText("a<&>\x01\tb"); got != "a&lt;&amp;&gt;\tb" {
		t.Fatalf("EscapeText: %q", got)
	}
}
