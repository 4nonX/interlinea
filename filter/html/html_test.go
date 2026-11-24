package html

import (
	"strings"
	"testing"

	"golang.org/x/text/encoding/charmap"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter"
)

const page = `<!DOCTYPE html>
<html lang="de">
<head>
  <meta charset="utf-8">
  <title>Mietrecht &amp; Fristen</title>
  <meta name="description" content="Was Mieter wissen müssen. Kurz erklärt.">
  <meta name="viewport" content="width=device-width">
  <style>p { color: red; } /* <b>kein Text</b> */</style>
  <script>var s = "<p>Kein Text</p>";</script>
</head>
<body>
  <h1>Fristen im   Mietrecht</h1>
  <p class="lead">Die Kündigung muss <b>schriftlich</b> erfolgen. Sie ist
     an den <a href="/vermieter" title="Zum Vermieter">Vermieter</a> zu richten.</p>
  <p>Zeile eins<br>Zeile zwei <img src="haus.png" alt="Ein Haus"> Ende</p>
  <ul>
    <li>Erster Punkt
    <li>Zweiter <em>Punkt</em>
  </ul>
  <table>
    <tr><th>Frist</th><th>Dauer</th></tr>
    <tr><td>Kündigung</td><td>3&nbsp;Monate</td></tr>
  </table>
  <p translate="no">Musterstraße 1</p>
  <p>Marke <span translate="no">Interlinea</span> bleibt.</p>
  <form>
    <input type="text" name="q" placeholder="Suchbegriff">
    <input type="submit" value="Suchen">
    <select><option>Wohnung<option value="h">Haus</select>
    <button type="button">Abschicken</button>
  </form>
  <pre>Zeile A
  Zeile B</pre>
  <div>Offen <i>kursiv <div>Innen</div> weiter</i> Schluss</div>
  <svg><text>Nicht lesen</text></svg>
</body>
</html>
`

func units(t *testing.T, data []byte, lang string) []string {
	t.Helper()
	ex, err := Extract(data, lang)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, u := range ex.Units {
		got = append(got, u.ID+" "+u.Source.String())
	}
	return got
}

