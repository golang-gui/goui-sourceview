package language

import (
	"fmt"
	"slices"
	"strings"

	"unicode/utf8"
)

const (
	fnCode byte = iota
	fnString
	fnLineComment
	fnBlockComment
	fnNumber

	fnPrefix
)

// These dialects share bounded words and persistent literal/comment frames,
// while punctuation, reader forms and literal prefixes remain language policy.
type fnLiteral struct {
	quote                             byte
	triple, raw, interpolate, comment bool
	arity                             int
}
type fnExpr struct {
	parent         *fnExpr
	literal        fnLiteral
	close          byte
	depth, matched int
}

type fnState struct {
	braceStart int
	id         ID
	mode, last byte
	literal    fnLiteral
	expr       *fnExpr

	commentDepth, braces, prefix int
	escape, wordOn, value        bool
	word                         scanWord
}

func (s fnState) Equal(other State) bool {
	t, ok := other.(fnState)
	if !ok {
		return false
	}
	for a, b := s.expr, t.expr; a != b; a, b = a.parent, b.parent {
		if a == nil || b == nil || a.literal != b.literal || a.close != b.close || a.depth != b.depth || a.matched != b.matched {
			return false
		}
	}

	s.expr, t.expr = nil, nil

	return s == t
}
func (s fnState) Context() Context {
	return Context{String: s.mode == fnString,
		Literal: s.mode == fnString && s.literal.raw,
		Comment: s.mode == fnLineComment || s.mode == fnBlockComment || s.literal.comment}
}

type functionalTokenizer struct{ id ID }

func (b functionalTokenizer) InitialState() State { return fnState{id: b.id} }

var functionalWords = map[ID]map[string]bool{
	FSharp: words("abstract and as assert base begin class default delegate do done downcast downto elif else end exception extern false finally fixed for fun function global if in inherit inline interface internal lazy let match member module mutable namespace new not null of open or override private public rec return select sig static struct then to true try type upcast use val void when while with yield async task seq"),
}

func functionalVocabulary(id ID) []string {
	var out []string
	for word := range functionalWords[id] {
		out = append(out, word)
	}
	slices.Sort(out)
	return out
}
func functionalEditing(id ID) EditingRules {
	d := defaultEditing()
	switch id {
	case FSharp:
		d.Comments = Comments{Line: "", Block: Pair{Open: "(*", Close: "*)"}}

	}

	d.Comments.Line = "//"

	d.Word = func(r rune) bool { return fnName(id, r) }
	d.AllowPair = func(c PairContext) bool {
		if c.Selected || c.Context.String {
			return true
		}
		if c.Typed == "'" {
			return false
		}
		return c.Typed != "\"" || !strings.HasSuffix(c.Before, "\"\"")
	}
	return d
}
func fnName(id ID, r rune) bool {
	if IdentifierRune(r) {
		return true
	}
	switch id {

	case FSharp:
		return r == '\''

	}
	return false
}
func fnComment(id ID) (string, string) {
	switch id {
	case FSharp:
		return "(*", "*)"

	}
	return "", ""
}
func fnLine(id ID) string {
	switch id {
	case FSharp:
		return "//"

	}
	return ""
}

