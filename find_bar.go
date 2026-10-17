package sourceview

import (
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/golang-gui/goui/core/signal"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/layout"
	"github.com/golang-gui/goui/mlang"
	"github.com/golang-gui/goui/platform/events"
	"github.com/golang-gui/goui/widgets"
)

// FindBar is an optional companion widget. Mount it beside a SourceView in the
// same Root, then SetTarget. It owns its controls, not the editor or document.
// Its content is initially closed; the application chooses its placement.
type FindBar struct {
	gui.WidgetBase
	target                                     *SourceView
	content                                    *gui.LinearBox
	find, replacement                          *gui.TextInput
	replaceRow                                 *gui.LinearBox
	previous, next, replace, replaceAll, close *gui.Button
	matchCase, wholeWord, toggle               *widgets.CheckButton
	status                                     *gui.Label
	labels                                     map[*gui.Label]mlang.Getter
	names                                      map[gui.Widget]mlang.Getter
	keys                                       *gui.KeyEventController
	lifetime, attached                         signal.Handles
	frame, language                            signal.Handle
	mounted, open, replaceMode, syncing        bool
	epoch                                      uint64
	openSignal                                 signal.Signal1[bool]
	errorSignal                                signal.Signal1[error]
	count                                      struct {
		document              *Document
		revision              uint64
		query                 SearchQuery
		at, total, matchedEnd int
		done, available       bool
	}
	message string
}

