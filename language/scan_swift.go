package language

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

type swiftMode uint8

const (
	swiftCode swiftMode = iota
	swiftString
	swiftRegex
	swiftBacktick
	swiftLineComment
	swiftBlockComment
	swiftPrefix
	swiftNumber
)

type swiftLiteral struct {
	hashes int
	triple bool
}
type swiftFrame struct {
	parent  *swiftFrame
	literal swiftLiteral
	parens  int
}
type swiftState struct {
	mode                                                                   swiftMode
	literal                                                                swiftLiteral
	frames                                                                 *swiftFrame
	depth, hashes, prefixStart, quotes, closing, escapeHashes, escapeStart int
	escaping, regexClass, expect, hexNumber                                bool
	last                                                                   byte
	word                                                                   scanWord
	wordOn                                                                 bool
}

func (s swiftState) Equal(other State) bool {
	t, ok := other.(swiftState)
	if !ok {
		return false
	}
	for a, b := s.frames, t.frames; a != b; a, b = a.parent, b.parent {
		if a == nil || b == nil || a.literal != b.literal || a.parens != b.parens {
			return false
		}
	}
	s.frames, t.frames = nil, nil
	return s == t
}
func (s swiftState) Context() Context {
	str := s.mode == swiftString || s.mode == swiftRegex || s.mode == swiftBacktick
	return Context{String: str, Literal: str && (s.literal.hashes > 0 || s.mode != swiftString), Comment: s.mode == swiftLineComment || s.mode == swiftBlockComment}
}

type swiftTokenizer struct{}

func (swiftTokenizer) InitialState() State { return swiftState{expect: true} }

var swiftKeywords = words("associatedtype borrowing class consuming deinit enum extension fileprivate func import init inout internal let nonisolated open operator package precedencegroup private protocol public rethrows static struct subscript typealias var break case catch continue default defer do else fallthrough for guard if in repeat return throw switch where while as Any any await catch false is nil rethrows self Self some super throws true try actor async convenience distributed dynamic didSet final get indirect infix isolated lazy left mutating none nonmutating optional override postfix prefix required right set unowned weak willSet sending each macro")

func (t swiftTokenizer) Scan(in Input, state State) (Result, error) {
	return t.scan(in, state, nil)
}

