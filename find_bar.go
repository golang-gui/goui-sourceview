package sourceview

import (
	"strings"
	"time"

	"github.com/golang-gui/goui-sourceview/internal/controls"
	"github.com/golang-gui/goui-sourceview/internal/resource"
	"github.com/golang-gui/goui/core/signal"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/layout"
	"github.com/golang-gui/goui/mlang"
	"github.com/golang-gui/goui/platform/events"
)

// findBar is the editor-owned find/replace UI. It keeps input state while hidden.
type findBar struct {
	gui.WidgetBase
	pointer *gui.MotionEventController
	target  *SourceView
	*controls.FindUI
	keys                                *gui.KeyEventController
	attached                            signal.Handles
	frame, language                     signal.Handle
	mounted, open, replaceMode, syncing bool
	epoch                               uint64
	openSignal                          signal.Signal1[bool]
	errorSignal                         signal.Signal1[error]
	count                               struct {
		document              *Document
		revision              uint64
		query                 SearchQuery
		scope                 textedit.Range
		limited               bool
		at, total, matchedEnd int
		done, available       bool
	}
	message string
}

func newFindBar() *findBar {
	b := &findBar{}
	b.SetLayoutManager(layout.NewFillLayout())
	b.SetVisible(false)
	b.pointer = gui.NewMotionEventController()
	b.AddEventController(b.pointer)
	b.FindUI = controls.NewFindUI(b, controls.FindActions{
		ReplaceMode: b.SetReplaceMode, MatchCase: func(checked bool) {
			if v := b.liveTarget(); v != nil {
				q := v.Search()
				q.IgnoreCase = !checked
				v.SetSearch(q)
			}
		},
		WholeWord: func(checked bool) {
			if v := b.liveTarget(); v != nil {
				q := v.Search()
				q.WholeWord = checked
				v.SetSearch(q)
			}
		},
		Regex: func(checked bool) {
			if v := b.liveTarget(); v != nil {
				q := v.Search()
				q.Regex = checked
				v.SetSearch(q)
			}
		},
		SelectionOnly: b.setSelectionOnly, Previous: func() { b.navigate(true) }, Next: func() { b.navigate(false) }, Close: b.Close, Replace: b.replaceCurrent, ReplaceAll: b.replaceEverywhere,
		Replacing: func() bool { return b.replaceMode }, Message: func() string { return b.message }, QueryError: func() error {
			if v := b.liveTarget(); v != nil {
				return v.SearchError()
			}
			return nil
		},
	})
	b.WidgetBase.AddChild(b, b.Content)
	b.Find.ConnectChange(func(textedit.Change) {
		text := b.Find.Model().Text()
		if !b.syncing {
			if v := b.liveTarget(); v != nil {
				q := v.Search()
				q.Text = text
				v.SetSearch(q)
			}
		}
	})
	b.keys = gui.NewKeyEventController()
	b.keys.SetPhase(gui.PhaseCapture)
	b.keys.ConnectKeyDown(func(ctx gui.EventContext, e events.KeyEvent) { b.key(ctx, e, true) })
	b.AddEventController(b.keys)
	b.ConnectMount(b.mount)
	b.ConnectUnmount(b.unmount)
	return b
}

func (b *findBar) Target() *SourceView { return b.target }

// SetTarget borrows an editor. nil detaches it. Only a mounted editor in the
// same Root gets subscriptions; replacing/unmounting the bar releases them.
func (b *findBar) SetTarget(v *SourceView) {
	if b.Destroyed() || b.target == v {
		return
	}
	epoch := b.epoch
	b.disconnect()
	if b.Destroyed() || b.epoch != epoch {
		return
	}
	b.target = v
	b.epoch++
	b.count.document = nil
	if b.mounted {
		b.connect()
	}
	b.refresh()
}

func (b *findBar) Opened() bool { return b.open }

func (b *findBar) ReplaceMode() bool { return b.replaceMode }

func (b *findBar) SetReplaceMode(replace bool) {
	if b.Destroyed() || b.replaceMode == replace {
		return
	}
	b.replaceMode = replace
	b.Toggle.SetChecked(replace)
	b.ReplacementScroll.SetVisible(replace)
	b.ReplacementInput.SetVisible(replace)
	b.Replace.SetVisible(replace && !b.Narrow)
	b.ReplaceAll.SetVisible(replace && !b.Narrow)
	if b.ToggleIcon != nil {
		if replace {
			b.ToggleIcon.SetSource(controls.FindIcon("down"))
		} else {
			b.ToggleIcon.SetSource(controls.FindIcon("right"))
		}
	}
	b.RequestLayout()
	b.refresh()
}

