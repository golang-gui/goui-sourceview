package language

import (
	"context"
	"strings"
)

// These formats have explicit records or literal continuations, not bracket
// structure. Observe only real closures; incomplete input yields no candidate.
func recordLiteralFolding(id ID, t Tokenizer) FoldingFunc {
	return func(ctx context.Context, text string) ([]FoldRange, error) {
		lex := &foldLexemes{row: -1}
		f := structuralFolder{}
		err := syntaxLines(ctx, foldingTokenizer{t, lex}, text, func(l syntaxLine) {
			for _, c := range lex.closed {
				end := codeFoldEndLine(l.text, c.end, l.row)
				if id == CSV || id == BibTeX {
					end = foldEndLine(l.text, c.end, l.row, false)
				}
				f.add(c.start, end)
			}
			lex.closed = lex.closed[:0]
		})
		if err != nil {
			return nil, err
		}
		return noncrossingMixedFolds(ctx, f.out)
	}
}

// Associate leading comments/context with a msgid and its following strings.
// Blank lines and actual new record keywords delimit records, never quoted
// appearances of those words. Plural forms remain inside their parent record.
func poFolding(t Tokenizer) FoldingFunc {
	return func(ctx context.Context, text string) ([]FoldRange, error) {
		f := structuralFolder{}
		start, last, comments := -1, -1, -1
		id, ctxt := false, false
		flush := func(end int) {
			if id {
				f.add(start, end)
			}
			start, last, comments, id, ctxt = -1, -1, -1, false, false
		}
		err := syntaxLines(ctx, t, text, func(l syntaxLine) {
			if strings.TrimSpace(l.text) == "" {
				flush(last)
				return
			}
			comment := len(l.tokens) > 0 && l.tokens[0].Class&ClassComment != 0
			if comment {
				if start < 0 {
					start = l.row
				}
				if id && comments < 0 {
					comments = l.row
				}
				last = l.row
				return
			}
			a, z := leadingFoldKeyword(l)
			word := l.text[a:z]
			if (word == "msgctxt" && (id || ctxt)) || (word == "msgid" && id) {
				next, end := l.row, last
				if comments >= 0 {
					next, end = comments, comments-1
				}
				flush(end)
				start = next
			}
			if start < 0 {
				start = l.row
			}
			id, ctxt = id || word == "msgid", ctxt || word == "msgctxt"
			comments, last = -1, l.row
		})
		if err != nil {
			return nil, err
		}
		flush(last)
		return f.out, nil
	}
}
