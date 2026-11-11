package sourceview

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/golang-gui/goui-sourceview/internal/editor"
	"github.com/golang-gui/goui-sourceview/internal/highlight"
	textcore "github.com/golang-gui/goui-sourceview/internal/text"
	"github.com/golang-gui/goui-sourceview/language"
)

type autoPair struct {
	open, close int
	left, right string
}

// AutoPairs controls insertion, surrounding and deletion of generated pairs.
// It does not affect explicit ReplaceSelection, paste or composed IME commits.
func (v *SourceView) AutoPairs() bool { return v.autoPairs }
func (v *SourceView) SetAutoPairs(enabled bool) {
	if v.autoPairs == enabled {
		return
	}
	v.editOptionsChanged()
	v.autoPairs = enabled
	if !enabled {
		v.pairs = nil
	}
}

func (v *SourceView) lexicalContext(at int) (highlight.Context, bool) {
	if v.document.definition.Tokenizer() == nil {
		return highlight.Context{}, true
	}
	if v.document.engine == nil {
		return highlight.Context{}, false
	}
	return v.document.engine.ContextAt(at)
}

// pairText handles literal (non-IME) typing; language policy never owns history.
type plannedPair struct {
	plan     editor.Plan
	created  *language.Pair
	consumed *autoPair
}

func (v *SourceView) pairText(r TextRange, text string) (bool, error) {
	result, handled, err := v.pairPlan(cursorState{selection: v.Selection()}, text)
	if !handled || err != nil {
		return handled, err
	}
	return true, v.applyPairPlans([]plannedPair{result}, textcore.MergeNone)
}
func (v *SourceView) pairPlan(c cursorState, text string) (plannedPair, bool, error) {
	r := c.selection.Range()
	if !v.autoPairs || text == "" {
		return plannedPair{}, false, nil
	}
	rules := v.definitionAt(c.selection.Caret).Editing()
	beforeText, afterText := v.lineContext(r)
	selected := r.Start != r.End
	pairs := rules.AutoClosingPairs
	if selected {
		pairs = rules.SurroundingPairs
	}
	// Prefer the longest completed opening sequence before overtyping a shorter
	// generated closer (e.g. a third backtick completing a fenced delimiter).
	var pair language.Pair
	for _, p := range pairs {
		matches := text == p.Open
		if !selected && len(text) <= len(p.Open) {
			matches = strings.HasSuffix(beforeText+text, p.Open)
		}
		if matches && len(p.Open) > len(pair.Open) {
			pair = p
		}
	}
	if !selected && len(pair.Open) <= len(text) {
		for i := len(v.pairs) - 1; i >= 0; i-- {
			p := v.pairs[i]
			at := r.Start - p.close
			if at >= 0 && at < len(p.right) && strings.HasPrefix(p.right[at:], text) && v.pairValid(p) && (p.right != `"` && p.right != "'" || !v.quoteEscaped(r.Start)) {
				after := TextSelection{Anchor: r.Start + len(text), Caret: r.Start + len(text)}
				result := plannedPair{plan: editor.Plan{Before: c.selection, After: after}}
				if at+len(text) == len(p.right) {
					copy := p
					result.consumed = &copy
				}
				return result, true, nil
			}
		}
	}
	if pair.Open == "" {
		return plannedPair{}, false, nil
	}
	ctx, known := v.lexicalContext(r.Start)
	if !known || pair.NotInComment && ctx.Comment || !selected && pair.NotInString && ctx.String {
		return plannedPair{}, false, nil
	}
	state := v.operationState()
	if rules.AllowPair != nil {
		allowed := rules.AllowPair(language.PairContext{Typed: text, Before: beforeText, After: afterText, Context: ctx, Known: known, Selected: selected})
		if !state.editingValid(v) {
			return plannedPair{}, true, ErrOperationChanged
		}
		if !allowed {
			return plannedPair{}, false, nil
		}
	}
	if !selected && afterText != "" {
		next, _ := utf8.DecodeRuneInString(afterText)
		if !unicode.IsSpace(next) && !strings.ContainsRune(rules.AutoCloseBefore, next) {
			return plannedPair{}, false, nil
		}
	}
	before := c.selection
	content, err := v.document.model.Slice(r)
	if err != nil {
		return plannedPair{}, true, err
	}
	inserted := text + content + pair.Close
	after := TextSelection{Anchor: r.Start + len(text), Caret: r.Start + len(text)}
	if selected {
		after = TextSelection{Anchor: before.Anchor + len(text), Caret: before.Caret + len(text)}
	}
	return plannedPair{plan: editor.Plan{Before: before, After: after, Edits: []TextEdit{{Range: r, Text: inserted}}}, created: &pair}, true, nil
}

