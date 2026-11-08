package language

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

type jsxMode uint8

const (
	jsxCode jsxMode = iota
	jsxName
	jsxAttrs
	jsxAttrName
	jsxAfterAttr
	jsxValue
	jsxQuoted
	jsxText
	jsxEntity
)

type jsxFrame struct {
	parent *jsxFrame
	mode   jsxMode
	code   codeState
	depth  int
}
type jsxState struct {
	id                 ID
	mode, back         jsxMode
	code               codeState
	frames             *jsxFrame
	depth              int
	closing, candidate bool
	quote              byte
	name, attr         scanWord
}

func (s jsxState) Equal(other State) bool {
	t, ok := other.(jsxState)
	if !ok || !s.code.Equal(t.code) {
		return false
	}
	for a, b := s.frames, t.frames; a != b; a, b = a.parent, b.parent {
		if a == nil || b == nil || a.mode != b.mode || a.depth != b.depth || !a.code.Equal(b.code) {
			return false
		}
	}
	s.code, t.code = codeState{}, codeState{}
	s.frames, t.frames = nil, nil
	return s == t
}
func (s jsxState) Context() Context {
	if s.mode == jsxCode {
		return s.code.Context()
	}
	m := s.mode
	if m == jsxEntity {
		m = s.back
	}
	str := m == jsxQuoted || m == jsxText
	return Context{String: str, Literal: str, Tag: m != jsxText, TagName: m == jsxName, Value: m == jsxValue || m == jsxQuoted}
}

type jsxTokenizer struct{ id ID }

func (b jsxTokenizer) base() ID {
	if b.id == TSX {
		return TypeScript
	}
	return JavaScript
}
func (b jsxTokenizer) InitialState() State {
	return jsxState{id: b.id, code: codeState{id: b.base(), canRegexp: true}}
}
func jsxNamePart(r rune) bool { return IdentifierRune(r) || strings.ContainsRune("$.-:", r) }
func jsxStart(text string, closing bool) bool {
	if !strings.HasPrefix(text, "<") || len(text) < 2 {
		return false
	}
	i := 1
	if text[i] == '/' {
		if !closing {
			return false
		}
		i++
	}
	if i == len(text) {
		return false
	}
	r, _ := utf8.DecodeRuneInString(text[i:])
	return r == '>' || IdentifierRune(r) && !(r >= '0' && r <= '9') || r == '$'
}

func (b jsxTokenizer) Scan(in Input, state State) (Result, error) {
	return b.scan(in, state, nil)
}

