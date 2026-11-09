package language

import (
	"fmt"
	"strings"
)

type adocTokenizer struct{}
type adocState struct {
	tail          bool
	block, marker byte
	width, run    int
	candidate     bool
	line, inline  Kind
	close         string
	escape        bool
}

func (s adocState) Equal(other State) bool { t, ok := other.(adocState); return ok && s == t }
func (s adocState) Context() Context {
	return Context{String: s.block != 0 && s.block != '/' && s.block != '_' || s.inline == KindCode,
		Literal: s.block != 0 || s.inline == KindCode, Comment: s.block == '/' || s.line == KindComment}
}
func (adocTokenizer) InitialState() State { return adocState{} }
func adocBlockKind(marker byte) Kind {
	switch marker {
	case '/':
		return KindComment
	case '_':
		return KindEmphasis
	}
	return KindCode
}
func proseClass(kind Kind) Class {
	switch kind {
	case KindComment:
		return ClassComment
	case KindCode:
		return ClassString
	}
	return 0
}
func (adocTokenizer) Scan(in Input, previous State) (Result, error) {
	s, ok := previous.(adocState)
	if !ok {
		return Result{}, fmt.Errorf("language: foreign AsciiDoc state")
	}
	limit, err := scanLimit(in)
	if err != nil {
		return Result{}, err
	}
	w := tokenWriter{base: in.Offset}
	if in.Offset == 0 {
		s.marker, s.run, s.candidate = 0, 0, false
		s.tail = false
		s.line, s.inline, s.close, s.escape = KindPlain, KindPlain, "", false
		if s.block != 0 {
			s.line = adocBlockKind(s.block)
		}
	}
	i := 0
	for i < limit {
		c, n := in.Text[i], runeSize(in.Text, i)
		at := in.Offset + i
		if at == 0 && strings.ContainsRune("-./_+*=", rune(c)) {
			s.marker, s.candidate = c, true
		}
		if s.candidate {
			if c == s.marker && !s.tail {
				s.run++
				kind := s.line
				if s.block == 0 && c == '/' && s.run >= 2 {
					kind = KindComment
					w.add(0, at+n, kind, ClassComment)
				} else {
					w.add(at, at+n, kind, proseClass(kind))
				}
				i += n
				continue
			}
			if space(c) && s.run >= 4 {
				if s.block == 0 && s.marker == '=' && s.run <= 6 {
					s.line = KindHeading
					w.add(0, at, KindHeading, 0)
				}
				s.tail = true
				w.add(at, at+n, s.line, proseClass(s.line))
				i += n
				continue
			}
			s.candidate = false
			if s.block == 0 {
				switch {
				case s.marker == '/' && s.run >= 2:
					s.line = KindComment
				case s.marker == '=' && s.run <= 6 && space(c):
					s.line = KindHeading
					w.add(0, at, KindHeading, 0)
				case strings.ContainsRune("*.-", rune(s.marker)) && space(c):
					w.add(0, at, KindMarker, 0)
				}
			}
		}
		if s.block != 0 || s.line != KindPlain {
			w.add(at, at+n, s.line, proseClass(s.line))
			i += n
			continue
		}
		if at == 0 && c == '[' {
			s.line = KindMeta
			continue
		}
		if at == 0 && c == ':' {
			s.inline, s.close = KindProperty, ":"
			w.add(at, at+n, KindProperty, 0)
			i += n
			continue
		}
		if s.escape {
			w.add(at, at+n, KindEscape, 0)
			s.escape = false
			i += n
			continue
		}
		if c == '\\' && s.inline != KindCode {
			s.escape = true
			w.add(at, at+n, KindEscape, 0)
			i += n
			continue
		}
		if s.close != "" {
			size := n
			if strings.HasPrefix(in.Text[i:], s.close) {
				size = len(s.close)
				w.add(at, at+size, s.inline, proseClass(s.inline))
				s.inline, s.close = KindPlain, ""
			} else {
				w.add(at, at+size, s.inline, proseClass(s.inline))
			}
			i += size
			continue
		}
		kind, close := KindPlain, ""
		switch c {
		case '`':
			kind, close = KindCode, "`"
		case '*', '_':
			kind, close = KindEmphasis, string(c)
		case '{':
			kind, close = KindVariable, "}"
		case '<':
			if strings.HasPrefix(in.Text[i:], "<<") {
				kind, close, n = KindLink, ">>", 2
			}
		case '>':
			if at == 0 {
				kind = KindMarker
			}
		}
		s.inline, s.close = kind, close
		w.add(at, at+n, kind, proseClass(kind))
		i += n
	}
	if in.LineEnd && i == len(in.Text) {
		if s.candidate && s.run >= 4 && (s.block == 0 || s.marker == s.block && s.run == s.width) {
			w.add(0, in.Offset+limit, KindFence, 0)
			if s.block == 0 {
				s.block, s.width = s.marker, s.run
			} else {
				s.block, s.width = 0, 0
			}
		}
		s.marker, s.run, s.candidate = 0, 0, false
		s.tail = false
		s.line, s.inline, s.close, s.escape = KindPlain, KindPlain, "", false
	}
	return w.result(i, s), nil
}

type rstTokenizer struct{}
type rstState struct {
	block, pending                  Kind
	baseIndent, indent, run, colons int
	first, candidate, directive     bool
	marker                          byte
	line, inline                    Kind
	close                           string
	word                            scanWord
	directiveDone, escape           bool
}

