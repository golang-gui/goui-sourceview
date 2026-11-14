package sourceview

import (
	"errors"
	"image/color"
	"time"

	"github.com/golang-gui/goui-sourceview/internal/controls"
	"github.com/golang-gui/goui-sourceview/internal/editor"
	"github.com/golang-gui/goui-sourceview/internal/folding"
	"github.com/golang-gui/goui-sourceview/internal/highlight"
	textcore "github.com/golang-gui/goui-sourceview/internal/text"
	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui-sourceview/scheme"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/core/signal"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/platform/typography"
)

var (
	ErrReadOnly             = errors.New("sourceview: read only")
	ErrDisabled             = errors.New("sourceview: disabled")
	ErrNotMounted           = errors.New("sourceview: not mounted")
	ErrClipboardUnavailable = errors.New("sourceview: clipboard unavailable")
	ErrUnknownLanguage      = errors.New("sourceview: unknown language")
	ErrOperationLimit       = editor.ErrOperationLimit
	ErrMultipleSelections   = errors.New("sourceview: operation requires one selection")
	ErrOperationChanged     = errors.New("sourceview: operation context changed")
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

// SourceView is an editable code control with built-in find, replace and go-to
// interfaces. Put it in gui.ScrollView to add scrollbars; set that host's
// StyleName to "source-view" for a shared outer
// frame. Construction owns no model/settings/root subscriptions.
type SourceView struct {
	gui.WidgetBase

	// Optional view-local overview.
	showMinimap     bool
	minimapOptions  MinimapOptions
	minimap         *minimapState
	minimapGeometry uint64

	// Document ownership and mounting.
	document, private      *Document
	mounted                bool
	mountEpoch, pasteEpoch uint64
	observed               uint64
	source                 textcore.EditSource
	windowFocus            signal.Handle

	// Selection and editing options.
	selectionTrail                                selectionTrail
	cursorUndo                                    cursorUndo
	selectionGestureID                            uint64
	dragRemoveSelection                           int
	dragStartPoint                                geometry.Point
	selection                                     TextSelection
	additional                                    []cursorState
	columnSelection                               *columnSelection
	selectionModifiers                            SelectionModifiers
	additiveDrag                                  bool
	dragBase                                      []cursorState
	selectionRangesCache                          []TextRange
	selectionRangesEpoch, selectionRangesRevision uint64
	selectionCommit                               *selectionCommit
	selectionRevision, selectionEpoch             uint64
	editOptionsEpoch                              uint64
	upstream                                      bool
	preferredX                                    float64
	preferredSet                                  bool
	readOnly, acceptsTab, escapeTab               bool
	indent                                        IndentOptions
	wrap                                          WrapMode
	showLineNumbers                               bool
	showLineMarks                                 bool
	markColumn                                    float32
	execution                                     *trackedLineMark
	lineMarkSignal                                signal.Signal1[LineMarkEvent]

	// Native text and input-method resources belong to this view.
	context                      typography.Context
	im                           gui.IMContext
	projection                   *textcore.Projection
	projectionRevision           uint64
	compositionSeen, resettingIM bool
	pendingPaste                 *pasteRequest
	editQueue                    *editingQueue

	// Effective appearance.
	explicitScheme, appliedScheme, themeScheme *scheme.Scheme
	highlightColors                            map[language.Kind]color.NRGBA
	highlightStyleEpoch                        uint64
	colors                                     scheme.Definition
	font                                       typography.FontInfo
	digitWidth, lineHeight, spaceWidth, gutter float32
	metricsReady                               bool

	// Viewport, measurements and deferred layout.
	viewport                   geometry.Size
	offset                     geometry.Point
	heights                    textcore.HeightIndex
	heightEstimated            bool
	visibleLines               folding.VisibleLines
	hiddenLines                []folding.Interval
	contentWidth               float32
	layouts                    map[layoutKey]*paragraph
	layoutFailures             map[layoutKey]struct{}
	layoutEpoch                uint64
	frameLayout, layoutPending bool
	initialLayout              *viewportInitialization
	tick                       uint64
	anchor                     viewportAnchor
	anchorRevision             uint64
	pendingAnchor              bool
	viewRestore                *viewRestore
	reveal                     *operationState

	// Folding and editing assistance.
	foldIntake                                                                        *foldIntake
	foldClear                                                                         bool
	foldState                                                                         uint64
	foldColumn                                                                        float32
	foldingEnabled                                                                    bool
	folds                                                                             []folding.Entry
	collapsedFolds                                                                    map[int]TextRange
	collapsedLines                                                                    map[int]TextRange
	collapsedRevision                                                                 uint64
	foldRevision                                                                      uint64
	foldDefinition                                                                    *language.Language
	whitespace                                                                        WhitespaceMode
	showLineEndings, indentGuides, smartHomeEnd, smartBackspace, highlightOccurrences bool
	showSpecialCharacters                                                             bool
	rulers                                                                            []int
	linkedTags                                                                        bool
	tagMatches                                                                        tagMatchState
	autoPairs                                                                         bool
	pairs                                                                             []autoPair
	brackets                                                                          bracketState

	// Keyboard, pointer and caret state.
	pendingInput     *inputBatch
	keyBindings      KeyBindings
	boundKeys        []controls.BoundKey[Command]
	keyBindingsEpoch uint64
	pointer          *gui.MotionEventController
	pointerPoint     geometry.Point
	dragging         bool
	dragPoint        geometry.Point
	dragInline       bool // keep the initial hit while dismissed inline space closes
	dragGutter       bool
	dragUnit         int // 0: caret, 1: word, 2: logical line
	dragRange        TextRange
	clickCount       int
	lastClick        time.Time
	lastClickPoint   geometry.Point
	blink            *gui.Timer
	blinkHandle      signal.Handle
	caretOn          bool
	menu             *editMenu

	// Search state is independent of the internal find UI lifetime.
	findBar                *findBar
	goToBar                *goToBar
	companionEpoch         uint64
	query                  SearchQuery
	highlightSearchMatches bool
	search                 *editor.Search
	searchRange            *TextRange
	currentMatch           *searchMatch

	// Optional host-provided inline suffix; its native layout belongs to this view.
	inlineSuggestionsEnabled bool
	inlineSuggestion         *inlineSuggestionState

	// Completion sessions and their cancellable provider work.
	completion                             *completionSession
	completionProvider                     *CompletionProvider
	completionQueue                        *completionQueue
	completionEdit                         *completionEdit
	completionEpoch                        uint64
	closingCompletion, automaticCompletion bool

	selectionSignal  signal.Signal1[TextSelection]
	selectionsSignal signal.Signal1[[]TextSelection]
	errorSignal      signal.Signal1[error]
	scrollSignal     signal.Signal1[geometry.Rectangle]
	changeSignal     signal.Signal1[TextChange]
	foldingSignal    signal.Signal0
	searchState      signal.Signal0
}

func NewSourceView() *SourceView {
	d := NewDocument("")
	v := &SourceView{linkedTags: true, document: d, private: d, showLineNumbers: true, autoPairs: true, indent: IndentOptions{4, 4, true}, lineHeight: 22, spaceWidth: 10, caretOn: true, layouts: make(map[layoutKey]*paragraph), font: typography.FontInfo{Family: "monospace", Size: 12}}
	v.selectionModifiers = defaultSelectionModifiers()
	v.indentGuides, v.smartHomeEnd, v.smartBackspace, v.highlightOccurrences = true, true, true, true
	v.highlightSearchMatches = true
	v.foldingEnabled = true
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
			v.ClearInlineSuggestion()
			v.cancelEdit()
			v.pendingInput = nil
			v.closeCompletion()
			v.cancelPreedit()
			v.dragging = false
			v.columnSelection = nil
			v.dragBase = nil
			v.additiveDrag = false
		}
		v.resetBlink()
		v.RequestPaint()
	})
	if err := v.SetKeyBindings(DefaultKeyBindings()); err != nil {
		panic(err) // Built-in tables are validated on every supported platform.
	}
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
	v.cursorUndo = cursorUndo{}
	v.cancelViewRestore()
	defer v.searchState.Emit()
	v.ClearInlineSuggestion()
	state := v.operationState()
	v.cancelPreedit()
	if !state.valid(v) {
		return
	}
	v.cancelPaste()
	v.cancelEdit()
	v.hideMenu()
	v.closeCompletion()
	if !state.valid(v) {
		return
	}
	if v.mounted {
		v.document.release(v)
	}
	v.columnSelection = nil
	v.dragBase = nil
	v.additiveDrag = false
	v.dragging = false
	v.document = d
	v.execution = nil
	v.minimap = nil
	v.searchRange = nil
	if v.search != nil {
		v.search.SetRange(nil)
	}
	v.foldIntake = nil
	v.foldClear = false
	v.folds = nil
	v.collapsedFolds = nil
	v.collapsedLines = nil
	v.hiddenLines = nil
	v.foldRevision = d.model.Revision()
	v.foldDefinition = d.definition
	v.pairs = nil
	v.brackets = bracketState{}
	v.tagMatches = tagMatchState{}
	v.selection = TextSelection{}
	v.additional = nil
	v.upstream = false
	v.selectionEpoch++
	v.selectionRevision = d.model.Revision()
	v.observed = d.model.Revision()
	if v.mounted {
		d.acquire(v)
		if v.Destroyed() || v.document != d || v.mountEpoch != state.mount {
			return
		}
	}
	v.currentMatch = nil
	v.anchor = viewportAnchor{}
	v.pendingAnchor = false
	v.reveal = nil
	v.offset = geometry.Point{}
	v.invalidateTextLayout()
	state = v.operationState()
	if v.mounted {
		v.updateScheme()
		if !state.valid(v) {
			return
		}
	}
	v.scrollSignal.Emit(geometry.Rect(0, 0, v.viewport.Width, v.viewport.Height))
	if state.valid(v) {
		v.selectionSignal.Emit(v.selection)
		if state.valid(v) {
			v.selectionsSignal.Emit(v.Selections())
		}
	}
}

