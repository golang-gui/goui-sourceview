package sourceview

import "sort"

// lineInterval is half-open in projected paragraph coordinates. Paragraph zero
// is always visible. It is not a replacement range in the text model.
type lineInterval struct{ start, end int }
type visibleRun struct{ start, end, index int }
type visibleLineMap struct {
	total, count int
	runs         []visibleRun
}

func (m *visibleLineMap) reset(total int, hidden []lineInterval) {
	m.total, m.count, m.runs = max(1, total), 0, m.runs[:0]
	start := 0
	appendRun := func(end int) {
		if end > start {
			m.runs = append(m.runs, visibleRun{start, end, m.count})
			m.count += end - start
		}
	}
	for _, h := range hidden { // sorted, merged by the folding state
		h.start, h.end = max(1, h.start), min(m.total, h.end)
		if h.end <= start || h.start >= m.total || h.end <= h.start {
			continue
		}
		appendRun(h.start)
		start = max(start, h.end)
	}
	appendRun(m.total)
}

func (m *visibleLineMap) line(index int) int {
	if index >= m.count {
		return m.total
	}
	index = max(0, index)
	i := sort.Search(len(m.runs), func(i int) bool { r := m.runs[i]; return r.index+r.end-r.start > index })
	if i == len(m.runs) {
		return 0
	}
	r := m.runs[i]
	return r.start + index - r.index
}

// index maps a hidden line to its preceding visible paragraph and reports false.
func (m *visibleLineMap) index(line int) (int, bool) {
	if line >= m.total {
		return m.count, true
	}
	line = max(0, line)
	i := sort.Search(len(m.runs), func(i int) bool { return m.runs[i].end > line })
	if i == len(m.runs) {
		return max(0, m.count-1), false
	}
	r := m.runs[i]
	if line < r.start {
		return max(0, r.index-1), false
	}
	return r.index + line - r.start, true
}

func (v *SourceView) resetHeights() {
	hidden := v.hiddenLines
	if v.projection != nil {
		hidden = nil
		first, last := v.projection.FirstLine(), v.projection.LastLine()
		delta := v.projection.ParagraphCount() - (last - first + 1)
		for _, h := range v.hiddenLines {
			if h.end <= first {
				hidden = append(hidden, h)
			} else if h.start > last {
				hidden = append(hidden, lineInterval{h.start + delta, h.end + delta})
			}
		}
	}
	v.visibleLines.reset(v.displayCount(), hidden)
	v.heights.Reset(v.visibleLines.count, v.lineHeight)
}
func (v *SourceView) paragraphTop(row int) float32 {
	v.ensureHeights()
	i, _ := v.visibleLines.index(row)
	return v.heights.Top(i)
}
func (v *SourceView) paragraphAt(y float32) int {
	v.ensureHeights()
	return v.visibleLines.line(v.heights.At(y))
}
func (v *SourceView) setParagraphHeight(row int, height float32) {
	i, visible := v.visibleLines.index(row)
	if visible {
		v.heights.Set(i, height)
	}
}
func (v *SourceView) nextDisplayRow(row int) int {
	i, _ := v.visibleLines.index(row)
	return v.visibleLines.line(i + 1)
}
func (v *SourceView) neighboringRow(row, delta int) int {
	v.ensureHeights()
	i, _ := v.visibleLines.index(row)
	if i+delta < 0 {
		return -1
	}
	return v.visibleLines.line(i + delta)
}
