package language

import (
	"context"
	"strings"
)

func batchFolding(t Tokenizer) FoldingFunc {
	return func(ctx context.Context, text string) ([]FoldRange, error) {
		f := structuralFolder{}
		start, last := -1, -1
		var regions []int
		err := syntaxLines(ctx, t, text, func(l syntaxLine) {
			at := len(l.text) - len(strings.TrimLeft(l.text, " \t"))
			label := false
			for _, token := range l.tokens {
				if token.Start <= at && at < token.End && token.Kind == KindTag && strings.HasPrefix(l.text[at:], ":") && strings.TrimSpace(l.text[at+1:]) != "" {
					label = true
					break
				}
			}
			if label {
				if start >= 0 {
					f.add(start, last)
				}
				start, last = l.row, l.row
			} else if start >= 0 && strings.TrimSpace(l.text) != "" {
				last = l.row
			}
			raw := strings.TrimSpace(l.text)
			rest := ""
			base := at
			if strings.HasPrefix(raw, "::") {
				rest = strings.TrimSpace(raw[2:])
				base = at + 2
			} else {
				if strings.HasPrefix(raw, "@") {
					raw = raw[1:]
					base++
				}
				if len(raw) >= 3 && strings.EqualFold(raw[:3], "REM") && (len(raw) == 3 || space(raw[3])) {
					rest = strings.TrimSpace(raw[3:])
					base += 3
				}
			}
			if rest == "" {
				return
			}
			marker := strings.ToLower(strings.Fields(rest)[0])
			if marker != "#region" && marker != "#endregion" {
				return
			}
			confirmed := false
			for _, token := range l.tokens {
				if token.End > base && token.Class&ClassComment != 0 {
					confirmed = true
					break
				}
			}
			if !confirmed {
				return
			}
			if marker == "#region" {
				regions = append(regions, l.row)
			} else if len(regions) > 0 {
				f.add(regions[len(regions)-1], l.row)
				regions = regions[:len(regions)-1]
			}
		})
		if err != nil {
			return nil, err
		}
		if start >= 0 {
			f.add(start, last)
		}
		return noncrossingMixedFolds(ctx, f.out)
	}
}
