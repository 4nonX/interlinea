package textfmt

import (
	"strings"
	"testing"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter"
)

// translate gives every unit a translation: its text in capitals, with
// its tags where they were.
func translate(ex *filter.Extraction) filter.Translations {
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

func roundTrip(t *testing.T, format, src string) (*filter.Extraction, string) {
	t.Helper()
	ex, err := Extract(format, []byte(src), "en-GB")
	if err != nil {
		t.Fatal(err)
	}
	// Nothing translated, the file comes back byte for byte.
	same, err := Merge(format, []byte(src), "en-GB", "", filter.Translations{})
	if err != nil {
		t.Fatal(err)
	}
	if string(same) != src {
		t.Fatalf("%s without translations changed:\n%s", format, same)
	}
	out, err := Merge(format, []byte(src), "en-GB", "de-DE", translate(ex))
	if err != nil {
		t.Fatal(err)
	}
	// What was written reads back as what was translated.
	back, err := Extract(format, out, "de-DE")
	if err != nil {
		t.Fatalf("the translated %s does not read: %v\n%s", format, err, out)
	}
	if len(back.Units) != len(ex.Units) {
		t.Fatalf("%d units written, %d read back:\n%s", len(ex.Units), len(back.Units), out)
	}
	for i, u := range back.Units {
		if want := strings.ToUpper(ex.Units[i].Source.Plain()); u.Source.Plain() != want {
			t.Errorf("unit %d reads back %q, want %q", i, u.Source.Plain(), want)
		}
	}
	return ex, string(out)
}

func unit(t *testing.T, ex *filter.Extraction, key string) filter.Unit {
	t.Helper()
	for _, u := range ex.Units {
		if u.Key == key {
			return u
		}
	}
	t.Fatalf("no unit %q in %+v", key, ex.Units)
	return filter.Unit{}
}

func TestProperties(t *testing.T) {
	src := "# Shown on the start page\n" +
		"welcome = Welcome, {0}!\n" +
		"! no translation needed below\n" +
		"empty=\n" +
		"path.with\\ space : A value \\\n" +
		"    over two lines\n" +
		"umlaut=Gr\\u00FC\\u00DFe und \\uD83D\\uDE00\n" +
		"tab\tseparated\n"
	ex, out := roundTrip(t, Properties, src)
	if len(ex.Units) != 4 {
		t.Fatalf("units %+v", ex.Units)
	}
	w := unit(t, ex, "welcome")
	if w.Source.Plain() != "Welcome, !" || len(w.Source.Tags()) != 1 || w.Source.Tags()[0].Native != "{0}" || len(w.Notes) != 2 || w.Notes[0] != "welcome" || w.Notes[1] != "Shown on the start page" {
		t.Errorf("welcome %+v", w)
	}
	if got := unit(t, ex, "path.with space").Source.Plain(); got != "A value over two lines" {
		t.Errorf("continued value %q", got)
	}
	if got := unit(t, ex, "umlaut").Source.Plain(); got != "Grüße und 😀" {
		t.Errorf("escapes %q", got)
	}
	for _, want := range []string{"welcome = WELCOME, {0}!\n", "empty=\n", "path.with\\ space : A VALUE OVER TWO LINES\n", "umlaut=GR\\u00DC\\u00DFE UND \\uD83D\\uDE00\n", "# Shown on the start page\n", "tab\tSEPARATED\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("translated file lacks %q:\n%s", want, out)
		}
	}
	// A file with its letters as they are keeps them, and Latin-1 stays Latin-1.
	_, out = roundTrip(t, Properties, "gruss=Grüße\n")
	if out != "gruss=GRÜßE\n" {
		t.Errorf("UTF-8 file: %q", out)
	}
	latin, err := Merge(Properties, []byte("gruss=Gr\xfc\xdfe\n"), "de", "en", filter.Translations{"k1:gruss": content.Text("Grüße ✓")})
	if err != nil || string(latin) != "gruss=Gr\\u00FC\\u00DFe \\u2713\n" {
		t.Errorf("Latin-1 file: %q %v", latin, err)
	}
}

