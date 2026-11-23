//go:build js && wasm

package main

import (
	"encoding/json"
	"fmt"
	"sync"
	"syscall/js"
	"time"

	"github.com/4nonX/interlinea/qa"
	"github.com/4nonX/interlinea/spell"
)

// Spelling keeps loaded dictionaries by name:
//
//	spellArchive(bytes) -> [{name, locales, aff: Uint8Array, dic: Uint8Array}]
//	spellLoad(name, aff bytes, dic bytes, extra words JSON) -> JSON {words, lang}
//	spellDrop(name)
//	spellCheck(name, words JSON) -> JSON [bool]
//	spellSuggest(name, word, max) -> JSON [string]
//
// Quality checks:
//
//	qaKinds() -> JSON [{kind, name, default, description}]
//	qaCheck(segments JSON, options JSON) -> JSON issues
//	qaReport(document, segments JSON, issues JSON, options JSON, format) -> Uint8Array
//
// qaCheck options are qa.Options as JSON plus "termIndex" (the name of a
// term index from termIndex) and "dictionary" (the name of a loaded
// dictionary). format is "html", "csv" or "json".
func qaFuncs() map[string]any {
	return map[string]any{
		"spellArchive": guard(spellArchive),
		"spellLoad":    guard(spellLoad),
		"spellDrop":    guard(spellDrop),
		"spellCheck":   guard(spellCheck),
		"spellSuggest": guard(spellSuggest),
		"qaKinds":      guard(qaKinds),
		"qaCheck":      guard(qaCheck),
		"qaReport":     guard(qaReport),
	}
}

var (
	dictMu sync.Mutex
	dicts  = map[string]*spell.Dictionary{}
)

func dictionary(name string) (*spell.Dictionary, error) {
	dictMu.Lock()
	defer dictMu.Unlock()
	d, ok := dicts[name]
	if !ok {
		return nil, fmt.Errorf("no dictionary %q is loaded", name)
	}
	return d, nil
}

func spellArchive(args []js.Value) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("spellArchive(bytes)")
	}
	data, err := bytesArg(args[0])
	if err != nil {
		return nil, err
	}
	bs, err := spell.ReadArchive(data)
	if err != nil {
		return nil, err
	}
	out := js.Global().Get("Array").New()
	for _, b := range bs {
		locales := js.Global().Get("Array").New()
		for _, l := range b.Locales {
			locales.Call("push", l)
		}
		o := js.Global().Get("Object").New()
		o.Set("name", b.Name)
		o.Set("locales", locales)
		o.Set("aff", bytesToJS(b.Aff))
		o.Set("dic", bytesToJS(b.Dic))
		out.Call("push", o)
	}
	return out, nil
}

func spellLoad(args []js.Value) (any, error) {
	if len(args) != 4 {
		return nil, fmt.Errorf("spellLoad(name, aff, dic, extraWords)")
	}
	aff, err := bytesArg(args[1])
	if err != nil {
		return nil, err
	}
	dic, err := bytesArg(args[2])
	if err != nil {
		return nil, err
	}
	var extra []string
	if s := args[3].String(); s != "" {
		if err := json.Unmarshal([]byte(s), &extra); err != nil {
			return nil, fmt.Errorf("extra words: %w", err)
		}
	}
	d, err := spell.Load(aff, dic)
	if err != nil {
		return nil, err
	}
	for _, w := range extra {
		d.Add(w)
	}
	dictMu.Lock()
	dicts[args[0].String()] = d
	dictMu.Unlock()
	return jsonOut(map[string]any{"words": d.Words(), "lang": d.Lang()})
}

func spellDrop(args []js.Value) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("spellDrop(name)")
	}
	dictMu.Lock()
	delete(dicts, args[0].String())
	dictMu.Unlock()
	return true, nil
}

func spellCheck(args []js.Value) (any, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("spellCheck(name, words)")
	}
	d, err := dictionary(args[0].String())
	if err != nil {
		return nil, err
	}
	var words []string
	if err := json.Unmarshal([]byte(args[1].String()), &words); err != nil {
		return nil, fmt.Errorf("words: %w", err)
	}
	out := make([]bool, len(words))
	for i, w := range words {
		out[i] = d.Spell(w)
	}
	return jsonOut(out)
}

func spellSuggest(args []js.Value) (any, error) {
	if len(args) != 3 {
		return nil, fmt.Errorf("spellSuggest(name, word, max)")
	}
	d, err := dictionary(args[0].String())
	if err != nil {
		return nil, err
	}
	s := d.Suggest(args[1].String(), args[2].Int())
	if s == nil {
		s = []string{}
	}
	return jsonOut(s)
}

func qaKinds(args []js.Value) (any, error) {
	return jsonOut(qa.Kinds)
}

type qaOptions struct {
	qa.Options
	TermIndex  string `json:"termIndex"`
	Dictionary string `json:"dictionary"`
}

func readQA(segsJSON, optsJSON string) ([]qa.Segment, qa.Options, error) {
	var segs []qa.Segment
	if err := json.Unmarshal([]byte(segsJSON), &segs); err != nil {
		return nil, qa.Options{}, fmt.Errorf("segments: %w", err)
	}
	var o qaOptions
	if err := json.Unmarshal([]byte(optsJSON), &o); err != nil {
		return nil, qa.Options{}, fmt.Errorf("options: %w", err)
	}
	if o.TermIndex != "" {
		x, err := index(o.TermIndex)
		if err != nil {
			return nil, qa.Options{}, err
		}
		o.Terms = x
	}
	if o.Dictionary != "" {
		d, err := dictionary(o.Dictionary)
		if err != nil {
			return nil, qa.Options{}, err
		}
		o.Spell = d
	}
	return segs, o.Options, nil
}

func qaCheck(args []js.Value) (any, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("qaCheck(segments, options)")
	}
	segs, o, err := readQA(args[0].String(), args[1].String())
	if err != nil {
		return nil, err
	}
	issues, err := qa.Check(segs, o)
	if err != nil {
		return nil, err
	}
	if issues == nil {
		issues = []qa.Issue{}
	}
	return jsonOut(issues)
}

func qaReport(args []js.Value) (any, error) {
	if len(args) != 5 {
		return nil, fmt.Errorf("qaReport(document, segments, issues, options, format)")
	}
	segs, o, err := readQA(args[1].String(), args[3].String())
	if err != nil {
		return nil, err
	}
	var issues []qa.Issue
	if err := json.Unmarshal([]byte(args[2].String()), &issues); err != nil {
		return nil, fmt.Errorf("issues: %w", err)
	}
	r := qa.NewReport(args[0].String(), segs, issues, o, time.Now())
	var data []byte
	switch args[4].String() {
	case "html":
		data = r.HTML()
	case "csv":
		data, err = r.CSV()
	case "json":
		data, err = r.JSON()
	default:
		return nil, fmt.Errorf("unknown report format %q", args[4].String())
	}
	if err != nil {
		return nil, err
	}
	return bytesToJS(data), nil
}
