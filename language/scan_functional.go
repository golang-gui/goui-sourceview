package language

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	fnCode byte = iota
	fnString
	fnLineComment
	fnBlockComment
	fnNumber
	fnVariable

	fnPrefix
)

// These dialects share bounded words and persistent literal/comment frames,
// while punctuation, reader forms and literal prefixes remain language policy.
type fnLiteral struct {
	quote, open                       byte
	triple, raw, interpolate, comment bool
	arity, nesting                    int
}
type fnExpr struct {
	parent         *fnExpr
	literal        fnLiteral
	close          byte
	depth, matched int
}
type fnDiscard struct {
	parent      *fnDiscard
	depth, need int
}
type fnState struct {
	braceStart                                 int
	id                                         ID
	mode, last                                 byte
	literal                                    fnLiteral
	expr                                       *fnExpr
	discard                                    *fnDiscard
	commentDepth, braces, prefix               int
	escape, wordOn, wordPrefix, value, atStart bool
	word                                       scanWord
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
	for a, b := s.discard, t.discard; a != b; a, b = a.parent, b.parent {
		if a == nil || b == nil || a.depth != b.depth || a.need != b.need {
			return false
		}
	}

	s.expr, t.expr = nil, nil
	s.discard, t.discard = nil, nil

	return s == t
}
func (s fnState) Context() Context {
	return Context{String: s.mode == fnString || s.mode == fnVariable,
		Literal: s.mode == fnString && s.literal.raw && s.id != Julia && s.id != Elixir,
		Comment: s.mode == fnLineComment || s.mode == fnBlockComment || s.literal.comment || s.discard != nil}
}

type functionalTokenizer struct{ id ID }

func (b functionalTokenizer) InitialState() State { return fnState{id: b.id, atStart: true} }

