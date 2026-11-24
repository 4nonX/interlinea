//go:build js && wasm

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"syscall/js"
	"time"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/tm"
)

// Translation memories live in the page while it is open, keyed by a name
// the page chooses. They are the core's in-memory backend, so scores and
// concordance behave exactly as in the desktop app.
var (
	memMu    sync.Mutex
	memories = map[string]*tm.InMemory{}
)

type jsUnit struct {
	ID         int64             `json:"id,omitempty"`
	Source     content.Content   `json:"source"`
	Target     content.Content   `json:"target"`
	CreatedBy  string            `json:"createdBy,omitempty"`
	CreatedAt  string            `json:"createdAt,omitempty"`
	ChangedBy  string            `json:"changedBy,omitempty"`
	ChangedAt  string            `json:"changedAt,omitempty"`
	Attributes map[string]string `json:"attributes,omitempty"`
	Score      int               `json:"score,omitempty"`
}

func toJS(u tm.Unit, score int) jsUnit {
	out := jsUnit{ID: u.ID, Source: u.Source, Target: u.Target, CreatedBy: u.CreatedBy, Score: score}
	if !u.CreatedAt.IsZero() {
		out.CreatedAt = u.CreatedAt.UTC().Format(time.RFC3339)
	}
	out.ChangedBy = u.ChangedBy
	if !u.ChangedAt.IsZero() {
		out.ChangedAt = u.ChangedAt.UTC().Format(time.RFC3339)
	}
	if len(u.Attributes) > 0 {
		out.Attributes = map[string]string{}
		for _, a := range u.Attributes {
			out.Attributes[a.Name] = a.Value
		}
	}
	return out
}

func memory(name string) (*tm.InMemory, error) {
	memMu.Lock()
	defer memMu.Unlock()
	m, ok := memories[name]
	if !ok {
		return nil, fmt.Errorf("no memory %q", name)
	}
	return m, nil
}

func guard(fn func(args []js.Value) (any, error)) js.Func {
	return js.FuncOf(func(_ js.Value, args []js.Value) (out any) {
		defer func() {
			if r := recover(); r != nil {
				out = result(nil, fmt.Errorf("internal error: %v", r))
			}
		}()
		v, err := fn(args)
		return result(v, err)
	})
}

func jsonOut(v any) (any, error) {
	b, err := json.Marshal(v)
	return string(b), err
}

// tmOpen(name, sourceLang, targetLang): creates or empties a memory.
func tmOpen(args []js.Value) (any, error) {
	if len(args) != 3 {
		return nil, fmt.Errorf("tmOpen(name, source, target)")
	}
	m, err := tm.NewInMemory(tm.Info{Name: args[0].String(), SourceLang: args[1].String(), TargetLang: args[2].String()})
	if err != nil {
		return nil, err
	}
	memMu.Lock()
	memories[args[0].String()] = m
	memMu.Unlock()
	return true, nil
}

// tmAdd(name, unitsJSON, overwrite?): adds units ([{source, target,
// createdBy, createdAt, attributes}]). With overwrite, a unit replaces the
// other translations of its source; without, they are kept beside it.
func tmAdd(args []js.Value) (any, error) {
	if len(args) != 2 && len(args) != 3 {
		return nil, fmt.Errorf("tmAdd(name, units, overwrite)")
	}
	policy := tm.KeepBoth
	if len(args) == 3 && args[2].Bool() {
		policy = tm.Overwrite
	}
	m, err := memory(args[0].String())
	if err != nil {
		return nil, err
	}
	var in []jsUnit
	if err := json.Unmarshal([]byte(args[1].String()), &in); err != nil {
		return nil, err
	}
	n := 0
	for _, u := range in {
		tu := tm.Unit{Source: u.Source, Target: u.Target, CreatedBy: u.CreatedBy}
		if t, err := time.Parse(time.RFC3339, u.CreatedAt); err == nil {
			tu.CreatedAt = t
		}
		for k, v := range u.Attributes {
			tu.Attributes = append(tu.Attributes, tm.Attribute{Name: k, Value: v})
		}
		tu.ChangedBy = u.CreatedBy
		if _, err := m.Add(context.Background(), tu, policy); err != nil {
			return nil, err
		}
		n++
	}
	return n, nil
}

