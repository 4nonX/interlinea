// Command interlinea-apidoc writes the team server's API reference: an
// OpenAPI 3.1 description (server/openapi.json, which the server serves at
// /api/openapi.json) and a Markdown reference (docs/api.md).
//
// Both come from the server's source: the routes it registers, the doc
// comments of their handlers, and the types the handlers read and write,
// found by type-checking the server package. Run it from the module's root
// after changing the API:
//
//	go run ./cmd/interlinea-apidoc
//
// With -check it writes nothing and fails if the files are out of date, as
// CI runs it.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/constant"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const serverPkg = "github.com/4nonX/interlinea/server"

func main() {
	check := flag.Bool("check", false, "fail if the files are out of date instead of writing them")
	dir := flag.String("dir", "server", "the server package's folder")
	spec := flag.String("openapi", "server/openapi.json", "where the OpenAPI description goes")
	md := flag.String("markdown", "docs/api.md", "where the Markdown reference goes")
	flag.Parse()
	api, err := scan(*dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "interlinea-apidoc:", err)
		os.Exit(1)
	}
	js, err := api.openAPI()
	if err != nil {
		fmt.Fprintln(os.Stderr, "interlinea-apidoc:", err)
		os.Exit(1)
	}
	out := map[string][]byte{*spec: js, *md: api.markdown()}
	names := []string{*spec, *md}
	stale := false
	for _, name := range names {
		old, _ := os.ReadFile(name)
		if bytes.Equal(old, out[name]) {
			continue
		}
		if *check {
			fmt.Fprintf(os.Stderr, "interlinea-apidoc: %s is out of date: run go run ./cmd/interlinea-apidoc\n", name)
			stale = true
			continue
		}
		if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			fmt.Fprintln(os.Stderr, "interlinea-apidoc:", err)
			os.Exit(1)
		}
		if err := os.WriteFile(name, out[name], 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "interlinea-apidoc:", err)
			os.Exit(1)
		}
		fmt.Println("wrote", name)
	}
	if stale {
		os.Exit(1)
	}
}

// route is one registered route and what its handler does.
type route struct {
	Method, Path string
	Handler      string
	// Access is the wrapper: anyone, signedIn or admin.
	Access  string
	Summary string
	// Doc is the rest of the doc comment, in paragraphs.
	Doc []string
	facts
}

// facts are what a handler (and the helpers it calls) read and write.
type facts struct {
	In      types.Type
	RawBody bool
	Outs    []output
	// Statuses are answers without a JSON body (WriteHeader).
	Statuses     []int
	ContentTypes []string
	BytesOut     bool
	Redirect     bool
	Query        []param
	Headers      []param
	Version      bool
}

type output struct {
	Status int
	Type   types.Type
}

type param struct {
	Name    string
	Integer bool
}

type api struct {
	fset   *token.FileSet
	pkg    *types.Package
	info   *types.Info
	funcs  map[types.Object]*ast.FuncDecl
	routes []*route
	docs   *docIndex
}

func scan(dir string) (*api, error) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, func(fi os.FileInfo) bool { return !strings.HasSuffix(fi.Name(), "_test.go") }, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	p, ok := pkgs["server"]
	if !ok {
		return nil, fmt.Errorf("no package server in %s", dir)
	}
	var files []*ast.File
	var names []string
	for n := range p.Files {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		files = append(files, p.Files[n])
	}
	info := &types.Info{
		Types: map[ast.Expr]types.TypeAndValue{},
		Defs:  map[*ast.Ident]types.Object{},
		Uses:  map[*ast.Ident]types.Object{},
	}
	conf := types.Config{Importer: importer.ForCompiler(fset, "source", nil)}
	pkg, err := conf.Check(serverPkg, fset, files, info)
	if err != nil {
		return nil, fmt.Errorf("type-checking the server: %w", err)
	}
	a := &api{fset: fset, pkg: pkg, info: info, funcs: map[types.Object]*ast.FuncDecl{}, docs: newDocIndex(fset)}
	for _, f := range files {
		for _, d := range f.Decls {
			if fd, ok := d.(*ast.FuncDecl); ok && fd.Body != nil {
				a.funcs[info.Defs[fd.Name]] = fd
			}
		}
	}
	if err := a.findRoutes(files); err != nil {
		return nil, err
	}
	return a, nil
}

