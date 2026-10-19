package highlight

import (
	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/gui/textedit"
)

// Context is lexical editing context, independent of theme and token eviction.
type Context struct{ String, Comment, Tag, TagName, Value, Literal bool }

func (s State) Context() Context {
	if t := s.toml; t != nil {
		return Context{String: t.quote != 0, Comment: t.comment, Literal: t.quote == '\'', Value: !t.key}
	}
	if y := s.yaml; y != nil {
		return Context{String: y.quote != 0 || y.block && !y.blockHeader, Comment: y.comment, Literal: y.quote == '\''}
	}
	if sh := s.shell; sh != nil {
		mode := shTop(sh.stack)
		return Context{String: sh.active != nil || sh.capture || mode == shSingle || mode == shDouble || mode == shANSI, Comment: sh.comment, Literal: mode == shSingle || sh.active != nil && sh.active.literal}
	}

	return Context{
		String:  s.mode == jsonString || s.mode == attrSingle || s.mode == attrDouble,
		Comment: s.mode == comment || s.mode == cdata || s.mode == pi || s.mode == declaration || s.mode == scriptData || s.mode == rawText || s.mode == plainText,
		Tag:     s.mode == tagName || s.mode == tagInside || s.mode == attrName || s.mode == afterAttr || s.mode == beforeValue,
		TagName: s.mode == tagName,
		Value:   s.mode == beforeValue,
	}
}

// ContextAt reuses the last reliable incoming line state. It never forces an
// unbounded scan or interprets an unresolved multiline prefix as ordinary text.
// A long prefix (>16 KiB) or unresolved predecessor returns false.
func (e *Engine) ContextAt(at int) (Context, bool) {
	row := e.model.LineAt(at)
	r, _ := e.model.LineRange(row)
	if at-r.Start > 16<<10 {
		return Context{}, false
	}
	state := Initial()
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
	_, state, consumed := Scan(e.id, state, prefix, at-r.Start, false)
	if consumed > at-r.Start {
		return Context{}, false
	}
	// An unfinished delimiter is an editing position before a valid tag exists.
	n := at - r.Start
	if (e.id == language.XML || e.id == language.HTML) && state.mode == data && n > 0 && prefix[n-1] == '<' && (n == len(prefix) || prefix[n] == '>') {
		state.mode = tagName
	}
	return state.Context(), true
}
