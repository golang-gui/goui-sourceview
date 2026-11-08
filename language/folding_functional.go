package language

import (
	"context"
	"strings"
)

func closedFunctional(id ID) bool { return id == Elixir || id == Erlang || id == Julia || id == OCaml }

type functionalFoldObserver struct {
	events    []queryFoldBoundary
	modifiers []bool
	pending   bool
	end       int
}

func (f *foldLexemes) functionalBoundary(start, end int, enter bool) {
	if f != nil && f.functional != nil {
		f.functional.events = append(f.functional.events, queryFoldBoundary{start, end, enter})
	}
}
func (f *foldLexemes) functionalModifiers() {
	if f != nil && f.functional != nil && len(f.functional.modifiers) > 0 {
		f.functional.modifiers[len(f.functional.modifiers)-1] = true
	}
}
func (f *foldLexemes) closeFunctionalLiteral(end int) {
	if f != nil && f.functional != nil {
		o := f.functional
		if len(o.modifiers) > 0 && o.modifiers[len(o.modifiers)-1] {
			o.pending, o.end = true, end
			return
		}
	}
	f.closeLiteral(end, true)
}

// Observe adjacent sigil/string-macro modifiers without reading beyond the
// scanner's current fragment or changing its tokenization/consumption.
func (f *foldLexemes) functionalModifier(at int, c byte, lineEnd bool) {
	if f == nil || f.functional == nil || !f.functional.pending {
		return
	}
	o := f.functional
	if !lineEnd && at == o.end && (c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z') {
		o.end++
		return
	}
	o.pending = false
	f.closeLiteral(o.end, true)
}

type functionalFoldBlock struct {
	row, level int
	end, word  string
	ready      bool
}
type functionalFoldHeader struct {
	row, level int
	word       string
}
type functionalFoldPending struct {
	row, level, stage int
	tokenRow, end     int
	word              string
}
type functionalFoldGroup struct {
	char                 byte
	row, base            int
	index, comprehension bool
}
type functionalFoldScope struct {
	groups  []functionalFoldGroup
	blocks  []functionalFoldBlock
	headers []functionalFoldHeader
	pending *functionalFoldPending
	prefix  *functionalFoldHeader
	last    string
	value   bool
}

// Each interpolation owns its control/group stacks. A terminator in an
// embedded expression must not consume a suspended outer block.
type functionalFoldPolicy struct {
	id     ID
	scopes []functionalFoldScope
}