var pattern = regexp.MustCompile(`^(GET|POST|PUT|PATCH|DELETE) (/\S*)$`)

// findRoutes reads the s.handle("METHOD /path", s.wrap(s.handler)) calls.
func (a *api) findRoutes(files []*ast.File) error {
	var problems []string
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			c, ok := n.(*ast.CallExpr)
			if !ok || len(c.Args) != 2 {
				return true
			}
			sel, ok := c.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "handle" {
				return true
			}
			lit, ok := c.Args[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			pat, _ := strconv.Unquote(lit.Value)
			m := pattern.FindStringSubmatch(pat)
			if m == nil || !strings.HasPrefix(m[2], "/api/") {
				return true
			}
			wrap, ok := c.Args[1].(*ast.CallExpr)
			if !ok || len(wrap.Args) != 1 {
				problems = append(problems, pat+": the handler is not s.anyone/s.signedIn/s.admin(s.handler)")
				return true
			}
			ws, ok1 := wrap.Fun.(*ast.SelectorExpr)
			hs, ok2 := wrap.Args[0].(*ast.SelectorExpr)
			if !ok1 || !ok2 {
				problems = append(problems, pat+": the handler is not s.anyone/s.signedIn/s.admin(s.handler)")
				return true
			}
			obj := a.info.Uses[hs.Sel]
			fd := a.funcs[obj]
			if fd == nil {
				problems = append(problems, pat+": the handler "+hs.Sel.Name+" is not found")
				return true
			}
			r := &route{Method: m[1], Path: m[2], Handler: hs.Sel.Name, Access: ws.Sel.Name}
			if r.Access != "anyone" && r.Access != "signedIn" && r.Access != "admin" {
				problems = append(problems, pat+": unknown wrapper "+r.Access)
			}
			summary, doc, err := handlerDoc(fd)
			if err != nil {
				problems = append(problems, pat+": "+err.Error())
			}
			r.Summary, r.Doc = summary, doc
			r.facts = a.factsOf(fd)
			a.routes = append(a.routes, r)
			return true
		})
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "\n"))
	}
	if len(a.routes) == 0 {
		return errors.New("no routes found")
	}
	sort.SliceStable(a.routes, func(i, j int) bool {
		if a.routes[i].Path != a.routes[j].Path {
			return a.routes[i].Path < a.routes[j].Path
		}
		return methodRank(a.routes[i].Method) < methodRank(a.routes[j].Method)
	})
	return nil
}

func methodRank(m string) int {
	return strings.Index("GET POST PUT PATCH DELETE", m)
}

// handlerDoc splits a handler's doc comment into its summary (the first
// sentence, without the handler's name) and paragraphs.
func handlerDoc(fd *ast.FuncDecl) (string, []string, error) {
	if fd.Doc == nil {
		return "", nil, fmt.Errorf("%s has no doc comment", fd.Name.Name)
	}
	text := fd.Doc.Text()
	rest, ok := strings.CutPrefix(text, fd.Name.Name+" ")
	if !ok {
		return "", nil, fmt.Errorf("the doc comment of %s does not start with its name", fd.Name.Name)
	}
	var paras []string
	for _, p := range strings.Split(strings.TrimSpace(rest), "\n\n") {
		paras = append(paras, strings.Join(strings.Fields(p), " "))
	}
	if !strings.HasPrefix(paras[len(paras)-1], "Access: ") {
		return "", nil, fmt.Errorf("the doc comment of %s does not end with an Access: paragraph", fd.Name.Name)
	}
	first := paras[0]
	end := strings.Index(first, ". ")
	summary, more := first, ""
	if end >= 0 {
		summary, more = first[:end+1], strings.TrimSpace(first[end+2:])
	}
	summary = strings.ToUpper(summary[:1]) + summary[1:]
	doc := paras[1:]
	if more != "" {
		doc = append([]string{more}, doc...)
	}
	return summary, doc, nil
}

