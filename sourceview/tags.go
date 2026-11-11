package sourceview

import (
	"sort"
	"strings"

	"github.com/golang-gui/goui-sourceview/internal/editor"
	"github.com/golang-gui/goui-sourceview/language"
)

type tagMatchState struct {
	bracketState
	definition *language.Language
}

func (v *SourceView) definitionAt(at int) *language.Language {
	if ctx, ok := v.lexicalContext(at); ok && ctx.Embedded != nil {
		return ctx.Embedded
	}
	return v.document.definition
}

// Tag assistance validates an immutable source region and never treats a
// random truncated prefix as a document boundary. Root markup uses the whole
// current document; embedded regions require analyzed boundaries.
// embeddedSpan finds a fully analyzed contiguous child region. Unresolved
// boundaries fail closed; no language ID has special treatment here.
func (v *SourceView) embeddedSpan(at int) (TextRange, bool) {
	m := v.document.model
	ctx, ok := v.lexicalContext(at)
	if !ok || ctx.Embedded == nil {
		return TextRange{}, false
	}
	first, last := m.LineAt(at), m.LineAt(at)
	for first > 0 {
		if r, _ := m.LineRange(first); at-r.Start > 64<<10 {
			return TextRange{}, false
		}
		r, _ := m.LineRange(first - 1)
		c, known := v.lexicalContext(r.End)
		if !known || c.Embedded != ctx.Embedded {
			break
		}
		first--
	}
	for last+1 < m.LineCount() {
		if r, _ := m.LineRange(last); r.End-at > 64<<10 {
			return TextRange{}, false
		}
		r, _ := m.LineRange(last + 1)
		c, known := v.lexicalContext(r.End)
		if !known || c.Embedded != ctx.Embedded {
			break
		}
		last++
	}
	a, _ := m.LineRange(first)
	b, _ := m.LineRange(last)
	return TextRange{Start: a.Start, End: b.End}, true
}
func (v *SourceView) tagText() (string, *language.TagRules, int) {
	return v.tagTextAt(v.Selection().Caret)
}
func (v *SourceView) tagTextAt(at int) (string, *language.TagRules, int) {
	rules := v.definitionAt(at).Editing().Tags
	if rules == nil {
		return "", nil, 0
	}
	r := TextRange{End: v.document.model.Len()}
	if ctx, ok := v.lexicalContext(at); ok && ctx.Embedded != nil {
		var valid bool
		r, valid = v.embeddedSpan(at)
		if !valid {
			return "", nil, 0
		}
	}
	text, _ := v.document.model.Slice(r)
	return text, rules, r.Start
}
func (v *SourceView) matchingTags() []TextRange { return v.matchingTagsAt(v.Selection().Caret) }
func (v *SourceView) matchingTagsAt(at int) []TextRange {
	definition := v.definitionAt(at)
	cache := v.tagMatches
	m := v.document.model
	if cache.valid && cache.document == v.document && cache.revision == m.Revision() && cache.caret == at && cache.language == v.document.languageEpoch && cache.definition == definition {
		return cache.ranges
	}
	text, rules, base := v.tagTextAt(at)
	if rules == nil {
		return nil
	}
	result := tagMatchState{bracketState: bracketState{document: v.document, revision: m.Revision(), caret: at, language: v.document.languageEpoch, valid: true}, definition: definition}
	match, ok := rules.Match(text, at-base)
	if ok {
		result.ranges = []TextRange{{Start: base + match.Opening.Start, End: base + match.Opening.End}, {Start: base + match.Closing.Start, End: base + match.Closing.End}}
	}
	// A negative markup result is independent of the bracket scanner's lexical
	// readiness. Keep it while that scanner waits, rather than parsing the whole
	// document again on each paint/analysis retry.
	v.tagMatches = result
	return result.ranges
}
func (v *SourceView) closeTag(r TextRange, typed string) (bool, error) {
	p, ok, err := v.closeTagPlan(cursorState{selection: v.Selection()}, r, typed)
	if !ok || err != nil {
		return ok, err
	}
	return true, v.applyPlan(p)
}
func (v *SourceView) closeTagPlan(c cursorState, r TextRange, typed string) (editor.Plan, bool, error) {
	if !v.autoPairs || typed != ">" || r.Start != r.End {
		return editor.Plan{}, false, nil
	}
	text, rules, base := v.tagTextAt(c.selection.Caret)
	if rules == nil {
		return editor.Plan{}, false, nil
	}
	close := rules.ClosingTag(text[:r.Start-base] + ">")
	if close == "" || strings.HasPrefix(text[r.Start-base:], close) {
		return editor.Plan{}, false, nil
	}
	// Consume a generated angle closer only when its exact ownership is valid.
	for _, p := range v.pairs {
		if p.close == r.Start && p.right == ">" && v.pairValid(p) {
			r.End++
			break
		}
	}
	at := r.Start + 1
	return editor.Plan{Before: c.selection, After: TextSelection{Anchor: at, Caret: at}, Edits: []TextEdit{{Range: r, Text: ">" + close}}}, true, nil
}

