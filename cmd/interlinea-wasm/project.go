//go:build js && wasm

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"syscall/js"
	"time"

	"github.com/4nonX/interlinea/analysis"
	"github.com/4nonX/interlinea/project"
	"github.com/4nonX/interlinea/tm"
)

// Project setups, analysis and pre-translation, against the memories the
// page opened with tmOpen.

// memoryRef names an open memory and the penalty the project gives it.
type memoryRef struct {
	Name    string `json:"name"`
	Penalty int    `json:"penalty"`
}

func openRefs(raw string) ([]tm.Memory, error) {
	var refs []memoryRef
	if err := json.Unmarshal([]byte(raw), &refs); err != nil {
		return nil, fmt.Errorf("memories: %w", err)
	}
	out := make([]tm.Memory, 0, len(refs))
	for _, r := range refs {
		m, err := memory(r.Name)
		if err != nil {
			return nil, err
		}
		p, err := tm.Penalized(m, r.Penalty)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

// projectDefaults() -> JSON settings a new project starts with.
func projectDefaults(args []js.Value) (any, error) {
	return jsonOut(project.Defaults())
}

// projectValidate(settingsJSON, catalogJSON) -> JSON [{field, message}]
func projectValidate(args []js.Value) (any, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("projectValidate(settings, catalog)")
	}
	var s project.Settings
	if err := json.Unmarshal([]byte(args[0].String()), &s); err != nil {
		return nil, fmt.Errorf("settings: %w", err)
	}
	var c project.Catalog
	if err := json.Unmarshal([]byte(args[1].String()), &c); err != nil {
		return nil, fmt.Errorf("catalog: %w", err)
	}
	ps := project.Validate(s, c)
	if ps == nil {
		ps = []project.Problem{}
	}
	return jsonOut(ps)
}

// projectTemplateValidate(templateJSON, catalogJSON) -> JSON [{field, message}]
func projectTemplateValidate(args []js.Value) (any, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("projectTemplateValidate(template, catalog)")
	}
	var t project.Template
	if err := json.Unmarshal([]byte(args[0].String()), &t); err != nil {
		return nil, fmt.Errorf("template: %w", err)
	}
	var c project.Catalog
	if err := json.Unmarshal([]byte(args[1].String()), &c); err != nil {
		return nil, fmt.Errorf("catalog: %w", err)
	}
	ps := project.ValidateTemplate(t, c)
	if ps == nil {
		ps = []project.Problem{}
	}
	return jsonOut(ps)
}

// analyze(filesJSON, memoriesJSON, optionsJSON) -> JSON analysis report.
// files: [{name, segments: [{id, source, locked}]}]; memories: [{name,
// penalty}]; options: analysis options.
func analyze(args []js.Value) (any, error) {
	if len(args) != 3 {
		return nil, fmt.Errorf("analyze(files, memories, options)")
	}
	var files []analysis.File
	if err := json.Unmarshal([]byte(args[0].String()), &files); err != nil {
		return nil, fmt.Errorf("files: %w", err)
	}
	mems, err := openRefs(args[1].String())
	if err != nil {
		return nil, err
	}
	var o analysis.Options
	if err := json.Unmarshal([]byte(args[2].String()), &o); err != nil {
		return nil, fmt.Errorf("options: %w", err)
	}
	r, err := analysis.Analyze(context.Background(), files, mems, o, time.Now())
	if err != nil {
		return nil, err
	}
	return jsonOut(r)
}

// pretranslate(filesJSON, memoriesJSON, prepareJSON, rule) -> JSON
// [{file, id, target, score, memory, context, status}]
func pretranslate(args []js.Value) (any, error) {
	if len(args) != 4 {
		return nil, fmt.Errorf("pretranslate(files, memories, prepare, rule)")
	}
	var files []project.FileSegments
	if err := json.Unmarshal([]byte(args[0].String()), &files); err != nil {
		return nil, fmt.Errorf("files: %w", err)
	}
	mems, err := openRefs(args[1].String())
	if err != nil {
		return nil, err
	}
	var p project.Prepare
	if err := json.Unmarshal([]byte(args[2].String()), &p); err != nil {
		return nil, fmt.Errorf("prepare: %w", err)
	}
	out, err := project.Pretranslate(context.Background(), files, mems, p, args[3].String())
	if err != nil {
		return nil, err
	}
	if out == nil {
		out = []project.Translation{}
	}
	return jsonOut(out)
}

func projectFuncs() map[string]any {
	return map[string]any{
		"projectDefaults":         guard(projectDefaults),
		"projectValidate":         guard(projectValidate),
		"projectTemplateValidate": guard(projectTemplateValidate),
		"analyze":                 guard(analyze),
		"pretranslate":            guard(pretranslate),
	}
}