// Helpers whose effects are not the route's own: errors are described
// once for all routes, and version is described as a parameter.
var skip = map[string]bool{"fail": true, "version": true, "authenticate": true}

func (a *api) factsOf(fd *ast.FuncDecl) facts {
	var f facts
	seen := map[*ast.FuncDecl]bool{}
	a.walk(fd, &f, seen, 0)
	f.Query = dedupe(f.Query)
	f.Headers = dedupe(f.Headers)
	return f
}

func dedupe(ps []param) []param {
	seen := map[string]bool{}
	var out []param
	for _, p := range ps {
		if !seen[p.Name] {
			seen[p.Name] = true
			out = append(out, p)
		}
	}
	return out
}

func (a *api) walk(fd *ast.FuncDecl, f *facts, seen map[*ast.FuncDecl]bool, statusHint int) {
	if seen[fd] {
		return
	}
	seen[fd] = true
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncLit:
			return true
		case *ast.SelectorExpr:
			// A raw request body: r.Body.
			if x.Sel.Name == "Body" && isType(a.info.TypeOf(x.X), "net/http", "Request") {
				f.RawBody = true
			}
			return true
		case *ast.CallExpr:
			a.call(x, f, seen, statusHint)
		}
		return true
	})
}

func (a *api) call(c *ast.CallExpr, f *facts, seen map[*ast.FuncDecl]bool, statusHint int) {
	var obj types.Object
	var name string
	switch fn := c.Fun.(type) {
	case *ast.Ident:
		obj, name = a.info.Uses[fn], fn.Name
	case *ast.SelectorExpr:
		obj, name = a.info.Uses[fn.Sel], fn.Sel.Name
		recv := a.info.TypeOf(fn.X)
		switch {
		case name == "Get" && len(c.Args) == 1 && isType(recv, "net/url", "Values"):
			if s, ok := a.str(c.Args[0]); ok {
				f.Query = append(f.Query, param{Name: s})
			}
		case name == "Get" && len(c.Args) == 1 && isType(recv, "net/http", "Header") && isRequestHeader(fn.X):
			// The proxy's headers are not the apps' to send.
			if s, ok := a.str(c.Args[0]); ok && !strings.HasPrefix(s, "X-Forwarded-") {
				f.Headers = append(f.Headers, param{Name: s})
			}
		case name == "Set" && len(c.Args) == 2 && isType(recv, "net/http", "Header"):
			if k, ok := a.str(c.Args[0]); ok && k == "Content-Type" {
				if v, ok := a.str(c.Args[1]); ok {
					f.ContentTypes = append(f.ContentTypes, v)
				} else {
					f.ContentTypes = append(f.ContentTypes, "application/octet-stream")
				}
			}
		case name == "Write" && len(c.Args) == 1 && isType(recv, "net/http", "ResponseWriter"):
			f.BytesOut = true
		case name == "WriteHeader" && len(c.Args) == 1:
			if v, ok := a.intValue(c.Args[0]); ok {
				f.Statuses = append(f.Statuses, v)
			} else if statusHint != 0 {
				f.Statuses = append(f.Statuses, statusHint)
			}
		case name == "Redirect" && obj != nil && obj.Pkg() != nil && obj.Pkg().Path() == "net/http":
			f.Redirect = true
		}
	}
	if obj == nil || obj.Pkg() == nil || obj.Pkg().Path() != serverPkg {
		return
	}
	switch name {
	case "writeJSON":
		if len(c.Args) == 3 {
			st, ok := a.intValue(c.Args[1])
			if !ok {
				st = statusHint
			}
			t := a.info.TypeOf(c.Args[2])
			if !isType(t, serverPkg, "apiError") {
				f.Outs = append(f.Outs, output{Status: st, Type: t})
			}
		}
		return
	case "readJSON":
		if len(c.Args) >= 3 {
			t := a.info.TypeOf(c.Args[2])
			if p, ok := t.(*types.Pointer); ok {
				t = p.Elem()
			}
			if f.In == nil {
				f.In = t
			}
		}
		return
	case "queryInt":
		if len(c.Args) >= 3 {
			if s, ok := a.str(c.Args[2]); ok {
				f.Query = append(f.Query, param{Name: s, Integer: true})
			}
		}
		return
	case "version":
		f.Version = true
		return
	}
	if skip[name] {
		return
	}
	fd := a.funcs[obj]
	if fd == nil {
		return
	}
	hint := statusHint
	for _, arg := range c.Args {
		if v, ok := a.intValue(arg); ok && v >= 100 && v < 600 {
			hint = v
			break
		}
	}
	a.walk(fd, f, seen, hint)
}

