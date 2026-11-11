package text

import "sort"

// MapOffsets maps many endpoints through sequential edits without an
// endpoints*edits scan. Monotone positions support lazy interval assignment and
// suffix addition; each edit takes O(log endpoints), plus sorting/materializing.
func MapOffsets(offsets []int, edits []Edit) []int {
	if len(offsets) == 0 {
		return nil
	}
	type endpoint struct{ value, index int }
	ordered := make([]endpoint, len(offsets))
	for i, v := range offsets {
		ordered[i] = endpoint{v, i}
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].value < ordered[j].value })
	t := offsetTree{max: make([]int, len(offsets)*4), add: make([]int, len(offsets)*4), set: make([]bool, len(offsets)*4)}
	var build func(int, int, int)
	build = func(n, l, r int) {
		if r-l == 1 {
			t.max[n] = ordered[l].value
			return
		}
		m := (l + r) / 2
		build(n*2, l, m)
		build(n*2+1, m, r)
		t.max[n] = t.max[n*2+1]
	}
	build(1, 0, len(offsets))
	for _, e := range edits {
		a := t.first(1, 0, len(offsets), e.Range.Start)
		b := t.first(1, 0, len(offsets), e.Range.End+1)
		t.update(1, 0, len(offsets), a, b, e.Range.Start+len(e.Text), true)
		t.update(1, 0, len(offsets), b, len(offsets), len(e.Text)-(e.Range.End-e.Range.Start), false)
	}
	out := make([]int, len(offsets))
	var collect func(int, int, int)
	collect = func(n, l, r int) {
		if r-l == 1 {
			out[ordered[l].index] = t.max[n]
			return
		}
		t.push(n)
		m := (l + r) / 2
		collect(n*2, l, m)
		collect(n*2+1, m, r)
	}
	collect(1, 0, len(offsets))
	return out
}

type offsetTree struct {
	max, add []int
	set      []bool
}

func (t *offsetTree) assign(n, value int)    { t.max[n] = value; t.add[n] = value; t.set[n] = true }
func (t *offsetTree) increment(n, value int) { t.max[n] += value; t.add[n] += value }
func (t *offsetTree) push(n int) {
	if t.set[n] {
		t.assign(n*2, t.add[n])
		t.assign(n*2+1, t.add[n])
	} else if t.add[n] != 0 {
		t.increment(n*2, t.add[n])
		t.increment(n*2+1, t.add[n])
	}
	t.add[n] = 0
	t.set[n] = false
}
func (t *offsetTree) first(n, l, r, value int) int {
	if t.max[n] < value {
		return r
	}
	if r-l == 1 {
		return l
	}
	t.push(n)
	m := (l + r) / 2
	if t.max[n*2] >= value {
		return t.first(n*2, l, m, value)
	}
	return t.first(n*2+1, m, r, value)
}
func (t *offsetTree) update(n, l, r, a, b, value int, assign bool) {
	if a >= r || b <= l || a >= b {
		return
	}
	if a <= l && r <= b {
		if assign {
			t.assign(n, value)
		} else {
			t.increment(n, value)
		}
		return
	}
	t.push(n)
	m := (l + r) / 2
	t.update(n*2, l, m, a, b, value, assign)
	t.update(n*2+1, m, r, a, b, value, assign)
	t.max[n] = t.max[n*2+1]
}
