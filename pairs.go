package sourceview

import (
	"fmt"
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
	left, right string
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
	if v.document.definition.Tokenizer() == nil {
		return highlight.Context{}, true
	}
	if v.document.engine == nil {
		return highlight.Context{}, false
	}
	return v.document.engine.ContextAt(at)
}

// pairText handles literal (non-IME) typing; language policy never owns history.
func (v *SourceView) pairText(r textedit.Range, text string) (bool, error) {
	if !v.autoPairs || text == "" {
		return false, nil
	}
	rules := v.definitionAt(v.Selection().Caret).Editing()
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
				if at+len(text) == len(p.right) {
					v.pairs = append(v.pairs[:i], v.pairs[i+1:]...)
				}
				v.moveTo(r.Start+len(text), false, false)
				return true, nil
			}
		}
	}
	if pair.Open == "" {
		return false, nil
	}
	ctx, known := v.lexicalContext(r.Start)
	if !known || pair.NotInComment && ctx.Comment || !selected && pair.NotInString && ctx.String {
		return false, nil
	}
	state := v.operationState()
	if rules.AllowPair != nil {
		allowed := rules.AllowPair(language.PairContext{Typed: text, Before: beforeText, After: afterText, Context: ctx, Known: known, Selected: selected})
		if !state.valid(v) {
			return true, ErrOperationChanged
		}
		if !allowed {
			return false, nil
		}
	}
	if !selected && afterText != "" {
		next, _ := utf8.DecodeRuneInString(afterText)
		if !unicode.IsSpace(next) && !strings.ContainsRune(rules.AutoCloseBefore, next) {
			return false, nil
		}
	}
	before := v.Selection()
	content, err := v.document.model.Slice(r)
	if err != nil {
		return true, err
	}
	inserted := text + content + pair.Close
	after := textedit.Selection{Anchor: r.Start + len(text), Caret: r.Start + len(text)}
	if selected {
		after = textedit.Selection{Anchor: before.Anchor + len(text), Caret: before.Caret + len(text)}
	}
	err = v.applyPlan(editor.Plan{Before: before, After: after, Edits: []textedit.Edit{{Range: r, Text: inserted}}})
	if err == nil && v.document == state.document && v.mountEpoch == state.mount && v.document.languageEpoch == state.language && v.document.model.Revision() == state.revision+1 && v.Selection() == after {
		if len(v.pairs) == 1024 {
			copy(v.pairs, v.pairs[1:])
			v.pairs = v.pairs[:1023]
		}
		v.pairs = append(v.pairs, autoPair{r.Start + len(text) - len(pair.Open), r.Start + len(text) + len(content), pair.Open, pair.Close})
	}
	return true, err
}

// lineContext copies at most 16 KiB on each side, excluding LF.
func (v *SourceView) lineContext(r textedit.Range) (string, string) {
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
	before, _ := m.Slice(textedit.Range{Start: start, End: r.Start})
	after, _ := m.Slice(textedit.Range{Start: r.End, End: end})
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
	r := s.Range()
	base := editor.LeadingIndent(m, s.Caret)
	unit := "\t"
	if v.indent.InsertSpaces {
		unit = strings.Repeat(" ", v.indent.IndentWidth)
	}
	before, after := v.lineContext(r)
	ctx, known := v.lexicalContext(r.Start)
	rules := v.definitionAt(v.Selection().Caret).Editing()
	if rules.Newline != nil {
		state := v.operationState()
		edit, handled := rules.Newline(language.NewlineContext{Before: before, After: after, Indent: base, Unit: unit, Context: ctx, Known: known})
		if !state.valid(v) {
			return ErrOperationChanged
		}
		if handled {
			if len(edit.Text) > 64<<10 || edit.DeleteBefore < 0 || edit.DeleteAfter < 0 || edit.DeleteBefore > len(before) || edit.DeleteAfter > len(after) || edit.Caret < 0 || edit.Caret > len(edit.Text) || !utf8.ValidString(edit.Text) || !utf8.ValidString(edit.Text[:edit.Caret]) || strings.ContainsRune(edit.Text, '\r') {
				return fmt.Errorf("sourceview: invalid newline edit")
			}
			rng := textedit.Range{Start: r.Start - edit.DeleteBefore, End: r.End + edit.DeleteAfter}
			end := rng.Start + edit.Caret
			return v.applyPlan(editor.Plan{Before: s, After: textedit.Selection{Anchor: end, Caret: end}, Edits: []textedit.Edit{{Range: rng, Text: edit.Text}}})
		}
	}
	if s.Anchor == s.Caret && known && !ctx.String && !ctx.Comment {
		for _, p := range rules.Brackets {
			if strings.HasSuffix(before, p.Open) && strings.HasPrefix(after, p.Close) {
				text := "\n" + base + unit + "\n" + base
				end := s.Caret + 1 + len(base) + len(unit)
				return v.applyPlan(editor.Plan{Before: s, After: textedit.Selection{Anchor: end, Caret: end}, Edits: []textedit.Edit{{Range: r, Text: text}}})
			}
		}
	}
	return v.replace(v.Selection().Range(), "\n"+base, true)
}

