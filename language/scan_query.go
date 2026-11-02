package language

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

const (
	queryCode byte = iota
	queryString
	queryLineComment
	queryBlockComment
	queryNumber
	queryPath
)

type queryFrame struct {
	parent             *queryFrame
	mode, quote, close byte
	indent             bool
	depth              int
}
type queryState struct {
	id                             ID
	mode, quote, last              byte
	indent, escape, wordOn, member bool
	word                           scanWord
	expr                           *queryFrame
}

func (s queryState) Equal(other State) bool {
	t, ok := other.(queryState)
	if !ok {
		return false
	}
	for a, b := s.expr, t.expr; a != b; a, b = a.parent, b.parent {
		if a == nil || b == nil || a.mode != b.mode || a.quote != b.quote || a.close != b.close || a.indent != b.indent || a.depth != b.depth {
			return false
		}
	}
	s.expr, t.expr = nil, nil
	return s == t
}
func (s queryState) Context() Context {
	return Context{String: s.mode == queryString || s.mode == queryPath, Literal: s.mode == queryPath || s.quote == 0x60 || s.indent, Comment: s.mode == queryLineComment || s.mode == queryBlockComment}
}

type queryTokenizer struct{ id ID }

func (b queryTokenizer) InitialState() State { return queryState{id: b.id} }

var nixWords = words("if then else assert with let in rec inherit or true false null import builtins derivation")
var jqWords = words("def as if then elif else end reduce foreach try catch label break module import include and or true false null empty select map map_values keys values length type tostring tonumber sort sort_by group_by unique unique_by add any all has in del path paths getpath setpath error debug halt first last limit range inputs input")
var promWords = words("and or unless by without on ignoring group_left group_right bool offset sum min max avg group stddev stdvar count count_values bottomk topk quantile limitk limit_ratio rate irate increase delta idelta changes resets deriv predict_linear histogram_quantile label_replace label_join sum_over_time avg_over_time max_over_time min_over_time count_over_time absent absent_over_time scalar vector time timestamp sort sort_desc start end step range min_of max_of")

