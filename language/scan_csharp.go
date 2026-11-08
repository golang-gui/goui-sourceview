package language

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

type csMode uint8

const (
	csNormal csMode = iota
	csPrefix
	csQuoteOpen
	csQuoted
	csRaw
	csFormat
	csLineComment
	csBlockComment
	csDirective
	csNumber
)

type csLiteral struct {
	quote                             byte
	verbatim                          bool
	dollars, quotes                   int
	multiline, openingLine, lineSpace bool
}
type csFrame struct {
	parent                    *csFrame
	literal                   csLiteral
	braces, parens, ternaries int
}
type csState struct {
	mode                                     csMode
	literal                                  csLiteral
	frames                                   *csFrame
	escape, leading                          bool
	quoteRun, braceRun, closing, prefixStart int
	word                                     scanWord
	wordOn, escapedWord                      bool
	last                                     byte
}

func (s csState) Equal(other State) bool {
	t, ok := other.(csState)
	if !ok {
		return false
	}
	for a, b := s.frames, t.frames; a != b; a, b = a.parent, b.parent {
		if a == nil || b == nil || a.literal != b.literal || a.braces != b.braces || a.parens != b.parens || a.ternaries != b.ternaries {
			return false
		}
	}
	s.frames, t.frames = nil, nil
	return s == t
}
func (s csState) Context() Context {
	if s.mode == csRaw && s.literal.dollars > 0 && s.braceRun >= s.literal.dollars {
		return Context{}
	}
	return Context{String: s.mode == csQuoteOpen && s.quoteRun != 2 || s.mode == csQuoted || s.mode == csRaw || s.mode == csFormat, Comment: s.mode == csLineComment || s.mode == csBlockComment, Literal: s.mode == csRaw || s.mode == csQuoteOpen && s.quoteRun >= 3 || s.mode == csQuoted && s.literal.verbatim}
}

type csTokenizer struct{}

func (csTokenizer) InitialState() State { return csState{leading: true} }

var csKeywords = words("abstract as base bool break byte case catch char checked class const continue decimal default delegate do double else enum event explicit extern false finally fixed float for foreach goto if implicit in int interface internal is lock long namespace new null object operator out override params private protected public readonly ref return sbyte sealed short sizeof stackalloc static string struct switch this throw true try typeof uint ulong unchecked unsafe ushort using virtual void volatile while add alias and args ascending async await by descending dynamic equals field file from get global group init into join let managed nameof nint not notnull nuint on or orderby partial record remove required scoped select set unmanaged value var when where with yield")

func (t csTokenizer) Scan(in Input, state State) (Result, error) {
	return t.scan(in, state, nil)
}

