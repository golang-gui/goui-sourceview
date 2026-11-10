package editor

import (
	"strings"

	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/gui/textedit"
)

// TokenReader borrows analyzed tokens. The caller owns analysis scheduling.
type TokenReader func(int) ([]language.Token, uint64, bool)

func StructureCandidates(m *textedit.Model, r textedit.Range, rules language.EditingRules, tokens func() TokenReader, lexical func(int) (language.Context, bool), add func(textedit.Range)) {
	start, end := max(0, r.Start-64*1024), min(m.Len(), r.End+64*1024)
	if end-start > 128*1024 {
		return
	}
	for !m.IsRuneBoundary(start) {
		start++
	}
	for !m.IsRuneBoundary(end) {
		end--
	}
	text, _ := m.Slice(textedit.Range{Start: start, End: end})
	// Markup parsing needs a known region boundary, not an arbitrary text suffix.
	if rules.Tags != nil && start == 0 && end == m.Len() {
		for _, q := range rules.Tags.SelectionRanges(text, language.NameRange{Start: r.Start, End: r.End}) {
			add(textedit.Range{Start: q.Start, End: q.End})
		}
	}
	allowed := make([]bool, len(text))
	known := make([]bool, len(text))
	read := tokens()
	if read == nil {
		return
	}
	for row, last := m.LineAt(start), m.LineAt(end); row <= last; row++ {
		ts, _, ok := read(row)
		if !ok {
			continue
		}
		line, _ := m.LineRange(row)
		for at := max(start, line.Start); at < min(end, line.End+1); at++ {
			known[at-start] = true
		}
		stringToken := func(t language.Token) bool {
			if t.Class&language.ClassString != 0 {
				return true
			}
			at := line.Start + t.Start
			if at < start || at >= end {
				return false
			}
			// Quoted keys may carry a property style without ClassString. Confirm
			// their lexical context instead of inferring behavior from a style key.
			if strings.ContainsRune("\"'`", rune(text[at-start])) {
				at++
			}
			c, known := lexical(at)
			return known && c.String
		}
		for i := 0; i < len(ts); i++ {
			t := ts[i]
			a, b := line.Start+t.Start, line.Start+t.End
			if t.Class&language.ClassDelimiter != 0 {
				for at := max(start, a); at < min(end, b); at++ {
					allowed[at-start] = true
				}
			}
			if !stringToken(t) || a < start {
				continue
			}
			for i+1 < len(ts) && ts[i+1].Start == ts[i].End && stringToken(ts[i+1]) {
				i++
				b = line.Start + ts[i].End
			}
			if b > end || b-a < 2 {
				continue
			}
			if ctx, known := lexical(b); !known || ctx.String {
				continue
			}
			literal := text[a-start : b-start]
			for _, p := range rules.SurroundingPairs {
				if p.Open != p.Close || !strings.HasPrefix(literal, p.Open) || !strings.HasSuffix(literal, p.Close) || len(literal) < len(p.Open)+len(p.Close) {
					continue
				}
				add(textedit.Range{Start: a + len(p.Open), End: b - len(p.Close)})
				add(textedit.Range{Start: a, End: b})
			}
		}
	}
	type opener struct{ at, end, pair int }
	var stack []opener
	for at := 0; at < len(text); {
		if !known[at] {
			stack = nil
			at++
			continue
		}
		if !allowed[at] {
			at++
			continue
		}
		pair, side, size := -1, false, 0
		for i, p := range rules.Brackets {
			for j, seq := range []string{p.Open, p.Close} {
				if len(seq) <= size || !strings.HasPrefix(text[at:], seq) {
					continue
				}
				valid := true
				for k := at; k < at+len(seq); k++ {
					valid = valid && allowed[k]
				}
				if valid {
					pair, side, size = i, j == 0, len(seq)
				}
			}
		}
		if pair < 0 {
			at++
			continue
		}
		if side {
			stack = append(stack, opener{at, at + size, pair})
		} else if len(stack) > 0 {
			o := stack[len(stack)-1]
			if o.pair != pair {
				stack = nil
			} else {
				stack = stack[:len(stack)-1]
				add(textedit.Range{Start: start + o.end, End: start + at})
				add(textedit.Range{Start: start + o.at, End: start + at + size})
			}
		}
		at += size
	}
}
