package language

import (
	"fmt"
	"unicode/utf8"
)

type propMode uint8

const (
	propStart propMode = iota
	propKey
	propAfterKey
	propValueStart
	propValue
	propComment
)

type propState struct {
	mode                propMode
	escape, skipLeading bool
	unicodeLeft         int
}

func (s propState) Equal(other State) bool { t, ok := other.(propState); return ok && s == t }
func (s propState) Context() Context {
	return Context{String: s.mode == propValueStart || s.mode == propValue, Literal: true, Comment: s.mode == propComment}
}

type propTokenizer struct{}

func (propTokenizer) InitialState() State { return propState{} }
func propSpace(c byte) bool               { return c == ' ' || c == '\t' || c == '\f' }
func (propTokenizer) Scan(in Input, state State) (Result, error) {
	s, ok := state.(propState)
	if !ok {
		return Result{}, fmt.Errorf("language: foreign Properties state")
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
		_, n := utf8.DecodeRuneInString(text[i:])
		if c == '\r' && in.LineEnd && i+1 == len(text) {
			i++
			continue
		}
		if s.skipLeading {
			if propSpace(c) {
				i++
				continue
			}
			s.skipLeading = false
		}
		if s.mode == propComment {
			add(i, i+n, KindComment, ClassComment)
			i += n
			continue
		}
		class := Class(0)
		if s.mode == propValue {
			class = ClassString
		}
		if s.unicodeLeft > 0 {
			if hex(c) {
				s.unicodeLeft--
				add(i, i+1, KindEscape, class)
				i++
				continue
			}
			s.unicodeLeft = 0
		}
		if s.escape {
			s.escape = false
			if s.mode == propStart {
				s.mode = propKey
			}
			if s.mode == propAfterKey || s.mode == propValueStart {
				s.mode = propValue
			}
			if s.mode == propValue {
				class = ClassString
			}
			if c == 'u' {
				s.unicodeLeft = 4
			}
			add(i, i+n, KindEscape, class)
			i += n
			continue
		}
		if c == '\\' && (s.mode == propStart || s.mode == propAfterKey || s.mode == propValueStart) {
			s.escape = true
			if s.mode != propStart {
				class = ClassString
			}
			add(i, i+1, KindEscape, class)
			i++
			continue
		}
		switch s.mode {
		case propStart:
			if propSpace(c) {
				i++
				continue
			}
			if c == '#' || c == '!' {
				s.mode = propComment
				continue
			}
			s.mode = propKey
			continue
		case propKey:
			if c == '=' || c == ':' {
				s.mode = propValueStart
				add(i, i+1, KindDelimiter, 0)
				i++
				continue
			}
			if propSpace(c) {
				s.mode = propAfterKey
				i++
				continue
			}
		case propAfterKey:
			if propSpace(c) {
				i++
				continue
			}
			s.mode = propValueStart
			if c == '=' || c == ':' {
				add(i, i+1, KindDelimiter, 0)
				i++
				continue
			}
			continue
		case propValueStart:
			if propSpace(c) {
				i++
				continue
			}
			s.mode = propValue
			continue
		}
		class = 0
		kind := KindProperty
		if s.mode == propValue {
			class, kind = ClassString, KindString
		}
		if c == '\\' {
			s.escape = true
			kind = KindEscape
		}
		add(i, i+n, kind, class)
		i += n
	}
	if in.LineEnd && i == len(text) {
		if s.escape {
			s.escape, s.skipLeading = false, true
		} else {
			s = propState{}
		}
	}
	return w.result(i, s), nil
}
