package language

import (
	"context"
	"strings"
	"unicode"
)

type texFoldBlock struct {
	row  int
	name string
}
type texFoldObserver struct {
	commandRow, environmentRow int
	stack                      []texFoldBlock
}

func (f *foldLexemes) texEnvironment(name string, ending bool, end int) {
	if f == nil || f.tex == nil || name == "" {
		return
	}
	for _, c := range name {
		if !unicode.IsLetter(c) && !unicode.IsDigit(c) && !strings.ContainsRune("*_-@.", c) {
			return
		}
	}
	o := f.tex
	if !ending {
		o.stack = append(o.stack, texFoldBlock{o.environmentRow, name})
		return
	}
	if len(o.stack) > 0 {
		top := o.stack[len(o.stack)-1]
		if top.name == name {
			o.stack = o.stack[:len(o.stack)-1]
			f.closed = append(f.closed, foldLexeme{top.row, end})
		} else {
			o.stack = nil
		}
	}
}
func texFolding(t Tokenizer) FoldingFunc {
	return func(ctx context.Context, text string) ([]FoldRange, error) {
		lex := &foldLexemes{row: -1, tex: &texFoldObserver{}}
		f := structuralFolder{}
		var headings []foldHeading
		last, depth := -1, 0
		err := syntaxLines(ctx, foldingTokenizer{t, lex}, text, func(l syntaxLine) {
			for _, c := range lex.closed {
				f.add(c.start, codeFoldEndLine(l.text, c.end, l.row))
			}
			lex.closed = lex.closed[:0]
			s := l.before.(texState)
			if s.raw == "" && s.math == "" && s.mathEnv == "" && depth == 0 {
				a, z := leadingFoldKeyword(l)
				level := 0
				switch l.text[a:z] {
				case `\part`:
					level = 1
				case `\chapter`:
					level = 2
				case `\section`:
					level = 3
				case `\subsection`:
					level = 4
				case `\subsubsection`:
					level = 5
				case `\paragraph`:
					level = 6
				case `\subparagraph`:
					level = 7
				}
				if level > 0 {
					tail := strings.TrimLeft(l.text[z:], "* \t")
					if strings.HasPrefix(tail, "{") || strings.HasPrefix(tail, "[") {
						closeFoldHeadings(&f, &headings, level, last)
						headings = append(headings, foldHeading{l.row, level, l.row})
					}
				}
			}
			for _, token := range l.tokens {
				if token.Class&ClassDelimiter != 0 {
					for at := token.Start; at < token.End; at++ {
						if l.text[at] == '{' {
							depth++
						} else if l.text[at] == '}' {
							depth = max(0, depth-1)
						}
					}
				}
			}
			if strings.TrimSpace(l.text) != "" {
				last = l.row
			}
		})
		if err != nil {
			return nil, err
		}
		closeFoldHeadings(&f, &headings, 0, last)
		return noncrossingMixedFolds(ctx, f.out)
	}
}