func TestJSON(t *testing.T) {
	src := `{
  "menu": {"file": "File", "open": "Open \"%s\"…"},
  "items": [{"title": "First <b>item</b>", "count": 3, "on": true}, "Second"],
  "empty": "",
  "nested": {"key with \"quotes\"": "A\nB"}
}
`
	ex, out := roundTrip(t, JSON, src)
	if len(ex.Units) != 5 {
		t.Fatalf("units %+v", ex.Units)
	}
	if u := unit(t, ex, "menu.open"); u.Source.Plain() != `Open ""…` || u.Source.Tags()[0].Native != "%s" {
		t.Errorf("menu.open %+v", u)
	}
	if u := unit(t, ex, "items[0].title"); len(u.Source.Tags()) != 2 {
		t.Errorf("markup %+v", u)
	}
	unit(t, ex, "items[1]")
	for _, want := range []string{`"file": "FILE"`, `"open": "OPEN \"%s\"…"`, `"title": "FIRST <b>ITEM</b>", "count": 3, "on": true}, "SECOND"]`, `"empty": ""`, `"key with \"quotes\"": "A\nB"`} {
		if !strings.Contains(out, want) {
			t.Errorf("translated file lacks %s:\n%s", want, out)
		}
	}
	if _, err := Extract(JSON, []byte(`{"a": `), "en"); err == nil {
		t.Error("broken JSON is read")
	}
}

func TestYAML(t *testing.T) {
	src := `# Rails locale
en:
  greeting: Hello, %{name}!   # shown first
  quoted: "It's \"fine\""
  single: 'It''s fine'
  count: 3
  enabled: true
  date: 2026-01-01
  menu:
    - Home
    - title: About us
      url: /about
  intro: |
    First line.
    Second line.
  folded: >-
    One sentence
    over two lines.
  list_inline: [a, b]
  empty:
`
	ex, out := roundTrip(t, YAML, src)
	keys := map[string]string{}
	for _, u := range ex.Units {
		keys[u.Key] = u.Source.Plain()
	}
	want := map[string]string{
		"en.greeting": "Hello, !", "en.quoted": `It's "fine"`, "en.single": "It's fine", "en.menu[0]": "Home", "en.menu[1].title": "About us",
		"en.menu[1].url": "/about", "en.intro": "First line.\nSecond line.", "en.folded": "One sentence over two lines.",
	}
	for k, v := range want {
		if keys[k] != v {
			t.Errorf("%s = %q, want %q", k, keys[k], v)
		}
	}
	if len(ex.Units) != len(want) {
		t.Errorf("units %v", keys)
	}
	if n := unit(t, ex, "en.greeting").Notes; len(n) != 1 || n[0] != "en.greeting" {
		// The comment over the file's first key belongs to that key, not to greeting.
		t.Errorf("notes %v", n)
	}
	for _, w := range []string{"  greeting: HELLO, %{name}!   # shown first\n", `  quoted: "IT'S \"FINE\""` + "\n", "  single: 'IT''S FINE'\n", "  count: 3\n", "  date: 2026-01-01\n", "    - HOME\n", "    - title: ABOUT US\n",
		"  intro: |\n    FIRST LINE.\n    SECOND LINE.\n", "  folded: >-\n    ONE SENTENCE OVER TWO LINES.\n", "  list_inline: [a, b]\n"} {
		if !strings.Contains(out, w) {
			t.Errorf("translated file lacks %q:\n%s", w, out)
		}
	}
	// A translation that plain YAML would misread is put in quotes.
	y, err := Merge(YAML, []byte("a: one\nb: two\nc: three\n"), "en", "de", filter.Translations{"y1": content.Text("yes"), "y2": content.Text("key: value"), "y3": content.Text("two\nlines")})
	if err != nil || string(y) != "a: \"yes\"\nb: \"key: value\"\nc: \"two\\nlines\"\n" {
		t.Errorf("quoting: %q %v", y, err)
	}
}

