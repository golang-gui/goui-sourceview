package folding

import (
	"sort"
	"strings"

	"github.com/golang-gui/goui-sourceview/internal/highlight"
	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/gui/textedit"
)

// Region is the value portion of a private entry. Public FoldRegion stays in
// the root package and is copied at that boundary; no view is retained here.
type Region struct {
	StartLine, EndLine int
	Collapsed          bool
}

// Entry stores normalized byte endpoints alongside logical rows. The caller
// owns mutable entries; document snapshots retain their own immutable slice.
type Entry struct {
	Region
	Start, End int
}

type Snapshot struct {
	Entries       []Entry
	Length, Lines int
}

// Hidden merges collapsed entries in header order; the caller owns the enable flag.
func Hidden(records []Entry) []Interval {
	var hidden []Interval
	for _, r := range records {
		if r.Collapsed {
			hidden = AppendHidden(hidden, r.StartLine+1, r.EndLine+1)
		}
	}
	return hidden
}

// BuildSnapshot copies and validates candidates against normalized UTF-8/LF text.
// Duplicate headers retain their outermost range; crossing ranges are invalid.
func BuildSnapshot(text string, ranges []language.FoldRange) (Snapshot, error) {
	text = textedit.Normalize(text)
	starts := []int{0}
	for at := 0; at < len(text); {
		n := strings.IndexByte(text[at:], '\n')
		if n < 0 {
			break
		}
		at += n + 1
		starts = append(starts, at)
	}
	ranges, err := Normalize(ranges, len(starts))
	if err != nil {
		return Snapshot{}, err
	}
	f := Snapshot{Length: len(text), Lines: len(starts), Entries: make([]Entry, len(ranges))}
	for i, r := range ranges {
		end := len(text)
		if r.EndLine+1 < len(starts) {
			end = starts[r.EndLine+1] - 1
		}
		f.Entries[i] = Entry{Region: Region{StartLine: r.StartLine, EndLine: r.EndLine}, Start: starts[r.StartLine], End: end}
	}
	return f, nil
}

// Reconcile preserves collapse intent only for identical byte endpoints. The
// returned slice belongs to the caller; published snapshots are never mutated.
func Reconcile(records []Entry, ranges []language.FoldRange, lineRange func(int) (textedit.Range, error)) []Entry {
	old := make(map[[2]int]bool, len(records))
	for _, r := range records {
		old[[2]int{r.Start, r.End}] = r.Collapsed
	}
	records = make([]Entry, 0, len(ranges))
	for _, r := range ranges {
		a, _ := lineRange(r.StartLine)
		b, _ := lineRange(r.EndLine)
		entry := Entry{Region: Region{StartLine: r.StartLine, EndLine: r.EndLine}, Start: a.Start, End: b.End}
		entry.Collapsed = old[[2]int{entry.Start, entry.End}]
		records = append(records, entry)
	}
	return records
}

// MapEntries maps a caller-owned mutable slice. The caller handles resets and
// revision checks; incremental means splices match this change's sequential edits.
func MapEntries(records []Entry, c textedit.Change, splices []highlight.Splice, incremental bool, lineAt func(int) int) []Entry {
	if incremental && len(c.Edits) > 1 {
		if mapped, ok := mapBatch(records, c, splices); ok {
			return mapped
		}
	}
	out := records[:0]
	for _, r := range records {
		valid := true
		for i, e := range c.Edits {
			before := e.Range.Start == r.Start && e.Range.Start == e.Range.End && strings.HasSuffix(e.Text, "\n")
			if !before && e.Range.Start <= r.End && e.Range.End >= r.Start {
				valid = false
				break
			}
			if incremental && e.Range.Start <= r.Start {
				delta := splices[i].Added - splices[i].Removed
				r.StartLine += delta
				r.EndLine += delta
			}
			r.Start = textedit.MapOffset(r.Start, e)
			r.End = textedit.MapOffset(r.End, e)
		}
		if valid {
			if !incremental {
				r.StartLine = lineAt(r.Start)
				r.EndLine = lineAt(r.End)
			}
			if r.EndLine > r.StartLine {
				out = append(out, r)
			}
		}
	}
	return out
}

// Monotone non-overlapping batch notifications can be mapped in O(E+F log E).
// Undo publishes ascending sequential coordinates; normalize those before
// searching. Ordinary arbitrary sequential changes retain the general path.
func mapBatch(records []Entry, c textedit.Change, splices []highlight.Splice) ([]Entry, bool) {
	type change struct {
		edit         textedit.Edit
		bytes, lines int
	}
	changes := make([]change, len(c.Edits))
	ascending := c.Edits[0].Range.Start < c.Edits[1].Range.Start
	delta := 0
	for i, e := range c.Edits {
		size := len(e.Text) - (e.Range.End - e.Range.Start)
		if ascending {
			e.Range.Start -= delta
			e.Range.End -= delta
			delta += size
		}
		at := i
		if !ascending {
			at = len(changes) - 1 - i
		}
		changes[at] = change{e, size, splices[i].Added - splices[i].Removed}
	}
	bytes, lines := make([]int, len(changes)+1), make([]int, len(changes)+1)
	for i, e := range changes {
		if i > 0 && (e.edit.Range.Start <= changes[i-1].edit.Range.Start || e.edit.Range.Start < changes[i-1].edit.Range.End) {
			return nil, false
		}
		bytes[i+1] = bytes[i] + e.bytes
		lines[i+1] = lines[i] + e.lines
	}
	out := records[:0]
	for _, r := range records {
		at := sort.Search(len(changes), func(i int) bool { return changes[i].edit.Range.End >= r.Start })
		next := at
		if at < len(changes) {
			e := changes[at].edit
			if e.Range.Start == r.Start && e.Range.End == r.Start && strings.HasSuffix(e.Text, "\n") {
				next++
			}
		}
		if next < len(changes) && changes[next].edit.Range.Start <= r.End {
			continue
		}
		r.Start += bytes[next]
		r.End += bytes[next]
		r.StartLine += lines[next]
		r.EndLine += lines[next]
		out = append(out, r)
	}
	return out, true
}
