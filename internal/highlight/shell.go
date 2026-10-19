package highlight

import (
	"strings"

	"github.com/golang-gui/goui-sourceview/language"
)

type shMode uint8

const (
	shCode shMode = iota
	shSingle
	shDouble
	shANSI
	shCommand
	shArithmetic
	shParameter
	shBacktick
	shVariable
)

type shFrame struct {
	parent *shFrame
	mode   shMode
	depth  int
}

// Delimiter spelling is immutable chunks, with no fixed maximum length.
type shText struct {
	previous *shText
	text     string
}
type shDelimiter struct {
	parts  []string
	length int
}
type shHere struct {
	delimiter     *shDelimiter
	literal, tabs bool
	owner         int
}
type shellLexical struct {
	bodyParameter                                  int
	wordPart, bodyVariable, bodyEscape             bool
	comment, escape, continued                     bool
	word                                           atom
	wordOn                                         bool
	capture, started, literal, tabs, captureEscape bool
	captureQuote                                   byte
	owner                                          int
	// A candidate terminator is compared as fragments arrive, then reclassified.
	matching, beginning             bool
	matchPart, matchByte, lineBytes int
}
type shellState struct {
	shellLexical
	stack    *shFrame
	pending  []shHere
	active   *shHere
	spelling *shText
}

