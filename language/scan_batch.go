package language

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

type batMode uint8

const (
	batCode batMode = iota
	batQuoted
	batComment
	batLabel
	batVariable
	batModified
)

type batState struct {
	mode, back                       batMode
	word                             scanWord
	wordOn, command, leading, escape bool
	variable                         byte
	stage                            int
}

func (s batState) Equal(other State) bool { t, ok := other.(batState); return ok && s == t }
func (s batState) Context() Context {
	m := s.mode
	if m == batVariable || m == batModified {
		m = s.back
	}
	return Context{String: m == batQuoted || m == batLabel, Literal: m == batQuoted || m == batLabel, Comment: m == batComment}
}

type batTokenizer struct{}

func (batTokenizer) InitialState() State { return batState{command: true, leading: true} }

var batKeywords = words("assoc break call cd chcp chdir cls color copy date del dir do echo else endlocal equ erase exist exit for ftype geq goto gtr if in leq lss md mkdir mklink move neq not path pause popd prompt pushd rd rem ren rename rmdir set setlocal shift start time title type ver verify vol defined errorlevel cmdextversion enabledelayedexpansion disabledelayedexpansion")

func (batTokenizer) Scan(in Input, state State) (Result, error) {
	s, ok := state.(batState)
	if !ok {
		return Result{}, fmt.Errorf("language: foreign Batch state")
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
		name := strings.ToLower(s.word.text)
		kind, class := KindPlain, Class(0)
		if batKeywords[name] && (name != "rem" || s.command) {
			kind = KindKeyword
		}
		if name == "rem" && s.command {
			s.mode = batComment
			kind, class = KindComment, ClassComment
		}
		w.add(s.word.start, end, kind, class)
		s.command = name == "call" || name == "do" || name == "else"
		s.leading = false
		s.word, s.wordOn = scanWord{}, false
	}
	for i < limit {
		c := text[i]
		r, n := utf8.DecodeRuneInString(text[i:])
		switch s.mode {
		case batComment:
			add(i, i+n, KindComment, ClassComment)
			i += n
			continue
		case batLabel:
			add(i, i+n, KindTag, ClassString)
			i += n
			continue
		case batVariable, batModified:
			class := Class(0)
			if s.back == batQuoted {
				class = ClassString
			}
			if s.mode == batModified {
				if IdentifierRune(r) || strings.ContainsRune("~$:", r) {
					add(i, i+n, KindVariable, class)
					i += n
					if digit(c) {
						s.mode = s.back
					}
					continue
				}
				s.mode = s.back
				continue
			}
			if s.stage == 0 && s.variable == '%' {
				s.stage = 1
				if c == '%' {
					add(i, i+1, KindVariable, class)
					i++
					s.stage = 2
					continue
				}
				if digit(c) || c == '*' {
					add(i, i+1, KindVariable, class)
					i++
					s.mode = s.back
					continue
				}
				if c == '~' {
					s.mode = batModified
					continue
				}
			} else if s.stage == 2 {
				if c == '~' {
					s.mode = batModified
					continue
				}
				if IdentifierRune(r) {
					add(i, i+n, KindVariable, class)
					i += n
				}
				s.mode = s.back
				continue
			}
			if c == s.variable {
				add(i, i+1, KindVariable, class)
				i++
				s.mode = s.back
				continue
			}
			if c == '"' || c == '^' || strings.ContainsRune("&|<>()", r) {
				s.mode = s.back
				continue
			}
			add(i, i+n, KindVariable, class)
			i += n
			continue
		}
		if s.wordOn {
			if IdentifierRune(r) || c == '-' {
				s.word.add(text[i : i+n])
				i += n
				continue
			}
			finishWord(in.Offset + i)
			continue
		}
		if s.escape {
			s.escape = false
			add(i, i+n, KindEscape, 0)
			i += n
			s.leading = false
			continue
		}
		if c == '%' || c == '!' {
			s.back, s.mode, s.variable, s.stage = s.mode, batVariable, c, 0
			class := Class(0)
			if s.back == batQuoted {
				class = ClassString
			}
			add(i, i+1, KindVariable, class)
			i++
			continue
		}
		if c == '"' {
			if s.mode == batQuoted {
				s.mode = batCode
			} else {
				s.mode = batQuoted
			}
			add(i, i+1, KindString, ClassString)
			i++
			s.leading = false
			continue
		}
		if s.mode == batQuoted {
			add(i, i+n, KindString, ClassString)
			i += n
			continue
		}
		if c == '^' {
			s.escape = true
			add(i, i+1, KindEscape, 0)
			i++
			continue
		}
		if c == ' ' || c == '\t' {
			i++
			continue
		}
		if s.leading && c == ':' {
			s.mode = batLabel
			if strings.HasPrefix(text[i:], "::") {
				s.mode = batComment
			}
			continue
		}
		if c == '@' && s.command {
			add(i, i+1, KindMeta, 0)
			i++
			continue
		}
		if IdentifierRune(r) && !digit(c) {
			s.word, s.wordOn = scanWord{start: in.Offset + i}, true
			s.word.add(text[i : i+n])
			i += n
			continue
		}
		class := Class(0)
		kind := KindPlain
		if strings.ContainsRune("&|<>=()+-*/", r) {
			kind = KindDelimiter
		}
		if c == '(' || c == ')' {
			class = ClassDelimiter
		}
		if digit(c) {
			kind = KindNumber
		}
		if c == '&' || c == '|' || c == '(' {
			s.command = true
		} else if c == ')' {
			s.command = false
		}
		add(i, i+n, kind, class)
		i += n
		s.leading = false
	}
	if in.LineEnd && i == len(text) {
		finishWord(in.Offset + i)
		if s.mode == batVariable || s.mode == batModified {
			s.mode = s.back
		}
		if !s.escape {
			s.command, s.leading = true, true
			s.mode = batCode
		}
		s.escape, s.back, s.variable, s.stage = false, batCode, 0, 0
	}
	return w.result(i, s), nil
}