// tmLookup(name, sourceJSON, minScore, limit, penalty?) -> JSON [{score,
// unit...}]; with a penalty, scores are lowered by it (tm.Penalized).
func tmLookup(args []js.Value) (any, error) {
	if len(args) != 4 && len(args) != 5 {
		return nil, fmt.Errorf("tmLookup(name, source, min, limit, penalty)")
	}
	mm, err := memory(args[0].String())
	if err != nil {
		return nil, err
	}
	var m tm.Memory = mm
	if len(args) == 5 {
		if m, err = tm.Penalized(mm, args[4].Int()); err != nil {
			return nil, err
		}
	}
	var src content.Content
	if err := src.UnmarshalJSON([]byte(args[1].String())); err != nil {
		return nil, err
	}
	ms, err := m.Lookup(context.Background(), src, tm.LookupOptions{MinScore: args[2].Int(), Limit: args[3].Int()})
	if err != nil {
		return nil, err
	}
	out := make([]jsUnit, 0, len(ms))
	for _, x := range ms {
		out = append(out, toJS(x.Unit, x.Score))
	}
	return jsonOut(out)
}

// tmConcordance(name, query, inTarget, limit)
func tmConcordance(args []js.Value) (any, error) {
	if len(args) != 4 {
		return nil, fmt.Errorf("tmConcordance(name, query, inTarget, limit)")
	}
	m, err := memory(args[0].String())
	if err != nil {
		return nil, err
	}
	us, err := m.Concordance(context.Background(), args[1].String(), tm.ConcordanceOptions{InTarget: args[2].Bool(), Limit: args[3].Int()})
	if err != nil {
		return nil, err
	}
	out := make([]jsUnit, 0, len(us))
	for _, u := range us {
		out = append(out, toJS(u, 0))
	}
	return jsonOut(out)
}

// tmSetTarget(name, id, targetJSON, by) -> JSON unit: changes a unit's
// translation.
func tmSetTarget(args []js.Value) (any, error) {
	if len(args) != 4 {
		return nil, fmt.Errorf("tmSetTarget(name, id, target, by)")
	}
	m, err := memory(args[0].String())
	if err != nil {
		return nil, err
	}
	var target content.Content
	if err := target.UnmarshalJSON([]byte(args[2].String())); err != nil {
		return nil, err
	}
	u, err := m.SetTarget(context.Background(), int64(args[1].Int()), target, args[3].String())
	if err != nil {
		return nil, err
	}
	return jsonOut(toJS(u, 0))
}

// tmDelete(name, id): deletes a unit.
func tmDelete(args []js.Value) (any, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("tmDelete(name, id)")
	}
	m, err := memory(args[0].String())
	if err != nil {
		return nil, err
	}
	return true, m.Delete(context.Background(), int64(args[1].Int()))
}

// tmCount(name)
func tmCount(args []js.Value) (any, error) {
	m, err := memory(args[0].String())
	if err != nil {
		return nil, err
	}
	return m.Count(context.Background())
}

// tmImportTMX(name, bytes) -> {read, stored, skipped}
func tmImportTMX(args []js.Value) (any, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("tmImportTMX(name, bytes)")
	}
	m, err := memory(args[0].String())
	if err != nil {
		return nil, err
	}
	data, err := bytesArg(args[1])
	if err != nil {
		return nil, err
	}
	st, err := tm.ImportTMX(context.Background(), m, bytes.NewReader(data), tm.ImportOptions{AllowPrimaryMatch: true})
	if err != nil {
		return nil, err
	}
	return jsonOut(map[string]int{"read": st.Read, "stored": st.Stored, "skipped": st.Skipped})
}

// tmExportTMX(name) -> Uint8Array
func tmExportTMX(args []js.Value) (any, error) {
	m, err := memory(args[0].String())
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := tm.ExportTMX(context.Background(), m, &buf, "interlinea-prototype"); err != nil {
		return nil, err
	}
	arr := js.Global().Get("Uint8Array").New(buf.Len())
	js.CopyBytesToJS(arr, buf.Bytes())
	return arr, nil
}

func tmFuncs() map[string]any {
	return map[string]any{
		"tmOpen":        guard(tmOpen),
		"tmAdd":         guard(tmAdd),
		"tmLookup":      guard(tmLookup),
		"tmConcordance": guard(tmConcordance),
		"tmCount":       guard(tmCount),
		"tmSetTarget":   guard(tmSetTarget),
		"tmDelete":      guard(tmDelete),
		"tmImportTMX":   guard(tmImportTMX),
		"tmExportTMX":   guard(tmExportTMX),
	}
}
