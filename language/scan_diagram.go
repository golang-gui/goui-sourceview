package language

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

type mermaidTokenizer struct{}
type mermaidState struct {
	quoted, first, markdown, tick, comment, directive, wordOn bool
	directiveEnd                                              int
	word                                                      scanWord
}

func (s mermaidState) Equal(other State) bool { t, ok := other.(mermaidState); return ok && s == t }
func (s mermaidState) Context() Context {
	return Context{String: s.quoted || s.directive, Literal: s.quoted || s.directive, Comment: s.comment}
}
func (mermaidTokenizer) InitialState() State { return mermaidState{} }

var mermaidWords = strings.Fields("flowchart flowchart-elk graph subgraph end direction TB TD BT LR RL sequenceDiagram participant actor as Note note over left right of loop alt else opt par and rect critical option break box autonumber activate deactivate classDiagram class namespace stateDiagram-v2 stateDiagram state erDiagram gantt dateFormat axisFormat excludes section title pie showData journey gitGraph commit branch checkout merge mindmap timeline requirementDiagram requirement element style classDef linkStyle click href accTitle accDescr")

func (mermaidTokenizer) Scan(in Input, previous State) (Result, error) {
	s, ok := previous.(mermaidState)
	if !ok {
		return Result{}, fmt.Errorf("language: foreign Mermaid state")
	}
	limit, err := scanLimit(in)
	if err != nil {
		return Result{}, err
	}
	w := tokenWriter{base: in.Offset}
	finish := func(end int) {
		kind := s.word.kind
		if slices.Contains(mermaidWords, s.word.text) {
			kind = KindKeyword
		}
		w.add(s.word.start, end, kind, 0)
		s.word, s.wordOn = scanWord{}, false
	}
	i := 0
	for i < limit {
		c := in.Text[i]
		r, n := utf8.DecodeRuneInString(in.Text[i:])
		at := in.Offset + i
		if s.comment {
			w.add(at, at+n, KindComment, ClassComment)
			i += n
			continue
		}
		if s.directive {
			close := "}%%"
			if c == close[s.directiveEnd] {
				s.directiveEnd++
			} else if c == '}' {
				s.directiveEnd = 1
			} else {
				s.directiveEnd = 0
			}
			w.add(at, at+n, KindMeta, ClassString)
			i += n
			if s.directiveEnd == len(close) {
				s.directive, s.directiveEnd = false, 0
			}
			continue
		}
		if s.quoted {
			w.add(at, at+n, KindString, ClassString)
			if s.first && c == '`' {
				s.markdown = true
			} else if c == '"' && (!s.markdown || s.tick) {
				s.quoted, s.markdown, s.tick = false, false, false
			}
			s.first = false
			if s.markdown {
				s.tick = c == '`'
			}
			i += n
			continue
		}
		if s.wordOn {
			if IdentifierRune(r) || c == '-' && i+1 < len(in.Text) && in.Text[i+1] != '-' && in.Text[i+1] != '>' {
				s.word.add(in.Text[i : i+n])
				i += n
				continue
			}
			finish(at)
			continue
		}
		if strings.HasPrefix(in.Text[i:], "%%{") {
			s.directive = true
			w.add(at, at+3, KindMeta, ClassString)
			i += 3
			continue
		}
		if strings.HasPrefix(in.Text[i:], "%%") {
			s.comment = true
			continue
		}
		if c == '"' {
			s.quoted, s.first = true, true
			w.add(at, at+n, KindString, ClassString)
			i += n
			continue
		}
		if IdentifierRune(r) {
			kind := KindPlain
			if digit(c) {
				kind = KindNumber
			}
			s.word, s.wordOn = scanWord{start: at, kind: kind}, true
			s.word.add(in.Text[i : i+n])
			i += n
			continue
		}
		if !space(c) {
			class := Class(0)
			if strings.ContainsRune("()[]{}", r) {
				class = ClassDelimiter
			}
			w.add(at, at+n, KindDelimiter, class)
		}
		i += n
	}
	if in.LineEnd && i == len(in.Text) {
		if s.wordOn {
			finish(in.Offset + i)
		}
		s.comment, s.directiveEnd = false, 0
		s.first, s.tick = false, false
	}
	return w.result(i, s), nil
}

type dotTokenizer struct{}
type dotState struct {
	mode, htmlQuote                               byte
	htmlDepth, htmlEnd, slashAt, keyStart, keyEnd int
	htmlComment, slash, wordOn, pendingKey        bool
	word                                          scanWord
}

func (s dotState) Equal(other State) bool { t, ok := other.(dotState); return ok && s == t }
func (s dotState) Context() Context {
	return Context{String: s.mode == '"' || s.mode == '<', Literal: s.mode == '<', Comment: s.mode == '/' || s.mode == '*'}
}
func (dotTokenizer) InitialState() State { return dotState{} }

var dotWords = strings.Fields("strict graph digraph subgraph node edge label rankdir rank shape style color fillcolor fontname fontsize penwidth")

func dotKeyword(s string) bool {
	switch strings.ToLower(s) {
	case "strict", "graph", "digraph", "subgraph", "node", "edge":
		return true
	}
	return false
}
func (b dotTokenizer) Scan(in Input, previous State) (Result, error) {
	return b.scan(in, previous, nil)
}

