package language

import (
	"context"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestRemainingSecondFolding(t *testing.T) {
	for _, tt := range []struct {
		id         ID
		name, text string
		want       []FoldRange
	}{
		{AsciiDoc, "headings", "= Doc\nintro\n== One\ntext\n=== Child\nvalue\n== Two\nlast\n", []FoldRange{{0, 7}, {2, 5}, {4, 5}, {6, 7}}},
		{AsciiDoc, "opaque fence", "= Doc\n----\n== fake\nbody\n----\n== Real\ntext", []FoldRange{{0, 6}, {1, 4}, {5, 6}}},
		{AsciiDoc, "example fence", "==== \n== fake\n==== \n== Real\ntext", []FoldRange{{0, 2}, {3, 4}}},
		{AsciiDoc, "deep headings", "==== Four\nbody\n===== Five\nlast", []FoldRange{{0, 3}, {2, 3}}},
		{AsciiDoc, "comment fence", "////\n== fake\n////", []FoldRange{{0, 2}}},
		{AsciiDoc, "unclosed fence", "----\n== fake\nbody", nil},
		{ReStructuredText, "headings", "Title\n=====\nintro\nChild\n-----\nvalue\nNext\n====\nlast", []FoldRange{{0, 5}, {3, 5}, {6, 8}}},
		{ReStructuredText, "overline", "=====\nTitle\n=====\nbody\n=====\nOther\n=====\nlast", []FoldRange{{1, 3}, {5, 7}}},
		{ReStructuredText, "closing adornment not reopened", "Title\n=====\nbody\n=====\nnot a title", []FoldRange{{0, 4}}},
		{ReStructuredText, "inline code title", "Title `x`\n=========\nbody", []FoldRange{{0, 2}}},
		{ReStructuredText, "code block", "Example::\n\n  text\n  ==== fake\n  more\n\nTail", []FoldRange{{0, 4}}},
		{ReStructuredText, "comment block", ".. comment\n   first\n   next\n\nTail", []FoldRange{{0, 2}}},
		{ReStructuredText, "empty section", "Title\n=====\n\nNext\n====\nlast", []FoldRange{{3, 5}}},
		{ReStructuredText, "transition", "paragraph\n\n----\n\ntext", nil},
		{ReStructuredText, "unicode headings", "文档\n====\n正文\n子节\n----\n内容", []FoldRange{{0, 5}, {3, 5}}},
		{LaTeX, "sections", `\section{One}` + "\ntext\n" + `\subsection{Child}` + "\nvalue\n" + `\section{Next}` + "\nlast", []FoldRange{{0, 3}, {2, 3}, {4, 5}}},
		{LaTeX, "environments", `\begin{outer}` + "\n" + `\begin{inner}` + "\nvalue\n" + `\end{inner}` + "\n" + `\end{outer}`, []FoldRange{{0, 4}, {1, 3}}},
		{LaTeX, "verbatim", `\begin{verbatim}` + "\n" + `\begin{fake}` + "\n" + `\section{fake}` + "\n" + `\end{verbatim}` + "\n" + `\section{Real}` + "\nbody", []FoldRange{{0, 3}, {4, 5}}},
		{LaTeX, "fake commands", `% \begin{fake}` + "\n" + `\verb|\begin{fake}|` + "\n" + `\section{Real}` + "\nbody", []FoldRange{{2, 3}}},
		{LaTeX, "multiline environment name", `\begin` + "\n{quote}\ntext\n" + `\end` + "\n{quote}", []FoldRange{{0, 4}}},
		{LaTeX, "closing tail", `\begin{quote}` + "\ntext\n" + `\end{quote} tail`, []FoldRange{{0, 1}}},
		{LaTeX, "mismatch", `\begin{a}` + "\n" + `\begin{b}` + "\ntext\n" + `\end{a}` + "\n" + `\end{b}`, nil},
		{LaTeX, "unclosed outer", `\begin{a}` + "\n" + `\begin{b}` + "\ntext\n" + `\end{b}`, []FoldRange{{1, 3}}},
		{LaTeX, "macro argument", `\textbf{` + "\n" + `\section{fake}` + "\n}\n" + `\section{Real}` + "\nbody", []FoldRange{{3, 4}}},
	} {
		t.Run(string(tt.id)+"/"+tt.name, func(t *testing.T) {
			l, _ := Builtin(tt.id)
			if l.Folding() == nil {
				t.Fatal("missing folding")
			}
			got, err := l.Folding()(context.Background(), tt.text)
			if err != nil || !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("%v %v; want %v", got, err, tt.want)
			}
		})
	}
}

