package markdown

import (
	"strings"
	"testing"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter"
)

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

const sample = "---\ntitle: Not translated\n---\n\n# Getting started\n\nInstall the tool with `npm install`. Then **run** it,\nand read the [manual](https://example.org/doc \"Docs\").\n\n" +
	"Setext heading\n==============\n\n- First item\n- Second *item* with snake_case_name\n  1. Nested one\n- [ ] A task\n\n> A quote that\n> goes on.\n\n" +
	"```sh\necho \"not translated\"\n```\n\n    indented code\n\n| Name | Meaning |\n|------|---------|\n| `a`  | The first |\n| b    | The *second* |\n\n" +
	"Line one  \nline two\\\nline three\n\n![A picture](img.png)\n\n<div class=\"note\">\n\nInside <span>html</span> 2 * 3 * 4.\n\n</div>\n\n[ref]: https://example.org\n\n***\n\nLast ## not a closing ##\n"

func TestMarkdown(t *testing.T) {
	ex, err := Extract([]byte(sample), "en-GB")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, u := range ex.Units {
		got = append(got, u.Source.Plain())
	}
	want := []string{
		"Getting started", "Install the tool with .", "Then run it, and read the manual.", "Setext heading", "First item", "Second item with snake_case_name", "Nested one", "A task",
		"A quote that goes on.", "Name", "Meaning", "The first", "b", "The second", "Line oneline twoline three", "A picture", "Inside html 2 * 3 * 4.", "Last ## not a closing ##",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("units:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if ex.Blocks[0].Heading != 1 || ex.Blocks[2].Heading != 1 || !ex.Blocks[3].List {
		t.Errorf("blocks %+v", ex.Blocks[:4])
	}
	tags := ex.Units[2].Source.Tags()
	if len(tags) != 4 || tags[0].Type != "bold" || tags[2].Type != "link" || tags[3].Native != `](https://example.org/doc "Docs")` {
		t.Errorf("tags %+v", tags)
	}
	same, err := Merge([]byte(sample), "en-GB", "", filter.Translations{})
	if err != nil || string(same) != sample {
		t.Fatalf("without translations the file changed: %v\n%s", err, same)
	}
	out, err := Merge([]byte(sample), "en-GB", "de-DE", upper(ex))
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range []string{
		"---\ntitle: Not translated\n---\n\n# GETTING STARTED\n\n",
		"INSTALL THE TOOL WITH `npm install`. THEN **RUN** IT, AND READ THE [MANUAL](https://example.org/doc \"Docs\").\n\n",
		"SETEXT HEADING\n==============\n",
		"- FIRST ITEM\n- SECOND *ITEM* WITH SNAKE_CASE_NAME\n  1. NESTED ONE\n- [ ] A TASK\n",
		"> A QUOTE THAT GOES ON.\n",
		"```sh\necho \"not translated\"\n```\n\n    indented code\n",
		"| NAME | MEANING |\n|------|---------|\n| `a`  | THE FIRST |\n| B    | THE *SECOND* |\n",
		"LINE ONE  \nLINE TWO  \nLINE THREE\n",
		"![A PICTURE](img.png)\n",
		"<div class=\"note\">\n\nINSIDE <span>HTML</span> 2 * 3 * 4.\n\n</div>\n\n[ref]: https://example.org\n\n***\n\nLAST ## NOT A CLOSING ##\n",
	} {
		if !strings.Contains(string(out), w) {
			t.Errorf("translated file lacks %q:\n%s", w, out)
		}
	}
	// What was written reads back as what was translated.
	back, err := Extract(out, "de-DE")
	if err != nil || len(back.Units) != len(ex.Units) {
		t.Fatalf("read back %d units of %d: %v", len(back.Units), len(ex.Units), err)
	}
	if _, err := Extract([]byte("caf\xe9"), "en"); err == nil {
		t.Error("bytes that are no UTF-8 are read")
	}
	crlf, err := Merge([]byte("# Title\r\n\r\nText here.\r\n"), "en", "de", filter.Translations{"b1/s1": content.Text("Titel"), "b2/s1": content.Text("Text hier.")})
	if err != nil || string(crlf) != "# Titel\r\n\r\nText hier.\r\n" {
		t.Errorf("CRLF: %q %v", crlf, err)
	}
}
