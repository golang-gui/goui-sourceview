package sourceview

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/platform"
	"github.com/golang-gui/goui/platform/events"
)

// Completion is a literal edit. Nil Range uses the requested word range. Empty
// Text uses Label. Caret, when set, is a UTF-8 offset relative to inserted Text.
// AdditionalEdits use the same original document coordinates as the primary edit.
type Completion struct {
	Label, Text, Detail string
	Range               *textedit.Range
	AdditionalEdits     []textedit.Edit
	Caret               *int
}

// CompletionContext is an immutable, bounded snapshot, safe on a worker. It has
// no live Model, Document or Widget. Offsets are UTF-8 bytes. NearbyText has at
// most 32 KiB on either side; Before/After have at most 16 KiB of the current line.
// Language/Definition describe the effective language at the caret, including
// an explicit embedded language. DocumentID remains the outer document identity.
type CompletionContext struct {
	DocumentID, Revision         uint64
	Language                     language.ID
	Definition                   *language.Language
	Range                        textedit.Range
	Caret                        int
	Prefix                       string
	Explicit                     bool
	Trigger                      string
	Lexical                      language.Context
	ContextKnown                 bool
	Before, After, NearbyText    string
	NearbyOffset, DocumentLength int
}

// CompletionProvider completely owns candidates, filtering and order. Treat
// this configuration as immutable; reuse its pointer across declarative updates.
// Complete runs on one worker per view, must honor cancellation, and must not
// touch GUI objects. Nil provider selects DefaultCompletions. Empty results stay
// empty. TriggerCharacters allows automatic requests even with an empty prefix.
type CompletionProvider struct {
	TriggerCharacters string
	Complete          func(context.Context, CompletionContext) ([]Completion, error)
}

type completionSession struct {
	state    operationState
	language language.ID
	range_   textedit.Range
	prefix   string
	items    []Completion
	index    int
	popup    gui.Popover
	list     *completionList
	panel    *completionPanel
	shown    bool
}

func (v *SourceView) SetCompletionProvider(provider *CompletionProvider) {
	if v.completionProvider == provider {
		return
	}
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
// normal result. Asynchronous failures are delivered once through ConnectError.
func (v *SourceView) ShowCompletion() error { return v.complete(true) }

func (v *SourceView) completionContext(explicit bool) (CompletionContext, bool) {
	m := v.document.model
	s := v.Selection()
	if s.Anchor != s.Caret {
		return CompletionContext{}, false
	}
	definition := v.definitionAt(s.Caret)
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
		if !definition.IsWord(r) {
			break
		}
		a -= n
	}
	for b < len(text) {
		r, n := utf8.DecodeRuneInString(text[b:])
		if !definition.IsWord(r) {
			break
		}
		b += n
	}
	if a == 0 && start > r.Start || b == len(text) && end < r.End {
		return CompletionContext{}, false
	}
	before, after := v.lineContext(s.Range())
	nearbyStart, nearbyEnd := max(0, s.Caret-(32<<10)), min(m.Len(), s.Caret+(32<<10))
	for !m.IsRuneBoundary(nearbyStart) {
		nearbyStart++
	}
	for !m.IsRuneBoundary(nearbyEnd) {
		nearbyEnd--
	}
	nearby, _ := m.Slice(textedit.Range{Start: nearbyStart, End: nearbyEnd})
	wordRange := textedit.Range{Start: start + a, End: start + b}
	ctx, known := v.lexicalContext(wordRange.Start)
	return CompletionContext{DocumentID: v.document.id, Revision: m.Revision(), Language: definition.ID(), Definition: definition, Range: wordRange, Caret: s.Caret, Prefix: text[a : s.Caret-start], Explicit: explicit, Lexical: ctx, ContextKnown: known, Before: before, After: after, NearbyText: nearby, NearbyOffset: nearbyStart, DocumentLength: m.Len()}, true
}

