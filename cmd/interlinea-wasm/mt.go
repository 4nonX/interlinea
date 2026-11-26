//go:build js && wasm

package main

import (
	"encoding/json"
	"fmt"
	"syscall/js"

	"github.com/4nonX/interlinea/mt"
)

// Translation services (package mt), for the browser on its own: the core
// builds each service's request and reads its answer, and the page sends
// the request with fetch.
//
//	mtServices() -> JSON [mt.Info]
//	mtTranslate(settings, src, tgt, segments) -> JSON mt.Request
//	mtAnswer(settings, segments, status, body) -> JSON [mt.Segment]
//	mtModels(settings) -> JSON mt.Request, or "" for a service without models
//	mtModelsAnswer(settings, status, body) -> JSON [string]
//	mtCheck(settings) -> JSON [mt.Problem]
//
// settings is JSON mt.Settings; segments JSON [mt.Segment] in markup.
func mtFuncs() map[string]any {
	return map[string]any{
		"mtServices":     guard(func([]js.Value) (any, error) { return jsonOut(mt.Services) }),
		"mtTranslate":    guard(mtTranslate),
		"mtAnswer":       guard(mtAnswer),
		"mtModels":       guard(mtModels),
		"mtModelsAnswer": guard(mtModelsAnswer),
		"mtCheck":        guard(mtCheck),
	}
}

func mtSettings(v js.Value) (mt.Settings, error) {
	var s mt.Settings
	if err := json.Unmarshal([]byte(v.String()), &s); err != nil {
		return s, fmt.Errorf("settings: %w", err)
	}
	return s, nil
}

func mtSegments(v js.Value) ([]mt.Segment, error) {
	var segs []mt.Segment
	if err := json.Unmarshal([]byte(v.String()), &segs); err != nil {
		return nil, fmt.Errorf("segments: %w", err)
	}
	return segs, nil
}

func mtTranslate(args []js.Value) (any, error) {
	if len(args) != 4 {
		return nil, fmt.Errorf("mtTranslate(settings, src, tgt, segments)")
	}
	s, err := mtSettings(args[0])
	if err != nil {
		return nil, err
	}
	segs, err := mtSegments(args[3])
	if err != nil {
		return nil, err
	}
	r, err := s.Translate(args[1].String(), args[2].String(), segs)
	if err != nil {
		return nil, err
	}
	return jsonOut(r)
}

func mtAnswer(args []js.Value) (any, error) {
	if len(args) != 4 {
		return nil, fmt.Errorf("mtAnswer(settings, segments, status, body)")
	}
	s, err := mtSettings(args[0])
	if err != nil {
		return nil, err
	}
	segs, err := mtSegments(args[1])
	if err != nil {
		return nil, err
	}
	got, err := s.Answer(segs, args[2].Int(), []byte(args[3].String()))
	if err != nil {
		return nil, err
	}
	return jsonOut(got)
}

func mtModels(args []js.Value) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("mtModels(settings)")
	}
	s, err := mtSettings(args[0])
	if err != nil {
		return nil, err
	}
	r, ok, err := s.Models()
	if !ok {
		return "", nil
	}
	if err != nil {
		return nil, err
	}
	return jsonOut(r)
}

func mtModelsAnswer(args []js.Value) (any, error) {
	if len(args) != 3 {
		return nil, fmt.Errorf("mtModelsAnswer(settings, status, body)")
	}
	s, err := mtSettings(args[0])
	if err != nil {
		return nil, err
	}
	models, err := s.ModelsAnswer(args[1].Int(), []byte(args[2].String()))
	if err != nil {
		return nil, err
	}
	return jsonOut(models)
}

func mtCheck(args []js.Value) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("mtCheck(settings)")
	}
	s, err := mtSettings(args[0])
	if err != nil {
		return nil, err
	}
	p := s.Check()
	if p == nil {
		p = []mt.Problem{}
	}
	return jsonOut(p)
}
