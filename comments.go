package sourceview

import (
	"strings"

	"github.com/golang-gui/goui-sourceview/internal/editor"
	"github.com/golang-gui/goui/gui/textedit"
)

// ToggleLineComment uses the language's line marker. Empty configuration is a
// normal no-op. A selection ending at the next line start excludes that line.
func (v *SourceView) ToggleLineComment() error {
	marker := v.definitionAt(v.Selection().Caret).Editing().Comments.Line
	if marker == "" {
		return nil
	}
	m := v.document.model
	s := v.Selection()
	first, last, err := editor.SelectedLines(m, s)
	if err != nil {
		return err
	}
	var edits []textedit.Edit
	all := true
	for row := first; row <= last; row++ {
		r, _ := m.LineRange(row)
		indent := editor.LeadingIndent(m, r.End)
		at := r.Start + len(indent)
		if at == r.End {
			continue
		}
		end := min(r.End, at+len(marker)+1)
		for !m.IsRuneBoundary(end) {
			end--
		}
		prefix, _ := m.Slice(textedit.Range{Start: at, End: end})
		e := textedit.Edit{Range: textedit.Range{Start: at, End: at}, Text: marker + " "}
		if strings.HasPrefix(prefix, marker) {
			e.Range.End += len(marker)
			if strings.HasPrefix(prefix[len(marker):], " ") {
				e.Range.End++
			}
		} else {
			all = false
		}
		edits = append(edits, e)
	}
	for i := range edits {
		if all {
			edits[i].Text = ""
		} else {
			edits[i].Range.End = edits[i].Range.Start
		}
	}
	after := textedit.Selection{Anchor: editor.MapPosition(s.Anchor, edits), Caret: editor.MapPosition(s.Caret, edits)}
	return v.ApplyEdits(v.document, m.Revision(), edits, after)
}

// ToggleBlockComment wraps the selection or removes its exact configured
// wrapper. Content is literal; this does not parse or nest block comments.
func (v *SourceView) ToggleBlockComment() error {
	p := v.definitionAt(v.Selection().Caret).Editing().Comments.Block
	if p.Open == "" || p.Close == "" {
		return nil
	}
	m := v.document.model
	s := v.Selection()
	r := s.Range()
	if r.End-r.Start > 32<<20 {
		return ErrOperationLimit
	}
	text, err := m.Slice(r)
	if err != nil {
		return err
	}
	after := s
	if len(text) >= len(p.Open)+len(p.Close) && strings.HasPrefix(text, p.Open) && strings.HasSuffix(text, p.Close) {
		text = text[len(p.Open) : len(text)-len(p.Close)]
		after = textedit.Selection{Anchor: r.Start, Caret: r.Start + len(text)}
		if s.Anchor > s.Caret {
			after.Anchor, after.Caret = after.Caret, after.Anchor
		}
	} else {
		text = p.Open + text + p.Close
		after.Anchor += len(p.Open)
		after.Caret += len(p.Open)
	}
	return v.ApplyEdits(v.document, m.Revision(), []textedit.Edit{{Range: r, Text: text}}, after)
}
