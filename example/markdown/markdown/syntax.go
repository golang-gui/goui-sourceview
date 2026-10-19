// Package markdown demonstrates an application-owned SourceView language. It is
// deliberately a small Markdown subset, not a CommonMark parser or renderer.
package markdown

import (
	"strings"
	"unicode/utf8"

	"github.com/golang-gui/goui-sourceview/language"
)

const (
	Heading  language.Kind = "markdown.heading"
	Emphasis language.Kind = "markdown.emphasis"
	Code     language.Kind = "markdown.code"
	Fence    language.Kind = "markdown.fence"
	Link     language.Kind = "markdown.link"
	Marker   language.Kind = "markdown.marker"
)

// state is a value snapshot; no scan mutates a previously returned state.
type state struct {
	fence    byte
	comment  bool
	link     bool
	inline   byte
	emphasis string
	line     language.Kind
	closing  bool
	closeEnd int
}

func (s state) Equal(other language.State) bool { t, ok := other.(state); return ok && s == t }
func (s state) Context() language.Context {
	return language.Context{String: s.fence != 0 || s.inline != 0 || s.link, Comment: s.comment, Literal: s.fence != 0 || s.inline != 0}
}

type tokenizer struct{}

func (tokenizer) InitialState() language.State { return state{} }
func (tokenizer) Scan(in language.Input, previous language.State) (language.Result, error) {
	s := previous.(state)
	if in.Limit == 0 && len(in.Text) > 0 {
		return language.Result{State: s}, nil
	}
	n := in.Limit
	for n < len(in.Text) && !utf8.RuneStart(in.Text[n]) {
		n++
	}
	text := in.Text
	var out []language.Token
	emit := func(a, b int, kind language.Kind, class language.Class) {
		if a < b {
			out = append(out, language.Token{Start: in.Offset + a, End: in.Offset + b, Kind: kind, Class: class})
		}
	}
	if in.Offset == 0 {
		s.line = ""
		trimmed := strings.TrimLeft(text, " \t")
		indent := len(text) - len(trimmed)
		if !s.comment && indent <= 3 && (strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~")) {
			c := trimmed[0]
			if s.fence == 0 {
				s.fence = c
				s.line = Fence
			} else if s.fence == c {
				s.line = Fence
				s.closing = strings.TrimSpace(strings.TrimLeft(trimmed, string(c))) == ""
				if s.closing {
					s.closeEnd = indent + len(trimmed) - len(strings.TrimLeft(trimmed, string(c)))
				}
			}
		} else if s.fence == 0 && strings.HasPrefix(trimmed, "#") {
			width := len(trimmed) - len(strings.TrimLeft(trimmed, "#"))
			if width <= 6 && (width == len(trimmed) || trimmed[width] == ' ' || trimmed[width] == '\t') {
				s.line = Heading
			}
		}
	}
	if in.Offset > 0 && s.closing && strings.TrimSpace(in.Text[min(n, max(0, s.closeEnd-in.Offset)):n]) != "" {
		s.closing = false
		s.closeEnd = 0
	}
	i := 0
	for i < n {
		start := i
		if s.line == Fence {
			emit(i, n, Fence, language.ClassString)
			i = n
			continue
		}
		if s.fence != 0 {
			emit(i, n, Code, language.ClassString)
			i = n
			continue
		}
		if s.comment || strings.HasPrefix(text[i:], "<!--") {
			offset := 0
			if !s.comment {
				offset = 4
			}
			end := strings.Index(text[i+offset:], "-->")
			if end >= 0 && i+offset+end < n {
				i += offset + end + 3
				s.comment = false
			} else {
				i = n
				s.comment = true
			}
			emit(start, i, language.KindComment, language.ClassComment)
			continue
		}
		if s.inline != 0 {
			end := strings.IndexByte(text[i:], s.inline)
			if end >= 0 && i+end < n {
				i += end + 1
				s.inline = 0
			} else {
				i = n
			}
			emit(start, i, Code, language.ClassString)
			continue
		}
		if text[i] == '`' {
			s.inline = '`'
			i++
			emit(start, i, Code, language.ClassString)
			continue
		}
		if strings.HasPrefix(text[i:], "**") || strings.HasPrefix(text[i:], "__") {
			marker := text[i : i+2]
			if s.emphasis == marker {
				s.emphasis = ""
			} else if s.emphasis == "" {
				s.emphasis = marker
			}
			i += 2
			emit(start, i, Emphasis, 0)
			continue
		}
		if s.link && strings.ContainsRune(" \t)", rune(text[i])) {
			s.link = false
		}
		if strings.ContainsRune("[]()", rune(text[i])) {
			i++
			emit(start, i, language.KindDelimiter, language.ClassDelimiter)
			continue
		}
		if s.link || strings.HasPrefix(text[i:], "https://") || strings.HasPrefix(text[i:], "http://") {
			s.link = true
			for i < n && !strings.ContainsRune(" \t)", rune(text[i])) {
				_, size := utf8.DecodeRuneInString(text[i:])
				i += size
			}
			emit(start, i, Link, language.ClassString)
			if i < n {
				s.link = false
			}
			if i > start {
				continue
			}
		}
		_, size := utf8.DecodeRuneInString(text[i:])
		i += size
		kind := s.line
		if s.emphasis != "" {
			kind = Emphasis
		}
		if kind != "" {
			emit(start, i, kind, 0)
		} else if strings.ContainsRune("#>*-+", rune(text[start])) {
			emit(start, i, Marker, 0)
		}
	}
	if in.LineEnd && i == len(text) {
		if s.closing {
			s.fence = 0
			s.closing = false
			s.closeEnd = 0
		}
		s.closeEnd = 0
		s.line = ""
		s.link = false
		s.inline = 0
		s.emphasis = ""
	}
	return language.Result{Consumed: i, State: s, Tokens: out}, nil
}

// NewLanguage builds an independent definition with application-owned styles
// and editing rules. SourceView itself does not gain a Markdown builtin.
func NewLanguage() (*language.Language, error) {
	pair := func(open, close string) language.Pair {
		return language.Pair{Open: open, Close: close, NotInString: true, NotInComment: true}
	}
	brackets := []language.Pair{pair("[", "]"), pair("(", ")")}
	pairs := append(append([]language.Pair(nil), brackets...), pair("`", "`"), pair("```", "```"), pair("**", "**"))
	return language.New(language.Definition{Info: language.Info{ID: "markdown", Name: "Markdown", Extensions: []string{".md"}}, Tokenizer: tokenizer{}, Styles: []language.Kind{Heading, Emphasis, Code, Fence, Link, Marker}, Editing: language.EditingRules{Word: func(r rune) bool { return language.IdentifierRune(r) || r == '-' }, AutoClosingPairs: pairs, SurroundingPairs: pairs, Brackets: brackets, AutoCloseBefore: ")]},:;*`", Newline: newline, Comments: language.Comments{Block: language.Pair{Open: "<!--", Close: "-->"}}}})
}
