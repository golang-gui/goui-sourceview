package language

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

type zigMode uint8

const (
	zigCode zigMode = iota
	zigQuoted
	zigMultiline
	zigComment
	zigNumber
)

type zigState struct {
	mode                             zigMode
	quote                            byte
	escape, unicodeBraces, hexNumber bool
	escapeLeft                       int
	last                             byte
	quotedName                       bool
	word                             scanWord
	wordOn                           bool
}

func (s zigState) Equal(other State) bool { t, ok := other.(zigState); return ok && s == t }
func (s zigState) Context() Context {
	return Context{String: s.mode == zigQuoted || s.mode == zigMultiline, Literal: s.mode == zigMultiline, Comment: s.mode == zigComment}
}

type zigTokenizer struct{}

func (zigTokenizer) InitialState() State { return zigState{} }

var zigKeywords = words("addrspace align allowzero and anyframe anytype asm async await break callconv catch comptime const continue defer else enum errdefer error export extern false fn for if inline linksection noalias noinline nosuspend null opaque or orelse packed pub resume return section static struct suspend switch test threadlocal true try undefined union unreachable usingnamespace var volatile while bool anyopaque anyerror void noreturn type usize isize comptime_int comptime_float c_char c_short c_ushort c_int c_uint c_long c_ulong c_longlong c_ulonglong c_longdouble f16 f32 f64 f80 f128")

func zigKeyword(word string) bool {
	return zigKeywords[word] || len(word) > 1 && (word[0] == 'u' || word[0] == 'i') && strings.Trim(word[1:], "0123456789") == ""
}
func (t zigTokenizer) Scan(in Input, state State) (Result, error) {
	return t.scan(in, state, nil)
}

func (zigTokenizer) scan(in Input, state State, folds *foldLexemes) (Result, error) {
	s, ok := state.(zigState)
	if !ok {
		return Result{}, fmt.Errorf("language: foreign Zig state")
	}
	limit, err := scanLimit(in)
	if err != nil {
		return Result{}, err
	}
	w := tokenWriter{base: in.Offset}
	text, i := in.Text, 0
	add := func(a, z int, k Kind, c Class) { w.add(in.Offset+a, in.Offset+z, k, c) }
	finish := func(end int) {
		if s.wordOn {
			kind := s.word.kind
			if kind == KindPlain && zigKeyword(s.word.text) {
				kind = KindKeyword
			}
			w.add(s.word.start, end, kind, 0)
			s.word, s.wordOn = scanWord{}, false
		}
	}
	for i < limit {
		c := text[i]
		r, n := utf8.DecodeRuneInString(text[i:])
		switch s.mode {
		case zigComment:
			add(i, i+n, KindComment, ClassComment)
			i += n
			continue
		case zigMultiline:
			add(i, i+n, KindString, ClassString)
			i += n
			continue
		case zigQuoted:
			if s.escapeLeft > 0 || s.unicodeBraces {
				if hex(c) {
					add(i, i+1, KindEscape, ClassString)
					i++
					if s.escapeLeft > 0 {
						s.escapeLeft--
					}
					continue
				}
				braced := s.unicodeBraces
				s.escapeLeft, s.unicodeBraces = 0, false
				if braced && c == '}' {
					add(i, i+1, KindEscape, ClassString)
					i++
					continue
				}
			}
			if s.escape {
				s.escape = false
				if c == 'x' {
					s.escapeLeft = 2
				}
				if c == 'u' && i+1 < len(text) && text[i+1] == '{' {
					s.unicodeBraces = true
					add(i, i+2, KindEscape, ClassString)
					i += 2
					continue
				}
				add(i, i+n, KindEscape, ClassString)
				i += n
				continue
			}
			if c == '\\' {
				s.escape = true
				add(i, i+1, KindEscape, ClassString)
				i++
				continue
			}
			kind := KindString
			if s.quotedName {
				kind = KindVariable
			}
			add(i, i+n, kind, ClassString)
			i += n
			if c == s.quote {
				s.mode, s.quote, s.quotedName = zigCode, 0, false
			}
			continue
		case zigNumber:
			if IdentifierRune(r) || c == '.' && i+1 < len(text) && (digit(text[i+1]) || s.hexNumber && hex(text[i+1])) && !strings.HasPrefix(text[i:], "..") || (c == '+' || c == '-') && (s.last == 'p' || s.last == 'P' || !s.hexNumber && (s.last == 'e' || s.last == 'E')) {
				if c == 'x' || c == 'X' {
					s.hexNumber = true
				}
				s.last = c
				add(i, i+n, KindNumber, 0)
				i += n
				continue
			}
			s.mode, s.last, s.hexNumber = zigCode, 0, false
		}
		if s.wordOn {
			if c == '"' && s.word.text == "@" {
				w.add(s.word.start, in.Offset+i+1, KindVariable, ClassString)
				s.word, s.wordOn = scanWord{}, false
				s.mode, s.quote, s.quotedName = zigQuoted, '"', true
				i++
				continue
			}
			if IdentifierRune(r) {
				s.word.add(text[i : i+n])
				i += n
				continue
			}
			finish(in.Offset + i)
		}
		if space(c) {
			i++
			continue
		}
		if strings.HasPrefix(text[i:], "//") {
			s.mode = zigComment
			continue
		}
		if strings.HasPrefix(text[i:], `\\`) {
			if folds != nil {
				folds.lineLiteral = in.Offset + i
			}
			s.mode = zigMultiline
			continue
		}
		if c == '"' || c == '\'' {
			s.mode, s.quote = zigQuoted, c
			add(i, i+1, KindString, ClassString)
			i++
			continue
		}
		if digit(c) {
			s.mode, s.last = zigNumber, c
			add(i, i+1, KindNumber, 0)
			i++
			continue
		}
		if IdentifierRune(r) || c == '@' {
			kind := KindPlain
			if c == '@' {
				kind = KindMeta
			}
			s.word, s.wordOn = scanWord{start: in.Offset + i, kind: kind}, true
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
		finish(in.Offset + i)
		s = zigState{}
	}
	return w.result(i, s), nil
}
