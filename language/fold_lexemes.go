package language

// foldLexemes observes confirmed lexical boundaries during one folding scan.
// It belongs to that worker, never to a registered tokenizer or saved State.
// Stacks retain outer literals across interpolation and nested block comments.
type foldLexemes struct {
	row                int
	literals, comments []int
	closed             []foldLexeme
	mixed              *mixedFoldObserver
	heres              map[any]foldHere
	keywords           *keywordFoldObserver
	hcl                *hclFoldObserver
	lineLiteral        int // Zig's confirmed line-string token, or -1.
}

type foldLexeme struct{ start, end int }
type foldHere struct{ row, literals, comments int }

func (f *foldLexemes) openLiteral() {
	if f != nil {
		f.literals = append(f.literals, f.row)
		if f.hcl != nil {
			f.hcl.templates = append(f.hcl.templates, nil)
		}
	}
}

func (f *foldLexemes) closeLiteral(end int, complete bool) {
	if f != nil {
		f.close(&f.literals, end, complete)
		if f.hcl != nil && len(f.hcl.templates) > 0 {
			f.hcl.templates = f.hcl.templates[:len(f.hcl.templates)-1]
		}
	}
}

func (f *foldLexemes) openComment() {
	if f != nil {
		f.comments = append(f.comments, f.row)
	}
}

func (f *foldLexemes) closeComment(end int) {
	if f != nil {
		f.close(&f.comments, end, true)
	}
}

func (f *foldLexemes) openHere(key any) {
	if f != nil {
		if f.heres == nil {
			f.heres = make(map[any]foldHere)
		}
		f.heres[key] = foldHere{row: f.row, literals: -1, comments: -1}
	}
}
func (f *foldLexemes) activateHere(key any) {
	if f != nil {
		if h, ok := f.heres[key]; ok {
			h.literals, h.comments = len(f.literals), len(f.comments)
			f.heres[key] = h
			if f.keywords != nil {
				f.keywords.heres = append(f.keywords.heres, keywordFoldHere{key: key, enter: true})
			}
		}
	}
}
func (f *foldLexemes) closeHere(key any, end int) {
	if f != nil {
		if h, ok := f.heres[key]; ok {
			delete(f.heres, key)
			if h.literals >= 0 && f.keywords != nil {
				f.keywords.heres = append(f.keywords.heres, keywordFoldHere{key: key})
			}
			// Ruby restores its suspended state at a terminator even when an
			// interpolation was incomplete. Drop its abandoned observations too.
			if h.literals >= 0 && len(f.literals) > h.literals {
				f.literals = f.literals[:h.literals]
			}
			if h.comments >= 0 && len(f.comments) > h.comments {
				f.comments = f.comments[:h.comments]
			}
			if f.row > h.row {
				f.closed = append(f.closed, foldLexeme{h.row, end})
			}
		}
	}
}

func (f *foldLexemes) close(stack *[]int, end int, complete bool) {
	if n := len(*stack); n > 0 {
		start := (*stack)[n-1]
		*stack = (*stack)[:n-1]
		if complete && start < f.row {
			f.closed = append(f.closed, foldLexeme{start, end})
		}
	}
}

type foldingTokenizer struct {
	Tokenizer
	lexemes *foldLexemes
}

func (t foldingTokenizer) Scan(in Input, state State) (Result, error) {
	if in.Offset == 0 {
		t.lexemes.row++
		t.lexemes.lineLiteral = -1
	}
	switch scanner := t.Tokenizer.(type) {
	case cTokenizer:
		return scanner.scan(in, state, t.lexemes)
	case javaTokenizer:
		return scanner.scan(in, state, t.lexemes)
	case csTokenizer:
		return scanner.scan(in, state, t.lexemes)
	case rustTokenizer:
		return scanner.scan(in, state, t.lexemes)
	case ktTokenizer:
		return scanner.scan(in, state, t.lexemes)
	case swiftTokenizer:
		return scanner.scan(in, state, t.lexemes)
	case dartTokenizer:
		return scanner.scan(in, state, t.lexemes)
	case scalaTokenizer:
		return scanner.scan(in, state, t.lexemes)
	case grTokenizer:
		return scanner.scan(in, state, t.lexemes)
	case psTokenizer:
		return scanner.scan(in, state, t.lexemes)
	case jsxTokenizer:
		return scanner.scan(in, state, t.lexemes)
	case phpTokenizer:
		return scanner.scan(in, state, t.lexemes)
	case luaTokenizer:
		return scanner.scan(in, state, t.lexemes)
	case rbTokenizer:
		return scanner.scan(in, state, t.lexemes)
	case configTokenizer:
		return scanner.scan(in, state, t.lexemes)
	case codeTokenizer:
		return scanner.scanFolding(in, state, nil, t.lexemes)
	case builtinTokenizer:
		return scanner.scan(in, state, t.lexemes)
	case zigTokenizer:
		return scanner.scan(in, state, t.lexemes)
	case rTokenizer:
		return scanner.scan(in, state, t.lexemes)
	case gqlTokenizer:
		return scanner.scan(in, state, t.lexemes)
	case hcTokenizer:
		return scanner.scan(in, state, t.lexemes)
	case plTokenizer:
		return scanner.scan(in, state, t.lexemes)
	default:
		return t.Tokenizer.Scan(in, state)
	}
}