func (swiftTokenizer) scan(in Input, state State, folds *foldLexemes) (Result, error) {
	s, ok := state.(swiftState)
	if !ok {
		return Result{}, fmt.Errorf("language: foreign Swift state")
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
		if kind == KindPlain && swiftKeywords[s.word.text] {
			kind = KindKeyword
		}
		w.add(s.word.start, end, kind, 0)
		s.expect = kind == KindKeyword && strings.Contains(" return throw try await case in ", " "+s.word.text+" ")
		s.word, s.wordOn = scanWord{}, false
	}
	endLiteral := func(complete bool) {
		folds.closeLiteral(in.Offset+i, complete)
		s.mode, s.literal, s.quotes, s.closing, s.escapeHashes, s.escaping, s.regexClass, s.expect = swiftCode, swiftLiteral{}, 0, 0, 0, false, false, false
	}
	startLiteral := func(regex bool) {
		folds.openLiteral()
		start := in.Offset + i
		if s.mode == swiftPrefix {
			start = s.prefixStart
		}
		n := 1
		s.literal = swiftLiteral{hashes: s.hashes, triple: !regex && strings.HasPrefix(text[i:], `"""`)}
		if s.literal.triple {
			n = 3
		}
		s.mode = swiftString
		if regex {
			s.mode = swiftRegex
		}
		w.add(start, in.Offset+i+n, KindString, ClassString)
		s.hashes, s.prefixStart = 0, 0
		i += n
	}
	for i < limit {
		c := text[i]
		r, n := utf8.DecodeRuneInString(text[i:])
		switch s.mode {
		case swiftLineComment:
			add(i, i+n, KindComment, ClassComment)
			i += n
			continue
		case swiftBlockComment:
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
					s.mode = swiftCode
				}
				continue
			}
			add(i, i+n, KindComment, ClassComment)
			i += n
			continue
		case swiftBacktick:
			add(i, i+n, KindVariable, ClassString)
			i += n
			if c == '`' {
				s.mode, s.expect = swiftCode, false
			}
			continue
		case swiftPrefix:
			if c == '#' {
				s.hashes++
				add(i, i+1, KindDelimiter, 0)
				i++
				continue
			}
			if c == '"' || c == '/' {
				startLiteral(c == '/')
				continue
			}
			if IdentifierRune(r) {
				s.word, s.wordOn = scanWord{start: s.prefixStart, kind: KindMeta}, true
				s.mode, s.hashes, s.prefixStart = swiftCode, 0, 0
				continue
			}
			s.mode, s.hashes, s.prefixStart = swiftCode, 0, 0
			continue
		case swiftString, swiftRegex:
			if s.closing > 0 {
				if c == '#' {
					s.closing++
					add(i, i+1, KindString, ClassString)
					i++
					if s.closing-1 == s.literal.hashes {
						endLiteral(true)
					}
					continue
				}
				s.closing, s.quotes = 0, 0
			}
			if s.escaping {
				if s.mode == swiftString && s.escapeHashes < s.literal.hashes {
					if c == '#' {
						s.escapeHashes++
						add(i, i+1, KindString, ClassString)
						i++
						continue
					}
					s.escaping, s.escapeHashes = false, 0
					continue
				}
				start := s.escapeStart
				s.escaping, s.escapeHashes, s.escapeStart = false, 0, 0
				if s.mode == swiftString && c == '(' {
					w.add(start, in.Offset+i+1, KindDelimiter, ClassDelimiter)
					s.frames = &swiftFrame{parent: s.frames, literal: s.literal, parens: 1}
					s.mode, s.literal, s.expect = swiftCode, swiftLiteral{}, true
					i++
					continue
				}
				w.add(start, in.Offset+i+n, KindEscape, ClassString)
				i += n
				continue
			}
			if c == '\\' {
				s.quotes = 0
				s.escaping, s.escapeStart = true, in.Offset+i
				kind := KindString
				if s.mode == swiftRegex || s.literal.hashes == 0 {
					kind = KindEscape
				}
				add(i, i+1, kind, ClassString)
				i++
				continue
			}
			if s.mode == swiftRegex {
				if c == '[' {
					s.regexClass = true
				}
				if c == ']' {
					s.regexClass = false
				}
				if c == '/' && !s.regexClass {
					add(i, i+1, KindString, ClassString)
					i++
					if s.literal.hashes == 0 {
						endLiteral(true)
					} else {
						s.closing = 1
					}
					continue
				}
			} else if c == '"' {
				s.quotes++
				add(i, i+1, KindString, ClassString)
				i++
				needed := 1
				if s.literal.triple {
					needed = 3
				}
				if s.quotes == needed {
					if s.literal.hashes == 0 {
						endLiteral(true)
					} else {
						s.closing = 1
					}
				}
				continue
			} else {
				s.quotes = 0
			}
			add(i, i+n, KindString, ClassString)
			i += n
			continue
		case swiftNumber:
			if IdentifierRune(r) || c == '.' && i+1 < len(text) && (digit(text[i+1]) || s.hexNumber && hex(text[i+1])) && !strings.HasPrefix(text[i:], "..") || (c == '+' || c == '-') && (s.last == 'p' || s.last == 'P' || !s.hexNumber && (s.last == 'e' || s.last == 'E')) {
				if c == 'x' || c == 'X' {
					s.hexNumber = true
				}
				s.last = c
				add(i, i+n, KindNumber, 0)
				i += n
				continue
			}
			s.mode, s.last, s.hexNumber, s.expect = swiftCode, 0, false, false
		}
		if s.wordOn {
			if IdentifierRune(r) {
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
			s.mode = swiftLineComment
			continue
		}
		if strings.HasPrefix(text[i:], "/*") {
			folds.openComment()
			s.mode, s.depth = swiftBlockComment, 1
			add(i, i+2, KindComment, ClassComment)
			i += 2
			continue
		}
		if c == '#' {
			s.mode, s.hashes, s.prefixStart = swiftPrefix, 1, in.Offset+i
			add(i, i+1, KindDelimiter, 0)
			i++
			continue
		}
		if c == '"' {
			startLiteral(false)
			continue
		}
		if c == '/' && s.expect && i+1 < len(text) && !space(text[i+1]) && text[i+1] != '=' {
			startLiteral(true)
			continue
		}
		if c == '`' {
			s.mode = swiftBacktick
			add(i, i+1, KindVariable, ClassString)
			i++
			continue
		}
		if digit(c) {
			s.mode, s.last = swiftNumber, c
			add(i, i+1, KindNumber, 0)
			i++
			continue
		}
		if IdentifierRune(r) || c == '@' || c == '$' {
			kind := KindPlain
			if c == '@' {
				kind = KindMeta
			}
			if c == '$' {
				kind = KindVariable
			}
			s.word, s.wordOn = scanWord{start: in.Offset + i, kind: kind}, true
			s.word.add(text[i : i+n])
			i += n
			continue
		}
		if f := s.frames; f != nil && (c == '(' || c == ')') {
			next := *f
			if c == '(' {
				next.parens++
			} else {
				next.parens--
			}
			if next.parens == 0 {
				s.mode, s.literal, s.frames = swiftString, f.literal, f.parent
			} else {
				s.frames = &next
			}
		}
		class := Class(0)
		if strings.ContainsRune("()[]{}", r) {
			class = ClassDelimiter
		}
		if s.mode == swiftCode {
			s.expect = !strings.ContainsRune(")]}", r) && c != '.'
		}
		add(i, i+n, KindDelimiter, class)
		i += n
	}
	if in.LineEnd && i == len(text) {
		finishWord(in.Offset + i)
		if s.mode == swiftString && !s.literal.triple || s.mode == swiftRegex && s.literal.hashes == 0 {
			endLiteral(false)
		}
		if s.mode == swiftLineComment || s.mode == swiftNumber || s.mode == swiftBacktick || s.mode == swiftPrefix {
			s.mode = swiftCode
		}
		s.hashes, s.prefixStart, s.quotes, s.closing, s.escapeHashes, s.escapeStart, s.escaping, s.last, s.hexNumber = 0, 0, 0, 0, 0, 0, false, 0, false
	}
	return w.result(i, s), nil
}
