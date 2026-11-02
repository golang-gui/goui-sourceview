package language

import (
	"fmt"
	"strings"
)

type editorConfigTokenizer struct{}
type editorConfigState struct{ phase byte }

func (s editorConfigState) Equal(other State) bool {
	t, ok := other.(editorConfigState)
	return ok && s == t
}
func (s editorConfigState) Context() Context {
	return Context{String: s.phase == 2, Literal: s.phase == 2, Comment: s.phase == '#', Value: s.phase == 2}
}
func (editorConfigTokenizer) InitialState() State { return editorConfigState{} }

var editorConfigWords = strings.Fields("root indent_style indent_size tab_width end_of_line charset trim_trailing_whitespace insert_final_newline max_line_length unset")

func (editorConfigTokenizer) Scan(in Input, previous State) (Result, error) {
	s, ok := previous.(editorConfigState)
	if !ok {
		return Result{}, fmt.Errorf("language: foreign EditorConfig state")
	}
	limit, err := scanLimit(in)
	if err != nil {
		return Result{}, err
	}
	w := tokenWriter{base: in.Offset}
	for i := 0; i < limit; {
		c, n := in.Text[i], runeSize(in.Text, i)
		at := in.Offset + i
		if s.phase == 0 {
			if space(c) {
				i += n
				continue
			}
			switch c {
			case '#', ';':
				s.phase = '#'
			case '[':
				s.phase = '['
			default:
				s.phase = 1
			}
		}
		kind, class := KindProperty, Class(0)
		switch s.phase {
		case '#':
			kind, class = KindComment, ClassComment
		case '[':
			kind = KindMeta
			if strings.ContainsRune("*?[]{},!", rune(c)) {
				kind = KindDelimiter
			}
		case 2:
			kind, class = KindString, ClassString
		default:
			if c == '=' {
				kind, s.phase = KindDelimiter, 2
			} else if space(c) {
				i += n
				continue
			}
		}
		w.add(at, at+n, kind, class)
		i += n
	}
	if in.LineEnd && limit == len(in.Text) {
		s = editorConfigState{}
	}
	return w.result(limit, s), nil
}