func (v *SourceView) Selection() TextSelection { return v.clampSelection(v.selection) }

func (v *SourceView) SetSelection(s TextSelection) {
	v.pendingInput = nil
	state := v.operationState()
	v.cancelPreedit()
	if !state.valid(v) {
		return
	}
	v.document.model.BreakUndoGroup()
	v.setSelection(s, false, true)
}

func (v *SourceView) clampSelection(s TextSelection) TextSelection {
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

func (v *SourceView) setSelection(s TextSelection, upstream, reveal bool) {
	v.setSelectionKeepingCompletion(s, upstream, reveal, nil)
}

// Only owned input or keyboard movement inside a completion word retains its UI.
// Explicit setters, pointer input and reentrant callbacks use setSelection.
func (v *SourceView) setSelectionKeepingCompletion(s TextSelection, upstream, reveal bool, keep *completionSession) {
	v.setCursorStatesKeepingCompletion([]cursorState{{selection: s, upstream: upstream}}, reveal, keep)
}

func (v *SourceView) ReadOnly() bool { return v.readOnly }

func (v *SourceView) SetReadOnly(value bool) {
	if v.readOnly == value {
		return
	}
	v.pendingInput = nil
	v.readOnly = value
	defer v.searchState.Emit()
	if value {
		v.ClearInlineSuggestion()
		v.closeCompletion()
		v.cancelPreedit()
		v.cancelPaste()
		v.cancelEdit()
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
		v.editOptionsChanged()
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

// SetScheme overrides code-area colors only. nil uses the owning stylesheet.
func (v *SourceView) SetScheme(s *scheme.Scheme) { v.explicitScheme = s; v.updateScheme() }

func (v *SourceView) ConnectSelection(fn func(TextSelection)) signal.Handle {
	return v.selectionSignal.Connect(fn)
}

func (v *SourceView) ConnectError(fn func(error)) signal.Handle { return v.errorSignal.Connect(fn) }

func (v *SourceView) ConnectChange(fn func(TextChange)) signal.Handle {
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

// Capture before operations that can synchronously call application code.
type operationState struct {
	document                                       *Document
	revision, mount, selection, language, settings uint64
}

func (v *SourceView) operationState() operationState {
	return operationState{v.document, v.document.model.Revision(), v.mountEpoch, v.selectionEpoch, v.document.languageEpoch, v.editOptionsEpoch}
}

func (s operationState) valid(v *SourceView) bool {
	return v.document == s.document && v.document.model.Revision() == s.revision && v.mountEpoch == s.mount && v.selectionEpoch == s.selection && v.document.languageEpoch == s.language
}

func (s operationState) editingValid(v *SourceView) bool {
	return s.valid(v) && v.editOptionsEpoch == s.settings
}

func (v *SourceView) report(err error) {
	if err != nil {
		v.pendingInput = nil
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

func (v *SourceView) Snapshot() gui.WidgetInfo {
	info := v.WidgetBase.Snapshot()
	info.Role = gui.Role("sourceview")
	info.Text = ""
	info.SetAttribute("goui.sourceview", map[string]any{"language": string(v.document.Language()), "revision": v.document.model.Revision(), "wrap": int(v.wrap), "lineNumbers": v.showLineNumbers, "layoutPending": v.layoutPending, "specialCharacters": v.showSpecialCharacters})
	m := v.document.model
	if v.mounted && v.observed != m.Revision() {
		return info
	}
	first, last := v.visibleRows()
	firstDisplay := first
	if v.projection != nil {
		first = m.LineAt(v.projection.ModelOffset(v.displayRange(first).Start, false))
		last = m.LineAt(v.projection.ModelOffset(v.displayRange(last).End, true))
	}
	r, _ := m.LineRange(first)
	end, _ := m.LineRange(last)
	if end.End-r.Start > 64*1024 {
		// Snapshot reads cached geometry only: it must not shape text, recolor,
		// emit callbacks or alter the viewport just to describe visible content.
		p := v.cachedParagraph(firstDisplay)
		if p != nil && p.Geometry.LineCount() > 0 {
			origin := v.paragraphOrigin(firstDisplay, p)
			pos := p.Geometry.Hit(geometry.Point{X: v.gutter + padding - origin.X, Y: -origin.Y})
			anchor := v.displayRange(firstDisplay).Start + pos.Offset
			if v.projection != nil {
				anchor = v.projection.ModelOffset(anchor, pos.Upstream)
			}
			r.Start = max(r.Start, min(end.End, anchor)-1024)
			for r.Start < end.End && !m.IsRuneBoundary(r.Start) {
				r.Start++
			}
		}
	}
	r.End = min(end.End, r.Start+64*1024)
	for r.End > r.Start && !m.IsRuneBoundary(r.End) {
		r.End--
	}
	text, _ := m.Slice(r)
	info.Attributes["goui.sourceview"].(map[string]any)["selections"] = v.snapshotSelections()
	info.Attributes["goui.sourceview"].(map[string]any)["inputPending"] = v.pendingInput != nil
	info.TextEditing = &gui.TextEditingInfo{ReadOnly: v.readOnly, Selection: gui.TextSelection(v.Selection()), Length: m.Len(), VisibleRange: gui.TextRange(r), VisibleText: text}
	v.snapshotFolding(&info, first, last)
	v.snapshotCollapsedLines(&info, first, last)
	v.snapshotLineMarks(&info, first, last)
	v.snapshotMinimap(&info)
	v.snapshotInlineSuggestion(&info)
	if v.projection != nil {
		text := v.projection.Text()
		limit := textcore.ClampOffset(text, min(len(text), 64*1024))
		info.TextEditing.Preedit = &gui.TextPreeditInfo{Replacement: gui.TextRange(v.projection.Replacement()), Text: text[:limit], Caret: v.projection.Caret(), Length: len(text)}
	}
	return info
}

func (v *SourceView) changed(c TextChange, splices []highlight.Splice, reset bool) {
	v.cursorUndo = cursorUndo{}
	v.columnSelection = nil
	v.cancelViewRestore()
	v.ClearInlineSuggestion()
	v.initialLayout = nil
	if e := v.execution; e != nil && e.revision < c.Revision {
		if e.revision+1 != c.Revision {
			e.valid = false
		}
		e.offset, e.valid = editor.MapLineAnchor(e.offset, e.valid, c)
		e.revision = c.Revision
	}
	defer v.searchState.Emit()
	epoch, d := v.mountEpoch, v.document
	v.observed = c.Revision
	v.reveal = nil
	if !v.retainCompletionChange(c) {
		v.closeCompletion()
	}
	if v.document != d || v.mountEpoch != epoch || !v.mounted {
		return
	}
	hadHiddenRows := len(v.hiddenLines) > 0
	lineCollapseChanged := v.mapCollapsedLines(c)
	v.mapFolds(c, splices)
	v.mapParagraphs(splices)
	v.changeLongParagraphs(c)
	v.mapPairs(c)
	v.mapSearchRange(c)
	if c.Reset {
		v.anchor.valid = false
		v.pendingAnchor = false
	} else if v.anchor.valid && v.anchorRevision < c.Revision {
		for _, edit := range c.Edits {
			v.anchor.offset = textcore.MapOffset(v.anchor.offset, edit)
		}
		v.anchorRevision = c.Revision
		v.pendingAnchor = true
	}
	v.cancelPreedit()
	if v.document != d || v.mountEpoch != epoch || !v.mounted {
		return
	}
	v.cancelPaste()
	v.cancelEdit()
	v.currentMatch = nil
	v.updateMenu()
	old := v.Selections()
	if v.selectionRevision < c.Revision {
		if commit := v.selectionCommit; commit != nil && !commit.applied && commit.state.document == v.document && commit.state.mount == v.mountEpoch && commit.state.revision+1 == c.Revision && commit.state.selection == v.selectionEpoch {
			v.assignCursors(commit.after)
			v.selectionEpoch++
			commit.applied = true
		} else if c.Reset {
			v.assignCursors(nil)
		} else {
			cursors := v.cursors()
			offsets := make([]int, 0, len(cursors)*2)
			for _, s := range cursors {
				offsets = append(offsets, s.selection.Anchor, s.selection.Caret)
			}
			mapped := textcore.MapOffsets(offsets, c.Edits)
			for i := range cursors {
				cursors[i].selection = TextSelection{Anchor: mapped[i*2], Caret: mapped[i*2+1]}
			}
			v.assignCursors(normalizeCursors(cursors))
		}
		v.selectionRevision = c.Revision
	}

	if reset || c.Reset || hadHiddenRows {
		if c.Reset {
			v.invalidateTextLayout()
		} else {
			v.invalidateLayout()
		}
	} else {
		for _, s := range splices {
			estimate := v.lineHeight
			if s.Removed == 1 && s.Added == 1 {
				// A local edit invalidates geometry, not the previous scrolling
				// estimate. Keeping it avoids hiding the scrollbar and laying
				// out the replacement at an unnecessary intermediate width.
				estimate = v.heights.Top(s.Index+1) - v.heights.Top(s.Index)
			}
			v.heights.Splice(s.Index, s.Removed, s.Added, estimate)
		}
		// With no collapsed ranges, logical/display rows still correspond
		// directly. Update their count without throwing away spliced heights.
		v.visibleLines.Reset(v.displayCount(), nil)
		v.contentWidth = 0
		// Stable line IDs retain layouts of untouched suffixes; changed paragraphs
		// receive new IDs and are evicted by the bounded view-local cache.
		v.RequestLayout()
		v.RequestPaint()
	}
	if lineCollapseChanged {
		v.beginViewportInitialization()
		v.requestLayoutWork()
	}
	if c.Reset {
		v.offset = geometry.Point{}
		v.scrollSignal.Emit(geometry.Rect(0, 0, v.viewport.Width, v.viewport.Height))
	}
	if v.mountEpoch != epoch || v.document != d || !v.mounted {
		return
	}
	v.emitSelections(old)
	if v.mounted && v.mountEpoch == epoch && v.document == d {
		v.changeSignal.Emit(c)
		if lineCollapseChanged && v.mounted && v.mountEpoch == epoch && v.document == d && !v.Destroyed() {
			v.foldingSignal.Emit()
		}
	}
}

func (v *SourceView) mount() {
	request := v.reveal
	if request != nil && !request.valid(v) {
		v.reveal = nil
	}
	v.mounted = true
	v.mountEpoch++
	if v.reveal != nil {
		// A selection requested before first allocation still needs revealing.
		*v.reveal = v.operationState()
	}
	if win := v.Surface(); win != nil {
		v.windowFocus = win.ConnectFocus(func(focused bool) {
			if !focused {
				v.ClearInlineSuggestion()
				v.pendingInput = nil
				v.closeCompletion()
			}
			v.RequestPaint()
		})
	}
	if v.observed != v.document.model.Revision() {
		v.collapsedLines = nil
		v.searchRange = nil
		if v.search != nil {
			v.search.SetRange(nil)
		}
		v.anchor.valid = false
		v.pendingAnchor = false
		v.offset = geometry.Point{}
	}
	if v.foldRevision != v.document.model.Revision() || v.foldDefinition != v.document.definition {
		v.clearFolds()
	}
	v.context = gui.App.Typography()
	state := v.operationState()
	v.document.acquire(v)
	if !state.valid(v) || !v.mounted || v.Destroyed() {
		return
	}
	attachFrame(v)
	v.updateScheme()
	v.invalidateTextLayout()
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
	v.cursorUndo = cursorUndo{}
	v.columnSelection = nil
	v.dragBase = nil
	v.additiveDrag = false
	v.cancelViewRestore()
	if v.editQueue != nil {
		v.cancelEdit()
		v.editQueue.owner = nil
		v.editQueue = nil
	}
	v.foldIntake = nil
	v.execution = nil
	v.minimap = nil
	v.pendingInput = nil
	v.ClearInlineSuggestion()
	v.mounted = false
	v.HidePanel()
	v.mountEpoch++
	v.reveal = nil
	v.cancelPaste()
	v.cancelEdit()
	v.hideMenu()
	v.closeCompletion()
	if v.completionQueue != nil {
		v.completionQueue.owner = nil
	}
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
	v.tagMatches = tagMatchState{}
	if v.blink != nil {
		v.blink.Stop()
		v.blinkHandle.Disconnect()
		v.blink = nil
	}
	detachFrame(v)
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
	return v.Focused() && (v.Surface() == nil || v.Surface().Focused() || v.menu != nil && v.menu.popup.Visible())
}
