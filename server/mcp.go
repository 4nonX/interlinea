package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/4nonX/interlinea/content"
)

// The Model Context Protocol endpoint: an AI assistant, or any other
// program that speaks the protocol, works on the server's projects through
// it, as the person whose token it carries and with no more rights than
// that person has. It is the HTTP transport of the protocol, answering
// each request with one JSON message; the tools call the API's own
// handlers, so everything the API checks is checked here too.

func (s *Server) mcpRoutes() {
	s.handle("POST /mcp", s.signedIn(s.mcp))
	s.handle("GET /mcp", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Allow", "POST")
		writeError(w, http.StatusMethodNotAllowed, "badRequest", "This endpoint answers each POST with one message and keeps no stream open.")
	})
	s.handle("POST /api/me/tokens", s.signedIn(s.createOwnToken))
}

// mcpVersions are the versions of the protocol the endpoint speaks, the
// newest first.
var mcpVersions = []string{"2025-06-18", "2025-03-26", "2024-11-05"}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// mcp answers one message of the Model Context Protocol (JSON-RPC 2.0):
// initialize, ping, tools/list and tools/call. A notification is taken
// with 202 and no body.
//
// Access: signed in; each tool needs what its API call needs.
func (s *Server) mcp(w http.ResponseWriter, r *http.Request, me session) {
	var req rpcRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	if err := dec.Decode(&req); err != nil || req.JSONRPC != "2.0" || req.Method == "" {
		writeJSON(w, http.StatusBadRequest, rpcResponse{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{-32700, "Send one JSON-RPC 2.0 request."}})
		return
	}
	if len(req.ID) == 0 {
		// A notification: nothing to answer.
		w.WriteHeader(http.StatusAccepted)
		return
	}
	res := rpcResponse{JSONRPC: "2.0", ID: req.ID}
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &p)
		version := mcpVersions[0]
		for _, v := range mcpVersions {
			if v == p.ProtocolVersion {
				version = v
			}
		}
		res.Result = map[string]any{
			"protocolVersion": version,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "interlinea", "title": "Interlinea", "version": s.cfg.Version},
			"instructions": "Interlinea is a translation tool. Segments are sentences of a document with their translations. " +
				"In a segment's text, <1>…</1> and <2/> stand for formatting and placeholders of the document: keep them in a translation, each as often as the source has it, around or at the words they belong to. " +
				"Look up the memory and the terms of a project before translating a segment, and use the terms the termbase gives. " +
				"Save a translation as a draft unless the person asked for it to be confirmed.",
		}
	case "ping":
		res.Result = map[string]any{}
	case "tools/list":
		res.Result = map[string]any{"tools": mcpTools}
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			res.Error = &rpcError{-32602, "The call needs a tool's name and its arguments."}
			break
		}
		tool, ok := mcpToolFuncs[p.Name]
		if !ok {
			res.Error = &rpcError{-32602, fmt.Sprintf("There is no tool %q.", p.Name)}
			break
		}
		args := map[string]any{}
		if len(p.Arguments) > 0 {
			if err := json.Unmarshal(p.Arguments, &args); err != nil {
				res.Error = &rpcError{-32602, "The arguments are not a JSON object."}
				break
			}
		}
		out, err := tool(&mcpCall{s: s, r: r, args: args})
		if err != nil {
			// What a tool could not do is told to the assistant, which
			// can then do something about it.
			res.Result = map[string]any{"content": []any{map[string]any{"type": "text", "text": err.Error()}}, "isError": true}
			break
		}
		text, _ := json.MarshalIndent(out, "", "  ")
		res.Result = map[string]any{"content": []any{map[string]any{"type": "text", "text": string(text)}}}
	default:
		res.Error = &rpcError{-32601, fmt.Sprintf("The method %q is not supported.", req.Method)}
	}
	writeJSON(w, http.StatusOK, res)
}

