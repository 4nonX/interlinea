package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/types"
	"net/http"
	"regexp"
	"sort"
	"strings"
)

const intro = `The API of the Interlinea team server: accounts and roles, projects
and their files, segments and work, translation memories, termbases and the
review of their changes, and a live feed of changes.

Requests and answers are JSON (UTF-8), except file uploads and downloads,
which say their type. Every answer that is not a success has an Error body
with a stable code. Writes to things that have a version (projects, segments,
entries, units, accounts, templates) carry the version they were made from,
in If-Match or ?version=; a write made from an older version is refused with
409 conflict, and the app reloads.

Sign in with a password (/api/auth/password), through the identity provider
(/api/auth/oidc/start), or as the first administrator (/api/setup). The
answer's token goes in "Authorization: Bearer <token>". The web app the
server serves itself asks for a cookie instead (X-Interlinea-Session: cookie
on the sign-in): it is HttpOnly and SameSite=Strict, and every request it
signs in must carry the X-Interlinea header.

Roles on projects, memories and termbases are read < suggest < edit <
approve < manage, given per resource or for all of a kind; administrators
may do everything.

The desktop app's core, interlinea-core, serves the same API on the
person's own computer, for them alone: on the loopback interface, with one
account and a token it hands the app when it starts.`

// tagOf groups routes by their first path segment after /api/.
func tagOf(path string) string {
	p := strings.TrimPrefix(path, "/api/")
	p, _, _ = strings.Cut(p, "/")
	switch p {
	case "server", "setup", "auth", "openapi.json":
		return "Signing in"
	case "me":
		return "The signed-in person"
	case "people", "users", "grants":
		return "Accounts and roles"
	case "events":
		return "Live feed"
	case "memories":
		return "Memories"
	case "catalog", "projects", "templates":
		return "Projects"
	case "termbases":
		return "Termbases"
	case "merge-requests":
		return "Merge requests"
	case "files", "qa", "dictionaries":
		return "Files, checks and spelling"
	}
	return "Other"
}

var tagOrder = []string{"Signing in", "The signed-in person", "Accounts and roles", "Projects", "Files, checks and spelling", "Memories", "Termbases", "Merge requests", "Live feed", "Other"}

var pathParam = regexp.MustCompile(`\{(\w+)\}`)

// pathParamDoc says what each path parameter is.
var pathParamDoc = map[string]string{
	"id":    "The number of the account, sign-in, project or merge request.",
	"name":  "The name of the memory or termbase.",
	"uid":   "The number of the translation unit.",
	"eid":   "The entry's ID.",
	"aid":   "The number of the assignment.",
	"iid":   "The number of the merge request's item.",
	"lang":  "A language tag (BCP 47), such as de-DE.",
	"field": "The field's ID.",
}

// errorStatuses are the error answers routes may give besides 422, 500.
func errorStatuses(r *route) []int {
	var out []int
	if r.Access != "anyone" {
		out = append(out, 401, 403)
	}
	if strings.Contains(r.Path, "{") {
		out = append(out, 404)
	}
	if r.Version {
		out = append(out, 409, 428)
	}
	if r.Method != "GET" || len(r.Query) > 0 {
		out = append(out, 422)
	}
	return out
}

var errorText = map[int]string{
	401: "Not signed in, or the sign-in has ended (code signIn).",
	403: "The sign-in may not do this (code forbidden).",
	404: "It doesn't exist, or the sign-in may not see it (code notFound).",
	409: "It changed since the version sent (code conflict), or is in use.",
	422: "The request has problems (code invalid, with problems by field).",
	428: "The version is missing (If-Match or ?version=).",
}

