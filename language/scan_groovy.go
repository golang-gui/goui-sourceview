package language

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

type grMode uint8

const (
	grCode grMode = iota
	grString
	grVariable
	grLineComment
	grBlockComment
	grNumber
)

type grLiteral struct {
	quote                              byte
	triple, interpolate, slash, dollar bool
}
type grExpr struct {
	parent  *grExpr
	literal grLiteral
	depth   int
}
type grState struct {
	mode                                                          grMode
	literal                                                       grLiteral
	expr                                                          *grExpr
	word                                                          scanWord
	wordOn, member, expect, escape, hexNumber, octal, unicodeLead bool
	escapeLeft                                                    int
	last                                                          byte
}

func (s grState) Equal(other State) bool {
	t, ok := other.(grState)
	if !ok {
		return false
	}
	for a, b := s.expr, t.expr; a != b; a, b = a.parent, b.parent {
		if a == nil || b == nil || a.literal != b.literal || a.depth != b.depth {
			return false
		}
	}
	s.expr, t.expr = nil, nil
	return s == t
}
func (s grState) Context() Context {
	return Context{String: s.mode == grString || s.mode == grVariable, Literal: s.literal.slash || s.literal.dollar, Comment: s.mode == grLineComment || s.mode == grBlockComment}
}

type grTokenizer struct{}

func (grTokenizer) InitialState() State { return grState{expect: true} }

var grKeywords = words("abstract as assert boolean break byte case catch char class const continue def default do double else enum extends final finally float for goto if implements import in instanceof int interface long native new package private protected public record return sealed permits short static strictfp super switch synchronized this threadsafe throw throws trait transient try var void volatile while yield true false null")

