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

func (b texTokenizer) Scan(in Input, previous State) (Result, error) {
	return b.scan(in, previous, nil)
}

func (texTokenizer) scan(in Input, previous State, folds *foldLexemes) (Result, error) {
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
			if folds != nil && folds.tex != nil {
				folds.tex.environmentRow = folds.tex.commandRow
			}
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
				folds.texEnvironment(s.raw, true, in.Offset+i)
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
				if s.word.length == len(env) {
					folds.texEnvironment(env, s.ending, at+n)
				}
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
			if folds != nil && folds.tex != nil {
				folds.tex.commandRow = folds.row
			}
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

type bibTokenizer struct{}
type bibState struct {
	head, entry, key, value, quoted bool
	close                           byte
	depth                           int
	word                            scanWord
}

func (s bibState) Equal(other State) bool { t, ok := other.(bibState); return ok && s == t }
func (s bibState) Context() Context {
	// Entry suggestions must remain available before the leading @. Free text
	// is styled as a comment, but is not a syntactic comment context.
	return Context{String: s.quoted || s.depth > 0, Literal: s.quoted || s.depth > 0, Data: s.head || s.entry}
}
func (bibTokenizer) InitialState() State { return bibState{} }

var bibWords = strings.Fields("@article @book @inproceedings @incollection @misc @online @string @preamble @comment author title year journal volume number pages publisher editor booktitle doi url")

func (bibTokenizer) Scan(in Input, previous State) (Result, error) {
	s, ok := previous.(bibState)
	if !ok {
		return Result{}, fmt.Errorf("language: foreign BibTeX state")
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
		if s.head {
			if c == '{' || c == '(' {
				name := strings.ToLower(s.word.text)
				s.head, s.word = false, scanWord{}
				if name == "comment" {
					w.add(at, at+n, KindComment, ClassComment)
				} else {
					s.entry, s.key, s.value = true, name != "string" && name != "preamble", name == "preamble"
					s.close = '}'
					if c == '(' {
						s.close = ')'
					}
					w.add(at, at+n, KindDelimiter, ClassDelimiter)
				}
			} else {
				if !space(c) {
					s.word.add(in.Text[i : i+n])
				}
				w.add(at, at+n, KindKeyword, 0)
			}
			i += n
			continue
		}
		if !s.entry {
			if c == '@' {
				s.head = true
				s.word = scanWord{}
				w.add(at, at+n, KindKeyword, 0)
			} else {
				w.add(at, at+n, KindComment, ClassComment)
			}
			i += n
			continue
		}
		if s.quoted || s.depth > 0 {
			w.add(at, at+n, KindString, ClassString)
			if c == '{' {
				s.depth++
			}
			if c == '}' && s.depth > 0 {
				s.depth--
			}
			if c == '"' && s.quoted && s.depth == 0 {
				s.quoted = false
			}
			i += n
			continue
		}
		kind, class := KindPlain, Class(0)
		switch {
		case c == s.close:
			kind, class, s.entry = KindDelimiter, ClassDelimiter, false
		case c == ',':
			kind, s.key, s.value = KindDelimiter, false, false
		case c == '=':
			kind, s.value = KindDelimiter, true
		case s.value && c == '{':
			kind, class, s.depth = KindString, ClassString, 1
		case s.value && c == '"':
			kind, class, s.quoted = KindString, ClassString, true
		case s.value && c == '#':
			kind = KindDelimiter
		case space(c):
		case s.key:
			kind = KindTag
		case !s.value:
			kind = KindProperty
		case digit(c):
			kind = KindNumber
		default:
			kind = KindVariable
		}
		w.add(at, at+n, kind, class)
		i += n
	}
	return w.result(i, s), nil
}
