package language

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

type scalaMode uint8

const (
	scalaCode scalaMode = iota
	scalaQuoted
	scalaBacktick
	scalaVariable
	scalaLineComment
	scalaBlockComment
	scalaNumber
)

type scalaLiteral struct {
	quote                    byte
	triple, interpolate, raw bool
}
type scalaFrame struct {
	parent  *scalaFrame
	literal scalaLiteral
	braces  int
}
type scalaState struct {
	mode                       scalaMode
	literal                    scalaLiteral
	frames                     *scalaFrame
	depth, quotes, unicodeLeft int
	escape, hexNumber          bool
	last                       byte
	word                       scanWord
	wordOn                     bool
}

func (s scalaState) Equal(other State) bool {
	t, ok := other.(scalaState)
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
func (s scalaState) Context() Context {
	str := s.mode == scalaQuoted || s.mode == scalaVariable || s.mode == scalaBacktick
	if s.mode == scalaQuoted && s.literal.triple && s.quotes >= 3 {
		str = false
	}
	return Context{String: str, Literal: str && s.literal.triple, Comment: s.mode == scalaLineComment || s.mode == scalaBlockComment}
}

type scalaTokenizer struct{}

func (scalaTokenizer) InitialState() State { return scalaState{} }

var scalaKeywords = words("abstract case catch class def do else extends false final finally for forSome if implicit import lazy match new null object override package private protected return sealed super this throw trait try true type val var while with yield enum export given then using end extension inline opaque open transparent derives infix erased macro")

func (t scalaTokenizer) Scan(in Input, state State) (Result, error) {
	return t.scan(in, state, nil)
}

func (scalaTokenizer) scan(in Input, state State, folds *foldLexemes) (Result, error) {
	s, ok := state.(scalaState)
	if !ok {
		return Result{}, fmt.Errorf("language: foreign Scala state")
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
			if kind == KindPlain && scalaKeywords[s.word.text] {
				kind = KindKeyword
			}
			w.add(s.word.start, end, kind, 0)
			s.word, s.wordOn = scanWord{}, false
		}
	}
	endLiteral := func(complete bool) {
		folds.closeLiteral(in.Offset+i, complete)
		s.mode, s.literal, s.escape, s.quotes, s.unicodeLeft = scalaCode, scalaLiteral{}, false, 0, 0
	}
	startLiteral := func(start int, interp, raw bool) {
		folds.openLiteral()
		n := 1
		triple := text[i] == '"' && strings.HasPrefix(text[i:], `"""`)
		if triple {
			n = 3
		}
		s.mode, s.literal = scalaQuoted, scalaLiteral{quote: text[i], triple: triple, interpolate: interp, raw: raw}
		w.add(start, in.Offset+i+n, KindString, ClassString)
		i += n
	}
	for i < limit {
		c := text[i]
		r, n := utf8.DecodeRuneInString(text[i:])
		switch s.mode {
		case scalaLineComment:
			add(i, i+n, KindComment, ClassComment)
			i += n
			continue
		case scalaBlockComment:
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
					s.mode = scalaCode
				}
				continue
			}
			add(i, i+n, KindComment, ClassComment)
			i += n
			continue
		case scalaBacktick:
			kind := KindVariable
			if s.escape {
				kind = KindEscape
				s.escape = false
			} else if c == '\\' {
				kind = KindEscape
				s.escape = true
			} else if c == '`' {
				s.mode = scalaCode
			}
			add(i, i+n, kind, ClassString)
			i += n
			continue
		case scalaVariable:
			if IdentifierRune(r) {
				add(i, i+n, KindVariable, ClassString)
				i += n
				continue
			}
			s.mode = scalaQuoted
			continue
		case scalaQuoted:
			if s.literal.triple {
				if c == '"' {
					s.quotes++
					add(i, i+1, KindString, ClassString)
					i++
					continue
				}
				if s.quotes >= 3 {
					endLiteral(true)
					continue
				}
				s.quotes = 0
			} else {
				if s.unicodeLeft > 0 {
					if hex(c) {
						add(i, i+1, KindEscape, ClassString)
						s.unicodeLeft--
						i++
						continue
					}
					s.unicodeLeft = 0
				}
				if s.escape {
					kind := KindEscape
					if s.literal.raw {
						kind = KindString
					}
					s.escape = false
					if c == 'u' && !s.literal.interpolate {
						s.unicodeLeft = 4
					}
					add(i, i+n, kind, ClassString)
					i += n
					continue
				}
				if c == '\\' {
					// Interpolated backslashes do not consume a following dollar.
					s.escape = !(s.literal.interpolate && i+1 < len(text) && text[i+1] == '$')
					kind := KindEscape
					if s.literal.raw {
						kind = KindString
					}
					add(i, i+1, kind, ClassString)
					i++
					continue
				}
				if c == s.literal.quote {
					add(i, i+1, KindString, ClassString)
					i++
					endLiteral(true)
					continue
				}
			}
			if s.literal.interpolate && c == '$' && i+1 < len(text) {
				next, _ := utf8.DecodeRuneInString(text[i+1:])
				if next == '$' || next == '"' {
					add(i, i+2, KindEscape, ClassString)
					i += 2
					continue
				}
				if next == '{' {
					add(i, i+2, KindDelimiter, ClassDelimiter)
					s.frames = &scalaFrame{parent: s.frames, literal: s.literal, braces: 1}
					s.mode, s.literal = scalaCode, scalaLiteral{}
					i += 2
					continue
				}
				if codeNameStart(Scala, next) {
					s.mode = scalaVariable
					add(i, i+1, KindVariable, ClassString)
					i++
					continue
				}
			}
			add(i, i+n, KindString, ClassString)
			i += n
			continue
		case scalaNumber:
			if IdentifierRune(r) || c == '.' && !s.hexNumber && i+1 < len(text) && digit(text[i+1]) || (c == '+' || c == '-') && !s.hexNumber && (s.last == 'e' || s.last == 'E') {
				if c == 'x' || c == 'X' {
					s.hexNumber = true
				}
				s.last = c
				add(i, i+n, KindNumber, 0)
				i += n
				continue
			}
			s.mode, s.hexNumber, s.last = scalaCode, false, 0
		}
		if s.wordOn {
			if c == '"' && s.word.kind == KindPlain && !scalaKeywords[s.word.text] {
				start, raw := s.word.start, s.word.text == "raw"
				s.word, s.wordOn = scanWord{}, false
				startLiteral(start, true, raw)
				continue
			}
			if IdentifierRune(r) || c == '$' || s.word.kind == KindMeta && c == '.' {
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
		if strings.HasPrefix(text[i:], "//") || in.Offset+i == 0 && strings.HasPrefix(text[i:], "#!") {
			s.mode = scalaLineComment
			continue
		}
		if strings.HasPrefix(text[i:], "/*") {
			folds.openComment()
			s.mode, s.depth = scalaBlockComment, 1
			add(i, i+2, KindComment, ClassComment)
			i += 2
			continue
		}
		if c == '"' {
			startLiteral(in.Offset+i, false, false)
			continue
		}
		if c == '`' {
			s.mode = scalaBacktick
			add(i, i+1, KindVariable, ClassString)
			i++
			continue
		}
		if c == '\'' {
			var next rune
			size := 0
			if i+1 < len(text) {
				next, size = utf8.DecodeRuneInString(text[i+1:])
			}
			if next == '\\' || size > 0 && i+1+size < len(text) && text[i+1+size] == '\'' {
				startLiteral(in.Offset+i, false, false)
				continue
			}
			if codeNameStart(Scala, next) {
				s.word, s.wordOn = scanWord{start: in.Offset + i, kind: KindVariable}, true
				s.word.add("'")
				i++
				continue
			}
			add(i, i+1, KindMeta, 0)
			i++
			continue
		}
		if digit(c) || c == '.' && i+1 < len(text) && digit(text[i+1]) {
			s.mode, s.last = scalaNumber, c
			add(i, i+1, KindNumber, 0)
			i++
			continue
		}
		if IdentifierRune(r) || c == '$' || c == '@' {
			kind := KindPlain
			if c == '@' {
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
				s.mode, s.literal, s.frames = scalaQuoted, f.literal, f.parent
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
		finish(in.Offset + i)
		if s.mode == scalaVariable {
			s.mode = scalaQuoted
		}
		if s.mode == scalaQuoted && (!s.literal.triple || s.quotes >= 3) {
			endLiteral(s.literal.triple && s.quotes >= 3)
		}
		if s.mode == scalaLineComment || s.mode == scalaNumber || s.mode == scalaBacktick {
			s.mode = scalaCode
		}
		s.quotes, s.unicodeLeft, s.escape, s.hexNumber, s.last = 0, 0, false, false, 0
	}
	return w.result(i, s), nil
}
