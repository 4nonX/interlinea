package xliff

import (
	"strings"
	"testing"

	"github.com/4nonX/interlinea/filter"
)

func TestPhrase(t *testing.T) {
	file := func(level string) []byte {
		return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns:m="http://www.memsource.com/mxlf/2.0" xmlns="urn:oasis:names:tc:xliff:document:1.2" version="1.2" m:version="2.15" m:level="` + level + `">
<file original="a.docx" source-language="de" target-language="en" datatype="x-undefined"><body><group id="0">
<trans-unit id="t:0" xml:space="preserve" m:score="1.01" m:trans-origin="tm" m:confirmed="1" m:locked="false"><source>Der {1>Mieter&lt;1} zahlt {2}.</source><target>The {1>tenant&lt;1} pays {2}.</target>
<m:tunit-metadata><m:mark id="1"><m:type>code</m:type><m:content>&lt;w:b/&gt;</m:content></m:mark><m:mark id="2"><m:content>&lt;w:tab/&gt;</m:content></m:mark></m:tunit-metadata></trans-unit>
<trans-unit id="t:1" xml:space="preserve" m:confirmed="0" m:locked="true"><source>Fest.</source><target>Fixed.</target></trans-unit>
<trans-unit id="t:2" xml:space="preserve" m:confirmed="0" m:locked="false" m:score="0.62" m:trans-origin="mt"><source>{b>Neu&lt;b}</source><target>New</target></trans-unit>
<trans-unit id="t:3" xml:space="preserve" m:confirmed="0" m:locked="false"><source>Leer.</source><target></target></trans-unit>
</group></body></file></xliff>`)
	}
	data := file("1")
	ex, m := units(t, data)
	if ex.Format != FormatPhrase || len(ex.Units) != 4 {
		t.Fatalf("%s %d", ex.Format, len(ex.Units))
	}
	u := m["1/t:0"]
	if show(u.Source) != "Der {m1}Mieter{/m1} zahlt {m2/}." || u.Source[1].Tag.Desc != "<w:b/>" || u.Source[5].Tag.Desc != "<w:tab/>" ||
		u.State != filter.StateTranslated || u.Match != 101 || u.Origin != "tm" {
		t.Fatalf("t:0 %q %+v", show(u.Source), u)
	}
	if u := m["1/t:1"]; !u.Locked || u.State != filter.StateDraft {
		t.Fatalf("t:1 %+v", u)
	}
	if u := m["1/t:2"]; show(u.Source) != "{mb}Neu{/mb}" || u.Source[0].Tag.Type != "bold" || u.State != filter.StateDraft || u.Match != 62 || u.Origin != "mt" {
		t.Fatalf("t:2 %q %+v", show(u.Source), u)
	}
	if u := m["1/t:3"]; u.State != filter.StateNew {
		t.Fatalf("t:3 %+v", u)
	}
	if _, err := Merge(data, map[string]Update{"1/t:1": {Target: parse("x")}}); err == nil {
		t.Fatal("a locked segment changed")
	}
	out, err := Merge(data, map[string]Update{
		"1/t:2": {Target: parse("{mb}New{/mb}!"), State: filter.StateTranslated},
		"1/t:3": {Target: parse("Empty."), State: filter.StateDraft},
		"1/t:0": {State: filter.StateDraft},
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{
		`<trans-unit id="t:2" xml:space="preserve" m:confirmed="1" m:locked="false" m:score="0.62" m:trans-origin="mt"><source>{b>Neu&lt;b}</source><target>{b>New&lt;b}!</target>`,
		`<trans-unit id="t:3" xml:space="preserve" m:confirmed="0" m:locked="false"><source>Leer.</source><target>Empty.</target>`,
		`<trans-unit id="t:0" xml:space="preserve" m:score="1.01" m:trans-origin="tm" m:confirmed="0" m:locked="false">`,
		// No XLIFF state attributes: Phrase keeps its own.
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %s in\n%s", want, s)
		}
	}
	if strings.Contains(s, `state="`) || strings.Contains(s, `approved=`) {
		t.Errorf("XLIFF states written:\n%s", s)
	}
	// A file for review: confirming is the reviewer's.
	review := file("2")
	_, m = units(t, review)
	if m["1/t:0"].State != filter.StateReviewed || m["1/t:2"].State != filter.StateTranslated || m["1/t:3"].State != filter.StateNew {
		t.Fatalf("review: %s %s %s", m["1/t:0"].State, m["1/t:2"].State, m["1/t:3"].State)
	}
	out, err = Merge(review, map[string]Update{"1/t:2": {State: filter.StateReviewed}, "1/t:3": {Target: parse("Empty."), State: filter.StateTranslated}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `<trans-unit id="t:2" xml:space="preserve" m:confirmed="1"`) || !strings.Contains(string(out), `<trans-unit id="t:3" xml:space="preserve" m:confirmed="0"`) {
		t.Fatalf("review written:\n%s", out)
	}
}

func TestWordfastPro(t *testing.T) {
	data := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<txml version="1.0" locale="DE" targetlocale="en-GB" createdby="WF2.4.1">
<translatable blockId="0"><segment segmentId="1" gtmt="false"><source>Der <ut type="font" x="1">&lt;b&gt;</ut>Mieter<ut type="font" x="2">&lt;/b&gt;</ut> zahlt.</source></segment><ws> </ws><segment segmentId="2" gtmt="true"><source>Zweiter.</source><target score="0">Second.</target></segment></translatable>
<translatable blockId="1"><segment segmentId="1" modified="true" unconfirmed="true"><ws><ut type="content" x="1">&lt;tab/&gt;</ut> </ws><source>Entwurf.</source><ws> </ws><target>Draft.</target></segment></translatable>
<translatable blockId="2"><segment segmentId="1" locked="true"><source>Fest.</source><target>Fixed.</target></segment></translatable>
</txml>`)
	ex, m := units(t, data)
	if ex.Format != FormatTXML || ex.SourceLang != "de" || ex.TargetLang != "en-GB" || len(ex.Units) != 4 {
		t.Fatalf("%+v", ex)
	}
	u := m["b0/1"]
	if show(u.Source) != "Der {x1/}Mieter{x2/} zahlt." || u.Source[1].Tag.Desc != "<b>" || u.State != filter.StateNew {
		t.Fatalf("b0/1 %q %+v", show(u.Source), u)
	}
	if u := m["b0/2"]; u.State != filter.StateTranslated || u.Origin != "mt" {
		t.Fatalf("b0/2 %+v", u)
	}
	if u := m["b1/1"]; u.State != filter.StateDraft || u.Target.Plain() != "Draft." {
		t.Fatalf("b1/1 %+v", u)
	}
	if u := m["b2/1"]; !u.Locked {
		t.Fatalf("b2/1 %+v", u)
	}
	out, err := Merge(data, map[string]Update{
		"b0/1": {Target: parse("The {x1/}tenant{x2/} pays."), State: filter.StateTranslated},
		"b1/1": {State: filter.StateTranslated},
		"b0/2": {Target: parse("Second one."), State: filter.StateDraft},
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{
		`<segment segmentId="1" gtmt="false" modified="true"><source>Der <ut type="font" x="1">&lt;b&gt;</ut>Mieter<ut type="font" x="2">&lt;/b&gt;</ut> zahlt.</source><target>The <ut type="font" x="1">&lt;b&gt;</ut>tenant<ut type="font" x="2">&lt;/b&gt;</ut> pays.</target></segment>`,
		`<segment segmentId="1" modified="true"><ws><ut type="content" x="1">&lt;tab/&gt;</ut> </ws><source>Entwurf.</source><ws> </ws><target>Draft.</target></segment>`,
		`<segment segmentId="2" gtmt="true" modified="true" unconfirmed="true"><source>Zweiter.</source><target score="0">Second one.</target></segment>`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %s in\n%s", want, s)
		}
	}
	if _, err := Merge(data, map[string]Update{"b2/1": {Target: parse("x")}}); err == nil {
		t.Fatal("a locked segment changed")
	}
}

func TestTagEditor(t *testing.T) {
	data := []byte(`<?xml version='1.0'?>
<TRADOStag Version="2.0"><FrontMatter><ToolSettings CreationTool="SDL TRADOS TagEditor"/><UserSettings DataType="HTML" SourceLanguage="DE-DE" TargetLanguage="" SourceDocumentPath="C:\a.html"/></FrontMatter><Body><Raw><ut Type="start" Style="external" DisplayText="p">&lt;p&gt;</ut><Tu Origin="TM" MatchPercent="100"><Tuv Lang="DE-DE">Der <df Bold="on"><ut Type="start" DisplayText="b">&lt;b&gt;</ut>Mieter</df><ut Type="end" DisplayText="b">&lt;/b&gt;</ut> zahlt.</Tuv></Tu> Die Miete &amp; die Kaution. Sie ist <df Italic="on">f&#228;llig.</df> Danach<ut DisplayText="br">&lt;br/&gt;</ut> kommt nichts.<ut Type="end" Style="external" DisplayText="p">&lt;/p&gt;</ut>
<df Size="8"><ut Type="start" Style="external" DisplayText="p">&lt;p&gt;</ut>In der Formatierung.</df></Raw></Body></TRADOStag>`)
	ex, m := units(t, data)
	if ex.Format != FormatTTX || ex.SourceLang != "de-DE" || ex.TargetLang != "" || len(ex.Units) != 5 || len(ex.Warnings) != 1 {
		t.Fatalf("%+v", ex)
	}
	got := []string{}
	for _, u := range ex.Units {
		got = append(got, show(u.Source))
	}
	want := []string{"Der {d1}{u2}Mieter{/d1}{/u2} zahlt.", "Die Miete & die Kaution.", "Sie ist {d1}fällig.{/d1}", "Danach{u1/} kommt nichts.", "In der Formatierung."}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("segments:\n%q\nwant\n%q", got, want)
	}
	if u := m["b1/1"]; u.Match != 100 || u.Origin != "tm" || u.State != filter.StateNew {
		t.Fatalf("b1/1 %+v", u)
	}
	up := map[string]Update{
		"b1/1": {Target: parse("The {d1}{u2}tenant{/d1}{/u2} pays."), State: filter.StateTranslated},
		"b1/2": {Target: parse("The rent & the deposit."), State: filter.StateTranslated},
		"b1/3": {Target: parse("It is {d1}due.{/d1}"), State: filter.StateTranslated},
		"b1/5": {Target: parse("In the formatting."), State: filter.StateTranslated},
	}
	if _, err := Merge(data, up); err == nil || !strings.Contains(err.Error(), "no target language") {
		t.Fatalf("no target language: %v", err)
	}
	out, err := MergeTo(data, up, "en-GB")
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, w := range []string{
		`TargetLanguage="EN-GB"`,
		`<Tu Origin="TM" MatchPercent="100"><Tuv Lang="DE-DE">Der <df Bold="on"><ut Type="start" DisplayText="b">&lt;b&gt;</ut>Mieter</df><ut Type="end" DisplayText="b">&lt;/b&gt;</ut> zahlt.</Tuv><Tuv Lang="EN-GB">The <df Bold="on"><ut Type="start" DisplayText="b">&lt;b&gt;</ut>tenant</df><ut Type="end" DisplayText="b">&lt;/b&gt;</ut> pays.</Tuv></Tu>`,
		`</Tu> <Tu Origin="manual" MatchPercent="0"><Tuv Lang="DE-DE">Die Miete &amp; die Kaution.</Tuv><Tuv Lang="EN-GB">The rent &amp; the deposit.</Tuv></Tu> <Tu Origin="manual" MatchPercent="0"><Tuv Lang="DE-DE">Sie ist <df Italic="on">f&#228;llig.</df></Tuv><Tuv Lang="EN-GB">It is <df Italic="on">due.</df></Tuv></Tu> Danach<ut DisplayText="br">`,
		`<df Size="8"><ut Type="start" Style="external" DisplayText="p">&lt;p&gt;</ut><Tu Origin="manual" MatchPercent="0"><Tuv Lang="DE-DE">In der Formatierung.</Tuv><Tuv Lang="EN-GB">In the formatting.</Tuv></Tu></df>`,
	} {
		if !strings.Contains(s, w) {
			t.Errorf("missing %s in\n%s", w, s)
		}
	}
	ex2, m2 := units(t, out)
	if ex2.TargetLang != "en-GB" || len(ex2.Units) != 5 || m2["b1/2"].Target.Plain() != "The rent & the deposit." || m2["b1/2"].State != filter.StateTranslated || m2["b1/4"].State != filter.StateNew {
		t.Fatalf("after writing: %+v", ex2)
	}
	// A second round changes the targets that are there now.
	out2, err := Merge(out, map[string]Update{"b1/2": {Target: parse("Rent and deposit."), State: filter.StateTranslated}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out2), `<Tuv Lang="EN-GB">Rent and deposit.</Tuv>`) {
		t.Fatalf("second round:\n%s", out2)
	}
}
