package language

import (
	"context"
	"strings"
	"unicode/utf8"
)

func layoutFunctional(id ID) bool { return id == FSharp || id == Haskell }

const (
	layoutBinding byte = iota
	layoutControl
	layoutBranch
	layoutExplicit
	layoutHaskell
	layoutLetRef
)

type layoutFoldBlock struct {
	row, column, level int
	kind               byte
	word               string
	ready, body, arms  bool
}
type layoutFoldHead struct {
	row, column, level int
	word               string
	module             bool
}
type layoutFoldScope struct {
	groups             []byte
	blocks             []layoutFoldBlock
	pending            *layoutFoldHead
	last, seen, column int
	previous           string
	module             bool
}

// The two languages share source positions and lexical boundaries, but use
// different offside rules: equal columns retain a Haskell layout list, while
// F# declarations end and aligned branch introducers retain their owner.
type layoutFoldPolicy struct {
	id       ID
	scopes   []layoutFoldScope
	disabled bool
}

func (p *layoutFoldPolicy) line(ctx context.Context, l syntaxLine, events []queryFoldBoundary, f *structuralFolder) {
	atoms := keywordFoldAtoms(ctx, l)
	if p.id == FSharp && len(p.scopes) == 1 && !l.before.Context().String && len(atoms) > 0 && l.text[atoms[0].start:atoms[0].end] == "#" {
		if len(atoms) > 2 && l.text[atoms[1].start:atoms[1].end] == "light" && atoms[2].class&ClassString != 0 {
			value := l.text[atoms[2].start:atoms[2].end]
			if value == `"off"` || value == `"on"` {
				p.scopes[0].flush(f, p.scopes[0].last, false)
				p.disabled = value == `"off"`
			}
		}
		return // Directives do not act as ordinary declarations or dedents.
	}
	event, skip, cursor, column := 0, -1, 0, 0
	checkpoint := 0
	for i := 0; i < len(atoms); i++ {
		if i%1024 == 0 && ctx.Err() != nil {
			return
		}
		a := atoms[i]
		for event < len(events) && events[event].start <= a.start {
			e := events[event]
			event++
			skip = e.end
			if e.enter {
				p.scopes = append(p.scopes, layoutFoldScope{seen: -1, last: l.row})
			} else if len(p.scopes) > 1 {
				s := &p.scopes[len(p.scopes)-1]
				s.flush(f, min(s.last, l.row-1), true)
				p.scopes = p.scopes[:len(p.scopes)-1]
				p.scopes[len(p.scopes)-1].touch(l.row)
			}
		}
		if a.start < skip || a.class&ClassComment != 0 {
			continue
		}
		for cursor < a.start {
			if cursor >= checkpoint {
				checkpoint = cursor + 4096
				if ctx.Err() != nil {
					return
				}
			}
			r, n := utf8.DecodeRuneInString(l.text[cursor:a.start])
			cursor += n
			if r == '\t' {
				column = (column/8 + 1) * 8
			} else {
				column++
			}
		}
		value := l.text[a.start:a.end]
		if a.kind == KindDelimiter && i+1 < len(atoms) && atoms[i+1].start == a.end {
			pair := l.text[a.start:atoms[i+1].end]
			if pair == "->" || pair == ";;" {
				value = pair
				a.end = atoms[i+1].end
				i++
			}
		}
		s := &p.scopes[len(p.scopes)-1]
		first := s.seen != l.row
		if first {
			s.seen, s.column = l.row, column
			if p.id == FSharp && !l.before.Context().Comment && !(a.class&ClassString != 0 && l.before.Context().String) {
				leading := l.text[:len(l.text)-len(strings.TrimLeft(l.text, " \t"))]
				if strings.ContainsRune(leading, '\t') {
					// F# forbids indentation tabs. Retain only explicit blocks.
					out := s.blocks[:0]
					for _, b := range s.blocks {
						if b.kind == layoutExplicit {
							out = append(out, b)
						}
					}
					s.blocks = out
					return
				}
			}
		}
		if p.id == Haskell {
			s.haskellBody(value, column)
		}
		if first && !(a.class&ClassString != 0 && l.before.Context().String) && value != "in" && value != "end" && value != "done" && value != ")" && value != "]" && value != "}" {
			s.offside(p.id, value, column, f)
		}
		s.touch(l.row)
		if a.class&ClassString != 0 {
			s.previous = "literal"
			continue
		}
		if p.id == FSharp {
			s.fsharp(a, value, l, f, !p.disabled)
		} else {
			s.haskell(a, value, l, f)
		}
		if a.class&ClassDelimiter != 0 && len(value) == 1 {
			s.group(value[0], l, a.start, f)
		}
		s.previous = value
	}
}