type mcpTool struct {
	Name        string         `json:"name"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

func schema(required []string, props map[string]any) map[string]any {
	return map[string]any{"type": "object", "properties": props, "required": required, "additionalProperties": false}
}

func prop(kind, description string) map[string]any {
	return map[string]any{"type": kind, "description": description}
}

var (
	propProject = prop("integer", "The project's number, as list_projects gives it.")
	propFile    = prop("string", "The file's name in the project.")
	propLang    = prop("string", "The target language, as a tag such as en-GB.")
	propSegment = prop("string", "The segment's ID, as list_segments gives it.")
)

var mcpTools = []mcpTool{
	{"list_projects", "List projects", "The projects the person may see: number, name, languages, and the files to translate.", schema([]string{}, map[string]any{})},
	{"list_segments", "List segments", "Segments of a file in a target language, in the order of the document: ID, source, translation, status (new, draft, confirmed), whether it is locked, the file's notes and people's comments. Tags are written <1>…</1> and <2/>.",
		schema([]string{"project", "file", "lang"}, map[string]any{
			"project": propProject, "file": propFile, "lang": propLang,
			"status": map[string]any{"type": "string", "enum": []string{"new", "draft", "confirmed", "open"}, "description": "Only segments of this status; open is new and draft."},
			"offset": prop("integer", "How many segments to skip (default 0)."),
			"limit":  prop("integer", "How many segments to give (default 50, at most 200)."),
		})},
	{"save_translation", "Save a translation", "Saves the translation of a segment, as a draft or confirmed. Keep the source's tags (<1>…</1>, <2/>) in the translation. A confirmed translation goes into the project's memories.",
		schema([]string{"project", "file", "lang", "segment", "translation"}, map[string]any{
			"project": propProject, "file": propFile, "lang": propLang, "segment": propSegment,
			"translation": prop("string", "The translation, with the source's tags."),
			"confirm":     prop("boolean", "Confirm the translation (default false: it is saved as a draft)."),
		})},
	{"search_memory", "Search the memory", "Earlier translations of a sentence like this one from the project's memories, best first, with how alike they are in percent.",
		schema([]string{"project", "lang", "text"}, map[string]any{
			"project": propProject, "lang": propLang, "text": prop("string", "The sentence to find matches for, in the source language."),
			"min_score": prop("integer", "The least likeness in percent (default 70)."),
		})},
	{"search_concordance", "Search for words in the memory", "Units of the project's memories that hold the words, in the source or in the translations: how a word or phrase was translated before.",
		schema([]string{"project", "lang", "query"}, map[string]any{
			"project": propProject, "lang": propLang, "query": prop("string", "The words to look for."),
			"in_translations": prop("boolean", "Look in the translations instead of the source (default false)."),
		})},
	{"find_terms", "Find terms", "The terms of the project's termbases found in a source text, with their translations and status (approved, proposed, forbidden). Use approved terms; do not use forbidden ones.",
		schema([]string{"project", "lang", "text"}, map[string]any{"project": propProject, "lang": propLang, "text": prop("string", "The source text to find terms in.")})},
	{"add_comment", "Comment on a segment", "Leaves a comment on a segment for the people working on the project, such as a question or a doubt. It does not change the translation.",
		schema([]string{"project", "file", "lang", "segment", "text"}, map[string]any{"project": propProject, "file": propFile, "lang": propLang, "segment": propSegment, "text": prop("string", "The comment.")})},
}

type mcpCall struct {
	s    *Server
	r    *http.Request
	args map[string]any
}

func (c *mcpCall) str(name string) string {
	v, _ := c.args[name].(string)
	return v
}

func (c *mcpCall) num(name string, def int) int {
	switch v := c.args[name].(type) {
	case float64:
		return int(v)
	case string:
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func (c *mcpCall) flag(name string) bool {
	v, _ := c.args[name].(bool)
	return v
}

// api calls one of the server's own API handlers as the person the
// request is signed in as, and decodes its answer.
func (c *mcpCall) api(method, path string, query url.Values, body, out any) error {
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return err
		}
	}
	u := path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(c.r.Context(), method, u, &buf)
	if err != nil {
		return err
	}
	for _, h := range []string{"Authorization", "Cookie", cookieHeader} {
		if v := c.r.Header.Get(h); v != "" {
			req.Header.Set(h, v)
		}
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := &recorder{header: http.Header{}, status: http.StatusOK}
	c.s.mux.ServeHTTP(rec, req)
	if rec.status >= 400 {
		var e apiError
		if json.Unmarshal(rec.body.Bytes(), &e) == nil && e.Message != "" {
			return fmt.Errorf("%s", e.Message)
		}
		return fmt.Errorf("the server answered %d", rec.status)
	}
	if out == nil || rec.body.Len() == 0 {
		return nil
	}
	return json.Unmarshal(rec.body.Bytes(), out)
}

type recorder struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (r *recorder) Header() http.Header         { return r.header }
func (r *recorder) WriteHeader(status int)      { r.status = status }
func (r *recorder) Write(b []byte) (int, error) { return r.body.Write(b) }

func (c *mcpCall) project() string { return "/api/projects/" + strconv.Itoa(c.num("project", 0)) }

// markup writes content as text an assistant reads: a pair of tags as
// <1>…</1>, a tag on its own as <2/>.
func markup(c content.Content) string {
	var b strings.Builder
	for _, r := range c {
		switch {
		case r.Tag == nil:
			b.WriteString(strings.NewReplacer("&", "&amp;", "<", "&lt;").Replace(r.Text))
		case r.Tag.Kind == content.TagOpen:
			b.WriteString("<" + r.Tag.ID + ">")
		case r.Tag.Kind == content.TagClose:
			b.WriteString("</" + r.Tag.ID + ">")
		default:
			b.WriteString("<" + r.Tag.ID + "/>")
		}
	}
	return b.String()
}

// parseMarkup reads what markup writes. A tag the source does not have
// is refused, so a translation can't bring in formatting of its own.
func parseMarkup(text string, source content.Content) (content.Content, error) {
	known := map[string]bool{}
	for _, t := range source.Tags() {
		known[t.Kind.String()+":"+t.ID] = true
	}
	unescape := strings.NewReplacer("&lt;", "<", "&amp;", "&")
	var c content.Content
	for text != "" {
		lt := strings.IndexByte(text, '<')
		if lt < 0 {
			c = c.Append(unescape.Replace(text))
			break
		}
		gt := strings.IndexByte(text[lt:], '>')
		if gt < 0 {
			return nil, fmt.Errorf("the translation has a < that no > closes; write a < that is text as &lt;")
		}
		c = c.Append(unescape.Replace(text[:lt]))
		tag := text[lt+1 : lt+gt]
		text = text[lt+gt+1:]
		kind := content.TagOpen
		switch {
		case strings.HasPrefix(tag, "/"):
			kind, tag = content.TagClose, tag[1:]
		case strings.HasSuffix(tag, "/"):
			kind, tag = content.TagStandalone, tag[:len(tag)-1]
		}
		if !known[kind.String()+":"+tag] {
			return nil, fmt.Errorf("the translation has the tag <%s>, which the source does not have; its tags are those in %q", tag, markup(source))
		}
		c = c.AppendTag(content.Tag{Kind: kind, ID: tag})
	}
	if c == nil {
		c = content.Content{}
	}
	return c, nil
}

type mcpSegment struct {
	ID          string   `json:"id"`
	Source      string   `json:"source"`
	Translation string   `json:"translation"`
	Status      string   `json:"status"`
	Locked      bool     `json:"locked,omitempty"`
	Notes       []string `json:"notes,omitempty"`
	Comments    []string `json:"comments,omitempty"`
}

type mcpStored struct {
	ID       string          `json:"id"`
	Source   content.Content `json:"source"`
	Target   content.Content `json:"target"`
	Status   string          `json:"status"`
	Locked   bool            `json:"locked"`
	Version  int             `json:"version"`
	Notes    []string        `json:"notes"`
	Comments []struct {
		Text string `json:"text"`
		By   string `json:"by"`
	} `json:"comments"`
}

func (c *mcpCall) segments() ([]mcpStored, error) {
	var segs []mcpStored
	err := c.api("GET", c.project()+"/segments", url.Values{"file": {c.str("file")}, "lang": {c.str("lang")}}, nil, &segs)
	return segs, err
}

var mcpToolFuncs = map[string]func(*mcpCall) (any, error){
	"list_projects": func(c *mcpCall) (any, error) {
		var ps []struct {
			ID       int64 `json:"id"`
			Settings struct {
				Name        string   `json:"name"`
				Client      string   `json:"client"`
				SourceLang  string   `json:"sourceLang"`
				TargetLangs []string `json:"targetLangs"`
				Due         string   `json:"due"`
				Files       []struct {
					Name string `json:"name"`
					Role string `json:"role"`
				} `json:"files"`
			} `json:"settings"`
		}
		if err := c.api("GET", "/api/projects", nil, nil, &ps); err != nil {
			return nil, err
		}
		out := []map[string]any{}
		for _, p := range ps {
			files := []string{}
			for _, f := range p.Settings.Files {
				if f.Role == "translatable" {
					files = append(files, f.Name)
				}
			}
			out = append(out, map[string]any{"project": p.ID, "name": p.Settings.Name, "client": p.Settings.Client, "sourceLang": p.Settings.SourceLang, "targetLangs": p.Settings.TargetLangs, "files": files})
		}
		return out, nil
	},
	"list_segments": func(c *mcpCall) (any, error) {
		segs, err := c.segments()
		if err != nil {
			return nil, err
		}
		want := c.str("status")
		var all []mcpSegment
		for _, s := range segs {
			if want != "" && s.Status != want && !(want == "open" && s.Status != "confirmed") {
				continue
			}
			m := mcpSegment{ID: s.ID, Source: markup(s.Source), Translation: markup(s.Target), Status: s.Status, Locked: s.Locked, Notes: s.Notes}
			for _, cm := range s.Comments {
				m.Comments = append(m.Comments, cm.By+": "+cm.Text)
			}
			all = append(all, m)
		}
		offset, limit := max(0, c.num("offset", 0)), c.num("limit", 50)
		if limit < 1 || limit > 200 {
			limit = 50
		}
		total := len(all)
		all = all[min(offset, total):min(offset+limit, total)]
		if all == nil {
			all = []mcpSegment{}
		}
		return map[string]any{"total": total, "offset": offset, "segments": all}, nil
	},
	"save_translation": func(c *mcpCall) (any, error) {
		segs, err := c.segments()
		if err != nil {
			return nil, err
		}
		for _, s := range segs {
			if s.ID != c.str("segment") {
				continue
			}
			target, err := parseMarkup(c.str("translation"), s.Source)
			if err != nil {
				return nil, err
			}
			status := "draft"
			switch {
			case c.flag("confirm"):
				status = "confirmed"
			case strings.TrimSpace(target.Plain()) == "" && !target.HasTags():
				status = "new"
			}
			var saved mcpStored
			q := url.Values{"file": {c.str("file")}, "lang": {c.str("lang")}, "segment": {s.ID}, "version": {strconv.Itoa(s.Version)}}
			if err := c.api("PUT", c.project()+"/segments", q, map[string]any{"target": target, "status": status}, &saved); err != nil {
				return nil, err
			}
			return mcpSegment{ID: saved.ID, Source: markup(saved.Source), Translation: markup(saved.Target), Status: saved.Status}, nil
		}
		return nil, fmt.Errorf("the file has no segment %q", c.str("segment"))
	},
	"search_memory": func(c *mcpCall) (any, error) {
		var ms []struct {
			Score  int    `json:"score"`
			Memory string `json:"memory"`
			Unit   struct {
				Source content.Content `json:"source"`
				Target content.Content `json:"target"`
			} `json:"unit"`
		}
		body := map[string]any{"lang": c.str("lang"), "source": content.Text(c.str("text")), "minScore": c.num("min_score", 70), "limit": 8}
		if err := c.api("POST", c.project()+"/lookup", nil, body, &ms); err != nil {
			return nil, err
		}
		out := []map[string]any{}
		for _, m := range ms {
			out = append(out, map[string]any{"score": m.Score, "memory": m.Memory, "source": m.Unit.Source.Plain(), "translation": m.Unit.Target.Plain()})
		}
		return out, nil
	},
	"search_concordance": func(c *mcpCall) (any, error) {
		var hs []struct {
			Memory string `json:"memory"`
			Unit   struct {
				Source content.Content `json:"source"`
				Target content.Content `json:"target"`
			} `json:"unit"`
		}
		body := map[string]any{"lang": c.str("lang"), "query": c.str("query"), "inTarget": c.flag("in_translations"), "limit": 15}
		if err := c.api("POST", c.project()+"/concordance", nil, body, &hs); err != nil {
			return nil, err
		}
		out := []map[string]any{}
		for _, h := range hs {
			out = append(out, map[string]any{"memory": h.Memory, "source": h.Unit.Source.Plain(), "translation": h.Unit.Target.Plain()})
		}
		return out, nil
	},
	"find_terms": func(c *mcpCall) (any, error) {
		var hits []map[string]any
		if err := c.api("POST", c.project()+"/recognize", nil, map[string]any{"lang": c.str("lang"), "text": c.str("text")}, &hits); err != nil {
			return nil, err
		}
		if hits == nil {
			hits = []map[string]any{}
		}
		return hits, nil
	},
	"add_comment": func(c *mcpCall) (any, error) {
		q := url.Values{"file": {c.str("file")}, "lang": {c.str("lang")}, "segment": {c.str("segment")}}
		if err := c.api("POST", c.project()+"/comments", q, map[string]any{"text": c.str("text")}, nil); err != nil {
			return nil, err
		}
		return map[string]any{"commented": true}, nil
	},
}

// maxTokenDays is the longest an access token made for a program lasts.
const maxTokenDays = 366

// createOwnToken makes an access token for a program that works as the
// signed-in person: an assistant on the MCP endpoint, a script on the
// API. The token is in the answer once and can't be read again; it is
// listed with the person's sign-ins and ended there.
//
// Access: signed in.
func (s *Server) createOwnToken(w http.ResponseWriter, r *http.Request, me session) {
	var req struct {
		Name string `json:"name"`
		// Days is how long the token lasts (default 90, at most 366).
		Days int `json:"days"`
	}
	if !readJSON(w, r, &req, maxBody) {
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		invalid(w, "name", "Name the program the token is for.")
		return
	}
	if req.Days == 0 {
		req.Days = 90
	}
	if req.Days < 1 || req.Days > maxTokenDays {
		invalid(w, "days", fmt.Sprintf("A token lasts between 1 and %d days.", maxTokenDays))
		return
	}
	secret, t, err := s.cfg.Auth.IssueToken(r.Context(), me.User.ID, name, time.Duration(req.Days)*24*time.Hour)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"token": secret, "id": t.ID, "name": t.Name, "expiresAt": t.ExpiresAt})
}
