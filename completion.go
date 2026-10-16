package sourceview

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/golang-gui/goui-sourceview/internal/editor"
	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/platform/events"
)

// Completion is a literal replacement for CompletionContext.Range. Empty Text
// inserts Label. Snippets, asynchronous work and additional edits are separate
// capabilities; providers do not own the popup or an undo stack.
type Completion struct{ Label, Text, Detail string }

// CompletionContext describes one synchronous request on the GUI thread.
// Range covers the complete word, Prefix ends at Caret; offsets are UTF-8 bytes.
type CompletionContext struct {
	Document *Document
	Revision uint64
	Language language.ID
	Range    textedit.Range
	Caret    int
	Prefix   string
	Explicit bool
}

// CompletionProvider adds application proposals to built-in language and word
// suggestions. Return promptly; do not access GUI objects from background work.
type CompletionProvider func(CompletionContext) ([]Completion, error)

type completionSession struct {
	state    operationState
	language language.ID
	range_   textedit.Range
	prefix   string
	items    []Completion
	index    int
	popup    gui.Popover
	list     *completionList
	shown    bool
}

func (v *SourceView) SetCompletionProvider(provider CompletionProvider) {
	v.closeCompletion()
	v.completionProvider = provider
}
func (v *SourceView) AutomaticCompletion() bool { return v.automaticCompletion }
func (v *SourceView) SetAutomaticCompletion(enabled bool) {
	v.automaticCompletion = enabled
	if !enabled {
		v.closeCompletion()
	}
}

// ShowCompletion requests suggestions at the current caret. No proposals is a
// normal result. Errors from providers or popup creation are returned directly.
func (v *SourceView) ShowCompletion() error { return v.complete(true) }

func completionRune(r rune, id language.ID) bool {
	return unicode.IsLetter(r) || unicode.IsMark(r) || unicode.IsNumber(r) || r == '_' || (id == language.XML || id == language.HTML) && strings.ContainsRune("-:.", r)
}
func (v *SourceView) completionContext(explicit bool) (CompletionContext, bool) {
	m := v.document.model
	s := v.Selection()
	if s.Anchor != s.Caret {
		return CompletionContext{}, false
	}
	r, _ := m.LineRange(m.LineAt(s.Caret))
	// A word never causes an unbounded copy of a long logical line.
	start, end := max(r.Start, s.Caret-4096), min(r.End, s.Caret+4096)
	for !m.IsRuneBoundary(start) {
		start++
	}
	for !m.IsRuneBoundary(end) {
		end--
	}
	text, _ := m.Slice(textedit.Range{Start: start, End: end})
	a, b := s.Caret-start, s.Caret-start
	for a > 0 {
		r, n := utf8.DecodeLastRuneInString(text[:a])
		if !completionRune(r, v.document.language) {
			break
		}
		a -= n
	}
	for b < len(text) {
		r, n := utf8.DecodeRuneInString(text[b:])
		if !completionRune(r, v.document.language) {
			break
		}
		b += n
	}
	if a == 0 && start > r.Start || b == len(text) && end < r.End {
		return CompletionContext{}, false
	}
	return CompletionContext{Document: v.document, Revision: m.Revision(), Language: v.document.language, Range: textedit.Range{Start: start + a, End: start + b}, Caret: s.Caret, Prefix: text[a : s.Caret-start], Explicit: explicit}, true
}

