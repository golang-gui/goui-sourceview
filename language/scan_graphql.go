package language

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

type gqlMode uint8

const (
	gqlCode gqlMode = iota
	gqlString
	gqlBlock
	gqlComment
	gqlNumber
)

type gqlState struct {
	mode                         gqlMode
	word                         scanWord
	wordOn, escape, escapeBraces bool
	escapeLeft                   int
	last                         byte
}

func (s gqlState) Equal(other State) bool { t, ok := other.(gqlState); return ok && s == t }
func (s gqlState) Context() Context {
	return Context{String: s.mode == gqlString || s.mode == gqlBlock, Literal: s.mode == gqlBlock, Comment: s.mode == gqlComment}
}

type gqlTokenizer struct{}

func (gqlTokenizer) InitialState() State { return gqlState{} }

var gqlKeywords = words("query mutation subscription fragment on schema scalar type interface union enum input extend implements directive repeatable true false null Int Float String Boolean ID QUERY MUTATION SUBSCRIPTION FIELD FRAGMENT_DEFINITION FRAGMENT_SPREAD INLINE_FRAGMENT VARIABLE_DEFINITION SCHEMA SCALAR OBJECT FIELD_DEFINITION ARGUMENT_DEFINITION INTERFACE UNION ENUM ENUM_VALUE INPUT_OBJECT INPUT_FIELD_DEFINITION")

func gqlName(r rune) bool {
	return r == '_' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9'
}
func (gqlTokenizer) Scan(in Input, state State) (Result, error) {
	s, ok := state.(gqlState)
	if !ok {
		return Result{}, fmt.Errorf("language: foreign GraphQL state")
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
		kind := s.word.kind
		if kind == KindProperty && gqlKeywords[s.word.text] {
			kind = KindKeyword
		}
		w.add(s.word.start, end, kind, 0)
		s.word, s.wordOn = scanWord{}, false
	}
	for i < limit {
		c := text[i]
		r, n := utf8.DecodeRuneInString(text[i:])
		switch s.mode {
		case gqlComment:
			add(i, i+n, KindComment, ClassComment)
			i += n
			continue
		case gqlBlock:
			if strings.HasPrefix(text[i:], `\"""`) {
				add(i, i+4, KindEscape, ClassString)
				i += 4
				continue
			}
			if strings.HasPrefix(text[i:], `"""`) {
				add(i, i+3, KindString, ClassString)
				i += 3
				s.mode = gqlCode
				continue
			}
			add(i, i+n, KindString, ClassString)
			i += n
			continue
		case gqlString:
			if s.escapeLeft > 0 || s.escapeBraces {
				if hex(c) {
					add(i, i+1, KindEscape, ClassString)
					i++
					if s.escapeLeft > 0 {
						s.escapeLeft--
					}
					continue
				}
				if s.escapeBraces && c == '}' {
					s.escapeBraces = false
					add(i, i+1, KindEscape, ClassString)
					i++
					continue
				}
				s.escapeLeft, s.escapeBraces = 0, false
			}
			if s.escape {
				s.escape = false
				if c == 'u' {
					if strings.HasPrefix(text[i:], "u{") {
						s.escapeBraces = true
						add(i, i+2, KindEscape, ClassString)
						i += 2
						continue
					}
					s.escapeLeft = 4
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
			if c == '"' {
				s.mode = gqlCode
			}
			add(i, i+n, KindString, ClassString)
			i += n
			continue
		case gqlNumber:
			if digit(c) || c == '.' && i+1 < len(text) && digit(text[i+1]) || c == 'e' || c == 'E' || (c == '+' || c == '-') && (s.last == 'e' || s.last == 'E') {
				add(i, i+1, KindNumber, 0)
				s.last = c
				i++
				continue
			}
			s.mode, s.last = gqlCode, 0
		}
		if s.wordOn {
			if gqlName(r) {
				s.word.add(text[i : i+n])
				i += n
				continue
			}
			finishWord(in.Offset + i)
			continue
		}
		if space(c) || c == ',' || r == '\ufeff' {
			i += n
			continue
		}
		if c == '#' {
			s.mode = gqlComment
			continue
		}
		if c == '"' {
			s.mode = gqlString
			size := 1
			if strings.HasPrefix(text[i:], `"""`) {
				s.mode, size = gqlBlock, 3
			}
			add(i, i+size, KindString, ClassString)
			i += size
			continue
		}
		if digit(c) || c == '-' && i+1 < len(text) && digit(text[i+1]) {
			s.mode, s.last = gqlNumber, c
			add(i, i+1, KindNumber, 0)
			i++
			continue
		}
		if gqlName(r) || c == '$' || c == '@' {
			kind := KindProperty
			if c == '$' {
				kind = KindVariable
			}
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
		finishWord(in.Offset + i)
		if s.mode != gqlBlock {
			s.mode = gqlCode
		}
		s.escape, s.escapeBraces, s.escapeLeft, s.last = false, false, 0, 0
	}
	return w.result(i, s), nil
}
