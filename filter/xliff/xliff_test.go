package xliff

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter"
	"github.com/4nonX/interlinea/internal/xmlraw"
)

func load(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// show renders content with tags as {id}, {/id} and {id/}.
func show(c content.Content) string {
	var b strings.Builder
	for _, r := range c {
		if !r.IsTag() {
			b.WriteString(r.Text)
			continue
		}
		switch r.Tag.Kind {
		case content.TagOpen:
			b.WriteString("{" + r.Tag.ID + "}")
		case content.TagClose:
			b.WriteString("{/" + r.Tag.ID + "}")
		default:
			b.WriteString("{" + r.Tag.ID + "/}")
		}
	}
	return b.String()
}

// parse builds content from the {id} notation, without native markup, as
// an editor sends a translation.
func parse(s string) content.Content {
	var c content.Content
	for s != "" {
		i := strings.IndexByte(s, '{')
		if i < 0 {
			return c.Append(s)
		}
		c = c.Append(s[:i])
		j := strings.IndexByte(s[i:], '}')
		tag := s[i+1 : i+j]
		s = s[i+j+1:]
		switch {
		case strings.HasPrefix(tag, "/"):
			c = c.AppendTag(content.Tag{Kind: content.TagClose, ID: tag[1:]})
		case strings.HasSuffix(tag, "/"):
			c = c.AppendTag(content.Tag{Kind: content.TagStandalone, ID: strings.TrimSuffix(tag, "/")})
		default:
			c = c.AppendTag(content.Tag{Kind: content.TagOpen, ID: tag})
		}
	}
	return c
}

func units(t *testing.T, data []byte) (*filter.Extraction, map[string]filter.Unit) {
	t.Helper()
	ex, err := Extract(data)
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]filter.Unit{}
	for _, u := range ex.Units {
		m[u.ID] = u
	}
	return ex, m
}