func TestPO(t *testing.T) {
	src := `msgid ""
msgstr ""
"Language: en\n"
"Plural-Forms: nplurals=2; plural=(n != 1);\n"

#. Shown on the start page
#: src/main.c:12
#, c-format, fuzzy
msgctxt "start"
msgid "Welcome, %s!"
msgstr "Willkommn, %s!"

#: src/files.c:40
#, c-format
msgid "%d file"
msgid_plural "%d files"
msgstr[0] ""
msgstr[1] ""

msgid ""
"Two lines\n"
"of text"
msgstr ""

#~ msgid "Old"
#~ msgstr "Alt"
`
	ex, err := Extract(PO, []byte(src), "en")
	if err != nil {
		t.Fatal(err)
	}
	if len(ex.Units) != 4 {
		t.Fatalf("units %+v", ex.Units)
	}
	w := ex.Units[0]
	if w.Key != "start" || w.Source.Plain() != "Welcome, !" || w.Source.Tags()[0].Native != "%s" || len(w.Notes) != 2 || w.Notes[0] != "start" || w.Notes[1] != "Shown on the start page" {
		t.Errorf("welcome %+v", w)
	}
	if ex.Units[1].Source.Plain() != " file" || ex.Units[2].Source.Plain() != " files" || ex.Units[3].Source.Plain() != "Two lines\nof text" {
		t.Errorf("units %+v", ex.Units[1:])
	}
	out, err := Merge(PO, []byte(src), "en", "de-DE", translate(ex))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"\"Language: de_DE\\n\"\n",
		"#. Shown on the start page\n#: src/main.c:12\n#, c-format\nmsgctxt \"start\"\nmsgid \"Welcome, %s!\"\nmsgstr \"WELCOME, %s!\"\n",
		"msgstr[0] \"%d FILE\"\nmsgstr[1] \"%d FILES\"\n",
		"msgid \"\"\n\"Two lines\\n\"\n\"of text\"\nmsgstr \"\"\n\"TWO LINES\\n\"\n\"OF TEXT\"\n",
		"#~ msgid \"Old\"\n#~ msgstr \"Alt\"\n",
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf("translated file lacks %q:\n%s", want, out)
		}
	}
	same, _ := Merge(PO, []byte(src), "en", "", filter.Translations{})
	if string(same) != src {
		t.Errorf("without translations the file changed:\n%s", same)
	}
}

func TestRESX(t *testing.T) {
	src := "\xef\xbb\xbf<?xml version=\"1.0\" encoding=\"utf-8\"?>\r\n<root>\r\n  <resheader name=\"version\"><value>2.0</value></resheader>\r\n" +
		"  <data name=\"Greeting\" xml:space=\"preserve\">\r\n    <value>Hello &amp; welcome, {0}</value>\r\n    <comment>Shown at start</comment>\r\n  </data>\r\n" +
		"  <data name=\"Icon\" type=\"System.Drawing.Bitmap, System.Drawing\" mimetype=\"application/x-microsoft.net.object.bytearray.base64\">\r\n    <value>AAAA</value>\r\n  </data>\r\n" +
		"  <data name=\"Empty\"><value></value></data>\r\n  <data name=\"Less\"><value>a &lt; b</value></data>\r\n</root>\r\n"
	ex, out := roundTrip(t, RESX, src)
	if len(ex.Units) != 2 {
		t.Fatalf("units %+v", ex.Units)
	}
	g := unit(t, ex, "Greeting")
	if g.Source.Plain() != "Hello & welcome, " || g.Source.Tags()[0].Native != "{0}" || g.Notes[1] != "Shown at start" {
		t.Errorf("greeting %+v", g)
	}
	for _, want := range []string{"\xef\xbb\xbf<?xml", "<value>HELLO &amp; WELCOME, {0}</value>\r\n", "<value>AAAA</value>", "<value>2.0</value>", "<value>A &lt; B</value>"} {
		if !strings.Contains(out, want) {
			t.Errorf("translated file lacks %q:\n%s", want, out)
		}
	}
}