func (b *findBar) Replacement() string { return b.ReplacementInput.Model().Text() }

func (b *findBar) SetReplacement(text string) {
	if !b.Destroyed() {
		setFindText(b.ReplacementInput, text)
		b.RequestLayout()
	}
}

func (b *findBar) ConnectOpen(fn func(bool)) signal.Handle {
	return b.openSignal.Connect(func(open bool) {
		if !b.Destroyed() && b.open == open {
			fn(open)
		}
	})
}

func (b *findBar) ConnectError(fn func(error)) signal.Handle {
	return b.errorSignal.Connect(func(err error) {
		if !b.Destroyed() {
			fn(err)
		}
	})
}

// Open focuses the query, seeding it from a short single-line selection when
// invoked from the editor. It never replaces a multiline selection with spaces.
func (b *findBar) Open(replace bool) error {
	v := b.liveTarget()
	if v == nil {
		return ErrNotMounted
	}
	host, ok := b.Root().(gui.EventTarget)
	if !ok {
		return ErrNotMounted
	}
	state, epoch := v.operationState(), b.epoch
	v.closeCompletion()
	v.cancelPreedit()
	if !state.valid(v) || b.epoch != epoch || b.liveTarget() != v {
		return ErrOperationChanged
	}
	if !b.open || v.Focused() {
		r := v.Selection().Range()
		if r.End > r.Start && r.End-r.Start <= 4096 {
			text, _ := v.document.model.Slice(r)
			if !strings.ContainsRune(text, '\n') {
				q := v.Search()
				q.Text = text
				v.SetSearch(q)
			}
		}
	}
	b.SetReplaceMode(replace)
	if b.Destroyed() || b.epoch != epoch || b.liveTarget() != v {
		return ErrOperationChanged
	}
	changed := !b.open
	b.open = true
	b.SetVisible(true)
	v.SetHighlightSearchMatches(true)
	b.Content.SetVisible(true)
	b.refresh()
	if b.liveTarget() != v || b.epoch != epoch {
		return ErrOperationChanged
	}
	host.SetFocusedWidget(b.Find)
	if b.Destroyed() || b.epoch != epoch || !b.open {
		return nil
	}
	b.Find.SetSelection(textedit.Selection{Caret: len(b.Find.Model().Text())})
	if changed && !b.Destroyed() && b.epoch == epoch && b.open {
		b.openSignal.Emit(true)
	}
	return nil
}

// Close hides search backgrounds and clears the search scope, retaining the
// query for reopening and next/previous navigation. It returns focus only when
// focus was inside the bar, preserving ordinary text selection.
func (b *findBar) Close() {
	if b.Destroyed() || !b.open {
		return
	}
	focus := b.ContainsFocus()
	b.open = false
	b.epoch++
	epoch, v := b.epoch, b.target
	attached := len(b.attached) != 0
	valid := func() bool { return !b.Destroyed() && !b.open && b.epoch == epoch && b.target == v }
	b.Content.SetVisible(false)
	b.SetVisible(false)
	if !valid() {
		return
	}
	if attached && v != nil {
		v.SetHighlightSearchMatches(false)
		_ = v.SetSearchRange(nil)
		if !valid() {
			return
		}
	}
	if focus {
		if v := b.liveTarget(); v != nil && visible(v) {
			if host, ok := b.Root().(gui.EventTarget); ok {
				host.SetFocusedWidget(v)
			}
		}
	}
	if valid() {
		b.openSignal.Emit(false)
	}
}

func (b *findBar) liveTarget() *SourceView {
	v := b.target
	if b.Destroyed() || !b.mounted || b.Root() == nil || v == nil || v.Destroyed() || !v.mounted || v.Root() != b.Root() {
		return nil
	}
	return v
}

func (b *findBar) mount() {
	b.mounted = true
	b.epoch++
	b.connect()
	b.frame = b.Root().ConnectFrame(func(time.Time) { b.refresh(); b.countStep() })
	b.language = mlang.ConnectChanged(b.updateLabels)
	b.updateLabels()
}

func (b *findBar) unmount() {
	b.mounted = false
	b.epoch++
	b.disconnect()
	if b.frame != nil {
		b.frame.Disconnect()
		b.frame = nil
	}
	if b.language != nil {
		b.language.Disconnect()
		b.language = nil
	}
	b.count.document = nil
}

func (b *findBar) connect() { b.attach() }

func (b *findBar) attach() {
	if len(b.attached) != 0 || b.liveTarget() == nil {
		return
	}
	// SourceView captures activation commands; this widget handles only its own inputs.
	b.attached = signal.Handles{b.target.searchState.Connect(b.refresh)}
	if b.open {
		b.target.SetHighlightSearchMatches(true)
	}
	b.refresh()
}

