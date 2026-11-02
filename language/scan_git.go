package language

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

type gitState struct {
	id                       ID
	comment, escape, bracket bool
}

func (s gitState) Equal(other State) bool { t, ok := other.(gitState); return ok && s == t }
func (s gitState) Context() Context {
	return Context{String: !s.comment, Literal: true, Comment: s.comment}
}

type gitTokenizer struct{ id ID }

func (b gitTokenizer) InitialState() State { return gitState{id: b.id} }
func (b gitTokenizer) Scan(in Input, state State) (Result, error) {
	s, ok := state.(gitState)
	if !ok || s.id != b.id {
		return Result{}, fmt.Errorf("language: foreign Git pattern state")
	}
	limit, err := scanLimit(in)
	if err != nil {
		return Result{}, err
	}
	w := tokenWriter{base: in.Offset}
	text, i := in.Text, 0
	add := func(a, z int, k Kind, c Class) { w.add(in.Offset+a, in.Offset+z, k, c) }
	for i < limit {
		c := text[i]
		r, n := utf8.DecodeRuneInString(text[i:])
		if s.comment {
			add(i, i+n, KindComment, ClassComment)
			i += n
			continue
		}
		if in.Offset+i == 0 && c == '#' {
			s.comment = true
			continue
		}
		kind := KindString
		if s.escape {
			s.escape = false
			kind = KindEscape
		} else if c == '\\' {
			s.escape = true
			kind = KindEscape
		} else {
			if c == '[' {
				s.bracket = true
			}
			if s.bracket || c == '/' {
				kind = KindDelimiter
			}
			if strings.ContainsRune("*?", r) || in.Offset+i == 0 && c == '!' {
				kind = KindKeyword
			}
			if c == ']' {
				s.bracket = false
			}
		}
		add(i, i+n, kind, ClassString)
		i += n
	}
	if in.LineEnd && i == len(text) {
		s = gitState{id: b.id}
	}
	return w.result(i, s), nil
}
