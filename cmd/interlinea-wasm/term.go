//go:build js && wasm

package main

// termFuncs is filled in by the termbase import and export bindings.
func termFuncs() map[string]any { return map[string]any{} }
