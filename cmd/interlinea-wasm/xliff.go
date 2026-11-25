//go:build js && wasm

package main

import (
	"encoding/json"
	"fmt"
	"syscall/js"

	"github.com/4nonX/interlinea/filter"
	"github.com/4nonX/interlinea/filter/xliff"
	"github.com/4nonX/interlinea/project"
)

// Bilingual XLIFF files (XLIFF 1.2, SDLXLIFF, MQXLIFF, XLIFF 2):
//
//	xliffDetect(bytes) -> the variant, or "" when the file is not XLIFF
//	xliffExtract(bytes) -> JSON extraction with targets and states
//	xliffMerge(bytes, updates JSON {id: {target, state}}) -> Uint8Array
func xliffFuncs() map[string]any {
	return map[string]any{
		"xliffDetect":  guard(xliffDetect),
		"xliffExtract": guard(xliffExtract),
		"xliffMerge":   guard(xliffMerge),
	}
}

func xliffDetect(args []js.Value) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("xliffDetect(bytes)")
	}
	data, err := bytesArg(args[0])
	if err != nil {
		return nil, err
	}
	f, _ := xliff.Detect(data)
	return f, nil
}

func xliffExtract(args []js.Value) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("xliffExtract(bytes)")
	}
	data, err := bytesArg(args[0])
	if err != nil {
		return nil, err
	}
	ex, err := xliff.Extract(data)
	if err != nil {
		return nil, err
	}
	return jsonOut(ex)
}

// xliffMerge(bytes, segments): writes the segments' translations and
// states into an XLIFF file. Each segment comes as the editor has it,
// with "state", its state in the file; segments that were split or joined
// are written as the file has its segments.
func xliffMerge(args []js.Value) (any, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("xliffMerge(bytes, segments)")
	}
	data, err := bytesArg(args[0])
	if err != nil {
		return nil, err
	}
	var in []struct {
		project.Segment
		State filter.State `json:"state"`
	}
	if err := json.Unmarshal([]byte(args[1].String()), &in); err != nil {
		return nil, fmt.Errorf("segments: %w", err)
	}
	segs := make([]project.Segment, len(in))
	states := map[string]filter.State{}
	for i, x := range in {
		segs[i] = x.Segment
		states[x.ID] = x.State
	}
	up := project.XLIFFUpdatesBy(segs, func(s project.Segment) filter.State { return states[s.ID] })
	out, err := xliff.Merge(data, up)
	if err != nil {
		return nil, err
	}
	arr := js.Global().Get("Uint8Array").New(len(out))
	js.CopyBytesToJS(arr, out)
	return arr, nil
}