// answers are a route's success answers by status.
func (a *api) answers(r *route, s *schemas) map[int]map[string]any {
	out := map[int]map[string]any{}
	for _, o := range r.Outs {
		st := o.Status
		if st == 0 {
			st = 200
		}
		out[st] = map[string]any{
			"description": http.StatusText(st),
			"content":     map[string]any{"application/json": map[string]any{"schema": s.of(o.Type)}},
		}
	}
	if len(r.ContentTypes) > 0 && len(r.Outs) == 0 {
		content := map[string]any{}
		for _, ct := range r.ContentTypes {
			if strings.HasPrefix(ct, "text/html") {
				continue
			}
			mt, _, _ := strings.Cut(ct, ";")
			sch := map[string]any{"type": "string"}
			if !strings.HasPrefix(mt, "text/") && mt != "application/json" && !strings.HasSuffix(mt, "+xml") {
				sch["format"] = "binary"
			}
			if mt == "application/json" {
				sch = map[string]any{}
			}
			content[mt] = map[string]any{"schema": sch}
		}
		if len(content) > 0 {
			out[200] = map[string]any{"description": http.StatusText(200), "content": content}
		}
	}
	for _, st := range r.Statuses {
		if st >= 400 {
			continue
		}
		if _, ok := out[st]; !ok {
			out[st] = map[string]any{"description": http.StatusText(st)}
		}
	}
	if r.Redirect {
		out[303] = map[string]any{"description": "The browser goes back to the app, with the result in the address's fragment."}
	}
	return out
}

