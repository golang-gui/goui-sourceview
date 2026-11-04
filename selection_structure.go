package sourceview

import (
	"strings"

	"github.com/golang-gui/goui-sourceview/internal/editor"
	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
)

// MoveWordPart moves across camel-case, acronym, digit and underscore boundaries.
// Without extension, a nonempty selection collapses towards the requested side.
func (v *SourceView) MoveWordPart(forward, extend bool) {
	if v.Destroyed() || !gui.IsEnabled(v) {
		return
	}
	s := v.Selection()
	at := s.Caret
	if !extend && s.Anchor != s.Caret {
		at = s.Range().Start
		if forward {
			at = s.Range().End
		}
	} else {
		at = editor.WordPartBoundary(v.document.model, at, forward, false, v.definitionAt(at).IsWord)
	}
	v.moveTo(v.foldNavigationTarget(at, forward), false, extend)
}

// DeleteWordPart deletes one component or the selection as one undo item.
func (v *SourceView) DeleteWordPart(forward bool) error {
	if err := v.editable(); err != nil {
		return err
	}
	if v.unfoldDeleteBoundary(forward) {
		return nil
	}
	s := v.Selection()
	r := s.Range()
	if r.Start == r.End {
		at := editor.WordPartBoundary(v.document.model, s.Caret, forward, true, v.definitionAt(s.Caret).IsWord)
		if forward {
			r.End = v.snapOffset(at, true)
		} else {
			r.Start = v.snapOffset(at, false)
		}
	}
	if r.Start == r.End {
		return nil
	}
	if s.Anchor == s.Caret && v.expandFoldRange(r) {
		v.foldingSignal.Emit()
		return nil
	}
	return v.applyPlan(editor.Plan{Before: s, After: textedit.Selection{Anchor: r.Start, Caret: r.Start}, Edits: []textedit.Edit{{Range: r}}})
}

type selectionTrail struct {
	state operationState
	steps []textedit.Selection
}

// ExpandSelection selects the smallest confirmed enclosing word, literal,
// bracket/tag region, logical line or document. Analysis never changes a later selection.
func (v *SourceView) ExpandSelection() bool {
	if v.Destroyed() || !gui.IsEnabled(v) || v.projection != nil {
		return false
	}
	state := v.operationState()
	s := v.Selection()
	r := s.Range()
	m := v.document.model
	if !v.selectionTrail.state.valid(v) {
		v.selectionTrail.steps = nil
	}
	best := textedit.Range{Start: 0, End: m.Len()}
	found := best != r
	add := func(q textedit.Range) {
		if q.Start <= r.Start && q.End >= r.End && q != r && (!found || q.End-q.Start < best.End-best.Start) {
			best = q
			found = true
		}
	}
	add(editor.WordRange(m, r.Start, v.definitionAt(s.Caret).IsWord))
	a, _ := m.LineRange(m.LineAt(r.Start))
	b, _ := m.LineRange(m.LineAt(r.End))
	add(textedit.Range{Start: a.Start, End: b.End})
	add(textedit.Range{Start: a.Start, End: min(m.Len(), b.End+1)})
	v.structureCandidates(r, add)
	if !found || !state.valid(v) {
		return false
	}
	steps := append([]textedit.Selection(nil), v.selectionTrail.steps...)
	if len(steps) == 64 {
		steps = steps[1:]
	}
	steps = append(steps, s)
	next := textedit.Selection{Anchor: best.Start, Caret: best.End}
	if s.Anchor > s.Caret {
		next.Anchor, next.Caret = next.Caret, next.Anchor
	}
	v.document.model.BreakUndoGroup()
	v.setSelection(next, false, true)
	if v.document != state.document || v.mountEpoch != state.mount || v.document.model.Revision() != state.revision || v.selectionEpoch != state.selection+1 {
		return false
	}
	v.selectionTrail = selectionTrail{v.operationState(), steps}
	return true
}

// ShrinkSelection restores the preceding actual selection, including direction.
func (v *SourceView) ShrinkSelection() bool {
	t := v.selectionTrail
	if v.Destroyed() || !gui.IsEnabled(v) || v.projection != nil || !t.state.valid(v) || len(t.steps) == 0 {
		v.selectionTrail.steps = nil
		return false
	}
	s := t.steps[len(t.steps)-1]
	v.setSelection(s, false, true)
	if v.document != t.state.document || v.mountEpoch != t.state.mount || v.document.model.Revision() != t.state.revision || v.selectionEpoch != t.state.selection+1 {
		return false
	}
	v.selectionTrail = selectionTrail{v.operationState(), t.steps[:len(t.steps)-1]}
	return true
}