// isRequestHeader reports whether e is r.Header for a request r.
func isRequestHeader(e ast.Expr) bool {
	s, ok := e.(*ast.SelectorExpr)
	return ok && s.Sel.Name == "Header"
}

func (a *api) str(e ast.Expr) (string, bool) {
	tv, ok := a.info.Types[e]
	if !ok || tv.Value == nil || tv.Value.Kind() != constant.String {
		return "", false
	}
	return constant.StringVal(tv.Value), true
}

func (a *api) intValue(e ast.Expr) (int, bool) {
	tv, ok := a.info.Types[e]
	if !ok || tv.Value == nil || tv.Value.Kind() != constant.Int {
		return 0, false
	}
	v, ok := constant.Int64Val(tv.Value)
	return int(v), ok
}

func isType(t types.Type, pkg, name string) bool {
	if t == nil {
		return false
	}
	if p, ok := t.(*types.Pointer); ok {
		t = p.Elem()
	}
	n, ok := t.(*types.Named)
	return ok && n.Obj().Pkg() != nil && n.Obj().Pkg().Path() == pkg && n.Obj().Name() == name
}

// docIndex finds the doc comments of types and fields by their position,
// parsing the files they are in.
type docIndex struct {
	fset  *token.FileSet
	files map[string]map[token.Pos]string
}

func newDocIndex(fset *token.FileSet) *docIndex {
	return &docIndex{fset: fset, files: map[string]map[token.Pos]string{}}
}

// of returns the doc comment of the type or field declared at pos.
func (d *docIndex) of(pos token.Pos) string {
	tf := d.fset.File(pos)
	if tf == nil {
		return ""
	}
	name := tf.Name()
	idx, ok := d.files[name]
	if !ok {
		idx = map[token.Pos]string{}
		d.files[name] = idx
		// Parse into the same file set, at the same base, so positions
		// match those the type checker recorded.
		src, err := os.ReadFile(name)
		if err == nil && len(src) == tf.Size() {
			fs := token.NewFileSet()
			f, err := parser.ParseFile(fs, name, src, parser.ParseComments)
			if err == nil {
				shift := token.Pos(tf.Base()) - token.Pos(fs.File(f.Pos()).Base())
				add := func(id *ast.Ident, cg ...*ast.CommentGroup) {
					for _, c := range cg {
						if c != nil {
							idx[id.Pos()+shift] = strings.Join(strings.Fields(c.Text()), " ")
							return
						}
					}
				}
				ast.Inspect(f, func(n ast.Node) bool {
					switch x := n.(type) {
					case *ast.GenDecl:
						if x.Tok == token.TYPE {
							for _, s := range x.Specs {
								ts := s.(*ast.TypeSpec)
								if len(x.Specs) == 1 {
									add(ts.Name, ts.Doc, x.Doc, ts.Comment)
								} else {
									add(ts.Name, ts.Doc, ts.Comment)
								}
							}
						}
					case *ast.Field:
						for _, id := range x.Names {
							add(id, x.Doc, x.Comment)
						}
					}
					return true
				})
			}
		}
	}
	return idx[pos]
}
