package language

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

type cMode uint8

const (
	cNormal cMode = iota
	cQuoted
	cLineComment
	cBlockComment
	cNumber
	cRawOpen
	cRawBody
)

// C and C++ share preprocessing and ordinary literals. Raw string delimiters
// are bounded by the C++ grammar, rather than by an arbitrary scanner limit.
type cState struct {
	id                                         ID
	mode                                       cMode
	quote                                      byte
	escape, star                               bool
	leading, directive, directiveName, include bool
	last                                       byte
	word                                       scanWord
	wordOn                                     bool
	raw                                        string
}

func (s cState) Equal(other State) bool { t, ok := other.(cState); return ok && s == t }
func (s cState) Context() Context {
	return Context{String: s.mode == cQuoted || s.mode == cRawOpen || s.mode == cRawBody, Comment: s.mode == cLineComment || s.mode == cBlockComment, Literal: s.mode == cRawOpen || s.mode == cRawBody}
}

type cTokenizer struct{ id ID }

func (b cTokenizer) InitialState() State { return cState{id: b.id, leading: true} }

var cKeywords = words("alignas alignof auto bool break case char const constexpr continue default do double else enum extern false float for goto if inline int long nullptr register restrict return short signed sizeof static static_assert struct switch thread_local true typedef typeof typeof_unqual union unsigned void volatile while _Alignas _Alignof _Atomic _BitInt _Bool _Complex _Decimal32 _Decimal64 _Decimal128 _Generic _Imaginary _Noreturn _Static_assert _Thread_local")
var cppKeywords = words("alignas alignof and and_eq asm auto bitand bitor bool break case catch char char8_t char16_t char32_t class compl concept const consteval constexpr constinit const_cast continue co_await co_return co_yield decltype default delete do double dynamic_cast else enum explicit export extern false float for friend goto if inline int long mutable namespace new noexcept not not_eq nullptr operator or or_eq private protected public register reinterpret_cast requires return short signed sizeof static static_assert static_cast struct switch template this thread_local throw true try typedef typeid typename union unsigned using virtual void volatile wchar_t while xor xor_eq")

var objcKeywords = words("id Class SEL BOOL instancetype YES NO nil Nil self super __kindof __weak __strong __autoreleasing __unsafe_unretained __block _Nonnull _Nullable _Null_unspecified __covariant __contravariant")
var objcDirectives = words("interface implementation end class protocol property synthesize dynamic selector encode defs optional required public protected private package try catch finally throw synchronized autoreleasepool import available compatibility_alias YES NO true false")

func cBaseID(id ID) ID {
	if id == ObjectiveC {
		return C
	}
	return id
}
func cObjective(id ID) bool { return id == ObjectiveC }

func cPrefix(id ID, word string, quote byte) (raw, ok bool) {
	if cObjective(id) && word == "@" && quote == '"' {
		return false, true
	}
	if word == "L" || word == "u" || word == "U" || word == "u8" {
		return false, true
	}
	if cBaseID(id) == Cpp && quote == '"' {
		switch word {
		case "R", "LR", "uR", "UR", "u8R":
			return true, true
		}
	}
	return false, false
}