// lineContext copies at most 16 KiB on each side, excluding LF.
func (v *SourceView) lineContext(r TextRange) (string, string) {
	m := v.document.model
	left, _ := m.LineRange(m.LineAt(r.Start))
	right, _ := m.LineRange(m.LineAt(r.End))
	start, end := max(left.Start, r.Start-(16<<10)), min(right.End, r.End+(16<<10))
	for !m.IsRuneBoundary(start) {
		start++
	}
	for !m.IsRuneBoundary(end) {
		end--
	}
	before, _ := m.Slice(TextRange{Start: start, End: r.Start})
	after, _ := m.Slice(TextRange{Start: r.End, End: end})
	return before, after
}

func (v *SourceView) quoteEscaped(at int) bool {
	if ctx, known := v.lexicalContext(at); known && ctx.Literal {
		return false
	}
	m := v.document.model
	r, _ := m.LineRange(m.LineAt(at))
	start := max(r.Start, at-(16<<10))
	for !m.IsRuneBoundary(start) {
		start++
	}
	text, _ := m.Slice(TextRange{Start: start, End: at})
	escape := v.definitionAt(at).Editing().QuoteEscape
	if escape == 0 {
		escape = '\\'
	}
	n := 0
	left := len(text)
	for left > 0 {
		r, size := utf8.DecodeLastRuneInString(text[:left])
		if r != escape {
			break
		}
		n++
		left -= size
	}
	return n%2 != 0 || left == 0 && start > r.Start
}

func (v *SourceView) newline() error {
	if len(v.additional) > 0 {
		return v.multiplePlans(v.multipleNewlinePlan)
	}
	p, err := v.multipleNewlinePlan(cursorState{selection: v.Selection()})
	if err != nil {
		return err
	}
	return v.applyPlan(p)
}
func (v *SourceView) multipleNewlinePlan(c cursorState) (editor.Plan, error) {
	m := v.document.model
	s := c.selection
	r := s.Range()
	base := editor.LeadingIndent(m, s.Caret)
	unit := "\t"
	if v.indent.InsertSpaces {
		unit = strings.Repeat(" ", v.indent.IndentWidth)
	}
	before, after := v.lineContext(r)
	ctx, known := v.lexicalContext(r.Start)
	rules := v.definitionAt(s.Caret).Editing()
	if rules.Newline != nil {
		state := v.operationState()
		edit, handled := rules.Newline(language.NewlineContext{Before: before, After: after, Indent: base, Unit: unit, Context: ctx, Known: known})
		if !state.editingValid(v) {
			return editor.Plan{}, ErrOperationChanged
		}
		if handled {
			if len(edit.Text) > 64<<10 || edit.DeleteBefore < 0 || edit.DeleteAfter < 0 || edit.DeleteBefore > len(before) || edit.DeleteAfter > len(after) || edit.Caret < 0 || edit.Caret > len(edit.Text) || !utf8.ValidString(edit.Text) || !utf8.ValidString(edit.Text[:edit.Caret]) || strings.ContainsRune(edit.Text, '\r') {
				return editor.Plan{}, fmt.Errorf("sourceview: invalid newline edit")
			}
			rng := TextRange{Start: r.Start - edit.DeleteBefore, End: r.End + edit.DeleteAfter}
			end := rng.Start + edit.Caret
			return editor.Plan{Before: s, After: TextSelection{Anchor: end, Caret: end}, Edits: []TextEdit{{Range: rng, Text: edit.Text}}}, nil
		}
	}
	if s.Anchor == s.Caret && known && !ctx.String && !ctx.Comment {
		for _, p := range rules.Brackets {
			if strings.HasSuffix(before, p.Open) && strings.HasPrefix(after, p.Close) {
				text := "\n" + base + unit + "\n" + base
				end := s.Caret + 1 + len(base) + len(unit)
				return editor.Plan{Before: s, After: TextSelection{Anchor: end, Caret: end}, Edits: []TextEdit{{Range: r, Text: text}}}, nil
			}
		}
	}
	return replacementPlan(c, "\n"+base), nil
}