func TestSubtitles(t *testing.T) {
	srt := "1\r\n00:00:01,000 --> 00:00:03,500\r\nGood evening.\r\nWelcome to the <i>show</i>.\r\n\r\n2\r\n00:00:04,000 --> 00:00:05,000\r\n- Who is it?\r\n\r\n"
	ex, out := roundTrip(t, SRT, srt)
	if len(ex.Units) != 2 {
		t.Fatalf("units %+v", ex.Units)
	}
	c := ex.Units[0]
	if c.Source.Plain() != "Good evening.Welcome to the show." || c.Key != "00:00:01,000 --> 00:00:03,500" {
		t.Errorf("cue %+v", c)
	}
	tags := c.Source.Tags()
	if len(tags) != 3 || tags[0].Type != "break" || tags[1].Type != "i" || tags[1].Kind != content.TagOpen || tags[2].Kind != content.TagClose || tags[2].ID != tags[1].ID {
		t.Errorf("tags %+v", tags)
	}
	if want := "1\r\n00:00:01,000 --> 00:00:03,500\r\nGOOD EVENING.\r\nWELCOME TO THE <i>SHOW</i>.\r\n\r\n2\r\n00:00:04,000 --> 00:00:05,000\r\n- WHO IS IT?\r\n\r\n"; out != want {
		t.Errorf("translated:\n%q\nwant\n%q", out, want)
	}
	vtt := "WEBVTT\n\nNOTE made by hand\n\nintro\n00:01.000 --> 00:03.000 align:start\n<v Anna>Hello there\n\nSTYLE\n::cue { color: lime }\n\n00:04.000 --> 00:05.000\nBye\n"
	ex, out = roundTrip(t, VTT, vtt)
	if len(ex.Units) != 2 || ex.Units[0].Source.Plain() != "Hello there" {
		t.Fatalf("units %+v", ex.Units)
	}
	if !strings.Contains(out, "NOTE made by hand\n") || !strings.Contains(out, "<v Anna>HELLO THERE\n") || !strings.Contains(out, "::cue { color: lime }") {
		t.Errorf("translated:\n%s", out)
	}
	if _, err := Extract(SRT, []byte("just text\n"), "en"); err == nil {
		t.Error("text without times is read as subtitles")
	}
	if _, err := Extract(VTT, []byte("1\n00:01.000 --> 00:02.000\nHi\n"), "en"); err == nil {
		t.Error("a file without WEBVTT is read as WebVTT")
	}
}

func TestEncodings(t *testing.T) {
	// UTF-16 with its byte order mark comes back as UTF-16.
	src := "\ufeffa=Hello\n"
	var u16 []byte
	for _, r := range src {
		u16 = append(u16, byte(r), byte(r>>8))
	}
	out, err := Merge(Properties, u16, "en", "de", filter.Translations{"k1:a": content.Text("Hallo")})
	if err != nil {
		t.Fatal(err)
	}
	want := "\ufeffa=Hallo\n"
	var w16 []byte
	for _, r := range want {
		w16 = append(w16, byte(r), byte(r>>8))
	}
	if string(out) != string(w16) {
		t.Errorf("UTF-16: %v", out)
	}
	if _, err := Extract(JSON, []byte("{\"a\": \"\xff\"}"), "en"); err == nil {
		t.Error("bytes that are no UTF-8 are read")
	}
}

func TestSBV(t *testing.T) {
	src := "0:00:00.599,0:00:04.160\n>> ALICE: Hi, my name is Alice\nand this is my show.\n\n0:00:04.160,0:00:06.770\nWelcome.\n"
	ex, out := roundTrip(t, SBV, src)
	if len(ex.Units) != 2 || ex.Units[0].Key != "0:00:00.599 --> 0:00:04.160" || len(ex.Units[0].Source.Tags()) != 1 {
		t.Fatalf("units %+v", ex.Units)
	}
	if want := "0:00:00.599,0:00:04.160\n>> ALICE: HI, MY NAME IS ALICE\nAND THIS IS MY SHOW.\n\n0:00:04.160,0:00:06.770\nWELCOME.\n"; out != want {
		t.Errorf("translated:\n%q", out)
	}
	if _, err := Extract(SBV, []byte("hello\n"), "en"); err == nil {
		t.Error("text without times is read as subtitles")
	}
}

