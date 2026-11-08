package language

// foldLexemes observes confirmed lexical boundaries during one folding scan.
// It belongs to that worker, never to a registered tokenizer or saved State.
// Stacks retain outer literals across interpolation and nested block comments.
type foldLexemes struct {
	row                int
	literals, comments []int
	closed             []foldLexeme
}

type foldLexeme struct{ start, end int }

func (f *foldLexemes) openLiteral() {
	if f != nil {
		f.literals = append(f.literals, f.row)
	}
}

func (f *foldLexemes) closeLiteral(end int, complete bool) {
	if f != nil {
		f.close(&f.literals, end, complete)
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
	default:
		return t.Tokenizer.Scan(in, state)
	}
}