func TestExtract(t *testing.T) {
	got := units(t, []byte(page), "de-DE")
	want := []string{
		"b1/s1 Mietrecht & Fristen",
		"b2/s1 Was Mieter wissen müssen.",
		"b2/s2 Kurz erklärt.",
		"b3/s1 Fristen im Mietrecht",
		"b4/s1 Zum Vermieter",
		"b5/s1 Die Kündigung muss {1}schriftlich{/1} erfolgen.",
		"b5/s2 Sie ist an den {1}Vermieter{/1} zu richten.",
		"b6/s1 Ein Haus",
		"b7/s1 Zeile eins{1/}Zeile zwei {2/} Ende",
		"b8/s1 Erster Punkt",
		"b9/s1 Zweiter {1}Punkt{/1}",
		"b10/s1 Frist",
		"b11/s1 Dauer",
		"b12/s1 Kündigung",
		"b13/s1 3\u00a0Monate",
		"b14/s1 Marke {1/} bleibt.",
		"b15/s1 Suchbegriff",
		"b16/s1 Suchen",
		"b17/s1 Wohnung",
		"b18/s1 Haus",
		"b19/s1 Abschicken",
		"b20/s1 Zeile A", // a line of preformatted text is a segment
		"b20/s2 Zeile B",
		"b21/s1 Offen {1/}kursiv",
		"b22/s1 Innen",
		"b23/s1 weiter{1/} Schluss",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("units:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	ex, _ := Extract([]byte(page), "de-DE")
	blocks := map[string]filter.Block{}
	for _, b := range ex.Blocks {
		blocks[b.ID] = b
	}
	if b := blocks["b1"]; b.Part != filter.PartMeta || b.Style != "title" {
		t.Errorf("title: %+v", b)
	}
	if b := blocks["b2"]; b.Part != filter.PartMeta || b.StyleName != "content" {
		t.Errorf("description: %+v", b)
	}
	if b := blocks["b3"]; b.Heading != 1 {
		t.Errorf("heading: %+v", b)
	}
	if b := blocks["b6"]; b.Part != filter.PartAttribute || b.Style != "img" || b.StyleName != "alt" {
		t.Errorf("alt: %+v", b)
	}
	if b := blocks["b9"]; !b.List {
		t.Errorf("list item: %+v", b)
	}
	if b := blocks["b13"]; b.Cell == nil || *b.Cell != (filter.Cell{Table: 0, Row: 1, Col: 1}) {
		t.Errorf("cell: %+v", b.Cell)
	}
	if tag := ex.Units[5].Source.Tags()[0]; tag.Type != "bold" || tag.Desc != "<b>" {
		t.Errorf("tag: %+v", tag)
	}
}

func opened(id string) content.Tag { return content.Tag{Kind: content.TagOpen, ID: id} }
func closeT(id string) content.Tag { return content.Tag{Kind: content.TagClose, ID: id} }
func alone(id string) content.Tag  { return content.Tag{Kind: content.TagStandalone, ID: id} }

func TestMerge(t *testing.T) {
	out, err := Merge([]byte(page), "de-DE", "en-GB", filter.Translations{
		"b1/s1":  content.Text("Tenancy law & notice periods"),
		"b2/s1":  content.Text(`What "tenants" must know.`),
		"b4/s1":  content.Text("To the landlord"),
		"b5/s1":  content.Text("Notice must be given ").AppendTag(opened("1")).Append("in writing").AppendTag(closeT("1")).Append("."),
		"b5/s2":  content.Text("It is to be addressed to the ").AppendTag(opened("1")).Append("landlord").AppendTag(closeT("1")).Append("."),
		"b6/s1":  content.Text("A house"),
		"b7/s1":  content.Text("Line one").AppendTag(alone("1")).Append("line two ").AppendTag(alone("2")).Append(" end"),
		"b9/s1":  content.Text("Second ").AppendTag(opened("1")).Append("item").AppendTag(closeT("1")),
		"b13/s1": content.Text("3\u00a0months"),
		"b14/s1": content.Text("The brand ").AppendTag(alone("1")).Append(" stays <as is>."),
		"b15/s1": content.Text("Search term"),
		"b16/s1": content.Text("Search"),
		"b18/s1": content.Text("House"),
		"b20/s1": content.Text("Line A"),
		"b20/s2": content.Text("Line B"),
		"b21/s1": content.Text("Open ").AppendTag(alone("1")).Append("italic"),
		"b22/s1": content.Text("Inside"),
	})
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	for _, want := range []string{
		`<html lang="en-GB">`,
		`<title>Tenancy law &amp; notice periods</title>`,
		`content="What &quot;tenants&quot; must know. Kurz erklärt.">`,
		`<meta name="viewport" content="width=device-width">`,
		`<style>p { color: red; } /* <b>kein Text</b> */</style>`,
		`<script>var s = "<p>Kein Text</p>";</script>`,
		`<h1>Fristen im   Mietrecht</h1>`, // untouched: every byte as it was
		`<p class="lead">Notice must be given <b>in writing</b>. It is to be addressed to the <a href="/vermieter" title="To the landlord">landlord</a>.</p>`,
		`<p>Line one<br>line two <img src="haus.png" alt="A house"> end</p>`,
		"<li>Erster Punkt\n    <li>Second <em>item</em>\n  </ul>",
		`<td>3&nbsp;months</td>`,
		`<p translate="no">Musterstraße 1</p>`,
		`<p>The brand <span translate="no">Interlinea</span> stays &lt;as is&gt;.</p>`,
		`placeholder="Search term">`,
		`<input type="submit" value="Search">`,
		`<option>Wohnung<option value="h">House</select>`,
		"<pre>Line A\n  Line B</pre>",
		`<div>Open <i>italic <div>Inside</div> weiter</i> Schluss</div>`,
		`<svg><text>Nicht lesen</text></svg>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the page lacks %s", want)
		}
	}
	if t.Failed() {
		t.Log(got)
	}
	// Read again, the page gives the translations as its text.
	back := units(t, out, "en-GB")
	if len(back) != 26 || back[0] != "b1/s1 Tenancy law & notice periods" {
		t.Errorf("read back: %d units, first %q", len(back), back[0])
	}
}

func TestMergeNothingKeepsThePage(t *testing.T) {
	out, err := Merge([]byte(page), "de-DE", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != page {
		t.Error("the page changed without a translation")
	}
}

func TestTagsOutOfOrder(t *testing.T) {
	src := `<p>Sehr <b>wichtig</b> und <i>dringend</i> heute</p>`
	// The translation closes the pairs crosswise and leaves one open: the
	// page must still nest.
	out, err := Merge([]byte(src), "de", "en", filter.Translations{
		"b1/s1": content.Text("Very ").AppendTag(opened("1")).Append("important ").AppendTag(opened("2")).Append("and").AppendTag(closeT("1")).Append(" urgent today"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(out), `<p>Very <b>important <i>and</i></b><i> urgent today</i></p>`; got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

func TestEncodings(t *testing.T) {
	src := "<html><head><meta http-equiv=\"Content-Type\" content=\"text/html; charset=ISO-8859-1\"></head><body><p>Grüße aus München</p></body></html>"
	latin, err := charmap.ISO8859_1.NewEncoder().Bytes([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if got := units(t, latin, "de"); len(got) != 1 || got[0] != "b1/s1 Grüße aus München" {
		t.Fatalf("units: %q", got)
	}
	out, err := Merge(latin, "de", "pl", filter.Translations{"b1/s1": content.Text("Pozdrowienia z Monachium, śnieg i café")})
	if err != nil {
		t.Fatal(err)
	}
	// The page stays in its encoding: é is a byte of it, ś a reference.
	if !strings.Contains(string(out), "Pozdrowienia z Monachium, &#347;nieg i caf\xe9") {
		t.Errorf("merged: %q", out)
	}
	if !strings.Contains(string(out), `<html lang="pl">`) {
		t.Errorf("language: %q", out)
	}

	// UTF-16 with a byte order mark, and a page that names no encoding.
	u16 := []byte{0xFF, 0xFE}
	for _, r := range "<p>Grüße</p>" {
		u16 = append(u16, byte(r), byte(r>>8))
	}
	if got := units(t, u16, "de"); len(got) != 1 || got[0] != "b1/s1 Grüße" {
		t.Fatalf("UTF-16: %q", got)
	}
	back, err := Merge(u16, "de", "", filter.Translations{"b1/s1": content.Text("Hi")})
	if err != nil || string(back) != "\xff\xfe<\x00p\x00>\x00H\x00i\x00<\x00/\x00p\x00>\x00" {
		t.Errorf("UTF-16 merged: %q (%v)", back, err)
	}
	if got := units(t, []byte("<p>Gr\xfc\xdfe</p>"), "de"); len(got) != 1 || got[0] != "b1/s1 Grüße" {
		t.Fatalf("no encoding named: %q", got)
	}
}

func TestDirection(t *testing.T) {
	out, err := Merge([]byte(`<html lang="de" class="x"><body><p>Hallo</p></body></html>`), "de", "ar-SA", filter.Translations{"b1/s1": content.Text("مرحبا")})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(out), `<html lang="ar-SA" class="x" dir="rtl">`) {
		t.Errorf("to Arabic: %s", out)
	}
	back, err := Merge(out, "ar-SA", "de", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(back), `<html lang="de" class="x" dir="ltr">`) {
		t.Errorf("back to German: %s", back)
	}
	// A fragment without an html element stays one.
	frag, err := Merge([]byte(`<p>Hallo</p>`), "de", "ar", filter.Translations{"b1/s1": content.Text("مرحبا")})
	if err != nil || string(frag) != `<p>مرحبا</p>` {
		t.Errorf("fragment: %s (%v)", frag, err)
	}
}

func TestNotHTML(t *testing.T) {
	if _, err := Extract([]byte("PK\x03\x04\x00\x00\x00\x00binary"), "de"); err == nil {
		t.Error("binary data was read as a page")
	}
}

func TestOddMarkup(t *testing.T) {
	for src, want := range map[string]string{
		`a < b und c > d`:                             "b1/s1 a < b und c > d",
		`<p>Text</b> danach`:                          "b1/s1 Text{1/} danach",
		`<P CLASS=x>Gross <B>fett</B></P>`:            "b1/s1 Gross {1}fett{/1}",
		`<p title='Ein "Tipp"'>x`:                     "b1/s1 Ein \"Tipp\"\nb2/s1 x",
		`<p>eins<!-- Hinweis -->zwei</p>`:             "b1/s1 eins{1/}zwei",
		`<p>Kaffee &amp; Kuchen &#8211; t&auml;glich`: "b1/s1 Kaffee & Kuchen – täglich",
		`<textarea>Bitte   <b>hier`:                   "b1/s1 Bitte   <b>hier",
		`<p>unfertig <a href="x`:                      "b1/s1 unfertig",
	} {
		if got := strings.Join(units(t, []byte(src), "de"), "\n"); got != want {
			t.Errorf("%s:\n got %q\nwant %q", src, got, want)
		}
		// Whatever the markup, nothing is lost when nothing is translated.
		if out, err := Merge([]byte(src), "de", "", nil); err != nil || string(out) != src {
			t.Errorf("%s came back as %s (%v)", src, out, err)
		}
	}
}
