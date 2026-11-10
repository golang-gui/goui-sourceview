// Package folding contains detached snapshots, change mapping and visible-line indexes.
package folding

import "sort"

// Interval is half-open in projected paragraph coordinates. Paragraph zero
// is always visible. It is not a replacement range in the text model.
type Interval struct{ Start, End int }

type visibleRun struct{ start, end, index int }

type VisibleLines struct {
	total, count int
	runs         []visibleRun
}

func (m *VisibleLines) Reset(total int, hidden []Interval) {
	m.total, m.count, m.runs = max(1, total), 0, m.runs[:0]
	start := 0
	appendRun := func(end int) {
		if end > start {
			m.runs = append(m.runs, visibleRun{start, end, m.count})
			m.count += end - start
		}
	}
	for _, h := range hidden { // sorted, merged by the folding state
		h.Start, h.End = max(1, h.Start), min(m.total, h.End)
		if h.End <= start || h.Start >= m.total || h.End <= h.Start {
			continue
		}
		appendRun(h.Start)
		start = max(start, h.End)
	}
	appendRun(m.total)
}

func (m *VisibleLines) Line(index int) int {
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
func (m *VisibleLines) Index(line int) (int, bool) {
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

func (m *VisibleLines) Count() int { return m.count }
func (m *VisibleLines) Total() int { return m.total }

// AppendHidden merges a range into sorted hidden intervals.
func AppendHidden(hidden []Interval, start, end int) []Interval {
	if n := len(hidden); n > 0 && start <= hidden[n-1].End {
		hidden[n-1].End = max(hidden[n-1].End, end)
		return hidden
	}
	return append(hidden, Interval{start, end})
}
