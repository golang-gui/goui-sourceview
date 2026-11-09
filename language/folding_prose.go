package language

import (
	"context"
	"strings"
	"unicode/utf8"
)

type foldHeading struct{ row, level, headerEnd int }

func closeFoldHeadings(f *structuralFolder, stack *[]foldHeading, level, end int) {
	for len(*stack) > 0 && (*stack)[len(*stack)-1].level >= level {
		h := (*stack)[len(*stack)-1]
		*stack = (*stack)[:len(*stack)-1]
		if end > h.headerEnd {
			f.add(h.row, end)
		}
	}
}

type rstFoldLine struct {
	row, lastBefore int
	text            string
	marker          byte
	plain           bool
	used            bool // A closing adornment is not a new opening adornment.
}

func proseFolding(id ID, t Tokenizer) FoldingFunc {
	return func(ctx context.Context, text string) ([]FoldRange, error) {
		f := structuralFolder{}
		var headings []foldHeading
		last, block, pending, blockLast := -1, -1, -1, -1
		prev, prev2 := rstFoldLine{row: -1, lastBefore: -1}, rstFoldLine{row: -2, lastBefore: -1}
		styles := map[string]int{}
		err := syntaxLines(ctx, t, text, func(l syntaxLine) {
			nonempty := strings.TrimSpace(l.text) != ""
			if id == AsciiDoc {
				before, after := l.before.(adocState), l.after.(adocState)
				if before.block == 0 && after.block != 0 {
					block = l.row
				}
				if before.block != 0 && after.block == 0 && block >= 0 {
					f.add(block, l.row)
					block = -1
				}
				if before.block == 0 && after.block == 0 && len(l.tokens) > 0 && l.tokens[0].Start == 0 && l.tokens[0].Kind == KindHeading {
					n := 0
					for n < len(l.text) && l.text[n] == '=' {
						n++
					}
					if n > 0 && n <= 6 && n < len(l.text) && space(l.text[n]) && strings.TrimSpace(l.text[n:]) != "" {
						closeFoldHeadings(&f, &headings, n, last)
						headings = append(headings, foldHeading{l.row, n, l.row})
					}
				}
			} else {
				before, after := l.before.(rstState), l.after.(rstState)
				if block >= 0 && before.block != KindPlain && after.block == KindPlain {
					f.add(block, blockLast)
					block = -1
				}
				if after.pending != KindPlain && nonempty {
					pending = l.row
				}
				if after.block != KindPlain && block < 0 {
					block = pending
					if block < 0 {
						block = l.row
					}
				}
				if after.block != KindPlain && nonempty {
					blockLast = l.row
				}
				m := rstFoldLine{row: l.row, lastBefore: last, text: l.text, plain: nonempty && after.block == KindPlain && after.pending == KindPlain && len(l.text) == len(strings.TrimLeft(l.text, " \t"))}
				for _, token := range l.tokens {
					if token.Class&ClassComment != 0 || token.Kind == KindMeta || token.Kind == KindHeading {
						m.plain = false
					}
				}
				raw := strings.TrimRight(l.text, " \t\r")
				if after.block == KindPlain && len(raw) >= 3 && strings.ContainsRune("=-~^\"`:+#*.", rune(raw[0])) && strings.Trim(raw, string(raw[0])) == "" {
					for _, token := range l.tokens {
						if token.Start == 0 && token.Kind == KindHeading {
							m.marker = raw[0]
						}
					}
				}
				if m.marker != 0 && prev.plain && !(prev2.used && prev2.marker == m.marker) && len(raw) >= utf8.RuneCountInString(strings.TrimSpace(prev.text)) {
					over := prev2.marker == m.marker && !prev2.used
					key := string(m.marker)
					end := prev.lastBefore
					if over {
						key += "over"
						end = prev2.lastBefore
					}
					level, ok := styles[key]
					if !ok {
						level = len(styles) + 1
						styles[key] = level
					}
					closeFoldHeadings(&f, &headings, level, end)
					headings = append(headings, foldHeading{prev.row, level, l.row})
					m.used = true
				}
				prev2, prev = prev, m
			}
			if nonempty {
				last = l.row
			}
		})
		if err != nil {
			return nil, err
		}
		if id == ReStructuredText && block >= 0 {
			f.add(block, blockLast)
		}
		closeFoldHeadings(&f, &headings, 0, last)
		return noncrossingMixedFolds(ctx, f.out)
	}
}