func (p *functionalFoldPolicy) line(ctx context.Context, l syntaxLine, events []queryFoldBoundary, f *structuralFolder) {
	at, skip := 0, -1
	atoms := keywordFoldAtoms(ctx, l)
	for i := 0; i < len(atoms); i++ {
		if i%1024 == 0 && ctx.Err() != nil {
			return
		}
		a := atoms[i]
		for at < len(events) && events[at].start <= a.start {
			e := events[at]
			at++
			skip = e.end
			if e.enter {
				p.scopes = append(p.scopes, functionalFoldScope{})
			} else if len(p.scopes) > 1 {
				p.scopes[len(p.scopes)-1] = functionalFoldScope{}
				p.scopes = p.scopes[:len(p.scopes)-1]
			}
		}
		if a.start < skip {
			continue
		}
		value := l.text[a.start:a.end]
		s := &p.scopes[len(p.scopes)-1]
		if a.kind == KindDelimiter && i+1 < len(atoms) && atoms[i+1].start == a.end && a.end-a.start == 1 && atoms[i+1].end-atoms[i+1].start == 1 {
			pair := l.text[a.start:atoms[i+1].end]
			if pair == "->" {
				s.arrow()
				s.last, s.value = pair, false
				i++
				continue
			}
			binary := p.id == Erlang || p.id == Elixir && (a.start == 0 || l.text[a.start-1] != value[0]) && (atoms[i+1].end == len(l.text) || l.text[atoms[i+1].end] != value[0])
			if binary && (pair == "<<" || pair == ">>") {
				if pair == "<<" {
					s.group(p.id, '<', l, atoms[i+1].end, f)
				} else {
					s.group(p.id, '>', l, atoms[i+1].end, f)
				}
				s.last, s.value = pair, pair == ">>"
				i++
				continue
			}
		}
		s.atom(p.id, a, value, l, f)
	}
}
func (s *functionalFoldScope) open(word, end string, row int, ready bool) {
	s.blocks = append(s.blocks, functionalFoldBlock{row, len(s.groups), end, word, ready})
}
func (s *functionalFoldScope) close(end string, l syntaxLine, at int, f *structuralFolder) {
	if n := len(s.blocks); n > 0 {
		b := s.blocks[n-1]
		if b.level != len(s.groups) {
			return
		}
		if b.end != end {
			s.blocks = nil
			return
		}
		s.blocks = s.blocks[:n-1]
		if b.ready {
			f.add(b.row, codeFoldEndLine(l.text, at, l.row))
		}
	}
}
func (s *functionalFoldScope) header(word string, row int) {
	for i := len(s.headers) - 1; i >= 0; i-- {
		if s.headers[i].level == len(s.groups) {
			s.headers = append(s.headers[:i], s.headers[i+1:]...)
			break
		}
	}
	s.headers = append(s.headers, functionalFoldHeader{row, len(s.groups), word})
}
func (s *functionalFoldScope) takeHeader(fallback int) int {
	for i := len(s.headers) - 1; i >= 0; i-- {
		if s.headers[i].level == len(s.groups) {
			row := s.headers[i].row
			s.headers = append(s.headers[:i], s.headers[i+1:]...)
			return row
		}
	}
	return fallback
}
func (s *functionalFoldScope) arrow() {
	if p := s.pending; p != nil && p.word == "fun" && p.stage == 2 && p.level == len(s.groups) {
		s.open("fun", "end", p.row, true)
		s.pending = nil
	}
}
func (s *functionalFoldScope) atom(id ID, a keywordFoldAtom, value string, l syntaxLine, f *structuralFolder) {
	if a.class&ClassComment != 0 {
		return
	}
	if p := s.pending; p != nil {
		switch p.word {
		case "do", "fn":
			// Only an adjacent colon makes a keyword key; :ok on the next
			// line is an ordinary block expression.
			if l.row != p.tokenRow || a.start != p.end || !strings.HasPrefix(value, ":") {
				s.open(p.word, "end", p.row, true)
			}
			s.pending = nil
		case "indexbegin":
			if !strings.Contains(" + - * / ^ % & | : , ] ) = < > ", " "+value+" ") {
				s.open("begin", "end", p.row, true)
			}
			s.pending = nil
		case "fun":
			if p.stage == 0 {
				if value == "/" || value == "," || value == ";" || a.kind == KindKeyword {
					s.pending = nil
				} else if value == "(" && p.level == len(s.groups) {
					p.stage = 1
				}
			}
		}
	}
	prefixRow := -1
	if p := s.prefix; p != nil {
		if p.level == len(s.groups) && (value == "type" && (p.word == "abstract" || p.word == "primitive") || value == "struct" && p.word == "mutable") {
			prefixRow = p.row
		}
		s.prefix = nil
	}
	if a.class&ClassString != 0 {
		s.last, s.value = "literal", true
		return
	}
	member := s.last == "." || (id == Erlang || id == Julia) && s.last == ":"
	if a.kind == KindKeyword && !member {
		switch id {
		case Elixir:
			switch value {
			case "defmodule", "def", "defp", "defmacro", "defprotocol", "defimpl", "if", "unless", "case", "cond", "receive", "try", "for", "with", "quote":
				s.header(value, l.row)
			case "do":
				s.pending = &functionalFoldPending{row: s.takeHeader(l.row), level: len(s.groups), tokenRow: l.row, end: a.end, word: "do"}
			case "fn":
				s.pending = &functionalFoldPending{row: l.row, level: len(s.groups), tokenRow: l.row, end: a.end, word: "fn"}
			case "end":
				s.close("end", l, a.end, f)
			}
		case Erlang:
			switch value {
			case "begin", "case", "if", "receive", "try", "maybe":
				s.open(value, "end", l.row, value != "case")
			case "of":
				if n := len(s.blocks); n > 0 && s.blocks[n-1].word == "case" && s.blocks[n-1].level == len(s.groups) {
					s.blocks[n-1].ready = true
				}
			case "fun":
				s.pending = &functionalFoldPending{row: l.row, level: len(s.groups), word: "fun"}
			case "end":
				s.pending = nil
				s.close("end", l, a.end, f)
			}
		case OCaml:
			switch value {
			case "begin", "struct", "sig", "object":
				s.open(value, "end", l.row, true)
			case "for", "while":
				s.header(value, l.row)
			case "do":
				if row := s.takeHeader(-1); row >= 0 {
					s.open("loop", "done", row, true)
				}
			case "end", "done":
				s.close(value, l, a.end, f)
			}
		case Julia:
			comprehension := false
			if n := len(s.groups); n > 0 {
				g := &s.groups[n-1]
				if (g.char == '[' || g.char == '(') && len(s.blocks) == g.base {
					// A generator follows its yielded expression. Explicit
					// blocks opened within this group still require their end.
					if value == "for" && s.value {
						g.comprehension = true
						comprehension = true
					}
					if value == "if" && g.comprehension {
						comprehension = true
					}
				}
			}
			if !comprehension {
				switch value {
				case "abstract", "primitive", "mutable":
					s.prefix = &functionalFoldHeader{l.row, len(s.groups), value}
				case "type":
					if prefixRow >= 0 {
						s.open(value, "end", prefixRow, true)
					}
				case "struct":
					if prefixRow < 0 {
						prefixRow = l.row
					}
					s.open(value, "end", prefixRow, true)
				case "function", "macro", "module", "baremodule", "if", "for", "while", "let", "quote", "try", "do":
					s.open(value, "end", l.row, true)
				case "begin":
					if n := len(s.groups); n > 0 && s.groups[n-1].char == '[' && s.groups[n-1].index {
						s.pending = &functionalFoldPending{row: l.row, level: n, word: "indexbegin"}
					} else {
						s.open(value, "end", l.row, true)
					}
				case "end":
					s.close("end", l, a.end, f)
				}
			}
		}
	}
	if a.class&ClassDelimiter != 0 && len(value) == 1 {
		s.group(id, value[0], l, a.end, f)
	}
	s.value = a.kind == KindPlain || a.kind == KindVariable || a.kind == KindProperty || a.kind == KindNumber || value == ")" || value == "]" || value == "}" || value == "end" || value == "'" || value == "true" || value == "false"
	s.last = value
}
func (s *functionalFoldScope) group(id ID, c byte, l syntaxLine, end int, f *structuralFolder) {
	switch c {
	case '(', '[', '{', '<':
		s.groups = append(s.groups, functionalFoldGroup{char: c, row: l.row, base: len(s.blocks), index: id == Julia && c == '[' && s.value})
	case ')', ']', '}', '>':
		if n := len(s.groups); n > 0 {
			o := s.groups[n-1]
			if o.char == '(' && c == ')' || o.char == '[' && c == ']' || o.char == '{' && c == '}' || o.char == '<' && c == '>' {
				if o.char == '<' {
					f.add(o.row, codeFoldEndLine(l.text, end, l.row))
				}
				if p := s.pending; p != nil && p.word == "fun" && p.stage == 1 && n == p.level+1 && c == ')' {
					p.stage = 2
				}
				s.groups = s.groups[:n-1]
				for len(s.blocks) > 0 && s.blocks[len(s.blocks)-1].level > len(s.groups) {
					s.blocks = s.blocks[:len(s.blocks)-1]
				}
				for len(s.headers) > 0 && s.headers[len(s.headers)-1].level > len(s.groups) {
					s.headers = s.headers[:len(s.headers)-1]
				}
				if s.pending != nil && s.pending.level > len(s.groups) {
					s.pending = nil
				}
			} else {
				s.groups, s.blocks, s.headers, s.pending = nil, nil, nil, nil
			}
		}
	}
}
