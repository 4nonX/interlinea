package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter"
	"github.com/4nonX/interlinea/filter/docx"
	"github.com/4nonX/interlinea/filter/formats"
	"github.com/4nonX/interlinea/langtag"
)

// document names the formats extract and merge read.
const documentKinds = "a Word, PowerPoint, Excel, HTML or InDesign IDML file"

// extractDocument reads a document by the format its name says.
func extractDocument(name string, data []byte, lang string, opts docx.Options) (*filter.Extraction, error) {
	switch f := formats.Of(name); {
	case f == formats.Word:
		return docx.Extract(data, lang, opts)
	case formats.Document(f):
		return formats.Extract(f, data, lang)
	}
	return nil, usageErr("%s is not %s", name, documentKinds)
}

// runExtract prints the translatable units of a document.
func runExtract(args []string, _ io.Reader, stdout, stderr io.Writer) error {
	fs := newFlags("extract", "file", stderr)
	lang := fs.String("lang", "en", "source language of the document (BCP 47)")
	asJSON := fs.Bool("json", false, "print units and blocks as JSON")
	keep := fs.Bool("keep-revisions", false, "Word: keep tracked changes as tags instead of accepting them")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	opts := docx.Options{KeepRevisions: *keep}
	if fs.NArg() != 1 {
		return usageErr("expected one file: " + documentKinds)
	}
	if _, err := langtag.Normalize(*lang); err != nil {
		return usageErr("invalid language %q", *lang)
	}
	data, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return err
	}
	ex, err := extractDocument(fs.Arg(0), data, *lang, opts)
	if err != nil {
		return err
	}
	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(ex)
	}
	for _, u := range ex.Units {
		fmt.Fprintf(stdout, "%s\t%s\n", u.ID, u.Source)
	}
	for _, w := range ex.Warnings {
		fmt.Fprintf(stderr, "note: %s\n", w)
	}
	return nil
}

// runMerge writes a translated copy of a document.
func runMerge(args []string, _ io.Reader, stdout, stderr io.Writer) error {
	fs := newFlags("merge", "file translations.json", stderr)
	lang := fs.String("lang", "en", "source language of the document (as used for extract)")
	target := fs.String("target", "", "target language written to the translated text (BCP 47)")
	outName := fs.String("o", "", "output file (required)")
	keep := fs.Bool("keep-revisions", false, "as used for extract")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	opts := docx.Options{KeepRevisions: *keep}
	if fs.NArg() != 2 {
		return usageErr("expected a file (" + documentKinds + ") and a translations file")
	}
	if *outName == "" {
		return usageErr("-o is required")
	}
	if *target == "" {
		return usageErr("-target is required")
	}
	for _, l := range []string{*lang, *target} {
		if _, err := langtag.Normalize(l); err != nil {
			return usageErr("invalid language %q", l)
		}
	}
	data, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(fs.Arg(1))
	if err != nil {
		return err
	}
	tr, err := parseTranslations(raw)
	if err != nil {
		return fmt.Errorf("%s: %w", fs.Arg(1), err)
	}
	var out []byte
	switch f := formats.Of(fs.Arg(0)); {
	case f == formats.Word:
		out, err = docx.Merge(data, *lang, *target, tr, opts)
	case formats.Document(f):
		out, err = formats.Merge(f, data, *lang, *target, tr)
	default:
		return usageErr("%s is not %s", fs.Arg(0), documentKinds)
	}
	if err != nil {
		return err
	}
	if err := os.WriteFile(*outName, out, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "wrote %s (%d translated units)\n", *outName, len(tr))
	return nil
}

// parseTranslations reads {"unit id": content} where content is either a
// JSON array of runs (as printed by extract -json) or a plain string.
func parseTranslations(b []byte) (filter.Translations, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	tr := filter.Translations{}
	for id, v := range m {
		s := strings.TrimSpace(string(v))
		if strings.HasPrefix(s, `"`) {
			var text string
			if err := json.Unmarshal(v, &text); err != nil {
				return nil, fmt.Errorf("unit %s: %w", id, err)
			}
			tr[id] = content.Text(text)
			continue
		}
		var c content.Content
		if err := c.UnmarshalJSON(v); err != nil {
			return nil, fmt.Errorf("unit %s: %w", id, err)
		}
		tr[id] = c
	}
	return tr, nil
}