func (v *SourceView) proposals(c CompletionContext) []Completion {
	var out []Completion
	ctx, known := v.lexicalContext(c.Range.Start)
	if known && !ctx.Comment && !ctx.String {
		switch c.Language {
		case language.JSON:
			for _, word := range []string{"false", "null", "true"} {
				out = append(out, Completion{Label: word, Detail: "JSON"})
			}
		case language.HTML:
			words := []string{"a", "article", "body", "button", "div", "footer", "form", "h1", "head", "header", "html", "img", "input", "label", "li", "link", "main", "meta", "nav", "p", "script", "section", "span", "style", "table", "tbody", "td", "textarea", "th", "thead", "title", "tr", "ul"}
			if ctx.Tag && !ctx.TagName {
				words = []string{"alt", "class", "disabled", "href", "id", "lang", "name", "rel", "role", "src", "style", "title", "type", "value"}
			}
			if ctx.Tag {
				for _, word := range words {
					out = append(out, Completion{Label: word, Detail: "HTML"})
				}
			}
		case language.XML:
			if ctx.Tag && !ctx.TagName {
				for _, word := range []string{"xmlns", "xml:lang", "xml:space"} {
					out = append(out, Completion{Label: word, Detail: "XML"})
				}
			}
		}
	}
	// Document words use a bounded nearby window, never a full-file scan per key.
	m := c.Document.model
	start, end := max(0, c.Caret-(32<<10)), min(m.Len(), c.Caret+(32<<10))
	for !m.IsRuneBoundary(start) {
		start++
	}
	for !m.IsRuneBoundary(end) {
		end--
	}
	text, _ := m.Slice(textedit.Range{Start: start, End: end})
	wordStart := -1
	for i, r := range text {
		if completionRune(r, c.Language) {
			if wordStart < 0 {
				wordStart = i
			}
			continue
		}
		if wordStart >= 0 {
			if start+wordStart != c.Range.Start && (wordStart > 0 || start == 0) {
				out = append(out, Completion{Label: text[wordStart:i], Detail: "word"})
			}
			wordStart = -1
		}
	}
	if wordStart >= 0 && end == m.Len() && start+wordStart != c.Range.Start {
		out = append(out, Completion{Label: text[wordStart:], Detail: "word"})
	}
	return out
}

func normalizeCompletions(items []Completion, c CompletionContext) []Completion {
	out := make([]Completion, 0, min(64, len(items)))
	seen := map[string]bool{}
	prefix := strings.ToLower(c.Prefix)
	for _, item := range items {
		if len(item.Label) > 512 || len(item.Detail) > 1024 || len(item.Text) > 64<<10 {
			continue
		}
		item.Label = textedit.Normalize(item.Label)
		item.Detail = strings.Join(strings.Fields(textedit.Normalize(item.Detail)), " ")
		if item.Text == "" {
			item.Text = item.Label
		}
		item.Text = textedit.Normalize(item.Text)
		if item.Label == "" || len(item.Label) > 512 || len(item.Detail) > 1024 || len(item.Text) > 64<<10 || strings.ContainsAny(item.Label, "\r\n\t\v\f\u0085\u2028\u2029") {
			continue
		}
		if !strings.HasPrefix(strings.ToLower(item.Label), prefix) || item.Text == c.Prefix || seen[item.Text] {
			continue
		}
		seen[item.Text] = true
		out = append(out, item)
		if len(out) == 64 {
			break
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Label < out[j].Label })
	return out
}

// Prefix matching folds Unicode runes, whose UTF-8 lengths can change (İ -> i).
// Convert the folded prefix length back to a boundary in the original label.
func completionPrefixEnd(label, prefix string) int {
	folded := strings.ToLower(prefix)
	if folded == "" || !strings.HasPrefix(strings.ToLower(label), folded) {
		return 0
	}
	length := 0
	for at, r := range label {
		length += utf8.RuneLen(unicode.ToLower(r))
		if length >= len(folded) {
			return at + utf8.RuneLen(r)
		}
	}
	return 0
}

func (v *SourceView) completionPosition(c CompletionContext, caret geometry.Rectangle) geometry.Point {
	x := caret.X
	row := v.document.model.LineAt(c.Range.Start)
	r, _ := v.document.model.LineRange(row)
	if p := v.paragraph(row); p != nil {
		start := p.geometry.Caret(textedit.Position{Offset: c.Range.Start - r.Start})
		start = start.Translate(geometry.Point{X: v.gutter + padding - v.offset.X, Y: padding + v.heights.Top(row) - v.offset.Y})
		// A wrapped or horizontally hidden word start cannot anchor the visible
		// list. Keep it at the caret in that case; never alter editor selection.
		if start.Y == caret.Y && start.X >= v.gutter+padding {
			x = start.X
		}
	}
	return geometry.Point{X: x - completionTextInset, Y: caret.Y + caret.Height + 2}
}

