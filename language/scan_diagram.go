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

var mermaidWords = strings.Fields("flowchart flowchart-elk graph subgraph end direction TB TD BT LR RL sequenceDiagram participant actor as Note note over left right of loop alt else opt par and rect critical option break autonumber activate deactivate classDiagram class namespace stateDiagram-v2 stateDiagram state erDiagram gantt dateFormat axisFormat excludes section title pie showData journey gitGraph commit branch checkout merge mindmap timeline requirementDiagram requirement element style classDef linkStyle click href accTitle accDescr")

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