func (s rstState) Equal(other State) bool { t, ok := other.(rstState); return ok && s == t }
func (s rstState) Context() Context {
	return Context{String: s.line == KindCode || s.inline == KindCode,
		Literal: s.line == KindCode || s.inline == KindCode, Comment: s.line == KindComment}
}
func (rstTokenizer) InitialState() State { return rstState{} }
func (rstTokenizer) Scan(in Input, previous State) (Result, error) {
	s, ok := previous.(rstState)
	if !ok {
		return Result{}, fmt.Errorf("language: foreign reStructuredText state")
	}
	limit, err := scanLimit(in)
	if err != nil {
		return Result{}, err
	}
	w := tokenWriter{base: in.Offset}
	if in.Offset == 0 {
		s.indent, s.run, s.colons = 0, 0, 0
		s.first, s.candidate, s.directive, s.directiveDone = true, false, false, false
		s.line, s.inline, s.close, s.escape, s.word = KindPlain, KindPlain, "", false, scanWord{}
	}
	i := 0
	for i < limit {
		c, n := in.Text[i], runeSize(in.Text, i)
		at := in.Offset + i
		if s.first {
			if c == ' ' || c == '\t' || c == '\r' {
				s.indent++
				if c == '\t' {
					s.indent += 7
				}
				i += n
				continue
			}
			s.first = false
			if s.pending != KindPlain {
				if s.indent > s.baseIndent {
					s.block = s.pending
				}
				s.pending = KindPlain
			}
			if s.block != KindPlain && s.indent <= s.baseIndent {
				s.block = KindPlain
			}
			s.line = s.block
			if s.line != KindPlain {
				w.add(0, at, s.line, proseClass(s.line))
			} else if strings.HasPrefix(in.Text[i:], ".. ") || strings.HasPrefix(in.Text[i:], "..\t") {
				s.directive, s.line = true, KindComment
				w.add(at, at+2, KindComment, ClassComment)
				i += 2
				continue
			} else if strings.ContainsRune("=-~^\"`:+#*.", rune(c)) {
				s.marker, s.candidate = c, true
			} else if strings.ContainsRune("-+*", rune(c)) && i+1 < len(in.Text) && space(in.Text[i+1]) {
				w.add(at, at+n, KindMarker, 0)
				i += n
				continue
			}
		}
		if s.directive {
			if !s.directiveDone {
				if len(s.word.text) == 0 && space(c) {
					i += n
					continue
				}
				if strings.HasPrefix(in.Text[i:], "::") {
					s.directiveDone = true
					s.line = KindMeta
					w.add(0, at+2, KindMeta, 0)
					switch s.word.text {
					case "code", "code-block", "sourcecode", "raw":
						s.pending, s.baseIndent = KindCode, s.indent
					}
					i += 2
					continue
				}
				s.word.add(in.Text[i : i+n])
			}
			w.add(at, at+n, s.line, proseClass(s.line))
			i += n
			continue
		}
		if s.line != KindPlain {
			w.add(at, at+n, s.line, proseClass(s.line))
			i += n
			continue
		}
		if s.candidate {
			if c == s.marker {
				s.run++
			} else {
				s.candidate = false
				if s.run == 1 && space(c) && strings.ContainsRune("*-+", rune(s.marker)) {
					w.add(at-1, at, KindMarker, 0)
					s.inline, s.close = KindPlain, ""
				}
			}
		}
		if c == ':' {
			s.colons++
		} else if !space(c) {
			s.colons = 0
		}
		if s.close != "" {
			size := n
			if strings.HasPrefix(in.Text[i:], s.close) {
				size = len(s.close)
				w.add(at, at+size, s.inline, proseClass(s.inline))
				s.inline, s.close = KindPlain, ""
			} else {
				w.add(at, at+size, s.inline, proseClass(s.inline))
			}
			i += size
			continue
		}
		if s.escape {
			w.add(at, at+n, KindEscape, 0)
			s.escape = false
			i += n
			continue
		}
		if c == '\\' {
			s.escape = true
			w.add(at, at+n, KindEscape, 0)
			i += n
			continue
		}
		if strings.HasPrefix(in.Text[i:], "``") {
			s.inline, s.close, n = KindCode, "``", 2
		} else if c == '`' {
			s.inline, s.close = KindLink, "`"
		} else if c == '*' {
			s.inline, s.close = KindEmphasis, "*"
		}
		w.add(at, at+n, s.inline, proseClass(s.inline))
		i += n
	}
	if in.LineEnd && i == len(in.Text) {
		if s.candidate && s.run >= 3 {
			w.add(0, in.Offset+limit, KindHeading, 0)
		}
		if !s.first && s.line == KindPlain && s.colons >= 2 {
			s.pending, s.baseIndent = KindCode, s.indent
		}
		if s.directive && !s.directiveDone {
			s.pending, s.baseIndent = KindComment, s.indent
		}
		s.line, s.inline, s.close, s.word = KindPlain, KindPlain, "", scanWord{}
		s.indent, s.run, s.colons = 0, 0, 0
		s.first, s.candidate, s.directive, s.directiveDone, s.escape = false, false, false, false, false
	}
	return w.result(i, s), nil
}