func (v *SourceView) complete(explicit bool) error {
	if v.closingCompletion {
		return ErrOperationChanged
	}
	v.closeCompletion()
	if !v.mounted {
		return ErrNotMounted
	}
	if err := v.editable(); err != nil {
		return err
	}
	if !v.Focused() || v.projection != nil || v.Window() == nil {
		return nil
	}
	c, ok := v.completionContext(explicit)
	if !ok || !explicit && c.Prefix == "" {
		return nil
	}
	state, epoch := v.operationState(), v.completionEpoch
	items := v.proposals(c)
	if v.completionProvider != nil {
		extra, err := v.completionProvider(c)
		if !state.valid(v) || epoch != v.completionEpoch || v.document.language != c.Language {
			return ErrOperationChanged
		}
		if err != nil {
			return err
		}
		items = append(extra, items...)
	}
	items = normalizeCompletions(items, c)
	if len(items) == 0 {
		return nil
	}
	caret, ok := v.caretRect()
	if !ok || !state.valid(v) {
		return nil
	}
	position := v.completionPosition(c, caret)
	if !state.valid(v) {
		return nil
	}
	s := &completionSession{state: state, language: c.Language, range_: c.Range, prefix: c.Prefix, items: items}
	s.popup = gui.NewPopover(v, nil)
	s.list = newCompletionList(v, s)
	s.popup.SetWidget(s.list)
	s.popup.SetPosition(position)
	s.popup.ConnectDismissRequest(v.closeCompletion)
	v.completion = s
	if err := s.popup.Show(); err != nil {
		if v.completion == s {
			v.closeCompletion()
		}
		return err
	}
	if err := s.list.err; err != nil {
		v.closeCompletion()
		return err
	}
	s.shown = true
	if !state.valid(v) || v.completion != s {
		s.popup.Destroy()
	}
	return nil
}

func (v *SourceView) closeCompletion() {
	if v.closingCompletion {
		return
	}
	v.closingCompletion = true
	defer func() { v.closingCompletion = false }()
	v.completionEpoch++
	s := v.completion
	v.completion = nil
	if s != nil {
		s.list.release()
		s.popup.Destroy()
	}
}

func (v *SourceView) acceptCompletion(s *completionSession, index int) error {
	if v.completion != s || !s.state.valid(v) || s.language != v.document.language || index < 0 || index >= len(s.items) {
		return nil
	}
	text := s.items[index].Text
	r := s.range_
	before := v.Selection()
	v.closeCompletion()
	if !s.state.valid(v) || s.language != v.document.language {
		return ErrOperationChanged
	}
	end := r.Start + len(text)
	return v.applyPlan(editor.Plan{Edits: []textedit.Edit{{Range: r, Text: text}}, Before: before, After: textedit.Selection{Anchor: end, Caret: end}})
}

func (v *SourceView) completionKey(event events.KeyEvent) bool {
	s := v.completion
	if s == nil {
		return false
	}
	if !s.state.valid(v) || s.language != v.document.language {
		v.closeCompletion()
		return false
	}
	if event.Modifiers != 0 {
		v.closeCompletion()
		return false
	}
	switch event.Key {
	case events.KeyArrowDown, events.KeyArrowUp:
		delta := 1
		if event.Key == events.KeyArrowUp {
			delta = -1
		}
		s.index = (s.index + delta + len(s.items)) % len(s.items)
		s.list.refresh()
		return true
	case events.KeyEnter, events.KeyTab:
		v.report(v.acceptCompletion(s, s.index))
		return true
	case events.KeyEscape:
		v.closeCompletion()
		return true
	case events.KeyBackspace:
		return false
	default:
		v.closeCompletion()
		return false
	}
}
