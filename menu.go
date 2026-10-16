package sourceview

import (
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/core/signal"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/mlang"
)

type editCommand int

const (
	commandUndo editCommand = iota
	commandRedo
	commandCut
	commandCopy
	commandPaste
	commandDelete
	commandSelectAll
)

var commandLabels = [...]mlang.Getter{
	mlang.String(mlang.En("Undo"), mlang.ZhCN("撤销")),
	mlang.String(mlang.En("Redo"), mlang.ZhCN("重做")),
	mlang.String(mlang.En("Cut"), mlang.ZhCN("剪切")),
	mlang.String(mlang.En("Copy"), mlang.ZhCN("复制")),
	mlang.String(mlang.En("Paste"), mlang.ZhCN("粘贴")),
	mlang.String(mlang.En("Delete"), mlang.ZhCN("删除")),
	mlang.String(mlang.En("Select All"), mlang.ZhCN("全选")),
}

type editMenu struct {
	popup    *gui.PopoverMenu
	items    map[editCommand]*gui.MenuItem
	language signal.Handle
}

func (v *SourceView) canCommand(c editCommand) bool {
	if v.Destroyed() || !v.mounted || !gui.IsEnabled(v) {
		return false
	}
	selected := v.Selection().Anchor != v.Selection().Caret
	clipboard := gui.App != nil && gui.App.Clipboard() != nil
	switch c {
	case commandUndo:
		return !v.readOnly && v.document.model.CanUndo()
	case commandRedo:
		return !v.readOnly && v.document.model.CanRedo()
	case commandCopy:
		return selected && clipboard
	case commandCut:
		return !v.readOnly && selected && clipboard
	case commandPaste:
		return !v.readOnly && clipboard
	case commandDelete:
		return !v.readOnly && selected
	case commandSelectAll:
		return v.document.model.Len() > 0
	}
	return false
}

func (v *SourceView) runCommand(c editCommand) {
	if !v.canCommand(c) {
		return
	}
	switch c {
	case commandUndo:
		v.Undo()
	case commandRedo:
		v.Redo()
	case commandCut:
		v.report(v.Cut())
	case commandCopy:
		v.report(v.Copy())
	case commandPaste:
		v.report(v.Paste())
	case commandDelete:
		v.report(v.ReplaceSelection(""))
	case commandSelectAll:
		v.SetSelection(textedit.Selection{Caret: v.document.model.Len()})
	}
}

func (v *SourceView) installContextMenu() {
	c := gui.NewContextMenuEventController()
	c.ConnectRequest(func(ctx gui.EventContext) {
		if !v.mounted || !gui.IsEnabled(v) {
			return
		}
		point, pointer := ctx.Position()
		v.report(v.showContextMenu(point, pointer))
		ctx.StopPropagation()
	})
	v.AddEventController(c)
}

func (v *SourceView) showContextMenu(point geometry.Point, pointer bool) error {
	state := v.operationState()
	v.cancelPreedit()
	if !state.valid(v) || !v.mounted || v.Window() == nil {
		return nil
	}
	v.Window().SetFocusedWidget(v)
	if !state.valid(v) || !v.mounted {
		return nil
	}
	if pointer {
		if !v.selectionContains(point) {
			pos := v.hit(point)
			v.setSelection(textedit.Selection{Anchor: pos.Offset, Caret: pos.Offset}, pos.Upstream, false)
		}
	} else if caret, ok := v.caretRect(); ok {
		point = geometry.Point{X: caret.X, Y: caret.Y + caret.Height}
	}
	if v.document != state.document || v.mountEpoch != state.mount || !v.mounted {
		return nil
	}
	v.document.model.BreakUndoGroup()
	if v.menu == nil {
		v.menu = &editMenu{popup: gui.NewPopoverMenu(v)}
		v.menu.popup.ConnectClosed(v.RequestPaint)
	}
	m := v.menu
	if m.language == nil {
		m.language = mlang.ConnectChanged(v.updateMenu)
	}
	items := gui.NewMenu()
	m.items = make(map[editCommand]*gui.MenuItem)
	for c := commandUndo; c <= commandSelectAll; c++ {
		if c == commandCut || c == commandSelectAll {
			items.AppendSeparator()
		}
		item := items.Append(commandLabels[c](), func() {
			if v.document == state.document && v.mountEpoch == state.mount && m.popup.Menu() == items {
				v.runCommand(c)
			}
		})
		item.SetEnabled(v.canCommand(c))
		m.items[c] = item
	}
	m.popup.SetMenu(items)
	return m.popup.ShowAt(point)
}

func (v *SourceView) updateMenu() {
	if v.menu == nil || !v.menu.popup.Visible() {
		return
	}
	for c, item := range v.menu.items {
		item.SetLabel(commandLabels[c]())
		item.SetEnabled(v.canCommand(c))
	}
}

func (v *SourceView) hideMenu() {
	if v.menu != nil {
		v.menu.popup.Hide()
	}
}

func (v *SourceView) selectionContains(point geometry.Point) bool {
	if point.X < v.gutter {
		return false
	}
	row := v.heights.At(max(0, point.Y+v.offset.Y-padding))
	r, _ := v.document.model.LineRange(row)
	s := v.Selection().Range()
	if s.End <= r.Start || s.Start > r.End {
		return false
	}
	p := v.paragraph(row)
	if p == nil {
		return false
	}
	local := geometry.Point{X: point.X - v.gutter - padding + v.offset.X, Y: point.Y + v.offset.Y - padding - v.heights.Top(row)}
	for _, rect := range p.geometry.Selection(textedit.Range{Start: max(s.Start, r.Start) - r.Start, End: min(s.End, r.End) - r.Start}) {
		if containsPoint(rect, local) {
			return true
		}
	}
	if s.End > r.End && row+1 < v.document.model.LineCount() {
		rect := p.geometry.Caret(textedit.Position{Offset: r.End - r.Start, Upstream: true})
		rect.Width = v.spaceWidth
		return containsPoint(rect, local)
	}
	return false
}
