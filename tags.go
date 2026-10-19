package sourceview

import (
	"sort"
	"strings"

	"github.com/golang-gui/goui-sourceview/internal/editor"
	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/gui/textedit"
)

func (v *SourceView) definitionAt(at int) *language.Language {
	if ctx, ok := v.lexicalContext(at); ok && ctx.Embedded != nil {
		return ctx.Embedded
	}
	return v.document.definition
}

// Tag assistance is bounded and never treats a random truncated prefix as a
// document boundary. Large documents retain ordinary input and token matching.
// embeddedSpan finds a fully analyzed contiguous child region. Unresolved
// boundaries fail closed; no language ID has special treatment here.
func (v *SourceView) embeddedSpan(at int) (textedit.Range, bool) {
	m := v.document.model
	ctx, ok := v.lexicalContext(at)
	if !ok || ctx.Embedded == nil {
		return textedit.Range{}, false
	}
	first, last := m.LineAt(at), m.LineAt(at)
	for first > 0 {
		if r, _ := m.LineRange(first); at-r.Start > 64<<10 {
			return textedit.Range{}, false
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
			return textedit.Range{}, false
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
	return textedit.Range{Start: a.Start, End: b.End}, true
}
func (v *SourceView) tagText() (string, *language.TagRules, int) {
	rules := v.definitionAt(v.Selection().Caret).Editing().Tags
	if rules == nil || v.document.model.Len() > 256<<10 {
		return "", nil, 0
	}
	r := textedit.Range{End: v.document.model.Len()}
	if ctx, ok := v.lexicalContext(v.Selection().Caret); ok && ctx.Embedded != nil {
		var valid bool
		r, valid = v.embeddedSpan(v.Selection().Caret)
		if !valid {
			return "", nil, 0
		}
	}
	text, _ := v.document.model.Slice(r)
	return text, rules, r.Start
}
func (v *SourceView) matchingTags() []textedit.Range {
	text, rules, base := v.tagText()
	if rules == nil {
		return nil
	}
	match, ok := rules.Match(text, v.Selection().Caret-base)
	if !ok {
		return nil
	}
	return []textedit.Range{{Start: base + match.Opening.Start, End: base + match.Opening.End}, {Start: base + match.Closing.Start, End: base + match.Closing.End}}
}
func (v *SourceView) closeTag(r textedit.Range, typed string) (bool, error) {
	if !v.autoPairs || typed != ">" || r.Start != r.End {
		return false, nil
	}
	text, rules, base := v.tagText()
	if rules == nil {
		return false, nil
	}
	close := rules.ClosingTag(text[:r.Start] + ">")
	if close == "" || strings.HasPrefix(text[r.Start-base:], close) {
		return false, nil
	}
	// Consume a generated angle closer only when its exact ownership is valid.
	for _, p := range v.pairs {
		if p.close == r.Start && p.right == ">" && v.pairValid(p) {
			r.End++
			break
		}
	}
	at := r.Start + 1
	return true, v.applyPlan(editor.Plan{Before: v.Selection(), After: textedit.Selection{Anchor: at, Caret: at}, Edits: []textedit.Edit{{Range: r, Text: ">" + close}}})
}

// Linked name edits revalidate both ranges against the original revision and
// make one ApplyBatch. External edits, paste and IME never trigger a mirror edit.
func (v *SourceView) linkedTag(r textedit.Range, insert string) (bool, error) {
	if !v.linkedTags {
		return false, nil
	}
	text, rules, base := v.tagText()
	if rules == nil {
		return false, nil
	}
	match, ok := rules.Match(text, r.Start-base)
	if !ok {
		return false, nil
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
		return false, nil
	}
	name := text[a.Start-base:r.Start-base] + insert + text[r.End-base:a.End-base]
	if !rules.ValidName(name) {
		return false, nil
	}
	mirror := textedit.Edit{Range: textedit.Range{Start: b.Start, End: b.End}, Text: name}
	edits := []textedit.Edit{{Range: r, Text: insert}, mirror}
	sort.Slice(edits, func(i, j int) bool { return edits[i].Range.Start < edits[j].Range.Start })
	caret := r.Start + len(insert)
	if b.End <= r.Start {
		caret += len(name) - (b.End - b.Start)
	}
	return true, v.applyPlan(editor.Plan{Before: v.Selection(), After: textedit.Selection{Anchor: caret, Caret: caret}, Edits: edits})
}

// LinkedTags controls typed/deleted name mirroring on confirmed markup pairs.
func (v *SourceView) LinkedTags() bool { return v.linkedTags }
func (v *SourceView) SetLinkedTags(b bool) {
	if !v.Destroyed() {
		v.linkedTags = b
	}
}
