package language_test

import (
	"strings"
	"testing"

	"github.com/golang-gui/goui-sourceview/language"
)

// 标签属性只能属于当前标签，不能因分片位置不同而残留到后续标签或注释。
func TestMarkupTagStateIsolation(t *testing.T) {
	for _, id := range []language.ID{language.XML, language.HTML} {
		for _, text := range []string{`<a x="v"><b`, `<a x="v"/><b`, `<a x="v"><!--open`} {
			l, _ := language.Builtin(id)
			scanner := l.Tokenizer()
			whole, err := scanner.Scan(language.Input{Text: text, Limit: len(text), LineEnd: true}, scanner.InitialState())
			if err != nil {
				t.Fatal(err)
			}
			cut := strings.IndexByte(text, '>') + 1
			part, err := scanner.Scan(language.Input{Text: text[:cut], Limit: cut}, scanner.InitialState())
			if err != nil {
				t.Fatal(err)
			}
			tail := language.Input{Text: text[cut:], Offset: cut, Limit: len(text) - cut, LineEnd: true}
			part, err = scanner.Scan(tail, part.State)
			if err != nil {
				t.Fatal(err)
			}
			if err = language.ValidateResult(tail, part); err != nil {
				t.Fatal(err)
			}
			if !part.State.Equal(whole.State) {
				t.Fatalf("%s %q: fragment state differs", id, text)
			}
			c := part.State.Context()
			if strings.Contains(text, "<!--") {
				if !c.Comment {
					t.Fatal("lost comment context")
				}
			} else if !c.Tag || c.Comment || c.String {
				t.Fatal("lost tag context")
			}
		}
	}
}

func TestDefinitionCopiesConfiguration(t *testing.T) {
	pairs := []language.Pair{{Open: "<<", Close: ">>"}}
	d := language.Definition{Info: language.Info{ID: "custom", Name: "Custom", Extensions: []string{".x"}}, Editing: language.EditingRules{Brackets: pairs}, Styles: []language.Kind{"heading"}}
	l, err := language.New(d)
	if err != nil {
		t.Fatal(err)
	}
	pairs[0].Open = "changed"
	d.Info.Extensions[0] = ".bad"
	d.Styles[0] = "bad"
	copy := l.Definition()
	copy.Editing.Brackets[0].Close = "bad"
	copy.Info.Extensions[0] = ".bad"
	if got := l.Definition(); got.Editing.Brackets[0].Open != "<<" || got.Editing.Brackets[0].Close != ">>" || got.Info.Extensions[0] != ".x" || got.Styles[0] != "heading" {
		t.Fatalf("mutable definition: %+v", got)
	}
	if !l.IsWord('名') || l.IsWord('-') {
		t.Fatal("default words")
	}
}
func TestScanResultValidation(t *testing.T) {
	in := language.Input{Text: "é{}", Limit: 2}
	for _, result := range []language.Result{
		{Consumed: 1, State: language.EmptyState{}},
		{Consumed: 2},
		{Consumed: 2, State: language.EmptyState{}, Tokens: []language.Token{{Start: 0, End: 1, Kind: "custom"}}},
		{Consumed: 2, State: language.EmptyState{}, Tokens: []language.Token{{Start: 0, End: 2, Kind: "bad key"}}},
		{Consumed: 2, State: language.EmptyState{}, ReplaceFrom: new(int)},
	} {
		// A replacement from zero is valid; all other cases must be rejected.
		err := language.ValidateResult(in, result)
		if (result.ReplaceFrom != nil) != (err == nil) {
			t.Fatalf("result %+v: %v", result, err)
		}
	}
	if err := language.ValidateResult(in, language.Result{Consumed: 2, State: language.EmptyState{}, Tokens: []language.Token{{Start: 0, End: 2, Kind: "heading", Class: language.ClassString}}}); err != nil {
		t.Fatal(err)
	}
}
func TestBuiltinDefinitions(t *testing.T) {
	for _, id := range []language.ID{
		language.Plain, language.JSON, language.JSONC, language.JSONLines, language.XML, language.HTML, language.TOML, language.YAML, language.Shell,
		language.HTTP, language.Makefile, language.CMake, language.Nginx, language.INI, language.Dockerfile, language.SQL, language.Protobuf,
		language.Go, language.Python, language.JavaScript, language.TypeScript, language.CSS, language.C, language.Cpp, language.Java, language.Rust,
		language.CSharp, language.Lua, language.PowerShell, language.Ruby, language.PHP, language.JSX, language.TSX, language.Markdown,
		language.Kotlin, language.Swift, language.Dart, language.R, language.Zig, language.ObjectiveC, language.ObjectiveCpp, language.Scala, language.Perl,
		language.Diff,
		language.Batch,
		language.HCL,
		language.GraphQL,
		language.SCSS,
		language.Less,
		language.GoMod, language.GoWork, language.GoSum,
		language.Dotenv,
		language.Properties,
		language.GitIgnore,
		language.GitAttributes,
		language.Starlark,
		language.Groovy,
		language.GoTemplate, language.Jinja2,
	} {
		l, ok := language.Builtin(id)
		if !ok || l.ID() != id || l.Info().Name == "" {
			t.Fatalf("missing %q", id)
		}
		if id == language.Plain {
			continue
		}
		scanner := l.Tokenizer()
		in := language.Input{Text: "{}", Limit: 2, LineEnd: true, DocumentEnd: true}
		result, err := scanner.Scan(in, scanner.InitialState())
		if err != nil {
			t.Fatal(err)
		}
		if err = language.ValidateResult(in, result); err != nil {
			t.Fatalf("%s: %v", id, err)
		}
	}
}

func TestQuoteEscapeValidation(t *testing.T) {
	for _, r := range []rune{0, '\\', '`', '§', -1, '\n', '\r', 0xd800, 0x110000} {
		d := language.Definition{Info: language.Info{ID: "test", Name: "Test"}, Editing: language.EditingRules{QuoteEscape: r}}
		l, err := language.New(d)
		valid := r == 0 || r == '\\' || r == '`' || r == '§'
		if (err == nil) != valid {
			t.Fatalf("%U: %v", r, err)
		}
		if valid && l.Editing().QuoteEscape != r {
			t.Fatal("lost quote escape")
		}
	}
}

func TestCompletionTriggersBelongToDefinition(t *testing.T) {
	custom, err := language.New(language.Definition{Info: language.Info{ID: language.HTML, Name: "Custom"}})
	if err != nil {
		t.Fatal(err)
	}
	builtin, _ := language.Builtin(language.HTML)
	if custom.CompletionTriggers() != "" || builtin.CompletionTriggers() != "<" {
		t.Fatal("trigger policy inferred from ID")
	}
	d := builtin.Definition()
	d.CompletionTriggers = "["
	derived, err := language.New(d)
	if err != nil {
		t.Fatal(err)
	}
	if derived.CompletionTriggers() != "[" || builtin.CompletionTriggers() != "<" {
		t.Fatal("trigger configuration was not isolated")
	}
}
