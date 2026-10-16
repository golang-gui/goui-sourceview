package sourceview

import (
	"errors"
	"time"

	"github.com/golang-gui/goui-sourceview/internal/editor"
	"github.com/golang-gui/goui-sourceview/internal/highlight"
	"github.com/golang-gui/goui-sourceview/scheme"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/core/signal"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/platform/typography"
)

var (
	ErrReadOnly             = errors.New("sourceview: read only")
	ErrDisabled             = errors.New("sourceview: disabled")
	ErrNotMounted           = errors.New("sourceview: not mounted")
	ErrClipboardUnavailable = errors.New("sourceview: clipboard unavailable")
	ErrUnknownLanguage      = errors.New("sourceview: unknown language")
	ErrOperationLimit       = editor.ErrOperationLimit
	ErrOperationChanged     = errors.New("sourceview: operation superseded by a callback")
)

type WrapMode = typography.WrapMode

const (
	WrapNone     = typography.WrapNone
	WrapChar     = typography.WrapChar
	WrapWordChar = typography.WrapWordChar
)

type IndentOptions struct {
	TabWidth, IndentWidth int
	InsertSpaces          bool
}
type SearchQuery struct {
	Text                  string
	IgnoreCase, WholeWord bool
}
type searchMatch struct {
	raw                      textedit.Range
	revision, selectionEpoch uint64
}

// SourceView is an independent editable leaf. Put it in gui.ScrollView to add
// scrollbars. Construction owns no model/settings/root subscriptions.
type SourceView struct {
	pendingPaste    *pasteRequest
	compositionSeen bool
	gui.WidgetBase
	anchor                                      viewportAnchor
	anchorRevision                              uint64
	pendingAnchor                               bool
	document, private                           *Document
	selection                                   textedit.Selection
	selectionRevision, observed, selectionEpoch uint64
	upstream                                    bool
	readOnly, acceptsTab, escapeTab             bool
	indent                                      IndentOptions
	wrap                                        WrapMode
	showLineNumbers                             bool
	explicitScheme                              *scheme.Scheme
	appliedScheme                               *scheme.Scheme
	digitWidth                                  float32
	colors                                      scheme.Definition
	mounted                                     bool
	mountEpoch, pasteEpoch                      uint64
	context                                     typography.Context
	im                                          gui.IMContext
	projection                                  *textedit.Projection
	projectionRevision                          uint64
	resettingIM                                 bool
	source                                      textedit.EditSource
	selectionSignal                             signal.Signal1[textedit.Selection]
	errorSignal                                 signal.Signal1[error]
	scrollSignal                                signal.Signal1[geometry.Rectangle]
	changeSignal                                signal.Signal1[textedit.Change]
	viewport                                    geometry.Size
	offset                                      geometry.Point
	heights                                     textedit.HeightIndex
	lineHeight, spaceWidth, gutter              float32
	contentWidth                                float32
	metricsReady                                bool
	font                                        typography.FontInfo
	layouts                                     map[layoutKey]*paragraph
	layoutEpoch                                 uint64
	tick                                        uint64
	blink                                       *gui.Timer
	blinkHandle                                 signal.Handle
	caretOn                                     bool
	preferredX                                  float32
	preferredSet                                bool
	dragging                                    bool
	dragPoint                                   geometry.Point
	dragGutter                                  bool
	dragUnit                                    int // 0: caret, 1: word, 2: logical line
	dragRange                                   textedit.Range
	clickCount                                  int
	lastClick                                   time.Time
	lastClickPoint                              geometry.Point
	query                                       SearchQuery
	search                                      *editor.Search
	currentMatch                                *searchMatch
	menu                                        *editMenu
	autoPairs                                   bool
	pairs                                       []autoPair
	brackets                                    bracketState
	completion                                  *completionSession
	completionProvider                          CompletionProvider
	completionEpoch                             uint64
	closingCompletion                           bool
	automaticCompletion                         bool
	windowFocus                                 signal.Handle
}

