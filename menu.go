package sourceview

import (
	"github.com/golang-gui/goui-sourceview/internal/editor"
	"github.com/golang-gui/goui-sourceview/internal/resource"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/core/signal"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/mlang"
)

var commandLabels = [...]mlang.Getter{
	CommandUndo:               resource.Undo,
	CommandRedo:               resource.Redo,
	CommandCut:                resource.Cut,
	CommandCopy:               resource.Copy,
	CommandPaste:              resource.Paste,
	CommandDeleteSelection:    resource.Delete,
	CommandMoveLinesUp:        resource.MoveLinesUp,
	CommandMoveLinesDown:      resource.MoveLinesDown,
	CommandDuplicateLinesUp:   resource.DuplicateLinesUp,
	CommandDuplicateLinesDown: resource.DuplicateLinesDown,
	CommandDeleteLines:        resource.DeleteLines,
	CommandSelectAll:          resource.SelectAll,
	CommandFold:               resource.Fold,
	CommandUnfold:             resource.Unfold,
	CommandFoldAll:            resource.FoldAll,
	CommandUnfoldAll:          resource.UnfoldAll,
}

type editMenu struct {
	popup    *gui.PopoverMenu
	items    map[Command]*gui.MenuItem
	language signal.Handle
}

func (v *SourceView) canCommand(c Command) bool {
	if v.Destroyed() || !v.mounted || !gui.IsEnabled(v) {
		return false
	}
	selected := v.Selection().Anchor != v.Selection().Caret
	clipboard := gui.App != nil && gui.App.Clipboard() != nil
	switch c {
	case CommandFold, CommandUnfold, CommandFoldAll, CommandUnfoldAll:
		return v.canFold() && len(v.folds) > 0
	case CommandUndo:
		return !v.readOnly && v.document.model.CanUndo()
	case CommandRedo:
		return !v.readOnly && v.document.model.CanRedo()
	case CommandCopy:
		return selected && clipboard
	case CommandCut:
		return !v.readOnly && selected && clipboard
	case CommandPaste:
		return !v.readOnly && clipboard
	case CommandDeleteSelection:
		return !v.readOnly && selected
	case CommandMoveLinesUp, CommandMoveLinesDown:
		if v.readOnly {
			return false
		}
		first, last, err := editor.SelectedLines(v.document.model, v.Selection())
		return err == nil && (c == CommandMoveLinesUp && first > 0 || c == CommandMoveLinesDown && last+1 < v.document.model.LineCount())
	case CommandDuplicateLinesUp, CommandDuplicateLinesDown:
		return !v.readOnly
	case CommandDeleteLines:
		return !v.readOnly && v.document.model.Len() > 0
	case CommandSelectAll:
		return v.document.model.Len() > 0
	}
	return false
}

func (v *SourceView) runCommand(c Command) {
	if !v.canCommand(c) {
		return
	}
	switch c {
	case CommandFold:
		v.foldAtCaret(true)
	case CommandUnfold:
		v.foldAtCaret(false)
	case CommandFoldAll:
		v.FoldAll()
	case CommandUnfoldAll:
		v.UnfoldAll()
	case CommandUndo:
		v.Undo()
	case CommandRedo:
		v.Redo()
	case CommandCut:
		v.report(v.Cut())
	case CommandCopy:
		v.report(v.Copy())
	case CommandPaste:
		v.report(v.Paste())
	case CommandDeleteSelection:
		v.report(v.ReplaceSelection(""))
	case CommandMoveLinesUp, CommandMoveLinesDown:
		v.report(v.MoveLines(c == CommandMoveLinesDown))
	case CommandDuplicateLinesUp, CommandDuplicateLinesDown:
		v.report(v.DuplicateLines(c == CommandDuplicateLinesDown))
	case CommandDeleteLines:
		v.report(v.DeleteLines())
	case CommandSelectAll:
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
	v.closeCompletion()
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
	m.items = make(map[Command]*gui.MenuItem)
	for c := CommandUndo; c <= CommandUnfoldAll; c++ {
		if c == CommandCut || c == CommandMoveLinesUp || c == CommandSelectAll || c == CommandFold {
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
	row := v.paragraphAt(max(0, point.Y+v.offset.Y-padding))
	r, _ := v.document.model.LineRange(row)
	s := v.Selection().Range()
	if s.End <= r.Start || s.Start > r.End {
		return false
	}
	p := v.paragraph(row)
	if p == nil {
		return false
	}
	local := geometry.Point{X: point.X - v.gutter - padding + v.offset.X, Y: point.Y + v.offset.Y - padding - v.paragraphTop(row)}
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