func TestRemainingSecondWorkers(t *testing.T) {
	for _, id := range []ID{AsciiDoc, ReStructuredText, LaTeX} {
		t.Run(string(id), func(t *testing.T) {
			l, _ := Builtin(id)
			var wg sync.WaitGroup
			for w := 0; w < 8; w++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for n := 4090; n <= 4097; n++ {
						text := strings.Repeat("\n", w)
						switch id {
						case AsciiDoc:
							mark := strings.Repeat("-", n)
							text += mark + "\n" + strings.Repeat("x", 9000) + "\n" + mark
						case ReStructuredText:
							text += strings.Repeat("A", n) + "\n" + strings.Repeat("=", n) + "\n" + strings.Repeat("x", 9000)
						case LaTeX:
							text += strings.Repeat(" ", n) + `\begin{quote}` + "\n" + strings.Repeat("x", 9000) + "\n" + strings.Repeat(" ", n) + `\end{quote}`
						}
						got, err := l.Folding()(context.Background(), text)
						if err != nil || !reflect.DeepEqual(got, []FoldRange{{w, w + 2}}) {
							t.Errorf("%s %d/%d: %v %v", id, w, n, got, err)
						}
					}
				}()
			}
			wg.Wait()
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if got, err := l.Folding()(ctx, "text"); err != context.Canceled || got != nil {
				t.Fatalf("cancel: %v %v", got, err)
			}
			ctx, cancel = context.WithCancel(context.Background())
			defer cancel()
			scan := cancelCodeScanner{l.Tokenizer(), cancel}
			fold := proseFolding(id, scan)
			if id == LaTeX {
				fold = texFolding(scan)
			}
			if got, err := fold(ctx, strings.Repeat("x ", 16000)); err != context.Canceled || got != nil {
				t.Fatalf("mid scan: %v %v", got, err)
			}
		})
	}
}

func TestRemainingTexObservation(t *testing.T) {
	l, _ := Builtin(LaTeX)
	text := `\begin{quote}` + "\n正文\n" + `\begin{verbatim}` + "\n" + `\section{fake}` + "\n" + `\end{verbatim}` + "\n" + `\end{quote}`
	for chunk := 1; chunk <= 17; chunk++ {
		obs := foldingTokenizer{l.Tokenizer(), &foldLexemes{row: -1, tex: &texFoldObserver{}}}
		a, b := l.Tokenizer().InitialState(), l.Tokenizer().InitialState()
		for _, line := range strings.Split(text, "\n") {
			for at := 0; ; {
				in := Input{Text: line[at:], Offset: at, Limit: min(chunk, len(line)-at), LineEnd: true}
				x, err := l.Tokenizer().Scan(in, a)
				if err != nil {
					t.Fatal(err)
				}
				y, err := obs.Scan(in, b)
				if err != nil {
					t.Fatal(err)
				}
				a, b = x.State, y.State
				if !a.Equal(b) {
					t.Fatal("state changed")
				}
				y.State = x.State
				if !reflect.DeepEqual(x, y) {
					t.Fatal("scan result changed")
				}
				at += x.Consumed
				if at == len(line) {
					break
				}
				if x.Consumed <= 0 {
					t.Fatal("no progress")
				}
			}
		}
	}
}