func (a *api) openAPI() ([]byte, error) {
	s := newSchemas(a)
	errSchema := s.of(a.pkg.Scope().Lookup("apiError").Type())
	paths := map[string]any{}
	tags := map[string]bool{}
	for _, r := range a.routes {
		op := map[string]any{
			"operationId": r.Handler,
			"summary":     r.Summary,
			"tags":        []string{tagOf(r.Path)},
		}
		tags[tagOf(r.Path)] = true
		if len(r.Doc) > 0 {
			op["description"] = strings.Join(r.Doc, "\n\n")
		}
		if r.Access == "anyone" {
			op["security"] = []any{}
		}
		var params []any
		for _, m := range pathParam.FindAllStringSubmatch(r.Path, -1) {
			p := map[string]any{"name": m[1], "in": "path", "required": true, "schema": map[string]any{"type": "string"}}
			if d := pathParamDoc[m[1]]; d != "" {
				p["description"] = d
			}
			params = append(params, p)
		}
		for _, q := range r.Query {
			sch := map[string]any{"type": "string"}
			if q.Integer {
				sch = map[string]any{"type": "integer"}
			}
			params = append(params, map[string]any{"name": q.Name, "in": "query", "schema": sch})
		}
		if r.Version {
			params = append(params,
				map[string]any{"name": "If-Match", "in": "header", "description": "The version the change was made from (or send ?version=).", "schema": map[string]any{"type": "string"}},
				map[string]any{"name": "version", "in": "query", "description": "The version the change was made from, when If-Match is not sent.", "schema": map[string]any{"type": "integer"}},
			)
		}
		for _, h := range r.Headers {
			if h.Name == "If-Match" {
				continue
			}
			params = append(params, map[string]any{"name": h.Name, "in": "header", "schema": map[string]any{"type": "string"}})
		}
		if len(params) > 0 {
			op["parameters"] = params
		}
		switch {
		case r.In != nil:
			op["requestBody"] = map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": s.of(r.In)}}}
		case r.RawBody:
			op["requestBody"] = map[string]any{"required": true, "description": "The file's bytes.",
				"content": map[string]any{"application/octet-stream": map[string]any{"schema": map[string]any{"type": "string", "format": "binary"}}}}
		}
		resp := map[string]any{}
		for st, v := range a.answers(r, s) {
			resp[fmt.Sprint(st)] = v
		}
		for _, st := range errorStatuses(r) {
			resp[fmt.Sprint(st)] = map[string]any{"$ref": "#/components/responses/" + fmt.Sprint(st)}
		}
		resp["default"] = map[string]any{"$ref": "#/components/responses/error"}
		op["responses"] = resp
		item, _ := paths[r.Path].(map[string]any)
		if item == nil {
			item = map[string]any{}
			paths[r.Path] = item
		}
		item[strings.ToLower(r.Method)] = op
	}
	if s.err != nil {
		return nil, s.err
	}
	responses := map[string]any{
		"error": map[string]any{"description": "An error.", "content": map[string]any{"application/json": map[string]any{"schema": errSchema}}},
	}
	for st, text := range errorText {
		responses[fmt.Sprint(st)] = map[string]any{"description": text, "content": map[string]any{"application/json": map[string]any{"schema": errSchema}}}
	}
	comps := map[string]any{}
	for k, v := range s.components {
		comps[k] = v
	}
	var tagList []any
	for _, t := range tagOrder {
		if tags[t] {
			tagList = append(tagList, map[string]any{"name": t})
		}
	}
	doc := map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":       "Interlinea team server",
			"version":     "1",
			"description": strings.Join(strings.Fields(intro), " "),
			"license":     map[string]any{"name": "See the repository's licence"},
		},
		"tags":     tagList,
		"paths":    paths,
		"security": []any{map[string]any{"bearer": []any{}}, map[string]any{"cookie": []any{}}},
		"components": map[string]any{
			"schemas":   comps,
			"responses": responses,
			"securitySchemes": map[string]any{
				"bearer": map[string]any{"type": "http", "scheme": "bearer", "description": "A token from a sign-in."},
				"cookie": map[string]any{"type": "apiKey", "in": "cookie", "name": "interlinea_session",
					"description": "The web app's sign-in. Requests must also carry the X-Interlinea header."},
			},
		},
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// markdown writes the reference for people.
func (a *api) markdown() []byte {
	s := newSchemas(a)
	s.of(a.pkg.Scope().Lookup("apiError").Type())
	var b strings.Builder
	b.WriteString("# Team server API\n\n")
	b.WriteString("<!-- Written by `go run ./cmd/interlinea-apidoc` from the server's source. Do not edit. -->\n\n")
	for _, p := range strings.Split(intro, "\n\n") {
		b.WriteString(strings.Join(strings.Fields(p), " ") + "\n\n")
	}
	b.WriteString("The same description is served as OpenAPI 3.1 at `/api/openapi.json` (and kept in `server/openapi.json`).\n\n")
	b.WriteString("Every route may also answer with an [Error](#error): 422 for a request with problems, 429 when there are too many sign-in attempts, 500 when the server failed.\n\n")
	byTag := map[string][]*route{}
	for _, r := range a.routes {
		byTag[tagOf(r.Path)] = append(byTag[tagOf(r.Path)], r)
	}
	b.WriteString("## Routes\n\n")
	for _, t := range tagOrder {
		if len(byTag[t]) == 0 {
			continue
		}
		fmt.Fprintf(&b, "- [%s](#%s)\n", t, anchor(t))
	}
	b.WriteString("- [Types](#types)\n\n")
	for _, t := range tagOrder {
		rs := byTag[t]
		if len(rs) == 0 {
			continue
		}
		fmt.Fprintf(&b, "## %s\n\n", t)
		for _, r := range rs {
			fmt.Fprintf(&b, "### `%s %s`\n\n%s\n\n", r.Method, r.Path, r.Summary)
			for _, p := range r.Doc {
				b.WriteString(p + "\n\n")
			}
			var ps []string
			for _, m := range pathParam.FindAllStringSubmatch(r.Path, -1) {
				ps = append(ps, fmt.Sprintf("`%s` (path)", m[1]))
			}
			for _, q := range r.Query {
				ps = append(ps, fmt.Sprintf("`%s` (query)", q.Name))
			}
			if r.Version {
				ps = append(ps, "`If-Match` or `?version=`")
			}
			for _, h := range r.Headers {
				if h.Name != "If-Match" {
					ps = append(ps, fmt.Sprintf("`%s` (header)", h.Name))
				}
			}
			if len(ps) > 0 {
				fmt.Fprintf(&b, "- Parameters: %s\n", strings.Join(ps, ", "))
			}
			switch {
			case r.In != nil:
				fmt.Fprintf(&b, "- Body: %s\n", s.text(r.In))
			case r.RawBody:
				b.WriteString("- Body: the file's bytes\n")
			}
			ans := a.answers(r, s)
			var sts []int
			for st := range ans {
				sts = append(sts, st)
			}
			sort.Ints(sts)
			for _, st := range sts {
				line := fmt.Sprintf("%d %s", st, http.StatusText(st))
				for _, o := range r.Outs {
					if o.Status == st || (o.Status == 0 && st == 200) {
						line += ": " + s.text(o.Type)
						break
					}
				}
				if len(r.Outs) == 0 && st == 200 && len(r.ContentTypes) > 0 {
					var cts []string
					for _, ct := range r.ContentTypes {
						if !strings.HasPrefix(ct, "text/html") {
							mt, _, _ := strings.Cut(ct, ";")
							cts = append(cts, "`"+mt+"`")
						}
					}
					line += ": " + strings.Join(cts, ", ")
				}
				fmt.Fprintf(&b, "- Answer: %s\n", line)
			}
			var errs []string
			for _, st := range errorStatuses(r) {
				errs = append(errs, fmt.Sprint(st))
			}
			if len(errs) > 0 {
				fmt.Fprintf(&b, "- Errors: %s (see [Error](#error))\n", strings.Join(errs, ", "))
			}
			b.WriteString("\n")
		}
	}
	b.WriteString("## Types\n\n")
	var names []string
	for n := range s.components {
		names = append(names, n)
	}
	sort.Strings(names)
	for i := 0; i < len(names); i++ {
		n := names[i]
		sch := s.components[n]
		fmt.Fprintf(&b, "### %s\n\n", n)
		if d, _ := sch["description"].(string); d != "" {
			b.WriteString(d + "\n\n")
		}
		if e, ok := sch["enum"].([]any); ok {
			var vs []string
			for _, v := range e {
				vs = append(vs, fmt.Sprintf("`%v`", v))
			}
			fmt.Fprintf(&b, "One of %s.\n\n", strings.Join(vs, ", "))
		}
		if props, ok := sch["properties"].(map[string]any); ok && len(props) > 0 {
			b.WriteString("| Field | Type | Description |\n|---|---|---|\n")
			var keys []string
			for k := range props {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				p := props[k].(map[string]any)
				d, _ := p["description"].(string)
				fmt.Fprintf(&b, "| `%s` | %s | %s |\n", k, schemaText(p), strings.ReplaceAll(d, "|", "\\|"))
			}
			b.WriteString("\n")
		} else if _, isObj := sch["properties"]; !isObj && sch["enum"] == nil {
			fmt.Fprintf(&b, "%s.\n\n", schemaText(sch))
		}
		// Describing a type may have added more.
		if len(s.components) != len(names) {
			names = names[:0]
			for k := range s.components {
				names = append(names, k)
			}
			sort.Strings(names)
		}
	}
	return []byte(b.String())
}

