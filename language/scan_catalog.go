package language

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

type poTokenizer struct{}
type poState struct {
	quoted, first, comment, special, wordOn bool
	escape                                  byte
	left                                    int
	word                                    scanWord
}

func (s poState) Equal(other State) bool { t, ok := other.(poState); return ok && s == t }
func (s poState) Context() Context       { return Context{String: s.quoted, Comment: s.comment} }
func (poTokenizer) InitialState() State  { return poState{first: true} }

var poWords = []string{"msgctxt", "msgid", "msgid_plural", "msgstr"}

func (poTokenizer) Scan(in Input, previous State) (Result, error) {
	s, ok := previous.(poState)
	if !ok {
		return Result{}, fmt.Errorf("language: foreign PO state")
	}
	limit, err := scanLimit(in)
	if err != nil {
		return Result{}, err
	}
	w := tokenWriter{base: in.Offset}
	finish := func(end int) {
		kind := KindInvalid
		if slices.Contains(poWords, s.word.text) {
			kind = KindKeyword
		}
		w.add(s.word.start, end, kind, 0)
		s.word, s.wordOn = scanWord{}, false
	}
	i := 0
	for i < limit {
		c := in.Text[i]
		r, n := utf8.DecodeRuneInString(in.Text[i:])
		at := in.Offset + i
		if s.first {
			if space(c) {
				i += n
				continue
			}
			s.first = false
			if c == '#' {
				s.comment = true
				if i+1 < len(in.Text) {
					s.special = strings.ContainsRune(":,|", rune(in.Text[i+1]))
				}
			}
		}
		if s.comment {
			kind := KindComment
			if s.special {
				kind = KindMeta
			}
			w.add(at, at+n, kind, ClassComment)
			i += n
			continue
		}
		if s.quoted {
			if s.escape == 2 || s.escape == 3 {
				valid := hex(c)
				if s.escape == 3 {
					valid = c >= '0' && c <= '7' && s.left > 0
				}
				if valid {
					w.add(at, at+n, KindEscape, ClassString)
					i += n
					s.left--
					continue
				}
				s.escape = 0
			}
			if s.escape == 1 {
				s.escape = 0
				kind := KindEscape
				if c >= '0' && c <= '7' {
					s.escape, s.left = 3, 2
				} else if c == 'x' {
					s.escape = 2
				} else if !strings.ContainsRune("abfnrtv\\\"", r) {
					kind = KindInvalid
				}
				w.add(at, at+n, kind, ClassString)
				i += n
				continue
			}
			kind := KindString
			if c == '\\' {
				s.escape, kind = 1, KindEscape
			} else if c == '"' {
				s.quoted = false
			}
			w.add(at, at+n, kind, ClassString)
			i += n
			continue
		}
		if s.wordOn {
			if IdentifierRune(r) {
				s.word.add(in.Text[i : i+n])
				i += n
				continue
			}
			finish(at)
			continue
		}
		switch {
		case c == '"':
			s.quoted = true
			w.add(at, at+n, KindString, ClassString)
		case c == '[' || c == ']':
			w.add(at, at+n, KindDelimiter, 0)
		case digit(c):
			w.add(at, at+n, KindNumber, 0)
		case IdentifierRune(r):
			s.word, s.wordOn = scanWord{start: at}, true
			s.word.add(in.Text[i : i+n])
		case !space(c):
			w.add(at, at+n, KindInvalid, 0)
		}
		i += n
	}
	if in.LineEnd && i == len(in.Text) {
		if s.wordOn {
			finish(in.Offset + i)
		}
		s = poState{first: true}
	}
	return w.result(i, s), nil
}
