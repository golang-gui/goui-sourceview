package sourceview

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/golang-gui/goui-sourceview/internal/editor"
	"github.com/golang-gui/goui-sourceview/internal/highlight"
	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/gui/textedit"
)

type autoPair struct {
	open, close int
	left, right byte
}

// AutoPairs controls insertion, surrounding and deletion of generated pairs.
// It does not affect explicit ReplaceSelection, paste or composed IME commits.
func (v *SourceView) AutoPairs() bool { return v.autoPairs }
func (v *SourceView) SetAutoPairs(enabled bool) {
	v.autoPairs = enabled
	if !enabled {
		v.pairs = nil
	}
}

func (v *SourceView) lexicalContext(at int) (highlight.Context, bool) {
	if v.document.language == language.Plain {
		return highlight.Context{}, true
	}
	if v.document.engine == nil {
		return highlight.Context{}, false
	}
	return v.document.engine.ContextAt(at)
}

func (v *SourceView) pairText(r textedit.Range, text string) (bool, error) {
	if !v.autoPairs || len(text) != 1 {
		return false, nil
	}
	c := text[0]
	if r.Start == r.End {
		for i := len(v.pairs) - 1; i >= 0; i-- {
			p := v.pairs[i]
			if p.close == r.Start && p.right == c && v.pairValid(p) && (c != '"' && c != '\'' || !v.quoteEscaped(r.Start)) {
				v.pairs = append(v.pairs[:i], v.pairs[i+1:]...)
				v.moveTo(r.Start+1, false, false)
				return true, nil
			}
		}
	}
	markup := v.document.language == language.XML || v.document.language == language.HTML
	close := byte(0)
	switch c {
	case '(':
		close = ')'
	case '[':
		close = ']'
	case '{':
		close = '}'
	case '"':
		close = '"'
	case '\'':
		close = '\''
	case '<':
		if markup {
			close = '>'
		}
	}
	if close == 0 {
		return false, nil
	}
	if v.document.language == language.JSON && (c == '(' || c == '\'') {
		return false, nil
	}
	ctx, known := v.lexicalContext(r.Start)
	if !known || ctx.Comment {
		return false, nil
	}
	selected := r.Start != r.End
	if !selected {
		if ctx.String {
			return false, nil
		}
		if markup && (c == '"' || c == '\'') && !ctx.Value {
			return false, nil
		}
		if markup && c == '<' && ctx.Tag {
			return false, nil
		}
		if r.End < v.document.model.Len() {
			end := min(v.document.model.Len(), r.End+4)
			for !v.document.model.IsRuneBoundary(end) {
				end--
			}
			next, _ := v.document.model.Slice(textedit.Range{Start: r.End, End: end})
			rune, _ := utf8.DecodeRuneInString(next)
			if !unicode.IsSpace(rune) && !strings.ContainsRune(")]},:;>", rune) {
				return false, nil
			}
		}
	}
	before := v.Selection()
	content, err := v.document.model.Slice(r)
	if err != nil {
		return true, err
	}
	after := textedit.Selection{Anchor: r.Start + 1, Caret: r.Start + 1}
	if selected {
		after = textedit.Selection{Anchor: before.Anchor + 1, Caret: before.Caret + 1}
	}
	p := editor.Plan{Before: before, After: after, Edits: []textedit.Edit{{Range: r, Text: string(c) + content + string(close)}}}
	state := v.operationState()
	err = v.applyPlan(p)
	if err == nil && v.document == state.document && v.mountEpoch == state.mount && v.document.model.Revision() == state.revision+1 && v.Selection() == after {
		if len(v.pairs) == 1024 {
			copy(v.pairs, v.pairs[1:])
			v.pairs = v.pairs[:1023]
		}
		v.pairs = append(v.pairs, autoPair{r.Start, r.Start + 1 + len(content), c, close})
	}
	return true, err
}

func (v *SourceView) quoteEscaped(at int) bool {
	m := v.document.model
	r, _ := m.LineRange(m.LineAt(at))
	start := max(r.Start, at-(16<<10))
	for !m.IsRuneBoundary(start) {
		start++
	}
	text, _ := m.Slice(textedit.Range{Start: start, End: at})
	n := 0
	for i := len(text) - 1; i >= 0 && text[i] == '\\'; i-- {
		n++
	}
	return n%2 != 0 || n == len(text) && start > r.Start
}

func (v *SourceView) newline() error {
	m := v.document.model
	s := v.Selection()
	base := editor.LeadingIndent(m, s.Caret)
	if s.Anchor == s.Caret && s.Caret > 0 && s.Caret < m.Len() {
		previous := v.adjacent(s.Caret, false)
		next := v.adjacent(s.Caret, true)
		text, _ := m.Slice(textedit.Range{Start: previous, End: next})
		ctx, known := v.lexicalContext(s.Caret)
		if known && !ctx.String && !ctx.Comment && (text == "{}" || text == "[]" || text == "()") {
			unit := "\t"
			if v.indent.InsertSpaces {
				unit = strings.Repeat(" ", v.indent.IndentWidth)
			}
			text = "\n" + base + unit + "\n" + base
			end := s.Caret + 1 + len(base) + len(unit)
			return v.applyPlan(editor.Plan{Before: s, After: textedit.Selection{Anchor: end, Caret: end}, Edits: []textedit.Edit{{Range: s.Range(), Text: text}}})
		}
	}
	return v.ReplaceSelection("\n" + base)
}

