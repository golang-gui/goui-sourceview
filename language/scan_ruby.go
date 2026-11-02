package language

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

type rbMode uint8

const (
	rbCode rbMode = iota
	rbQuoted
	rbRegexpFlags
	rbLineComment
	rbBlockComment
	rbHereOpen
	rbHereBody
	rbNumber
	rbData
	rbStringVariable
)

type rbHere struct {
	label           string
	indent, literal bool
}

// A persistent append tree keeps queued heredocs in declaration order without
// copying the queue on each fragment or reversing it in one UI turn.
type rbQueue struct {
	left, right *rbQueue
	here        *rbHere
	n           int
}

func rbEnqueue(q *rbQueue, h *rbHere) *rbQueue {
	leaf := &rbQueue{here: h, n: 1}
	if q == nil {
		return leaf
	}
	if q.n&(q.n-1) == 0 {
		return &rbQueue{left: q, right: leaf, n: q.n + 1}
	}
	return &rbQueue{left: q.left, right: rbEnqueue(q.right, h), n: q.n + 1}
}
func (q *rbQueue) at(i int) *rbHere {
	for q.here == nil {
		if i < q.left.n {
			q = q.left
		} else {
			i -= q.left.n
			q = q.right
		}
	}
	return q.here
}
func rbQueuesEqual(a, b *rbQueue) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil || a.n != b.n {
		return false
	}
	if a.here != nil || b.here != nil {
		return a.here != nil && b.here != nil && *a.here == *b.here
	}
	return rbQueuesEqual(a.left, b.left) && rbQueuesEqual(a.right, b.right)
}

type rbLiteral struct {
	open, close                rune
	depth                      int
	interpolate, regexp, class bool
}
type rbExpr struct {
	parent  *rbExpr
	mode    rbMode
	literal rbLiteral
	braces  int
}
type rbResume struct {
	parent             *rbResume
	mode               rbMode
	literal            rbLiteral
	expr               *rbExpr
	active             *rbHere
	queue              *rbQueue
	next               int
	canRegexp, command bool
}
type rbState struct {
	back                                                         rbMode
	mode                                                         rbMode
	literal                                                      rbLiteral
	expr                                                         *rbExpr
	resume                                                       *rbResume
	queue                                                        *rbQueue
	next                                                         int
	active                                                       *rbHere
	matching, beginning                                          bool
	matched, matchStart                                          int
	word                                                         scanWord
	wordOn, escape, canRegexp, command, statement, gap, blockEnd bool
	last                                                         byte
	hereStart                                                    int
	hereQuote                                                    byte
	hereIndent                                                   bool
	capture                                                      *scanText
}

func rbExprsEqual(a, b *rbExpr) bool {
	for a != b {
		if a == nil || b == nil || a.mode != b.mode || a.literal != b.literal || a.braces != b.braces {
			return false
		}
		a, b = a.parent, b.parent
	}
	return true
}
func rbHeresEqual(a, b *rbHere) bool { return a == b || a != nil && b != nil && *a == *b }
func (s rbState) Equal(other State) bool {
	t, ok := other.(rbState)
	if !ok || !rbExprsEqual(s.expr, t.expr) || !rbHeresEqual(s.active, t.active) || !rbQueuesEqual(s.queue, t.queue) || s.capture != t.capture && captureText(s.capture) != captureText(t.capture) {
		return false
	}
	for a, b := s.resume, t.resume; a != b; a, b = a.parent, b.parent {
		if a == nil || b == nil || a.mode != b.mode || a.literal != b.literal || a.next != b.next || a.canRegexp != b.canRegexp || a.command != b.command || !rbExprsEqual(a.expr, b.expr) || !rbHeresEqual(a.active, b.active) || !rbQueuesEqual(a.queue, b.queue) {
			return false
		}
	}
	s.expr, t.expr = nil, nil
	s.resume, t.resume = nil, nil
	s.active, t.active = nil, nil
	s.queue, t.queue = nil, nil
	s.capture, t.capture = nil, nil
	return s == t
}
func (s rbState) Context() Context {
	m := s.mode
	if m == rbStringVariable {
		m = s.back
	}
	return Context{String: m == rbQuoted || m == rbRegexpFlags || m == rbHereBody || m == rbData, Literal: m == rbData || m == rbHereBody && s.active != nil && s.active.literal, Comment: m == rbLineComment || m == rbBlockComment}
}

type rbTokenizer struct{}

func (rbTokenizer) InitialState() State { return rbState{canRegexp: true, statement: true} }

