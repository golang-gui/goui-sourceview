package language

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// tokenWriter applies late classifications within the current logical line.
// Its coordinates are line-relative, including when a token began in an earlier
// fragment. Spelling retained by the scanners is bounded independently of length.
type tokenWriter struct {
	base    int
	tokens  []Token
	replace *int
}

func (w *tokenWriter) add(start, end int, kind Kind, class Class) {
	if start >= end {
		return
	}
	if start < w.base && (w.replace == nil || start < *w.replace) {
		n := start
		w.replace = &n
	}
	for len(w.tokens) > 0 && w.tokens[len(w.tokens)-1].End > start {
		t := &w.tokens[len(w.tokens)-1]
		if t.Start < start {
			t.End = start
			break
		}
		w.tokens = w.tokens[:len(w.tokens)-1]
	}
	if kind == KindPlain && class == 0 {
		return
	}
	if len(w.tokens) > 0 {
		t := &w.tokens[len(w.tokens)-1]
		if t.End == start && t.Kind == kind && t.Class == class {
			t.End = end
			return
		}
	}
	w.tokens = append(w.tokens, Token{Start: start, End: end, Kind: kind, Class: class})
}

func (w *tokenWriter) result(n int, state State) Result {
	return Result{Consumed: n, State: state, Tokens: w.tokens, ReplaceFrom: w.replace}
}

func scanLimit(in Input) (int, error) {
	if in.Offset < 0 || in.Limit < 0 || in.Limit > len(in.Text) {
		return 0, fmt.Errorf("language: invalid scan input")
	}
	n := in.Limit
	for n < len(in.Text) && !utf8.RuneStart(in.Text[n]) {
		n++
	}
	return n, nil
}

type scanWord struct {
	start, length int
	text          string
	kind          Kind
}

func (w *scanWord) add(text string) {
	w.length += len(text)
	if w.length <= 128 {
		w.text += text
	} else {
		w.text = ""
	}
}

func words(text string) map[string]bool {
	out := make(map[string]bool)
	for _, word := range strings.Fields(text) {
		out[word] = true
	}
	return out
}

// scanText retains immutable spelling fragments without copying a long prefix
// on each scan. State equality compares spelling rather than chunk boundaries.
type scanText struct {
	previous *scanText
	text     string
}

func captureText(p *scanText) string {
	var parts []string
	for p != nil {
		parts = append(parts, p.text)
		p = p.previous
	}
	var b strings.Builder
	for i := len(parts) - 1; i >= 0; i-- {
		b.WriteString(parts[i])
	}
	return b.String()
}
