package editor

import (
	"strings"

	textcore "github.com/golang-gui/goui-sourceview/internal/text"
	"github.com/golang-gui/goui-sourceview/language"
)

func BracketCandidates(m *textcore.Model, caret int, pairs []language.Pair) []textcore.Range {
	longest := 0
	for _, p := range pairs {
		longest = max(longest, len(p.Open), len(p.Close))
	}
	if longest == 0 {
		return nil
	}
	start, end := max(0, caret-longest), min(m.Len(), caret+longest)
	for !m.IsRuneBoundary(start) {
		start--
	}
	for !m.IsRuneBoundary(end) {
		end++
	}
	text, _ := m.Slice(textcore.Range{Start: start, End: end})
	local := caret - start
	var candidates []textcore.Range
	for _, p := range pairs {
		for _, sequence := range []string{p.Open, p.Close} {
			for at := max(0, local-len(sequence)); at <= local && at+len(sequence) <= len(text); at++ {
				if strings.HasPrefix(text[at:], sequence) {
					candidates = append(candidates, textcore.Range{Start: start + at, End: start + at + len(sequence)})
				}
			}
		}
	}
	return candidates
}

func MatchingBrackets(text string, start, caret int, allowed []bool, pairs []language.Pair) []textcore.Range {
	var ranges []textcore.Range
	type delimiter struct {
		at, end, pair int
		open          bool
	}
	var delimiters []delimiter
	for i := 0; i < len(text); {
		best := delimiter{}
		found := false
		for index, p := range pairs {
			for side, seq := range []string{p.Open, p.Close} {
				if i+len(seq) > len(text) || !strings.HasPrefix(text[i:], seq) {
					continue
				}
				valid := true
				for j := i; j < i+len(seq); j++ {
					if !allowed[j] {
						valid = false
						break
					}
				}
				if valid && (!found || len(seq) > best.end-best.at) {
					best = delimiter{i, i + len(seq), index, side == 0}
					found = true
				}
			}
		}
		if found {
			delimiters = append(delimiters, best)
			i = best.end
		} else {
			i++
		}
	}
	at := -1
	local := caret - start
	for i, d := range delimiters {
		if d.at <= local && local < d.end {
			at = i
			break
		}
	}
	if at < 0 {
		for i, d := range delimiters {
			if d.end == local {
				at = i
				break
			}
		}
	}
	if at < 0 {
		return nil
	}
	current := delimiters[at]
	step := 1
	if !current.open {
		step = -1
	}
	stack := []int{current.pair}
	ranges = []textcore.Range{{Start: start + current.at, End: start + current.end}}
	for i := at + step; i >= 0 && i < len(delimiters); i += step {
		d := delimiters[i]
		if d.open == current.open {
			stack = append(stack, d.pair)
			continue
		}
		if stack[len(stack)-1] != d.pair {
			break
		}
		stack = stack[:len(stack)-1]
		if len(stack) == 0 {
			ranges = append(ranges, textcore.Range{Start: start + d.at, End: start + d.end})
			break
		}
	}
	return ranges
}