func queryWords(id ID) map[string]bool {
	switch id {
	case Nix:
		return nixWords
	case JQ:
		return jqWords
	default:
		return promWords
	}
}
func sortedQueryWords(id ID) []string {
	out := make([]string, 0, len(queryWords(id)))
	for w := range queryWords(id) {
		out = append(out, w)
	}
	slices.Sort(out)
	return out
}
func queryName(id ID, r rune) bool {
	return IdentifierRune(r) || id == Nix && (r == '-' || r == '\'') || id == PromQL && r == ':'
}
func (b queryTokenizer) Scan(in Input, state State) (Result, error) {
	s, ok := state.(queryState)
	if !ok || s.id != b.id {
		return Result{}, fmt.Errorf("language: foreign query state")
	}
	limit, err := scanLimit(in)
	if err != nil {
		return Result{}, err
	}
	w := tokenWriter{base: in.Offset}
	text, i := in.Text, 0
	add := func(a, z int, k Kind, c Class) { w.add(in.Offset+a, in.Offset+z, k, c) }
	finish := func(end int) {
		if !s.wordOn {
			return
		}
		k := s.word.kind
		if k == KindPlain && !s.member && queryWords(s.id)[s.word.text] {
			k = KindKeyword
		}
		if s.id == PromQL && (strings.EqualFold(s.word.text, "inf") || strings.EqualFold(s.word.text, "nan")) {
			k = KindNumber
		}
		w.add(s.word.start, end, k, 0)
		s.word, s.wordOn, s.member = scanWord{}, false, false
	}
	for i < limit {
		c := text[i]
		r, n := utf8.DecodeRuneInString(text[i:])
		switch s.mode {
		case queryLineComment:
			add(i, i+n, KindComment, ClassComment)
			i += n
			continue
		case queryBlockComment:
			if strings.HasPrefix(text[i:], "*/") {
				add(i, i+2, KindComment, ClassComment)
				i += 2
				s.mode = queryCode
			} else {
				add(i, i+n, KindComment, ClassComment)
				i += n
			}
			continue
		case queryString, queryPath:
			if s.escape {
				add(i, i+n, KindEscape, ClassString)
				i += n
				s.escape = false
				continue
			}
			if s.id == Nix && s.indent && strings.HasPrefix(text[i:], "''") {
				if i+2 < len(text) && (text[i+2] == '\'' || text[i+2] == '$' || text[i+2] == '\\') {
					add(i, i+3, KindEscape, ClassString)
					s.escape = text[i+2] == '\\'
					i += 3
					continue
				}
				add(i, i+2, KindString, ClassString)
				i += 2
				s.mode, s.indent = queryCode, false
				continue
			}
			if s.mode == queryString && !s.indent && c == s.quote {
				add(i, i+1, KindString, ClassString)
				i++
				s.mode, s.quote = queryCode, 0
				continue
			}
			if s.id == Nix && strings.HasPrefix(text[i:], "$"+"{") || s.id == JQ && strings.HasPrefix(text[i:], "\\(") {
				close := byte('}')
				if s.id == JQ {
					close = ')'
				}
				s.expr = &queryFrame{parent: s.expr, mode: s.mode, quote: s.quote, indent: s.indent, close: close}
				add(i, i+2, KindDelimiter, ClassDelimiter)
				i += 2
				s.mode, s.quote, s.indent = queryCode, 0, false
				continue
			}
			if s.mode == queryPath {
				if s.quote == '>' && c == '>' {
					add(i, i+1, KindString, ClassString)
					i++
					s.mode, s.quote = queryCode, 0
					continue
				}
				if space(c) || strings.ContainsRune(";)]}", r) {
					s.mode, s.quote = queryCode, 0
					continue
				}
			} else if !s.indent && s.quote != 0x60 && c == '\\' {
				add(i, i+1, KindEscape, ClassString)
				i++
				s.escape = true
				continue
			}
			add(i, i+n, KindString, ClassString)
			i += n
			continue
		case queryNumber:
			if IdentifierRune(r) || c == '.' && i+1 < len(text) && digit(text[i+1]) || (c == '+' || c == '-') && (s.last == 'e' || s.last == 'E') {
				add(i, i+n, KindNumber, 0)
				s.last = c
				i += n
				continue
			}
			s.mode = queryCode
		}
		if s.wordOn {
			if queryName(s.id, r) || s.id == JQ && strings.ContainsRune("$:", r) {
				s.word.add(text[i : i+n])
				i += n
				continue
			}
			if s.id == Nix && c == '/' && i+1 < len(text) && !space(text[i+1]) && text[i+1] != '/' {
				w.add(s.word.start, in.Offset+i, KindString, ClassString)
				s.word, s.wordOn = scanWord{}, false
				s.mode = queryPath
				continue
			}
			finish(in.Offset + i)
			continue
		}
		if space(c) {
			i += n
			continue
		}
		if c == '#' {
			s.mode = queryLineComment
			continue
		}
		if s.id == Nix && strings.HasPrefix(text[i:], "/*") {
			s.mode = queryBlockComment
			add(i, i+2, KindComment, ClassComment)
			i += 2
			continue
		}
		if c == '"' || s.id == PromQL && (c == '\'' || c == 0x60) || s.id == Nix && strings.HasPrefix(text[i:], "''") {
			s.mode, s.quote = queryString, c
			s.indent = s.id == Nix && c == '\''
			size := 1
			if s.indent {
				size = 2
			}
			add(i, i+size, KindString, ClassString)
			i += size
			continue
		}
		if s.id == Nix && (strings.HasPrefix(text[i:], "./") || strings.HasPrefix(text[i:], "../") || strings.HasPrefix(text[i:], "~/") || c == '/' && i+1 < len(text) && !space(text[i+1]) && text[i+1] != '/' || c == '<' && i+1 < len(text) && queryName(Nix, rune(text[i+1]))) {
			s.mode = queryPath
			if c == '<' {
				s.quote = '>'
			}
			add(i, i+1, KindString, ClassString)
			i++
			continue
		}
		if digit(c) || c == '.' && i+1 < len(text) && digit(text[i+1]) {
			s.mode, s.last = queryNumber, c
			add(i, i+1, KindNumber, 0)
			i++
			continue
		}
		if queryName(s.id, r) || s.id == JQ && (c == '$' || c == '@') {
			k := KindPlain
			if s.member {
				k = KindProperty
			}
			if c == '$' {
				k = KindVariable
			} else if c == '@' {
				k = KindMeta
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
				add(i, i+1, KindDelimiter, ClassDelimiter)
				i++
				s.mode, s.quote, s.indent, s.expr = f.mode, f.quote, f.indent, f.parent
				continue
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
		add(i, i+n, KindDelimiter, class)
		s.member = s.id == JQ && c == '.'
		i += n
	}
	if in.LineEnd && i == len(text) {
		finish(in.Offset + i)
		if s.mode == queryLineComment || s.mode == queryNumber || s.mode == queryPath {
			s.mode, s.quote = queryCode, 0
		}
		if s.mode == queryString && s.id == PromQL && s.quote != 0x60 {
			s.mode, s.quote = queryCode, 0
		}
		s.escape, s.last, s.member = false, 0, false
	}
	return w.result(i, s), nil
}
