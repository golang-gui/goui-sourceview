package language

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

type envMode uint8

const (
	envStart envMode = iota
	envKey
	envEquals
	envValue
	envBare
	envQuoted
	envAfter
	envComment
	envInvalid
)

type envState struct {
	mode          envMode
	word          scanWord
	quote         byte
	bad, exported bool
}

func (s envState) Equal(other State) bool { t, ok := other.(envState); return ok && s == t }
func (s envState) Context() Context {
	return Context{String: s.mode == envBare || s.mode == envQuoted || s.mode == envAfter, Literal: true, Comment: s.mode == envComment, Data: s.mode}
}

type envTokenizer struct{}

func (envTokenizer) InitialState() State { return envState{} }
func envName(c rune) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}
func (t envTokenizer) Scan(in Input, state State) (Result, error) {
	return t.scan(in, state, nil)
}
func (envTokenizer) scan(in Input, state State, folds *foldLexemes) (Result, error) {
	s, ok := state.(envState)
	if !ok {
		return Result{}, fmt.Errorf("language: foreign Dotenv state")
	}
	limit, err := scanLimit(in)
	if err != nil {
		return Result{}, err
	}
	w := tokenWriter{base: in.Offset}
	text, i := in.Text, 0
	add := func(a, z int, k Kind, c Class) { w.add(in.Offset+a, in.Offset+z, k, c) }
	finishKey := func(end int, prefix bool) {
		kind := KindProperty
		s.mode = envEquals
		if s.bad {
			kind = KindInvalid
		}
		if prefix && !s.exported && s.word.text == "export" {
			kind, s.mode, s.exported = KindKeyword, envStart, true
		}
		w.add(s.word.start, end, kind, 0)
		s.word, s.bad = scanWord{}, false
	}
	for i < limit {
		c := text[i]
		r, n := utf8.DecodeRuneInString(text[i:])
		switch s.mode {
		case envComment:
			add(i, i+n, KindComment, ClassComment)
			i += n
			continue
		case envInvalid:
			add(i, i+n, KindInvalid, 0)
			i += n
			continue
		case envQuoted:
			if s.quote == '"' && strings.HasPrefix(text[i:], `\n`) {
				add(i, i+2, KindEscape, ClassString)
				i += 2
				continue
			}
			if c == s.quote {
				folds.closeLiteral(in.Offset+i+n, true)
				s.quote, s.mode = 0, envAfter
			}
			add(i, i+n, KindString, ClassString)
			i += n
			continue
		case envBare:
			if c == '#' {
				s.mode = envComment
				continue
			}
			add(i, i+n, KindString, ClassString)
			i += n
			continue
		case envAfter:
			if c == '#' {
				s.mode = envComment
				continue
			}
			add(i, i+n, KindPlain, ClassString)
			i += n
			continue
		case envKey:
			if space(c) || c == '=' || c == '#' {
				finishKey(in.Offset+i, space(c))
				continue
			}
			s.bad = s.bad || !envName(r)
			s.word.add(text[i : i+n])
			i += n
			continue
		case envEquals:
			if space(c) {
				i += n
				continue
			}
			if c == '=' {
				s.mode = envValue
				add(i, i+1, KindDelimiter, 0)
				i++
				continue
			}
			if c == '#' {
				s.mode = envComment
				continue
			}
			s.mode = envInvalid
			continue
		case envValue:
			if space(c) {
				i += n
				continue
			}
			if c == '#' {
				s.mode = envComment
				continue
			}
			if c == '"' || c == '\'' || c == '`' {
				folds.openLiteral()
				s.mode, s.quote = envQuoted, c
				add(i, i+1, KindString, ClassString)
				i++
				continue
			}
			s.mode = envBare
			continue
		case envStart:
			if space(c) {
				i += n
				continue
			}
			if c == '#' {
				s.mode = envComment
				continue
			}
			s.mode, s.word, s.bad = envKey, scanWord{start: in.Offset + i}, !envName(r) || digit(c)
			s.word.add(text[i : i+n])
			i += n
		}
	}
	if in.LineEnd && i == len(text) {
		if s.mode == envKey {
			finishKey(in.Offset+i, false)
		}
		if s.mode != envQuoted {
			s = envState{}
		}
	}
	return w.result(i, s), nil
}