func (v *SourceView) pairValid(p autoPair) bool {
	m := v.document.model
	if p.open < 0 || p.close+len(p.right) > m.Len() || p.close <= p.open {
		return false
	}
	a, err := m.Slice(TextRange{Start: p.open, End: p.open + len(p.left)})
	if err != nil {
		return false
	}
	b, err := m.Slice(TextRange{Start: p.close, End: p.close + len(p.right)})
	return err == nil && a == p.left && b == p.right
}
func (v *SourceView) deletePair() (bool, error) {
	s := v.Selection()
	if s.Anchor != s.Caret {
		return false, nil
	}
	for _, p := range v.pairs {
		if p.open+len(p.left) == s.Caret && p.close == s.Caret && v.pairValid(p) {
			after := TextSelection{Anchor: p.open, Caret: p.open}
			return true, v.applyPlan(editor.Plan{Before: s, After: after, Edits: []TextEdit{{Range: TextRange{Start: p.open, End: p.close + len(p.right)}}}})
		}
	}
	return false, nil
}
func (v *SourceView) mapPairs(c TextChange) {
	if c.Reset {
		v.pairs = nil
		return
	}
	// Normal batches are one descending run. Merged undo/redo can contain
	// several sequential runs, each with its own original coordinate system.
	for first := 0; first < len(c.Edits) && len(v.pairs) > 0; {
		end := first + 1
		for end < len(c.Edits) && c.Edits[end].Range.End <= c.Edits[end-1].Range.Start {
			end++
		}
		edits := c.Edits[first:end]
		touches := func(at, size int) bool {
			i := sort.Search(len(edits), func(i int) bool { return edits[len(edits)-1-i].Range.End > at })
			return i < len(edits) && editor.TouchesPair(edits[len(edits)-1-i].Range, at, size)
		}
		kept := v.pairs[:0]
		var points []int
		for _, pair := range v.pairs {
			if touches(pair.open, len(pair.left)) || touches(pair.close, len(pair.right)) {
				continue
			}
			kept = append(kept, pair)
			points = append(points, pair.open, pair.close)
		}
		points = textcore.MapOffsets(points, edits)
		for i := range kept {
			kept[i].open, kept[i].close = points[2*i], points[2*i+1]
		}
		v.pairs = kept
		first = end
	}
}

type bracketState struct {
	document *Document
	revision uint64
	caret    int
	language uint64
	ranges   []TextRange
	valid    bool
}

// Matching scans at most 64 KiB on either side and never guesses unresolved
// lexical styles. A single range identifies a candidate; only two ranges confirm
// a pair and qualify for matching decoration.
func (v *SourceView) matchingBrackets() []TextRange { return v.matchingBracketsAt(v.Selection()) }
func (v *SourceView) matchingBracketsAt(s TextSelection) []TextRange {
	m := v.document.model
	if s.Anchor != s.Caret || v.projection != nil {
		return nil
	}

	cache := v.brackets
	if cache.valid && cache.document == v.document && cache.revision == m.Revision() && cache.caret == s.Caret && cache.language == v.document.languageEpoch {
		return cache.ranges
	}
	result := bracketState{document: v.document, revision: m.Revision(), caret: s.Caret, language: v.document.languageEpoch}
	if ranges := v.matchingTagsAt(s.Caret); len(ranges) > 0 {
		result.valid = true
		result.ranges = ranges
		v.brackets = result
		return ranges
	}
	context, known := v.lexicalContext(s.Caret)
	definition := v.document.definition
	if known && context.Embedded != nil {
		definition = context.Embedded
	}
	rules := definition.Editing()
	candidates := editor.BracketCandidates(m, s.Caret, rules.Brackets)
	if len(candidates) == 0 {
		// No neighboring sequence can become a match. Do not demand lexical
		// payload for an entire search window (possibly far offscreen) just to
		// establish that ordinary text has no bracket at this caret.
		if known {
			result.valid = true
			v.brackets = result
		}
		return nil
	}
	if v.document.definition.Tokenizer() != nil {
		// First validate only the adjacent candidate's logical line. A '<'
		// inside an XML declaration (or '[' in a string) must not reload all
		// evicted payload in the 128-KiB search window one paint at a time.
		e := v.document.engine
		if e == nil {
			return nil
		}
		row := m.LineAt(s.Caret)
		tokens, _, ready := e.Tokens(row)
		if !ready {
			e.Ensure(row)
			v.document.requestWork()
			return nil
		}
		line, _ := m.LineRange(row)
		structural := false
		for _, candidate := range candidates {
			at := candidate.Start - line.Start
			for _, token := range tokens {
				if token.End <= at {
					continue
				}
				if token.Start > at || token.Class&language.ClassDelimiter == 0 {
					break
				}
				at = token.End
				if at >= candidate.End-line.Start {
					structural = true
					break
				}
			}
			if structural {
				break
			}
		}
		if !structural {
			if known {
				result.valid = true
				v.brackets = result
			}
			return nil
		}
	}
	start, end := max(0, s.Caret-(64<<10)), min(m.Len(), s.Caret+(64<<10))
	for !m.IsRuneBoundary(start) {
		start++
	}
	for !m.IsRuneBoundary(end) {
		end--
	}
	if span, ok := v.embeddedSpan(s.Caret); ok {
		start = max(start, span.Start)
		end = min(end, span.End)
	}
	text, _ := m.Slice(TextRange{Start: start, End: end})
	allowed := make([]bool, len(text))
	if v.document.definition.Tokenizer() == nil {
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
				if t.Class&language.ClassDelimiter == 0 {
					continue
				}
				for i, limit := max(start, r.Start+t.Start), min(end, r.Start+t.End); i < limit; i++ {
					allowed[i-start] = true
				}
			}
		}
	}
	result.ranges = editor.MatchingBrackets(text, start, s.Caret, allowed, rules.Brackets)
	result.valid = true
	v.brackets = result
	return result.ranges
}
