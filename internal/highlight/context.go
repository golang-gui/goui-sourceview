package highlight

import (
	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/gui/textedit"
)

type Context = language.Context

// ContextAt never forces unbounded analysis or guesses unresolved predecessors.
func (e *Engine) ContextAt(at int) (Context, bool) {
	if e.err != nil {
		return Context{}, false
	}
	row := e.model.LineAt(at)
	r, _ := e.model.LineRange(row)
	if at-r.Start > 16<<10 {
		return Context{}, false
	}
	tokenizer := e.syntax.Tokenizer()
	if tokenizer == nil {
		return Context{}, true
	}
	state := tokenizer.InitialState()
	if row > 0 {
		if !e.Ready() || e.next >= 0 && row-1 >= e.next {
			return Context{}, false
		}
		previous := e.Line(row - 1)
		if previous == nil || !previous.complete {
			return Context{}, false
		}
		state = previous.out
	}
	end := min(r.End, at+32)
	for !e.model.IsRuneBoundary(end) {
		end--
	}
	prefix, err := e.model.Slice(textedit.Range{Start: r.Start, End: end})
	if err != nil {
		return Context{}, false
	}
	in := language.Input{Text: prefix, Limit: at - r.Start}
	result, err := tokenizer.Scan(in, state)
	if err != nil || language.ValidateResult(in, result) != nil || result.Consumed != in.Limit {
		return Context{}, false
	}
	return result.State.Context(), true
}
