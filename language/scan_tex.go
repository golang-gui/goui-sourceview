package language

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

type texTokenizer struct{}
type texState struct {
	comment, command, wantEnv, ending, inEnv, wantVerb bool
	word                                               scanWord
	math, mathEnv, raw                                 string
	verb                                               rune
	rawMatch, rawStart                                 int
}

func (s texState) Equal(other State) bool { t, ok := other.(texState); return ok && s == t }
func (s texState) Context() Context {
	return Context{String: s.raw != "" || s.verb != 0, Literal: s.raw != "" || s.verb != 0, Comment: s.comment}
}
func (texTokenizer) InitialState() State { return texState{} }

var texWords = strings.Fields("\\begin \\end \\section \\subsection \\chapter \\documentclass \\usepackage \\title \\author \\date \\maketitle \\label \\ref \\cite \\textbf \\textit \\emph \\item \\include \\input \\frac \\sqrt \\alpha \\beta \\sum \\int \\verb")

func (texTokenizer) Scan(in Input, previous State) (Result, error) {
	s, ok := previous.(texState)
	if !ok {
		return Result{}, fmt.Errorf("language: foreign TeX state")
	}
	limit, err := scanLimit(in)
	if err != nil {
		return Result{}, err
	}
	w := tokenWriter{base: in.Offset}
	finish := func() {
		switch s.word.text {
		case "begin", "end":
			s.wantEnv, s.ending = true, s.word.text == "end"
		case "verb":
			s.wantVerb = true
		}
		s.word, s.command = scanWord{}, false
	}
	i := 0
	for i < limit {
		c := in.Text[i]
		r, n := utf8.DecodeRuneInString(in.Text[i:])
		at := in.Offset + i
		if s.raw != "" {
			close := "\\end{" + s.raw + "}"
			if c == close[s.rawMatch] {
				if s.rawMatch == 0 {
					s.rawStart = at
				}
				s.rawMatch++
			} else {
				s.rawMatch = 0
				if c == '\\' {
					s.rawMatch, s.rawStart = 1, at
				}
			}
			w.add(at, at+n, KindString, ClassString)
			i += n
			if s.rawMatch == len(close) {
				w.add(s.rawStart, in.Offset+i, KindKeyword, 0)
				s.raw, s.rawMatch, s.rawStart = "", 0, 0
			}
			continue
		}
		if s.comment {
			w.add(at, at+n, KindComment, ClassComment)
			i += n
			continue
		}
		if s.verb != 0 {
			w.add(at, at+n, KindString, ClassString)
			if r == s.verb {
				s.verb = 0
			}
			i += n
			continue
		}
		if s.command {
			if unicode.IsLetter(r) || c == '@' {
				s.word.add(in.Text[i : i+n])
				w.add(at, at+n, KindKeyword, 0)
				i += n
				continue
			}
			if s.word.length == 0 {
				switch c {
				case '(':
					s.math = "\\)"
				case '[':
					s.math = "\\]"
				case ')', ']':
					if s.math == "\\"+string(c) {
						s.math = ""
					}
				}
				w.add(at, at+n, KindEscape, 0)
				i += n
				s.command = false
				continue
			}
			finish()
			continue
		}
		if s.inEnv {
			if c == '}' {
				w.add(at, at+n, KindDelimiter, ClassDelimiter)
				env := s.word.text
				if s.ending {
					if env == s.mathEnv {
						s.mathEnv = ""
					}
				} else {
					switch env {
					case "verbatim", "verbatim*", "lstlisting", "minted":
						s.raw = env
					case "math", "displaymath", "equation", "equation*", "align", "align*", "gather", "gather*":
						s.mathEnv = env
					}
				}
				s.inEnv, s.word = false, scanWord{}
			} else {
				s.word.add(in.Text[i : i+n])
				w.add(at, at+n, KindMeta, 0)
			}
			i += n
			continue
		}
		if s.wantVerb {
			if c == '*' {
				w.add(at, at+n, KindKeyword, 0)
				i += n
				continue
			}
			s.wantVerb = false
			if !unicode.IsSpace(r) {
				s.verb = r
				w.add(at, at+n, KindString, ClassString)
				i += n
				continue
			}
		}
		if s.wantEnv {
			if space(c) {
				i += n
				continue
			}
			s.wantEnv = false
			if c == '{' {
				s.inEnv = true
				s.word = scanWord{}
				w.add(at, at+n, KindDelimiter, ClassDelimiter)
				i += n
				continue
			}
		}
		if c == '%' {
			s.comment = true
			continue
		}
		if c == '\\' {
			s.command = true
			s.word = scanWord{}
			w.add(at, at+n, KindKeyword, 0)
			i += n
			continue
		}
		if c == '$' {
			marker := "$"
			if strings.HasPrefix(in.Text[i:], "$$") {
				marker = "$$"
			}
			if s.math == marker {
				s.math = ""
			} else if s.math == "" {
				s.math = marker
			}
			w.add(at, at+len(marker), KindMeta, 0)
			i += len(marker)
			continue
		}
		kind, class := KindPlain, Class(0)
		if s.math != "" || s.mathEnv != "" {
			kind = KindNumber
		}
		if strings.ContainsRune("{}[]", r) {
			kind, class = KindDelimiter, ClassDelimiter
		}
		if c == '#' || c == '&' || c == '^' || c == '_' {
			kind = KindMeta
		}
		w.add(at, at+n, kind, class)
		i += n
	}
	if in.LineEnd && i == len(in.Text) {
		if s.command && s.word.length > 0 {
			finish()
		}
		s.command, s.comment, s.verb, s.wantVerb = false, false, 0, false
		s.rawMatch, s.rawStart = 0, 0
	}
	return w.result(i, s), nil
}