func (v *SourceView) structureCandidates(r textedit.Range, add func(textedit.Range)) {
	m := v.document.model
	start, end := max(0, r.Start-64*1024), min(m.Len(), r.End+64*1024)
	if end-start > 128*1024 {
		return
	}
	for !m.IsRuneBoundary(start) {
		start++
	}
	for !m.IsRuneBoundary(end) {
		end--
	}
	text, _ := m.Slice(textedit.Range{Start: start, End: end})
	rules := v.definitionAt(r.Start).Editing()
	// Markup parsing needs a known region boundary, not an arbitrary text suffix.
	if rules.Tags != nil && start == 0 && end == m.Len() {
		for _, q := range rules.Tags.SelectionRanges(text, language.NameRange{Start: r.Start, End: r.End}) {
			add(textedit.Range{Start: q.Start, End: q.End})
		}
	}
	allowed := make([]bool, len(text))
	known := make([]bool, len(text))
	e := v.document.engine
	if e == nil {
		return
	}
	for row, last := m.LineAt(start), m.LineAt(end); row <= last; row++ {
		ts, _, ok := e.Tokens(row)
		if !ok {
			continue
		}
		line, _ := m.LineRange(row)
		for at := max(start, line.Start); at < min(end, line.End+1); at++ {
			known[at-start] = true
		}
		stringToken := func(t language.Token) bool {
			if t.Class&language.ClassString != 0 {
				return true
			}
			at := line.Start + t.Start
			if at < start || at >= end {
				return false
			}
			// Quoted keys may carry a property style without ClassString. Confirm
			// their lexical context instead of inferring behavior from a style key.
			if strings.ContainsRune("\"'`", rune(text[at-start])) {
				at++
			}
			c, known := v.lexicalContext(at)
			return known && c.String
		}
		for i := 0; i < len(ts); i++ {
			t := ts[i]
			a, b := line.Start+t.Start, line.Start+t.End
			if t.Class&language.ClassDelimiter != 0 {
				for at := max(start, a); at < min(end, b); at++ {
					allowed[at-start] = true
				}
			}
			if !stringToken(t) || a < start {
				continue
			}
			for i+1 < len(ts) && ts[i+1].Start == ts[i].End && stringToken(ts[i+1]) {
				i++
				b = line.Start + ts[i].End
			}
			if b > end || b-a < 2 {
				continue
			}
			if ctx, known := v.lexicalContext(b); !known || ctx.String {
				continue
			}
			literal := text[a-start : b-start]
			for _, p := range rules.SurroundingPairs {
				if p.Open != p.Close || !strings.HasPrefix(literal, p.Open) || !strings.HasSuffix(literal, p.Close) || len(literal) < len(p.Open)+len(p.Close) {
					continue
				}
				add(textedit.Range{Start: a + len(p.Open), End: b - len(p.Close)})
				add(textedit.Range{Start: a, End: b})
			}
		}
	}
	type opener struct{ at, end, pair int }
	var stack []opener
	for at := 0; at < len(text); {
		if !known[at] {
			stack = nil
			at++
			continue
		}
		if !allowed[at] {
			at++
			continue
		}
		pair, side, size := -1, false, 0
		for i, p := range rules.Brackets {
			for j, seq := range []string{p.Open, p.Close} {
				if len(seq) <= size || !strings.HasPrefix(text[at:], seq) {
					continue
				}
				valid := true
				for k := at; k < at+len(seq); k++ {
					valid = valid && allowed[k]
				}
				if valid {
					pair, side, size = i, j == 0, len(seq)
				}
			}
		}
		if pair < 0 {
			at++
			continue
		}
		if side {
			stack = append(stack, opener{at, at + size, pair})
		} else if len(stack) > 0 {
			o := stack[len(stack)-1]
			if o.pair != pair {
				stack = nil
			} else {
				stack = stack[:len(stack)-1]
				add(textedit.Range{Start: start + o.end, End: start + at})
				add(textedit.Range{Start: start + o.at, End: start + at + size})
			}
		}
		at += size
	}
}
