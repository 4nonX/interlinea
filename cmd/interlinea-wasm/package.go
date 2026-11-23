//go:build js && wasm

package main

import (
	"fmt"
	"syscall/js"

	"github.com/4nonX/interlinea/filter/sdlpackage"
)

// Trados packages:
//
//	sdlPackageOpen(bytes) -> JSON manifest and files
//	sdlPackageRead(bytes, path) -> Uint8Array
//	sdlPackageReturn(bytes, {path: Uint8Array}) -> Uint8Array (.sdlrpx)
func packageFuncs() map[string]any {
	return map[string]any{
		"sdlPackageOpen":   guard(sdlPackageOpen),
		"sdlPackageRead":   guard(sdlPackageRead),
		"sdlPackageReturn": guard(sdlPackageReturn),
	}
}

func bytesToJS(b []byte) js.Value {
	arr := js.Global().Get("Uint8Array").New(len(b))
	js.CopyBytesToJS(arr, b)
	return arr
}

func sdlPackageOpen(args []js.Value) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("sdlPackageOpen(bytes)")
	}
	data, err := bytesArg(args[0])
	if err != nil {
		return nil, err
	}
	p, err := sdlpackage.Open(data)
	if err != nil {
		return nil, err
	}
	return jsonOut(p)
}

func sdlPackageRead(args []js.Value) (any, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("sdlPackageRead(bytes, path)")
	}
	data, err := bytesArg(args[0])
	if err != nil {
		return nil, err
	}
	b, err := sdlpackage.Read(data, args[1].String())
	if err != nil {
		return nil, err
	}
	return bytesToJS(b), nil
}

func sdlPackageReturn(args []js.Value) (any, error) {
	if len(args) != 2 || args[1].Type() != js.TypeObject {
		return nil, fmt.Errorf("sdlPackageReturn(bytes, {path: bytes})")
	}
	data, err := bytesArg(args[0])
	if err != nil {
		return nil, err
	}
	keys := js.Global().Get("Object").Call("keys", args[1])
	translated := map[string][]byte{}
	for i := 0; i < keys.Length(); i++ {
		k := keys.Index(i).String()
		b, err := bytesArg(args[1].Get(k))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", k, err)
		}
		translated[k] = b
	}
	out, err := sdlpackage.Return(data, translated)
	if err != nil {
		return nil, err
	}
	return bytesToJS(out), nil
}