func (s *layoutFoldScope) touch(row int) {
	s.last = row
	if n := len(s.blocks); n > 0 && s.blocks[n-1].ready {
		s.blocks[n-1].body = true
	}
}
func (s *layoutFoldScope) emit(f *structuralFolder, b layoutFoldBlock, end int) {
	if b.ready && b.body && b.kind != layoutLetRef {
		f.add(b.row, end)
	}
}
func (s *layoutFoldScope) pop(f *structuralFolder, end int) {
	n := len(s.blocks) - 1
	s.emit(f, s.blocks[n], end)
	s.blocks = s.blocks[:n]
}
func (s *layoutFoldScope) flush(f *structuralFolder, end int, all bool) {
	var keep []layoutFoldBlock
	for _, b := range s.blocks {
		if b.kind != layoutExplicit {
			s.emit(f, b, end)
		} else if !all {
			keep = append(keep, b)
		}
	}
	s.blocks, s.pending = keep, nil
}
func (s *layoutFoldScope) open(kind byte, word string, row int, ready, arms bool) {
	s.blocks = append(s.blocks, layoutFoldBlock{row: row, column: s.column, level: len(s.groups), kind: kind, word: word, ready: ready, arms: arms})
}
func (s *layoutFoldScope) offside(id ID, value string, column int, f *structuralFolder) {
	for len(s.blocks) > 0 {
		b := s.blocks[len(s.blocks)-1]
		if b.level < len(s.groups) || b.kind == layoutExplicit {
			return
		}
		if id == Haskell {
			if column > b.column || column == b.column && b.kind != layoutLetRef {
				return
			}
		} else {
			if column > b.column {
				return
			}
			if column == b.column && (b.arms && value == "|" || b.word == "if" && (value == "else" || value == "elif") || b.word == "try" && (value == "with" || value == "finally")) {
				return
			}
		}
		s.pop(f, s.last)
	}
}
func (s *layoutFoldScope) find(words ...string) int {
	for i := len(s.blocks) - 1; i >= 0; i-- {
		b := s.blocks[i]
		if b.level != len(s.groups) {
			continue
		}
		for _, word := range words {
			if b.word == word {
				return i
			}
		}
		if b.kind == layoutExplicit {
			break
		}
	}
	return -1
}
func (s *layoutFoldScope) closeAt(index int, l syntaxLine, at int, f *structuralFolder, include bool) {
	if index < 0 {
		return
	}
	end := codeFoldEndLine(l.text, at, l.row)
	if !include {
		end = min(end, l.row-1)
	}
	// Only the matching owner contains its explicit terminator. Implicit
	// children must keep that outer closing line visible when folded alone.
	for len(s.blocks) > index+1 {
		s.pop(f, min(end, l.row-1))
	}
	s.pop(f, end)
}
func (s *layoutFoldScope) fsharp(a keywordFoldAtom, value string, l syntaxLine, f *structuralFolder, enabled bool) {
	if value == "=" {
		if i := s.find("let", "use", "and", "type", "module", "member", "override", "default"); i >= 0 {
			s.blocks[i].ready = true
		}
	}
	if value == "->" {
		if i := s.find("arm", "fun"); i >= 0 {
			s.blocks[i].ready = true
		}
	}
	if value == ";;" {
		for len(s.blocks) > 0 && s.blocks[len(s.blocks)-1].kind != layoutExplicit && s.blocks[len(s.blocks)-1].level == len(s.groups) {
			s.pop(f, l.row-1)
		}
	}
	if value == "|" && enabled {
		for i := len(s.blocks) - 1; i >= 0; i-- {
			if s.blocks[i].level == len(s.groups) && s.blocks[i].arms {
				s.open(layoutBranch, "arm", l.row, false, false)
				break
			}
		}
	}
	if a.kind != KindKeyword || s.previous == "." {
		return
	}
	switch value {
	case "begin", "class", "sig":
		s.open(layoutExplicit, "explicit", l.row, true, false)
	case "end":
		s.closeAt(s.find("explicit"), l, a.end, f, true)
	case "done":
		s.closeAt(s.find("for", "while"), l, a.end, f, true)
	case "in":
		// A for ... in header is not the terminator of a let expression.
		if i := s.find("for"); i < 0 || s.blocks[i].ready {
			s.closeAt(s.find("let", "use"), l, a.start, f, false)
		}
	}
	if !enabled {
		switch value {
		case "for", "while":
			s.open(layoutExplicit, value, l.row, false, false)
		case "do":
			if i := s.find("for", "while"); i >= 0 {
				s.blocks[i].ready = true
			}
		}
		return
	}
	switch value {
	case "let", "use", "and", "type", "module", "member", "override", "default":
		s.open(layoutBinding, value, l.row, false, value == "type")
	case "if", "for", "while", "match", "fun":
		s.open(layoutControl, value, l.row, false, false)
	case "try", "function":
		s.open(layoutControl, value, l.row, true, value == "function")
	case "then":
		if i := s.find("if", "elif"); i >= 0 {
			s.blocks[i].ready = true
		}
	case "do":
		if i := s.find("for", "while"); i >= 0 && !s.blocks[i].ready {
			s.blocks[i].ready = true
		} else {
			s.open(layoutControl, "do", l.row, true, false)
		}
	case "with":
		if i := s.find("match", "try"); i >= 0 {
			s.blocks[i].ready, s.blocks[i].arms = true, true
			if s.blocks[i].word == "try" {
				s.open(layoutBranch, "with", l.row, true, true)
			}
		}
	case "else", "elif", "finally":
		owner := s.find("if")
		if value == "finally" {
			owner = s.find("try")
		}
		if owner >= 0 {
			s.open(layoutBranch, value, l.row, value != "elif", false)
		}
	}
}

