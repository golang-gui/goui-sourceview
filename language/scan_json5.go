package language

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

type json5Frame struct {
	parent      *json5Frame
	object, key bool
}
type json5State struct {
	stack                                               *json5Frame
	quote                                               byte
	property, escape, wordOn, lineComment, blockComment bool
	escapeLeft                                          int
	word                                                scanWord
	number                                              json5Number
}

func (s json5State) Equal(other State) bool {
	t, ok := other.(json5State)
	if !ok {
		return false
	}
	for a, b := s.stack, t.stack; a != b; a, b = a.parent, b.parent {
		if a == nil || b == nil || a.object != b.object || a.key != b.key {
			return false
		}
	}
	s.stack, t.stack = nil, nil
	return s == t
}
func (s json5State) Context() Context {
	return Context{String: s.quote != 0, Comment: s.lineComment || s.blockComment, Value: s.stack != nil && !s.stack.key}
}

type json5Tokenizer struct{}

func (json5Tokenizer) InitialState() State { return json5State{} }

// JSON5's decimal/hexadecimal forms are recognized without retaining digits.
type json5Number struct {
	phase byte
	bad   bool
}

func (n *json5Number) add(c byte) {
	if n.bad {
		return
	}
	switch n.phase {
	case 0, 1:
		if n.phase == 0 && (c == '+' || c == '-') {
			n.phase = 1
		} else if c == '0' {
			n.phase = 2
		} else if digit(c) {
			n.phase = 3
		} else if c == '.' {
			n.phase = 4
		} else {
			n.bad = true
		}
	case 2, 3:
		switch {
		case c == '.':
			n.phase = 5
		case c == 'e' || c == 'E':
			n.phase = 6
		case n.phase == 2 && (c == 'x' || c == 'X'):
			n.phase = 9
		case digit(c) && n.phase == 3:
		default:
			n.bad = true
		}
	case 4:
		if digit(c) {
			n.phase = 5
		} else {
			n.bad = true
		}
	case 5:
		if c == 'e' || c == 'E' {
			n.phase = 6
		} else if !digit(c) {
			n.bad = true
		}
	case 6:
		if c == '+' || c == '-' {
			n.phase = 7
		} else if digit(c) {
			n.phase = 8
		} else {
			n.bad = true
		}
	case 7:
		if digit(c) {
			n.phase = 8
		} else {
			n.bad = true
		}
	case 8:
		if !digit(c) {
			n.bad = true
		}
	case 9, 10:
		if hex(c) {
			n.phase = 10
		} else {
			n.bad = true
		}
	}
}
func (n json5Number) valid() bool {
	return !n.bad && (n.phase == 2 || n.phase == 3 || n.phase == 5 || n.phase == 8 || n.phase == 10)
}
func (json5Tokenizer) Scan(in Input, previous State) (Result, error) {
	s, ok := previous.(json5State)
	if !ok {
		return Result{}, fmt.Errorf("language: foreign JSON5 state")
	}
	limit, err := scanLimit(in)
	if err != nil {
		return Result{}, err
	}
	w := tokenWriter{base: in.Offset}
	finish := func(end int) {
		kind := KindInvalid
		numberWord := s.word.text
		if len(numberWord) > 0 && (numberWord[0] == '+' || numberWord[0] == '-') {
			numberWord = numberWord[1:]
		}
		switch {
		case s.property:
			kind = KindProperty
		case s.word.text == "true" || s.word.text == "false" || s.word.text == "null":
			kind = KindKeyword
		case s.number.valid() || numberWord == "Infinity" || numberWord == "NaN":
			kind = KindNumber
		}
		w.add(s.word.start, end, kind, 0)
		s.word, s.wordOn, s.number = scanWord{}, false, json5Number{}
	}
	i := 0
	for i < limit {
		c := in.Text[i]
		r, n := utf8.DecodeRuneInString(in.Text[i:])
		at := in.Offset + i
		if s.lineComment {
			w.add(at, at+n, KindComment, ClassComment)
			i += n
			continue
		}
		if s.blockComment {
			if strings.HasPrefix(in.Text[i:], "*/") {
				w.add(at, at+2, KindComment, ClassComment)
				i += 2
				s.blockComment = false
			} else {
				w.add(at, at+n, KindComment, ClassComment)
				i += n
			}
			continue
		}
		if s.quote != 0 {
			kind := KindString
			if s.property {
				kind = KindProperty
			}
			switch {
			case s.escapeLeft > 0:
				kind = KindEscape
				if !hex(c) {
					kind = KindInvalid
				}
				s.escapeLeft--
			case s.escape:
				s.escape = false
				kind = KindEscape
				if c == 'u' {
					s.escapeLeft = 4
				}
				if c == 'x' {
					s.escapeLeft = 2
				}
				if c >= '1' && c <= '9' {
					kind = KindInvalid
				}
			case c == '\\':
				s.escape, kind = true, KindEscape
			case c == s.quote:
				s.quote = 0
			}
			w.add(at, at+n, kind, ClassString)
			i += n
			continue
		}
		if s.wordOn {
			if !unicode.IsSpace(r) && !strings.ContainsRune("{}[]:,/\"'", r) {
				s.word.add(in.Text[i : i+n])
				for _, b := range []byte(in.Text[i : i+n]) {
					s.number.add(b)
				}
				i += n
				continue
			}
			finish(at)
			continue
		}
		if unicode.IsSpace(r) {
			i += n
			continue
		}
		if strings.HasPrefix(in.Text[i:], "//") {
			s.lineComment = true
			continue
		}
		if strings.HasPrefix(in.Text[i:], "/*") {
			s.blockComment = true
			w.add(at, at+2, KindComment, ClassComment)
			i += 2
			continue
		}
		if c == '"' || c == '\'' {
			s.quote, s.property = c, s.stack != nil && s.stack.object && s.stack.key
			kind := KindString
			if s.property {
				kind = KindProperty
			}
			w.add(at, at+n, kind, ClassString)
			i += n
			continue
		}
		switch c {
		case '/':
			w.add(at, at+n, KindInvalid, 0)
			i += n
			continue
		case '{', '[':
			s.stack = &json5Frame{parent: s.stack, object: c == '{', key: c == '{'}
		case '}', ']':
			if s.stack != nil {
				s.stack = s.stack.parent
			}
		case ':', ',':
			if s.stack != nil {
				v := *s.stack
				v.key = c == ',' && v.object
				s.stack = &v
			}
		default:
			s.property = s.stack != nil && s.stack.object && s.stack.key
			s.word, s.wordOn, s.number = scanWord{start: at}, true, json5Number{}
			continue
		}
		class := Class(0)
		if strings.ContainsRune("{}[]", r) {
			class = ClassDelimiter
		}
		w.add(at, at+n, KindDelimiter, class)
		i += n
	}
	if in.LineEnd && i == len(in.Text) {
		if s.wordOn {
			finish(in.Offset + i)
		}
		if s.escape {
			s.escape = false
		} else {
			s.quote = 0
		}
		s.lineComment, s.escapeLeft = false, 0
	}
	return w.result(i, s), nil
}
