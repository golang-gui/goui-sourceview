package language

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

type rMode uint8

const (
	rCode rMode = iota
	rString
	rComment
	rRawPrefix
	rRawBody
	rNumber
	rOperator
)

type rState struct {
	mode                                                  rMode
	quote, rawClose, last                                 byte
	rawDashes, closingDashes, rawStart, escapeLeft        int
	escape, escapeOctal, escapeBraces, closing, hexNumber bool
	word                                                  scanWord
	wordOn                                                bool
}

func (s rState) Equal(other State) bool { t, ok := other.(rState); return ok && s == t }
func (s rState) Context() Context {
	return Context{String: s.mode == rString || s.mode == rRawPrefix || s.mode == rRawBody, Literal: s.mode == rRawBody, Comment: s.mode == rComment}
}

type rTokenizer struct{}

func (rTokenizer) InitialState() State { return rState{} }

var rKeywords = words("if else repeat while function for in next break TRUE FALSE NULL Inf NaN NA NA_integer_ NA_real_ NA_complex_ NA_character_")

func rRawQuotePrefix(before string) bool {
	if len(before) == 0 || before[len(before)-1] != 'r' && before[len(before)-1] != 'R' {
		return false
	}
	if len(before) == 1 {
		return true
	}
	prev, _ := utf8.DecodeLastRuneInString(before[:len(before)-1])
	return !IdentifierRune(prev) && prev != '.'
}

func (t rTokenizer) Scan(in Input, state State) (Result, error) {
	return t.scan(in, state, nil)
}

func (rTokenizer) scan(in Input, state State, folds *foldLexemes) (Result, error) {
	s, ok := state.(rState)
	if !ok {
		return Result{}, fmt.Errorf("language: foreign R state")
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
		if rKeywords[s.word.text] {
			kind = KindKeyword
		}
		w.add(s.word.start, end, kind, 0)
		s.word, s.wordOn = scanWord{}, false
	}
	endLiteral := func() {
		s.mode, s.quote, s.rawClose, s.rawDashes, s.closingDashes, s.rawStart, s.escapeLeft = rCode, 0, 0, 0, 0, 0, 0
		s.escape, s.escapeOctal, s.escapeBraces, s.closing = false, false, false, false
	}
	for i < limit {
		c := text[i]
		r, n := utf8.DecodeRuneInString(text[i:])
		switch s.mode {
		case rComment:
			add(i, i+n, KindComment, ClassComment)
			i += n
			continue
		case rOperator:
			add(i, i+n, KindDelimiter, 0)
			i += n
			if c == '%' {
				s.mode = rCode
			}
			continue
		case rRawPrefix:
			if c == '-' {
				s.rawDashes++
				add(i, i+1, KindString, ClassString)
				i++
				continue
			}
			if strings.ContainsRune("([{", r) {
				s.rawClose = map[byte]byte{'(': ')', '[': ']', '{': '}'}[c]
				w.add(s.rawStart, in.Offset+i+1, KindString, ClassString)
				s.mode, s.rawStart = rRawBody, 0
				i++
				continue
			}
			s.mode, s.rawDashes, s.rawStart = rString, 0, 0
			continue
		case rRawBody:
			if s.closing {
				if c == '-' && s.closingDashes < s.rawDashes {
					s.closingDashes++
					add(i, i+1, KindString, ClassString)
					i++
					continue
				}
				if c == s.quote && s.closingDashes == s.rawDashes {
					add(i, i+1, KindString, ClassString)
					i++
					folds.closeLiteral(in.Offset+i, true)
					endLiteral()
					continue
				}
				s.closing, s.closingDashes = false, 0
			}
			if c == s.rawClose {
				s.closing = true
			}
			add(i, i+n, KindString, ClassString)
			i += n
			continue
		case rString:
			kind := KindString
			if s.quote == '`' {
				kind = KindVariable
			}
			if s.escapeLeft > 0 || s.escapeBraces {
				valid := hex(c)
				if s.escapeOctal {
					valid = c >= '0' && c <= '7'
				}
				if s.escapeLeft > 0 && valid {
					s.escapeLeft--
					add(i, i+1, KindEscape, ClassString)
					i++
					continue
				}
				braced := s.escapeBraces
				s.escapeLeft, s.escapeBraces, s.escapeOctal = 0, false, false
				if braced && c == '}' {
					add(i, i+1, KindEscape, ClassString)
					i++
					continue
				}
			}
			if s.escape {
				s.escape = false
				switch c {
				case 'x':
					s.escapeLeft = 2
				case 'u', 'U':
					s.escapeLeft = 4
					if c == 'U' {
						s.escapeLeft = 8
					}
					if i+1 < len(text) && text[i+1] == '{' {
						s.escapeBraces = true
						add(i, i+2, KindEscape, ClassString)
						i += 2
						continue
					}
				default:
					if c >= '0' && c <= '7' {
						s.escapeLeft, s.escapeOctal = 2, true
					}
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
			add(i, i+n, kind, ClassString)
			i += n
			if c == s.quote {
				folds.closeLiteral(in.Offset+i, true)
				endLiteral()
			}
			continue
		case rNumber:
			if IdentifierRune(r) || c == '.' || (c == '+' || c == '-') && (s.last == 'p' || s.last == 'P' || !s.hexNumber && (s.last == 'e' || s.last == 'E')) {
				if c == 'x' || c == 'X' {
					s.hexNumber = true
				}
				s.last = c
				add(i, i+n, KindNumber, 0)
				i += n
				continue
			}
			s.mode, s.last, s.hexNumber = rCode, 0, false
		}
		if s.wordOn {
			if (c == '"' || c == '\'') && (s.word.text == "r" || s.word.text == "R") && s.word.length == 1 {
				folds.openLiteral()
				finishWord(in.Offset + i)
				s.mode, s.quote, s.rawStart = rRawPrefix, c, in.Offset+i-1
				add(i, i+1, KindString, ClassString)
				i++
				continue
			}
			if IdentifierRune(r) || c == '.' {
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
		if c == '#' {
			s.mode = rComment
			continue
		}
		if c == '%' {
			s.mode = rOperator
			add(i, i+1, KindDelimiter, 0)
			i++
			continue
		}
		if c == '"' || c == '\'' || c == '`' {
			folds.openLiteral()
			s.mode, s.quote = rString, c
			kind := KindString
			if c == '`' {
				kind = KindVariable
			}
			add(i, i+1, kind, ClassString)
			i++
			continue
		}
		if digit(c) || c == '.' && i+1 < len(text) && digit(text[i+1]) {
			s.mode, s.last = rNumber, c
			add(i, i+1, KindNumber, 0)
			i++
			continue
		}
		if c == '.' || codeNameStart(R, r) && c != '_' {
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
		if s.mode == rRawPrefix {
			s.mode, s.rawStart, s.rawDashes = rString, 0, 0
		}
		if s.mode == rComment || s.mode == rNumber || s.mode == rOperator {
			s.mode = rCode
		}
		s.escape, s.escapeLeft, s.escapeOctal, s.escapeBraces, s.last, s.hexNumber, s.closing, s.closingDashes = false, 0, false, false, 0, false, false, 0
	}
	return w.result(i, s), nil
}
