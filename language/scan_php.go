package language

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

type phpMode uint8

const (
	phpCode phpMode = iota
	phpQuoted
	phpVariable
	phpLineComment
	phpBlockComment
	phpNumber
	phpHereOpen
	phpHereBody
)

type phpHere struct {
	label   string
	literal bool
}
type phpFrame struct {
	parent *phpFrame
	mode   phpMode
	quote  byte
	here   *phpHere
	braces int
}
type phpState struct {
	html                builtinState
	htmlDefinition      *Language
	inCode, fragment    bool
	mode, back          phpMode
	quote               byte
	escape              bool
	frames              *phpFrame
	here                *phpHere
	matching, beginning bool
	matched             int
	word                scanWord
	wordOn              bool
	last                byte
	capture             *scanText
	header              uint8
	headerQuote         byte
}

func phpHeresEqual(a, b *phpHere) bool { return a == b || a != nil && b != nil && *a == *b }
func (s phpState) Equal(other State) bool {
	t, ok := other.(phpState)
	if !ok || !s.html.Equal(t.html) || !phpHeresEqual(s.here, t.here) || s.capture != t.capture && captureText(s.capture) != captureText(t.capture) {
		return false
	}
	for a, b := s.frames, t.frames; a != b; a, b = a.parent, b.parent {
		if a == nil || b == nil || a.mode != b.mode || a.quote != b.quote || a.braces != b.braces || !phpHeresEqual(a.here, b.here) {
			return false
		}
	}
	s.html, t.html = builtinState{}, builtinState{}
	s.frames, t.frames = nil, nil
	s.here, t.here = nil, nil
	s.capture, t.capture = nil, nil
	return s == t
}
func (s phpState) Context() Context {
	if !s.inCode {
		c := s.html.Context()
		if c.Embedded == nil {
			c.Embedded = s.htmlDefinition
		}
		return c
	}
	m := s.mode
	if m == phpVariable {
		m = s.back
	}
	return Context{String: m == phpQuoted || m == phpHereBody, Literal: m == phpHereBody && s.here != nil && s.here.literal, Comment: m == phpLineComment || m == phpBlockComment}
}

type phpTokenizer struct {
	fragment bool
	html     *Language
}

func (b phpTokenizer) InitialState() State {
	return phpState{html: initialBuiltin(), htmlDefinition: b.html, inCode: b.fragment, fragment: b.fragment}
}

var phpKeywords = words("__halt_compiler abstract and array as break callable case catch class clone const continue declare default die do echo else elseif empty enddeclare endfor endforeach endif endswitch endwhile enum eval exit extends final finally fn for foreach from function global goto if implements include include_once instanceof insteadof interface isset list match namespace new or print private protected public readonly require require_once return static switch throw trait try unset use var while xor yield true false null self parent int float bool string void iterable object mixed never get set")

func phpName(r rune) bool { return IdentifierRune(r) || r >= utf8.RuneSelf }
func phpOpening(text string) int {
	if strings.HasPrefix(text, "<?=") {
		return 3
	}
	if len(text) >= 5 && strings.EqualFold(text[:5], "<?php") && (len(text) == 5 || space(text[5]) || text[5] == '\n') {
		return 5
	}
	return 0
}

