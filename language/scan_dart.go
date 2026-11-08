package language

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

type dartMode uint8

const (
	dartCode dartMode = iota
	dartString
	dartVariable
	dartLineComment
	dartBlockComment
	dartNumber
)

type dartLiteral struct {
	quote       byte
	raw, triple bool
}
type dartFrame struct {
	parent  *dartFrame
	literal dartLiteral
	braces  int
}
type dartState struct {
	mode                             dartMode
	literal                          dartLiteral
	frames                           *dartFrame
	depth, unicodeLeft               int
	escape, unicodeBraces, hexNumber bool
	last                             byte
	word                             scanWord
	wordOn                           bool
}

func (s dartState) Equal(other State) bool {
	t, ok := other.(dartState)
	if !ok {
		return false
	}
	for a, b := s.frames, t.frames; a != b; a, b = a.parent, b.parent {
		if a == nil || b == nil || a.literal != b.literal || a.braces != b.braces {
			return false
		}
	}
	s.frames, t.frames = nil, nil
	return s == t
}
func (s dartState) Context() Context {
	str := s.mode == dartString || s.mode == dartVariable
	return Context{String: str, Literal: str && s.literal.raw, Comment: s.mode == dartLineComment || s.mode == dartBlockComment}
}

type dartTokenizer struct{}

func (dartTokenizer) InitialState() State { return dartState{} }

var dartKeywords = words("abstract as assert async await augment base break case catch class const continue covariant default deferred do dynamic else enum export extends extension external factory false final finally for Function get hide if implements import in interface is late library mixin native new null of on operator part required rethrow return sealed set show static super switch sync this throw true try type typedef var void when while with yield")

func (t dartTokenizer) Scan(in Input, state State) (Result, error) {
	return t.scan(in, state, nil)
}

