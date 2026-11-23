//go:build js && wasm

// Command interlinea-wasm exposes the core's file filters to the browser
// prototype. It registers a global object interlineaCore with:
//
//	extract(bytes: Uint8Array, lang: string, options?: string) -> {ok: true, value: string (JSON extraction)} | {ok: false, error}
//	merge(bytes: Uint8Array, lang: string, target: string, translations: string (JSON), options?: string) -> {ok: true, value: Uint8Array} | {ok: false, error}
//
// options is JSON: {"keepRevisions": true}.
//
// plus translation memory (tm*, see tm.go), termbase (term*, see
// term.go), XLIFF, package, spelling and QA (spell*, qa*, see qa.go)
// functions, and project setups, analysis and pre-translation (see
// project.go).
//
// Build: make wasm
package main

import (
	"encoding/json"
	"fmt"
	"syscall/js"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/filter"
	"github.com/4nonX/interlinea/filter/docx"
)

func result(v any, err error) any {
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}
	}
	return map[string]any{"ok": true, "value": v}
}

func bytesArg(v js.Value) ([]byte, error) {
	if v.Type() != js.TypeObject || v.Get("length").Type() != js.TypeNumber {
		return nil, fmt.Errorf("expected a Uint8Array")
	}
	b := make([]byte, v.Get("length").Int())
	js.CopyBytesToGo(b, v)
	return b, nil
}

func options(args []js.Value, i int) (docx.Options, error) {
	var o struct {
		KeepRevisions bool `json:"keepRevisions"`
	}
	if len(args) > i && args[i].Type() == js.TypeString && args[i].String() != "" {
		if err := json.Unmarshal([]byte(args[i].String()), &o); err != nil {
			return docx.Options{}, fmt.Errorf("options: %w", err)
		}
	}
	return docx.Options{KeepRevisions: o.KeepRevisions}, nil
}

func extract(_ js.Value, args []js.Value) (out any) {
	defer func() {
		if r := recover(); r != nil {
			out = result(nil, fmt.Errorf("internal error: %v", r))
		}
	}()
	if len(args) < 2 {
		return result(nil, fmt.Errorf("extract(bytes, lang, options)"))
	}
	opts, err := options(args, 2)
	if err != nil {
		return result(nil, err)
	}
	data, err := bytesArg(args[0])
	if err != nil {
		return result(nil, err)
	}
	ex, err := docx.Extract(data, args[1].String(), opts)
	if err != nil {
		return result(nil, err)
	}
	b, err := json.Marshal(ex)
	return result(string(b), err)
}

func merge(_ js.Value, args []js.Value) (out any) {
	defer func() {
		if r := recover(); r != nil {
			out = result(nil, fmt.Errorf("internal error: %v", r))
		}
	}()
	if len(args) < 4 {
		return result(nil, fmt.Errorf("merge(bytes, lang, target, translations, options)"))
	}
	opts, err := options(args, 4)
	if err != nil {
		return result(nil, err)
	}
	data, err := bytesArg(args[0])
	if err != nil {
		return result(nil, err)
	}
	var raw map[string]content.Content
	if err := json.Unmarshal([]byte(args[3].String()), &raw); err != nil {
		return result(nil, fmt.Errorf("translations: %w", err))
	}
	out2, err := docx.Merge(data, args[1].String(), args[2].String(), filter.Translations(raw), opts)
	if err != nil {
		return result(nil, err)
	}
	arr := js.Global().Get("Uint8Array").New(len(out2))
	js.CopyBytesToJS(arr, out2)
	return result(arr, nil)
}

func main() {
	api := map[string]any{
		"extract": js.FuncOf(extract),
		"merge":   js.FuncOf(merge),
		"version": "5",
	}
	for k, v := range tmFuncs() {
		api[k] = v
	}
	for k, v := range termFuncs() {
		api[k] = v
	}
	for k, v := range xliffFuncs() {
		api[k] = v
	}
	for k, v := range packageFuncs() {
		api[k] = v
	}
	for k, v := range qaFuncs() {
		api[k] = v
	}
	for k, v := range projectFuncs() {
		api[k] = v
	}
	js.Global().Set("interlineaCore", api)
	if cb := js.Global().Get("interlineaCoreReady"); cb.Type() == js.TypeFunction {
		cb.Invoke()
	}
	select {}
}