func (b *findBar) detach() {
	v := b.target
	attached := len(b.attached) != 0
	b.attached.Disconnect()
	b.attached = nil
	if attached && b.open && v != nil && !v.Destroyed() {
		v.SetHighlightSearchMatches(false)
		_ = v.SetSearchRange(nil)
	}
}

func (b *findBar) disconnect() { b.detach() }

func (b *findBar) refresh() {
	if b.Destroyed() || b.syncing {
		return
	}
	b.syncing = true
	defer func() { b.syncing = false }()
	v := b.liveTarget()
	epoch := b.epoch
	valid := func() bool { return !b.Destroyed() && b.epoch == epoch && b.liveTarget() == v }
	active := v != nil && gui.IsEnabled(v)
	editable := active && !v.ReadOnly()
	searchable := active && v.SearchError() == nil && v.query.Text != ""
	canScope := false
	if active {
		_, limited := v.SearchRange()
		selection := v.Selection().Range()
		canScope = limited || selection.Start < selection.End
	}
	for _, item := range []struct {
		widget  gui.Widget
		enabled bool
	}{
		{b.Find, active}, {b.MatchCase, active}, {b.WholeWord, active}, {b.Regex, active}, {b.SelectionOnly, canScope},
		{b.Previous, searchable}, {b.Next, searchable},
		{b.ReplacementInput, editable}, {b.Replace, editable && searchable}, {b.ReplaceAll, editable && searchable},
	} {
		item.widget.SetEnabled(item.enabled)
		if !valid() {
			return
		}
	}
	if v == nil {
		b.Status.SetText("")
		return
	}
	q := v.Search()
	b.MatchCase.SetChecked(!q.IgnoreCase)
	b.WholeWord.SetChecked(q.WholeWord)
	b.Regex.SetChecked(q.Regex)
	scope, limited := v.SearchRange()
	b.SelectionOnly.SetChecked(limited)
	// The multiline field preserves programmatic and pasted line breaks.
	setFindText(b.Find, q.Text)
	if !valid() {
		return
	}
	if b.count.document != v.document || b.count.revision != v.document.model.Revision() || b.count.query != q || b.count.scope != scope || b.count.limited != limited {
		b.count.scope, b.count.limited = scope, limited
		b.count.document = v.document
		b.count.revision = v.document.model.Revision()
		b.count.query = q
		b.count.at, b.count.total, b.count.matchedEnd = 0, 0, 0
		b.count.done = q.Text == ""
		b.count.available = len(q.Text) <= 4096
		b.message = ""
		if err := v.SearchError(); err != nil {
			b.message = resource.InvalidExpression()
			b.count.done = true
		}
		if b.open && !b.count.done {
			b.RequestPaint()
		}
	}
	b.updateStatus()
}

func (b *findBar) countStep() {
	v := b.liveTarget()
	if !b.open || !visible(b) || v == nil || b.count.done || !b.count.available {
		return
	}
	next, count, after := v.search.CountChunk(v.document.model, b.count.at, 64<<10, b.count.matchedEnd)
	b.count.matchedEnd = after
	b.count.at, b.count.total = next, b.count.total+count
	b.count.done = next == v.document.model.Len()
	b.updateStatus()
	if !b.count.done {
		b.RequestPaint()
	}
}

func (b *findBar) updateStatus() {
	text := b.message
	if text == "" && b.count.query.Text != "" {
		switch {
		case !b.count.available:
			text = resource.QueryTooLong()
		case !b.count.done:
			text = resource.Counting()
		default:
			text = resource.Matches(b.count.total)
		}
	}
	changed := b.Status.Text() != text
	b.Status.SetText(text)
	b.Status.SetName(text)
	if v := b.liveTarget(); v != nil && v.SearchError() != nil {
		b.Status.SetName(text + ": " + v.SearchError().Error())
	}
	name := "sourceview.find.status"
	if b.message != "" || b.count.done && b.count.available && b.count.query.Text != "" && b.count.total == 0 {
		name = "sourceview.find.error"
	}
	changed = changed || b.Status.StyleName() != name
	b.Status.SetStyleName(name)
	if changed {
		b.Content.RequestPaint()
	}
}

func (b *findBar) updateLabels() {
	b.Find.SetPlaceholder(resource.Find())
	b.ReplacementInput.SetPlaceholder(resource.Replace())
	for widget, getter := range b.Names {
		widget.SetName(getter())
	}
	b.refresh()
}

func (b *findBar) navigate(backward bool) {
	if v := b.liveTarget(); v != nil && gui.IsEnabled(v) {
		v.FindNext(backward)
	}
}

