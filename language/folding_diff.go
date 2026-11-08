package language

import (
	"context"
	"strings"
)

// Only the existing lexer confirms unified hunk headers and distinguishes file
// headings from removed/added body lines. No source-language or patch parser.
type diffFoldObserver struct{ hunk bool }

func diffFolding(t Tokenizer) FoldingFunc {
	return func(ctx context.Context, text string) ([]FoldRange, error) {
		lex := &foldLexemes{row: -1, diff: &diffFoldObserver{}}
		f := structuralFolder{}
		file, hunk, last, oldHeader := -1, -1, -1, -1
		gitFile, headings, opaque := false, false, false
		closeHunk := func(end int) {
			if hunk >= 0 {
				f.add(hunk, min(last, end))
			}
			hunk = -1
		}
		closeFile := func(end int) {
			closeHunk(end)
			if file >= 0 {
				f.add(file, min(last, end))
			}
			file = -1
		}
		err := syntaxLines(ctx, foldingTokenizer{t, lex}, text, func(l syntaxLine) {
			kind := KindPlain
			if len(l.tokens) > 0 && l.tokens[0].Start == 0 {
				kind = l.tokens[0].Kind
			}
			if kind == KindMeta && strings.HasPrefix(l.text, "diff ") {
				closeFile(l.row - 1)
				gitFile = strings.HasPrefix(l.text, "diff --git ") && strings.TrimSpace(l.text[len("diff --git "):]) != ""
				headings, opaque, oldHeader = false, !gitFile, -1
				if gitFile {
					file = l.row
				}
			} else if kind == KindMeta && (strings.HasPrefix(l.text, "GIT binary patch") || strings.HasPrefix(l.text, "Binary files ")) {
				closeHunk(l.row - 1)
				opaque = true
			}
			if !opaque {
				if kind == KindHeading && strings.HasPrefix(l.text, "+++ ") && oldHeader >= 0 && oldHeader == l.row-1 {
					if !gitFile || headings {
						closeFile(oldHeader - 1)
						file, gitFile = oldHeader, false
					}
					headings = true
				}
				oldHeader = -1
				if kind == KindHeading && strings.HasPrefix(l.text, "--- ") {
					closeHunk(l.row - 1)
					oldHeader = l.row
				}
				if lex.diff.hunk {
					closeHunk(l.row - 1)
					hunk = l.row
				}
			}
			lex.diff.hunk = false
			// A single space is a meaningful empty context line in a patch.
			if l.text != "" {
				last = l.row
			}
		})
		if err != nil {
			return nil, err
		}
		closeFile(last)
		return noncrossingMixedFolds(ctx, f.out)
	}
}