func (dotTokenizer) scan(in Input, previous State, folds *foldLexemes) (Result, error) {
	s, ok := previous.(dotState)
	if !ok {
		return Result{}, fmt.Errorf("language: foreign DOT state")
	}
	limit, err := scanLimit(in)
	if err != nil {
		return Result{}, err
	}
	w := tokenWriter{base: in.Offset}
	finish := func(end int) {
		kind := s.word.kind
		if dotKeyword(s.word.text) {
			kind = KindKeyword
		}
		w.add(s.word.start, end, kind, 0)
		s.pendingKey, s.keyStart, s.keyEnd = kind == KindPlain, s.word.start, end
		s.word, s.wordOn = scanWord{}, false
	}
	i := 0
	for i < limit {
		c := in.Text[i]
		r, n := utf8.DecodeRuneInString(in.Text[i:])
		at := in.Offset + i
		if s.mode == '/' {
			w.add(at, at+n, KindComment, ClassComment)
			i += n
			continue
		}
		if s.mode == '*' {
			if strings.HasPrefix(in.Text[i:], "*/") {
				w.add(at, at+2, KindComment, ClassComment)
				folds.closeComment(at + 2)
				s.mode = 0
				i += 2
			} else {
				w.add(at, at+n, KindComment, ClassComment)
				i += n
			}
			continue
		}
		if s.mode == '"' {
			if s.slash {
				s.slash = false
				if c == '"' {
					w.add(s.slashAt, at+n, KindEscape, ClassString)
					s.slashAt = 0
					i += n
					continue
				}
			}
			w.add(at, at+n, KindString, ClassString)
			if c == '\\' {
				s.slash, s.slashAt = true, at
			} else if c == '"' {
				folds.closeLiteral(at+n, true)
				s.mode = 0
			}
			i += n
			continue
		}
		if s.mode == '<' {
			if s.htmlComment {
				close := "-->"
				if c == close[s.htmlEnd] {
					s.htmlEnd++
				} else if c == '-' {
					s.htmlEnd = 1
				} else {
					s.htmlEnd = 0
				}
				if s.htmlEnd == len(close) {
					s.htmlComment, s.htmlEnd = false, 0
					s.htmlDepth--
				}
			} else if s.htmlQuote != 0 {
				if c == s.htmlQuote {
					s.htmlQuote = 0
				}
			} else {
				switch c {
				case '<':
					s.htmlDepth++
					if strings.HasPrefix(in.Text[i:], "<!--") {
						s.htmlComment = true
						w.add(at, at+4, KindString, ClassString)
						i += 4
						continue
					}
				case '>':
					s.htmlDepth--
				case '\'', '"':
					if s.htmlDepth > 1 {
						s.htmlQuote = c
					}
				}
			}
			w.add(at, at+n, KindString, ClassString)
			i += n
			if s.htmlDepth == 0 {
				folds.closeLiteral(at+n, true)
				s.mode = 0
			}
			continue
		}
		if s.wordOn {
			if IdentifierRune(r) || s.word.kind == KindNumber && c == '.' {
				s.word.add(in.Text[i : i+n])
				i += n
				continue
			}
			finish(at)
			continue
		}
		if space(c) {
			i += n
			continue
		}
		if c == '=' && s.pendingKey {
			w.add(s.keyStart, s.keyEnd, KindProperty, 0)
		}
		s.pendingKey = false
		if c == '#' || strings.HasPrefix(in.Text[i:], "//") {
			s.mode = '/'
			continue
		}
		if strings.HasPrefix(in.Text[i:], "/*") {
			folds.openComment()
			s.mode = '*'
			w.add(at, at+2, KindComment, ClassComment)
			i += 2
			continue
		}
		if c == '"' {
			folds.openLiteral()
			s.mode = '"'
			w.add(at, at+n, KindString, ClassString)
			i += n
			continue
		}
		if c == '<' {
			folds.openLiteral()
			s.mode, s.htmlDepth = '<', 1
			w.add(at, at+n, KindString, ClassString)
			i += n
			continue
		}
		if IdentifierRune(r) || (c == '-' || c == '.') && i+1 < len(in.Text) && digit(in.Text[i+1]) {
			kind := KindPlain
			if digit(c) || c == '-' || c == '.' {
				kind = KindNumber
			}
			s.word, s.wordOn = scanWord{start: at, kind: kind}, true
			s.word.add(in.Text[i : i+n])
			i += n
			continue
		}
		class := Class(0)
		if strings.ContainsRune("{}[]", r) {
			class = ClassDelimiter
		}
		w.add(at, at+n, KindDelimiter, class)
		i += n
	}
	if in.LineEnd && i == len(in.Text) {
		if s.wordOn {
			finish(in.Offset + i)
		}
		if s.mode == '/' || s.mode == '"' && !s.slash {
			if s.mode == '"' {
				folds.closeLiteral(in.Offset+i, false)
			}
			s.mode = 0
		}
		s.slash, s.slashAt, s.htmlEnd = false, 0, 0
		s.pendingKey, s.keyStart, s.keyEnd = false, 0, 0
	}
	return w.result(i, s), nil
}
