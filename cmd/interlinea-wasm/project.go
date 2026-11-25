//go:build js && wasm

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"syscall/js"
	"time"

	"github.com/4nonX/interlinea/analysis"
	"github.com/4nonX/interlinea/content"
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

// resegment(how, segments, index, before): cuts a file's segments
// differently. how is "split" (where before, the start of the segment's
// source, ends) or "join" (with the segment after it). It returns the file's segments as they are afterwards;
// what the segments do not allow is the error's text.
func resegment(args []js.Value) (any, error) {
	if len(args) != 4 {
		return nil, fmt.Errorf("resegment(how, segments, index, before)")
	}
	var segs []project.Segment
	if err := json.Unmarshal([]byte(args[1].String()), &segs); err != nil {
		return nil, fmt.Errorf("segments: %w", err)
	}
	i := args[2].Int()
	var out []project.Segment
	var err error
	switch args[0].String() {
	case "split":
		var before content.Content
		if err := json.Unmarshal([]byte(args[3].String()), &before); err != nil {
			return nil, fmt.Errorf("before: %w", err)
		}
		out, err = project.Split(segs, i, before)
	case "join":
		out, err = project.Join(segs, i)
	default:
		return nil, fmt.Errorf("resegment: unknown way %q", args[0].String())
	}
	if err != nil {
		return nil, err
	}
	return jsonOut(out)
}

// segmentation(segments, rules, lang): cuts a document's segments as the
// project's own segmentation rules say, by joining and splitting the ones
// the filter read. It returns the segments as they are then.
func segmentation(args []js.Value) (any, error) {
	if len(args) != 3 {
		return nil, fmt.Errorf("segmentation(segments, rules, lang)")
	}
	var segs []project.Segment
	if err := json.Unmarshal([]byte(args[0].String()), &segs); err != nil {
		return nil, fmt.Errorf("segments: %w", err)
	}
	var rules []project.SegmentationRule
	if err := json.Unmarshal([]byte(args[1].String()), &rules); err != nil {
		return nil, fmt.Errorf("rules: %w", err)
	}
	return jsonOut(project.ApplySegmentation(segs, rules, args[2].String()))
}

// review(files): the scored review of files, [{name, lang, segments}]:
// each file's score from the issues marked on its segments, and the
// total. reviewCSV(files) lists the issues as a CSV table.
func review(args []js.Value) (any, error) {
	files, err := reviewedFiles(args)
	if err != nil {
		return nil, err
	}
	return jsonOut(project.ReviewOf(files))
}

func reviewCSV(args []js.Value) (any, error) {
	files, err := reviewedFiles(args)
	if err != nil {
		return nil, err
	}
	return string(project.ReviewCSV(files)), nil
}

func reviewedFiles(args []js.Value) ([]project.ReviewedFile, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("review(files)")
	}
	var files []project.ReviewedFile
	if err := json.Unmarshal([]byte(args[0].String()), &files); err != nil {
		return nil, fmt.Errorf("files: %w", err)
	}
	return files, nil
}

func projectFuncs() map[string]any {
	return map[string]any{
		"projectDefaults":         guard(projectDefaults),
		"projectValidate":         guard(projectValidate),
		"projectTemplateValidate": guard(projectTemplateValidate),
		"analyze":                 guard(analyze),
		"pretranslate":            guard(pretranslate),
		"resegment":               guard(resegment),
		"segmentation":            guard(segmentation),
		"review":                  guard(review),
		"reviewCSV":               guard(reviewCSV),
	}
}
