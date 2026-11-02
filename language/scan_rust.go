package language

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

type rustMode uint8

const (
	rustNormal rustMode = iota
	rustQuoted
	rustLineComment
	rustBlockComment
	rustNumber
	rustRawOpen
	rustRawBody
)

type rustState struct {
	mode                             rustMode
	quote                            byte
	escape                           bool
	rawIdentOpen                     bool
	depth, hashes, matched, rawStart int
	word                             scanWord
	wordOn, rawIdentifier, lifetime  bool
	last                             byte
}

func (s rustState) Equal(other State) bool { t, ok := other.(rustState); return ok && s == t }
func (s rustState) Context() Context {
	return Context{String: s.mode == rustQuoted || s.mode == rustRawBody, Comment: s.mode == rustLineComment || s.mode == rustBlockComment, Literal: s.mode == rustRawBody}
}

type rustTokenizer struct{}

func (rustTokenizer) InitialState() State { return rustState{} }

var rustKeywords = words("as async await break const continue crate dyn else enum extern false fn for if impl in let loop match mod move mut pub ref return self Self static struct super trait true type unsafe use where while abstract become box do final gen macro override priv try typeof unsized virtual yield bool char str i8 i16 i32 i64 i128 isize u8 u16 u32 u64 u128 usize f32 f64")

