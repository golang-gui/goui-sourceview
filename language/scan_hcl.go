package language

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

type hcMode uint8

const (
	hcCode hcMode = iota
	hcQuoted
	hcLineComment
	hcBlockComment
	hcHereHead
	hcHereBody
	hcNumber
)

type hcExpr struct {
	parent *hcExpr
	mode   hcMode
	here   string
	depth  int
}
type hcState struct {
	mode                                   hcMode
	expr                                   *hcExpr
	word, pending                          scanWord
	wordOn, pendingOn                      bool
	capture                                *scanText
	here                                   string
	hereGap, matching, beginning, trailing bool
	matched                                int
	escapeLeft                             int
	escape                                 bool
	last                                   byte
}

func (s hcState) Equal(other State) bool {
	t, ok := other.(hcState)
	if !ok {
		return false
	}
	for a, b := s.expr, t.expr; a != b; a, b = a.parent, b.parent {
		if a == nil || b == nil || a.mode != b.mode || a.here != b.here || a.depth != b.depth {
			return false
		}
	}
	if s.capture != t.capture && captureText(s.capture) != captureText(t.capture) {
		return false
	}
	s.expr, t.expr = nil, nil
	s.capture, t.capture = nil, nil
	return s == t
}
func (s hcState) Context() Context {
	return Context{String: s.mode == hcQuoted || s.mode == hcHereHead || s.mode == hcHereBody, Literal: s.mode == hcHereBody, Comment: s.mode == hcLineComment || s.mode == hcBlockComment}
}

type hcTokenizer struct{}

func (hcTokenizer) InitialState() State { return hcState{} }

var hcKeywords = words("true false null for in if else endif endfor resource data variable output locals module provider terraform backend required_providers required_version dynamic content provisioner connection moved removed import check assert validation lifecycle depends_on count for_each bool number string list set map object tuple any")

func hcName(r rune) bool { return IdentifierRune(r) || r == '-' }