var rbKeywords = words("BEGIN END alias and begin break case class def defined? do else elsif end ensure false for if in module next nil not or redo rescue retry return self super then true undef unless until when while yield __ENCODING__ __FILE__ __LINE__")

func (rbTokenizer) Scan(in Input, state State) (Result, error) {
	s, ok := state.(rbState)
	if !ok {
		return Result{}, fmt.Errorf("language: foreign Ruby state")
	}
	limit, err := scanLimit(in)
	if err != nil {
		return Result{}, err
	}
	w := tokenWriter{base: in.Offset}
	text, i := in.Text, 0
	add := func(a, z int, k Kind, c Class) { w.add(in.Offset+a, in.Offset+z, k, c) }
	var capture strings.Builder
	flush := func() {
		if capture.Len() > 0 {
			s.capture = &scanText{previous: s.capture, text: capture.String()}
			capture.Reset()
		}
	}
	finishWord := func(end int) {
		if !s.wordOn {
			return
		}
		kind := s.word.kind
		if kind == KindPlain && rbKeywords[s.word.text] {
			kind = KindKeyword
		}
		w.add(s.word.start, end, kind, 0)
		s.canRegexp = kind == KindKeyword && !strings.Contains(" true false nil self end ", " "+s.word.text+" ")
		s.command = s.statement && kind == KindPlain
		if s.word.text == "__END__" && s.word.start == 0 && end == 7 && in.LineEnd && end == in.Offset+len(text) {
			s.mode = rbData
		}
		s.statement = false
		s.word, s.wordOn = scanWord{}, false
	}
	finishHere := func() {
		flush()
		h := &rbHere{label: captureText(s.capture), indent: s.hereIndent, literal: s.hereQuote == '\''}
		s.queue = rbEnqueue(s.queue, h)
		s.capture, s.hereQuote, s.hereIndent, s.hereStart = nil, 0, false, 0
		s.mode, s.canRegexp = rbCode, false
	}
	for i < limit {
		c := text[i]
		r, n := utf8.DecodeRuneInString(text[i:])
		switch s.mode {
		case rbStringVariable:
			if IdentifierRune(r) {
				add(i, i+n, KindVariable, ClassString)
				i += n
				continue
			}
			s.mode, s.back = s.back, rbCode
			continue
		case rbData:
			add(i, i+n, KindString, ClassString)
			i += n
			continue
		case rbLineComment, rbBlockComment:
			if s.mode == rbBlockComment && in.Offset+i == 0 && strings.HasPrefix(text[i:], "=end") && (i+4 == len(text) || space(text[i+4])) {
				s.blockEnd = true
			}
			add(i, i+n, KindComment, ClassComment)
			i += n
			continue
		case rbHereOpen:
			if s.hereQuote != 0 {
				if c == s.hereQuote {
					add(i, i+1, KindString, ClassString)
					i++
					finishHere()
					continue
				}
				capture.WriteString(text[i : i+n])
				add(i, i+n, KindString, ClassString)
				i += n
				continue
			}
			if IdentifierRune(r) {
				capture.WriteString(text[i : i+n])
				add(i, i+n, KindString, ClassString)
				i += n
				continue
			}
			finishHere()
			continue
		case rbRegexpFlags:
			if strings.ContainsRune("imxounes", r) {
				add(i, i+n, KindString, ClassString)
				i += n
				continue
			}
			s.mode = rbCode
		case rbQuoted, rbHereBody:
			interp := s.mode == rbQuoted && s.literal.interpolate || s.mode == rbHereBody && !s.active.literal
			if s.escape {
				add(i, i+n, KindEscape, ClassString)
				s.escape = false
				i += n
				continue
			}
			if c == '\\' {
				escape := interp
				if s.mode == rbQuoted && i+1 < len(text) {
					next, _ := utf8.DecodeRuneInString(text[i+1:])
					escape = escape || next == s.literal.close || next == '\\' || next == s.literal.open
				}
				if escape {
					add(i, i+1, KindEscape, ClassString)
					s.escape = true
					i++
					continue
				}
			}
			if interp && strings.HasPrefix(text[i:], "#{") {
				s.expr = &rbExpr{parent: s.expr, mode: s.mode, literal: s.literal, braces: 1}
				s.mode, s.literal, s.canRegexp, s.command = rbCode, rbLiteral{}, true, false
				s.statement = true
				add(i, i+2, KindDelimiter, ClassDelimiter)
				i += 2
				continue
			}
			if interp && c == '#' && i+1 < len(text) && (text[i+1] == '@' || text[i+1] == '$') {
				add(i, i+1, KindString, ClassString)
				i++
				s.back, s.mode = s.mode, rbStringVariable
				z := i + 1
				if text[i] == '@' && z < len(text) && text[z] == '@' {
					z++
				}
				add(i, z, KindVariable, ClassString)
				i = z
				continue
			}
			if s.mode == rbQuoted {
				lit := &s.literal
				wasClass := lit.class
				if lit.regexp {
					if r == '[' {
						lit.class = true
					} else if r == ']' {
						lit.class = false
					}
				}
				if !lit.class && !wasClass {
					if lit.open != lit.close && r == lit.open {
						lit.depth++
					}
					if r == lit.close {
						lit.depth--
						if lit.depth == 0 {
							regexp := lit.regexp
							s.mode, s.literal, s.canRegexp = rbCode, rbLiteral{}, false
							if regexp {
								s.mode = rbRegexpFlags
							}
						}
					}
				}
			}
			add(i, i+n, KindString, ClassString)
			i += n
			continue
		case rbNumber:
			if IdentifierRune(r) || c == '.' && !strings.HasPrefix(text[i:], "..") || (c == '+' || c == '-') && strings.ContainsRune("eE", rune(s.last)) {
				add(i, i+n, KindNumber, 0)
				s.last = c
				i += n
				continue
			}
			s.mode, s.last = rbCode, 0
		}
		if s.wordOn {
			if IdentifierRune(r) || c == '?' || c == '!' && (s.word.kind == KindVariable || i+1 == len(text) || text[i+1] != '=') {
				s.word.add(text[i : i+n])
				i += n
				continue
			}
			finishWord(in.Offset + i)
		}
		if c == ' ' || c == '\t' || c == '\r' {
			s.gap = true
			i++
			continue
		}
		if c == '#' {
			s.mode = rbLineComment
			continue
		}
		if in.Offset+i == 0 && strings.HasPrefix(text[i:], "=begin") && (i+6 == len(text) || space(text[i+6])) {
			s.mode = rbBlockComment
			continue
		}
		canLiteral := s.canRegexp || s.command && s.gap
		if strings.HasPrefix(text[i:], "<<") && canLiteral {
			z := i + 2
			indent := false
			if z < len(text) && (text[z] == '-' || text[z] == '~') {
				indent = true
				z++
			}
			if z < len(text) {
				next, _ := utf8.DecodeRuneInString(text[z:])
				if next == '\'' || next == '"' || next == '`' || codeNameStart(Ruby, next) {
					s.mode, s.hereStart, s.hereIndent = rbHereOpen, in.Offset+i, indent
					if next == '\'' || next == '"' || next == '`' {
						s.hereQuote = byte(next)
						z++
					}
					add(i, z, KindString, ClassString)
					i = z
					continue
				}
			}
		}
		if c == '\'' || c == '"' || c == '`' {
			s.mode, s.literal = rbQuoted, rbLiteral{open: r, close: r, depth: 1, interpolate: c != '\''}
			s.gap = false
			add(i, i+n, KindString, ClassString)
			i += n
			continue
		}
		if c == '%' && canLiteral && i+1 < len(text) {
			z := i + 1
			kind := byte('Q')
			if strings.ContainsRune("qQwWiIrsx", rune(text[z])) {
				kind = text[z]
				z++
			}
			if z < len(text) {
				open, size := utf8.DecodeRuneInString(text[z:])
				if !IdentifierRune(open) && !strings.ContainsRune(" \t\r", open) {
					close := open
					switch open {
					case '(':
						close = ')'
					case '[':
						close = ']'
					case '{':
						close = '}'
					case '<':
						close = '>'
					}
					s.mode, s.literal = rbQuoted, rbLiteral{open: open, close: close, depth: 1, interpolate: strings.ContainsRune("QWIrx", rune(kind)), regexp: kind == 'r'}
					add(i, z+size, KindString, ClassString)
					i = z + size
					s.gap = false
					continue
				}
			}
		}
		if c == '/' && canLiteral && !strings.HasPrefix(text[i:], "/=") && (s.canRegexp || i+1 < len(text) && !space(text[i+1])) {
			s.mode, s.literal = rbQuoted, rbLiteral{open: '/', close: '/', depth: 1, interpolate: true, regexp: true}
			add(i, i+1, KindString, ClassString)
			i++
			continue
		}
		if c == '@' || c == '$' || c == ':' && s.canRegexp && !strings.HasPrefix(text[i:], "::") {
			kind := KindVariable
			if c == ':' {
				kind = KindProperty
				if i+1 < len(text) && (text[i+1] == '\'' || text[i+1] == '"') {
					add(i, i+1, KindString, ClassString)
					i++
					continue
				}
			}
			s.word, s.wordOn = scanWord{start: in.Offset + i, kind: kind}, true
			s.word.add(text[i : i+1])
			i++
			if c == '@' && i < len(text) && text[i] == '@' {
				s.word.add("@")
				i++
			}
			continue
		}
		if digit(c) {
			s.mode, s.last, s.canRegexp = rbNumber, c, false
			add(i, i+1, KindNumber, 0)
			i++
			continue
		}
		if c == '?' && s.canRegexp && i+1 < len(text) && !space(text[i+1]) {
			z := i + 1 + runeSize(text, i+1)
			if text[i+1] == '\\' && z < len(text) {
				esc := text[z]
				z += runeSize(text, z)
				if esc == 'u' && z < len(text) && text[z] == '{' {
					z++
					for z < len(text) && z < i+16 && hex(text[z]) {
						z++
					}
					if z < len(text) && text[z] == '}' {
						z++
					}
				} else if esc == 'u' || esc == 'x' {
					count := 2
					if esc == 'u' {
						count = 4
					}
					for count > 0 && z < len(text) && hex(text[z]) {
						z++
						count--
					}
				}
			}
			add(i, z, KindString, ClassString)
			i = z
			s.canRegexp = false
			continue
		}
		if IdentifierRune(r) {
			s.word, s.wordOn = scanWord{start: in.Offset + i}, true
			s.word.add(text[i : i+n])
			i += n
			continue
		}
		if f := s.expr; f != nil && (c == '{' || c == '}') {
			next := *f
			if c == '{' {
				next.braces++
			} else {
				next.braces--
			}
			if next.braces == 0 {
				s.mode, s.literal, s.expr = f.mode, f.literal, f.parent
				add(i, i+1, KindDelimiter, ClassDelimiter)
				i++
				continue
			}
			s.expr = &next
		}
		s.canRegexp = c != ')' && c != ']' && c != '}' && c != '.'
		s.command, s.gap = false, false
		if c == ';' {
			s.statement = true
		}
		class := Class(0)
		if strings.ContainsRune("()[]{}", r) {
			class = ClassDelimiter
		}
		add(i, i+n, KindDelimiter, class)
		i += n
	}
	flush()
	if s.active != nil {
		for j := 0; j < i && s.matching; j++ {
			c := text[j]
			if s.beginning && s.active.indent && (c == ' ' || c == '\t') {
				s.matchStart = in.Offset + j + 1
				continue
			}
			s.beginning = false
			if s.matched >= len(s.active.label) || c != s.active.label[s.matched] {
				s.matching = false
			} else {
				s.matched++
			}
		}
	}
	if in.LineEnd && i == len(text) {
		if s.mode == rbStringVariable {
			s.mode, s.back = s.back, rbCode
		}
		finishWord(in.Offset + i)
		if s.mode == rbHereOpen {
			if s.hereQuote == 0 {
				finishHere()
			} else {
				s.mode, s.capture, s.hereQuote, s.hereStart, s.hereIndent = rbCode, nil, 0, 0, false
			}
		}
		if s.active != nil && s.matching && s.matched == len(s.active.label) {
			w.add(s.matchStart, in.Offset+i, KindString, ClassString)
			f := s.resume
			s.mode, s.literal, s.expr, s.active, s.queue, s.next, s.canRegexp, s.command, s.resume = f.mode, f.literal, f.expr, f.active, f.queue, f.next, f.canRegexp, f.command, f.parent
		}
		if s.mode == rbLineComment || s.mode == rbRegexpFlags || s.mode == rbNumber || s.mode == rbBlockComment && s.blockEnd {
			s.mode = rbCode
		}
		if s.queue != nil && s.next < s.queue.n {
			h := s.queue.at(s.next)
			s.next++
			if s.next == s.queue.n {
				s.queue, s.next = nil, 0
			}
			s.resume = &rbResume{parent: s.resume, mode: s.mode, literal: s.literal, expr: s.expr, active: s.active, queue: s.queue, next: s.next, canRegexp: s.canRegexp, command: s.command}
			s.mode, s.literal, s.expr, s.active, s.queue, s.next = rbHereBody, rbLiteral{}, nil, h, nil, 0
		}
		s.matching, s.beginning, s.matched, s.matchStart = s.active != nil, true, 0, 0
		s.escape, s.last, s.blockEnd, s.gap = false, 0, false, false
		if s.mode == rbCode {
			s.canRegexp, s.command, s.statement = true, false, true
		}
	}
	return w.result(i, s), nil
}