func (v *SourceView) pairValid(p autoPair) bool {
	m := v.document.model
	if p.open < 0 || p.close >= m.Len() || p.close <= p.open {
		return false
	}
	a, err := m.Slice(textedit.Range{Start: p.open, End: p.open + 1})
	if err != nil {
		return false
	}
	b, err := m.Slice(textedit.Range{Start: p.close, End: p.close + 1})
	return err == nil && a == string(p.left) && b == string(p.right)
}
func (v *SourceView) deletePair() (bool, error) {
	s := v.Selection()
	if s.Anchor != s.Caret {
		return false, nil
	}
	for _, p := range v.pairs {
		if p.open+1 == s.Caret && p.close == s.Caret && v.pairValid(p) {
			after := textedit.Selection{Anchor: p.open, Caret: p.open}
			return true, v.applyPlan(editor.Plan{Before: s, After: after, Edits: []textedit.Edit{{Range: textedit.Range{Start: p.open, End: p.close + 1}}}})
		}
	}
	return false, nil
}
func (v *SourceView) mapPairs(c textedit.Change) {
	if c.Reset {
		v.pairs = nil
		return
	}
	for _, e := range c.Edits {
		kept := v.pairs[:0]
		for _, p := range v.pairs {
			if e.Range.Start <= p.open && p.open < e.Range.End || e.Range.Start <= p.close && p.close < e.Range.End {
				continue
			}
			p.open = textedit.MapOffset(p.open, e)
			p.close = textedit.MapOffset(p.close, e)
			kept = append(kept, p)
		}
		v.pairs = kept
	}
}

type bracketState struct {
	document *Document
	revision uint64
	caret    int
	language language.ID
	ranges   []textedit.Range
	valid    bool
}

// Matching scans at most 64 KiB on either side and never guesses unresolved
// lexical styles. A typed mismatch highlights the candidate alone.
func (v *SourceView) matchingBrackets() []textedit.Range {
	m := v.document.model
	s := v.Selection()
	if s.Anchor != s.Caret || v.projection != nil {
		return nil
	}
	cache := v.brackets
	if cache.valid && cache.document == v.document && cache.revision == m.Revision() && cache.caret == s.Caret && cache.language == v.document.language {
		return cache.ranges
	}
	result := bracketState{document: v.document, revision: m.Revision(), caret: s.Caret, language: v.document.language}
	start, end := max(0, s.Caret-(64<<10)), min(m.Len(), s.Caret+(64<<10))
	for !m.IsRuneBoundary(start) {
		start++
	}
	for !m.IsRuneBoundary(end) {
		end--
	}
	text, _ := m.Slice(textedit.Range{Start: start, End: end})
	allowed := make([]bool, len(text))
	if v.document.language == language.Plain {
		for i := range allowed {
			allowed[i] = true
		}
	} else {
		e := v.document.engine
		if e == nil {
			return nil
		}
		for row, last := m.LineAt(start), m.LineAt(end); row <= last; row++ {
			tokens, _, valid := e.Tokens(row)
			if !valid {
				e.Ensure(row)
				v.document.requestWork()
				return nil
			}
			r, _ := m.LineRange(row)
			for _, t := range tokens {
				if t.Kind != language.KindDelimiter {
					continue
				}
				for i, limit := max(start, r.Start+t.Start), min(end, r.Start+t.End); i < limit; i++ {
					allowed[i-start] = true
				}
			}
		}
	}
	markup := v.document.language == language.XML || v.document.language == language.HTML
	isBracket := func(i int) bool {
		return i >= 0 && i < len(text) && allowed[i] && (strings.ContainsRune("()[]{}", rune(text[i])) || markup && (text[i] == '<' || text[i] == '>'))
	}
	at := s.Caret - start
	if !isBracket(at) {
		at--
	}
	if !isBracket(at) {
		result.valid = true
		v.brackets = result
		return nil
	}
	left, right := "([{<", ")]}>"
	step := 1
	if strings.IndexByte(right, text[at]) >= 0 {
		step = -1
	}
	stack := []byte{text[at]}
	result.ranges = []textedit.Range{{Start: start + at, End: start + at + 1}}
	for i := at + step; i >= 0 && i < len(text); i += step {
		if !isBracket(i) {
			continue
		}
		c := text[i]
		opening := strings.IndexByte(left, c) >= 0
		if opening == (step > 0) {
			stack = append(stack, c)
			continue
		}
		last := stack[len(stack)-1]
		matched := strings.IndexByte(left, last) == strings.IndexByte(right, c)
		if step < 0 {
			matched = strings.IndexByte(right, last) == strings.IndexByte(left, c)
		}
		if !matched {
			break
		}
		stack = stack[:len(stack)-1]
		if len(stack) == 0 {
			result.ranges = append(result.ranges, textedit.Range{Start: start + i, End: start + i + 1})
			break
		}
	}
	result.valid = true
	v.brackets = result
	return result.ranges
}
