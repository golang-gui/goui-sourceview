package language

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

type modState struct {
	id                             ID
	word                           scanWord
	wordOn, comment, escape, group bool
	quote                          byte
	field                          int
}

func (s modState) Equal(other State) bool { t, ok := other.(modState); return ok && s == t }
func (s modState) Context() Context {
	return Context{String: s.quote != 0 || s.id == GoSum, Literal: s.quote == '`' || s.id == GoSum, Comment: s.comment}
}

type modTokenizer struct{ id ID }

func (b modTokenizer) InitialState() State { return modState{id: b.id} }

var modKeywords = words("module go toolchain godebug require exclude replace retract tool ignore")
var workKeywords = words("go toolchain godebug use replace")

func modVersion(text string) bool {
	if strings.HasPrefix(text, "v") {
		text = text[1:]
	} else if strings.HasPrefix(text, "go") {
		text = text[2:]
	}
	if text == "" || !digit(text[0]) {
		return false
	}
	for _, c := range text {
		if !IdentifierRune(c) && !strings.ContainsRune(".+-", c) {
			return false
		}
	}
	return true
}
func (b modTokenizer) Scan(in Input, state State) (Result, error) {
	s, ok := state.(modState)
	if !ok || s.id != b.id {
		return Result{}, fmt.Errorf("language: foreign module state")
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
		kind := KindString
		if b.id == GoSum {
			if s.field == 1 {
				kind = KindNumber
			}
			if s.field >= 2 {
				kind = KindMeta
			}
		} else {
			if s.field > 0 && modVersion(s.word.text) {
				kind = KindNumber
			}
			if s.field == 0 && !s.group && (b.id == GoMod && modKeywords[s.word.text] || b.id == GoWork && workKeywords[s.word.text]) {
				kind = KindKeyword
			}
		}
		w.add(s.word.start, end, kind, 0)
		s.field = min(2, s.field+1)
		s.word, s.wordOn = scanWord{}, false
	}
	for i < limit {
		c := text[i]
		r, n := utf8.DecodeRuneInString(text[i:])
		if s.comment {
			add(i, i+n, KindComment, ClassComment)
			i += n
			continue
		}
		if s.quote != 0 {
			if s.escape {
				s.escape = false
				add(i, i+n, KindEscape, ClassString)
				i += n
				continue
			}
			if c == '\\' && s.quote == '"' {
				s.escape = true
				add(i, i+1, KindEscape, ClassString)
				i++
				continue
			}
			if c == s.quote {
				s.quote = 0
			}
			add(i, i+n, KindString, ClassString)
			i += n
			continue
		}
		boundary := space(c) || b.id != GoSum && (strings.ContainsRune("()[],", r) || c == '"' || c == '`' || strings.HasPrefix(text[i:], "//") || strings.HasPrefix(text[i:], "=>"))
		if s.wordOn {
			if !boundary {
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
		if b.id != GoSum {
			if strings.HasPrefix(text[i:], "//") {
				s.comment = true
				continue
			}
			if c == '"' || c == '`' {
				s.quote = c
				s.field = min(2, s.field+1)
				add(i, i+1, KindString, ClassString)
				i++
				continue
			}
			if strings.HasPrefix(text[i:], "=>") {
				add(i, i+2, KindDelimiter, 0)
				i += 2
				continue
			}
			if c == '(' || c == ')' {
				s.group = c == '('
				add(i, i+1, KindDelimiter, ClassDelimiter)
				i++
				continue
			}
			if strings.ContainsRune("[],", r) {
				class := Class(0)
				if c != ',' {
					class = ClassDelimiter
				}
				add(i, i+1, KindDelimiter, class)
				i++
				continue
			}
		}
		s.word, s.wordOn = scanWord{start: in.Offset + i}, true
		s.word.add(text[i : i+n])
		i += n
	}
	if in.LineEnd && i == len(text) {
		finishWord(in.Offset + i)
		s.comment, s.escape, s.quote, s.field = false, false, 0, 0
	}
	return w.result(i, s), nil
}