func NewSourceView() *SourceView {
	d := NewDocument("")
	v := &SourceView{document: d, private: d, showLineNumbers: true, autoPairs: true, indent: IndentOptions{4, 4, true}, lineHeight: 22, spaceWidth: 10, caretOn: true, layouts: make(map[layoutKey]*paragraph), font: typography.FontInfo{Family: "monospace", Size: 12}}
	v.colors = scheme.Light().Definition()
	v.automaticCompletion = true
	v.SetFocusable(true)
	v.SetCursor(gui.CursorText)
	v.im = gui.NewIMContext()
	v.im.ConnectCommit(v.commit)
	v.im.ConnectPreedit(v.preedit)
	v.ConnectMount(v.mount)
	v.ConnectUnmount(v.unmount)
	v.ConnectFocused(func(focused bool) {
		if !focused {
			v.closeCompletion()
			v.cancelPreedit()
			v.dragging = false
		}
		v.resetBlink()
		v.RequestPaint()
	})
	v.installInput()
	v.installContextMenu()
	return v
}
func (v *SourceView) Document() *Document { return v.document }
func (v *SourceView) SetDocument(d *Document) {
	if d == nil {
		d = v.private
	}
	if v.document == d {
		return
	}
	state := v.operationState()
	v.cancelPreedit()
	if !state.valid(v) {
		return
	}
	v.cancelPaste()
	v.hideMenu()
	v.closeCompletion()
	if !state.valid(v) {
		return
	}
	if v.mounted {
		v.document.release(v)
	}
	v.document = d
	v.pairs = nil
	v.brackets = bracketState{}
	v.selection = textedit.Selection{}
	v.upstream = false
	v.selectionEpoch++
	v.selectionRevision = d.model.Revision()
	v.observed = d.model.Revision()
	if v.mounted {
		d.acquire(v)
	}
	v.currentMatch = nil
	v.anchor = viewportAnchor{}
	v.pendingAnchor = false
	v.offset = geometry.Point{}
	v.invalidateLayout()
	state = v.operationState()
	v.scrollSignal.Emit(geometry.Rect(0, 0, v.viewport.Width, v.viewport.Height))
	if state.valid(v) {
		v.selectionSignal.Emit(v.selection)
	}
}
func (v *SourceView) Selection() textedit.Selection { return v.clampSelection(v.selection) }
func (v *SourceView) SetSelection(s textedit.Selection) {
	state := v.operationState()
	v.cancelPreedit()
	if !state.valid(v) {
		return
	}
	v.document.model.BreakUndoGroup()
	v.setSelection(s, false, true)
}
func (v *SourceView) clampSelection(s textedit.Selection) textedit.Selection {
	s.Anchor = v.clampOffset(s.Anchor)
	s.Caret = v.clampOffset(s.Caret)
	return s
}
func (v *SourceView) clampOffset(offset int) int {
	m := v.document.model
	offset = max(0, min(offset, m.Len()))
	for offset > 0 && !m.IsRuneBoundary(offset) {
		offset--
	}
	return offset
}
func (v *SourceView) setSelection(s textedit.Selection, upstream, reveal bool) {
	state := v.operationState()
	s = v.clampSelection(s)
	if v.context != nil && v.projection == nil {
		if s.Anchor == s.Caret {
			s.Caret = v.snapOffset(s.Caret, true)
			s.Anchor = s.Caret
		} else {
			forward := s.Caret >= s.Anchor
			s.Anchor = v.snapOffset(s.Anchor, !forward)
			s.Caret = v.snapOffset(s.Caret, forward)
		}
	}
	if !state.valid(v) {
		return
	}
	old := v.selection
	if reveal {
		v.pendingAnchor = false
		v.anchor.valid = false
	}
	v.selection = s
	v.upstream = upstream
	v.selectionRevision = v.document.model.Revision()
	v.selectionEpoch++
	v.cancelPaste()
	v.closeCompletion()
	if v.document != state.document || v.mountEpoch != state.mount || v.document.model.Revision() != state.revision || v.selectionEpoch != state.selection+1 {
		return
	}
	v.currentMatch = nil
	v.preferredSet = false
	v.resetBlink()
	v.RequestPaint()
	selectionEpoch := v.selectionEpoch
	v.updateMenu()
	if reveal {
		v.RevealSelection()
	}
	if old != s && v.selectionEpoch == selectionEpoch {
		v.selectionSignal.Emit(s)
	}
}
func (v *SourceView) ReadOnly() bool { return v.readOnly }
func (v *SourceView) SetReadOnly(value bool) {
	if v.readOnly == value {
		return
	}
	v.readOnly = value
	if value {
		v.closeCompletion()
		v.cancelPreedit()
		v.cancelPaste()
	}
	v.updateMenu()
	v.RequestPaint()
}
func (v *SourceView) AcceptsTab() bool { return v.acceptsTab }
func (v *SourceView) SetAcceptsTab(value bool) {
	if v.acceptsTab == value {
		return
	}
	v.acceptsTab = value
	v.escapeTab = false
}
func (v *SourceView) IndentOptions() IndentOptions { return v.indent }
func (v *SourceView) SetIndentOptions(o IndentOptions) error {
	normalized, err := (editor.IndentOptions{TabWidth: o.TabWidth, IndentWidth: o.IndentWidth, InsertSpaces: o.InsertSpaces}).Normalized()
	if err != nil {
		return err
	}
	value := IndentOptions{normalized.TabWidth, normalized.IndentWidth, normalized.InsertSpaces}
	if value != v.indent {
		v.indent = value
		v.invalidateLayout()
	}
	return nil
}
func (v *SourceView) WrapMode() WrapMode { return v.wrap }
func (v *SourceView) SetWrapMode(w WrapMode) {
	if w < WrapNone || w > WrapWordChar {
		panic("sourceview: invalid wrap mode")
	}
	if v.wrap != w {
		v.wrap = w
		v.offset.X = 0
		v.anchor.x = 0
		v.invalidateLayout()
	}
}
func (v *SourceView) ShowLineNumbers() bool { return v.showLineNumbers }
func (v *SourceView) SetShowLineNumbers(show bool) {
	if v.showLineNumbers != show {
		v.showLineNumbers = show
		v.invalidateLayout()
	}
}
func (v *SourceView) SetScheme(s *scheme.Scheme) { v.explicitScheme = s; v.updateScheme() }
func (v *SourceView) ConnectSelection(fn func(textedit.Selection)) signal.Handle {
	return v.selectionSignal.Connect(fn)
}
func (v *SourceView) ConnectError(fn func(error)) signal.Handle { return v.errorSignal.Connect(fn) }
func (v *SourceView) ConnectChange(fn func(textedit.Change)) signal.Handle {
	return v.changeSignal.Connect(fn)
}
func (v *SourceView) ConnectScrollIntoView(fn func(geometry.Rectangle)) signal.Handle {
	return v.scrollSignal.Connect(fn)
}
func (v *SourceView) IMContext() gui.IMContext {
	if v.readOnly || !gui.IsEnabled(v) {
		return nil
	}
	return v.im
}
func (v *SourceView) changed(c textedit.Change, splices []highlight.Splice, reset bool) {
	epoch, d := v.mountEpoch, v.document
	v.observed = c.Revision
	v.closeCompletion()
	if v.document != d || v.mountEpoch != epoch || !v.mounted {
		return
	}
	v.mapPairs(c)
	if c.Reset {
		v.anchor.valid = false
		v.pendingAnchor = false
	} else if v.anchor.valid && v.anchorRevision < c.Revision {
		for _, edit := range c.Edits {
			v.anchor.offset = textedit.MapOffset(v.anchor.offset, edit)
		}
		v.anchorRevision = c.Revision
		v.pendingAnchor = true
	}
	v.cancelPreedit()
	if v.document != d || v.mountEpoch != epoch || !v.mounted {
		return
	}
	v.cancelPaste()
	v.currentMatch = nil
	v.updateMenu()
	old := v.selection
	if v.selectionRevision < c.Revision {
		if c.Reset {
			v.selection = textedit.Selection{}
		} else {
			for _, e := range c.Edits {
				v.selection.Anchor = textedit.MapOffset(v.selection.Anchor, e)
				v.selection.Caret = textedit.MapOffset(v.selection.Caret, e)
			}
		}
		v.selectionRevision = c.Revision
	}
	if reset || c.Reset {
		v.invalidateLayout()
	} else {
		for _, s := range splices {
			v.heights.Splice(s.Index, s.Removed, s.Added, v.lineHeight)
		}
		v.contentWidth = 0
		// Stable line IDs retain layouts of untouched suffixes; changed paragraphs
		// receive new IDs and are evicted by the bounded view-local cache.
		v.RequestLayout()
		v.RequestPaint()
	}
	if c.Reset {
		v.offset = geometry.Point{}
		v.scrollSignal.Emit(geometry.Rect(0, 0, v.viewport.Width, v.viewport.Height))
	}
	if v.mountEpoch != epoch || v.document != d || !v.mounted {
		return
	}
	if old != v.selection {
		v.selectionSignal.Emit(v.Selection())
	}
	if v.mounted && v.mountEpoch == epoch && v.document == d {
		v.changeSignal.Emit(c)
	}
}
func (v *SourceView) mount() {
	v.mounted = true
	v.mountEpoch++
	if win := v.Window(); win != nil {
		v.windowFocus = win.ConnectFocus(func(focused bool) {
			if !focused {
				v.closeCompletion()
			}
			v.RequestPaint()
		})
	}
	if v.observed != v.document.model.Revision() {
		v.anchor.valid = false
		v.pendingAnchor = false
		v.offset = geometry.Point{}
	}
	v.context = gui.App.Typography()
	v.document.acquire(v)
	attachFrame(v)
	attachSettings(v)
	v.updateScheme()
	v.invalidateLayout()
	v.blink = gui.App.NewTimer()
	v.blinkHandle = v.blink.ConnectTimeout(func() {
		if !v.mounted || !activeFocus(v) || !visible(v) {
			v.blink.Stop()
			return
		}
		v.caretOn = !v.caretOn
		if v.dragging {
			v.dragSelection(v.dragPoint)
		}
		v.RequestPaint()
	})
	v.resetBlink()
}
func (v *SourceView) unmount() {
	v.mounted = false
	v.mountEpoch++
	v.cancelPaste()
	v.hideMenu()
	v.closeCompletion()
	if v.windowFocus != nil {
		v.windowFocus.Disconnect()
		v.windowFocus = nil
	}
	if v.menu != nil && v.menu.language != nil {
		v.menu.language.Disconnect()
		v.menu.language = nil
	}
	v.dragging = false
	v.pairs = nil
	v.brackets = bracketState{}
	if v.blink != nil {
		v.blink.Stop()
		v.blinkHandle.Disconnect()
		v.blink = nil
	}
	detachFrame(v)
	detachSettings(v)
	v.document.release(v)
	v.cancelPreedit()
	v.dropLayouts()
	v.context = nil
}
func (v *SourceView) resetBlink() {
	v.caretOn = true
	if v.blink == nil {
		return
	}
	if activeFocus(v) && visible(v) {
		_ = v.blink.Start(500 * time.Millisecond)
	} else {
		v.blink.Stop()
	}
}
func visible(v gui.Widget) bool {
	for w := v; w != nil; w = w.Parent() {
		if !w.Visible() {
			return false
		}
	}
	return true
}

func activeFocus(v *SourceView) bool {
	return v.Focused() && (v.Window() == nil || v.Window().Focused() || v.menu != nil && v.menu.popup.Visible())
}

// Capture before operations that can synchronously call application code.
type operationState struct {
	document                   *Document
	revision, mount, selection uint64
}

func (v *SourceView) operationState() operationState {
	return operationState{v.document, v.document.model.Revision(), v.mountEpoch, v.selectionEpoch}
}
func (s operationState) valid(v *SourceView) bool {
	return v.document == s.document && v.document.model.Revision() == s.revision && v.mountEpoch == s.mount && v.selectionEpoch == s.selection
}
func (v *SourceView) report(err error) {
	if err != nil {
		v.errorSignal.Emit(err)
	}
}
func (v *SourceView) editable() error {
	if v.Destroyed() {
		return ErrDisabled
	}
	if v.readOnly {
		return ErrReadOnly
	}
	if !gui.IsEnabled(v) {
		return ErrDisabled
	}
	return nil
}

var _ gui.Widget = (*SourceView)(nil)
var _ gui.Scrollable = (*SourceView)(nil)
var _ gui.ScrollIntoViewRequester = (*SourceView)(nil)
var _ gui.IMClient = (*SourceView)(nil)