// Linked name edits revalidate both ranges against the original revision and
// make one ApplyBatch. External edits, paste and IME never trigger a mirror edit.
func (v *SourceView) linkedTag(r TextRange, insert string) (bool, error) {
	p, ok, err := v.linkedTagPlan(cursorState{selection: v.Selection()}, r, insert)
	if !ok || err != nil {
		return ok, err
	}
	return true, v.applyPlan(p)
}
func (v *SourceView) linkedTagPlan(c cursorState, r TextRange, insert string) (editor.Plan, bool, error) {
	if !v.linkedTags {
		return editor.Plan{}, false, nil
	}
	text, rules, base := v.tagTextAt(c.selection.Caret)
	if rules == nil {
		return editor.Plan{}, false, nil
	}
	match, ok := rules.Match(text, r.Start-base)
	if !ok {
		return editor.Plan{}, false, nil
	}
	a, b := match.Opening, match.Closing
	a.Start += base
	a.End += base
	b.Start += base
	b.End += base
	if r.Start >= b.Start && r.End <= b.End {
		a, b = b, a
	}
	if r.Start < a.Start || r.End > a.End {
		return editor.Plan{}, false, nil
	}
	// Explicit cursors in both names own their edits. Mirroring again would
	// duplicate boundary insertions that are otherwise non-overlapping batches.
	if len(v.additional) > 0 {
		for _, other := range v.cursors() {
			q := other.selection.Range()
			if q.Start == q.End && q.Start >= b.Start && q.Start <= b.End || q.Start < b.End && q.End > b.Start {
				return editor.Plan{}, false, nil
			}
		}
	}
	name := text[a.Start-base:r.Start-base] + insert + text[r.End-base:a.End-base]
	if !rules.ValidName(name) {
		return editor.Plan{}, false, nil
	}
	mirror := TextEdit{Range: TextRange{Start: b.Start, End: b.End}, Text: name}
	edits := []TextEdit{{Range: r, Text: insert}, mirror}
	sort.Slice(edits, func(i, j int) bool { return edits[i].Range.Start < edits[j].Range.Start })
	caret := r.Start + len(insert)
	if b.End <= r.Start {
		caret += len(name) - (b.End - b.Start)
	}
	return editor.Plan{Before: c.selection, After: TextSelection{Anchor: caret, Caret: caret}, Edits: edits}, true, nil
}

// LinkedTags controls typed/deleted name mirroring on confirmed markup pairs.
func (v *SourceView) LinkedTags() bool { return v.linkedTags }
func (v *SourceView) SetLinkedTags(b bool) {
	if !v.Destroyed() && v.linkedTags != b {
		v.editOptionsChanged()
		v.linkedTags = b
	}
}