func (b cTokenizer) Scan(in Input, state State) (Result, error) {
	s, ok := state.(cState)
	if !ok || s.id != b.id {
		return Result{}, fmt.Errorf("language: foreign state for %s", b.id)
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
		if s.directiveName {
			kind = KindMeta
			s.include = s.word.text == "include" || s.word.text == "include_next" || s.word.text == "import"
			s.directiveName = false
		} else if cObjective(b.id) && (objcKeywords[s.word.text] || strings.HasPrefix(s.word.text, "@") && objcDirectives[strings.TrimPrefix(s.word.text, "@")]) {
			kind = KindKeyword
		} else if cBaseID(b.id) == C && cKeywords[s.word.text] || cBaseID(b.id) == Cpp && cppKeywords[s.word.text] {
			kind = KindKeyword
		}
		w.add(s.word.start, end, kind, 0)
		s.word, s.wordOn = scanWord{}, false
	}
	for i < limit {
		c := text[i]
		r, n := utf8.DecodeRuneInString(text[i:])
		switch s.mode {
		case cLineComment:
			add(i, i+n, KindComment, ClassComment)
			s.last = c
			i += n
			continue
		case cBlockComment:
			add(i, i+n, KindComment, ClassComment)
			if s.star && c == '/' {
				s.mode = cNormal
				s.star = false
			} else {
				s.star = c == '*'
			}
			s.last = c
			i += n
			continue
		case cQuoted:
			kind := KindString
			if s.escape {
				kind = KindEscape
				s.escape = false
			} else if c == '\\' && s.quote != '>' {
				kind = KindEscape
				s.escape = true
			} else if c == s.quote {
				s.mode = cNormal
				s.quote = 0
			}
			add(i, i+n, kind, ClassString)
			s.last = c
			i += n
			continue
		case cRawOpen:
			if c == '(' {
				s.mode = cRawBody
				add(i, i+1, KindString, ClassString)
				i++
				continue
			}
			if len(s.raw) < 16 && c > ' ' && c < 127 && !strings.ContainsRune("()\\", r) {
				s.raw += string(c)
				add(i, i+1, KindString, ClassString)
				i++
				continue
			}
			// Invalid openers recover on this physical line.
			s.mode, s.raw = cNormal, ""
			continue
		case cRawBody:
			close := ")" + s.raw + `"`
			if c == ')' && strings.HasPrefix(text[i:], close) {
				add(i, i+len(close), KindString, ClassString)
				i += len(close)
				s.mode, s.raw = cNormal, ""
				continue
			}
			add(i, i+n, KindString, ClassString)
			i += n
			continue
		case cNumber:
			if IdentifierRune(r) || c == '.' || c == '\'' || (c == '+' || c == '-') && strings.ContainsRune("eEpP", rune(s.last)) {
				add(i, i+n, KindNumber, 0)
				s.last = c
				i += n
				continue
			}
			s.mode = cNormal
		}
		if s.wordOn {
			if IdentifierRune(r) {
				s.word.add(text[i : i+n])
				i += n
				s.last = c
				continue
			}
			if c == '"' || c == '\'' {
				if raw, ok := cPrefix(b.id, s.word.text, c); ok {
					w.add(s.word.start, in.Offset+i+1, KindString, ClassString)
					s.word, s.wordOn = scanWord{}, false
					s.mode, s.quote, s.escape = cQuoted, c, false
					if raw {
						s.mode, s.raw = cRawOpen, ""
					}
					i++
					s.last = c
					continue
				}
			}
			finishWord(in.Offset + i)
		}
		if c == ' ' || c == '\t' || c == '\r' {
			s.last = c
			i++
			continue
		}
		if c == '/' && i+1 < len(text) && (text[i+1] == '/' || text[i+1] == '*') {
			s.mode = cLineComment
			if text[i+1] == '*' {
				s.mode = cBlockComment
			}
			s.star = false
			add(i, i+2, KindComment, ClassComment)
			s.last = text[i+1]
			i += 2
			continue
		}
		if c == '#' && s.leading {
			s.directive, s.directiveName, s.leading = true, true, false
			add(i, i+1, KindMeta, 0)
			s.last = c
			i++
			continue
		}
		s.leading = false
		if cObjective(b.id) && c == '@' && i+1 < len(text) && (digit(text[i+1]) || (text[i+1] == '+' || text[i+1] == '-') && i+2 < len(text) && digit(text[i+2])) {
			n = 1
			if text[i+1] == '+' || text[i+1] == '-' {
				n = 2
			}
			add(i, i+n, KindNumber, 0)
			s.mode, s.last = cNumber, text[i+n-1]
			i += n
			continue
		}
		if c == '"' || c == '\'' || c == '<' && s.include {
			s.mode, s.quote, s.escape = cQuoted, c, false
			if c == '<' {
				s.quote = '>'
			}
			s.include = false
			add(i, i+1, KindString, ClassString)
			s.last = c
			i++
			continue
		}
		if digit(c) || c == '.' && i+1 < len(text) && digit(text[i+1]) {
			s.mode = cNumber
			s.last = c
			add(i, i+1, KindNumber, 0)
			i++
			continue
		}
		if IdentifierRune(r) || cObjective(b.id) && c == '@' {
			s.word, s.wordOn = scanWord{start: in.Offset + i}, true
			if c == '@' {
				s.word.kind = KindMeta
			}
			s.word.add(text[i : i+n])
			s.last = c
			i += n
			continue
		}
		class := Class(0)
		if strings.ContainsRune("()[]{}", r) {
			class = ClassDelimiter
		}
		add(i, i+n, KindDelimiter, class)
		s.last = c
		i += n
	}
	if in.LineEnd && i == len(text) {
		splice := s.last == '\\' && s.mode != cRawBody
		if !splice {
			finishWord(in.Offset + i)
			s.leading, s.directive, s.directiveName, s.include = true, false, false, false
			if s.mode == cQuoted || s.mode == cLineComment || s.mode == cNumber || s.mode == cRawOpen {
				s.mode, s.quote, s.raw = cNormal, 0, ""
			}
			s.escape, s.star = false, false
		} else {
			// Removing the final backslash changes escape parity even after \\.
			if s.mode == cQuoted {
				s.escape = !s.escape
			}
			if s.wordOn {
				s.word.start = 0
			}
		}
		s.last = 0
	}
	return w.result(i, s), nil
}
