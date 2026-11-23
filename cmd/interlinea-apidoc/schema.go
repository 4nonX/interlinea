package main

import (
	"encoding/json"
	"fmt"
	"go/types"
	"reflect"
	"sort"
	"strings"
)

// schemas turns Go types into JSON schemas as encoding/json writes them.
// Named types of the module become components.
type schemas struct {
	a          *api
	components map[string]map[string]any
	// owner is the type behind each component name, to catch two types
	// with one name.
	owner map[string]types.Type
	err   error
}

func newSchemas(a *api) *schemas {
	return &schemas{a: a, components: map[string]map[string]any{}, owner: map[string]types.Type{}}
}

const module = "github.com/4nonX/interlinea/"

// componentName is the name a named type is described under.
func componentName(n *types.Named) string {
	o := n.Obj()
	if o.Pkg().Path() == serverPkg {
		if o.Name() == "apiError" {
			return "Error"
		}
		return strings.ToUpper(o.Name()[:1]) + o.Name()[1:]
	}
	return o.Pkg().Name() + "." + o.Name()
}

// local reports whether a named type is declared inside a function.
func local(n *types.Named) bool {
	return n.Obj().Parent() != nil && n.Obj().Parent() != n.Obj().Pkg().Scope()
}

// tagKinds are the names content.TagKind is written as (its MarshalText).
var tagKinds = []any{"open", "close", "standalone"}

func (s *schemas) of(t types.Type) map[string]any {
	switch x := t.(type) {
	case *types.Pointer:
		return s.of(x.Elem())
	case *types.Alias:
		return s.of(types.Unalias(x))
	case *types.Named:
		return s.named(x)
	case *types.Basic:
		return basic(x)
	case *types.Slice:
		if b, ok := x.Elem().(*types.Basic); ok && b.Kind() == types.Byte {
			return map[string]any{"type": "string", "format": "byte"}
		}
		return map[string]any{"type": "array", "items": s.of(x.Elem())}
	case *types.Array:
		return map[string]any{"type": "array", "items": s.of(x.Elem()), "minItems": x.Len(), "maxItems": x.Len()}
	case *types.Map:
		return map[string]any{"type": "object", "additionalProperties": s.of(x.Elem())}
	case *types.Struct:
		return s.object(x)
	case *types.Interface:
		return map[string]any{}
	}
	s.fail(fmt.Errorf("no schema for %s", t))
	return map[string]any{}
}

func (s *schemas) fail(err error) {
	if s.err == nil {
		s.err = err
	}
}

func basic(b *types.Basic) map[string]any {
	switch {
	case b.Info()&types.IsBoolean != 0:
		return map[string]any{"type": "boolean"}
	case b.Info()&types.IsString != 0:
		return map[string]any{"type": "string"}
	case b.Info()&types.IsInteger != 0:
		m := map[string]any{"type": "integer"}
		switch b.Kind() {
		case types.Int64, types.Uint64:
			m["format"] = "int64"
		case types.Int32, types.Uint32:
			m["format"] = "int32"
		}
		return m
	case b.Info()&types.IsFloat != 0:
		return map[string]any{"type": "number"}
	}
	return map[string]any{}
}

func (s *schemas) named(n *types.Named) map[string]any {
	o := n.Obj()
	if o.Pkg() == nil {
		// error, comparable and other universe types.
		return map[string]any{}
	}
	path := o.Pkg().Path() + "." + o.Name()
	switch path {
	case "time.Time":
		return map[string]any{"type": "string", "format": "date-time"}
	case "time.Duration":
		return map[string]any{"type": "integer", "format": "int64", "description": "nanoseconds"}
	case "encoding/json.RawMessage":
		return map[string]any{}
	case module + "content.TagKind":
		return map[string]any{"type": "string", "enum": tagKinds}
	}
	if !strings.HasPrefix(o.Pkg().Path(), module) {
		s.fail(fmt.Errorf("no schema for %s: describe it in schema.go", path))
		return map[string]any{}
	}
	if hasMethod(n, "MarshalText") {
		s.fail(fmt.Errorf("%s is written with MarshalText: describe it in schema.go", path))
	}
	if hasMethod(n, "MarshalJSON") && path != module+"content.Content" {
		// content.Content writes itself as its runs, as its type says.
		s.fail(fmt.Errorf("%s is written with MarshalJSON: describe it in schema.go", path))
	}
	if local(n) {
		return s.of(n.Underlying())
	}
	name := componentName(n)
	if prev, ok := s.owner[name]; ok {
		if !types.Identical(prev, n) {
			s.fail(fmt.Errorf("two types are called %s", name))
		}
		return ref(name)
	}
	s.owner[name] = n
	s.components[name] = nil // being described: refs to itself work
	sch := s.of(n.Underlying())
	if d := s.a.docs.of(o.Pos()); d != "" {
		sch["description"] = d
	}
	if b, ok := n.Underlying().(*types.Basic); ok {
		if vals := s.constants(n, b); len(vals) > 1 {
			sch["enum"] = vals
		}
	}
	s.components[name] = sch
	return ref(name)
}

