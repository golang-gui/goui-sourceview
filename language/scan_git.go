package language

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

type gitState struct {
	id                                        ID
	comment, escape, bracket                  bool
	started, attributes, quoted, value, macro bool
	octalLeft                                 int
}

func (s gitState) Equal(other State) bool { t, ok := other.(gitState); return ok && s == t }
func (s gitState) Context() Context {
	return Context{String: !s.comment && (!s.attributes || s.value), Literal: true, Comment: s.comment, Data: s.attributes}
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
		if b.id == GitAttributes {
			if !s.started && space(c) {
				i += n
				continue
			}
			if s.attributes {
				if space(c) {
					s.value = false
					i += n
					continue
				}
				kind, class := KindProperty, Class(0)
				if c == '=' && !s.value {
					kind, s.value = KindDelimiter, true
				} else if s.value {
					kind, class = KindString, ClassString
				} else if c == '-' || c == '!' {
					kind = KindDelimiter
				}
				add(i, i+n, kind, class)
				i += n
				continue
			}
			if s.started && !s.quoted && space(c) {
				s.attributes, s.escape, s.bracket = true, false, false
				i += n
				continue
			}
		}
		if !s.started {
			if c == '#' {
				s.comment = true
				continue
			}
			s.started = true
			if b.id == GitAttributes {
				if strings.HasPrefix(text[i:], "[attr]") {
					s.macro = true
					add(i, i+6, KindMeta, ClassString)
					i += 6
					continue
				}
				if c == '"' {
					s.quoted = true
					add(i, i+1, KindString, ClassString)
					i++
					continue
				}
			}
		}
		kind := KindString
		if s.macro {
			kind = KindProperty
		}
		if s.octalLeft > 0 {
			if c >= '0' && c <= '7' {
				s.octalLeft--
				add(i, i+1, KindEscape, ClassString)
				i++
				continue
			}
			s.octalLeft = 0
		}
		if s.escape {
			s.escape = false
			kind = KindEscape
			if s.quoted && c >= '0' && c <= '7' {
				s.octalLeft = 2
			}
		} else if c == '\\' {
			s.escape = true
			kind = KindEscape
		} else {
			if s.quoted && c == '"' {
				s.quoted = false
				add(i, i+1, KindString, ClassString)
				i++
				continue
			}
			if c == '[' {
				s.bracket = true
			}
			if s.bracket || c == '/' {
				kind = KindDelimiter
			}
			if strings.ContainsRune("*?", r) || b.id == GitIgnore && in.Offset+i == 0 && c == '!' {
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
