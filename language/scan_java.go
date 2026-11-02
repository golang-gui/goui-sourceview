package language

import (
	"fmt"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

type javaMode uint8

const (
	javaNormal javaMode = iota
	javaSlash
	javaLineComment
	javaBlockComment
	javaQuoteOpen
	javaQuoted
	javaTextBlock
	javaNumber
	javaDot
)

type javaState struct {
	mode               javaMode
	quote              rune
	escape, star       bool
	quotes             int
	pendingStart       int
	word               scanWord
	wordOn, annotation bool
	last               rune
	// Unicode translation precedes lexing, including in comments. Only the
	// source range and four hex digits are retained, even for many 'u' markers.
	unicodeStage, digits, unicodeStart, unicodeEnd int
	value                                          rune
	oddBackslashes, translated                     bool
	high                                           rune
	highStart, highEnd                             int
}

func (s javaState) Equal(other State) bool { t, ok := other.(javaState); return ok && s == t }
func (s javaState) Context() Context {
	return Context{String: s.mode == javaQuoteOpen && s.quotes == 1 || s.mode == javaQuoted || s.mode == javaTextBlock, Comment: s.mode == javaLineComment || s.mode == javaBlockComment}
}

type javaTokenizer struct{}

func (javaTokenizer) InitialState() State { return javaState{} }

var javaKeywords = words("abstract assert boolean break byte case catch char class const continue default do double else enum extends final finally float for goto if implements import instanceof int interface long native new package private protected public return short static strictfp super switch synchronized this throw throws transient try void volatile while true false null _ exports module open opens permits provides record requires sealed to transitive uses var when with yield")

func hexValue(r rune) rune {
	if r >= '0' && r <= '9' {
		return r - '0'
	}
	if r >= 'a' && r <= 'f' {
		return r - 'a' + 10
	}
	if r >= 'A' && r <= 'F' {
		return r - 'A' + 10
	}
	return -1
}

func (javaTokenizer) Scan(in Input, state State) (Result, error) {
	s, ok := state.(javaState)
	if !ok {
		return Result{}, fmt.Errorf("language: foreign Java state")
	}
	limit, err := scanLimit(in)
	if err != nil {
		return Result{}, err
	}
	w := tokenWriter{base: in.Offset}
	finishWord := func(end int) {
		if !s.wordOn {
			return
		}
		kind := KindPlain
		if s.annotation {
			kind = KindMeta
		} else if javaKeywords[s.word.text] {
			kind = KindKeyword
		}
		w.add(s.word.start, end, kind, 0)
		s.word, s.wordOn, s.annotation = scanWord{}, false, false
	}
	lineEnd := func(end int) {
		finishWord(end)
		if s.mode != javaBlockComment && s.mode != javaTextBlock {
			s.mode, s.quote = javaNormal, 0
		}
		s.escape, s.star, s.quotes, s.pendingStart, s.last = false, false, 0, 0, 0
	}
	lex := func(r rune, start, end int) {
		for {
			if r == '\r' || r == '\n' {
				lineEnd(start)
				return
			}
			switch s.mode {
			case javaDot:
				if r >= '0' && r <= '9' {
					w.add(s.pendingStart, end, KindNumber, 0)
					s.mode, s.last, s.pendingStart = javaNumber, r, 0
					return
				}
				s.mode, s.pendingStart = javaNormal, 0
				continue
			case javaLineComment:
				w.add(start, end, KindComment, ClassComment)
				return
			case javaBlockComment:
				w.add(start, end, KindComment, ClassComment)
				if s.star && r == '/' {
					s.mode, s.star = javaNormal, false
				} else {
					s.star = r == '*'
				}
				return
			case javaSlash:
				if r == '/' || r == '*' {
					w.add(s.pendingStart, end, KindComment, ClassComment)
					s.mode = javaLineComment
					if r == '*' {
						s.mode = javaBlockComment
					}
					s.pendingStart, s.star = 0, false
					return
				}
				s.mode, s.pendingStart = javaNormal, 0
				continue
			case javaQuoteOpen:
				if r == '"' {
					s.quotes++
					w.add(start, end, KindString, ClassString)
					if s.quotes == 3 {
						s.mode, s.quotes = javaTextBlock, 0
					}
					return
				}
				if s.quotes == 2 {
					s.mode = javaNormal
				} else {
					s.mode = javaQuoted
				}
				s.quotes = 0
				continue
			case javaQuoted, javaTextBlock:
				kind := KindString
				if s.escape {
					kind, s.escape, s.quotes = KindEscape, false, 0
				} else if r == '\\' {
					kind, s.escape, s.quotes = KindEscape, true, 0
				} else if r == s.quote {
					if s.mode == javaQuoted {
						s.mode, s.quote = javaNormal, 0
					} else {
						s.quotes++
						if s.quotes == 3 {
							s.mode, s.quote, s.quotes = javaNormal, 0, 0
						}
					}
				} else {
					s.quotes = 0
				}
				w.add(start, end, kind, ClassString)
				return
			case javaNumber:
				if IdentifierRune(r) || r == '.' || (r == '+' || r == '-') && strings.ContainsRune("eEpP", s.last) {
					w.add(start, end, KindNumber, 0)
					s.last = r
					return
				}
				s.mode, s.last = javaNormal, 0
			}
			if s.wordOn {
				if IdentifierRune(r) || r == '$' {
					s.word.add(string(r))
					return
				}
				finishWord(start)
			}
			if r == ' ' || r == '\t' || r == '\f' {
				s.annotation = false
				return
			}
			if IdentifierRune(r) && !(r >= '0' && r <= '9') || r == '$' {
				wordStart := start
				if s.annotation {
					wordStart = s.pendingStart
				}
				s.pendingStart = 0
				s.word, s.wordOn = scanWord{start: wordStart}, true
				s.word.add(string(r))
				return
			}
			s.annotation = false
			switch r {
			case '.':
				s.mode, s.pendingStart = javaDot, start
				w.add(start, end, KindDelimiter, 0)
			case '@':
				s.annotation, s.pendingStart = true, start
				w.add(start, end, KindMeta, 0)
			case '/':
				s.mode, s.pendingStart = javaSlash, start
				w.add(start, end, KindDelimiter, 0)
			case '"':
				s.mode, s.quote, s.quotes = javaQuoteOpen, r, 1
				w.add(start, end, KindString, ClassString)
			case '\'':
				s.mode, s.quote = javaQuoted, r
				w.add(start, end, KindString, ClassString)
			default:
				if r >= '0' && r <= '9' {
					s.mode, s.last = javaNumber, r
					w.add(start, end, KindNumber, 0)
					return
				}
				class := Class(0)
				if strings.ContainsRune("()[]{}", r) {
					class = ClassDelimiter
				}
				w.add(start, end, KindDelimiter, class)
			}
			return
		}
	}
	emit := func(r rune, start, end int, translated bool) {
		if r == '\\' {
			s.oddBackslashes = !s.oddBackslashes
		} else {
			s.oddBackslashes = false
		}
		s.translated = translated
		if s.high != 0 {
			if r >= 0xdc00 && r <= 0xdfff {
				r, start = utf16.DecodeRune(s.high, r), s.highStart
				s.high, s.highStart, s.highEnd = 0, 0, 0
			} else {
				lex(utf8.RuneError, s.highStart, s.highEnd)
				s.high, s.highStart, s.highEnd = 0, 0, 0
			}
		}
		if r >= 0xd800 && r <= 0xdbff {
			s.high, s.highStart, s.highEnd = r, start, end
			return
		}
		lex(r, start, end)
	}
	i := 0
	for i < limit {
		r, n := utf8.DecodeRuneInString(in.Text[i:])
		start, end := in.Offset+i, in.Offset+i+n
		if s.unicodeStage == 1 {
			if r == 'u' {
				s.unicodeStage, s.unicodeEnd = 2, end
				i += n
				continue
			}
			emit('\\', s.unicodeStart, s.unicodeEnd, false)
			s.unicodeStage, s.unicodeStart, s.unicodeEnd = 0, 0, 0
		}
		if s.unicodeStage == 2 {
			if r == 'u' && s.digits == 0 {
				s.unicodeEnd = end
				i += n
				continue
			}
			if value := hexValue(r); value >= 0 {
				s.value = s.value*16 + value
				s.digits++
				s.unicodeEnd = end
				i += n
				if s.digits == 4 {
					emit(s.value, s.unicodeStart, end, true)
					s.unicodeStage, s.digits, s.unicodeStart, s.unicodeEnd, s.value = 0, 0, 0, 0, 0
				}
				continue
			}
			// Invalid Unicode escapes do not manufacture a quote or comment.
			lex(utf8.RuneError, s.unicodeStart, s.unicodeEnd)
			s.unicodeStage, s.digits, s.unicodeStart, s.unicodeEnd, s.value = 0, 0, 0, 0, 0
		}
		if r == '\\' && (s.translated || !s.oddBackslashes) {
			s.unicodeStage, s.unicodeStart, s.unicodeEnd = 1, start, end
		} else {
			emit(r, start, end, false)
		}
		i += n
	}
	if in.LineEnd && i == len(in.Text) {
		if s.unicodeStage != 0 {
			lex('\\', s.unicodeStart, s.unicodeEnd)
		}
		if s.high != 0 {
			lex(utf8.RuneError, s.highStart, s.highEnd)
		}
		lineEnd(in.Offset + i)
		s.unicodeStage, s.digits, s.unicodeStart, s.unicodeEnd, s.value = 0, 0, 0, 0, 0
		s.high, s.highStart, s.highEnd, s.oddBackslashes, s.translated = 0, 0, 0, false, false
	}
	return w.result(i, s), nil
}
