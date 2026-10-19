package language_test

import (
	"testing"

	"github.com/golang-gui/goui-sourceview/language"
)

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
	for _, id := range []language.ID{language.Plain, language.JSON, language.XML, language.HTML, language.TOML, language.YAML, language.Shell} {
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