func TestASS(t *testing.T) {
	src := "[Script Info]\r\nTitle: Test\r\nScriptType: v4.00+\r\n\r\n[V4+ Styles]\r\nFormat: Name, Fontname\r\nStyle: Default,Arial\r\n\r\n[Events]\r\n" +
		"Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\r\n" +
		"Dialogue: 0,0:00:01.00,0:00:03.50,Default,Anna,0,0,0,,{\\i1}Good evening,{\\i0} and\\Nwelcome, friends.\r\n" +
		"Comment: 0,0:00:02.00,0:00:03.00,Default,,0,0,0,,a note\r\n" +
		"Dialogue: 0,0:00:04.00,0:00:05.00,Sign,,0,0,0,,{\\p1}m 0 0 l 100 0 100 100\r\n" +
		"Dialogue: 0,0:00:06.00,0:00:07.00,Default,,0,0,0,,Bye\\hnow\r\n"
	ex, out := roundTrip(t, ASS, src)
	if len(ex.Units) != 2 {
		t.Fatalf("units %+v", ex.Units)
	}
	u := ex.Units[0]
	if u.Key != "0:00:01.00 --> 0:00:03.50" || u.Source.Plain() != "Good evening, andwelcome, friends." || len(u.Notes) != 3 || u.Notes[2] != "Speaker: Anna" {
		t.Errorf("unit %+v", u)
	}
	if tags := u.Source.Tags(); len(tags) != 3 || tags[0].Kind != content.TagOpen || tags[1].Kind != content.TagClose || tags[1].ID != tags[0].ID || tags[2].Type != "break" {
		t.Errorf("tags %+v", tags)
	}
	for _, want := range []string{
		",,{\\i1}GOOD EVENING,{\\i0} AND\\NWELCOME, FRIENDS.\r\n",
		"Comment: 0,0:00:02.00,0:00:03.00,Default,,0,0,0,,a note\r\n",
		"{\\p1}m 0 0 l 100 0 100 100\r\n",
		",,BYE\\hNOW\r\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("translated has no %q:\n%s", want, out)
		}
	}
	// SubStation Alpha's own Format, with Marked in front.
	ssa := "[Events]\nFormat: Marked, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\nDialogue: Marked=0,0:00:01.00,0:00:02.00,*Default,NTP,0000,0000,0000,!Effect,Hello, you\n"
	ex, _ = roundTrip(t, ASS, ssa)
	if len(ex.Units) != 1 || ex.Units[0].Source.Plain() != "Hello, you" {
		t.Fatalf("SSA units %+v", ex.Units)
	}
	if _, err := Extract(ASS, []byte("[Script Info]\nTitle: x\n"), "en"); err == nil {
		t.Error("a script without events is read")
	}
	if _, err := Extract(ASS, []byte("[Events]\nFormat: Start, End, Text\nDialogue: 0:00:01.00\n"), "en"); err == nil {
		t.Error("a Dialogue line with too few fields is read")
	}
}

func TestMicroDVD(t *testing.T) {
	src := "{1}{1}25\r\n{25}{75}{y:i}Good evening.|Welcome.\r\n{100}{150}Bye\r\n"
	ex, out := roundTrip(t, MicroDVD, src)
	if len(ex.Units) != 2 || ex.Units[0].Key != "00:00:01,000 --> 00:00:03,000" {
		t.Fatalf("units %+v", ex.Units)
	}
	if tags := ex.Units[0].Source.Tags(); len(tags) != 2 || tags[0].Desc != "{y:i}" || tags[1].Type != "break" {
		t.Errorf("tags %+v", tags)
	}
	if want := "{1}{1}25\r\n{25}{75}{y:i}GOOD EVENING.|WELCOME.\r\n{100}{150}BYE\r\n"; out != want {
		t.Errorf("translated:\n%q", out)
	}
	ex, _ = roundTrip(t, MicroDVD, "{10}{20}Hi\n")
	if ex.Units[0].Key != "frames 10 to 20" {
		t.Errorf("without a frame rate: %+v", ex.Units[0])
	}
	if _, err := Extract(MicroDVD, []byte("1\n00:00:01,000 --> 00:00:02,000\nHi\n"), "en"); err == nil {
		t.Error("SubRip is read as MicroDVD")
	}
}

