package language

import (
	"context"
	"strings"
	"unicode/utf8"
)

type keywordFoldAtom struct {
	start, end int
	kind       Kind
	class      Class
	event      int // Shell command substitution: 1 enter, -1 leave.
}
type keywordFoldObserver struct {
	shell   []keywordFoldAtom
	hereRow int
	heres   []keywordFoldHere
}
type keywordFoldHere struct {
	key   any
	enter bool
}
type keywordFoldResume struct {
	key                                    any
	blocks, groups, delims                 int
	blockFloor, groupFloor, delimiterFloor int
	last, previous                         string
	canStart                               bool
	definition                             *rubyFoldDefinition
}

func (f *foldLexemes) shellBoundary(enter bool, start, end int) {
	if f != nil && f.keywords != nil {
		event := -1
		if enter {
			event = 1
		}
		f.keywords.shell = append(f.keywords.shell, keywordFoldAtom{start: start, end: end, event: event})
	}
}

// Complete elided plain-code gaps without re-lexing strings or comments.
// Only lexer-confirmed KindKeyword atoms can open keyword structures.
func keywordFoldAtoms(ctx context.Context, l syntaxLine) []keywordFoldAtom {
	var out []keywordFoldAtom
	checkpoint, active := 0, true
	check := func(at int) bool {
		if active && at >= checkpoint {
			checkpoint, active = at+4096, ctx.Err() == nil
		}
		return active
	}
	plain := func(start, end int) {
		for i := start; i < end; {
			if !check(i) {
				return
			}
			if space(l.text[i]) {
				i++
				continue
			}
			at := i
			r, n := utf8.DecodeRuneInString(l.text[i:end])
			i += n
			if IdentifierRune(r) {
				for i < end {
					if !check(i) {
						return
					}
					r, n = utf8.DecodeRuneInString(l.text[i:end])
					if !IdentifierRune(r) {
						break
					}
					i += n
				}
			}
			out = append(out, keywordFoldAtom{start: at, end: i, kind: KindPlain})
		}
	}
	at := 0
	for _, t := range l.tokens {
		plain(at, t.Start)
		if !active {
			return nil
		}
		if t.Kind == KindDelimiter {
			for i := t.Start; i < t.End; {
				if !check(i) {
					return nil
				}
				_, n := utf8.DecodeRuneInString(l.text[i:t.End])
				out = append(out, keywordFoldAtom{start: i, end: i + n, kind: t.Kind, class: t.Class})
				i += n
			}
		} else {
			out = append(out, keywordFoldAtom{start: t.Start, end: t.End, kind: t.Kind, class: t.Class})
		}
		at = t.End
	}
	plain(at, len(l.text))
	if !active {
		return nil
	}
	return out
}

func keywordFolding(id ID, t Tokenizer) FoldingFunc {
	return func(ctx context.Context, text string) ([]FoldRange, error) {
		observer := &keywordFoldObserver{}
		lexemes := &foldLexemes{row: -1, keywords: observer}
		f := structuralFolder{}
		policy := keywordFoldPolicy{id: id, canStart: true}
		var resumes []keywordFoldResume
		shell := shellFoldPolicy{scopes: []shellFoldScope{{command: true}}}
		err := syntaxLines(ctx, foldingTokenizer{t, lexemes}, text, func(l syntaxLine) {
			if ctx.Err() != nil {
				return
			}
			if id == Shell {
				shell.line(ctx, l, observer.shell, &f)
				observer.shell = observer.shell[:0]
			} else {
				policy.line(ctx, l, &f)
				if ctx.Err() != nil {
					return
				}
				f.delimiters(l, false, id)
			}
			for _, e := range observer.heres {
				if e.enter {
					// Protect suspended prefixes while Ruby scans the heredoc's
					// interpolation; recovery discards only its temporary state.
					resumes = append(resumes, keywordFoldResume{
						key: e.key, blocks: len(policy.blocks), groups: len(policy.groups), delims: len(f.stack),
						blockFloor: policy.blockFloor, groupFloor: policy.groupFloor, delimiterFloor: f.floor,
						last: policy.last, previous: policy.previous, canStart: policy.canStart,
						definition: policy.definition,
					})
					policy.blockFloor, policy.groupFloor, f.floor = len(policy.blocks), len(policy.groups), len(f.stack)
					policy.definition = nil
				} else if n := len(resumes); n > 0 && resumes[n-1].key == e.key {
					r := resumes[n-1]
					resumes = resumes[:n-1]
					policy.blocks, policy.groups, f.stack = policy.blocks[:r.blocks], policy.groups[:r.groups], f.stack[:r.delims]
					policy.blockFloor, policy.groupFloor, f.floor = r.blockFloor, r.groupFloor, r.delimiterFloor
					policy.last, policy.previous, policy.canStart, policy.definition = r.last, r.previous, r.canStart, r.definition
				}
			}
			observer.heres = observer.heres[:0]
			for _, closed := range lexemes.closed {
				f.add(closed.start, codeFoldEndLine(l.text, closed.end, l.row))
			}
			lexemes.closed = lexemes.closed[:0]
		})
		if err != nil {
			return nil, err
		}
		policy.finishSQL(&f, "")
		return noncrossingMixedFolds(ctx, f.out)
	}
}

