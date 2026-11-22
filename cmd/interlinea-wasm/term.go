//go:build js && wasm

package main

import (
	"encoding/json"
	"fmt"
	"sync"
	"syscall/js"

	"github.com/4nonX/interlinea/term"
)

// Termbases are kept by the page; these functions read, write and plan
// imports without holding state:
//
//	termDetect(bytes, fileName) -> JSON {format, tabular, rows?, mapping?}
//	termImport(bytes, format, mapping?) -> JSON {termbase, report}
//	termExport(termbase JSON, format) -> Uint8Array
//	termPlan(existing entries JSON, incoming entries JSON, policy JSON) -> JSON {actions, report}
//	termMerge(entry JSON, entry JSON) -> JSON entry
//	termDuplicates(entries JSON, lang) -> JSON [[index, ...], ...]
//
// Term recognition keeps an index per name, rebuilt when the page's
// termbases change:
//
//	termIndex(name, JSON [{source, entries}]) -> number of entries
//	termRecognize(name, text, sourceLang, targetLang) -> JSON hits
//	termCheck(name, sourceText, sourceLang, targetText, targetLang) -> JSON issues
//
// Offsets in hits and issues count UTF-16 code units, as JavaScript strings do.
func termFuncs() map[string]any {
	return map[string]any{
		"termDetect":     guard(termDetect),
		"termImport":     guard(termImport),
		"termExport":     guard(termExport),
		"termPlan":       guard(termPlan),
		"termMerge":      guard(termMerge),
		"termDuplicates": guard(termDuplicates),
		"termIndex":      guard(termIndex),
		"termRecognize":  guard(termRecognize),
		"termCheck":      guard(termCheck),
	}
}

var (
	indexMu sync.Mutex
	indexes = map[string]*term.Index{}
)

func termIndex(args []js.Value) (any, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("termIndex(name, sources)")
	}
	var sources []struct {
		Source  string       `json:"source"`
		Entries []term.Entry `json:"entries"`
	}
	if err := json.Unmarshal([]byte(args[1].String()), &sources); err != nil {
		return nil, fmt.Errorf("sources: %w", err)
	}
	x := term.NewIndex()
	for _, s := range sources {
		x.Add(s.Source, s.Entries)
	}
	indexMu.Lock()
	indexes[args[0].String()] = x
	indexMu.Unlock()
	return x.Len(), nil
}

func index(name string) (*term.Index, error) {
	indexMu.Lock()
	defer indexMu.Unlock()
	x, ok := indexes[name]
	if !ok {
		return nil, fmt.Errorf("no term index %q", name)
	}
	return x, nil
}

// utf16Offsets converts byte offsets into s to UTF-16 offsets.
func utf16Offsets(s string) func(int) int {
	table := make([]int, len(s)+1)
	n := 0
	for i, r := range s {
		table[i] = n
		if r >= 0x10000 {
			n += 2
		} else {
			n++
		}
	}
	table[len(s)] = n
	// Bytes inside a rune map to the rune's start.
	last := 0
	for i := range table {
		if i > 0 && table[i] == 0 {
			table[i] = last
		}
		last = table[i]
	}
	return func(b int) int { return table[b] }
}

func termRecognize(args []js.Value) (any, error) {
	if len(args) != 4 {
		return nil, fmt.Errorf("termRecognize(name, text, sourceLang, targetLang)")
	}
	x, err := index(args[0].String())
	if err != nil {
		return nil, err
	}
	text := args[1].String()
	hits := x.Recognize(text, args[2].String(), args[3].String())
	off := utf16Offsets(text)
	for i := range hits {
		hits[i].Start, hits[i].End = off(hits[i].Start), off(hits[i].End)
	}
	if hits == nil {
		hits = []term.Hit{}
	}
	return jsonOut(hits)
}

func termCheck(args []js.Value) (any, error) {
	if len(args) != 5 {
		return nil, fmt.Errorf("termCheck(name, sourceText, sourceLang, targetText, targetLang)")
	}
	x, err := index(args[0].String())
	if err != nil {
		return nil, err
	}
	target := args[3].String()
	issues := x.Check(x.Recognize(args[1].String(), args[2].String(), args[4].String()), target, args[4].String())
	off := utf16Offsets(target)
	for i := range issues {
		if issues[i].Kind == "forbidden" {
			issues[i].Start, issues[i].End = off(issues[i].Start), off(issues[i].End)
		}
	}
	if issues == nil {
		issues = []term.Issue{}
	}
	return jsonOut(issues)
}