func TestSubtitlesInWindows1252(t *testing.T) {
	// "Grüße – bis bald" in Windows-1252, as subtitles made on Windows are.
	src := "1\r\n00:00:01,000 --> 00:00:02,000\r\nGr\xfc\xdfe \x96 bis bald\r\n"
	ex, err := Extract(SRT, []byte(src), "de-DE")
	if err != nil {
		t.Fatal(err)
	}
	if got := ex.Units[0].Source.Plain(); got != "Grüße – bis bald" {
		t.Fatalf("read %q", got)
	}
	same, _ := Merge(SRT, []byte(src), "de-DE", "", filter.Translations{})
	if string(same) != src {
		t.Errorf("changed without translations: %q", same)
	}
	out, _ := Merge(SRT, []byte(src), "de-DE", "fr-FR", filter.Translations{ex.Units[0].ID: content.Text("À bientôt")})
	if string(out) != "1\r\n00:00:01,000 --> 00:00:02,000\r\n\xc0 bient\xf4t\r\n" {
		t.Errorf("kept in Windows-1252: %q", out)
	}
	// Letters the encoding lacks: the file is written in UTF-8.
	out, _ = Merge(SRT, []byte(src), "de-DE", "pl-PL", filter.Translations{ex.Units[0].ID: content.Text("Do zobaczenia, Łódź")})
	if string(out) != "\xef\xbb\xbf1\r\n00:00:01,000 --> 00:00:02,000\r\nDo zobaczenia, Łódź\r\n" {
		t.Errorf("written in UTF-8: %q", out)
	}
}

func TestSubViewer(t *testing.T) {
	src := "[INFORMATION]\r\n[TITLE]Film\r\n[END INFORMATION]\r\n\r\n00:00:01.00,00:00:03.50\r\nGood evening.[br]Welcome.\r\n\r\n00:00:04.00,00:00:05.00\r\nBye\r\n"
	ex, out := roundTrip(t, MicroDVD, src)
	if len(ex.Units) != 2 || ex.Units[0].Key != "00:00:01.00 --> 00:00:03.50" || ex.Units[0].Source.Tags()[0].Type != "break" {
		t.Fatalf("units %+v", ex.Units)
	}
	if !strings.Contains(out, "\r\nGOOD EVENING.[br]WELCOME.\r\n") {
		t.Errorf("translated:\n%s", out)
	}
	if _, err := Extract(MicroDVD, []byte{0, 0, 1, 0xBA, 1, 2}, "en"); err == nil || !strings.Contains(err.Error(), "pictures") {
		t.Errorf("VobSub: %v", err)
	}
}

func TestSubtitleTimes(t *testing.T) {
	for _, c := range []struct {
		format, src string
		start, end  int64
	}{
		{SRT, "1\n01:02:03,500 --> 01:02:05,250\nHi\n", 3723500, 3725250},
		{VTT, "WEBVTT\n\n02:03.500 --> 02:05.000 align:start\nHi\n", 123500, 125000},
		{SBV, "0:00:01.000,0:00:03.500\nHi\n", 1000, 3500},
		{ASS, "[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\nDialogue: 0,0:00:01.50,0:00:04.25,Default,,0,0,0,,Hi\n", 1500, 4250},
		{MicroDVD, "{1}{1}25\n{25}{100}Hi\n", 1000, 4000},
		{MicroDVD, "00:00:01.00,00:00:02.50\nHi\n", 1000, 2500},
	} {
		ex, err := Extract(c.format, []byte(c.src), "en")
		if err != nil {
			t.Fatal(err)
		}
		if tm := ex.Units[0].Timing; tm == nil || tm.Start != c.start || tm.End != c.end {
			t.Errorf("%s: %+v", c.format, tm)
		}
	}
	// Frames without a frame rate: a subtitle, but its time is not known.
	ex, _ := Extract(MicroDVD, []byte("{25}{100}Hi\n"), "en")
	if tm := ex.Units[0].Timing; tm == nil || tm.Duration() != 0 {
		t.Errorf("without a frame rate: %+v", tm)
	}
	// Strings of software are no subtitles.
	ex, _ = Extract(Properties, []byte("a=Hi\n"), "en")
	if ex.Units[0].Timing != nil {
		t.Error("a property has a time")
	}
}