func (hcTokenizer) Scan(in Input, state State) (Result, error) {
	s, ok := state.(hcState)
	if !ok {
		return Result{}, fmt.Errorf("language: foreign HCL state")
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
		kind := KindPlain
		if hcKeywords[s.word.text] {
			kind = KindKeyword
		}
		w.add(s.word.start, end, kind, 0)
		s.pending, s.pendingOn = s.word, true
		s.word, s.wordOn = scanWord{}, false
	}
	var captured strings.Builder
	flush := func() {
		if captured.Len() > 0 {
			s.capture = &scanText{previous: s.capture, text: captured.String()}
			captured.Reset()
		}
	}
	for i < limit {
		c := text[i]
		r, n := utf8.DecodeRuneInString(text[i:])
		switch s.mode {
		case hcLineComment:
			add(i, i+n, KindComment, ClassComment)
			i += n
			continue
		case hcBlockComment:
			if strings.HasPrefix(text[i:], "*/") {
				add(i, i+2, KindComment, ClassComment)
				i += 2
				s.mode = hcCode
				continue
			}
			add(i, i+n, KindComment, ClassComment)
			i += n
			continue
		case hcHereHead:
			if hcName(r) && !s.hereGap {
				captured.WriteString(text[i : i+n])
			} else if c == ' ' || c == '\t' || c == '\r' {
				s.hereGap = true
			} else {
				s.mode, s.capture, s.hereGap = hcCode, nil, false
				captured.Reset()
				continue
			}
			add(i, i+n, KindString, ClassString)
			i += n
			continue
		case hcQuoted, hcHereBody:
			if s.mode == hcHereBody && s.matching {
				for j := i; j < i+n; j++ {
					b := text[j]
					if s.beginning && (b == ' ' || b == '\t') {
						continue
					}
					s.beginning = false
					if s.matched == len(s.here) && (b == ' ' || b == '\t' || b == '\r') {
						s.trailing = true
						continue
					}
					if s.trailing || s.matched >= len(s.here) || s.here[s.matched] != b {
						s.matching = false
						break
					}
					s.matched++
				}
			}
			if s.escapeLeft > 0 {
				if hex(c) {
					add(i, i+1, KindEscape, ClassString)
					s.escapeLeft--
					i++
					continue
				}
				s.escapeLeft = 0
			}
			if s.escape {
				s.escape = false
				if c == 'u' {
					s.escapeLeft = 4
				}
				if c == 'U' {
					s.escapeLeft = 8
				}
				add(i, i+n, KindEscape, ClassString)
				i += n
				continue
			}
			if s.mode == hcQuoted && c == '\\' {
				s.escape = true
				add(i, i+1, KindEscape, ClassString)
				i++
				continue
			}
			if strings.HasPrefix(text[i:], "$${") || strings.HasPrefix(text[i:], "%%{") {
				add(i, i+3, KindEscape, ClassString)
				i += 3
				s.matching = false
				continue
			}
			if strings.HasPrefix(text[i:], "${") || strings.HasPrefix(text[i:], "%{") {
				s.expr = &hcExpr{parent: s.expr, mode: s.mode, here: s.here}
				s.mode, s.here, s.matching, s.beginning, s.trailing, s.matched = hcCode, "", false, false, false, 0
				add(i, i+2, KindDelimiter, ClassDelimiter)
				i += 2
				continue
			}
			if s.mode == hcQuoted && c == '"' {
				s.mode = hcCode
			}
			add(i, i+n, KindString, ClassString)
			i += n
			continue
		case hcNumber:
			if digit(c) || c == '.' && i+1 < len(text) && digit(text[i+1]) || c == 'e' || c == 'E' || (c == '+' || c == '-') && (s.last == 'e' || s.last == 'E') {
				add(i, i+1, KindNumber, 0)
				s.last = c
				i++
				continue
			}
			s.mode, s.last = hcCode, 0
		}
		if s.wordOn {
			if hcName(r) {
				s.word.add(text[i : i+n])
				i += n
				continue
			}
			finishWord(in.Offset + i)
			continue
		}
		if space(c) {
			i += n
			continue
		}
		if s.pendingOn {
			if c == '=' && !strings.HasPrefix(text[i:], "==") {
				w.add(s.pending.start, s.pending.start+s.pending.length, KindProperty, 0)
			}
			s.pending, s.pendingOn = scanWord{}, false
		}
		if c == '#' || strings.HasPrefix(text[i:], "//") {
			s.mode = hcLineComment
			continue
		}
		if strings.HasPrefix(text[i:], "/*") {
			s.mode = hcBlockComment
			add(i, i+2, KindComment, ClassComment)
			i += 2
			continue
		}
		if c == '"' {
			s.mode = hcQuoted
			add(i, i+1, KindString, ClassString)
			i++
			continue
		}
		if strings.HasPrefix(text[i:], "<<") {
			head := 2
			if len(text) > i+head && text[i+head] == '-' {
				head++
			}
			if len(text) > i+head {
				next, _ := utf8.DecodeRuneInString(text[i+head:])
				if codeNameStart(HCL, next) {
					s.mode, s.capture, s.hereGap = hcHereHead, nil, false
					add(i, i+head, KindString, ClassString)
					i += head
					continue
				}
			}
		}
		if digit(c) {
			s.mode, s.last = hcNumber, c
			add(i, i+1, KindNumber, 0)
			i++
			continue
		}
		if codeNameStart(HCL, r) {
			s.word, s.wordOn = scanWord{start: in.Offset + i}, true
			s.word.add(text[i : i+n])
			i += n
			continue
		}
		if c == '}' && s.expr != nil && s.expr.depth == 0 {
			f := s.expr
			s.mode, s.here, s.expr = f.mode, f.here, f.parent
			s.matching, s.beginning, s.trailing, s.matched = false, false, false, 0
			add(i, i+1, KindDelimiter, ClassDelimiter)
			i++
			continue
		}
		if s.expr != nil && (c == '{' || c == '}') {
			f := *s.expr
			if c == '{' {
				f.depth++
			} else {
				f.depth--
			}
			s.expr = &f
		}
		class := Class(0)
		if strings.ContainsRune("()[]{}", r) {
			class = ClassDelimiter
		}
		add(i, i+n, KindDelimiter, class)
		i += n
	}
	flush()
	if in.LineEnd && i == len(text) {
		finishWord(in.Offset + i)
		s.pending, s.pendingOn = scanWord{}, false
		if s.mode == hcHereHead {
			s.here = captureText(s.capture)
			s.capture, s.hereGap = nil, false
			if s.here != "" {
				s.mode = hcHereBody
			} else {
				s.mode = hcCode
			}
		} else if s.mode == hcHereBody && s.matching && s.matched == len(s.here) {
			s.mode, s.here = hcCode, ""
		}
		if s.mode == hcLineComment || s.mode == hcNumber {
			s.mode = hcCode
		}
		s.matching, s.beginning, s.trailing, s.matched = s.mode == hcHereBody, true, false, 0
		s.escape, s.escapeLeft, s.last = false, 0, 0
	}
	return w.result(i, s), nil
}
