package language

import (
	"context"
	"strings"
)

// Request bodies are opaque. The HTTP lexer owns separator recognition and
// method spelling; folding owns only physical request regions, never execution.
func requestFolding(t Tokenizer) FoldingFunc {
	return func(ctx context.Context, text string) ([]FoldRange, error) {
		f := structuralFolder{}
		start, last, request := -1, -1, false
		flush := func() {
			if request {
				f.add(start, last)
			}
		}
		err := syntaxLines(ctx, t, text, func(l syntaxLine) {
			at := len(l.text) - len(strings.TrimLeft(l.text, " \t"))
			separator := false
			if strings.HasPrefix(l.text[at:], "###") {
				for _, token := range l.tokens {
					if token.Start <= at && at < token.End && token.Class&ClassComment != 0 {
						separator = true
						break
					}
				}
			}
			if separator {
				flush()
				start, last, request = l.row, l.row, false
				return
			}
			if !request {
				a, z := leadingFoldKeyword(l)
				if z > a && httpMethods[l.text[a:z]] && strings.TrimSpace(l.text[z:]) != "" || bareRequestURL(l, at) {
					if start < 0 {
						start = l.row
					}
					request = true
				}
			}
			if start >= 0 && strings.TrimSpace(l.text) != "" {
				last = l.row
			}
		})
		if err != nil {
			return nil, err
		}
		flush()
		return f.out, nil
	}
}

// REST files also allow an omitted GET. Recognize an explicit HTTP(S) URL,
// without treating quoted text, variables or arbitrary body words as requests.
func bareRequestURL(l syntaxLine, at int) bool {
	end := len(l.text)
	if n := strings.IndexAny(l.text[at:], " \t\r"); n >= 0 {
		end = at + n
	}
	word := l.text[at:end]
	scheme, target, ok := strings.Cut(word, "://")
	if !ok || target == "" || !strings.EqualFold(scheme, "http") && !strings.EqualFold(scheme, "https") {
		return false
	}
	return len(l.tokens) > 0 && l.tokens[0].Start == at && l.tokens[0].Kind == KindString && l.tokens[0].Class == 0
}