type keywordFoldOpen struct {
	word       string
	row, level int
	wait       bool
}
type rubyFoldDefinition struct {
	block, base, nameEnd int
	name, bare, closed   bool
}
type luaFoldUntil struct {
	row, level, blocks int
	seen               bool
}
type cmakeFoldCommand struct {
	name       string
	row, depth int
	opened     bool
}
type keywordFoldPolicy struct {
	id                     ID
	blocks                 []keywordFoldOpen
	groups                 []byte
	last, previous         string
	canStart               bool
	definition             *rubyFoldDefinition
	command                *cmakeFoldCommand
	sqlEnd                 *FoldRange
	sqlQualifier           bool
	untils                 []luaFoldUntil
	blockFloor, groupFloor int
}

func (p *keywordFoldPolicy) close(f *structuralFolder, word string, l syntaxLine, end int) {
	if len(p.blocks) <= p.blockFloor {
		return
	}
	o := p.blocks[len(p.blocks)-1]
	if o.level != len(p.groups) {
		return
	}
	if o.word != word {
		p.blocks = nil
		return
	}
	p.blocks = p.blocks[:len(p.blocks)-1]
	if !o.wait {
		f.add(o.row, codeFoldEndLine(l.text, end, l.row))
	}
}
func (p *keywordFoldPolicy) open(word string, row int, wait bool) {
	p.blocks = append(p.blocks, keywordFoldOpen{word: word, row: row, level: len(p.groups), wait: wait})
}
func (p *keywordFoldPolicy) finishSQL(f *structuralFolder, next string) bool {
	if p.sqlEnd == nil {
		return false
	}
	if next == "IF" || next == "LOOP" || next == "WHILE" || next == "FOR" || next == "REPEAT" {
		p.blocks = nil
	} else {
		f.add(p.sqlEnd.StartLine, p.sqlEnd.EndLine)
		p.blocks = p.blocks[:len(p.blocks)-1]
	}
	p.sqlEnd = nil
	return next == "CASE"
}

