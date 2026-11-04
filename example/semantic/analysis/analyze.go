// Package analysis is an application-owned, single-file Go analysis service.
// It never executes source, resolves imports, or reads SourceView internals.
package analysis

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"go/types"
	"sort"
	"strings"
	"unicode/utf8"

	source "github.com/golang-gui/goui-sourceview"
	"github.com/golang-gui/goui-sourceview/language"
)

const MaxBytes = 256 * 1024
const (
	Type           language.Kind = "semantic.type"
	Function       language.Kind = "semantic.function"
	Field          language.Kind = "semantic.field"
	StringVariable language.Kind = "semantic.string-variable"
	NumberVariable language.Kind = "semantic.number-variable"
	Variable       language.Kind = "semantic.variable"
	Constant       language.Kind = "semantic.constant"
	Invalid        language.Kind = "semantic.invalid"
)

// Snapshot contains a complete, immutable UTF-8 source file and its identity.
type Snapshot struct {
	DocumentID, Revision uint64
	Text                 string
}
type Span struct {
	Start, End int
	Kind       language.Kind
}
type Result struct {
	Snapshot    Snapshot
	Spans       []Span
	Diagnostics []string
}
type checked struct {
	file        *ast.File
	set         *token.FileSet
	info        *types.Info
	pkg         *types.Package
	diagnostics []string
}

