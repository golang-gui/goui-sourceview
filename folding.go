package sourceview

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/core/signal"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
)

// FoldRegion is a detached snapshot. Lines are zero-based and EndLine inclusive.
type FoldRegion struct {
	StartLine, EndLine int
	Collapsed          bool
}
type foldEntry struct {
	FoldRegion
	start, end int
}

func normalizeFolds(ranges []language.FoldRange, lines int) ([]language.FoldRange, error) {
	ranges = slices.Clone(ranges)
	for _, r := range ranges {
		if r.StartLine < 0 || r.EndLine <= r.StartLine || r.EndLine >= lines {
			return nil, fmt.Errorf("sourceview: invalid fold range %v", r)
		}
	}
	if len(ranges) > lines {
		return nil, fmt.Errorf("sourceview: more fold headers than lines")
	}
	slices.SortFunc(ranges, func(a, b language.FoldRange) int {
		if a.StartLine != b.StartLine {
			return a.StartLine - b.StartLine
		}
		return b.EndLine - a.EndLine
	})
	var out []language.FoldRange
	var stack []int
	for _, r := range ranges {
		if r.StartLine < 0 || r.EndLine <= r.StartLine || r.EndLine >= lines {
			return nil, fmt.Errorf("sourceview: invalid fold range %v", r)
		}
		if len(out) > 0 && out[len(out)-1].StartLine == r.StartLine {
			continue
		}
		for len(stack) > 0 && r.StartLine > stack[len(stack)-1] {
			stack = stack[:len(stack)-1]
		}
		if len(stack) > 0 && r.EndLine > stack[len(stack)-1] {
			return nil, fmt.Errorf("sourceview: crossing fold range %v", r)
		}
		stack = append(stack, r.EndLine)
		out = append(out, r)
	}
	return out, nil
}

func (v *SourceView) FoldingEnabled() bool { return v.foldingEnabled }
func (v *SourceView) SetFoldingEnabled(enabled bool) {
	if v.Destroyed() || v.foldingEnabled == enabled {
		return
	}
	v.foldingEnabled = enabled
	if !enabled {
		for i := range v.folds {
			v.folds[i].Collapsed = false
		}
	}
	v.refreshFoldGeometry()
	if enabled {
		v.document.requestFolding(true)
	}
	v.foldingSignal.Emit()
}
func (v *SourceView) FoldingRegions() []FoldRegion {
	if v.foldRevision != v.document.model.Revision() || v.foldDefinition != v.document.definition {
		return nil
	}
	out := make([]FoldRegion, len(v.folds))
	for i, r := range v.folds {
		out[i] = r.FoldRegion
	}
	return out
}
func (v *SourceView) ConnectFoldingChanged(fn func()) signal.Handle {
	return v.foldingSignal.Connect(fn)
}
func (v *SourceView) Fold(line int) bool   { return v.setFolded(line, true) }
func (v *SourceView) Unfold(line int) bool { return v.setFolded(line, false) }
func (v *SourceView) ToggleFold(line int) bool {
	i := v.foldHeader(line)
	if i < 0 {
		return false
	}
	return v.setFolded(line, !v.folds[i].Collapsed)
}
func (v *SourceView) FoldAll()   { v.setAllFolded(true) }
func (v *SourceView) UnfoldAll() { v.setAllFolded(false) }
func (v *SourceView) foldHeader(line int) int {
	i := sort.Search(len(v.folds), func(i int) bool { return v.folds[i].StartLine >= line })
	if i < len(v.folds) && v.folds[i].StartLine == line {
		return i
	}
	return -1
}
func (v *SourceView) canFold() bool {
	return !v.Destroyed() && v.foldingEnabled && gui.IsEnabled(v) && v.projection == nil && v.foldRevision == v.document.model.Revision() && v.foldDefinition == v.document.definition
}
func (v *SourceView) setFolded(line int, collapsed bool) bool {
	i := v.foldHeader(line)
	if !v.canFold() || i < 0 || v.folds[i].Collapsed == collapsed {
		return false
	}
	v.folds[i].Collapsed = collapsed
	v.finishFoldCommand()
	return true
}
func (v *SourceView) setAllFolded(collapsed bool) {
	if !v.canFold() {
		return
	}
	changed := false
	for i := range v.folds {
		if v.folds[i].Collapsed != collapsed {
			v.folds[i].Collapsed = collapsed
			changed = true
		}
	}
	if changed {
		v.finishFoldCommand()
	}
}
func (v *SourceView) finishFoldCommand() {
	s := v.Selection()
	for _, r := range v.folds {
		if !r.Collapsed {
			continue
		}
		a, b := v.document.model.LineAt(s.Anchor), v.document.model.LineAt(s.Caret)
		if a > r.StartLine && a <= r.EndLine || b > r.StartLine && b <= r.EndLine {
			header, _ := v.document.model.LineRange(r.StartLine)
			s = textedit.Selection{Anchor: header.End, Caret: header.End}
			break
		}
	}
	state := v.operationState()
	v.closeCompletion()
	if !state.valid(v) || v.Destroyed() {
		return
	}
	v.refreshFoldGeometry()
	if s != v.Selection() {
		v.setSelection(s, false, true)
	}
	if !v.Destroyed() && v.document == state.document && v.mountEpoch == state.mount {
		v.foldingSignal.Emit()
	}
}