func (p *keywordFoldPolicy) line(ctx context.Context, l syntaxLine, f *structuralFolder) {
	for i, a := range keywordFoldAtoms(ctx, l) {
		if i%1024 == 0 && ctx.Err() != nil {
			return
		}
		if a.class&ClassComment != 0 {
			continue
		}
		value := l.text[a.start:a.end]
		if p.id == Lua && len(p.untils) > 0 {
			u := &p.untils[len(p.untils)-1]
			if value == ";" && len(p.groups) == u.level && len(p.blocks) == u.blocks {
				if u.seen {
					f.add(u.row, codeFoldEndLine(l.text, a.start, l.row))
				}
				p.untils = p.untils[:len(p.untils)-1]
			} else if !u.seen && a.kind == KindKeyword && !wordsBeforeLuaCondition[value] {
				p.untils = p.untils[:len(p.untils)-1]
			} else {
				u.seen = true
			}
		}
		if p.id == SQL && p.finishSQL(f, strings.ToUpper(value)) {
			continue
		}
		if p.id == CMake {
			p.cmake(a, value, l, f)
			continue
		}
		if a.class&ClassString != 0 || a.kind == KindVariable || a.kind == KindProperty {
			p.previous, p.last, p.canStart = p.last, "literal", false
			continue
		}
		member := p.last == "." || p.last == ":" && p.previous == ":"
		if p.id == Ruby && p.definition != nil {
			naming := !p.definition.name && len(p.groups) == p.definition.base
			p.rubyDefinition(a, value)
			// A reserved method name is a name, not a block opener.
			if naming && a.kind == KindKeyword {
				p.previous, p.last, p.canStart = p.last, value, false
				continue
			}
		}
		if a.kind == KindKeyword && !member {
			switch p.id {
			case Lua:
				switch value {
				case "function", "repeat":
					p.open(value, l.row, false)
				case "if":
					p.open("if", l.row, true)
				case "for", "while":
					p.open("loop", l.row, true)
				case "then":
					if n := len(p.blocks); n > 0 && p.blocks[n-1].word == "if" {
						p.blocks[n-1].wait = false
					}
				case "do":
					if n := len(p.blocks); n > 0 && p.blocks[n-1].word == "loop" && p.blocks[n-1].wait {
						p.blocks[n-1].wait = false
					} else {
						p.open("do", l.row, false)
					}
				case "end":
					if n := len(p.blocks); n > 0 && p.blocks[n-1].word != "repeat" {
						p.close(f, p.blocks[n-1].word, l, a.end)
					} else {
						p.blocks = nil
					}
				case "until":
					if n := len(p.blocks); n > 0 && p.blocks[n-1].word == "repeat" && p.blocks[n-1].level == len(p.groups) {
						o := p.blocks[n-1]
						p.blocks = p.blocks[:n-1]
						p.untils = append(p.untils, luaFoldUntil{row: o.row, level: len(p.groups), blocks: len(p.blocks)})
					} else {
						p.blocks = nil
					}
				}
			case Ruby:
				switch value {
				case "class", "module", "begin", "case", "for", "def":
					p.open(value, l.row, false)
					if value == "for" {
						p.blocks[len(p.blocks)-1].wait = true
					}
					if value == "def" {
						p.definition = &rubyFoldDefinition{block: len(p.blocks) - 1, base: len(p.groups)}
					}
				case "if", "unless", "while", "until":
					if p.canStart {
						p.open(value, l.row, value == "while" || value == "until")
					}
				case "do":
					if n := len(p.blocks); n > p.blockFloor && p.blocks[n-1].wait {
						p.blocks[n-1].wait = false
					} else {
						p.open("do", l.row, false)
					}
				case "end":
					if n := len(p.blocks); n > p.blockFloor {
						p.blocks[n-1].wait = false // Ruby loop 'do' is optional.
						p.close(f, p.blocks[n-1].word, l, a.end)
					}
				}
			case SQL:
				switch strings.ToUpper(value) {
				case "CASE":
					if !p.sqlQualifier {
						p.open("CASE", l.row, false)
					}
				case "END":
					if n := len(p.blocks); n > 0 && p.blocks[n-1].level == len(p.groups) {
						p.sqlEnd = &FoldRange{p.blocks[n-1].row, codeFoldEndLine(l.text, a.end, l.row)}
					}
					p.sqlQualifier = true
				}
			}
		}
		if p.id == SQL && strings.ToUpper(value) != "END" {
			p.sqlQualifier = false
		}
		if a.kind == KindDelimiter {
			p.group(value)
		}
		p.canStart = len(value) == 1 && strings.ContainsRune("(=,[{;!?:&|+-*/%<>", rune(value[0])) || a.kind == KindKeyword && rubyFoldBeforeExpression[value]
		p.previous, p.last = p.last, value
	}
	if p.id == Lua && !luaFoldContinuation[p.last] {
		for len(p.untils) > 0 {
			u := p.untils[len(p.untils)-1]
			if !u.seen || len(p.groups) != u.level || len(p.blocks) != u.blocks {
				break
			}
			f.add(u.row, l.row)
			p.untils = p.untils[:len(p.untils)-1]
		}
	}
	if p.id == Ruby {
		continued := len(p.groups) > 0 || strings.Contains(" , . = + - * / & | \\ ", " "+p.last+" ")
		if p.definition != nil && len(p.groups) <= p.definition.base && !continued {
			p.definition = nil
		}
		if !continued {
			p.canStart = true
			for i := p.blockFloor; i < len(p.blocks); i++ {
				p.blocks[i].wait = false
			}
		}
	}
}

