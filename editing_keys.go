package sourceview

import (
	"runtime"

	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/platform/events"
)

// editingKey recognizes exact editor bindings after transient completion input.
// A recognized editing key remains consumed in read-only mode.
func (v *SourceView) editingKey(e events.KeyEvent) bool {
	primary, _ := gui.ModPrimary.Resolve()
	word := primary
	if runtime.GOOS == "darwin" {
		word, _ = gui.ModAlt.Resolve()
	}
	shift := e.Modifiers&events.ModifierShift != 0
	base := e.Modifiers &^ events.ModifierShift
	run := func(fn func() error) {
		if v.editable() == nil {
			v.report(fn())
		}
	}
	if base == word {
		switch e.Key {
		case events.KeyArrowLeft, events.KeyArrowRight:
			v.MoveWord(e.Key == events.KeyArrowRight, shift)
			return true
		case events.KeyBackspace, events.KeyDelete:
			if !shift {
				run(func() error { return v.DeleteWord(e.Key == events.KeyDelete) })
				return true
			}
		}
	}
	if base == primary {
		switch e.Key {
		case events.KeyEnter:
			run(func() error { return v.InsertLine(!shift) })
			return true
		case events.KeyBracketLeft, events.KeyBracketRight:
			if !shift {
				run(func() error { return v.Indent(e.Key == events.KeyBracketLeft) })
				return true
			}
		case events.KeyL:
			if !shift {
				v.SelectLine()
				return true
			}
		case events.KeyBackslash:
			if shift {
				v.JumpToBracket()
				return true
			}
		case events.KeyG:
			if runtime.GOOS == "darwin" {
				v.FindNext(shift)
				return true
			}
		case events.KeyArrowLeft, events.KeyArrowRight:
			if runtime.GOOS == "darwin" {
				key := events.KeyHome
				if e.Key == events.KeyArrowRight {
					key = events.KeyEnd
				}
				v.navigate(key, shift)
				return true
			}
		case events.KeyArrowUp, events.KeyArrowDown:
			if runtime.GOOS == "darwin" {
				at := 0
				if e.Key == events.KeyArrowDown {
					at = v.document.model.Len()
				}
				v.moveTo(at, false, shift)
				return true
			}
		}
	}
	if e.Key == events.KeyF3 && base == 0 {
		v.FindNext(shift)
		return true
	}
	return false
}
