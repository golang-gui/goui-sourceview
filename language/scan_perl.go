package language

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

type plMode uint8

const (
	plCode plMode = iota
	plQuoted
	plQuotePending
	plSecondDelimiter
	plFlags
	plVariable
	plNumber
	plLineComment
	plPod
	plData
	plHereOpen
	plHereBody
)

type plLiteral struct {
	open, close         rune
	depth, parts        int
	op                  string
	interpolate, regexp bool
}
type plState struct {
	mode, back, commentBack, resumeMode               plMode
	literal, resumeLiteral                            plLiteral
	expect, resumeExpect, nameOnly, escape, hexNumber bool
	word                                              scanWord
	wordOn                                            bool
	last                                              byte
	pendingOp                                         string
	pendingStart, pendingEnd                          int
	pendingGap                                        bool
	varFirst, varControl                              bool
	varDepth                                          int
	podEnd                                            bool
	queue                                             *scanHereQueue
	next                                              int
	active                                            *scanHere
	capture                                           *scanText
	herePhase                                         int
	hereQuote                                         byte
	hereIndent, hereLiteral                           bool
	matching, beginning                               bool
	matched, matchStart                               int
}

func (s plState) Equal(other State) bool {
	t, ok := other.(plState)
	if !ok || !hereQueuesEqual(s.queue, t.queue) || !heresEqual(s.active, t.active) || s.capture != t.capture && captureText(s.capture) != captureText(t.capture) {
		return false
	}
	s.queue, t.queue = nil, nil
	s.active, t.active = nil, nil
	s.capture, t.capture = nil, nil
	return s == t
}
func (s plState) Context() Context {
	m := s.mode
	if m == plVariable {
		m = s.back
	}
	return Context{String: m == plQuoted || m == plSecondDelimiter || m == plFlags || m == plHereOpen || m == plHereBody || m == plData, Literal: m == plData || m == plHereBody && s.active != nil && s.active.literal, Comment: m == plLineComment || m == plPod}
}

type plTokenizer struct{}

func (plTokenizer) InitialState() State { return plState{expect: true} }

var plKeywords = words("ADJUST BEGIN CHECK END INIT UNITCHECK __DATA__ __END__ __FILE__ __LINE__ __PACKAGE__ and bless break caller catch chdir chmod chomp chop class close cmp continue default defined delete die do dump each else elsif eq eval exists exit field finally for foreach format given goto grep gt if index isa join keys last lc le length local lock lt m map method my ne next no not open or our package pop print printf prototype push q qq qr qw qx read readline redo ref require return reverse rindex s say scalar shift sort splice split sprintf state sub substr tr try uc undef unless unshift until use values wantarray warn when while write x xor y")

func plQuoteOp(s string) bool {
	return s == "q" || s == "qq" || s == "qw" || s == "qx" || s == "qr" || s == "m" || s == "s" || s == "tr" || s == "y"
}
func plClose(r rune) rune {
	switch r {
	case '(':
		return ')'
	case '[':
		return ']'
	case '{':
		return '}'
	case '<':
		return '>'
	}
	return r
}
func (t plTokenizer) Scan(in Input, state State) (Result, error) {
	return t.scan(in, state, nil)
}