var rubyFoldBeforeExpression = words("return yield then else elsif when rescue ensure and or not begin do")
var wordsBeforeLuaCondition = words("not function true false nil")
var luaFoldContinuation = words("and or not + - * / % ^ . , = < > ~ & |")

func (p *keywordFoldPolicy) group(value string) {
	if len(value) != 1 {
		return
	}
	c := value[0]
	if strings.ContainsRune("([{", rune(c)) {
		p.groups = append(p.groups, c)
	} else if strings.ContainsRune(")]}", rune(c)) && len(p.groups) > p.groupFloor {
		p.groups = p.groups[:len(p.groups)-1]
		for len(p.blocks) > p.blockFloor && p.blocks[len(p.blocks)-1].level > len(p.groups) {
			p.blocks = p.blocks[:len(p.blocks)-1]
		}
	}
}

func (p *keywordFoldPolicy) rubyDefinition(a keywordFoldAtom, value string) {
	d := p.definition
	if d.block >= len(p.blocks) || p.blocks[d.block].word != "def" {
		p.definition = nil
		return
	}
	if value == "=" && len(p.groups) == d.base && d.name && !d.bare && (d.closed || a.start > d.nameEnd) {
		p.blocks = p.blocks[:d.block]
		p.definition = nil
		return
	}
	if len(p.groups) == d.base && (a.kind == KindPlain || a.kind == KindKeyword) {
		if d.name && p.last != "." && p.last != ":" {
			d.bare = true
		}
		d.name, d.nameEnd = true, a.end
	}
	if value == ")" {
		d.nameEnd, d.closed = a.end, true
	} else if value == "]" {
		d.nameEnd = a.end
	} else if len(p.groups) == d.base && len(value) == 1 && strings.ContainsRune("[]+-*/%<>=!~|&^", rune(value[0])) {
		d.name, d.nameEnd = true, a.end
	}
}

var cmakeFoldPairs = map[string]string{"if": "endif", "while": "endwhile", "foreach": "endforeach", "function": "endfunction", "macro": "endmacro", "block": "endblock"}

func (p *keywordFoldPolicy) cmake(a keywordFoldAtom, value string, l syntaxLine, f *structuralFolder) {
	if a.class&(ClassString|ClassComment) != 0 || a.kind == KindVariable {
		return
	}
	if p.command == nil && a.kind == KindKeyword && len(p.groups) == 0 {
		p.command = &cmakeFoldCommand{name: strings.ToLower(value), row: l.row}
		return
	}
	if c := p.command; c != nil {
		if !c.opened {
			if value != "(" {
				p.command = nil
				return
			}
			c.opened, c.depth = true, len(p.groups)
		} else if value == ")" && len(p.groups) == c.depth+1 {
			if cmakeFoldPairs[c.name] != "" {
				p.blocks = append(p.blocks, keywordFoldOpen{word: c.name, row: c.row})
			} else if strings.HasPrefix(c.name, "end") {
				if n := len(p.blocks); n > 0 {
					o := p.blocks[n-1]
					if cmakeFoldPairs[o.word] == c.name {
						f.add(o.row, codeFoldEndLine(l.text, a.end, l.row))
						p.blocks = p.blocks[:n-1]
					} else {
						p.blocks = nil
					}
				}
			}
			p.command = nil
		}
	}
	if a.kind == KindDelimiter {
		// Command groups do not scope CMake control blocks.
		if value == "(" {
			p.groups = append(p.groups, '(')
		} else if value == ")" && len(p.groups) > 0 {
			p.groups = p.groups[:len(p.groups)-1]
		}
	}
}

type shellFoldScope struct {
	row     int
	command bool
	blocks  []shellFoldBlock
	delims  []foldOpen
}
type shellFoldBlock struct {
	word        string
	row         int
	ready       bool
	pattern     bool
	seen        bool
	patternNest int
}
type shellFoldPolicy struct{ scopes []shellFoldScope }