func check(ctx context.Context, text string) (*checked, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(text) > MaxBytes {
		return nil, fmt.Errorf("single-file example limit: %d bytes", MaxBytes)
	}
	c := &checked{set: token.NewFileSet(), info: &types.Info{Defs: map[*ast.Ident]types.Object{}, Uses: map[*ast.Ident]types.Object{}, Types: map[ast.Expr]types.TypeAndValue{}, Scopes: map[ast.Node]*types.Scope{}}}
	report := func(message string) {
		if len(c.diagnostics) < 8 {
			if len(message) > 512 {
				end := 512
				for !utf8.RuneStart(message[end]) {
					end--
				}
				message = message[:end] + "…"
			}
			c.diagnostics = append(c.diagnostics, message)
		}
	}
	var err error
	c.file, err = parser.ParseFile(c.set, "sample.go", text, parser.AllErrors|parser.ParseComments)
	if errs, ok := err.(scanner.ErrorList); ok {
		for _, e := range errs {
			report(e.Error())
		}
	} else if err != nil {
		report(err.Error())
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c.file != nil {
		config := types.Config{Error: func(err error) { report(err.Error()) }}
		// A nil importer intentionally refuses imports: this example has no project
		// loader, dependency downloads, or hidden filesystem reads.
		c.pkg, _ = config.Check("example", c.set, []*ast.File{c.file}, c.info)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return c, nil
}
func kindOf(obj types.Object) language.Kind {
	switch o := obj.(type) {
	case *types.TypeName:
		return Type
	case *types.Func, *types.Builtin:
		return Function
	case *types.Const:
		return Constant
	case *types.Var:
		if o.IsField() {
			return Field
		}
		if b, ok := o.Type().Underlying().(*types.Basic); ok {
			if b.Info()&types.IsString != 0 {
				return StringVariable
			}
			if b.Info()&types.IsNumeric != 0 {
				return NumberVariable
			}
		}
		return Variable
	}
	return language.KindPlain
}
func Analyze(ctx context.Context, s Snapshot) (Result, error) {
	c, err := check(ctx, s.Text)
	if err != nil {
		return Result{}, err
	}
	out := Result{Snapshot: s, Diagnostics: c.diagnostics}
	add := func(id *ast.Ident, obj types.Object) {
		if obj == nil || id.Name == "_" {
			return
		}
		kind := kindOf(obj)
		if kind == language.KindPlain {
			return
		}
		at := c.set.PositionFor(id.Pos(), false).Offset
		if at >= 0 && at+len(id.Name) <= len(s.Text) {
			out.Spans = append(out.Spans, Span{at, at + len(id.Name), kind})
		}
	}
	for id, obj := range c.info.Defs {
		add(id, obj)
	}
	for id, obj := range c.info.Uses {
		add(id, obj)
	}
	if c.file != nil {
		for _, id := range c.file.Unresolved {
			if c.info.Uses[id] == nil && types.Universe.Lookup(id.Name) == nil {
				at := c.set.PositionFor(id.Pos(), false).Offset
				if at >= 0 && at+len(id.Name) <= len(s.Text) {
					out.Spans = append(out.Spans, Span{at, at + len(id.Name), Invalid})
				}
			}
		}
	}
	sort.Slice(out.Spans, func(i, j int) bool { return out.Spans[i].Start < out.Spans[j].Start })
	return out, nil
}

// Complete checks a repaired copy at the requested word. The sentinel allows
// go/parser to retain a selector even while the user has only typed "person.".
func Complete(ctx context.Context, s Snapshot, request source.CompletionContext) ([]source.Completion, error) {
	if s.DocumentID != request.DocumentID || s.Revision != request.Revision || len(s.Text) != request.DocumentLength {
		return nil, nil
	}
	if request.ContextKnown && (request.Lexical.String || request.Lexical.Comment) {
		return nil, nil
	}
	r := request.Range
	if r.Start < 0 || r.End < r.Start || r.End > len(s.Text) {
		return nil, fmt.Errorf("completion range outside snapshot")
	}
	const sentinel = "__sourceview_complete__"
	text := s.Text[:r.Start] + sentinel + s.Text[r.End:]
	c, err := check(ctx, text)
	if err != nil {
		return nil, err
	}
	if c.file == nil || c.pkg == nil {
		return nil, nil
	}
	at := c.set.File(c.file.Pos()).Pos(r.Start)
	// Lexical context may be temporarily unknown while SourceView revalidates
	// a changed definition. The external AST still excludes comments/literals.
	for _, group := range c.file.Comments {
		for _, comment := range group.List {
			if comment.Pos() <= at && at < comment.End() {
				return nil, nil
			}
		}
	}
	literal := false
	ast.Inspect(c.file, func(n ast.Node) bool {
		if value, ok := n.(*ast.BasicLit); ok && (value.Kind == token.STRING || value.Kind == token.CHAR) && value.Pos() <= at && at < value.End() {
			literal = true
		}
		return !literal
	})
	if literal {
		return nil, nil
	}
	objects := map[string]types.Object{}
	selector := false
	ast.Inspect(c.file, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok || sel.Sel.Pos() != at {
			return true
		}
		selector = true
		typ := c.info.TypeOf(sel.X)
		if typ == nil {
			return false
		}
		base := typ
		if p, ok := base.(*types.Pointer); ok {
			base = p.Elem()
		}
		if st, ok := base.Underlying().(*types.Struct); ok {
			for i := 0; i < st.NumFields(); i++ {
				f := st.Field(i)
				objects[f.Name()] = f
			}
		}
		for _, t := range []types.Type{typ, types.NewPointer(base)} {
			ms := types.NewMethodSet(t)
			for i := 0; i < ms.Len(); i++ {
				obj := ms.At(i).Obj()
				objects[obj.Name()] = obj
			}
		}
		return false
	})
	if !selector {
		inner := c.pkg.Scope()
		for _, scope := range c.info.Scopes {
			if scope.Contains(at) && (inner == c.pkg.Scope() || scope.End()-scope.Pos() < inner.End()-inner.Pos()) {
				inner = scope
			}
		}
		for scope := inner; scope != nil; scope = scope.Parent() {
			for _, name := range scope.Names() {
				_, obj := inner.LookupParent(name, at)
				if obj != nil {
					objects[name] = obj
				}
			}
		}
	}
	names := make([]string, 0, len(objects))
	for name := range objects {
		if name != "_" && name != sentinel && strings.HasPrefix(name, request.Prefix) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	out := make([]source.Completion, 0, min(len(names), 100))
	for _, name := range names {
		if len(out) == 100 {
			break
		}
		out = append(out, source.Completion{Label: name, Text: name, Detail: types.ObjectString(objects[name], func(p *types.Package) string { return p.Name() })})
	}
	return out, ctx.Err()
}