// Character literals have a bounded spelling; an unmatched apostrophe belongs
// to an identifier/type variable or an operator rather than a long string.
func fnCharacter(text string) int {
	if len(text) < 3 || text[0] != '\'' {
		return 0
	}
	_, n := utf8.DecodeRuneInString(text[1:])
	end := 1 + n
	if text[1] == '\\' {
		end = 2
		if end < len(text) && (text[end] == '\'' || text[end] == '\\') {
			end++
		} else {
			for end < len(text) && end < 14 && text[end] != '\'' && !space(text[end]) {
				end++
			}
		}
	}
	if end < len(text) && text[end] == '\'' {
		return end + 1
	}
	return 0
}
func (b functionalTokenizer) Scan(in Input, state State) (Result, error) {
	s, ok := state.(fnState)
	if !ok || s.id != b.id {
		return Result{}, fmt.Errorf("language: foreign functional state")
	}
	limit, err := scanLimit(in)
	if err != nil {
		return Result{}, err
	}
	w := tokenWriter{base: in.Offset}
	text, i := in.Text, 0
	add := func(a, z int, k Kind, c Class) {

		w.add(in.Offset+a, in.Offset+z, k, c)
	}

	finish := func(end int) {
		if !s.wordOn {
			return
		}
		k := s.word.kind
		if (k == KindPlain) && functionalWords[s.id][s.word.text] {
			k = KindKeyword
		}

		class := Class(0)
		if k == KindComment {
			class = ClassComment
		} else if k == KindString {
			class = ClassString
		}
		w.add(s.word.start, end, k, class)

		s.word, s.wordOn, s.value = scanWord{}, false, true
	}
	endString := func() {
		wasComment := s.literal.comment
		s.literal = fnLiteral{}
		s.escape, s.braces = false, 0
		s.mode = fnCode
		s.value = true
		if wasComment {
			s.mode = fnBlockComment
		}

	}

	for i < limit {
		c := text[i]
		r, n := utf8.DecodeRuneInString(text[i:])
		if s.mode == fnCode && s.expr != nil && s.expr.matched != 0 && c != s.expr.close {
			v := *s.expr
			v.matched = 0
			s.expr = &v
		}
		switch s.mode {

		case fnLineComment:
			add(i, i+n, KindComment, ClassComment)
			i += n
			continue
		case fnBlockComment:
			open, close := fnComment(s.id)
			if strings.HasPrefix(text[i:], open) {
				s.commentDepth++
				add(i, i+len(open), KindComment, ClassComment)
				i += len(open)
				continue
			}
			if strings.HasPrefix(text[i:], close) {
				s.commentDepth--
				add(i, i+len(close), KindComment, ClassComment)
				i += len(close)
				if s.commentDepth == 0 {
					s.mode = fnCode
				}
				continue
			}
			if c == '"' {
				s.mode = fnString
				s.literal = fnLiteral{quote: c, comment: true}
				add(i, i+1, KindComment, ClassComment)
				i++
				continue
			}

			add(i, i+n, KindComment, ClassComment)
			i += n
			continue

		case fnPrefix:
			if c == '$' {
				s.prefix++
				add(i, i+1, KindMeta, 0)
				i++
				continue
			}
			if c == '@' {
				s.literal.raw = true
				add(i, i+1, KindMeta, 0)
				i++
				continue
			}
			if c == '"' {
				s.mode = fnString
				s.literal.quote = c
				s.literal.interpolate = s.prefix > 0
				s.literal.arity = max(1, s.prefix)
				s.literal.triple = strings.HasPrefix(text[i:], "\"\"\"")
				size := 1
				if s.literal.triple {
					size = 3
					s.literal.raw = true
				}
				add(i, i+size, KindString, ClassString)
				i += size
				s.prefix = 0
				continue
			}
			s.prefix, s.literal, s.mode = 0, fnLiteral{}, fnCode
			continue

		case fnString:
			k, class := KindString, ClassString
			if s.literal.comment {
				k, class = KindComment, ClassComment
			}
			if s.escape {
				if !s.literal.raw {
					k = KindEscape
				}
				add(i, i+n, k, class)
				i += n
				s.escape = false
				continue
			}
			if s.literal.raw && !s.literal.triple && strings.HasPrefix(text[i:], "\"\"") {
				add(i, i+2, KindEscape, class)
				i += 2
				continue
			}
			if s.literal.interpolate && !s.literal.comment {
				size, close := 0, byte('}')

				if s.literal.arity == 1 && (strings.HasPrefix(text[i:], "{{") || strings.HasPrefix(text[i:], "}}")) {
					add(i, i+2, KindEscape, class)
					i += 2
					continue
				}
				if c == '{' {
					if s.braces == 0 {
						s.braceStart = in.Offset + i
					}
					s.braces++
					add(i, i+1, KindString, ClassString)
					i++
					if s.braces < s.literal.arity {
						continue
					}
					w.add(s.braceStart, in.Offset+i, KindDelimiter, ClassDelimiter)
					size = -1
				} else {
					s.braces = 0
				}

				if size != 0 {
					s.expr = &fnExpr{parent: s.expr, literal: s.literal, close: close}
					if size > 0 {
						add(i, i+size, KindDelimiter, ClassDelimiter)
						i += size
					}
					s.mode, s.literal, s.braces, s.value = fnCode, fnLiteral{}, 0, false
					continue
				}
			}

			if c == s.literal.quote {

				size := 1
				if s.literal.triple {
					if !strings.HasPrefix(text[i:], strings.Repeat(string(c), 3)) {
						add(i, i+1, k, class)
						i++
						continue
					}
					size = 3
				}
				add(i, i+size, k, class)
				i += size
				endString()
				continue
			}
			if c == '\\' && (!s.literal.raw) {
				if !s.literal.raw {
					k = KindEscape
				}
				add(i, i+1, k, class)
				i++
				s.escape = true
				continue
			}
			add(i, i+n, k, class)
			i += n
			continue
		case fnNumber:
			if IdentifierRune(r) || c == '.' && i+1 < len(text) && digit(text[i+1]) || (c == '+' || c == '-') && (s.last == 'e' || s.last == 'E' || s.last == 'p' || s.last == 'P') {
				add(i, i+n, KindNumber, 0)
				s.last = c
				i += n
				continue
			}
			s.mode = fnCode
			s.value = true

		}
		if s.wordOn {
			if fnName(s.id, r) {
				s.word.add(text[i : i+n])
				i += n
				continue
			}

			finish(in.Offset + i)
			continue
		}
		if space(c) {
			i += n
			continue
		}
		line := fnLine(s.id)
		if line != "" && strings.HasPrefix(text[i:], line) {
			isComment := true

			if isComment {
				s.mode = fnLineComment
				continue
			}
		}
		open, _ := fnComment(s.id)
		if open != "" && strings.HasPrefix(text[i:], open) {
			s.mode, s.commentDepth = fnBlockComment, 1
			add(i, i+len(open), KindComment, ClassComment)
			i += len(open)
			continue
		}

		if c == '$' || c == '@' {
			s.mode = fnPrefix
			s.prefix = 0
			s.literal = fnLiteral{}
			continue
		}

		if c == '\'' {
			if size := fnCharacter(text[i:]); size > 0 {
				add(i, i+size, KindString, ClassString)
				i += size
				s.value = true

				continue
			}
		}
		if c == '"' {
			s.mode = fnString
			s.literal = fnLiteral{quote: c, arity: 1, interpolate: false}
			s.literal.triple = strings.HasPrefix(text[i:], strings.Repeat(string(c), 3))
			if s.literal.triple {
				s.literal.raw = true
			}
			size := 1
			if s.literal.triple {
				size = 3
			}
			add(i, i+size, KindString, ClassString)
			i += size
			continue
		}
		if digit(c) {
			s.mode, s.last = fnNumber, c
			add(i, i+1, KindNumber, 0)
			i++
			continue
		}
		if fnName(s.id, r) {
			k := KindPlain

			s.word = scanWord{start: in.Offset + i, kind: k}
			s.wordOn = true
			s.word.add(text[i : i+n])
			i += n
			continue
		}
		if s.expr != nil {
			f := s.expr
			if c == f.close && f.depth == 0 {
				v := *f
				v.matched++
				add(i, i+1, KindDelimiter, ClassDelimiter)
				i++
				if v.matched >= max(1, f.literal.arity) {
					s.mode, s.literal, s.expr = fnString, f.literal, f.parent
				} else {
					s.expr = &v
				}
				continue
			}
			if f.matched != 0 {
				v := *f
				v.matched = 0
				s.expr = &v
				f = &v
			}
			open := byte('{')
			if f.close == ')' {
				open = '('
			}
			if c == open || c == f.close {
				v := *f
				if c == open {
					v.depth++
				} else {
					v.depth--
				}
				s.expr = &v
			}
		}

		class := Class(0)
		if strings.ContainsRune("()[]{}", r) {
			class = ClassDelimiter
		}
		k := KindDelimiter

		add(i, i+n, k, class)
		i += n

		s.value = strings.ContainsRune(")]}", r)

	}

	if in.LineEnd && i == len(text) {

		finish(in.Offset + i)
		if s.mode == fnLineComment {
			s.mode = fnCode
		}
		if s.mode == fnNumber {
			s.mode = fnCode

		}

		if s.mode == fnString && !s.literal.triple && !s.literal.comment {
			endString()
		}
		s.last, s.escape, s.value = 0, false, false
		s.braces, s.braceStart = 0, 0
	}
	return w.result(i, s), nil
}
