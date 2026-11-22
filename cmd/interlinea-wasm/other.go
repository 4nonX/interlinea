//go:build !(js && wasm)

// Command interlinea-wasm only runs in a browser. Build it with:
//
//	GOOS=js GOARCH=wasm go build ./cmd/interlinea-wasm
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "interlinea-wasm runs in a browser; build it with GOOS=js GOARCH=wasm (make wasm)")
	os.Exit(2)
}