func NewFindBar() *FindBar {
	b := &FindBar{labels: make(map[*gui.Label]mlang.Getter), names: make(map[gui.Widget]mlang.Getter)}
	b.SetLayoutManager(layout.NewFillLayout())
	b.content = gui.NewLinearBox(layout.DirectionVertical)
	b.content.SetCrossAlign(layout.CrossStretch)
	b.content.SetSpacing(6)
	b.content.SetPadding(8)
	b.content.SetVisible(false)
	b.WidgetBase.AddChild(b, b.content)
	row := gui.NewLinearBox(layout.DirectionHorizontal)
	row.SetSpacing(6)
	options := gui.NewWrapBox(layout.DirectionHorizontal)
	options.SetSpacing(6)
	options.SetLineSpacing(4)
	b.toggle = b.option(options, "Replace", "替换", func(checked bool) { b.SetReplaceMode(checked) })
	b.find = gui.NewTextInput()
	b.find.SetName("Find")
	b.find.SetMainWeight(1)
	row.AddChild(b.find)
	b.previous = b.iconButton(row, "↑", "Previous", "上一项", func() { b.navigate(true) })
	b.next = b.iconButton(row, "↓", "Next", "下一项", func() { b.navigate(false) })
	b.close = b.iconButton(row, "×", "Close", "关闭", b.Close)
	b.content.AddChild(row)
	b.matchCase = b.option(options, "Match case", "区分大小写", func(checked bool) {
		if v := b.liveTarget(); v != nil {
			q := v.Search()
			q.IgnoreCase = !checked
			v.SetSearch(q)
		}
	})
	b.wholeWord = b.option(options, "Whole word", "整词", func(checked bool) {
		if v := b.liveTarget(); v != nil {
			q := v.Search()
			q.WholeWord = checked
			v.SetSearch(q)
		}
	})
	b.status = gui.NewLabel("")
	options.AddChild(b.status)
	b.content.AddChild(options)
	b.replaceRow = gui.NewLinearBox(layout.DirectionHorizontal)
	b.replaceRow.SetSpacing(6)
	b.replacement = gui.NewTextInput()
	b.replacement.SetName("Replacement")
	b.replacement.SetMainWeight(1)
	b.replaceRow.AddChild(b.replacement)
	b.replace = b.button(b.replaceRow, "Replace", "替换", b.replaceCurrent)
	b.replaceAll = b.button(b.replaceRow, "Replace all", "全部替换", b.replaceEverywhere)
	b.replaceRow.SetVisible(false)
	b.content.AddChild(b.replaceRow)
	b.find.ConnectText(func(text string) {
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
	b.keys.ConnectKeyDown(func(ctx gui.EventContext, e events.KeyEvent) { b.key(ctx, e, false) })
	keys := gui.NewKeyEventController()
	keys.SetPhase(gui.PhaseCapture)
	keys.ConnectKeyDown(func(ctx gui.EventContext, e events.KeyEvent) { b.key(ctx, e, true) })
	b.AddEventController(keys)
	b.ConnectMount(b.mount)
	b.ConnectUnmount(b.unmount)
	return b
}

func (b *FindBar) button(parent gui.Container, en, zh string, fn func()) *gui.Button {
	button := gui.NewButton()
	getter := mlang.String(mlang.En(en), mlang.ZhCN(zh))
	label := gui.NewLabel(getter())
	b.labels[label] = getter
	button.SetChild(label)
	button.ConnectClicked(fn)
	parent.AddChild(button)
	return button
}

func (b *FindBar) iconButton(parent gui.Container, symbol, en, zh string, fn func()) *gui.Button {
	button := b.button(parent, en, zh, fn)
	label := button.Children()[0].(*gui.Label)
	getter := b.labels[label]
	delete(b.labels, label)
	label.SetText(symbol)
	button.SetName(getter())
	b.names[button] = getter
	return button
}

func (b *FindBar) option(parent gui.Container, en, zh string, fn func(bool)) *widgets.CheckButton {
	button := widgets.NewCheckButton()
	getter := mlang.String(mlang.En(en), mlang.ZhCN(zh))
	label := gui.NewLabel(getter())
	b.labels[label] = getter
	button.SetChild(label)
	button.ConnectChange(func(state widgets.CheckState) { fn(state == widgets.CheckChecked) })
	parent.AddChild(button)
	return button
}

func (b *FindBar) Target() *SourceView { return b.target }

// SetTarget borrows an editor. nil detaches it. Only a mounted editor in the
// same Root gets bindings; replacing/unmounting the bar releases those bindings.
func (b *FindBar) SetTarget(v *SourceView) {
	if b.Destroyed() || b.target == v {
		return
	}
	b.disconnect()
	b.target = v
	b.epoch++
	b.count.document = nil
	if b.mounted {
		b.connect()
	}
	b.refresh()
}

func (b *FindBar) Opened() bool      { return b.open }
func (b *FindBar) ReplaceMode() bool { return b.replaceMode }
func (b *FindBar) SetReplaceMode(replace bool) {
	if b.Destroyed() || b.replaceMode == replace {
		return
	}
	b.replaceMode = replace
	b.toggle.SetChecked(replace)
	b.replaceRow.SetVisible(replace)
	b.refresh()
}
func (b *FindBar) Replacement() string { return b.replacement.Text() }
func (b *FindBar) SetReplacement(text string) {
	if !b.Destroyed() {
		b.replacement.SetText(text)
	}
}
func (b *FindBar) ConnectOpen(fn func(bool)) signal.Handle {
	return b.openSignal.Connect(func(open bool) {
		if !b.Destroyed() && b.open == open {
			fn(open)
		}
	})
}
func (b *FindBar) ConnectError(fn func(error)) signal.Handle {
	return b.errorSignal.Connect(func(err error) {
		if !b.Destroyed() {
			fn(err)
		}
	})
}

// Open focuses the query, seeding it from a short single-line selection when
// invoked from the editor. It never replaces a multiline selection with spaces.
func (b *FindBar) Open(replace bool) error {
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
	changed := !b.open
	b.open = true
	b.content.SetVisible(true)
	b.refresh()
	if b.liveTarget() != v || b.epoch != epoch {
		return ErrOperationChanged
	}
	host.SetFocusedWidget(b.find)
	if b.Destroyed() || b.epoch != epoch || !b.open {
		return nil
	}
	b.find.SetSelection(textedit.Selection{Caret: len(b.find.Text())})
	if changed && !b.Destroyed() && b.epoch == epoch && b.open {
		b.openSignal.Emit(true)
	}
	return nil
}

// Close returns focus only when it was inside the bar. The query and its
// highlights stay in the editor so next/previous bindings continue to work.
func (b *FindBar) Close() {
	if b.Destroyed() || !b.open {
		return
	}
	focus := b.ContainsFocus()
	b.open = false
	b.epoch++
	b.content.SetVisible(false)
	if focus {
		if v := b.liveTarget(); v != nil && visible(v) {
			if host, ok := b.Root().(gui.EventTarget); ok {
				host.SetFocusedWidget(v)
			}
		}
	}
	if !b.Destroyed() && !b.open {
		b.openSignal.Emit(false)
	}
}

func (b *FindBar) liveTarget() *SourceView {
	v := b.target
	if b.Destroyed() || !b.mounted || b.Root() == nil || v == nil || v.Destroyed() || !v.mounted || v.Root() != b.Root() {
		return nil
	}
	return v
}
func (b *FindBar) mount() {
	b.mounted = true
	b.epoch++
	b.connect()
	b.frame = b.Root().ConnectFrame(func(time.Time) { b.refresh(); b.countStep() })
	b.language = mlang.ConnectChanged(b.updateLabels)
	b.updateLabels()
}
func (b *FindBar) unmount() {
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
func (b *FindBar) connect() {
	if b.target == nil || b.target.Destroyed() {
		return
	}
	b.lifetime = signal.Handles{
		b.target.ConnectMount(b.attach),
		b.target.ConnectUnmount(func() { b.detach(); b.count.document = nil; b.refresh() }),
	}
	b.attach()
}
func (b *FindBar) attach() {
	if len(b.attached) != 0 || b.liveTarget() == nil {
		return
	}
	b.target.AddEventController(b.keys)
	b.attached = signal.Handles{b.target.searchState.Connect(b.refresh)}
	b.refresh()
}
func (b *FindBar) detach() {
	if b.target != nil {
		b.target.RemoveEventController(b.keys)
	}
	b.attached.Disconnect()
	b.attached = nil
}
func (b *FindBar) disconnect() { b.detach(); b.lifetime.Disconnect(); b.lifetime = nil }

func (b *FindBar) refresh() {
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
	for _, item := range []struct {
		widget  gui.Widget
		enabled bool
	}{
		{b.find, active}, {b.matchCase, active}, {b.wholeWord, active},
		{b.previous, active && v.query.Text != ""}, {b.next, active && v.query.Text != ""},
		{b.replacement, editable}, {b.replace, editable && v.query.Text != ""}, {b.replaceAll, editable && v.query.Text != ""},
	} {
		item.widget.SetEnabled(item.enabled)
		if !valid() {
			return
		}
	}
	if v == nil {
		b.status.SetText("")
		return
	}
	q := v.Search()
	b.matchCase.SetChecked(!q.IgnoreCase)
	b.wholeWord.SetChecked(q.WholeWord)
	// A single-line field cannot faithfully edit a programmatic multiline query.
	// Keep that query intact until the user explicitly changes the field.
	b.find.SetText(q.Text)
	if !valid() {
		return
	}
	if b.count.document != v.document || b.count.revision != v.document.model.Revision() || b.count.query != q {
		b.count.document = v.document
		b.count.revision = v.document.model.Revision()
		b.count.query = q
		b.count.at, b.count.total, b.count.matchedEnd = 0, 0, 0
		b.count.done = q.Text == ""
		b.count.available = len(q.Text) <= 4096
		b.message = ""
		if b.open && !b.count.done {
			b.RequestPaint()
		}
	}
	b.updateStatus()
}
func (b *FindBar) countStep() {
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
func (b *FindBar) updateStatus() {
	text := b.message
	if text == "" && b.count.query.Text != "" {
		switch {
		case !b.count.available:
			text = mlang.String(mlang.En("Query too long to count"), mlang.ZhCN("查询过长，无法计数"))()
		case !b.count.done:
			text = mlang.String(mlang.En("Counting…"), mlang.ZhCN("计数中…"))()
		default:
			text = fmt.Sprintf(mlang.String(mlang.En("%d matches"), mlang.ZhCN("%d 个匹配"))(), b.count.total)
		}
	}
	b.status.SetText(text)
}
func (b *FindBar) updateLabels() {
	for label, getter := range b.labels {
		label.SetText(getter())
	}
	for widget, getter := range b.names {
		widget.SetName(getter())
	}
	b.refresh()
}
func (b *FindBar) navigate(backward bool) {
	if v := b.liveTarget(); v != nil && gui.IsEnabled(v) {
		v.FindNext(backward)
	}
}
func (b *FindBar) fail(err error) {
	if err == nil || b.Destroyed() {
		return
	}
	b.message = err.Error()
	b.updateStatus()
	b.errorSignal.Emit(err)
}
func (b *FindBar) replaceCurrent() {
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
func (b *FindBar) replaceEverywhere() {
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
func (b *FindBar) key(ctx gui.EventContext, e events.KeyEvent, inBar bool) {
	v := b.liveTarget()
	if v == nil || !gui.IsEnabled(b) || !gui.IsEnabled(v) || !visible(b) {
		return
	}
	primary, _ := gui.ModPrimary.Resolve()
	open := e.Key == events.KeyF && e.Modifiers == primary
	replace := e.Key == events.KeyH && e.Modifiers == primary
	if runtime.GOOS == "darwin" {
		alt, _ := gui.ModAlt.Resolve()
		replace = e.Key == events.KeyF && e.Modifiers == primary|alt
	}
	handled := false
	switch {
	case open || replace:
		handled = true
		b.fail(b.Open(replace))
	case b.open && e.Key == events.KeyEscape && e.Modifiers == 0:
		handled = true
		b.Close()
	case b.open && inBar && (b.find.Focused() || b.replacement.Focused()) && e.Key == events.KeyEnter && e.Modifiers&^events.ModifierShift == 0:
		handled = true
		if b.replacement.Focused() && e.Modifiers == 0 {
			b.replaceCurrent()
		} else {
			b.navigate(e.Modifiers != 0)
		}
	case b.open && inBar && (e.Key == events.KeyF3 && e.Modifiers&^events.ModifierShift == 0 || runtime.GOOS == "darwin" && e.Key == events.KeyG && e.Modifiers&^events.ModifierShift == primary):
		handled = true
		b.navigate(e.Modifiers&events.ModifierShift != 0)
	}
	if handled {
		e.PreventDefault()
		ctx.StopPropagation()
	}
}
func (b *FindBar) Snapshot() gui.WidgetInfo {
	info := b.WidgetBase.Snapshot()
	info.Role = gui.Role("sourceview-find-bar")
	info.Text = b.find.Text()
	info.SetAttribute("sourceview.find", map[string]any{"open": b.open, "replace": b.replaceMode, "count": b.count.total, "countComplete": b.count.done && b.count.available})
	return info
}

var _ gui.Widget = (*FindBar)(nil)
