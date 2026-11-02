package language

import "fmt"

type csvTokenizer struct{}
type csvState struct {
	quoted, pending, fieldStart bool
	quoteAt                     int
}

func (s csvState) Equal(other State) bool { t, ok := other.(csvState); return ok && s == t }
func (s csvState) Context() Context {
	return Context{String: true, Literal: true}
}
func (csvTokenizer) InitialState() State { return csvState{fieldStart: true} }
func (csvTokenizer) Scan(in Input, previous State) (Result, error) {
	s, ok := previous.(csvState)
	if !ok {
		return Result{}, fmt.Errorf("language: foreign CSV state")
	}
	limit, err := scanLimit(in)
	if err != nil {
		return Result{}, err
	}
	w := tokenWriter{base: in.Offset}
	for i := 0; i < limit; {
		c, n := in.Text[i], runeSize(in.Text, i)
		at := in.Offset + i
		if s.pending {
			s.pending = false
			if c == '"' {
				w.add(s.quoteAt, at+n, KindEscape, ClassString)
				s.quoteAt = 0
				i += n
				continue
			}
			s.quoted, s.quoteAt = false, 0
		}
		kind, class := KindString, ClassString
		if s.quoted {
			if c == '"' {
				s.pending, s.quoteAt = true, at
			}
		} else if c == ',' {
			kind, class, s.fieldStart = KindDelimiter, 0, true
		} else if c == '"' && s.fieldStart {
			s.quoted, s.fieldStart = true, false
		} else {
			s.fieldStart = false
			if c == '"' {
				kind = KindInvalid
			}
		}
		w.add(at, at+n, kind, class)
		i += n
	}
	if in.LineEnd && limit == len(in.Text) {
		if s.pending {
			s.quoted, s.pending, s.quoteAt = false, false, 0
		}
		if !s.quoted {
			s.fieldStart = true
		}
	}
	return w.result(limit, s), nil
}