func (plTokenizer) scan(in Input, state State, folds *foldLexemes) (Result, error) {
	s, ok := state.(plState)
	if !ok {
		return Result{}, fmt.Errorf("language: foreign Perl state")
	}
	limit, err := scanLimit(in)
	if err != nil {
		return Result{}, err
	}
	w := tokenWriter{base: in.Offset}
	text, i := in.Text, 0
	add := func(a, z int, k Kind, c Class) { w.add(in.Offset+a, in.Offset+z, k, c) }
	finishWord := func(end int) {
		if !s.wordOn {
			return
		}
		word := s.word
		kind := KindPlain
		if !s.nameOnly && plKeywords[word.text] {
			kind = KindKeyword
		}
		if !s.nameOnly && plQuoteOp(word.text) {
			folds.openLiteral() // Tentative until a delimiter is confirmed.
			s.mode, s.pendingOp, s.pendingStart, s.pendingEnd, s.pendingGap = plQuotePending, word.text, word.start, end, false
			kind = KindKeyword
		} else {
			if !s.nameOnly && (word.text == "__DATA__" || word.text == "__END__") {
				s.mode = plData
			}
			s.nameOnly = !s.nameOnly && strings.Contains(" sub package use no class method ", " "+word.text+" ")
			s.expect = kind == KindKeyword
		}
		w.add(word.start, end, kind, 0)
		s.word, s.wordOn = scanWord{}, false
	}
	var captured strings.Builder
	flush := func() {
		if captured.Len() > 0 {
			s.capture = &scanText{previous: s.capture, text: captured.String()}
			captured.Reset()
		}
	}
	finishHere := func() {
		flush()
		h := &scanHere{label: captureText(s.capture), indent: s.hereIndent, literal: s.hereLiteral || s.hereQuote == '\''}
		folds.openHere(h)
		s.queue = enqueueHere(s.queue, h)
		s.capture = nil
		s.herePhase, s.hereQuote, s.hereIndent, s.hereLiteral = 0, 0, false, false
		s.mode, s.expect = plCode, false
	}
	startQuote := func(op string, delimiter rune, parts int) {
		if s.mode != plQuotePending && s.mode != plSecondDelimiter {
			folds.openLiteral()
		}
		interp := op != "q" && op != "qw" && op != "tr" && op != "y"
		regex := op == "m" || op == "qr" || op == "s"
		if delimiter == '\'' && regex {
			interp = false
		}
		s.literal = plLiteral{open: delimiter, close: plClose(delimiter), depth: 1, parts: parts, op: op, interpolate: interp, regexp: regex}
		s.mode, s.escape = plQuoted, false
	}
	startVariable := func(n int) {
		s.back, s.mode, s.varFirst, s.varControl, s.varDepth = s.mode, plVariable, true, false, 0
		class := Class(0)
		if s.back == plQuoted || s.back == plHereBody {
			class = ClassString
		}
		add(i, i+n, KindVariable, class)
		i += n
	}
	for i < limit {
		c := text[i]
		r, n := utf8.DecodeRuneInString(text[i:])
		switch s.mode {
		case plData:
			add(i, i+n, KindString, ClassString)
			i += n
			continue
		case plLineComment:
			add(i, i+n, KindComment, ClassComment)
			i += n
			continue
		case plPod:
			if in.Offset+i == 0 && strings.HasPrefix(text[i:], "=cut") && (i+4 == len(text) || space(text[i+4])) {
				s.podEnd = true
			}
			add(i, i+n, KindComment, ClassComment)
			i += n
			continue
		case plVariable:
			class := Class(0)
			if s.back == plQuoted || s.back == plHereBody {
				class = ClassString
			}
			if s.varDepth > 0 {
				if c == '{' {
					s.varDepth++
				}
				if c == '}' {
					s.varDepth--
				}
				add(i, i+n, KindVariable, class)
				i += n
				if s.varDepth == 0 {
					s.mode = s.back
				}
				continue
			}
			if s.varControl {
				add(i, i+n, KindVariable, class)
				i += n
				s.varControl = false
				s.mode = s.back
				continue
			}
			if s.varFirst {
				s.varFirst = false
				if c == '{' {
					s.varDepth = 1
					add(i, i+1, KindVariable, class)
					i++
					continue
				}
				if c == '^' {
					s.varControl = true
					add(i, i+1, KindVariable, class)
					i++
					continue
				}
				if !IdentifierRune(r) && c != ':' {
					add(i, i+n, KindVariable, class)
					i += n
					if c != '#' {
						s.mode = s.back
					}
					continue
				}
			}
			if IdentifierRune(r) {
				add(i, i+n, KindVariable, class)
				i += n
				continue
			}
			if strings.HasPrefix(text[i:], "::") {
				add(i, i+2, KindVariable, class)
				i += 2
				continue
			}
			s.mode = s.back
			continue
		case plFlags:
			if c >= 'a' && c <= 'z' {
				add(i, i+1, KindMeta, ClassString)
				i++
				continue
			}
			folds.closeLiteral(in.Offset+i, true)
			s.mode = plCode
		case plQuotePending, plSecondDelimiter:
			if space(c) {
				s.pendingGap = true
				i++
				continue
			}
			if c == '#' && s.pendingGap {
				s.commentBack, s.mode = s.mode, plLineComment
				continue
			}
			if s.mode == plQuotePending && (strings.HasPrefix(text[i:], "=>") || strings.ContainsRune(")]};,", r)) {
				folds.closeLiteral(in.Offset+i, false)
				if s.pendingStart >= 0 {
					w.add(s.pendingStart, s.pendingEnd, KindPlain, 0)
				}
				s.mode, s.pendingOp, s.pendingStart, s.pendingEnd, s.pendingGap = plCode, "", 0, 0, false
				continue
			}
			op, parts := s.pendingOp, 1
			if s.mode == plQuotePending && (op == "s" || op == "tr" || op == "y") {
				parts = 2
			}
			if s.mode == plSecondDelimiter {
				op = s.literal.op
			}
			startQuote(op, r, parts)
			if parts == 1 && (op == "s" || op == "tr" || op == "y") {
				s.literal.regexp = false
			}
			s.pendingOp, s.pendingStart, s.pendingEnd, s.pendingGap = "", 0, 0, false
			add(i, i+n, KindString, ClassString)
			i += n
			continue
		case plQuoted, plHereBody:
			interp := s.literal.interpolate
			if s.mode == plHereBody {
				interp = s.active != nil && !s.active.literal
			}
			if s.escape {
				kind := KindEscape
				if !interp && s.mode == plHereBody {
					kind = KindString
				}
				s.escape = false
				add(i, i+n, kind, ClassString)
				i += n
				continue
			}
			var following rune
			if i+1 < len(text) {
				following, _ = utf8.DecodeRuneInString(text[i+1:])
			}
			if c == '\\' && (interp || s.mode == plQuoted && (following == '\\' || following == s.literal.close || following == s.literal.open)) {
				s.escape = true
				add(i, i+1, KindEscape, ClassString)
				i++
				continue
			}
			variable := IdentifierRune(following) || following == '{' || following == ':' || c == '$' && strings.ContainsRune("!$@#%&*?^|~=/\\.,;:+-<>[]", following)
			if s.mode == plQuoted && (following == s.literal.close || s.literal.regexp && strings.ContainsRune(")| \t", following)) {
				variable = false
			}
			if interp && (c == '$' || c == '@') && i+1 < len(text) && variable {
				startVariable(n)
				continue
			}
			if s.mode == plQuoted {
				// Perl locates quote delimiters before interpreting regex classes.
				if r == s.literal.close {
					s.literal.depth--
					add(i, i+n, KindString, ClassString)
					i += n
					if s.literal.depth == 0 {
						if s.literal.parts == 2 {
							if s.literal.open == s.literal.close {
								s.literal.parts, s.literal.depth, s.literal.regexp = 1, 1, false
							} else {
								s.mode, s.pendingGap = plSecondDelimiter, false
							}
						} else {
							op := s.literal.op
							s.literal = plLiteral{}
							s.mode, s.expect = plCode, false
							if op == "m" || op == "qr" || op == "s" || op == "tr" || op == "y" {
								s.mode = plFlags
							} else {
								folds.closeLiteral(in.Offset+i, true)
							}
						}
					}
					continue
				}
				if r == s.literal.open && s.literal.open != s.literal.close {
					s.literal.depth++
				}
			}
			add(i, i+n, KindString, ClassString)
			i += n
			continue
		case plHereOpen:
			if s.herePhase == 0 {
				if c == '~' && !s.hereIndent {
					s.hereIndent = true
					add(i, i+1, KindString, ClassString)
					i++
					continue
				}
				if c == ' ' || c == '\t' {
					i++
					continue
				}
				if c == '\\' && !s.hereLiteral {
					s.hereLiteral = true
					add(i, i+1, KindString, ClassString)
					i++
					continue
				}
				if c == '\'' || c == '"' || c == '`' {
					s.herePhase, s.hereQuote = 2, c
					add(i, i+1, KindString, ClassString)
					i++
					continue
				}
				if !codeNameStart(Perl, r) {
					s.mode, s.hereIndent, s.hereLiteral, s.hereQuote = plCode, false, false, 0
					continue
				}
				s.herePhase = 1
			}
			if s.herePhase == 2 && c == s.hereQuote {
				add(i, i+1, KindString, ClassString)
				i++
				finishHere()
				continue
			}
			if s.herePhase == 1 && !IdentifierRune(r) {
				finishHere()
				continue
			}
			captured.WriteString(text[i : i+n])
			add(i, i+n, KindString, ClassString)
			i += n
			continue
		case plNumber:
			if IdentifierRune(r) || c == '.' && i+1 < len(text) && (digit(text[i+1]) || s.hexNumber && hex(text[i+1])) && !strings.HasPrefix(text[i:], "..") || (c == '+' || c == '-') && (s.last == 'p' || s.last == 'P' || !s.hexNumber && (s.last == 'e' || s.last == 'E')) {
				if c == 'x' || c == 'X' || c == 'b' || c == 'B' || c == 'o' {
					s.hexNumber = true
				}
				s.last = c
				add(i, i+n, KindNumber, 0)
				i += n
				continue
			}
			s.mode, s.expect, s.hexNumber, s.last = plCode, false, false, 0
		}
		if s.wordOn {
			if IdentifierRune(r) {
				s.word.add(text[i : i+n])
				i += n
				continue
			}
			finishWord(in.Offset + i)
			if s.mode != plCode {
				continue
			}
		}
		if space(c) {
			i++
			continue
		}
		if in.Offset+i == 0 && c == '=' && i+1 < len(text) && ((text[i+1] >= 'a' && text[i+1] <= 'z') || (text[i+1] >= 'A' && text[i+1] <= 'Z')) {
			folds.openComment()
			s.mode = plPod
			continue
		}
		if c == '#' {
			s.mode, s.commentBack = plLineComment, plCode
			continue
		}
		if strings.HasPrefix(text[i:], "<<") && s.expect {
			s.mode, s.herePhase, s.hereQuote, s.hereIndent, s.hereLiteral, s.capture = plHereOpen, 0, 0, false, false, nil
			add(i, i+2, KindString, ClassString)
			i += 2
			continue
		}
		if c == '\'' || c == '"' || c == '`' {
			op := "qq"
			if c == '\'' {
				op = "q"
			}
			if c == '`' {
				op = "qx"
			}
			startQuote(op, r, 1)
			add(i, i+1, KindString, ClassString)
			i++
			continue
		}
		if c == '/' && s.expect {
			startQuote("m", '/', 1)
			add(i, i+1, KindString, ClassString)
			i++
			continue
		}
		if c == '$' || c == '@' || s.expect && (c == '%' || c == '&' || c == '*') && i+1 < len(text) && (codeNameStart(Perl, rune(text[i+1])) || text[i+1] == '{') {
			startVariable(n)
			s.expect = false
			continue
		}
		if strings.HasPrefix(text[i:], "->") || strings.HasPrefix(text[i:], "::") {
			add(i, i+2, KindDelimiter, 0)
			i += 2
			s.nameOnly, s.expect = true, false
			continue
		}
		if strings.HasPrefix(text[i:], "..") || strings.HasPrefix(text[i:], "//") {
			add(i, i+2, KindDelimiter, 0)
			i += 2
			s.expect = true
			continue
		}
		if digit(c) || c == '.' && i+1 < len(text) && digit(text[i+1]) {
			s.mode, s.last = plNumber, c
			add(i, i+1, KindNumber, 0)
			i++
			continue
		}
		if IdentifierRune(r) {
			s.word, s.wordOn = scanWord{start: in.Offset + i}, true
			s.word.add(text[i : i+n])
			i += n
			continue
		}
		class := Class(0)
		if strings.ContainsRune("()[]{}", r) {
			class = ClassDelimiter
		}
		add(i, i+n, KindDelimiter, class)
		i += n
		s.expect = !strings.ContainsRune(")]}", r)
		s.nameOnly = false
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
		if s.mode == plVariable {
			s.mode = s.back
			s.varFirst, s.varControl, s.varDepth = false, false, 0
		}
		finishWord(in.Offset + i)
		if s.mode == plHereOpen {
			if s.herePhase == 1 {
				finishHere()
			} else {
				s.mode, s.capture, s.herePhase, s.hereQuote, s.hereIndent, s.hereLiteral = plCode, nil, 0, 0, false, false
			}
		}
		if s.active != nil && s.matching && s.matched == len(s.active.label) {
			folds.closeHere(s.active, in.Offset+i)
			w.add(s.matchStart, in.Offset+i, KindString, ClassString)
			s.active = nil
			s.mode, s.literal, s.expect = s.resumeMode, s.resumeLiteral, s.resumeExpect
		}
		if s.mode == plLineComment {
			s.mode = s.commentBack
			s.commentBack = plCode
		}
		if s.mode == plPod && s.podEnd {
			folds.closeComment(in.Offset + i)
			s.mode, s.podEnd, s.expect = plCode, false, true
		}
		if s.mode == plNumber || s.mode == plFlags {
			if s.mode == plFlags {
				folds.closeLiteral(in.Offset+i, true)
			}
			s.mode, s.expect = plCode, false
		}
		if s.queue != nil && s.next < s.queue.n && s.active == nil {
			h := s.queue.at(s.next)
			s.next++
			if s.next == s.queue.n {
				s.queue, s.next = nil, 0
			}
			if s.mode != plHereBody {
				s.resumeMode, s.resumeLiteral, s.resumeExpect = s.mode, s.literal, s.expect
			}
			s.active, s.mode, s.literal = h, plHereBody, plLiteral{}
		}
		if s.active == nil {
			s.resumeMode, s.resumeLiteral, s.resumeExpect = plCode, plLiteral{}, false
		}
		if s.mode == plQuotePending {
			s.pendingStart, s.pendingEnd, s.pendingGap = -1, 0, true
		}
		s.matching, s.beginning, s.matched, s.matchStart = s.active != nil, true, 0, 0
		s.escape, s.hexNumber, s.last = false, false, 0
		s.back = plCode
	}
	return w.result(i, s), nil
}