func (s *layoutFoldScope) haskellBody(value string, column int) {
	h := s.pending
	if h == nil {
		return
	}
	s.pending = nil
	if value == "{" {
		if h.word == "let" {
			s.open(layoutLetRef, "let", h.row, false, false)
			s.blocks[len(s.blocks)-1].column = h.column
		}
		return
	}
	base := 0
	if h.module {
		base = -1
	}
	for i := len(s.blocks) - 1; i >= 0; i-- {
		b := s.blocks[i]
		if b.level == len(s.groups) && b.kind == layoutHaskell {
			base = b.column
			break
		}
	}
	if column <= base {
		return
	} // Empty implicit list, as defined by the layout rule.
	s.blocks = append(s.blocks, layoutFoldBlock{row: h.row, column: column, level: h.level, kind: layoutHaskell, word: h.word, ready: true})
}
func (s *layoutFoldScope) haskell(a keywordFoldAtom, value string, l syntaxLine, f *structuralFolder) {
	if value == "," {
		// A tuple/list separator ends an expression containing an implicit
		// do/let/of list, even when subsequent elements align with its body.
		for len(s.blocks) > 0 {
			b := s.blocks[len(s.blocks)-1]
			if b.level != len(s.groups) || b.kind != layoutHaskell {
				break
			}
			s.pop(f, codeFoldEndLine(l.text, a.start, l.row))
		}
	}
	if a.kind != KindKeyword || s.previous == "." {
		return
	}
	switch value {
	case "module":
		s.module = true
	case "where", "let", "do", "mdo", "of":
		s.pending = &layoutFoldHead{row: l.row, column: s.column, level: len(s.groups), word: value, module: value == "where" && s.module}
		s.module = false
	case "in":
		s.closeAt(s.find("let"), l, a.start, f, false)
	}
}
func (s *layoutFoldScope) group(c byte, l syntaxLine, at int, f *structuralFolder) {
	switch c {
	case '(', '[', '{':
		s.groups = append(s.groups, c)
	case ')', ']', '}':
		if len(s.groups) == 0 {
			return
		}
		open := s.groups[len(s.groups)-1]
		if open == '(' && c != ')' || open == '[' && c != ']' || open == '{' && c != '}' {
			s.groups, s.blocks, s.pending = nil, nil, nil
			return
		}
		s.groups = s.groups[:len(s.groups)-1]
		for len(s.blocks) > 0 && s.blocks[len(s.blocks)-1].level > len(s.groups) {
			s.pop(f, min(s.last, codeFoldEndLine(l.text, at, l.row)))
		}
		if s.pending != nil && s.pending.level > len(s.groups) {
			s.pending = nil
		}
	}
}
