package editor

import textcore "github.com/golang-gui/goui-sourceview/internal/text"

// Mapping uses change coordinates, never the possibly newer model contents.
// Insertions at an anchor use right gravity; deletion covering it invalidates it.
func MapLineAnchor(offset int, valid bool, c textcore.Change) (int, bool) {
	if c.Reset {
		return offset, false
	}
	for _, edit := range c.Edits {
		if edit.Range.Start <= offset && offset < edit.Range.End {
			valid = false
		}
		offset = textcore.MapOffset(offset, edit)
	}
	return offset, valid
}

func TouchesPair(r textcore.Range, at, size int) bool {
	if r.Start == r.End {
		return r.Start > at && r.Start < at+size
	}
	return r.Start < at+size && r.End > at
}
