package language

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

type psMode uint8

const (
	psCode psMode = iota
	psQuoted
	psHereHeader
	psHereBody
	psLineComment
	psBlockComment
	psVariable
	psNumber
)

type psFrame struct {
	parent *psFrame
	mode   psMode
	quote  rune
	depth  int
}

type psState struct {
	mode, back     psMode
	quote          rune
	escape, braced bool
	frames         *psFrame
	word           scanWord
	wordOn         bool
	last           byte
}

func (s psState) Equal(other State) bool {
	t, ok := other.(psState)
	if !ok {
		return false
	}
	for a, b := s.frames, t.frames; a != b; a, b = a.parent, b.parent {
		if a == nil || b == nil || a.mode != b.mode || a.quote != b.quote || a.depth != b.depth {
			return false
		}
	}
	s.frames, t.frames = nil, nil
	return s == t
}
func (s psState) Context() Context {
	m := s.mode
	if m == psVariable {
		m = s.back
	}
	return Context{String: m == psQuoted || m == psHereHeader || m == psHereBody, Literal: (m == psQuoted || m == psHereHeader || m == psHereBody) && s.quote == '\'', Comment: m == psLineComment || m == psBlockComment}
}

type psTokenizer struct{}

func (psTokenizer) InitialState() State { return psState{} }

var psKeywords = words("begin break catch class clean configuration continue data define do dynamicparam else elseif end enum exit filter finally for foreach from function hidden if in inlinescript param process return static switch throw trap try until using var while workflow parallel sequence")
var psOperators = words("-and -or -xor -not -band -bor -bxor -bnot -shl -shr -eq -ne -gt -ge -lt -le -ceq -cne -cgt -cge -clt -cle -ieq -ine -igt -ige -ilt -ile -like -notlike -clike -cnotlike -ilike -inotlike -match -notmatch -cmatch -cnotmatch -imatch -inotmatch -contains -notcontains -ccontains -cnotcontains -icontains -inotcontains -in -notin -cin -cnotin -iin -inotin -is -isnot -as -replace -creplace -ireplace -split -csplit -isplit -join -f")

func psQuote(r rune) rune {
	switch r {
	case '\'', 0x2018, 0x2019:
		return '\''
	case '"', 0x201c, 0x201d:
		return '"'
	}
	return 0
}

func (t psTokenizer) Scan(in Input, state State) (Result, error) {
	return t.scan(in, state, nil)
}

