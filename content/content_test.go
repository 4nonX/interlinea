package content

import (
	"encoding/json"
	"testing"
)

func sample() Content {
	var c Content
	c = c.Append("Der Mieter haftet für ")
	c = c.AppendTag(Tag{Kind: TagOpen, ID: "1", Type: "bold", Native: "<b>"})
	c = c.Append("Schäden")
	c = c.AppendTag(Tag{Kind: TagClose, ID: "1", Type: "bold", Native: "</b>"})
	c = c.Append(".")
	return c
}

func TestPlainAndTags(t *testing.T) {
	c := sample()
	if got, want := c.Plain(), "Der Mieter haftet für Schäden."; got != want {
		t.Fatalf("Plain = %q, want %q", got, want)
	}
	if n := len(c.Tags()); n != 2 {
		t.Fatalf("Tags = %d, want 2", n)
	}
	if !c.HasTags() || Text("x").HasTags() {
		t.Fatal("HasTags wrong")
	}
	if got := c.String(); got != "Der Mieter haftet für {1}Schäden{/1}." {
		t.Fatalf("String = %q", got)
	}
	if got := c.Signature(); got != "<1>1" {
		t.Fatalf("Signature = %q", got)
	}
}

func TestAppendMergesText(t *testing.T) {
	c := Text("a").Append("b").Append("")
	if len(c) != 1 || c[0].Text != "ab" {
		t.Fatalf("got %#v", c)
	}
}

func TestEqualIgnoresNativeAndRunSplits(t *testing.T) {
	a := sample()
	b := Content{{Text: "Der Mieter "}, {Text: "haftet für "}}
	b = b.AppendTag(Tag{Kind: TagOpen, ID: "1", Type: "bold", Native: "<w:b/>"})
	b = b.Append("Schäden")
	b = b.AppendTag(Tag{Kind: TagClose, ID: "1", Type: "bold"})
	b = b.Append(".")
	if !a.Equal(b) {
		t.Fatal("expected equal")
	}
	if a.Equal(Text(a.Plain())) {
		t.Fatal("tags must matter")
	}
}

func TestJSONRoundTrip(t *testing.T) {
	c := sample()
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var back Content
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if !c.Equal(back) || back[1].Tag.Native != "<b>" {
		t.Fatalf("round trip mismatch: %s", b)
	}
	var nilC Content
	if b, _ := json.Marshal(nilC); string(b) != "[]" {
		t.Fatalf("nil content = %s", b)
	}
}

func TestValidate(t *testing.T) {
	bad := []Content{
		{{Text: ""}},
		{{Text: "x", Tag: &Tag{Kind: TagOpen, ID: "1"}}},
		{{Tag: &Tag{Kind: TagOpen}}},
		{{Tag: &Tag{Kind: TagOpen, ID: "1"}}, {Tag: &Tag{Kind: TagOpen, ID: "1"}}},
		{{Tag: &Tag{Kind: 9, ID: "1"}}},
	}
	for i, c := range bad {
		if err := c.Validate(); err == nil {
			t.Errorf("case %d: expected error", i)
		}
	}
	if err := sample().Validate(); err != nil {
		t.Fatal(err)
	}
	var c Content
	if err := json.Unmarshal([]byte(`[{"tag":{"kind":"open"}}]`), &c); err == nil {
		t.Fatal("expected validation error from JSON")
	}
}

func TestSliceBoundaries(t *testing.T) {
	// "A. {1}B{/1}. C"
	var c Content
	c = c.Append("A. ")
	c = c.AppendTag(Tag{Kind: TagOpen, ID: "1"})
	c = c.Append("B")
	c = c.AppendTag(Tag{Kind: TagClose, ID: "1"})
	c = c.Append(". C")
	plain := c.Plain() // "A. B. C"
	if plain != "A. B. C" {
		t.Fatal(plain)
	}
	first := c.Slice(0, 3)
	if first.String() != "A. " {
		t.Fatalf("first = %q", first.String())
	}
	second := c.Slice(3, 6)
	if second.String() != "{1}B{/1}. " {
		t.Fatalf("second = %q", second.String())
	}
	third := c.Slice(6, len(plain))
	if third.String() != "C" {
		t.Fatalf("third = %q", third.String())
	}
}

func TestSliceStandaloneAtBoundary(t *testing.T) {
	var c Content
	c = c.Append("one")
	c = c.AppendTag(Tag{Kind: TagStandalone, ID: "lb"})
	c = c.Append("two")
	if got := c.Slice(0, 3).String(); got != "one" {
		t.Fatalf("left = %q", got)
	}
	if got := c.Slice(3, 6).String(); got != "{lb/}two" {
		t.Fatalf("right = %q", got)
	}
}

func TestTagKindText(t *testing.T) {
	for _, k := range []TagKind{TagOpen, TagClose, TagStandalone} {
		b, err := k.MarshalText()
		if err != nil {
			t.Fatal(err)
		}
		var back TagKind
		if err := back.UnmarshalText(b); err != nil || back != k {
			t.Fatalf("round trip %v failed", k)
		}
	}
	if _, err := TagKind(0).MarshalText(); err == nil {
		t.Fatal("expected error")
	}
}
