package language

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

type ktMode uint8

const (
	ktCode ktMode = iota
	ktQuoted
	ktBacktick
	ktLineComment
	ktBlockComment
	ktNumber
	ktPrefix
	ktDollars
	ktVariable
)

type ktLiteral struct {
	quote  byte
	triple bool
	arity  int
}
type ktFrame struct {
	parent  *ktFrame
	literal ktLiteral
	braces  int
}
type ktState struct {
	mode                                               ktMode
	literal                                            ktLiteral
	frames                                             *ktFrame
	depth, quoteRun, dollars, dollarStart, unicodeLeft int
	escape, quotedVariable, numberStart, baseNumber    bool
	word                                               scanWord
	wordOn                                             bool
	last                                               byte
}

func (s ktState) Equal(other State) bool {
	t, ok := other.(ktState)
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
func (s ktState) Context() Context {
	str := s.mode == ktQuoted || s.mode == ktDollars || s.mode == ktVariable
	if s.mode == ktQuoted && s.literal.triple && s.quoteRun >= 3 {
		str = false
	}
	return Context{String: str || s.mode == ktBacktick, Literal: str && s.literal.triple || s.mode == ktBacktick, Comment: s.mode == ktLineComment || s.mode == ktBlockComment}
}

type ktTokenizer struct{}

func (ktTokenizer) InitialState() State { return ktState{} }

var ktKeywords = words("as break class continue do else false for fun if in interface is null object package return super this throw true try typealias typeof val var when while by catch constructor delegate dynamic field file finally get import init param property receiver set setparam where actual abstract annotation companion const crossinline data enum expect external final infix inline inner internal lateinit noinline open operator out override private protected public reified sealed suspend tailrec vararg value context")

func (ktTokenizer) Scan(in Input, state State) (Result, error) {
	s, ok := state.(ktState)
	if !ok {
		return Result{}, fmt.Errorf("language: foreign Kotlin state")
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
		if kind == KindPlain && ktKeywords[s.word.text] {
			kind = KindKeyword
		}
		w.add(s.word.start, end, kind, 0)
		s.word, s.wordOn = scanWord{}, false
	}
	endLiteral := func() { s.mode, s.literal, s.escape, s.quoteRun, s.unicodeLeft = ktCode, ktLiteral{}, false, 0, 0 }
	startString := func(arity int) {
		n := 1
		triple := text[i] == '"' && strings.HasPrefix(text[i:], `"""`)
		if triple {
			n = 3
		}
		start := in.Offset + i
		if s.mode == ktPrefix {
			start = s.dollarStart
		}
		s.mode, s.literal, s.dollars, s.dollarStart = ktQuoted, ktLiteral{quote: text[i], triple: triple, arity: arity}, 0, 0
		w.add(start, in.Offset+i+n, KindString, ClassString)
		i += n
	}
	for i < limit {
		c := text[i]
		r, n := utf8.DecodeRuneInString(text[i:])
		switch s.mode {
		case ktLineComment:
			add(i, i+n, KindComment, ClassComment)
			i += n
			continue
		case ktBlockComment:
			if strings.HasPrefix(text[i:], "/*") {
				s.depth++
				add(i, i+2, KindComment, ClassComment)
				i += 2
				continue
			}
			if strings.HasPrefix(text[i:], "*/") {
				s.depth--
				add(i, i+2, KindComment, ClassComment)
				i += 2
				if s.depth == 0 {
					s.mode = ktCode
				}
				continue
			}
			add(i, i+n, KindComment, ClassComment)
			i += n
			continue
		case ktBacktick:
			add(i, i+n, KindVariable, ClassString)
			i += n
			if c == '`' {
				s.mode = ktCode
			}
			continue
		case ktPrefix:
			if c == '$' {
				s.dollars++
				add(i, i+1, KindDelimiter, 0)
				i++
				continue
			}
			if c == '"' {
				startString(max(1, s.dollars))
				continue
			}
			s.mode, s.dollars, s.dollarStart = ktCode, 0, 0
			continue
		case ktDollars:
			if c == '$' {
				s.dollars++
				add(i, i+1, KindString, ClassString)
				i++
				continue
			}
			if s.dollars >= s.literal.arity && (c == '{' || codeNameStart(Kotlin, r) || c == '`') {
				start := in.Offset + i - s.literal.arity
				if c == '{' {
					w.add(start, in.Offset+i+1, KindDelimiter, ClassDelimiter)
					s.frames = &ktFrame{parent: s.frames, literal: s.literal, braces: 1}
					s.mode, s.literal = ktCode, ktLiteral{}
					i++
				} else {
					w.add(start, in.Offset+i, KindVariable, ClassString)
					s.mode = ktVariable
					if c == '`' {
						s.quotedVariable = true
						add(i, i+1, KindVariable, ClassString)
						i++
					}
				}
				s.dollars, s.dollarStart = 0, 0
				continue
			}
			s.mode, s.dollars, s.dollarStart = ktQuoted, 0, 0
			continue
		case ktVariable:
			if s.quotedVariable {
				add(i, i+n, KindVariable, ClassString)
				i += n
				if c == '`' {
					s.quotedVariable = false
					s.mode = ktQuoted
				}
				continue
			}
			if IdentifierRune(r) {
				add(i, i+n, KindVariable, ClassString)
				i += n
				continue
			}
			s.mode = ktQuoted
			continue
		case ktQuoted:
			if s.literal.triple {
				if c == '"' {
					s.quoteRun++
					add(i, i+1, KindString, ClassString)
					i++
					continue
				}
				if s.quoteRun >= 3 {
					endLiteral()
					continue
				}
				s.quoteRun = 0
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
					add(i, i+n, KindEscape, ClassString)
					s.escape = false
					if c == 'u' {
						s.unicodeLeft = 4
					}
					i += n
					continue
				}
				if c == '\\' {
					add(i, i+1, KindEscape, ClassString)
					s.escape = true
					i++
					continue
				}
				if c == s.literal.quote {
					add(i, i+1, KindString, ClassString)
					i++
					endLiteral()
					continue
				}
			}
			if c == '$' && s.literal.arity > 0 {
				s.mode, s.dollars, s.dollarStart = ktDollars, 1, in.Offset+i
				add(i, i+1, KindString, ClassString)
				i++
				continue
			}
			add(i, i+n, KindString, ClassString)
			i += n
			continue
		case ktNumber:
			if IdentifierRune(r) || c == '.' && !s.baseNumber && i+1 < len(text) && digit(text[i+1]) || (c == '+' || c == '-') && !s.baseNumber && strings.ContainsRune("eE", rune(s.last)) {
				if s.numberStart && strings.ContainsRune("xXbB", r) {
					s.baseNumber = true
				}
				s.numberStart = false
				s.last = c
				add(i, i+n, KindNumber, 0)
				i += n
				continue
			}
			s.mode, s.last, s.numberStart, s.baseNumber = ktCode, 0, false, false
		}
		if s.wordOn {
			if IdentifierRune(r) || s.word.kind == KindMeta && (c == ':' || c == '.') {
				s.word.add(text[i : i+n])
				i += n
				continue
			}
			if c == '@' && s.word.kind == KindPlain && !ktKeywords[s.word.text] {
				w.add(s.word.start, in.Offset+i+1, KindMeta, 0)
				s.word, s.wordOn = scanWord{}, false
				i++
				continue
			}
			finishWord(in.Offset + i)
		}
		if space(c) {
			i++
			continue
		}
		if strings.HasPrefix(text[i:], "//") || in.Offset+i == 0 && strings.HasPrefix(text[i:], "#!") {
			s.mode = ktLineComment
			continue
		}
		if strings.HasPrefix(text[i:], "/*") {
			s.mode, s.depth = ktBlockComment, 1
			add(i, i+2, KindComment, ClassComment)
			i += 2
			continue
		}
		if c == '"' || c == '\'' {
			arity := 0
			if c == '"' {
				arity = 1
			}
			startString(arity)
			continue
		}
		if c == '`' {
			s.mode = ktBacktick
			add(i, i+1, KindVariable, ClassString)
			i++
			continue
		}
		if c == '$' {
			s.mode, s.dollars, s.dollarStart = ktPrefix, 1, in.Offset+i
			add(i, i+1, KindDelimiter, 0)
			i++
			continue
		}
		if strings.HasPrefix(text[i:], "..") {
			n = 2
			if strings.HasPrefix(text[i:], "..<") {
				n = 3
			}
			add(i, i+n, KindDelimiter, 0)
			i += n
			continue
		}
		if digit(c) || c == '.' && i+1 < len(text) && digit(text[i+1]) {
			s.mode, s.last, s.numberStart = ktNumber, c, c == '0'
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
		if f := s.frames; f != nil && (c == '{' || c == '}') {
			next := *f
			if c == '{' {
				next.braces++
			} else {
				next.braces--
			}
			if next.braces == 0 {
				s.mode, s.literal, s.frames = ktQuoted, f.literal, f.parent
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
		if s.mode == ktVariable {
			s.mode, s.quotedVariable = ktQuoted, false
		}
		if s.mode == ktDollars {
			s.mode = ktQuoted
		}
		if s.mode == ktQuoted && (!s.literal.triple || s.quoteRun >= 3) {
			endLiteral()
		}
		if s.mode == ktLineComment || s.mode == ktBacktick || s.mode == ktNumber || s.mode == ktPrefix {
			s.mode = ktCode
		}
		s.escape, s.numberStart, s.baseNumber, s.last, s.quoteRun, s.dollars, s.dollarStart, s.unicodeLeft = false, false, false, 0, 0, 0, 0, 0
	}
	return w.result(i, s), nil
}