func (psTokenizer) scan(in Input, state State, folds *foldLexemes) (Result, error) {
	s, ok := state.(psState)
	if !ok {
		return Result{}, fmt.Errorf("language: foreign PowerShell state")
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
		kind := s.word.kind
		word := strings.ToLower(s.word.text)
		if psKeywords[word] || psOperators[word] {
			kind = KindKeyword
		}
		w.add(s.word.start, end, kind, 0)
		s.word, s.wordOn = scanWord{}, false
	}
	for i < limit {
		c := text[i]
		r, n := utf8.DecodeRuneInString(text[i:])
		switch s.mode {
		case psLineComment:
			add(i, i+n, KindComment, ClassComment)
			i += n
			continue
		case psBlockComment:
			if strings.HasPrefix(text[i:], "#>") {
				folds.closeComment(in.Offset + i + 2)
				add(i, i+2, KindComment, ClassComment)
				i += 2
				s.mode = psCode
				continue
			}
			add(i, i+n, KindComment, ClassComment)
			i += n
			continue
		case psHereHeader:
			if c == ' ' || c == '\t' || c == '\r' {
				add(i, i+n, KindString, ClassString)
				i += n
				continue
			}
			folds.closeLiteral(in.Offset+i, false)
			s.mode, s.quote = psCode, 0
			continue
		case psVariable:
			class := Class(0)
			if s.back == psQuoted || s.back == psHereBody {
				class = ClassString
			}
			if s.braced {
				add(i, i+n, KindVariable, class)
				i += n
				if s.escape {
					s.escape = false
				} else if c == '`' {
					s.escape = true
				} else if c == '}' {
					s.mode, s.back, s.braced = s.back, psCode, false
				}
				continue
			}
			if IdentifierRune(r) || c == ':' {
				add(i, i+n, KindVariable, class)
				i += n
				continue
			}
			s.mode, s.back = s.back, psCode
			continue
		case psQuoted, psHereBody:
			if s.mode == psHereBody && in.Offset+i == 0 && psQuote(r) == s.quote && i+n < len(text) && text[i+n] == '@' {
				add(i, i+n+1, KindString, ClassString)
				i += n + 1
				folds.closeLiteral(in.Offset+i, true)
				s.mode, s.quote = psCode, 0
				continue
			}
			if s.escape {
				add(i, i+n, KindEscape, ClassString)
				s.escape = false
				i += n
				continue
			}
			if c == '`' && s.quote == '"' {
				add(i, i+1, KindEscape, ClassString)
				s.escape = true
				i++
				continue
			}
			if s.mode == psQuoted && psQuote(r) == s.quote {
				if i+n < len(text) {
					next, size := utf8.DecodeRuneInString(text[i+n:])
					if psQuote(next) == s.quote {
						add(i, i+n+size, KindEscape, ClassString)
						i += n + size
						continue
					}
				}
				add(i, i+n, KindString, ClassString)
				i += n
				folds.closeLiteral(in.Offset+i, true)
				s.mode, s.quote = psCode, 0
				continue
			}
			if c == '$' && s.quote == '"' {
				break
			}
			add(i, i+n, KindString, ClassString)
			i += n
			continue
		case psNumber:
			if IdentifierRune(r) || c == '.' && !strings.HasPrefix(text[i:], "..") || (c == '+' || c == '-') && strings.ContainsRune("eE", rune(s.last)) {
				add(i, i+n, KindNumber, 0)
				s.last = c
				i += n
				continue
			}
			s.mode, s.last = psCode, 0
		}
		if s.wordOn {
			if IdentifierRune(r) || c == '-' {
				s.word.add(text[i : i+n])
				i += n
				continue
			}
			finishWord(in.Offset + i)
		}
		if c == '$' {
			class := Class(0)
			if s.mode == psQuoted || s.mode == psHereBody {
				class = ClassString
			}
			if strings.HasPrefix(text[i:], "$(") {
				s.frames = &psFrame{parent: s.frames, mode: s.mode, quote: s.quote, depth: 1}
				s.mode, s.quote = psCode, 0
				add(i, i+2, KindDelimiter, ClassDelimiter)
				i += 2
				continue
			}
			if i+1 < len(text) && strings.ContainsRune("?^$", rune(text[i+1])) {
				add(i, i+2, KindVariable, class)
				i += 2
				continue
			}
			s.back, s.mode, s.braced = s.mode, psVariable, false
			n = 1
			if i+1 < len(text) && text[i+1] == '{' {
				s.braced = true
				n++
			}
			add(i, i+n, KindVariable, class)
			i += n
			continue
		}
		if c == ' ' || c == '\t' || c == '\r' {
			i++
			continue
		}
		if c == '#' || in.Offset+i == 0 && strings.HasPrefix(text[i:], "#!") {
			s.mode = psLineComment
			continue
		}
		if strings.HasPrefix(text[i:], "<#") {
			folds.openComment()
			s.mode = psBlockComment
			add(i, i+2, KindComment, ClassComment)
			i += 2
			continue
		}
		if c == '@' && i+1 < len(text) {
			next, size := utf8.DecodeRuneInString(text[i+1:])
			if q := psQuote(next); q != 0 {
				folds.openLiteral()
				s.mode, s.quote = psHereHeader, q
				add(i, i+1+size, KindString, ClassString)
				i += 1 + size
				continue
			}
		}
		if q := psQuote(r); q != 0 {
			folds.openLiteral()
			s.mode, s.quote = psQuoted, q
			add(i, i+n, KindString, ClassString)
			i += n
			continue
		}
		if c == '`' {
			n = 1
			if i+1 < len(text) {
				n += runeSize(text, i+1)
			}
			add(i, i+n, KindEscape, 0)
			i += n
			continue
		}
		if strings.HasPrefix(text[i:], "..") {
			add(i, i+2, KindDelimiter, 0)
			i += 2
			continue
		}
		if digit(c) || c == '.' && i+1 < len(text) && digit(text[i+1]) {
			s.mode, s.last = psNumber, c
			add(i, i+1, KindNumber, 0)
			i++
			continue
		}
		if codeNameStart(PowerShell, r) || c == '-' && i+1 < len(text) && codeNameStart(PowerShell, rune(text[i+1])) {
			kind := KindPlain
			if c == '-' {
				kind = KindAttribute
			}
			s.word, s.wordOn = scanWord{start: in.Offset + i, kind: kind}, true
			s.word.add(text[i : i+n])
			i += n
			continue
		}
		if f := s.frames; f != nil && (c == '(' || c == ')') {
			next := *f
			if c == '(' {
				next.depth++
			} else {
				next.depth--
			}
			if next.depth == 0 {
				s.mode, s.quote, s.frames = f.mode, f.quote, f.parent
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
	if in.LineEnd && i == len(text) {
		finishWord(in.Offset + i)
		if s.mode == psHereHeader {
			s.mode = psHereBody
		}
		if s.mode == psVariable && !s.braced {
			s.mode, s.back = s.back, psCode
		}
		if s.mode == psLineComment || s.mode == psNumber {
			s.mode = psCode
		}
		s.escape, s.last = false, 0
	}
	return w.result(i, s), nil
}