func (v *SourceView) foldedLines() []lineInterval {
	var hidden []lineInterval
	if v.foldingEnabled {
		for _, r := range v.folds {
			if !r.Collapsed {
				continue
			}
			h := lineInterval{r.StartLine + 1, r.EndLine + 1}
			if n := len(hidden); n > 0 && h.start <= hidden[n-1].end {
				hidden[n-1].end = max(hidden[n-1].end, h.end)
			} else {
				hidden = append(hidden, h)
			}
		}
	}
	return hidden
}
func (v *SourceView) refreshFoldGeometry() {
	v.hiddenLines = v.foldedLines()
	if v.anchor.valid {
		row := v.document.model.LineAt(v.anchor.offset)
		for _, r := range v.folds {
			if r.Collapsed && row > r.StartLine && row <= r.EndLine {
				h, _ := v.document.model.LineRange(r.StartLine)
				v.anchor.offset = h.Start
				break
			}
		}
		v.pendingAnchor = true
	}
	v.layoutEpoch++
	v.resetHeights()
	v.contentWidth = 0
	v.RequestLayout()
	v.RequestPaint()
}

func (v *SourceView) acceptFolds(ranges []language.FoldRange) {
	if v.projection != nil {
		return
	}
	old := make(map[[2]int]bool, len(v.folds))
	for _, r := range v.folds {
		old[[2]int{r.start, r.end}] = r.Collapsed
	}
	v.folds = make([]foldEntry, 0, len(ranges))
	for _, r := range ranges {
		a, _ := v.document.model.LineRange(r.StartLine)
		b, _ := v.document.model.LineRange(r.EndLine)
		entry := foldEntry{FoldRegion: FoldRegion{StartLine: r.StartLine, EndLine: r.EndLine}, start: a.Start, end: b.End}
		entry.Collapsed = old[[2]int{entry.start, entry.end}]
		v.folds = append(v.folds, entry)
	}
	v.foldRevision = v.document.model.Revision()
	v.foldDefinition = v.document.definition
	// Candidate analysis changes controls, not paragraph visibility, when no
	// collapsed extent changed. Preserve measured rows and completed reveals.
	if slices.Equal(v.hiddenLines, v.foldedLines()) && v.visibleLines.total == v.displayCount() && v.heights.Count() == v.visibleLines.count {
		v.RequestPaint()
	} else {
		v.refreshFoldGeometry()
	}
	v.foldingSignal.Emit()
}
func (v *SourceView) clearFolds() {
	v.folds = nil
	v.foldRevision = v.document.model.Revision()
	v.foldDefinition = v.document.definition
	v.refreshFoldGeometry()
}
func (v *SourceView) mapFolds(c textedit.Change) {
	if c.Reset {
		v.clearFolds()
		return
	}
	if len(v.folds) == 0 {
		v.foldRevision = c.Revision
		return
	}
	if v.foldRevision >= c.Revision {
		return
	}
	out := v.folds[:0]
	for _, r := range v.folds {
		valid := true
		for _, e := range c.Edits {
			before := e.Range.Start == r.start && e.Range.Start == e.Range.End && strings.HasSuffix(e.Text, "\n")
			if !before && e.Range.Start <= r.end && e.Range.End >= r.start {
				valid = false
				break
			}
			r.start = textedit.MapOffset(r.start, e)
			r.end = textedit.MapOffset(r.end, e)
		}
		if valid {
			r.StartLine = v.document.model.LineAt(r.start)
			r.EndLine = v.document.model.LineAt(r.end)
			if r.EndLine > r.StartLine {
				out = append(out, r)
			}
		}
	}
	v.folds = out
	v.foldRevision = c.Revision
	v.refreshFoldGeometry()
}
func (v *SourceView) expandFoldRange(r textedit.Range) bool {
	changed := false
	for i := range v.folds {
		f := &v.folds[i]
		if !f.Collapsed {
			continue
		}
		start, _ := v.document.model.LineRange(f.StartLine + 1)
		if r.Start <= f.end && r.End >= start.Start {
			f.Collapsed = false
			changed = true
		}
	}
	if changed {
		v.refreshFoldGeometry()
	}
	return changed
}
func (v *SourceView) unfoldSelection(s textedit.Selection) bool {
	a := v.expandFoldRange(textedit.Range{Start: s.Anchor, End: s.Anchor})
	b := v.expandFoldRange(textedit.Range{Start: s.Caret, End: s.Caret})
	return a || b
}

func (v *SourceView) foldNavigationTarget(at int, forward bool) int {
	for _, f := range v.folds {
		if !f.Collapsed {
			continue
		}
		row := v.document.model.LineAt(at)
		if row <= f.StartLine || row > f.EndLine {
			continue
		}
		if forward {
			if f.EndLine+1 < v.document.model.LineCount() {
				r, _ := v.document.model.LineRange(f.EndLine + 1)
				return r.Start
			}
			r, _ := v.document.model.LineRange(f.StartLine)
			return r.End
		}
		r, _ := v.document.model.LineRange(f.StartLine)
		return r.End
	}
	return at
}
func (v *SourceView) unfoldDeleteBoundary(forward bool) bool {
	s := v.Selection()
	if s.Anchor != s.Caret {
		return false
	}
	at := v.adjacent(s.Caret, forward)
	a, b := min(at, s.Caret), max(at, s.Caret)
	if v.expandFoldRange(textedit.Range{Start: a, End: b}) {
		v.foldingSignal.Emit()
		return true
	}
	return false
}

func (v *SourceView) resumeFolding() {
	d := v.document
	if d.foldReady && d.foldRevision == d.model.Revision() && (d.folding == nil || !d.folding.busy) {
		v.acceptFolds(d.foldRanges)
	}
}