func shDepth(p *shFrame) int {
	n := 0
	for p != nil {
		n++
		p = p.parent
	}
	return n
}
func shTop(p *shFrame) shMode {
	if p == nil {
		return shCode
	}
	return p.mode
}
func (s *shellState) push(mode shMode) { s.stack = &shFrame{parent: s.stack, mode: mode} }
func (s *shellState) pop() {
	if s.stack != nil {
		s.stack = s.stack.parent
	}
}
func equalDelimiter(a, b *shDelimiter) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil || a.length != b.length {
		return false
	}
	ai, aj, bi, bj := 0, 0, 0, 0
	for ai < len(a.parts) && bi < len(b.parts) {
		if a.parts[ai][aj] != b.parts[bi][bj] {
			return false
		}
		aj++
		bj++
		if aj == len(a.parts[ai]) {
			ai++
			aj = 0
		}
		if bj == len(b.parts[bi]) {
			bi++
			bj = 0
		}
	}
	return ai == len(a.parts) && bi == len(b.parts)
}
func equalHere(a, b shHere) bool {
	return a.literal == b.literal && a.tabs == b.tabs && a.owner == b.owner && equalDelimiter(a.delimiter, b.delimiter)
}
func equalShell(a, b *shellState) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	x, y := *a, *b
	ap, bp := x.stack, y.stack
	x.stack, y.stack = nil, nil
	for ap != bp {
		if ap == nil || bp == nil || ap.mode != bp.mode || ap.depth != bp.depth {
			return false
		}
		ap, bp = ap.parent, bp.parent
	}
	if (x.active == nil) != (y.active == nil) {
		return false
	}
	if x.active != nil && !equalHere(*x.active, *y.active) {
		return false
	}
	x.active, y.active = nil, nil
	if len(x.pending) != len(y.pending) {
		return false
	}
	for i := range x.pending {
		if !equalHere(x.pending[i], y.pending[i]) {
			return false
		}
	}
	x.pending, y.pending = nil, nil
	// Capture chunks depend on fragmentation. Compare their spelling, not layout.
	ac, bc := x.spelling, y.spelling
	x.spelling, y.spelling = nil, nil
	if ac != bc {
		if captureText(ac) != captureText(bc) {
			return false
		}
	}
	return x.shellLexical == y.shellLexical
}
func captureText(p *shText) string {
	var parts []string
	for p != nil {
		parts = append(parts, p.text)
		p = p.previous
	}
	var b strings.Builder
	for i := len(parts) - 1; i >= 0; i-- {
		b.WriteString(parts[i])
	}
	return b.String()
}
func (s *shellState) activate() {
	depth := shDepth(s.stack)
	for i, d := range s.pending {
		if d.owner == depth {
			copyD := d
			s.active = &copyD
			s.pending = append(s.pending[:i], s.pending[i+1:]...)
			s.matching = true
			s.beginning = true
			s.matchPart = 0
			s.matchByte = 0
			s.lineBytes = 0
			return
		}
	}
}
func (s *shellState) match(c byte) {
	s.lineBytes++
	if s.beginning && s.active.tabs && c == '\t' {
		return
	}
	s.beginning = false
	if !s.matching {
		return
	}
	d := s.active.delimiter
	if s.matchPart >= len(d.parts) || d.parts[s.matchPart][s.matchByte] != c {
		s.matching = false
		return
	}
	s.matchByte++
	if s.matchByte == len(d.parts[s.matchPart]) {
		s.matchPart++
		s.matchByte = 0
	}
}
func shellName(c byte) bool { return alpha(c) || digit(c) || c == '_' }
func shellKeyword(word string) bool {
	switch word {
	case "if", "then", "else", "elif", "fi", "for", "while", "until", "do", "done", "case", "esac", "in", "function", "select", "time", "coproc", "!", "[[", "]]":
		return true
	}
	return false
}
func scanShell(state State, input string, limit int, lineEnd bool) ([]Token, State, int) {
	s := shellState{}
	if state.shell != nil {
		s = *state.shell
		s.pending = append([]shHere(nil), s.pending...)
	}
	var out []Token
	i := 0
	var captured strings.Builder
	flushCapture := func() {
		if captured.Len() > 0 {
			s.spelling = &shText{previous: s.spelling, text: captured.String()}
			captured.Reset()
		}
	}
	finishCapture := func() {
		flushCapture()
		var parts []string
		n := 0
		for p := s.spelling; p != nil; p = p.previous {
			parts = append(parts, p.text)
			n += len(p.text)
		}
		for a, b := 0, len(parts)-1; a < b; a, b = a+1, b-1 {
			parts[a], parts[b] = parts[b], parts[a]
		}
		s.pending = append(s.pending, shHere{&shDelimiter{parts, n}, s.literal, s.tabs, s.owner})
		s.capture = false
		s.spelling = nil
		s.started = false
		s.captureQuote = 0
		s.captureEscape = false
	}
	finishWord := func(assignment bool) {
		if s.wordOn {
			k := language.KindPlain
			if s.word.n <= 128 && shellKeyword(s.word.text) {
				k = language.KindKeyword
			}
			if shTop(s.stack) == shArithmetic && s.word.numeric() {
				k = language.KindNumber
			}
			if assignment {
				k = language.KindVariable
			}
			emit(&out, i-s.word.n, i, k)
			s.word = atom{}
			s.wordOn = false
		}
	}
	for i < limit {
		c := input[i]
		mode := shTop(s.stack)
		if s.active != nil {
			s.match(c)
			kind := language.KindString
			if !s.active.literal {
				if s.bodyEscape {
					s.bodyEscape = false
				} else if c == '\\' {
					s.bodyEscape = true
					s.bodyVariable = false
				} else if s.bodyParameter > 0 {
					kind = language.KindVariable
					if c == '{' {
						s.bodyParameter++
					}
					if c == '}' {
						s.bodyParameter--
					}
				} else if s.bodyVariable && c == '{' {
					s.bodyVariable = false
					s.bodyParameter = 1
					kind = language.KindVariable
				} else if c == '$' {
					s.bodyVariable = true
					kind = language.KindVariable
				} else if s.bodyVariable && shellName(c) {
					kind = language.KindVariable
				} else {
					s.bodyVariable = false
				}
			}
			n := runeSize(input, i)
			for j := 1; j < n; j++ {
				s.match(input[i+j])
			}
			emit(&out, i, i+n, kind)
			i += n
			continue
		}
		if s.capture {
			if !s.started && space(c) {
				i++
				continue
			}
			if s.captureEscape {
				captured.WriteByte(c)
				s.captureEscape = false
				s.started = true
				emit(&out, i, i+1, language.KindMeta)
				i++
				continue
			}
			if c == '\\' && (s.captureQuote == 0 || s.captureQuote == '"' && (i+1 == len(input) || strings.ContainsRune("$`\"\\", rune(input[i+1])))) {
				s.captureEscape = true
				s.literal = true
				s.started = true
				emit(&out, i, i+1, language.KindMeta)
				i++
				continue
			}
			if s.captureQuote != 0 {
				if c == s.captureQuote {
					s.captureQuote = 0
				} else {
					captured.WriteByte(c)
				}
				emit(&out, i, i+1, language.KindMeta)
				i++
				continue
			}
			if c == '\'' || c == '"' {
				s.captureQuote = c
				s.literal = true
				s.started = true
				emit(&out, i, i+1, language.KindMeta)
				i++
				continue
			}
			if space(c) || strings.ContainsRune(";&|()<>", rune(c)) {
				finishCapture()
				continue
			}
			captured.WriteByte(c)
			s.started = true
			emit(&out, i, i+1, language.KindMeta)
			i++
			continue
		}
		if s.comment {
			emit(&out, i, limit, language.KindComment)
			i = limit
			continue
		}
		if s.escape {
			n := runeSize(input, i)
			emit(&out, i, i+n, language.KindEscape)
			s.escape = false
			i += n
			continue
		}
		if mode == shVariable {
			if shellName(c) {
				emit(&out, i, i+1, language.KindVariable)
				i++
				continue
			}
			s.pop()
			continue
		}
		if mode == shSingle || mode == shANSI || mode == shDouble {
			end := byte('\'')
			if mode == shDouble {
				end = '"'
			}
			if c == end {
				emit(&out, i, i+1, language.KindString)
				s.pop()
				i++
				continue
			}
			if c == '\\' && mode != shSingle && (mode == shANSI || i+1 == len(input) || strings.ContainsRune("$`\"\\", rune(input[i+1]))) {
				emit(&out, i, i+1, language.KindEscape)
				s.escape = true
				i++
				continue
			}
			if mode != shDouble || c != '$' && c != '`' {
				n := runeSize(input, i)
				emit(&out, i, i+n, language.KindString)
				i += n
				continue
			}
		}
		if c == '$' {
			finishWord(false)
			switch {
			case strings.HasPrefix(input[i:], "$(("):
				emit(&out, i, i+3, language.KindDelimiter)
				s.push(shArithmetic)
				i += 3
			case strings.HasPrefix(input[i:], "$("):
				emit(&out, i, i+2, language.KindDelimiter)
				s.push(shCommand)
				i += 2
			case strings.HasPrefix(input[i:], "${"):
				emit(&out, i, i+2, language.KindDelimiter)
				s.push(shParameter)
				i += 2
			case mode != shDouble && strings.HasPrefix(input[i:], "$'"):
				emit(&out, i, i+2, language.KindString)
				s.push(shANSI)
				i += 2
			default:
				emit(&out, i, i+1, language.KindVariable)
				i++
				if i < len(input) && shellName(input[i]) {
					s.push(shVariable)
				} else if i < len(input) && strings.ContainsRune("@*#?$!-0123456789", rune(input[i])) {
					emit(&out, i, i+1, language.KindVariable)
					i++
				}
			}
			continue
		}
		if c == '`' {
			finishWord(false)
			emit(&out, i, i+1, language.KindDelimiter)
			if mode == shBacktick {
				s.pop()
			} else {
				s.push(shBacktick)
			}
			i++
			continue
		}
		if c == '\\' {
			finishWord(false)
			emit(&out, i, i+1, language.KindEscape)
			s.escape = true
			i++
			continue
		}
		if c == '\'' || c == '"' {
			s.wordPart = true
			finishWord(false)
			emit(&out, i, i+1, language.KindString)
			if c == '\'' {
				s.push(shSingle)
			} else {
				s.push(shDouble)
			}
			i++
			continue
		}
		if c == '#' && !s.wordPart && mode != shParameter && mode != shArithmetic {
			s.comment = true
			continue
		}
		if mode == shArithmetic && strings.HasPrefix(input[i:], "))") && s.stack.depth == 0 {
			finishWord(false)
			emit(&out, i, i+2, language.KindDelimiter)
			s.pop()
			i += 2
			continue
		}
		if mode == shParameter && c == '}' {
			finishWord(false)
			emit(&out, i, i+1, language.KindDelimiter)
			s.pop()
			i++
			continue
		}
		if strings.HasPrefix(input[i:], "((") && mode != shArithmetic && mode != shParameter {
			finishWord(false)
			emit(&out, i, i+2, language.KindDelimiter)
			s.push(shArithmetic)
			i += 2
			continue
		}
		if c == '(' || c == ')' {
			finishWord(false)
			emit(&out, i, i+1, language.KindDelimiter)
			i++
			if mode == shCommand || mode == shArithmetic {
				p := *s.stack
				if c == '(' {
					p.depth++
					s.stack = &p
				} else if p.depth > 0 {
					p.depth--
					s.stack = &p
				} else {
					s.pop()
				}
			}
			continue
		}
		if strings.HasPrefix(input[i:], "<<<") && mode != shArithmetic {
			finishWord(false)
			emit(&out, i, i+3, language.KindDelimiter)
			i += 3
			s.wordPart = false
			continue
		}
		if c == '<' && mode != shArithmetic && strings.HasPrefix(input[i:], "<<") && !strings.HasPrefix(input[i:], "<<<") {
			finishWord(false)
			n := 2
			s.tabs = i+2 < len(input) && input[i+2] == '-'
			if s.tabs {
				n++
			}
			emit(&out, i, i+n, language.KindDelimiter)
			i += n
			s.capture = true
			s.started = false
			s.literal = false
			s.owner = shDepth(s.stack)
			continue
		}
		if space(c) || strings.ContainsRune(";&|<>={}[]", rune(c)) || (mode == shArithmetic || mode == shParameter) && strings.ContainsRune(":+-*/%!?", rune(c)) {
			finishWord(c == '=')
			s.wordPart = false
			if !space(c) {
				emit(&out, i, i+1, language.KindDelimiter)
			}
			i++
			continue
		}
		if mode == shParameter {
			n := runeSize(input, i)
			emit(&out, i, i+n, language.KindVariable)
			i += n
			continue
		}
		s.wordOn = true
		s.wordPart = true
		s.word.add(c)
		i++
	}
	flushCapture()
	if lineEnd && i == len(input) {
		if s.active != nil {
			if s.matching && s.matchPart == len(s.active.delimiter.parts) {
				appendToken(&out, i-s.lineBytes, i, language.KindMeta)
				s.active = nil
				s.activate()
			}
			s.bodyVariable = false
			s.bodyParameter = 0
			s.bodyEscape = false
			s.lineBytes = 0
			s.matchPart = 0
			s.matchByte = 0
			s.matching = true
			s.beginning = true
		} else {
			finishWord(false)
			s.wordPart = false
			s.continued = s.escape
			s.escape = false
			s.comment = false
			if shTop(s.stack) == shVariable {
				s.pop()
			}
			if s.capture && s.started && s.captureQuote == 0 && !s.captureEscape {
				finishCapture()
			}
			if !s.continued && !s.capture {
				s.activate()
			}
		}
	}
	state.shell = &s
	return out, state, i
}