func (csTokenizer) scan(in Input, state State, folds *foldLexemes) (Result, error) {
	s, ok := state.(csState)
	if !ok {
		return Result{}, fmt.Errorf("language: foreign C# state")
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
		if !s.escapedWord && csKeywords[s.word.text] {
			kind = KindKeyword
		}
		w.add(s.word.start, end, kind, 0)
		s.word, s.wordOn, s.escapedWord = scanWord{}, false, false
	}
	endLiteral := func(end int, complete bool) {
		if s.literal.quote != 0 {
			folds.closeLiteral(end, complete)
		}
		s.mode, s.literal, s.escape, s.quoteRun = csNormal, csLiteral{}, false, 0
	}
	enterExpression := func() {
		s.frames = &csFrame{parent: s.frames, literal: s.literal}
		s.mode, s.literal, s.braceRun = csNormal, csLiteral{}, 0
	}
	leaveExpression := func() {
		s.literal, s.frames, s.closing = s.frames.literal, s.frames.parent, 0
		s.mode = csQuoted
		if s.literal.quotes >= 3 {
			s.mode = csRaw
		}
	}
	for i < limit {
		c := text[i]
		r, n := utf8.DecodeRuneInString(text[i:])
		switch s.mode {
		case csDirective:
			add(i, i+n, KindMeta, 0)
			i += n
			continue
		case csLineComment:
			add(i, i+n, KindComment, ClassComment)
			i += n
			continue
		case csBlockComment:
			if strings.HasPrefix(text[i:], "*/") {
				folds.closeComment(in.Offset + i + 2)
				add(i, i+2, KindComment, ClassComment)
				i += 2
				s.mode = csNormal
				continue
			}
			add(i, i+n, KindComment, ClassComment)
			i += n
			continue
		case csPrefix:
			if c == '$' && !s.literal.verbatim {
				s.literal.dollars++
				add(i, i+1, KindDelimiter, 0)
				i++
				continue
			}
			if c == '@' && !s.literal.verbatim && s.literal.dollars == 1 {
				s.literal.verbatim = true
				add(i, i+1, KindDelimiter, 0)
				i++
				continue
			}
			if c == '$' && s.literal.verbatim && s.literal.dollars == 0 {
				s.literal.dollars = 1
				add(i, i+1, KindDelimiter, 0)
				i++
				continue
			}
			if c == '"' {
				w.add(s.prefixStart, in.Offset+i+1, KindString, ClassString)
				folds.openLiteral()
				s.literal.quote, s.quoteRun, s.mode, s.prefixStart = c, 1, csQuoteOpen, 0
				if s.literal.verbatim {
					s.mode, s.quoteRun = csQuoted, 0
				}
				i++
				continue
			}
			if s.literal.verbatim && s.literal.dollars == 0 && codeNameStart(CSharp, r) {
				s.word, s.wordOn, s.escapedWord = scanWord{start: s.prefixStart}, true, true
				s.mode, s.literal, s.prefixStart = csNormal, csLiteral{}, 0
				continue
			}
			s.mode, s.literal, s.prefixStart = csNormal, csLiteral{}, 0
			continue
		case csQuoteOpen:
			if c == '"' {
				s.quoteRun++
				add(i, i+1, KindString, ClassString)
				i++
				continue
			}
			switch s.quoteRun {
			case 1:
				s.mode = csQuoted
			case 2:
				endLiteral(in.Offset+i, true)
			default:
				s.mode, s.literal.quotes, s.literal.openingLine, s.literal.lineSpace = csRaw, s.quoteRun, true, true
			}
			s.quoteRun = 0
			continue
		case csRaw:
			if s.braceRun > 0 && c != '{' {
				if s.braceRun >= s.literal.dollars {
					w.add(in.Offset+i-s.literal.dollars, in.Offset+i, KindDelimiter, ClassDelimiter)
					enterExpression()
					continue
				}
				s.braceRun = 0
			}
			if c == '{' && s.literal.dollars > 0 {
				s.braceRun++
				add(i, i+1, KindString, ClassString)
				s.literal.openingLine, s.literal.lineSpace = false, false
				i++
				continue
			}
			if c == '"' && (!s.literal.multiline || s.literal.lineSpace || s.quoteRun > 0) {
				s.quoteRun++
				add(i, i+1, KindString, ClassString)
				i++
				if s.quoteRun == s.literal.quotes {
					endLiteral(in.Offset+i, true)
				}
				continue
			}
			s.quoteRun = 0
			if c != ' ' && c != '\t' && c != '\r' {
				s.literal.openingLine, s.literal.lineSpace = false, false
			}
			add(i, i+n, KindString, ClassString)
			i += n
			continue
		case csQuoted:
			if s.literal.dollars > 0 && c == '{' && !s.escape {
				if strings.HasPrefix(text[i:], "{{") {
					add(i, i+2, KindString, ClassString)
					i += 2
					continue
				}
				add(i, i+1, KindDelimiter, ClassDelimiter)
				i++
				enterExpression()
				continue
			}
			if s.literal.verbatim && c == '"' && strings.HasPrefix(text[i:], `""`) {
				add(i, i+2, KindEscape, ClassString)
				i += 2
				continue
			}
			kind := KindString
			if s.escape {
				kind, s.escape = KindEscape, false
			} else if c == '\\' && !s.literal.verbatim {
				kind, s.escape = KindEscape, true
			} else if c == s.literal.quote {
				endLiteral(in.Offset+i+n, true)
			}
			add(i, i+n, kind, ClassString)
			i += n
			continue
		case csFormat:
			if c == '}' {
				s.closing++
				add(i, i+1, KindDelimiter, ClassDelimiter)
				i++
				if s.closing == s.frames.literal.dollars {
					leaveExpression()
				}
				continue
			}
			s.closing = 0
			add(i, i+n, KindString, ClassString)
			i += n
			continue
		case csNumber:
			if IdentifierRune(r) || c == '.' && !strings.HasPrefix(text[i:], "..") || (c == '+' || c == '-') && strings.ContainsRune("eE", rune(s.last)) {
				add(i, i+n, KindNumber, 0)
				s.last = c
				i += n
				continue
			}
			s.mode, s.last = csNormal, 0
		}
		if s.wordOn {
			if IdentifierRune(r) {
				s.word.add(text[i : i+n])
				i += n
				continue
			}
			finishWord(in.Offset + i)
		}
		if c == ' ' || c == '\t' || c == '\r' {
			i++
			continue
		}
		if c == '#' && s.leading && s.frames == nil {
			s.mode = csDirective
			add(i, i+1, KindMeta, 0)
			i++
			continue
		}
		if c == '/' && i+1 < len(text) && (text[i+1] == '/' || text[i+1] == '*') {
			s.mode = csLineComment
			if text[i+1] == '*' {
				s.mode = csBlockComment
				folds.openComment()
			}
			add(i, i+2, KindComment, ClassComment)
			i += 2
			continue
		}
		s.leading = false
		if strings.HasPrefix(text[i:], "..") {
			add(i, i+2, KindDelimiter, 0)
			i += 2
			continue
		}
		if f := s.frames; f != nil {
			if strings.HasPrefix(text[i:], "??") || strings.HasPrefix(text[i:], "?.") || strings.HasPrefix(text[i:], "::") {
				add(i, i+2, KindDelimiter, 0)
				i += 2
				continue
			}
			next := *f
			if c == '}' && f.braces == 0 {
				s.closing++
				add(i, i+1, KindDelimiter, ClassDelimiter)
				i++
				if s.closing == f.literal.dollars {
					leaveExpression()
				}
				continue
			}
			s.closing = 0
			switch c {
			case '{':
				next.braces++
			case '}':
				next.braces = max(0, next.braces-1)
			case '(', '[':
				next.parens++
			case ')', ']':
				next.parens = max(0, next.parens-1)
			case '?':
				if f.braces == 0 && f.parens == 0 && !strings.HasPrefix(text[i:], "??") && !strings.HasPrefix(text[i:], "?.") && !strings.HasPrefix(text[i:], "?[") {
					next.ternaries++
				}
			case ':':
				if f.braces == 0 && f.parens == 0 {
					if f.ternaries > 0 {
						next.ternaries--
					} else if !strings.HasPrefix(text[i:], "::") && (i == 0 || text[i-1] != ':') {
						s.mode = csFormat
					}
				}
			}
			s.frames = &next
		}
		if c == '$' || c == '@' {
			s.mode, s.prefixStart = csPrefix, in.Offset+i
			s.literal = csLiteral{verbatim: c == '@'}
			if c == '$' {
				s.literal.dollars = 1
			}
			add(i, i+1, KindDelimiter, 0)
			i++
			continue
		}
		if c == '"' {
			folds.openLiteral()
			s.mode, s.literal, s.quoteRun = csQuoteOpen, csLiteral{quote: c}, 1
			add(i, i+1, KindString, ClassString)
			i++
			continue
		}
		if c == '\'' {
			folds.openLiteral()
			s.mode, s.literal = csQuoted, csLiteral{quote: c}
			add(i, i+1, KindString, ClassString)
			i++
			continue
		}
		if digit(c) || c == '.' && i+1 < len(text) && digit(text[i+1]) {
			s.mode, s.last = csNumber, c
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
	}
	if in.LineEnd && i == len(text) {
		finishWord(in.Offset + i)
		if s.mode == csQuoteOpen {
			if s.quoteRun >= 3 {
				s.mode, s.literal.quotes, s.literal.openingLine = csRaw, s.quoteRun, true
			} else {
				endLiteral(in.Offset+i, s.quoteRun == 2)
			}
		}
		if s.mode == csRaw {
			if s.braceRun >= s.literal.dollars && s.literal.dollars > 0 {
				w.add(in.Offset+i-s.literal.dollars, in.Offset+i, KindDelimiter, ClassDelimiter)
				enterExpression()
			} else {
				s.braceRun = 0
			}
		}
		if s.mode == csRaw {
			s.literal.multiline = s.literal.multiline || s.literal.openingLine
			s.literal.openingLine, s.literal.lineSpace = false, true
		}
		if s.mode == csQuoted && !s.literal.verbatim || s.mode == csLineComment || s.mode == csDirective || s.mode == csNumber || s.mode == csPrefix {
			endLiteral(in.Offset+i, false)
		}
		s.escape, s.leading, s.quoteRun, s.last, s.prefixStart, s.closing = false, true, 0, 0, 0, 0
	}
	return w.result(i, s), nil
}
