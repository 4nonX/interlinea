package server

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/4nonX/interlinea/content"
	"github.com/4nonX/interlinea/project"
	"github.com/4nonX/interlinea/project/projectdb"
)

type mcpAnswer struct {
	ID     int `json:"id"`
	Result struct {
		ProtocolVersion string `json:"protocolVersion"`
		Tools           []struct {
			Name        string         `json:"name"`
			InputSchema map[string]any `json:"inputSchema"`
		} `json:"tools"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	} `json:"result"`
	Error *rpcError `json:"error"`
}

func TestMCP(t *testing.T) {
	te := newTeam(t)
	req := leaseRequest()
	req.Files = []fileUpload{{Name: "Hinweise.md", Role: project.Translatable, Data: []byte("Der **Mieter** zahlt die Miete.\n\nDie Kaution beträgt drei Monatsmieten.\n")}}
	var p projectView
	te.call("POST", "/api/projects", te.pm, req, 201, &p)

	n := 0
	rpc := func(token, method string, params any) mcpAnswer {
		t.Helper()
		n++
		var a mcpAnswer
		te.call("POST", "/mcp", token, map[string]any{"jsonrpc": "2.0", "id": n, "method": method, "params": params}, 200, &a)
		if a.ID != n {
			t.Fatalf("%s answered request %d with %d", method, n, a.ID)
		}
		return a
	}
	// tool calls a tool and decodes the JSON in its answer.
	tool := func(token, name string, args map[string]any, out any) (string, bool) {
		t.Helper()
		a := rpc(token, "tools/call", map[string]any{"name": name, "arguments": args})
		if a.Error != nil || len(a.Result.Content) != 1 || a.Result.Content[0].Type != "text" {
			t.Fatalf("%s: %+v", name, a)
		}
		if out != nil && !a.Result.IsError {
			if err := json.Unmarshal([]byte(a.Result.Content[0].Text), out); err != nil {
				t.Fatalf("%s answered %s", name, a.Result.Content[0].Text)
			}
		}
		return a.Result.Content[0].Text, a.Result.IsError
	}

	// Without signing in there is no endpoint to speak to.
	if r := te.do("POST", "/mcp", "", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "ping"}); r.status != 401 {
		t.Fatalf("not signed in: %d", r.status)
	}
	if a := rpc(te.pm, "initialize", map[string]any{"protocolVersion": "2025-03-26", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "test"}}); a.Result.ProtocolVersion != "2025-03-26" {
		t.Fatalf("initialize %+v", a)
	}
	if a := rpc(te.pm, "initialize", map[string]any{"protocolVersion": "1999-01-01"}); a.Result.ProtocolVersion != mcpVersions[0] {
		t.Fatalf("an unknown version gets %q", a.Result.ProtocolVersion)
	}
	// A notification gets no answer.
	if r := te.do("POST", "/mcp", te.pm, map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"}); r.status != 202 || len(r.body) != 0 {
		t.Fatalf("notification: %d %s", r.status, r.body)
	}
	if a := rpc(te.pm, "ping", nil); a.Error != nil {
		t.Fatalf("ping %+v", a)
	}
	if a := rpc(te.pm, "resources/list", nil); a.Error == nil || a.Error.Code != -32601 {
		t.Fatalf("an unknown method: %+v", a)
	}
	list := rpc(te.pm, "tools/list", nil)
	if len(list.Result.Tools) != len(mcpToolFuncs) {
		t.Fatalf("%d tools listed, %d there", len(list.Result.Tools), len(mcpToolFuncs))
	}
	for _, tl := range list.Result.Tools {
		if _, ok := mcpToolFuncs[tl.Name]; !ok || tl.InputSchema["type"] != "object" {
			t.Errorf("tool %+v", tl)
		}
	}

	var projects []struct {
		Project int64    `json:"project"`
		Name    string   `json:"name"`
		Files   []string `json:"files"`
	}
	tool(te.pm, "list_projects", nil, &projects)
	if len(projects) != 1 || projects[0].Project != p.ID || projects[0].Name != "Mietvertrag Hofmann" || len(projects[0].Files) != 1 {
		t.Fatalf("projects %+v", projects)
	}
	// Someone without the project sees none, and can't read its segments.
	tool(te.ben, "list_projects", nil, &projects)
	if len(projects) != 0 {
		t.Fatalf("a stranger's projects %+v", projects)
	}
	at := map[string]any{"project": p.ID, "file": "Hinweise.md", "lang": "en-GB"}
	if text, isErr := tool(te.ben, "list_segments", at, nil); !isErr || !strings.Contains(text, "doesn't exist") {
		t.Fatalf("a stranger lists segments: %s", text)
	}

	var segs struct {
		Total    int          `json:"total"`
		Segments []mcpSegment `json:"segments"`
	}
	tool(te.pm, "list_segments", at, &segs)
	if segs.Total != 2 || segs.Segments[0].Source != "Der <1>Mieter</1> zahlt die Miete." || segs.Segments[0].Status != "new" {
		t.Fatalf("segments %+v", segs)
	}
	first := segs.Segments[0].ID
	with := func(extra map[string]any) map[string]any {
		out := map[string]any{}
		for k, v := range at {
			out[k] = v
		}
		for k, v := range extra {
			out[k] = v
		}
		return out
	}
	// A tag the source does not have is refused, and says which tags there are.
	if text, isErr := tool(te.pm, "save_translation", with(map[string]any{"segment": first, "translation": "The <2>tenant</2> pays."}), nil); !isErr || !strings.Contains(text, "<2>") || !strings.Contains(text, "Der <1>Mieter</1>") {
		t.Fatalf("an unknown tag: %s", text)
	}
	var saved mcpSegment
	tool(te.pm, "save_translation", with(map[string]any{"segment": first, "translation": "The <1>tenant</1> pays the rent &amp; more."}), &saved)
	if saved.Status != "draft" || saved.Translation != "The <1>tenant</1> pays the rent &amp; more." {
		t.Fatalf("saved %+v", saved)
	}
	tool(te.pm, "save_translation", with(map[string]any{"segment": first, "translation": "The <1>tenant</1> pays the rent.", "confirm": true}), &saved)
	if saved.Status != "confirmed" {
		t.Fatalf("confirmed %+v", saved)
	}
	var stored []projectdb.StoredSegment
	te.call("GET", fmt.Sprintf("/api/projects/%d/segments?file=Hinweise.md&lang=en-GB", p.ID), te.pm, nil, 200, &stored)
	if stored[0].Target.Plain() != "The tenant pays the rent." || len(stored[0].Target.Tags()) != 2 || stored[0].Status != project.StatusConfirmed || stored[0].ChangedBy != "paula" {
		t.Fatalf("stored %+v", stored[0])
	}
	if text, isErr := tool(te.pm, "save_translation", with(map[string]any{"segment": "nope", "translation": "x"}), nil); !isErr || !strings.Contains(text, "no segment") {
		t.Fatalf("no such segment: %s", text)
	}
	tool(te.pm, "list_segments", with(map[string]any{"status": "open", "limit": 1}), &segs)
	if segs.Total != 1 || len(segs.Segments) != 1 || segs.Segments[0].ID == first {
		t.Fatalf("open segments %+v", segs)
	}

	// The confirmed translation is in the project's memory and found there.
	var matches []struct {
		Score       int    `json:"score"`
		Translation string `json:"translation"`
	}
	tool(te.pm, "search_memory", map[string]any{"project": p.ID, "lang": "en-GB", "text": "Der Mieter zahlt die Miete."}, &matches)
	if len(matches) == 0 || matches[0].Score < 95 || !strings.Contains(matches[0].Translation, "tenant") {
		t.Fatalf("memory %+v", matches)
	}
	var hits []map[string]any
	tool(te.pm, "search_concordance", map[string]any{"project": p.ID, "lang": "en-GB", "query": "tenant", "in_translations": true}, &hits)
	if len(hits) == 0 {
		t.Fatal("concordance finds nothing")
	}
	tool(te.pm, "find_terms", map[string]any{"project": p.ID, "lang": "en-GB", "text": "Der Mieter zahlt die Miete."}, &hits)
	var done map[string]any
	tool(te.pm, "add_comment", with(map[string]any{"segment": first, "text": "Checked against the glossary."}), &done)
	te.call("GET", fmt.Sprintf("/api/projects/%d/segments?file=Hinweise.md&lang=en-GB", p.ID), te.pm, nil, 200, &stored)
	if len(stored[0].Comments) != 1 || stored[0].Comments[0].Text != "Checked against the glossary." {
		t.Fatalf("comments %+v", stored[0].Comments)
	}
	if a := rpc(te.pm, "tools/call", map[string]any{"name": "delete_everything"}); a.Error == nil {
		t.Fatal("a tool that is not there is called")
	}

	// An access token made for a program works on the endpoint, and is refused once it is ended.
	var made struct {
		Token string `json:"token"`
		ID    int64  `json:"id"`
		Name  string `json:"name"`
	}
	te.call("POST", "/api/me/tokens", te.pm, map[string]any{"name": "My assistant", "days": 30}, 201, &made)
	if made.Token == "" || made.Name != "My assistant" {
		t.Fatalf("token %+v", made)
	}
	tool(made.Token, "list_projects", nil, &projects)
	if len(projects) != 1 {
		t.Fatalf("with the access token: %+v", projects)
	}
	for _, bad := range []map[string]any{{"name": " "}, {"name": "x", "days": 4000}} {
		if r := te.do("POST", "/api/me/tokens", te.pm, bad); r.status != 422 {
			t.Fatalf("token %v: %d", bad, r.status)
		}
	}
	te.call("DELETE", fmt.Sprintf("/api/me/tokens/%d", made.ID), te.pm, nil, 204, nil)
	if r := te.do("POST", "/mcp", made.Token, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "ping"}); r.status != 401 {
		t.Fatalf("an ended token: %d", r.status)
	}
}

func TestMarkup(t *testing.T) {
	src := content.Text("A ").AppendTag(content.Tag{Kind: content.TagOpen, ID: "1"}).Append("b < c").AppendTag(content.Tag{Kind: content.TagClose, ID: "1"}).AppendTag(content.Tag{Kind: content.TagStandalone, ID: "2"}).Append(" & d")
	text := markup(src)
	if text != "A <1>b &lt; c</1><2/> &amp; d" {
		t.Fatal(text)
	}
	back, err := parseMarkup(text, src)
	if err != nil || back.Plain() != src.Plain() || len(back.Tags()) != 3 {
		t.Fatalf("%+v %v", back, err)
	}
	for _, bad := range []string{"a <3/> b", "a <1/> b", "a < b"} {
		if _, err := parseMarkup(bad, src); err == nil {
			t.Errorf("%q is read", bad)
		}
	}
}
