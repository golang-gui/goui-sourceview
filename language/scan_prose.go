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
	for i := 0; i < limit; {
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
			if space(c) && s.run >= 4 && s.marker != '=' {
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
	if in.LineEnd && limit == len(in.Text) {
		if s.candidate && s.run >= 4 && s.marker != '=' && (s.block == 0 || s.marker == s.block && s.run == s.width) {
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
	return w.result(limit, s), nil
}
