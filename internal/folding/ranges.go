package folding

import (
	"fmt"
	"github.com/golang-gui/goui-sourceview/language"
	"slices"
)

func Normalize(ranges []language.FoldRange, lines int) ([]language.FoldRange, error) {
	ranges = slices.Clone(ranges)
	for _, r := range ranges {
		if r.StartLine < 0 || r.EndLine <= r.StartLine || r.EndLine >= lines {
			return nil, fmt.Errorf("sourceview: invalid fold range %v", r)
		}
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
