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
	"github.com/4nonX/interlinea/langtag"
)

// runExtract prints the translatable units of a Word document.
func runExtract(args []string, _ io.Reader, stdout, stderr io.Writer) error {
	fs := newFlags("extract", "file.docx", stderr)
	lang := fs.String("lang", "en", "source language of the document (BCP 47)")
	asJSON := fs.Bool("json", false, "print units and blocks as JSON")
	keep := fs.Bool("keep-revisions", false, "keep tracked changes as tags instead of accepting them")
	if err := fs.Parse(args); err != nil {
		return err
	}
	opts := docx.Options{KeepRevisions: *keep}
	if fs.NArg() != 1 {
		return usageErr("expected one .docx file")
	}
	if _, err := langtag.Normalize(*lang); err != nil {
		return usageErr("invalid language %q", *lang)
	}
	data, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return err
	}
	ex, err := docx.Extract(data, *lang, opts)
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

// runMerge writes a translated copy of a Word document.
func runMerge(args []string, _ io.Reader, stdout, stderr io.Writer) error {
	fs := newFlags("merge", "file.docx translations.json", stderr)
	lang := fs.String("lang", "en", "source language of the document (as used for extract)")
	target := fs.String("target", "", "target language written to the translated text (BCP 47)")
	outName := fs.String("o", "", "output file (required)")
	keep := fs.Bool("keep-revisions", false, "as used for extract")
	if err := fs.Parse(args); err != nil {
		return err
	}
	opts := docx.Options{KeepRevisions: *keep}
	if fs.NArg() != 2 {
		return usageErr("expected a .docx file and a translations file")
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
	out, err := docx.Merge(data, *lang, *target, tr, opts)
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
