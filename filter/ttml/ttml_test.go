package ttml

import (
	"strings"
	"testing"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter"
)

const sample = `<?xml version="1.0" encoding="UTF-8"?>
<tt xmlns="http://www.w3.org/ns/ttml" xmlns:tts="http://www.w3.org/ns/ttml#styling" xml:lang="en">
  <head>
    <metadata><title>Not a subtitle</title></metadata>
    <styling><style xml:id="s1" tts:color="white"/></styling>
  </head>
  <body>
    <div>
      <p begin="00:00:01.000" end="00:00:03.500" agent="anna">Good evening.<br/>
        Welcome to the <span tts:fontStyle="italic">show</span> &amp; more.</p>
      <p begin="00:00:04.000" dur="1s"><span tts:color="yellow">Who is it?</span></p>
      <p begin="00:00:06.000" end="00:00:07.000">   </p>
    </div>
  </body>
</tt>
`

// upper translates every unit into its text in capitals, tags kept.
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

func TestTTML(t *testing.T) {
	if !IsTTML([]byte(sample)) || IsTTML([]byte("<html/>")) || IsTTML([]byte("not xml")) {
		t.Fatal("IsTTML")
	}
	ex, err := Extract([]byte(sample), "en-GB")
	if err != nil {
		t.Fatal(err)
	}
	if len(ex.Units) != 2 {
		t.Fatalf("units %+v", ex.Units)
	}
	u := ex.Units[0]
	if u.Source.Plain() != "Good evening.Welcome to the show & more." || u.Key != "00:00:01.000 --> 00:00:03.500" || u.Notes[1] != "Speaker: anna" {
		t.Errorf("first %+v", u)
	}
	if tags := u.Source.Tags(); len(tags) != 3 || tags[0].Type != "break" || tags[1].Kind != content.TagOpen || tags[1].Type != "span" {
		t.Errorf("tags %+v", tags)
	}
	// A span around the whole subtitle is kept out of sight.
	if ex.Units[1].Source.Plain() != "Who is it?" || len(ex.Units[1].Source.Tags()) != 0 || ex.Units[1].Key != "00:00:04.000 + 1s" {
		t.Errorf("second %+v", ex.Units[1])
	}

	same, err := Merge([]byte(sample), "en-GB", "", filter.Translations{})
	if err != nil || string(same) != sample {
		t.Fatalf("without translations: %v\n%s", err, same)
	}
	out, err := Merge([]byte(sample), "en-GB", "de-DE", upper(ex))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`xml:lang="de-DE">`,
		`<p begin="00:00:01.000" end="00:00:03.500" agent="anna">GOOD EVENING.<br/>WELCOME TO THE <span tts:fontStyle="italic">SHOW</span> &amp; MORE.</p>`,
		`<p begin="00:00:04.000" dur="1s"><span tts:color="yellow">WHO IS IT?</span></p>`,
		`<metadata><title>Not a subtitle</title></metadata>`,
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf("no %s in\n%s", want, out)
		}
	}
	back, err := Extract(out, "de-DE")
	if err != nil || len(back.Units) != 2 || back.Units[0].Source.Plain() != "GOOD EVENING.WELCOME TO THE SHOW & MORE." {
		t.Fatalf("read back: %v %+v", err, back)
	}
}

func TestTTMLPrefixesAndLanguage(t *testing.T) {
	// DFXP with a prefix, no xml:lang yet, and kept spaces.
	src := `<tt:tt xmlns:tt="http://www.w3.org/ns/ttml"><tt:body><tt:div><tt:p begin="1s" end="2s" xml:space="preserve">Two  spaces</tt:p></tt:div></tt:body></tt:tt>`
	ex, err := Extract([]byte(src), "en")
	if err != nil || len(ex.Units) != 1 || ex.Units[0].Source.Plain() != "Two  spaces" {
		t.Fatalf("%v %+v", err, ex)
	}
	out, err := Merge([]byte(src), "en", "fr-FR", filter.Translations{ex.Units[0].ID: content.Text("Deux  espaces")})
	if err != nil {
		t.Fatal(err)
	}
	if want := `<tt:tt xmlns:tt="http://www.w3.org/ns/ttml" xml:lang="fr-FR"><tt:body><tt:div><tt:p begin="1s" end="2s" xml:space="preserve">Deux  espaces</tt:p></tt:div></tt:body></tt:tt>`; string(out) != want {
		t.Errorf("got\n%s", out)
	}
	for _, bad := range []string{"<html><body><p>x</p></body></html>", "<tt><body><p>x</body></tt>", "\xff\xfe\x00"} {
		if _, err := Extract([]byte(bad), "en"); err == nil {
			t.Errorf("%q is read", bad)
		}
	}
}

func TestTTMLTimes(t *testing.T) {
	src := `<tt xmlns="http://www.w3.org/ns/ttml" xmlns:ttp="http://www.w3.org/ns/ttml#parameter" ttp:frameRate="25" ttp:tickRate="10000000" xml:lang="de"><body><div begin="10s">
<p begin="00:00:01.500" end="00:00:03.000">a</p>
<p begin="00:00:04:12" dur="2s">b</p>
<p begin="50000000t" end="70000000t">c</p>
<p begin="1.5m">d</p>
</div></body></tt>`
	ex, err := Extract([]byte(src), "de")
	if err != nil {
		t.Fatal(err)
	}
	want := [][2]int64{{11500, 13000}, {14480, 16480}, {15000, 17000}, {100000, 100000}}
	for i, w := range want {
		if tm := ex.Units[i].Timing; tm == nil || tm.Start != w[0] || tm.End != w[1] {
			t.Errorf("subtitle %d: %+v, want %v", i+1, tm, w)
		}
	}
}