func (b jsxTokenizer) scan(in Input, state State, folds *foldLexemes) (Result, error) {
	s, ok := state.(jsxState)
	if !ok || s.id != b.id {
		return Result{}, fmt.Errorf("language: foreign JSX state")
	}
	limit, err := scanLimit(in)
	if err != nil {
		return Result{}, err
	}
	w := tokenWriter{base: in.Offset}
	text, i := in.Text, 0
	add := func(a, z int, k Kind, c Class) { w.add(in.Offset+a, in.Offset+z, k, c) }
	confirm := func() { s.candidate = false; s.name, s.attr = scanWord{}, scanWord{} }
	push := func(mode jsxMode) { s.frames = &jsxFrame{parent: s.frames, mode: mode, code: s.code, depth: s.depth} }
	pop := func() { f := s.frames; s.mode, s.code, s.depth, s.frames = f.mode, f.code, f.depth, f.parent }
	enterTag := func(root bool) {
		if root {
			push(jsxCode)
			s.depth = 0
		}
		s.mode, s.closing, s.candidate = jsxName, false, root && b.id == TSX
		n := 1
		if i+1 < len(text) && text[i+1] == '/' {
			s.closing = true
			n = 2
		}
		folds.startTag(s.closing)
		add(i, i+n, KindDelimiter, 0)
		i += n
		s.name, s.attr = scanWord{start: in.Offset + i}, scanWord{}
	}
	enterExpr := func() {
		confirm()
		push(jsxAttrs)
		if s.mode == jsxText {
			f := *s.frames
			f.mode = jsxText
			s.frames = &f
		}
		s.mode, s.depth, s.code = jsxCode, 0, codeState{id: b.base(), canRegexp: true}
		add(i, i+1, KindDelimiter, ClassDelimiter)
		i++
	}
	finishTag := func(self bool) {
		folds.endTag(in.Offset+i, self)
		if s.closing {
			s.depth = max(0, s.depth-1)
		} else if !self {
			s.depth++
		}
		confirm()
		s.closing, s.quote = false, 0
		if s.depth == 0 {
			pop()
			s.code.canRegexp, s.code.lastDot = false, false
		} else {
			s.mode = jsxText
		}
	}
	fallback := func(extends bool) {
		folds.abortTag()
		name, attr := s.name, s.attr
		pop()
		s.code.word, s.code.wordOn, s.code.canRegexp = name, true, true
		finishCodeWord(&s.code, &w, name.start+name.length)
		if extends {
			s.code.word, s.code.wordOn = attr, true
			finishCodeWord(&s.code, &w, attr.start+attr.length)
		}
		s.mode = jsxCode
		confirm()
		s.closing = false
	}
	for i < limit {
		c := text[i]
		r, n := utf8.DecodeRuneInString(text[i:])
		if s.mode == jsxCode {
			boundary := byte(0)
			part := Input{Text: text[i:], Offset: in.Offset + i, Limit: limit - i, LineEnd: in.LineEnd, DocumentEnd: in.DocumentEnd}
			result, err := (codeTokenizer{b.base()}).scanFolding(part, s.code, func(code codeState, at int) bool {
				if part.Text[at] == '}' && s.frames != nil && s.frames.mode != jsxCode && code.braces == 0 && code.strings == nil {
					boundary = '}'
					return true
				}
				if part.Text[at] == '<' && code.canRegexp && jsxStart(part.Text[at:], false) {
					boundary = '<'
					return true
				}
				return false
			}, folds)
			if err != nil {
				return Result{}, err
			}
			w.merge(result)
			s.code = result.State.(codeState)
			i += result.Consumed
			if boundary == '<' {
				enterTag(true)
			} else if boundary == '}' {
				add(i, i+1, KindDelimiter, ClassDelimiter)
				i++
				pop()
			} else if result.Consumed == 0 {
				return Result{}, fmt.Errorf("language: stalled JSX code")
			}
			continue
		}
		switch s.mode {
		case jsxName:
			if jsxNamePart(r) {
				folds.tagName(text[i : i+n])
				s.name.add(text[i : i+n])
				if strings.ContainsRune(".:-", r) {
					s.candidate = false
				}
				add(i, i+n, KindTag, 0)
				i += n
				continue
			}
			s.mode = jsxAttrs
			continue
		case jsxAttrName:
			if jsxNamePart(r) {
				s.attr.add(text[i : i+n])
				add(i, i+n, KindAttribute, 0)
				i += n
				continue
			}
			if !s.candidate || s.attr.text != "extends" {
				confirm()
			}
			s.mode = jsxAfterAttr
			continue
		case jsxEntity:
			if c == ';' {
				add(i, i+1, KindEntity, ClassString)
				i++
				s.mode, s.back = s.back, jsxCode
				continue
			}
			if IdentifierRune(r) || c == '#' {
				add(i, i+n, KindEntity, ClassString)
				i += n
				continue
			}
			s.mode, s.back = s.back, jsxCode
			continue
		case jsxQuoted:
			if c == s.quote {
				folds.closeLiteral(in.Offset+i+1, true)
				add(i, i+1, KindString, ClassString)
				i++
				s.mode, s.quote = jsxAttrs, 0
				continue
			}
			if c == '&' {
				s.back, s.mode = jsxQuoted, jsxEntity
				add(i, i+1, KindEntity, ClassString)
				i++
				continue
			}
			add(i, i+n, KindString, ClassString)
			i += n
			continue
		case jsxText:
			if c == '<' && jsxStart(text[i:], true) {
				enterTag(false)
				continue
			}
			if c == '{' {
				enterExpr()
				continue
			}
			if c == '&' {
				s.back, s.mode = jsxText, jsxEntity
				add(i, i+1, KindEntity, ClassString)
				i++
				continue
			}
			add(i, i+n, KindPlain, ClassString)
			i += n
			continue
		}
		if c == ' ' || c == '\t' || c == '\r' {
			i++
			continue
		}
		if s.candidate && (c == ',' || c == '=' && s.attr.length == 0) {
			fallback(false)
			continue
		}
		if s.candidate && s.mode == jsxAfterAttr && s.attr.text == "extends" && c != '=' && c != '>' && c != '/' {
			fallback(true)
			continue
		}
		if c == '>' {
			add(i, i+1, KindDelimiter, 0)
			i++
			finishTag(false)
			continue
		}
		if strings.HasPrefix(text[i:], "/>") {
			add(i, i+2, KindDelimiter, 0)
			i += 2
			finishTag(true)
			continue
		}
		if c == '{' {
			enterExpr()
			continue
		}
		if s.mode == jsxValue {
			confirm()
			if c == '\'' || c == '"' {
				folds.openLiteral()
				s.mode, s.quote = jsxQuoted, c
				add(i, i+1, KindString, ClassString)
				i++
				continue
			}
			s.mode = jsxAttrs
		}
		if c == '=' {
			confirm()
			s.mode = jsxValue
			add(i, i+1, KindDelimiter, 0)
			i++
			continue
		}
		if jsxNamePart(r) {
			s.mode, s.attr = jsxAttrName, scanWord{start: in.Offset + i}
			continue
		}
		confirm()
		add(i, i+n, KindDelimiter, 0)
		i += n
	}
	if in.LineEnd && i == len(text) {
		if s.mode == jsxEntity {
			s.mode, s.back = s.back, jsxCode
		}
		if s.mode == jsxName {
			s.mode = jsxAttrs
		}
		if s.mode == jsxAttrName {
			s.mode = jsxAfterAttr
		}
		confirm()
	}
	return w.result(i, s), nil
}