func (b phpTokenizer) Scan(in Input, state State) (Result, error) {
	s, ok := state.(phpState)
	if !ok || s.fragment != b.fragment || s.htmlDefinition != b.html {
		return Result{}, fmt.Errorf("language: foreign PHP state")
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
		kind := KindPlain
		if phpKeywords[strings.ToLower(s.word.text)] {
			kind = KindKeyword
		}
		w.add(s.word.start, end, kind, 0)
		s.word, s.wordOn = scanWord{}, false
	}
	endHere := func() { s.mode, s.here, s.matching, s.beginning, s.matched = phpCode, nil, false, false, 0 }
	for i < limit {
		if !s.inCode {
			if n := phpOpening(text[i:]); n > 0 {
				// Flush a suspended JS/CSS word before the PHP span so a later
				// classification can never overwrite the intervening PHP tokens.
				if s.html.code != nil && s.html.code.wordOn {
					code := *s.html.code
					finishCodeWord(&code, &w, in.Offset+i)
					s.html.code = &code
				}
				add(i, i+n, KindMeta, 0)
				i += n
				s.inCode = true
				continue
			}
			end := i + 1
			for end < len(text) && phpOpening(text[end:]) == 0 {
				end++
			}
			part := Input{Text: text[i:end], Offset: in.Offset + i, Limit: min(limit-i, end-i), LineEnd: in.LineEnd && end == len(text), DocumentEnd: in.DocumentEnd && end == len(text)}
			r, err := (builtinTokenizer{HTML}).Scan(part, s.html)
			if err != nil {
				return Result{}, err
			}
			w.merge(r)
			s.html = r.State.(builtinState)
			i += r.Consumed
			continue
		}
		c := text[i]
		r, n := utf8.DecodeRuneInString(text[i:])
		switch s.mode {
		case phpLineComment:
			if strings.HasPrefix(text[i:], "?>") {
				s.mode = phpCode
				continue
			}
			add(i, i+n, KindComment, ClassComment)
			i += n
			continue
		case phpBlockComment:
			if strings.HasPrefix(text[i:], "*/") {
				add(i, i+2, KindComment, ClassComment)
				i += 2
				s.mode = phpCode
				continue
			}
			add(i, i+n, KindComment, ClassComment)
			i += n
			continue
		case phpVariable:
			if phpName(r) {
				class := Class(0)
				if s.back == phpQuoted || s.back == phpHereBody {
					class = ClassString
				}
				add(i, i+n, KindVariable, class)
				i += n
				continue
			}
			s.mode, s.back = s.back, phpCode
			continue
		case phpHereOpen:
			if s.header == 0 {
				if c == ' ' || c == '\t' {
					add(i, i+1, KindString, ClassString)
					i++
					continue
				}
				if c == '\'' || c == '"' {
					s.headerQuote = c
					s.header = 1
					add(i, i+1, KindString, ClassString)
					i++
					continue
				}
				if phpName(r) && !digit(c) {
					s.header = 1
					continue
				}
			} else if s.header == 1 {
				if phpName(r) {
					capture.WriteString(text[i : i+n])
					add(i, i+n, KindString, ClassString)
					i += n
					continue
				}
				if s.headerQuote != 0 && c == s.headerQuote {
					s.header = 2
					add(i, i+1, KindString, ClassString)
					i++
					continue
				}
				if s.headerQuote == 0 && (c == ' ' || c == '\t' || c == '\r') {
					s.header = 2
					continue
				}
			} else if c == ' ' || c == '\t' || c == '\r' {
				add(i, i+1, KindString, ClassString)
				i++
				continue
			}
			s.mode, s.header, s.headerQuote, s.capture = phpCode, 0, 0, nil
			capture.Reset()
			continue
		case phpQuoted, phpHereBody:
			if s.mode == phpHereBody && s.matching {
				if s.beginning && (c == ' ' || c == '\t') {
					add(i, i+1, KindString, ClassString)
					i++
					continue
				}
				s.beginning = false
				if s.matched == len(s.here.label) {
					if !phpName(r) {
						endHere()
						continue
					}
					s.matching = false
				} else if strings.HasPrefix(s.here.label[s.matched:], text[i:i+n]) {
					s.matched += n
					add(i, i+n, KindString, ClassString)
					i += n
					continue
				} else {
					s.matching = false
				}
			}
			interp := s.mode == phpQuoted && s.quote != '\'' || s.mode == phpHereBody && !s.here.literal
			if s.escape {
				add(i, i+n, KindEscape, ClassString)
				s.escape = false
				i += n
				continue
			}
			if c == '\\' && (interp || s.mode == phpQuoted && i+1 < len(text) && (text[i+1] == '\\' || text[i+1] == '\'')) {
				add(i, i+1, KindEscape, ClassString)
				s.escape = true
				i++
				continue
			}
			if interp && (strings.HasPrefix(text[i:], "{$") || strings.HasPrefix(text[i:], "${")) {
				s.frames = &phpFrame{parent: s.frames, mode: s.mode, quote: s.quote, here: s.here, braces: 1}
				s.mode, s.quote, s.here = phpCode, 0, nil
				n = 1
				if c == '$' {
					n = 2
				}
				add(i, i+n, KindDelimiter, ClassDelimiter)
				i += n
				continue
			}
			if interp && c == '$' {
				s.back, s.mode = s.mode, phpVariable
				add(i, i+1, KindVariable, ClassString)
				i++
				continue
			}
			if s.mode == phpQuoted && c == s.quote {
				s.mode, s.quote = phpCode, 0
			}
			add(i, i+n, KindString, ClassString)
			i += n
			continue
		case phpNumber:
			if phpName(r) || c == '.' && !strings.HasPrefix(text[i:], "..") || (c == '+' || c == '-') && strings.ContainsRune("eE", rune(s.last)) {
				add(i, i+n, KindNumber, 0)
				s.last = c
				i += n
				continue
			}
			s.mode, s.last = phpCode, 0
		}
		if s.wordOn {
			if phpName(r) {
				s.word.add(text[i : i+n])
				i += n
				continue
			}
			if (s.word.text == "b" || s.word.text == "B") && (c == '\'' || c == '"') {
				w.add(s.word.start, in.Offset+i+1, KindString, ClassString)
				s.word, s.wordOn = scanWord{}, false
				s.mode, s.quote = phpQuoted, c
				i++
				continue
			}
			finishWord(in.Offset + i)
		}
		if strings.HasPrefix(text[i:], "?>") {
			add(i, i+2, KindMeta, 0)
			i += 2
			s.inCode = false
			s.frames = nil
			continue
		}
		if n := phpOpening(text[i:]); n > 0 {
			add(i, i+n, KindMeta, 0)
			i += n
			continue
		}
		if c == ' ' || c == '\t' || c == '\r' {
			i++
			continue
		}
		if strings.HasPrefix(text[i:], "#[") {
			add(i, i+1, KindMeta, 0)
			add(i+1, i+2, KindDelimiter, ClassDelimiter)
			i += 2
			continue
		}
		if c == '#' || strings.HasPrefix(text[i:], "//") {
			s.mode = phpLineComment
			continue
		}
		if strings.HasPrefix(text[i:], "/*") {
			s.mode = phpBlockComment
			add(i, i+2, KindComment, ClassComment)
			i += 2
			continue
		}
		if strings.HasPrefix(text[i:], "<<<") {
			s.mode = phpHereOpen
			add(i, i+3, KindString, ClassString)
			i += 3
			continue
		}
		if c == '\'' || c == '"' || c == '`' {
			s.mode, s.quote = phpQuoted, c
			add(i, i+1, KindString, ClassString)
			i++
			continue
		}
		if c == '$' {
			s.mode, s.back = phpVariable, phpCode
			add(i, i+1, KindVariable, 0)
			i++
			continue
		}
		if digit(c) || c == '.' && i+1 < len(text) && digit(text[i+1]) {
			s.mode, s.last = phpNumber, c
			add(i, i+1, KindNumber, 0)
			i++
			continue
		}
		if phpName(r) {
			s.word, s.wordOn = scanWord{start: in.Offset + i}, true
			s.word.add(text[i : i+n])
			i += n
			continue
		}
		if f := s.frames; f != nil && (c == '{' || c == '}') {
			next := *f
			if c == '{' {
				next.braces++
			} else {
				next.braces--
			}
			if next.braces == 0 {
				s.mode, s.quote, s.here, s.frames = f.mode, f.quote, f.here, f.parent
			} else {
				s.frames = &next
			}
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
		if s.mode == phpVariable {
			s.mode, s.back = s.back, phpCode
		}
		if s.mode == phpLineComment || s.mode == phpNumber {
			s.mode = phpCode
		}
		if s.mode == phpHereBody && s.matching && s.matched == len(s.here.label) {
			endHere()
		}
		if s.mode == phpHereOpen {
			label := captureText(s.capture)
			if label != "" && (s.header == 2 || s.header == 1 && s.headerQuote == 0) {
				s.here = &phpHere{label: label, literal: s.headerQuote == '\''}
				s.mode = phpHereBody
			} else {
				s.mode = phpCode
			}
			s.header, s.headerQuote, s.capture = 0, 0, nil
		}
		s.matching, s.beginning, s.matched = s.mode == phpHereBody, true, 0
		s.escape, s.last = false, 0
	}
	return w.result(i, s), nil
}