func (p *shellFoldPolicy) line(ctx context.Context, l syntaxLine, atoms []keywordFoldAtom, f *structuralFolder) {
	for i := 0; i < len(atoms); i++ {
		if i%1024 == 0 && ctx.Err() != nil {
			return
		}
		a := atoms[i]
		s := &p.scopes[len(p.scopes)-1]
		if a.event == 1 {
			s.command = false
			p.scopes = append(p.scopes, shellFoldScope{row: l.row, command: true})
			continue
		}
		if a.event == -1 {
			if len(p.scopes) > 1 {
				f.add(s.row, codeFoldEndLine(l.text, a.end, l.row))
				p.scopes = p.scopes[:len(p.scopes)-1]
			}
			continue
		}
		value := l.text[a.start:a.end]
		var top *shellFoldBlock
		if len(s.blocks) > 0 {
			top = &s.blocks[len(s.blocks)-1]
		}
		if top != nil && top.word == "case" && top.pattern {
			if value == "esac" && a.kind == KindKeyword && !top.seen {
				s.close(f, "case", l, a.end)
				continue
			}
			if value == "(" && top.seen {
				top.patternNest++
			}
			if value == ")" {
				if top.patternNest > 0 {
					top.patternNest--
				} else {
					top.pattern, s.command = false, true
				}
			}
			top.seen = true
			continue
		}
		if top != nil && top.word == "case" && !top.ready {
			if value == "in" && a.kind == KindKeyword {
				top.ready, top.pattern, top.seen = true, true, false
			}
			continue
		}
		if a.kind == KindComment {
			continue
		}
		if a.kind == KindKeyword && s.command {
			switch value {
			case "if":
				s.blocks = append(s.blocks, shellFoldBlock{word: "if", row: l.row})
				continue
			case "for", "while", "until", "select":
				s.blocks = append(s.blocks, shellFoldBlock{word: "loop", row: l.row})
				s.command = value == "while" || value == "until"
				continue
			case "case":
				s.blocks = append(s.blocks, shellFoldBlock{word: "case", row: l.row})
				continue
			case "then", "do":
				if top != nil && (value == "then" && top.word == "if" || value == "do" && top.word == "loop") {
					top.ready = true
				}
				continue
			case "else", "elif":
				if top != nil && top.word == "if" {
					top.ready = value == "else"
				}
				continue
			case "fi", "done", "esac":
				word := map[string]string{"fi": "if", "done": "loop", "esac": "case"}[value]
				s.close(f, word, l, a.end)
				s.command = false
				continue
			}
		}
		if a.kind == KindDelimiter {
			if value == ";" && i+1 < len(atoms) && atoms[i+1].start == a.end && (l.text[atoms[i+1].start:atoms[i+1].end] == ";" || l.text[atoms[i+1].start:atoms[i+1].end] == "&") {
				if top != nil && top.word == "case" {
					top.pattern, top.seen = true, false
				}
				i++
				if i+1 < len(atoms) && l.text[atoms[i+1].start:atoms[i+1].end] == "&" {
					i++
				}
				s.command = true
				continue
			}
			if len(value) == 1 {
				c := value[0]
				if strings.ContainsRune("([{", rune(c)) {
					s.delims = append(s.delims, foldOpen{l.row, c})
				} else if strings.ContainsRune(")]}", rune(c)) && len(s.delims) > 0 {
					o := s.delims[len(s.delims)-1]
					if o.char == '(' && c == ')' || o.char == '[' && c == ']' || o.char == '{' && c == '}' {
						f.add(o.line, codeFoldEndLine(l.text, a.end, l.row))
						s.delims = s.delims[:len(s.delims)-1]
					} else {
						s.delims = nil
					}
				}
			}
			s.command = strings.Contains(" ; & | ( { ", " "+value+" ")
		} else {
			s.command = false
		}
	}
	s := l.after.(builtinState).shell
	if s != nil && !s.continued && !s.capture && s.active == nil && (shTop(s.stack) == shCode || shTop(s.stack) == shCommand || shTop(s.stack) == shBacktick) {
		p.scopes[len(p.scopes)-1].command = true
	}
}
func (s *shellFoldScope) close(f *structuralFolder, word string, l syntaxLine, end int) {
	if n := len(s.blocks); n > 0 {
		o := s.blocks[n-1]
		if o.word != word {
			s.blocks = nil
			return
		}
		s.blocks = s.blocks[:n-1]
		if o.ready {
			f.add(o.row, codeFoldEndLine(l.text, end, l.row))
		}
	}
}