// text describes a Go type in a line, describing its components.
func (s *schemas) text(t types.Type) string {
	return schemaText(s.of(t))
}

func schemaText(sch map[string]any) string {
	if r, ok := sch["$ref"].(string); ok {
		n := strings.TrimPrefix(r, "#/components/schemas/")
		return fmt.Sprintf("[%s](#%s)", n, anchor(n))
	}
	if all, ok := sch["allOf"].([]any); ok && len(all) == 1 {
		return schemaText(all[0].(map[string]any))
	}
	if alts, ok := sch["anyOf"].([]any); ok {
		var parts []string
		for _, x := range alts {
			parts = append(parts, schemaText(x.(map[string]any)))
		}
		return strings.Join(parts, " or ")
	}
	switch sch["type"] {
	case "array":
		return "list of " + schemaText(sch["items"].(map[string]any))
	case "object":
		if ap, ok := sch["additionalProperties"].(map[string]any); ok {
			return "map of " + schemaText(ap)
		}
		if p, ok := sch["properties"].(map[string]any); ok && len(p) > 0 {
			var keys []string
			for k := range p {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			var fs []string
			for _, k := range keys {
				fs = append(fs, fmt.Sprintf("`%s`: %s", k, schemaText(p[k].(map[string]any))))
			}
			return "{" + strings.Join(fs, ", ") + "}"
		}
		return "object"
	case "string":
		if f, ok := sch["format"].(string); ok {
			return "string (" + f + ")"
		}
		if e, ok := sch["enum"].([]any); ok {
			var vs []string
			for _, v := range e {
				vs = append(vs, fmt.Sprintf("`%v`", v))
			}
			return strings.Join(vs, " \\| ")
		}
		return "string"
	case "integer", "number", "boolean", "null":
		return sch["type"].(string)
	}
	return "any"
}

// anchor is the id GitHub gives a heading.
func anchor(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('-')
		}
	}
	return b.String()
}