func (rustTokenizer) Scan(in Input, state State) (Result, error) {
	s, ok := state.(rustState)
	if !ok {
		return Result{}, fmt.Errorf("language: foreign Rust state")
	}
	limit, err := scanLimit(in)
	if err != nil {
		return Result{}, err
	}
	w := tokenWriter{base: in.Offset}
	text, i := in.Text, 0
	add := func(a, z int, k Kind, c Class) { w.add(in.Offset+a, in.Offset+z, k, c) }
	finishWord := func(end int, next byte) {
		if !s.wordOn {
			return
		}
		kind := KindPlain
		if s.lifetime {
			kind = KindVariable
		} else if !s.rawIdentifier && rustKeywords[s.word.text] {
			kind = KindKeyword
		} else if next == '!' {
			kind = KindMeta
		}
		w.add(s.word.start, end, kind, 0)
		s.word, s.wordOn, s.rawIdentifier, s.lifetime = scanWord{}, false, false, false
	}
	for i < limit {
		c := text[i]
		r, n := utf8.DecodeRuneInString(text[i:])
		switch s.mode {
		case rustLineComment:
			add(i, i+n, KindComment, ClassComment)
			i += n
			continue
		case rustBlockComment:
			if strings.HasPrefix(text[i:], "/*") {
				s.depth++
				add(i, i+2, KindComment, ClassComment)
				i += 2
				continue
			}
			if strings.HasPrefix(text[i:], "*/") {
				s.depth--
				add(i, i+2, KindComment, ClassComment)
				i += 2
				if s.depth == 0 {
					s.mode = rustNormal
				}
				continue
			}
			add(i, i+n, KindComment, ClassComment)
			i += n
			continue
		case rustQuoted:
			kind := KindString
			if s.escape {
				kind, s.escape = KindEscape, false
			} else if c == '\\' {
				kind, s.escape = KindEscape, true
			} else if c == s.quote {
				s.mode, s.quote = rustNormal, 0
			}
			add(i, i+n, kind, ClassString)
			i += n
			continue
		case rustRawOpen:
			if c == '#' && s.hashes < 255 {
				s.hashes++
				add(i, i+1, KindString, ClassString)
				i++
				continue
			}
			if c == '"' {
				s.mode = rustRawBody
				s.rawStart = 0
				s.rawIdentOpen = false
				add(i, i+1, KindString, ClassString)
				i++
				continue
			}
			if s.rawIdentOpen && s.hashes == 1 && (IdentifierRune(r) && !digit(c)) {
				// r#ident is an identifier, including when ident is a keyword.
				w.add(s.rawStart, in.Offset+i, KindPlain, 0)
				s.word, s.wordOn, s.rawIdentifier = scanWord{start: s.rawStart}, true, true
				s.mode, s.hashes, s.rawStart = rustNormal, 0, 0
				s.rawIdentOpen = false
				continue
			}
			w.add(s.rawStart, in.Offset+i, KindInvalid, 0)
			s.mode, s.hashes, s.rawStart = rustNormal, 0, 0
			s.rawIdentOpen = false
			continue
		case rustRawBody:
			add(i, i+n, KindString, ClassString)
			if s.matched > 0 && c == '#' {
				s.matched++
				if s.matched == s.hashes+1 {
					s.mode, s.hashes, s.matched = rustNormal, 0, 0
				}
			} else if c == '"' {
				if s.hashes == 0 {
					s.mode = rustNormal
				} else {
					s.matched = 1
				}
			} else {
				s.matched = 0
			}
			i += n
			continue
		case rustNumber:
			part := IdentifierRune(r) || (c == '+' || c == '-') && strings.ContainsRune("eE", rune(s.last))
			if c == '.' {
				part = i+1 >= len(text) || text[i+1] != '.' && !codeNameStart(Rust, rune(text[i+1]))
			}
			if part {
				add(i, i+n, KindNumber, 0)
				s.last = c
				i += n
				continue
			}
			s.mode, s.last = rustNormal, 0
		}
		if s.wordOn {
			if IdentifierRune(r) {
				s.word.add(text[i : i+n])
				i += n
				continue
			}
			prefix := s.word.text
			if !s.rawIdentifier && !s.lifetime && (c == '"' || c == '\'' && prefix == "b") && (prefix == "b" || prefix == "c" || prefix == "r" || prefix == "br" || prefix == "cr") {
				w.add(s.word.start, in.Offset+i+1, KindString, ClassString)
				s.mode, s.quote = rustQuoted, c
				if strings.HasSuffix(prefix, "r") {
					s.mode, s.hashes = rustRawBody, 0
				}
				s.word, s.wordOn = scanWord{}, false
				i++
				continue
			}
			if c == '#' && (prefix == "r" || prefix == "br" || prefix == "cr") && !s.rawIdentifier && !s.lifetime {
				s.mode, s.rawStart, s.hashes = rustRawOpen, s.word.start, 1
				s.rawIdentOpen = prefix == "r"
				w.add(s.word.start, in.Offset+i+1, KindString, ClassString)
				s.word, s.wordOn = scanWord{}, false
				i++
				continue
			}
			finishWord(in.Offset+i, c)
		}
		if c == ' ' || c == '\t' || c == '\r' {
			i++
			continue
		}
		if c == '/' && i+1 < len(text) && (text[i+1] == '/' || text[i+1] == '*') {
			s.mode = rustLineComment
			if text[i+1] == '*' {
				s.mode, s.depth = rustBlockComment, 1
			}
			add(i, i+2, KindComment, ClassComment)
			i += 2
			continue
		}
		if c == '\'' && i+1 < len(text) {
			next, size := utf8.DecodeRuneInString(text[i+1:])
			if codeNameStart(Rust, next) && !(i+1+size < len(text) && text[i+1+size] == '\'') {
				s.word, s.wordOn, s.lifetime = scanWord{start: in.Offset + i}, true, true
				i++
				continue
			}
		}
		if c == '"' || c == '\'' {
			s.mode, s.quote, s.escape = rustQuoted, c, false
			add(i, i+1, KindString, ClassString)
			i++
			continue
		}
		if digit(c) {
			s.mode, s.last = rustNumber, c
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
		kind, class := KindDelimiter, Class(0)
		if c == '#' {
			kind = KindMeta
		}
		if strings.ContainsRune("()[]{}", r) {
			class = ClassDelimiter
		}
		add(i, i+n, kind, class)
		i += n
	}
	if in.LineEnd && i == len(text) {
		finishWord(in.Offset+i, 0)
		if s.mode == rustRawOpen {
			w.add(s.rawStart, in.Offset+i, KindInvalid, 0)
			s.mode, s.hashes, s.rawStart = rustNormal, 0, 0
			s.rawIdentOpen = false
		}
		if s.mode == rustLineComment || s.mode == rustNumber || s.mode == rustQuoted && s.quote == '\'' {
			s.mode, s.quote = rustNormal, 0
		}
		s.escape, s.matched, s.last = false, 0, 0
	}
	return w.result(i, s), nil
}