func (dartTokenizer) scan(in Input, state State, folds *foldLexemes) (Result, error) {
	s, ok := state.(dartState)
	if !ok {
		return Result{}, fmt.Errorf("language: foreign Dart state")
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
		if kind == KindPlain && dartKeywords[s.word.text] {
			kind = KindKeyword
		}
		w.add(s.word.start, end, kind, 0)
		s.word, s.wordOn = scanWord{}, false
	}
	endLiteral := func(complete bool) {
		folds.closeLiteral(in.Offset+i, complete)
		s.mode, s.literal, s.escape, s.unicodeLeft, s.unicodeBraces = dartCode, dartLiteral{}, false, 0, false
	}
	startLiteral := func(raw bool, start int) {
		folds.openLiteral()
		quote := text[i]
		triple := strings.HasPrefix(text[i:], strings.Repeat(string(quote), 3))
		n := 1
		if triple {
			n = 3
		}
		s.mode, s.literal = dartString, dartLiteral{quote: quote, raw: raw, triple: triple}
		w.add(start, in.Offset+i+n, KindString, ClassString)
		i += n
	}
	for i < limit {
		c := text[i]
		r, n := utf8.DecodeRuneInString(text[i:])
		switch s.mode {
		case dartLineComment:
			add(i, i+n, KindComment, ClassComment)
			i += n
			continue
		case dartBlockComment:
			if strings.HasPrefix(text[i:], "/*") {
				folds.openComment()
				s.depth++
				add(i, i+2, KindComment, ClassComment)
				i += 2
				continue
			}
			if strings.HasPrefix(text[i:], "*/") {
				folds.closeComment(in.Offset + i + 2)
				s.depth--
				add(i, i+2, KindComment, ClassComment)
				i += 2
				if s.depth == 0 {
					s.mode = dartCode
				}
				continue
			}
			add(i, i+n, KindComment, ClassComment)
			i += n
			continue
		case dartVariable:
			if IdentifierRune(r) || c == '$' {
				add(i, i+n, KindVariable, ClassString)
				i += n
				continue
			}
			s.mode = dartString
			continue
		case dartString:
			if !s.literal.raw {
				if s.unicodeLeft > 0 || s.unicodeBraces {
					if hex(c) {
						add(i, i+1, KindEscape, ClassString)
						i++
						if s.unicodeLeft > 0 {
							s.unicodeLeft--
						}
						continue
					}
					braced := s.unicodeBraces
					s.unicodeLeft, s.unicodeBraces = 0, false
					if braced && c == '}' {
						add(i, i+1, KindEscape, ClassString)
						i++
						continue
					}
				}
				if s.escape {
					s.escape = false
					if c == 'u' {
						if i+1 < len(text) && text[i+1] == '{' {
							s.unicodeBraces = true
							add(i, i+2, KindEscape, ClassString)
							i += 2
							continue
						}
						s.unicodeLeft = 4
					} else if c == 'x' {
						s.unicodeLeft = 2
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
				if c == '$' && i+1 < len(text) {
					next, _ := utf8.DecodeRuneInString(text[i+1:])
					if next == '{' {
						add(i, i+2, KindDelimiter, ClassDelimiter)
						s.frames = &dartFrame{parent: s.frames, literal: s.literal, braces: 1}
						s.mode, s.literal = dartCode, dartLiteral{}
						i += 2
						continue
					}
					if codeNameStart(Dart, next) || next == '$' {
						s.mode = dartVariable
						add(i, i+1, KindVariable, ClassString)
						i++
						continue
					}
				}
			}
			if c == s.literal.quote {
				count := 1
				if s.literal.triple {
					count = 3
				}
				if strings.HasPrefix(text[i:], strings.Repeat(string(c), count)) {
					add(i, i+count, KindString, ClassString)
					i += count
					endLiteral(true)
					continue
				}
			}
			add(i, i+n, KindString, ClassString)
			i += n
			continue
		case dartNumber:
			if IdentifierRune(r) || c == '.' && !s.hexNumber && i+1 < len(text) && digit(text[i+1]) || (c == '+' || c == '-') && !s.hexNumber && (s.last == 'e' || s.last == 'E') {
				if c == 'x' || c == 'X' {
					s.hexNumber = true
				}
				s.last = c
				add(i, i+n, KindNumber, 0)
				i += n
				continue
			}
			s.mode, s.last, s.hexNumber = dartCode, 0, false
		}
		if s.wordOn {
			if (c == '"' || c == '\'') && s.word.kind == KindPlain && s.word.text == "r" && s.word.length == 1 {
				start := s.word.start
				s.word, s.wordOn = scanWord{}, false
				startLiteral(true, start)
				continue
			}
			if IdentifierRune(r) || c == '$' || s.word.kind == KindMeta && c == '.' {
				s.word.add(text[i : i+n])
				i += n
				continue
			}
			finishWord(in.Offset + i)
		}
		if space(c) {
			i++
			continue
		}
		if strings.HasPrefix(text[i:], "//") || in.Offset+i == 0 && strings.HasPrefix(text[i:], "#!") {
			s.mode = dartLineComment
			continue
		}
		if strings.HasPrefix(text[i:], "/*") {
			folds.openComment()
			s.mode, s.depth = dartBlockComment, 1
			add(i, i+2, KindComment, ClassComment)
			i += 2
			continue
		}
		if c == '"' || c == '\'' {
			startLiteral(false, in.Offset+i)
			continue
		}
		if strings.HasPrefix(text[i:], "..") {
			add(i, i+2, KindDelimiter, 0)
			i += 2
			continue
		}
		if digit(c) || c == '.' && i+1 < len(text) && digit(text[i+1]) {
			s.mode, s.last = dartNumber, c
			add(i, i+1, KindNumber, 0)
			i++
			continue
		}
		if IdentifierRune(r) || c == '$' || c == '@' || c == '#' {
			kind := KindPlain
			if c == '@' || c == '#' {
				kind = KindMeta
			}
			s.word, s.wordOn = scanWord{start: in.Offset + i, kind: kind}, true
			s.word.add(text[i : i+n])
			i += n
			continue
		}
		if f := s.frames; f != nil && (c == '{' || c == '}') {
			next := *f
			if c == '{' {
				next.braces++
			} else {
				next.braces--
			}
			if next.braces == 0 {
				s.mode, s.literal, s.frames = dartString, f.literal, f.parent
			} else {
				s.frames = &next
			}
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
		if s.mode == dartVariable {
			s.mode = dartString
		}
		if s.mode == dartString && !s.literal.triple {
			endLiteral(false)
		}
		if s.mode == dartLineComment || s.mode == dartNumber {
			s.mode = dartCode
		}
		s.escape, s.unicodeLeft, s.unicodeBraces, s.last, s.hexNumber = false, 0, false, 0, false
	}
	return w.result(i, s), nil
}