// LanguageCompletions returns the definition's static suggestions in code context.
func LanguageCompletions(c CompletionContext) []Completion {
	if c.Definition == nil || !c.ContextKnown || c.Lexical.String || c.Lexical.Comment {
		return nil
	}
	var out []Completion
	name := c.Definition.Info().Name
	for _, word := range c.Definition.Suggestions(c.Lexical) {
		out = append(out, Completion{Label: word, Detail: name})
	}
	return out
}

// DocumentCompletions returns whole words from the bounded nearby snapshot.
func DocumentCompletions(c CompletionContext) []Completion {
	if c.Definition == nil {
		return nil
	}
	text := c.NearbyText
	start, end := c.NearbyOffset, c.NearbyOffset+len(text)
	var out []Completion
	wordStart := -1
	for i, r := range text {
		if c.Definition.IsWord(r) {
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
	if wordStart >= 0 && end == c.DocumentLength && start+wordStart != c.Range.Start {
		out = append(out, Completion{Label: text[wordStart:], Detail: "word"})
	}
	return out
}

// DefaultCompletions explicitly combines language suggestions and nearby words,
// with case-insensitive prefix filtering, de-duplication and alphabetical order.
func DefaultCompletions(c CompletionContext) []Completion {
	out := LanguageCompletions(c)
	if c.Definition != nil && c.Definition.AllowsWordSuggestions(c.Lexical, c.ContextKnown) {
		out = append(out, DocumentCompletions(c)...)
	}
	return normalizeCompletions(out, c)
}
func (v *SourceView) proposals(c CompletionContext) []Completion { return DefaultCompletions(c) }

func sanitizeCompletions(items []Completion) []Completion {
	out := make([]Completion, 0, min(64, len(items)))
	for _, item := range items {
		if len(item.Label) > 512 || len(item.Detail) > 1024 || len(item.Text) > 64<<10 || len(item.AdditionalEdits) > 128 {
			continue
		}
		item.Label = textedit.Normalize(item.Label)
		item.Detail = strings.Join(strings.Fields(textedit.Normalize(item.Detail)), " ")
		if item.Text == "" {
			item.Text = item.Label
		}
		item.Text = textedit.Normalize(item.Text)
		if item.Label == "" || len(item.Text) > 64<<10 || strings.ContainsAny(item.Label, "\r\n\t\v\f\u0085\u2028\u2029") {
			continue
		}
		total := len(item.Text)
		valid := true
		for _, e := range item.AdditionalEdits {
			total += len(e.Text)
			if len(e.Text) > 64<<10 || total > 1<<20 {
				valid = false
				break
			}
		}
		if !valid {
			continue
		}
		if item.Range != nil {
			r := *item.Range
			item.Range = &r
		}
		if item.Caret != nil {
			at := *item.Caret
			item.Caret = &at
		}
		item.AdditionalEdits = slices.Clone(item.AdditionalEdits)
		out = append(out, item)
		if len(out) == 64 {
			break
		}
	}
	return out
}
func normalizeCompletions(items []Completion, c CompletionContext) []Completion {
	var out []Completion
	seen := map[string]bool{}
	prefix := strings.ToLower(c.Prefix)
	for _, item := range sanitizeCompletions(items) {
		if !strings.HasPrefix(strings.ToLower(item.Label), prefix) || item.Text == c.Prefix || seen[item.Text] {
			continue
		}
		seen[item.Text] = true
		out = append(out, item)
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
		start = start.Translate(geometry.Point{X: v.gutter + padding - v.offset.X, Y: padding + v.paragraphTop(row) - v.offset.Y})
		// A wrapped or horizontally hidden word start cannot anchor the visible
		// list. Keep it at the caret in that case; never alter editor selection.
		if start.Y == caret.Y && start.X >= v.gutter+padding {
			x = start.X
		}
	}
	return geometry.Point{X: x - completionTextInset, Y: caret.Y + caret.Height + 2}
}

func (v *SourceView) complete(explicit bool) error { return v.completeTriggered(explicit, "") }
func (v *SourceView) completeTriggered(explicit bool, trigger string) error {
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
	if !ok {
		return nil
	}
	c.Trigger = trigger
	triggers := c.Definition.CompletionTriggers()
	if v.completionProvider != nil {
		triggers = v.completionProvider.TriggerCharacters
	}
	triggered := utf8.RuneCountInString(trigger) == 1 && strings.Contains(triggers, trigger)
	if !explicit && c.Prefix == "" && !triggered {
		return nil
	}
	state, epoch := v.operationState(), v.completionEpoch
	if provider := v.completionProvider; provider != nil {
		if provider.Complete == nil {
			return nil
		}
		v.queueCompletion(state, epoch, c, provider.Complete)
		return nil
	}
	return v.presentCompletions(state, epoch, c, DefaultCompletions(c))
}
func (v *SourceView) presentCompletions(state operationState, epoch uint64, c CompletionContext, items []Completion) error {
	if !state.valid(v) || epoch != v.completionEpoch || !v.mounted || !v.Focused() || v.editable() != nil || v.projection != nil {
		return nil
	}
	items = sanitizeCompletions(items)
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
	s.list = newCompletionList(v, s)
	s.panel = newCompletionPanel(s.list)
	position.X -= s.panel.inset() - completionInset
	newPopup := func(transparent bool) gui.Popover {
		p := gui.NewPopover(v, &gui.PopoverOptions{Transparent: transparent})
		p.SetStyleName("sourceview.completion")
		p.SetWidget(s.panel)
		p.SetPosition(position)
		p.ConnectDismissRequest(v.closeCompletion)
		return p
	}
	s.popup = newPopup(true)
	v.completion = s
	err := s.popup.Show()
	// Unsupported/unavailable alpha surfaces are a normal presentation fallback.
	// Content errors and stale sessions must never trigger a second Show.
	if err != nil && s.list.err == nil && (errors.Is(err, platform.ErrUnsupported) || errors.Is(err, platform.ErrUnavailable)) && v.completion == s && state.valid(v) && epoch == v.completionEpoch {
		s.popup.SetWidget(nil)
		s.popup.Destroy()
		if v.completion != s || !state.valid(v) || epoch != v.completionEpoch || !v.mounted || !v.Focused() || v.editable() != nil {
			if v.completion == s {
				v.closeCompletion()
			}
			return nil
		}
		s.popup = newPopup(false)
		if retry := s.popup.Show(); retry != nil {
			err = errors.Join(err, retry)
		} else {
			err = nil
		}
	}
	if err != nil {
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
	if v.completionQueue != nil {
		v.completionQueue.cancel()
	}
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
	item := s.items[index]
	r := s.range_
	if item.Range != nil {
		r = *item.Range
	}
	caret := len(item.Text)
	if item.Caret != nil {
		caret = *item.Caret
	}
	if caret < 0 || caret > len(item.Text) || !utf8.ValidString(item.Text[:caret]) {
		return fmt.Errorf("sourceview: invalid completion caret")
	}
	edits := slices.Clone(item.AdditionalEdits)
	for _, e := range edits {
		if e.Range.Start < r.End && e.Range.End > r.Start || e.Range.Start == r.Start || e.Range.End == r.Start && e.Range.Start == e.Range.End {
			return fmt.Errorf("sourceview: overlapping completion edits")
		}
	}
	edits = append(edits, textedit.Edit{Range: r, Text: item.Text})
	sort.SliceStable(edits, func(i, j int) bool { return edits[i].Range.Start < edits[j].Range.Start })
	end := r.Start + caret
	for _, e := range item.AdditionalEdits {
		if e.Range.End <= r.Start {
			end += len(textedit.Normalize(e.Text)) - (e.Range.End - e.Range.Start)
		}
	}
	v.closeCompletion()
	if !s.state.valid(v) {
		return ErrOperationChanged
	}
	return v.ApplyEdits(s.state.document, s.state.revision, edits, textedit.Selection{Anchor: end, Caret: end})
}

func (v *SourceView) completionKey(event events.KeyEvent) bool {
	s := v.completion
	if s == nil {
		if event.Key == events.KeyEscape && v.completionQueue != nil && v.completionQueue.pending() {
			v.closeCompletion()
			return true
		}
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