var functionalWords = map[ID]map[string]bool{
	FSharp:  words("abstract and as assert base begin class default delegate do done downcast downto elif else end exception extern false finally fixed for fun function global if in inherit inline interface internal lazy let match member module mutable namespace new not null of open or override private public rec return select sig static struct then to true try type upcast use val void when while with yield async task seq"),
	Elixir:  words("after alias and case catch cond def defdelegate defexception defguard defimpl defmacro defmodule defp defprotocol defstruct do else end false fn for if import in nil not or quote raise receive require rescue super true try unless unquote unquote_splicing use when with"),
	Erlang:  words("after begin case catch cond end fun if let of receive try when and andalso band bnot bor bsl bsr bxor div not or orelse rem xor maybe else true false"),
	Julia:   words("abstract baremodule begin break catch const continue do else elseif end export false finally for function global if import in isa let local macro module mutable primitive public quote return struct true try using where while type"),
	Clojure: words("def defn defn- defmacro defonce defmulti defmethod defrecord deftype defprotocol fn fn* let letfn loop recur if if-not if-let when when-not when-let cond condp case do doseq dotimes for lazy-seq binding with-open try catch finally throw new set! ns in-ns require import quote var nil true false and or not"),
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

	case Clojure:
		d.Comments.Line = ";"
	case Elixir, Julia:
		d.Comments.Line = "#"
	case Erlang:
		d.Comments.Line = "%"
	}
	if id == FSharp {
		d.Comments.Line = "//"
	}
	if id == Julia {
		d.Comments.Block = Pair{Open: "#=", Close: "=#"}
	}
	d.Word = func(r rune) bool { return fnName(id, r) || id == Elixir && r == ':' }
	d.AllowPair = func(c PairContext) bool {
		if c.Selected || c.Context.String {
			return true
		}
		if c.Typed == "'" && (id == FSharp || id == Julia || id == Clojure) {
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
	case Clojure:
		return strings.ContainsRune("*+!_?-./:$%&=<>", r)
	case FSharp:
		return r == '\''
	case Elixir:
		return r == '!' || r == '?'
	}
	return false
}
func fnComment(id ID) (string, string) {
	switch id {
	case FSharp:
		return "(*", "*)"

	case Julia:
		return "#=", "=#"
	}
	return "", ""
}
func fnLine(id ID) string {
	switch id {
	case FSharp:
		return "//"

	case Erlang:
		return "%"
	case Clojure:
		return ";"
	case Julia, Elixir:
		return "#"
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
		if s.discard != nil {
			k, c = KindComment, ClassComment
		}
		w.add(in.Offset+a, in.Offset+z, k, c)
	}
	formDone := func() {
		if s.discard != nil && s.discard.depth == 0 {
			f := *s.discard
			f.need--
			if f.need == 0 {
				s.discard = f.parent
			} else {
				s.discard = &f
			}
		}
	}
	finish := func(end int) {
		if !s.wordOn {
			return
		}
		k := s.word.kind
		if (k == KindPlain || s.id == Erlang && k == KindProperty) && functionalWords[s.id][s.word.text] {
			k = KindKeyword
		}
		if s.discard != nil {
			k = KindComment
		}
		class := Class(0)
		if k == KindComment {
			class = ClassComment
		} else if k == KindString {
			class = ClassString
		}
		w.add(s.word.start, end, k, class)
		if !s.wordPrefix {
			formDone()
		}
		s.word, s.wordOn, s.wordPrefix, s.value, s.atStart = scanWord{}, false, false, true, false
	}
	endString := func() {
		wasComment := s.literal.comment
		s.literal = fnLiteral{}
		s.escape, s.braces = false, 0
		s.mode = fnCode
		s.value = true
		if wasComment {
			s.mode = fnBlockComment
		} else {
			formDone()
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
			if (s.id == FSharp) && c == '"' {
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
		case fnVariable:
			if IdentifierRune(r) {
				add(i, i+n, KindVariable, ClassString)
				i += n
				continue
			}
			s.mode = fnString
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
			if s.id == FSharp && s.literal.raw && !s.literal.triple && strings.HasPrefix(text[i:], "\"\"") {
				add(i, i+2, KindEscape, class)
				i += 2
				continue
			}
			if s.literal.interpolate && !s.literal.comment {
				size, close := 0, byte('}')
				if s.id == Elixir && strings.HasPrefix(text[i:], "#{") {
					size = 2
				}
				if s.id == Julia && strings.HasPrefix(text[i:], "$"+"(") {
					size, close = 2, ')'
				}
				if s.id == Julia && c == '$' && i+1 < len(text) {
					next, _ := utf8.DecodeRuneInString(text[i+1:])
					if IdentifierRune(next) {
						add(i, i+1, KindVariable, class)
						i++
						s.mode = fnVariable
						continue
					}
				}
				if s.id == FSharp {
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
			if s.literal.open != 0 && c == s.literal.open {
				s.literal.nesting++
				add(i, i+n, k, class)
				i += n
				continue
			}
			if c == s.literal.quote {
				if s.literal.nesting > 0 {
					s.literal.nesting--
					add(i, i+n, k, class)
					i += n
					continue
				}
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
			if c == '\\' && (!s.literal.raw || s.id == Julia || s.id == Elixir) {
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
			if IdentifierRune(r) || s.id == Erlang && c == '#' || c == '.' && i+1 < len(text) && digit(text[i+1]) || (c == '+' || c == '-') && (s.last == 'e' || s.last == 'E' || s.last == 'p' || s.last == 'P') {
				add(i, i+n, KindNumber, 0)
				s.last = c
				i += n
				continue
			}
			s.mode = fnCode
			s.value = true
			formDone()
		}
		if s.wordOn {
			if fnName(s.id, r) {
				s.word.add(text[i : i+n])
				i += n
				continue
			}
			if s.id == Julia && c == '"' {
				w.add(s.word.start, in.Offset+i, KindMeta, 0)
				s.word, s.wordOn = scanWord{}, false
				s.literal = fnLiteral{raw: true}
				s.mode = fnString
				s.literal.quote = '"'
				s.literal.triple = strings.HasPrefix(text[i:], "\"\"\"")
				size := 1
				if s.literal.triple {
					size = 3
				}
				add(i, i+size, KindString, ClassString)
				i += size
				continue
			}
			finish(in.Offset + i)
			continue
		}
		if space(c) || s.id == Clojure && c == ',' {
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
		if s.id == Clojure && strings.HasPrefix(text[i:], "#_") {
			s.discard = &fnDiscard{parent: s.discard, need: 1}
			add(i, i+2, KindComment, ClassComment)
			i += 2
			continue
		}
		if s.id == Clojure && strings.HasPrefix(text[i:], "#?") {
			size := 2
			if i+2 < len(text) && text[i+2] == '@' {
				size = 3
			}
			add(i, i+size, KindMeta, 0)
			i += size
			continue
		}
		if s.id == Clojure && c == '^' && s.discard != nil && s.discard.depth == 0 {
			v := *s.discard
			v.need++
			s.discard = &v
		}
		if s.id == Clojure && c == '\\' {
			s.word = scanWord{start: in.Offset + i, kind: KindString}
			s.wordOn = true
			s.word.add(text[i : i+n])
			i += n
			if i < len(text) {
				z := i + runeSize(text, i)
				s.word.add(text[i:z])
				i = z
			}
			continue
		}
		if s.id == Clojure && c == '#' && i+1 < len(text) && (unicode.IsLetter(rune(text[i+1])) || text[i+1] == ':') {
			s.word = scanWord{start: in.Offset + i, kind: KindMeta}
			s.wordOn, s.wordPrefix = true, true
			s.word.add("#")
			i++
			continue
		}
		if s.id == FSharp && (c == '$' || c == '@') {
			s.mode = fnPrefix
			s.prefix = 0
			s.literal = fnLiteral{}
			continue
		}

		if s.id == Elixir && c == '~' && i+2 < len(text) && text[i+1] < utf8.RuneSelf && unicode.IsLetter(rune(text[i+1])) && strings.ContainsRune("/|\"'([{<", rune(text[i+2])) {
			d := text[i+2]
			close := d
			paired := byte(0)
			if pos := strings.IndexByte("([{<", d); pos >= 0 {
				paired = d
				close = ")]}>"[pos]
			}
			s.literal = fnLiteral{quote: close, open: paired, raw: unicode.IsUpper(rune(text[i+1])), interpolate: unicode.IsLower(rune(text[i+1])), arity: 1}
			size := 3
			if (d == '"' || d == '\'') && strings.HasPrefix(text[i+2:], strings.Repeat(string(d), 3)) {
				s.literal.triple = true
				size = 5
			}
			s.mode = fnString
			add(i, i+size, KindString, ClassString)
			i += size
			continue
		}
		if s.id == Erlang && c == '$' {
			add(i, i+1, KindString, ClassString)
			i++
			if i < len(text) {
				z := i + runeSize(text, i)
				if text[i] == '\\' && z < len(text) {
					z += runeSize(text, z)
				}
				add(i, z, KindString, ClassString)
				i = z
			}
			s.value = true
			continue
		}
		if s.id == Elixir && c == '?' && i+1 < len(text) {
			z := i + 1 + runeSize(text, i+1)
			if text[i+1] == '\\' && z < len(text) {
				z += runeSize(text, z)
			}
			add(i, z, KindString, ClassString)
			i = z
			s.value = true
			continue
		}
		if s.id == Julia && c == '@' || s.id == Erlang && c == '-' && s.atStart {
			s.word = scanWord{start: in.Offset + i, kind: KindMeta}
			s.wordOn = true
			s.word.add(text[i : i+n])
			i += n
			continue
		}
		if c == '\'' && (s.id == FSharp || s.id == Julia) {
			if size := fnCharacter(text[i:]); size > 0 && (s.id != Julia || !s.value) {
				add(i, i+size, KindString, ClassString)
				i += size
				s.value = true
				formDone()
				continue
			}
		}
		if c == '"' || c == '\'' && (s.id == Elixir || s.id == Erlang) || s.id == Julia && c == 0x60 {
			s.mode = fnString
			s.literal = fnLiteral{quote: c, arity: 1, interpolate: s.id == Elixir || s.id == Julia}
			s.literal.triple = (s.id == FSharp || s.id == Elixir || s.id == Julia || s.id == Erlang) && strings.HasPrefix(text[i:], strings.Repeat(string(c), 3))
			if s.id == FSharp && s.literal.triple {
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
		if digit(c) || s.id == Clojure && (c == '-' || c == '+') && i+1 < len(text) && digit(text[i+1]) {
			s.mode, s.last = fnNumber, c
			add(i, i+1, KindNumber, 0)
			i++
			continue
		}
		if fnName(s.id, r) || s.id == Elixir && (c == ':' || c == '@') {
			k := KindPlain
			if s.id == Clojure && c == ':' || s.id == Elixir && c == ':' {
				k = KindProperty
			}
			if s.id == Elixir && c == '@' {
				k = KindMeta
			}
			if s.id == Elixir && unicode.IsUpper(r) {
				k = KindMeta
			}
			if s.id == Erlang {
				if unicode.IsUpper(r) || c == '_' {
					k = KindVariable
				} else {
					k = KindProperty
				}
				if functionalWords[s.id][string(r)] {
					k = KindPlain
				}
			}
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
		if s.id == Clojure && s.discard != nil && strings.ContainsRune("([{", r) {
			v := *s.discard
			v.depth++
			s.discard = &v
		}
		class := Class(0)
		if strings.ContainsRune("()[]{}", r) {
			class = ClassDelimiter
		}
		k := KindDelimiter
		if s.id == Clojure && strings.ContainsRune("'@^#~", r) {
			k = KindMeta
		}
		add(i, i+n, k, class)
		i += n
		if s.id == Clojure && s.discard != nil && strings.ContainsRune(")]}", r) {
			v := *s.discard
			if v.depth > 0 {
				v.depth--
			}
			s.discard = &v
			if v.depth == 0 {
				formDone()
			}
		}
		s.value = strings.ContainsRune(")]}", r) || s.id == Julia && c == '\''
		s.atStart = false
	}

	if in.LineEnd && i == len(text) {

		finish(in.Offset + i)
		if s.mode == fnLineComment {
			s.mode = fnCode
		}
		if s.mode == fnNumber {
			s.mode = fnCode
			formDone()
		}
		if s.mode == fnVariable {
			s.mode = fnString
		}

		if s.mode == fnString && !s.literal.triple && !s.literal.comment && (s.id == FSharp) {
			endString()
		}
		s.last, s.escape, s.value, s.atStart = 0, false, false, true
		s.braces, s.braceStart = 0, 0
	}
	return w.result(i, s), nil
}
