//go:build js && wasm

package main

import (
	"fmt"
	"syscall/js"

	"github.com/4nonX/interlinea/filter/formats"
	"github.com/4nonX/interlinea/filter/mqpackage"
	"github.com/4nonX/interlinea/filter/sdlpackage"
)

// Packages of bilingual files, Trados (.sdlppx) and memoQ (.mqout), told
// apart by the package's file name:
//
//	packageOpen(bytes, name) -> JSON: name, languages and files to translate
//	packageRead(bytes, name, path) -> Uint8Array, the bilingual file
//	packageReturn(bytes, name, {path: Uint8Array}) -> Uint8Array (.sdlrpx or .mqback)
//
// and memoQ XLIFF in its archive (.mqxlz):
//
//	archiveRead(bytes) -> Uint8Array, the memoQ XLIFF document
//	archiveWrite(bytes, document) -> Uint8Array, the archive with the document replaced
func packageFuncs() map[string]any {
	return map[string]any{
		"packageOpen":   guard(packageOpen),
		"packageRead":   guard(packageRead),
		"packageReturn": guard(packageReturn),
		"archiveRead":   guard(archiveRead),
		"archiveWrite":  guard(archiveWrite),
	}
}

func bytesToJS(b []byte) js.Value {
	arr := js.Global().Get("Uint8Array").New(len(b))
	js.CopyBytesToJS(arr, b)
	return arr
}

func memoQ(name string) bool { return formats.Of(name) == formats.MemoQPackage }

func packageOpen(args []js.Value) (any, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("packageOpen(bytes, name)")
	}
	data, err := bytesArg(args[0])
	if err != nil {
		return nil, err
	}
	if memoQ(args[1].String()) {
		p, err := mqpackage.Open(data)
		if err != nil {
			return nil, err
		}
		// In the shape of a Trados package, which the app reads.
		out := sdlpackage.Package{Name: p.Name, Type: "HandoffPackage", SourceLang: p.SourceLang, TargetLangs: p.TargetLangs, Other: p.Other, Files: []sdlpackage.File{}}
		for _, f := range p.Files {
			out.Files = append(out.Files, sdlpackage.File{Path: f.Path, Lang: f.Lang, Target: true, Size: f.Size})
		}
		return jsonOut(out)
	}
	p, err := sdlpackage.Open(data)
	if err != nil {
		return nil, err
	}
	return jsonOut(p)
}

func packageRead(args []js.Value) (any, error) {
	if len(args) != 3 {
		return nil, fmt.Errorf("packageRead(bytes, name, path)")
	}
	data, err := bytesArg(args[0])
	if err != nil {
		return nil, err
	}
	var b []byte
	if memoQ(args[1].String()) {
		b, err = mqpackage.Read(data, args[2].String())
	} else {
		b, err = sdlpackage.Read(data, args[2].String())
	}
	if err != nil {
		return nil, err
	}
	return bytesToJS(b), nil
}

func packageReturn(args []js.Value) (any, error) {
	if len(args) != 3 || args[2].Type() != js.TypeObject {
		return nil, fmt.Errorf("packageReturn(bytes, name, {path: bytes})")
	}
	data, err := bytesArg(args[0])
	if err != nil {
		return nil, err
	}
	keys := js.Global().Get("Object").Call("keys", args[2])
	translated := map[string][]byte{}
	for i := 0; i < keys.Length(); i++ {
		k := keys.Index(i).String()
		b, err := bytesArg(args[2].Get(k))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", k, err)
		}
		translated[k] = b
	}
	var out []byte
	if memoQ(args[1].String()) {
		out, err = mqpackage.Return(data, translated)
	} else {
		out, err = sdlpackage.Return(data, translated)
	}
	if err != nil {
		return nil, err
	}
	return bytesToJS(out), nil
}

func archiveRead(args []js.Value) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("archiveRead(bytes)")
	}
	data, err := bytesArg(args[0])
	if err != nil {
		return nil, err
	}
	doc, err := mqpackage.ReadArchive(data)
	if err != nil {
		return nil, err
	}
	return bytesToJS(doc), nil
}

func archiveWrite(args []js.Value) (any, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("archiveWrite(bytes, document)")
	}
	data, err := bytesArg(args[0])
	if err != nil {
		return nil, err
	}
	doc, err := bytesArg(args[1])
	if err != nil {
		return nil, err
	}
	out, err := mqpackage.WriteArchive(data, doc)
	if err != nil {
		return nil, err
	}
	return bytesToJS(out), nil
}