// previewRows is how many spreadsheet rows termDetect returns for the
// column mapping step.
const previewRows = 12

func termDetect(args []js.Value) (any, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("termDetect(bytes, fileName)")
	}
	data, err := bytesArg(args[0])
	if err != nil {
		return nil, err
	}
	f, err := term.Detect(args[1].String(), data)
	if err != nil {
		return nil, err
	}
	out := struct {
		Format  term.Format   `json:"format"`
		Tabular bool          `json:"tabular"`
		Rows    [][]string    `json:"rows,omitempty"`
		Total   int           `json:"total,omitempty"`
		Mapping *term.Mapping `json:"mapping,omitempty"`
	}{Format: f, Tabular: f.IsTabular()}
	if f.IsTabular() {
		rows, err := term.Rows(f, data)
		if err != nil {
			return nil, err
		}
		m := term.GuessMapping(rows)
		out.Mapping = &m
		out.Total = len(rows)
		if len(rows) > previewRows {
			rows = rows[:previewRows]
		}
		out.Rows = rows
	}
	return jsonOut(out)
}

func termImport(args []js.Value) (any, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("termImport(bytes, format, mapping)")
	}
	data, err := bytesArg(args[0])
	if err != nil {
		return nil, err
	}
	var m *term.Mapping
	if len(args) > 2 && args[2].Type() == js.TypeString && args[2].String() != "" {
		m = new(term.Mapping)
		if err := json.Unmarshal([]byte(args[2].String()), m); err != nil {
			return nil, fmt.Errorf("mapping: %w", err)
		}
	}
	tb, rep, err := term.Read(term.Format(args[1].String()), data, m)
	if err != nil {
		return nil, err
	}
	return jsonOut(struct {
		Termbase *term.Termbase `json:"termbase"`
		Report   term.Report    `json:"report"`
	}{tb, rep})
}

func termExport(args []js.Value) (any, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("termExport(termbase, format)")
	}
	var tb term.Termbase
	if err := json.Unmarshal([]byte(args[0].String()), &tb); err != nil {
		return nil, fmt.Errorf("termbase: %w", err)
	}
	data, err := term.Write(&tb, term.Format(args[1].String()))
	if err != nil {
		return nil, err
	}
	arr := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(arr, data)
	return arr, nil
}

func termPlan(args []js.Value) (any, error) {
	if len(args) != 3 {
		return nil, fmt.Errorf("termPlan(existing, incoming, policy)")
	}
	var existing, incoming []term.Entry
	var p term.Policy
	if err := json.Unmarshal([]byte(args[0].String()), &existing); err != nil {
		return nil, fmt.Errorf("existing: %w", err)
	}
	if err := json.Unmarshal([]byte(args[1].String()), &incoming); err != nil {
		return nil, fmt.Errorf("incoming: %w", err)
	}
	if err := json.Unmarshal([]byte(args[2].String()), &p); err != nil {
		return nil, fmt.Errorf("policy: %w", err)
	}
	actions, rep := term.Plan(existing, incoming, p)
	return jsonOut(struct {
		Actions []term.Action   `json:"actions"`
		Report  term.PlanReport `json:"report"`
	}{actions, rep})
}

func termMerge(args []js.Value) (any, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("termMerge(entry, entry)")
	}
	var a, b term.Entry
	if err := json.Unmarshal([]byte(args[0].String()), &a); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(args[1].String()), &b); err != nil {
		return nil, err
	}
	merged, _ := term.MergeEntries(a, b)
	return jsonOut(merged)
}

func termDuplicates(args []js.Value) (any, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("termDuplicates(entries, lang)")
	}
	var entries []term.Entry
	if err := json.Unmarshal([]byte(args[0].String()), &entries); err != nil {
		return nil, err
	}
	groups := term.Duplicates(entries, args[1].String())
	if groups == nil {
		groups = [][]int{}
	}
	return jsonOut(groups)
}