func (v *SourceView) pairValid(p autoPair) bool {
	m := v.document.model
	if p.open < 0 || p.close+len(p.right) > m.Len() || p.close <= p.open {
		return false
	}
	a, err := m.Slice(textedit.Range{Start: p.open, End: p.open + len(p.left)})
	if err != nil {
		return false
	}
	b, err := m.Slice(textedit.Range{Start: p.close, End: p.close + len(p.right)})
	return err == nil && a == p.left && b == p.right
}
func (v *SourceView) deletePair() (bool, error) {
	s := v.Selection()
	if s.Anchor != s.Caret {
		return false, nil
	}
	for _, p := range v.pairs {
		if p.open+len(p.left) == s.Caret && p.close == s.Caret && v.pairValid(p) {
			after := textedit.Selection{Anchor: p.open, Caret: p.open}
			return true, v.applyPlan(editor.Plan{Before: s, After: after, Edits: []textedit.Edit{{Range: textedit.Range{Start: p.open, End: p.close + len(p.right)}}}})
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
			if touchesPair(e.Range, p.open, len(p.left)) || touchesPair(e.Range, p.close, len(p.right)) {
				continue
			}
			p.open = textedit.MapOffset(p.open, e)
			p.close = textedit.MapOffset(p.close, e)
			kept = append(kept, p)
		}
		v.pairs = kept
	}
}

func touchesPair(r textedit.Range, at, size int) bool {
	if r.Start == r.End {
		return r.Start > at && r.Start < at+size
	}
	return r.Start < at+size && r.End > at
}

type bracketState struct {
	document *Document
	revision uint64
	caret    int
	language uint64
	ranges   []textedit.Range
	valid    bool
}

// Matching scans at most 64 KiB on either side and never guesses unresolved
// lexical styles. A single range identifies a candidate; only two ranges confirm
// a pair and qualify for matching decoration.
func (v *SourceView) matchingBrackets() []textedit.Range {
	m := v.document.model
	s := v.Selection()
	if s.Anchor != s.Caret || v.projection != nil {
		return nil
	}

	cache := v.brackets
	if cache.valid && cache.document == v.document && cache.revision == m.Revision() && cache.caret == s.Caret && cache.language == v.document.languageEpoch {
		return cache.ranges
	}
	result := bracketState{document: v.document, revision: m.Revision(), caret: s.Caret, language: v.document.languageEpoch}
	if ranges := v.matchingTags(); len(ranges) > 0 {
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
	candidates := v.bracketCandidates(s.Caret, rules.Brackets)
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
	text, _ := m.Slice(textedit.Range{Start: start, End: end})
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
	type delimiter struct {
		at, end, pair int
		open          bool
	}
	var delimiters []delimiter
	for i := 0; i < len(text); {
		best := delimiter{}
		found := false
		for index, p := range rules.Brackets {
			for side, seq := range []string{p.Open, p.Close} {
				if i+len(seq) > len(text) || !strings.HasPrefix(text[i:], seq) {
					continue
				}
				valid := true
				for j := i; j < i+len(seq); j++ {
					if !allowed[j] {
						valid = false
						break
					}
				}
				if valid && (!found || len(seq) > best.end-best.at) {
					best = delimiter{i, i + len(seq), index, side == 0}
					found = true
				}
			}
		}
		if found {
			delimiters = append(delimiters, best)
			i = best.end
		} else {
			i++
		}
	}
	at := -1
	local := s.Caret - start
	for i, d := range delimiters {
		if d.at <= local && local < d.end {
			at = i
			break
		}
	}
	if at < 0 {
		for i, d := range delimiters {
			if d.end == local {
				at = i
				break
			}
		}
	}
	if at < 0 {
		result.valid = true
		v.brackets = result
		return nil
	}
	current := delimiters[at]
	step := 1
	if !current.open {
		step = -1
	}
	stack := []int{current.pair}
	result.ranges = []textedit.Range{{Start: start + current.at, End: start + current.end}}
	for i := at + step; i >= 0 && i < len(delimiters); i += step {
		d := delimiters[i]
		if d.open == current.open {
			stack = append(stack, d.pair)
			continue
		}
		if stack[len(stack)-1] != d.pair {
			break
		}
		stack = stack[:len(stack)-1]
		if len(stack) == 0 {
			result.ranges = append(result.ranges, textedit.Range{Start: start + d.at, End: start + d.end})
			break
		}
	}
	result.valid = true
	v.brackets = result
	return result.ranges
}

func (v *SourceView) bracketCandidates(caret int, pairs []language.Pair) []textedit.Range {
	longest := 0
	for _, p := range pairs {
		longest = max(longest, len(p.Open), len(p.Close))
	}
	if longest == 0 {
		return nil
	}
	m := v.document.model
	start, end := max(0, caret-longest), min(m.Len(), caret+longest)
	for !m.IsRuneBoundary(start) {
		start--
	}
	for !m.IsRuneBoundary(end) {
		end++
	}
	text, _ := m.Slice(textedit.Range{Start: start, End: end})
	local := caret - start
	var candidates []textedit.Range
	for _, p := range pairs {
		for _, sequence := range []string{p.Open, p.Close} {
			for at := max(0, local-len(sequence)); at <= local && at+len(sequence) <= len(text); at++ {
				if strings.HasPrefix(text[at:], sequence) {
					candidates = append(candidates, textedit.Range{Start: start + at, End: start + at + len(sequence)})
				}
			}
		}
	}
	return candidates
}