func TestPlain12(t *testing.T) {
	data := load(t, "plain12.xlf")
	ex, m := units(t, data)
	if ex.Format != FormatXLIFF12 || ex.SourceLang != "de-DE" || ex.TargetLang != "en-GB" || len(ex.Units) != 4 {
		t.Fatalf("extraction: %+v", ex)
	}
	u := m["1/t1"]
	if show(u.Source) != "Der {1}Mieter{/1} zahlt{2/}monatlich." || show(u.Target) != "The {1}tenant{/1} pays{2/}monthly." ||
		u.State != filter.StateTranslated || u.Key != "title" || u.MaxLength != 40 || len(u.Notes) != 1 || u.Notes[0] != "Rechtlich geprüft" {
		t.Fatalf("t1: %+v", u)
	}
	if u.Source[1].Tag.Type != "bold" || u.Source[5].Tag.Type != "lb" {
		t.Fatalf("tag types: %+v %+v", u.Source[1].Tag, u.Source[5].Tag)
	}
	u = m["1/t2"]
	if show(u.Source) != "Siehe {3}Kaution{/3} und {4/}." || u.Target != nil || u.State != filter.StateNew {
		t.Fatalf("t2: %q %+v", show(u.Source), u)
	}
	if u.Source[1].Tag.Desc != `<a href="#k">` || u.Source[5].Tag.Desc != "<br/>" {
		t.Fatalf("descriptions: %+v", u.Source)
	}
	if m["1/t4"].State != filter.StateFinal || m["1/t5"].State != filter.StateNew {
		t.Fatalf("approved and empty: %+v %+v", m["1/t4"], m["1/t5"])
	}
	if _, ok := m["1/t3"]; ok {
		t.Fatal("translate=no group was extracted")
	}

	out, err := Merge(data, map[string]Update{
		"1/t1": {Target: parse("The {1}lessee{/1} pays{2/}each month."), State: filter.StateReviewed},
		"1/t2": {Target: parse("See {3}deposit{/3} and {4/}."), State: filter.StateDraft},
		"1/t4": {State: filter.StateTranslated},
		"1/t5": {Target: parse("Empty"), State: filter.StateFinal},
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{
		`<target state="signed-off">The <g id="1" ctype="bold">lessee</g> pays<x id="2" ctype="lb"/>each month.</target>`,
		`</source><target state="needs-translation">See <bpt id="3">&lt;a href="#k"&gt;</bpt>deposit<ept id="3">&lt;/a&gt;</ept> and <ph id="4">&lt;br/&gt;</ph>.</target>`,
		`<trans-unit id="t4" approved="no">`,
		`<trans-unit id="t5" approved="yes">`,
		`<target state="final">Empty</target>`,
		`<note>Rechtlich geprüft</note>`,
		`<trans-unit id="t3"><source>Nicht übersetzen</source></trans-unit>`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %s in\n%s", want, s)
		}
	}
	_, m2 := units(t, out)
	if m2["1/t1"].State != filter.StateReviewed || m2["1/t2"].State != filter.StateDraft || show(m2["1/t2"].Target) != "See {3}deposit{/3} and {4/}." || m2["1/t4"].State != filter.StateTranslated || m2["1/t5"].State != filter.StateFinal {
		t.Fatalf("after merge: %+v", m2)
	}
	// No updates, or updates that change nothing, leave the file as it was.
	same, err := Merge(data, map[string]Update{"1/t1": {Target: m["1/t1"].Target, State: filter.StateTranslated}})
	if err != nil || !bytes.Equal(same, data) {
		t.Fatalf("unchanged merge changed the file: %v", err)
	}
}

func TestSDLXLIFF(t *testing.T) {
	data := load(t, "trados.sdlxliff")
	ex, m := units(t, data)
	if ex.Format != FormatSDL || len(ex.Units) != 5 {
		t.Fatalf("units: %d %s", len(ex.Units), ex.Format)
	}
	u := m["1/u1/1"]
	if show(u.Source) != "Der Mieter zahlt." || show(u.Target) != "The tenant pays." || u.State != filter.StateTranslated || u.Match != 100 || u.Origin != "tm" {
		t.Fatalf("seg 1: %+v", u)
	}
	if u := m["1/u1/2"]; u.State != filter.StateNew || len(u.Target) != 0 {
		t.Fatalf("seg 2: %+v", u)
	}
	if u := m["1/u1/3"]; !u.Locked || u.State != filter.StateFinal || u.Target != nil {
		t.Fatalf("seg 3: %+v", u)
	}
	u = m["1/u2/4"]
	if show(u.Source) != "Neuer {pt2/}Satz." || u.Source[1].Tag.Desc != "<br/>" || u.Target != nil {
		t.Fatalf("seg 4: %+v", u)
	}
	u = m["1/u3/5"]
	if show(u.Source) != "Ge{m1/}{m2}ändert{/m2}" || u.Source.Plain() != "Geändert" || u.State != filter.StateRejected {
		t.Fatalf("seg 5: %q %+v", show(u.Source), u)
	}
	if len(ex.Warnings) != 1 || !strings.Contains(ex.Warnings[0], "tracked changes") {
		t.Fatalf("warnings: %v", ex.Warnings)
	}

	if _, err := Merge(data, map[string]Update{"1/u1/3": {Target: parse("x")}}); !errors.Is(err, ErrLocked) {
		t.Fatalf("locked segment: %v", err)
	}
	out, err := Merge(data, map[string]Update{
		"1/u1/2": {Target: parse("The deposit"), State: filter.StateTranslated},
		"1/u1/1": {State: filter.StateReviewed},
		"1/u2/4": {Target: parse("New {pt2/}sentence."), State: filter.StateDraft},
		"1/u3/5": {State: filter.StateRejected},
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{
		`<target><mrk mtype="seg" mid="1">The tenant pays.</mrk> <g id="pt1"><mrk mtype="seg" mid="2">The deposit</mrk></g> </target>`,
		`<sdl:seg id="1" conf="ApprovedTranslation" origin="tm" percent="100"/>`,
		`<sdl:seg id="2" conf="Translated" origin="interactive"/>`,
		`<sdl:seg id="3" locked="true" conf="ApprovedSignOff"/>`,
		`</seg-source><target><mrk mtype="seg" mid="4">New <x id="pt2"/>sentence.</mrk></target><sdl:seg-defs><sdl:seg id="4" conf="Draft" origin="interactive"/></sdl:seg-defs></trans-unit>`,
		`<sdl:seg id="5" conf="RejectedSignOff" origin="interactive"/>`,
		`<trans-unit translate="no" id="s0"><source><x id="0"/></source></trans-unit>`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %s in\n%s", want, s)
		}
	}
	_, m2 := units(t, out)
	if m2["1/u1/2"].State != filter.StateTranslated || show(m2["1/u1/2"].Target) != "The deposit" || m2["1/u2/4"].State != filter.StateDraft || m2["1/u1/1"].State != filter.StateReviewed {
		t.Fatalf("after merge: %+v", m2)
	}
}

func TestSDLMissingTargetMarker(t *testing.T) {
	src := `<?xml version="1.0"?><xliff xmlns:sdl="http://sdl.com/FileTypes/SdlXliff/1.0" xmlns="urn:oasis:names:tc:xliff:document:1.2" version="1.2"><file source-language="de" target-language="en"><body><trans-unit id="u"><source>A. B.</source><seg-source><mrk mtype="seg" mid="1">A.</mrk> <mrk mtype="seg" mid="2">B.</mrk></seg-source><target><mrk mtype="seg" mid="1">A.</mrk></target><sdl:seg-defs><sdl:seg id="1" conf="Translated"/><sdl:seg id="2"/></sdl:seg-defs></trans-unit></body></file></xliff>`
	out, err := Merge([]byte(src), map[string]Update{"1/u/2": {Target: parse("Bee."), State: filter.StateTranslated}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `<target><mrk mtype="seg" mid="1">A.</mrk><mrk mtype="seg" mid="2">Bee.</mrk></target>`) {
		t.Fatalf("got %s", out)
	}
}

func TestMQXLIFF(t *testing.T) {
	data := load(t, "memoq.mqxliff")
	ex, m := units(t, data)
	if ex.Format != FormatMQ || len(ex.Units) != 3 || ex.SourceLang != "de-de" {
		t.Fatalf("extraction: %+v", ex)
	}
	if u := m["1/1"]; show(u.Source) != "Der {1}Mieter{/1} zahlt." || u.State != filter.StateNew || len(u.Target) != 0 {
		t.Fatalf("1: %q %+v", show(u.Source), u)
	}
	if u := m["1/2"]; u.State != filter.StateDraft || u.Match != 101 || show(u.Target) != "The deposit." {
		t.Fatalf("2: %+v", u)
	}
	if u := m["1/3"]; !u.Locked || u.State != filter.StateFinal {
		t.Fatalf("3: %+v", u)
	}
	out, err := Merge(data, map[string]Update{
		"1/1": {Target: parse("The {1}tenant{/1} pays."), State: filter.StateTranslated},
		"1/2": {State: filter.StateDraft},
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{
		`<trans-unit id="1" mq:status="ManuallyConfirmed" mq:segmentguid="a">`,
		`<target xml:space="preserve">The <bpt id="1" ctype="bold">{}</bpt>tenant<ept id="1">{}</ept> pays.</target>`,
		`<mq:historical-unit mq:status="ManuallyConfirmed"><source>Der Mieter zahlt.</source><target>Old</target></mq:historical-unit>`,
		`<trans-unit id="2" mq:status="PreTranslated" mq:percent="101"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %s in\n%s", want, s)
		}
	}
	out, err = Merge(data, map[string]Update{"1/2": {Target: parse("The bond."), State: filter.StateDraft}})
	if err != nil || !strings.Contains(string(out), `<trans-unit id="2" mq:status="PartiallyEdited"`) {
		t.Fatalf("edited pre-translation: %v", err)
	}
}

func TestXLIFF2(t *testing.T) {
	data := load(t, "xliff2.xlf")
	ex, m := units(t, data)
	if ex.Format != FormatXLIFF2 || ex.SourceLang != "de-DE" || ex.TargetLang != "en-GB" || len(ex.Units) != 3 {
		t.Fatalf("extraction: %+v", ex)
	}
	u := m["1/u1/s1"]
	if show(u.Source) != "Hallo {1}Welt{/1} {2/}." || show(u.Target) != "Hello {1}world{/1} {2/}." || u.State != filter.StateTranslated || u.Key != "intro" || u.Notes[0] != "Kontext: Einleitung" {
		t.Fatalf("s1: %+v", u)
	}
	if u.Source[1].Tag.Desc != "<b>" || u.Source[5].Tag.Desc != "{name}" {
		t.Fatalf("original data: %+v", u.Source)
	}
	if u := m["1/u1/s2"]; show(u.Source) != "Zweiter {3}Satz{/3}." || u.State != filter.StateNew {
		t.Fatalf("s2: %q %+v", show(u.Source), u)
	}
	if u := m["1/u3/#1"]; show(u.Source) != "Entwurf {m1}Kaution{/m1}" || u.State != filter.StateDraft {
		t.Fatalf("u3: %+v", u)
	}
	out, err := Merge(data, map[string]Update{
		"1/u1/s2": {Target: parse("Second {3}sentence{/3}."), State: filter.StateRejected},
		"1/u3/#1": {State: filter.StateFinal},
		"1/u1/s1": {Target: parse("Hello {1}World{/1} {2/}!"), State: filter.StateReviewed},
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{
		`<segment id="s2" state="initial" subState="interlinea:rejected">`,
		`</source><target>Second <sc id="3"/>sentence<ec startRef="3"/>.</target>`,
		`<segment state="final">`,
		`<segment id="s1" state="reviewed">`,
		`<target>Hello <pc id="1" dataRefStart="d1" dataRefEnd="d2">World</pc> <ph id="2" dataRef="d3"/>!</target>`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %s in\n%s", want, s)
		}
	}
	_, m2 := units(t, out)
	if m2["1/u1/s2"].State != filter.StateRejected || m2["1/u3/#1"].State != filter.StateFinal {
		t.Fatalf("after merge: %+v", m2)
	}
}

func TestErrors(t *testing.T) {
	data := load(t, "plain12.xlf")
	cases := []struct {
		up   map[string]Update
		want error
	}{
		{map[string]Update{"1/none": {State: filter.StateDraft}}, ErrNoSegment},
		{map[string]Update{"1/t1": {State: "done"}}, ErrBadState},
		{map[string]Update{"1/t1": {Target: parse("The {9}x{/9}")}}, ErrTags},
		{map[string]Update{"1/t1": {Target: parse("The {1}x")}}, ErrTags},
		{map[string]Update{"1/t1": {Target: parse("The x{/1}")}}, ErrTags},
	}
	for _, c := range cases {
		if _, err := Merge(data, c.up); !errors.Is(err, c.want) {
			t.Errorf("%v: got %v, want %v", c.up, err, c.want)
		}
	}
	for _, bad := range []string{`<html/>`, `<xliff version="1.2"><file target-language="en"><body/></file></xliff>`, `not xml`} {
		if _, err := Extract([]byte(bad)); err == nil {
			t.Errorf("%s accepted", bad)
		}
	}
	if f, ok := Detect(load(t, "trados.sdlxliff")); !ok || f != FormatSDL {
		t.Fatalf("detect: %s %v", f, ok)
	}
	if _, ok := Detect([]byte(`<tmx version="1.4"/>`)); ok {
		t.Fatal("TMX detected as XLIFF")
	}
}

func TestUTF16(t *testing.T) {
	data := load(t, "memoq.mqxliff")
	doc := &xmlraw.Document{Encoding: xmlraw.UTF16LE}
	u16 := doc.Encode(bytes.Replace(data, []byte(`encoding="utf-8"`), []byte(`encoding="utf-16"`), 1))
	_, m := units(t, u16)
	if show(m["1/2"].Target) != "The deposit." {
		t.Fatal("UTF-16 file not read")
	}
	out, err := Merge(u16, map[string]Update{"1/2": {Target: parse("The bond."), State: filter.StateTranslated}})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(out, []byte{0xFF, 0xFE}) {
		t.Fatal("merged file lost its UTF-16 encoding")
	}
	_, m2 := units(t, out)
	if show(m2["1/2"].Target) != "The bond." || m2["1/2"].State != filter.StateTranslated {
		t.Fatalf("UTF-16 merge: %+v", m2["1/2"])
	}
}