func grNameStart(r rune) bool { return IdentifierRune(r) && !(r >= '0' && r <= '9') }
func (grTokenizer) Scan(in Input, state State) (Result, error) {
	s, ok := state.(grState)
	if !ok {
		return Result{}, fmt.Errorf("language: foreign Groovy state")
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
		if kind != KindMeta && !s.member && grKeywords[s.word.text] {
			kind = KindKeyword
		}
		w.add(s.word.start, end, kind, 0)
		s.expect = kind == KindKeyword && !strings.Contains(" true false null this super ", " "+s.word.text+" ")
		s.word, s.wordOn, s.member = scanWord{}, false, false
	}
	endString := func() {
		s.mode, s.literal, s.expect, s.member = grCode, grLiteral{}, false, false
		s.escape, s.escapeLeft, s.octal, s.unicodeLead = false, 0, false, false
	}
	for i < limit {
		c := text[i]
		r, n := utf8.DecodeRuneInString(text[i:])
		switch s.mode {
		case grLineComment:
			add(i, i+n, KindComment, ClassComment)
			i += n
			continue
		case grBlockComment:
			if strings.HasPrefix(text[i:], "*/") {
				add(i, i+2, KindComment, ClassComment)
				i += 2
				s.mode = grCode
				continue
			}
			add(i, i+n, KindComment, ClassComment)
			i += n
			continue
		case grVariable:
			if IdentifierRune(r) {
				add(i, i+n, KindVariable, ClassString)
				i += n
				continue
			}
			if c == '.' && i+1 < len(text) {
				next, _ := utf8.DecodeRuneInString(text[i+1:])
				if grNameStart(next) {
					add(i, i+1, KindVariable, ClassString)
					i++
					continue
				}
			}
			s.mode = grString
			continue
		case grString:
			if s.escapeLeft > 0 {
				valid := hex(c)
				if s.octal {
					valid = c >= '0' && c <= '7'
				}
				if s.unicodeLead && c == 'u' {
					add(i, i+1, KindEscape, ClassString)
					i++
					continue
				}
				s.unicodeLead = false
				if valid {
					s.escapeLeft--
					add(i, i+1, KindEscape, ClassString)
					i++
					continue
				}
				s.escapeLeft, s.octal = 0, false
			}
			if s.escape {
				s.escape = false
				s.octal, s.unicodeLead = false, false
				if c == 'u' {
					s.escapeLeft, s.unicodeLead = 4, true
				} else if c >= '0' && c <= '7' {
					s.escapeLeft, s.octal = 2, true
				}
				add(i, i+n, KindEscape, ClassString)
				i += n
				continue
			}
			if s.literal.dollar {
				if strings.HasPrefix(text[i:], "$$") || strings.HasPrefix(text[i:], "$/") {
					add(i, i+2, KindEscape, ClassString)
					i += 2
					continue
				}
				if strings.HasPrefix(text[i:], "/$") {
					add(i, i+2, KindString, ClassString)
					i += 2
					endString()
					continue
				}
			} else if s.literal.slash {
				if strings.HasPrefix(text[i:], `\/`) {
					add(i, i+2, KindEscape, ClassString)
					i += 2
					continue
				}
				if c == '/' {
					add(i, i+1, KindString, ClassString)
					i++
					endString()
					continue
				}
			} else {
				if c == '\\' {
					s.escape = true
					add(i, i+1, KindEscape, ClassString)
					i++
					continue
				}
				if c == s.literal.quote {
					size := 1
					if s.literal.triple {
						count := 0
						for j := i; j < len(text) && text[j] == c && count < 6; j++ {
							count++
						}
						if count < 3 {
							add(i, i+1, KindString, ClassString)
							i++
							continue
						}
						size = 3
						if count < 6 {
							size = count
						}
					}
					add(i, i+size, KindString, ClassString)
					i += size
					endString()
					continue
				}
			}
			if s.literal.interpolate && c == '$' && i+1 < len(text) {
				if text[i+1] == '{' {
					s.expr = &grExpr{parent: s.expr, literal: s.literal}
					s.mode, s.literal, s.expect = grCode, grLiteral{}, true
					add(i, i+2, KindDelimiter, ClassDelimiter)
					i += 2
					continue
				}
				next, _ := utf8.DecodeRuneInString(text[i+1:])
				if grNameStart(next) {
					s.mode = grVariable
					add(i, i+1, KindVariable, ClassString)
					i++
					continue
				}
			}
			add(i, i+n, KindString, ClassString)
			i += n
			continue
		case grNumber:
			if IdentifierRune(r) || c == '.' && i+1 < len(text) && !strings.HasPrefix(text[i:], "..") && (digit(text[i+1]) || s.hexNumber && hex(text[i+1])) || (c == '+' || c == '-') && (s.last == 'p' || s.last == 'P' || !s.hexNumber && (s.last == 'e' || s.last == 'E')) {
				if c == 'x' || c == 'X' {
					s.hexNumber = true
				}
				s.last = c
				add(i, i+n, KindNumber, 0)
				i += n
				continue
			}
			s.mode, s.last, s.hexNumber, s.expect = grCode, 0, false, false
		}
		if s.wordOn {
			if IdentifierRune(r) || c == '$' || s.word.kind == KindMeta && c == '.' {
				s.word.add(text[i : i+n])
				i += n
				continue
			}
			finishWord(in.Offset + i)
			continue
		}
		if space(c) {
			i += n
			continue
		}
		if strings.HasPrefix(text[i:], "//") || in.Offset+i == 0 && strings.HasPrefix(text[i:], "#!") {
			s.mode = grLineComment
			continue
		}
		if strings.HasPrefix(text[i:], "/*") {
			s.mode = grBlockComment
			add(i, i+2, KindComment, ClassComment)
			i += 2
			continue
		}
		if strings.HasPrefix(text[i:], "$/") {
			s.mode, s.literal = grString, grLiteral{dollar: true, interpolate: true}
			add(i, i+2, KindString, ClassString)
			i += 2
			continue
		}
		if c == '"' || c == '\'' || c == '/' && s.expect {
			s.literal = grLiteral{quote: c, slash: c == '/', interpolate: c != '\''}
			s.literal.triple = c != '/' && strings.HasPrefix(text[i:], strings.Repeat(string(c), 3))
			size := 1
			if s.literal.triple {
				size = 3
			}
			s.mode = grString
			add(i, i+size, KindString, ClassString)
			i += size
			continue
		}
		if digit(c) || c == '.' && i+1 < len(text) && digit(text[i+1]) {
			s.mode, s.last = grNumber, c
			add(i, i+1, KindNumber, 0)
			i++
			continue
		}
		if grNameStart(r) || c == '$' || c == '@' {
			kind := KindPlain
			if c == '@' {
				kind = KindMeta
			}
			s.word, s.wordOn = scanWord{start: in.Offset + i, kind: kind}, true
			s.word.add(text[i : i+n])
			i += n
			continue
		}
		if c == '}' && s.expr != nil && s.expr.depth == 0 {
			f := s.expr
			s.mode, s.literal, s.expr = grString, f.literal, f.parent
			add(i, i+1, KindDelimiter, ClassDelimiter)
			i++
			continue
		}
		if s.expr != nil && (c == '{' || c == '}') {
			f := *s.expr
			if c == '{' {
				f.depth++
			} else {
				f.depth--
			}
			s.expr = &f
		}
		if strings.HasPrefix(text[i:], "..") || (c == '+' || c == '-') && i+1 < len(text) && text[i+1] == c {
			add(i, i+2, KindDelimiter, 0)
			i += 2
			if c == '.' {
				s.expect = true
			}
			s.member = false
			continue
		}
		class := Class(0)
		if strings.ContainsRune("()[]{}", r) {
			class = ClassDelimiter
		}
		add(i, i+n, KindDelimiter, class)
		i += n
		s.expect = !strings.ContainsRune(")]}", r)
		s.member = c == '.'
	}
	if in.LineEnd && i == len(text) {
		finishWord(in.Offset + i)
		if s.mode == grVariable {
			s.mode = grString
		}
		if s.mode == grString && !s.literal.triple && !s.literal.slash && !s.literal.dollar && !s.escape {
			endString()
		}
		if s.mode == grLineComment || s.mode == grNumber {
			s.mode = grCode
		}
		if s.mode == grCode {
			s.expect = true
		}
		s.escape, s.escapeLeft, s.octal, s.unicodeLead, s.hexNumber, s.last, s.member = false, 0, false, false, false, 0, false
	}
	return w.result(i, s), nil
}
