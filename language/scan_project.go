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

type gitConfigTokenizer struct{}
type gitConfigState struct {
	phase                  byte
	quote, escape, comment bool
}

func (s gitConfigState) Equal(other State) bool { t, ok := other.(gitConfigState); return ok && s == t }
func (s gitConfigState) Context() Context {
	return Context{String: s.phase == 2 || s.quote, Comment: s.comment, Value: s.phase == 2}
}
func (gitConfigTokenizer) InitialState() State { return gitConfigState{} }

var gitConfigWords = strings.Fields("name email editor autocrlf ignorecase filemode bare repositoryformatversion url fetch pushurl path branch remote merge rebase defaultBranch signingkey gpgsign")

func (gitConfigTokenizer) Scan(in Input, previous State) (Result, error) {
	s, ok := previous.(gitConfigState)
	if !ok {
		return Result{}, fmt.Errorf("language: foreign Git config state")
	}
	limit, err := scanLimit(in)
	if err != nil {
		return Result{}, err
	}
	w := tokenWriter{base: in.Offset}
	for i := 0; i < limit; {
		c, n := in.Text[i], runeSize(in.Text, i)
		at := in.Offset + i
		if s.comment {
			w.add(at, at+n, KindComment, ClassComment)
			i += n
			continue
		}
		if s.phase == 0 {
			if space(c) {
				i += n
				continue
			}
			s.phase = 1
			if c == '[' {
				s.phase = 3
			}
		}
		if s.escape {
			kind := KindEscape
			if s.phase != 3 && !strings.ContainsRune("ntb\\\"", rune(c)) {
				kind = KindInvalid
			}
			w.add(at, at+n, kind, ClassString)
			s.escape = false
			i += n
			continue
		}
		if !s.quote && (c == '#' || c == ';') {
			s.comment = true
			continue
		}
		if c == '\\' && (s.phase == 2 || s.quote) {
			s.escape = true
			w.add(at, at+n, KindEscape, ClassString)
			i += n
			continue
		}
		if c == '"' && (s.phase == 2 || s.phase == 3) {
			s.quote = !s.quote
			w.add(at, at+n, KindString, ClassString)
			i += n
			continue
		}
		kind, class := KindProperty, Class(0)
		switch {
		case s.quote:
			kind, class = KindString, ClassString
		case s.phase == 3:
			kind = KindMeta
			if c == ']' {
				s.phase = 0
			}
		case s.phase == 2:
			kind, class = KindString, ClassString
		case c == '=':
			kind, s.phase = KindDelimiter, 2
		case space(c):
			i += n
			continue
		}
		w.add(at, at+n, kind, class)
		i += n
	}
	if in.LineEnd && limit == len(in.Text) {
		if s.escape && s.phase == 2 && !s.comment {
			s.escape = false
		} else {
			s = gitConfigState{}
		}
	}
	return w.result(limit, s), nil
}

type sshTokenizer struct{}
type sshState struct {
	valueStart                                bool
	phase                                     byte
	quote, escape, comment, variable, percent bool
}

func (s sshState) Equal(other State) bool { t, ok := other.(sshState); return ok && s == t }
func (s sshState) Context() Context {
	return Context{String: s.phase == 2, Value: s.phase == 2, Comment: s.comment}
}
func (sshTokenizer) InitialState() State { return sshState{} }

var sshWords = strings.Fields("Host Match HostName User Port IdentityFile IdentitiesOnly ProxyJump ProxyCommand ForwardAgent ServerAliveInterval ServerAliveCountMax StrictHostKeyChecking UserKnownHostsFile Include AddKeysToAgent ControlMaster ControlPath ControlPersist")

func (sshTokenizer) Scan(in Input, previous State) (Result, error) {
	s, ok := previous.(sshState)
	if !ok {
		return Result{}, fmt.Errorf("language: foreign SSH config state")
	}
	limit, err := scanLimit(in)
	if err != nil {
		return Result{}, err
	}
	w := tokenWriter{base: in.Offset}
	i := 0
	for i < limit {
		c, n := in.Text[i], runeSize(in.Text, i)
		at := in.Offset + i
		if s.comment {
			w.add(at, at+n, KindComment, ClassComment)
			i += n
			continue
		}
		if s.phase == 0 {
			if space(c) {
				i += n
				continue
			}
			if c == '#' {
				s.comment = true
				continue
			}
			s.phase = 1
		}
		if s.phase == 1 {
			if space(c) || c == '=' {
				s.phase = 2
				s.valueStart = c != '='
				if c == '=' {
					w.add(at, at+n, KindDelimiter, 0)
				}
			} else {
				w.add(at, at+n, KindKeyword, 0)
			}
			i += n
			continue
		}
		if s.valueStart {
			if space(c) {
				i += n
				continue
			}
			s.valueStart = false
			if c == '=' {
				w.add(at, at+n, KindDelimiter, 0)
				i += n
				continue
			}
		}
		if s.escape {
			w.add(at, at+n, KindEscape, ClassString)
			s.escape = false
			i += n
			continue
		}
		if s.variable {
			w.add(at, at+n, KindVariable, ClassString)
			if c == '}' {
				s.variable = false
			}
			i += n
			continue
		}
		if s.percent {
			w.add(at, at+n, KindVariable, ClassString)
			s.percent = false
			i += n
			continue
		}
		if !s.quote && c == '#' {
			s.comment = true
			continue
		}
		if c == '"' {
			s.quote = !s.quote
		}
		kind := KindString
		if c == '\\' {
			s.escape, kind = true, KindEscape
		}
		if c == '%' {
			s.percent, kind = true, KindVariable
		}
		if strings.HasPrefix(in.Text[i:], "${") {
			s.variable = true
			w.add(at, at+2, KindVariable, ClassString)
			i += 2
			continue
		}
		w.add(at, at+n, kind, ClassString)
		i += n
	}
	if in.LineEnd && i == len(in.Text) {
		s = sshState{}
	}
	return w.result(i, s), nil
}
