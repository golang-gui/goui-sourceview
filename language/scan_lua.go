package language

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

type luaMode uint8

const (
	luaNormal luaMode = iota
	luaQuoted
	luaLineComment
	luaLongOpen
	luaLongBody
	luaNumber
)

type luaState struct {
	mode                       luaMode
	quote                      byte
	escape, skipSpace, comment bool
	equals, matched, start     int
	word                       scanWord
	wordOn                     bool
	last                       byte
}

func (s luaState) Equal(other State) bool { t, ok := other.(luaState); return ok && s == t }
func (s luaState) Context() Context {
	return Context{String: s.mode == luaQuoted || s.mode == luaLongBody && !s.comment, Comment: s.mode == luaLineComment || (s.mode == luaLongOpen || s.mode == luaLongBody) && s.comment, Literal: s.mode == luaLongBody && !s.comment}
}

type luaTokenizer struct{}

func (luaTokenizer) InitialState() State { return luaState{} }

var luaKeywords = words("and break do else elseif end false for function goto if in local nil not or repeat return then true until while")

func (luaTokenizer) Scan(in Input, state State) (Result, error) {
	s, ok := state.(luaState)
	if !ok {
		return Result{}, fmt.Errorf("language: foreign Lua state")
	}
	limit, err := scanLimit(in)
	if err != nil {
		return Result{}, err
	}
	w := tokenWriter{base: in.Offset}
	text, i := in.Text, 0
	add := func(a, z int, k Kind, c Class) { w.add(in.Offset+a, in.Offset+z, k, c) }
	finishWord := func(end int) {
		if !s.wordOn {
			return
		}
		kind := KindPlain
		if luaKeywords[s.word.text] {
			kind = KindKeyword
		}
		w.add(s.word.start, end, kind, 0)
		s.word, s.wordOn = scanWord{}, false
	}
	for i < limit {
		c := text[i]
		r, n := utf8.DecodeRuneInString(text[i:])
		switch s.mode {
		case luaLineComment:
			add(i, i+n, KindComment, ClassComment)
			i += n
			continue
		case luaQuoted:
			if s.skipSpace {
				if strings.ContainsRune(" \t\r\v\f", r) {
					add(i, i+n, KindEscape, ClassString)
					i += n
					continue
				}
				s.skipSpace = false
			}
			kind := KindString
			if s.escape {
				kind, s.escape = KindEscape, false
				s.skipSpace = c == 'z'
			} else if c == '\\' {
				kind, s.escape = KindEscape, true
			} else if c == s.quote {
				s.mode, s.quote = luaNormal, 0
			}
			add(i, i+n, kind, ClassString)
			i += n
			continue
		case luaLongOpen:
			kind, class := KindDelimiter, Class(0)
			if s.comment {
				kind, class = KindComment, ClassComment
			}
			if c == '=' {
				s.equals++
				add(i, i+1, kind, class)
				i++
				continue
			}
			if c == '[' {
				if !s.comment {
					kind, class = KindString, ClassString
				}
				w.add(s.start, in.Offset+i+1, kind, class)
				s.mode, s.start = luaLongBody, 0
				i++
				continue
			}
			if s.comment {
				s.mode = luaLineComment
			} else {
				s.mode = luaNormal
			}
			s.equals, s.start = 0, 0
			continue
		case luaLongBody:
			kind, class := KindString, ClassString
			if s.comment {
				kind, class = KindComment, ClassComment
			}
			add(i, i+n, kind, class)
			if c == ']' && s.matched == s.equals+1 {
				s.mode, s.equals, s.matched, s.comment = luaNormal, 0, 0, false
			} else if c == ']' {
				s.matched = 1
			} else if c == '=' && s.matched > 0 {
				s.matched++
			} else {
				s.matched = 0
			}
			i += n
			continue
		case luaNumber:
			part := IdentifierRune(r) || (c == '+' || c == '-') && strings.ContainsRune("eEpP", rune(s.last)) || c == '.' && !(i+1 < len(text) && text[i+1] == '.')
			if part {
				add(i, i+n, KindNumber, 0)
				s.last = c
				i += n
				continue
			}
			s.mode, s.last = luaNormal, 0
		}
		if s.wordOn {
			if IdentifierRune(r) {
				s.word.add(text[i : i+n])
				i += n
				continue
			}
			finishWord(in.Offset + i)
		}
		if strings.ContainsRune(" \t\r\v\f", r) {
			i += n
			continue
		}
		if strings.HasPrefix(text[i:], "--") {
			add(i, i+2, KindComment, ClassComment)
			i += 2
			s.mode = luaLineComment
			if i < len(text) && text[i] == '[' {
				s.mode, s.comment, s.start = luaLongOpen, true, in.Offset+i
				add(i, i+1, KindComment, ClassComment)
				i++
			}
			continue
		}
		if c == '[' {
			s.mode, s.start = luaLongOpen, in.Offset+i
			add(i, i+1, KindDelimiter, ClassDelimiter)
			i++
			continue
		}
		if c == '"' || c == '\'' {
			s.mode, s.quote = luaQuoted, c
			add(i, i+1, KindString, ClassString)
			i++
			continue
		}
		if strings.HasPrefix(text[i:], "..") {
			n := 2
			if strings.HasPrefix(text[i:], "...") {
				n = 3
			}
			add(i, i+n, KindDelimiter, 0)
			i += n
			continue
		}
		if digit(c) || c == '.' && i+1 < len(text) && digit(text[i+1]) {
			s.mode, s.last = luaNumber, c
			add(i, i+1, KindNumber, 0)
			i++
			continue
		}
		if IdentifierRune(r) {
			s.word, s.wordOn = scanWord{start: in.Offset + i}, true
			s.word.add(text[i : i+n])
			i += n
			continue
		}
		class := Class(0)
		if strings.ContainsRune("()[]{}", r) {
			class = ClassDelimiter
		}
		add(i, i+n, KindDelimiter, class)
		i += n
	}
	if in.LineEnd && i == len(text) {
		finishWord(in.Offset + i)
		if s.mode == luaLineComment || s.mode == luaLongOpen || s.mode == luaNumber || s.mode == luaQuoted && !s.escape && !s.skipSpace {
			s.mode, s.quote, s.comment, s.equals, s.start = luaNormal, 0, false, 0, 0
		}
		s.escape, s.matched, s.last = false, 0, 0
	}
	return w.result(i, s), nil
}
