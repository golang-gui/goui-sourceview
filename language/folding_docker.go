package language

import (
	"context"
	"strings"
)

// Docker stages are physical text regions. Instructions use the lexer's actual
// continuation/heredoc lifetime; shell bodies and image dependencies are opaque.
func dockerFolding(t Tokenizer) FoldingFunc {
	return func(ctx context.Context, text string) ([]FoldRange, error) {
		f := structuralFolder{}
		stage, instruction, last := -1, -1, -1
		err := syntaxLines(ctx, t, text, func(l syntaxLine) {
			before, after := l.before.(configState), l.after.(configState)
			if !before.continued && before.dockerShell == nil && !before.variable && before.mode == configCode {
				a, z := leadingFoldKeyword(l)
				if z > a && dockerKeywords[strings.ToUpper(l.text[a:z])] {
					instruction = l.row
					if strings.EqualFold(l.text[a:z], "FROM") && strings.TrimSpace(l.text[z:]) != "" {
						if stage >= 0 {
							f.add(stage, last)
						}
						stage = l.row
					}
				}
			}
			if strings.TrimSpace(l.text) != "" {
				last = l.row
			}
			if instruction >= 0 && !after.continued && after.dockerShell == nil {
				f.add(instruction, l.row)
				instruction = -1
			}
		})
		if err != nil {
			return nil, err
		}
		if stage >= 0 {
			f.add(stage, last)
		}
		return noncrossingMixedFolds(ctx, f.out)
	}
}
