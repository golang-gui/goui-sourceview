package view

import (
	"errors"
	"math"

	textcore "github.com/golang-gui/goui-sourceview/internal/text"
)

var ErrColumnWindow = errors.New("sourceview: column span exceeds native working set")

var ErrColumnDirection = errors.New("sourceview: visual column requires discontiguous bidirectional ranges")

// ColumnRange uses shaped stops, never character counts or average advances.
// A visually discontiguous logical range is rejected rather than widened into
// text outside the requested rectangle. Short lines clamp to their real edge.
func ColumnRange(g *textcore.Geometry, line int, left, right float32) (textcore.SelectionState, error) {
	a, b := g.HitLine(line, left), g.HitLine(line, right)
	s := textcore.SelectionState{Selection: textcore.Selection{Anchor: a.Offset, Caret: b.Offset}, Upstream: b.Upstream}
	if a.Offset == b.Offset || math.Abs(float64(left-right)) < .001 {
		return s, nil
	}
	rects := g.SelectionLines(s.Range(), line, line+1)
	lo, hi := min(g.Caret(a).X, g.Caret(b).X), max(g.Caret(a).X, g.Caret(b).X)
	if len(rects) != 1 || rects[0].X < lo-.1 || rects[0].X+rects[0].Width > hi+.1 {
		return textcore.SelectionState{}, ErrColumnDirection
	}
	return s, nil
}