func ref(name string) map[string]any {
	return map[string]any{"$ref": "#/components/schemas/" + name}
}

func hasMethod(n *types.Named, name string) bool {
	for _, t := range []types.Type{n, types.NewPointer(n)} {
		ms := types.NewMethodSet(t)
		for i := 0; i < ms.Len(); i++ {
			if ms.At(i).Obj().Name() == name {
				return true
			}
		}
	}
	return false
}

// constants are the package's constants of a named basic type, as the
// values it takes.
func (s *schemas) constants(n *types.Named, b *types.Basic) []any {
	scope := n.Obj().Pkg().Scope()
	var out []any
	for _, name := range scope.Names() {
		c, ok := scope.Lookup(name).(*types.Const)
		if !ok || !types.Identical(c.Type(), n) {
			continue
		}
		switch {
		case b.Info()&types.IsString != 0:
			out = append(out, constantString(c))
		case b.Info()&types.IsInteger != 0:
			out = append(out, constantInt(c))
		}
	}
	sort.Slice(out, func(i, j int) bool { return fmt.Sprint(out[i]) < fmt.Sprint(out[j]) })
	return out
}

func (s *schemas) object(st *types.Struct) map[string]any {
	props := map[string]any{}
	s.fields(st, props)
	return map[string]any{"type": "object", "properties": props}
}

// fields adds a struct's JSON fields to props; embedded structs without
// a JSON name add theirs, as encoding/json does.
func (s *schemas) fields(st *types.Struct, props map[string]any) {
	for i := 0; i < st.NumFields(); i++ {
		f := st.Field(i)
		tag := reflect.StructTag(st.Tag(i)).Get("json")
		name, opts, _ := strings.Cut(tag, ",")
		if name == "-" && opts == "" {
			continue
		}
		if f.Embedded() && name == "" {
			t := f.Type()
			if p, ok := t.(*types.Pointer); ok {
				t = p.Elem()
			}
			if inner, ok := t.Underlying().(*types.Struct); ok {
				s.fields(inner, props)
				continue
			}
		}
		if !f.Exported() {
			continue
		}
		if name == "" {
			name = f.Name()
		}
		sch := s.of(f.Type())
		if strings.Contains(","+opts+",", ",string,") {
			sch = map[string]any{"type": "string"}
		}
		if _, isPtr := f.Type().(*types.Pointer); isPtr && !strings.Contains(opts, "omit") {
			sch = map[string]any{"anyOf": []any{sch, map[string]any{"type": "null"}}}
		}
		if d := s.a.docs.of(f.Pos()); d != "" {
			sch = withDescription(sch, d)
		}
		props[name] = sch
	}
}

// withDescription adds a description; next to a $ref it goes in an allOf,
// as older tools ignore siblings of $ref.
func withDescription(sch map[string]any, d string) map[string]any {
	if _, ok := sch["$ref"]; ok {
		return map[string]any{"allOf": []any{sch}, "description": d}
	}
	out := map[string]any{}
	for k, v := range sch {
		out[k] = v
	}
	out["description"] = d
	return out
}

func constantString(c *types.Const) any {
	var s string
	if err := json.Unmarshal([]byte(c.Val().ExactString()), &s); err != nil {
		return c.Val().ExactString()
	}
	return s
}

func constantInt(c *types.Const) any {
	var n int64
	if err := json.Unmarshal([]byte(c.Val().ExactString()), &n); err != nil {
		return c.Val().ExactString()
	}
	return n
}