func (b *findBar) fail(err error) {
	if err == nil || b.Destroyed() {
		return
	}
	b.message = err.Error()
	b.updateStatus()
	b.errorSignal.Emit(err)
}

func (b *findBar) replaceCurrent() {
	v := b.liveTarget()
	if v == nil || v.editable() != nil {
		return
	}
	state, epoch := v.operationState(), b.epoch
	replaced, err := v.ReplaceCurrent(b.Replacement())
	if b.Destroyed() || b.epoch != epoch || b.liveTarget() != v {
		return
	}
	if err != nil {
		b.fail(err)
		return
	}
	if !replaced && state.valid(v) || replaced && v.document == state.document && v.mountEpoch == state.mount && v.document.model.Revision() == state.revision+1 && v.selectionEpoch == state.selection+1 {
		v.FindNext(false)
	}
}

func (b *findBar) replaceEverywhere() {
	v := b.liveTarget()
	if v == nil || v.editable() != nil {
		return
	}
	epoch := b.epoch
	_, err := v.ReplaceAll(b.Replacement())
	if b.epoch == epoch && b.liveTarget() == v {
		b.fail(err)
	}
}

func (b *findBar) key(ctx gui.EventContext, e events.KeyEvent, inBar bool) {
	v := b.liveTarget()
	if v == nil || !gui.IsEnabled(b) || !gui.IsEnabled(v) || !visible(b) {
		return
	}
	command, bound := v.keyCommand(e)
	handled := false
	switch {
	case bound && (command == CommandFind || command == CommandReplace):
		handled = true
		if command == CommandReplace {
			b.fail(v.ShowReplace())
		} else {
			b.fail(v.ShowFind())
		}
	case b.open && e.Key == events.KeyEscape && e.Modifiers == 0:
		handled = true
		b.Close()
	case b.open && inBar && (b.Find.Focused() || b.ReplacementInput.Focused()) && e.Key == events.KeyEnter && e.Modifiers&^events.ModifierShift == 0:
		handled = true
		if b.ReplacementInput.Focused() && e.Modifiers == 0 {
			b.replaceCurrent()
		} else {
			b.navigate(e.Modifiers != 0)
		}
	case b.open && inBar && bound && (command == CommandFindNext || command == CommandFindPrevious):
		handled = true
		b.navigate(command == CommandFindPrevious)
	}
	if handled {
		e.PreventDefault()
		ctx.StopPropagation()
	}
}

func (b *findBar) Snapshot() gui.WidgetInfo {
	info := b.WidgetBase.Snapshot()
	info.Role = gui.Role("sourceview-find-bar")
	info.Text = b.Find.Model().Text()
	queryError := ""
	if v := b.liveTarget(); v != nil && v.SearchError() != nil {
		queryError = v.SearchError().Error()
	}
	info.SetAttribute("sourceview.find", map[string]any{"queryError": queryError, "open": b.open, "replace": b.replaceMode, "count": b.count.total, "countComplete": b.count.done && b.count.available})
	return info
}

var _ gui.Widget = (*findBar)(nil)

// setFindText preserves field history/selection on ordinary refreshes.
func setFindText(field *gui.TextView, text string) {
	text = textedit.Normalize(text)
	if !field.Model().EqualText(text) {
		field.Model().SetText(text)
	}
}

// Capture the range before navigating. A result selection never becomes the
// next search scope, and an empty selection is not an interactive scope.
func (b *findBar) setSelectionOnly(checked bool) {
	if b.syncing {
		return
	}
	v := b.liveTarget()
	if v == nil {
		return
	}
	selection := v.Selection().Range()
	if checked && selection.Start == selection.End {
		b.refresh()
		return
	}
	state, epoch, query := v.operationState(), b.epoch, v.Search()
	v.cancelPreedit()
	if !state.valid(v) || b.epoch != epoch || b.liveTarget() != v || v.Search() != query {
		return
	}
	var scope *textedit.Range
	if checked {
		scope = &selection
	}
	if err := v.SetSearchRange(scope); err != nil {
		b.fail(err)
		return
	}
	// Search-state subscribers may close/retarget the bar or replace the
	// document/query/selection. Do not navigate their replacement state.
	if !state.valid(v) || b.epoch != epoch || b.liveTarget() != v || v.Search() != query {
		return
	}
	actual, limited := v.SearchRange()
	if limited != checked || checked && actual != selection {
		return
	}
	v.findFrom(selection.Start, false)
	b.refresh()
	b.countStep()
}

func (b *findBar) StyleChanged() { b.FindUI.ReadAppearance() }
