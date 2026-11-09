package language

import "context"

type templateFoldAction struct {
	row, at                                  int
	mode                                     byte
	head                                     string
	seen, arg, assign, invalid, raw, comment bool
}
type templateFoldBlock struct {
	row  int
	kind string
}
type templateFoldObserver struct {
	action templateFoldAction
	stack  []templateFoldBlock
}

func (f *foldLexemes) templateOpen(at int, mode byte, raw bool) {
	if f != nil && f.template != nil {
		f.template.action = templateFoldAction{row: f.row, at: at, mode: mode, raw: raw, comment: mode == '#'}
	}
}
func (f *foldLexemes) templateWord(word string, kind Kind, first bool) {
	if f == nil || f.template == nil {
		return
	}
	a := &f.template.action
	if first && !a.seen {
		a.seen = true
		a.head = word
		a.invalid = a.invalid || kind != KindKeyword
	} else {
		a.arg = true
	}
}
func (f *foldLexemes) templateSymbol(c byte, at int, trim bool) {
	if f == nil || f.template == nil {
		return
	}
	a := &f.template.action
	if trim && (a.row == f.row && at == a.at+2 || a.seen) {
		return
	}
	if !a.seen {
		a.invalid = true
	} else {
		a.arg = true
		a.assign = a.assign || c == '='
	}
}
func (f *foldLexemes) templateClose(id ID, end int) {
	if f == nil || f.template == nil {
		return
	}
	o := f.template
	a := o.action
	if a.comment {
		f.closed = append(f.closed, foldLexeme{a.row, end})
		return
	}
	if a.invalid || id == Jinja2 && (a.mode != '%' || a.raw && a.head != "endraw") {
		return
	}
	kind, close := "", ""
	if id == GoTemplate {
		switch a.head {
		case "if", "range", "with", "define", "block":
			if a.arg {
				kind = a.head
			}
		case "end":
			close = "*"
		}
	} else {
		switch a.head {
		case "if", "for", "block", "macro", "call", "filter", "autoescape":
			if a.arg {
				kind = a.head
			}
		case "with", "raw":
			kind = a.head
		case "set":
			if a.arg && !a.assign {
				kind = a.head
			}
		case "endif":
			close = "if"
		case "endfor":
			close = "for"
		case "endblock":
			close = "block"
		case "endmacro":
			close = "macro"
		case "endcall":
			close = "call"
		case "endfilter":
			close = "filter"
		case "endautoescape":
			close = "autoescape"
		case "endwith":
			close = "with"
		case "endraw":
			close = "raw"
		case "endset":
			close = "set"
		}
	}
	if kind != "" {
		o.stack = append(o.stack, templateFoldBlock{a.row, kind})
	}
	if close != "" && len(o.stack) > 0 {
		last := o.stack[len(o.stack)-1]
		if close == "*" || close == last.kind {
			o.stack = o.stack[:len(o.stack)-1]
			f.closed = append(f.closed, foldLexeme{last.row, end})
		} else {
			o.stack = nil
		}
	}
}

func templateFolding(id ID, t Tokenizer) FoldingFunc {
	return func(ctx context.Context, text string) ([]FoldRange, error) {
		lex := &foldLexemes{row: -1, template: &templateFoldObserver{}}
		f := structuralFolder{}
		err := syntaxLines(ctx, foldingTokenizer{t, lex}, text, func(l syntaxLine) {
			for _, c := range lex.closed {
				f.add(c.start, codeFoldEndLine(l.text, c.end, l.row))
			}
			lex.closed = lex.closed[:0]
		})
		if err != nil {
			return nil, err
		}
		return noncrossingMixedFolds(ctx, f.out)
	}
}
