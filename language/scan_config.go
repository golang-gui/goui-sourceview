package language

import (
	"fmt"
	"strings"
)

type configMode uint8

const (
	configCode configMode = iota
	configQuote
	configComment
	configSection
	configBracketOpen
	configBracket
)

// Configuration grammars share streaming words, quoting and variable spelling,
// but keep their statement and physical-line rules separate.
type configState struct {
	id                                     ID
	mode                                   configMode
	quote                                  byte
	escape, seen, value, continued, recipe bool
	directive                              bool
	depth                                  int
	word                                   scanWord
	wordOn                                 bool
	variable                               bool
	variableClose                          byte
	variableDepth                          int
	variableBack                           configMode
	bracketComment                         bool
	bracketEquals, bracketMatch            int
	recipePrefix                           byte
	prefixAssignment                       bool
	dockerEscape                           byte
	dockerShell                            *shellState
	httpStage                              uint8
	httpRequest                            bool
	httpHeader                             string
	httpBody                               ID
	body                                   builtinState
}

func (s configState) Equal(other State) bool {
	t, ok := other.(configState)
	if !ok || !equalShell(s.dockerShell, t.dockerShell) || !s.body.Equal(t.body) {
		return false
	}
	s.dockerShell, t.dockerShell = nil, nil
	s.body, t.body = builtinState{}, builtinState{}
	return s == t
}
func (s configState) Context() Context {
	if s.dockerShell != nil {
		return (builtinState{shell: s.dockerShell}).Context()
	}
	if s.id == HTTP && s.httpStage == 2 && s.httpBody != Plain {
		c := s.body.Context()
		if c.Embedded == nil {
			c.Embedded, _ = Builtin(s.httpBody)
		}
		return c
	}
	return Context{String: s.mode == configQuote || s.mode == configBracket && !s.bracketComment,
		Comment: s.mode == configComment || s.mode == configBracket && s.bracketComment,
		Literal: s.mode == configBracket, Value: s.value}
}

type configTokenizer struct{ id ID }

func (b configTokenizer) InitialState() State {
	return configState{id: b.id, directive: true, recipePrefix: '\t', dockerEscape: '\\'}
}

var makeKeywords = words("include -include sinclude define endef override export unexport private vpath ifdef ifndef ifeq ifneq else endif undefine")
var dockerKeywords = words("ADD ARG CMD COPY ENTRYPOINT ENV EXPOSE FROM HEALTHCHECK LABEL MAINTAINER ONBUILD RUN SHELL STOPSIGNAL USER VOLUME WORKDIR")
var httpMethods = words("GET HEAD POST PUT DELETE CONNECT OPTIONS TRACE PATCH PROPFIND PROPPATCH MKCOL COPY MOVE LOCK UNLOCK")

func (b configTokenizer) Scan(in Input, state State) (Result, error) {
	s, ok := state.(configState)
	if !ok || s.id != b.id {
		return Result{}, fmt.Errorf("language: foreign state for %s", b.id)
	}
	limit, err := scanLimit(in)
	if err != nil {
		return Result{}, err
	}
	w := tokenWriter{base: in.Offset}
	i := 0
	text := in.Text
	add := func(a, z int, k Kind, c Class) { w.add(in.Offset+a, in.Offset+z, k, c) }
	// Outer request separators always terminate an embedded HTTP body.
	if b.id == HTTP && in.Offset == 0 {
		trim := strings.TrimLeft(text, " \t")
		if strings.HasPrefix(trim, "###") {
			s.httpStage, s.httpBody, s.body = 0, Plain, builtinState{}
			s.mode = configComment
		} else if len(text) == 0 && in.LineEnd && s.httpStage == 1 {
			s.httpStage = 2
		}
	}
	if b.id == HTTP && s.httpStage == 2 && s.httpBody != Plain {
		raw, next, n := scanBuiltin(s.httpBody, s.body, text, limit, in.LineEnd, in.Offset)
		s.body = next
		for _, t := range raw {
			add(t.Start, t.End, t.Kind, lexicalClass(t.Kind))
		}
		return w.result(n, s), nil
	}
	finishWord := func(end int) {
		if !s.wordOn {
			return
		}
		word := s.word
		kind := word.kind
		switch b.id {
		case Makefile:
			if !s.recipe && makeKeywords[word.text] {
				kind = KindKeyword
			}
			if word.text == ".RECIPEPREFIX" {
				s.prefixAssignment = true
			}
		case Dockerfile:
			if word.kind == KindKeyword && !dockerKeywords[strings.ToUpper(word.text)] {
				kind = KindPlain
			}
		case HTTP:
			if httpMethods[word.text] {
				kind = KindKeyword
				s.httpRequest = true
			} else if strings.HasPrefix(word.text, "HTTP/") {
				kind = KindMeta
			}
			if s.httpStage == 1 && !s.value {
				s.httpHeader = strings.ToLower(word.text)
			}
			if s.httpHeader == "content-type" && s.value {
				switch strings.ToLower(word.text) {
				case "application/json":
					s.httpBody = JSON
				case "application/jsonl", "application/x-ndjson":
					s.httpBody = JSONLines
				case "application/xml", "text/xml":
					s.httpBody = XML
				case "text/html":
					s.httpBody = HTML
				}
			}
		}
		w.add(word.start, end, kind, 0)
		s.word, s.wordOn = scanWord{}, false
	}
	for i < limit {
		c := text[i]
		n := runeSize(text, i)
		start := i
		if b.id == Makefile && in.Offset+i == 0 && s.mode == configCode && c == s.recipePrefix {
			s.recipe, s.seen = true, true
			add(i, i+n, KindMeta, 0)
			i += n
			continue
		}
		if s.dockerShell != nil {
			raw, next, used := scanShell(builtinState{shell: s.dockerShell}, text[i:], limit-i, in.LineEnd)
			for _, t := range raw {
				add(i+t.Start, i+t.End, t.Kind, lexicalClass(t.Kind))
			}
			s.dockerShell = next.shell
			i += used
			break
		}
		if s.variable {
			if s.variableClose == 0 {
				if !shellName(c) && c < utf8RuneSelf {
					s.variable = false
					s.mode = s.variableBack
					continue
				}
			} else {
				open := byte('(')
				if s.variableClose == '}' {
					open = '{'
				}
				if s.variableClose == '>' {
					open = '<'
				}
				if c == open {
					s.variableDepth++
				}
				if c == s.variableClose {
					s.variableDepth--
					if s.variableDepth == 0 {
						s.variable = false
						s.mode = s.variableBack
					}
				}
			}
			add(i, i+n, KindVariable, 0)
			i += n
			continue
		}
		if s.mode == configBracketOpen {
			kind, class := KindString, ClassString
			if s.bracketComment {
				kind, class = KindComment, ClassComment
			}
			if c == '=' {
				s.bracketEquals++
				add(i, i+1, kind, class)
				i++
				continue
			}
			if c == '[' {
				s.mode = configBracket
				s.bracketMatch = 0
				add(i, i+1, kind, class)
				i++
				continue
			}
			s.mode = configCode
			if s.bracketComment {
				s.mode = configComment
			}
			s.bracketEquals = 0
			continue
		}
		if s.mode == configBracket {
			kind, class := KindString, ClassString
			if s.bracketComment {
				kind, class = KindComment, ClassComment
			}
			if s.bracketMatch == 0 {
				if c == ']' {
					s.bracketMatch = 1
				}
			} else if s.bracketMatch <= s.bracketEquals && c == '=' {
				s.bracketMatch++
			} else if s.bracketMatch == s.bracketEquals+1 && c == ']' {
				s.mode = configCode
				s.bracketMatch = 0
			} else {
				s.bracketMatch = 0
				if c == ']' {
					s.bracketMatch = 1
				}
			}
			add(i, i+n, kind, class)
			i += n
			continue
		}
		if s.mode == configComment {
			add(i, i+n, KindComment, ClassComment)
			i += n
			continue
		}
		if s.escape {
			add(i, i+n, KindEscape, func() Class {
				if s.mode == configQuote {
					return ClassString
				}
				return 0
			}())
			s.escape = false
			i += n
			continue
		}
		if s.mode == configSection {
			add(i, i+n, KindTag, 0)
			i += n
			if c == ']' {
				s.mode = configCode
				s.value = true
			}
			continue
		}
		if s.mode == configQuote && c == s.quote {
			add(i, i+1, KindString, ClassString)
			i++
			s.mode = configCode
			s.quote = 0
			continue
		}
		if c == '$' && b.id != INI || b.id == HTTP && strings.HasPrefix(text[i:], "{{") {
			finishWord(in.Offset + i)
			close := byte(0)
			size := 1
			if i+1 < len(text) {
				switch text[i+1] {
				case '{':
					close = '}'
				case '(':
					if b.id == Makefile {
						close = ')'
					}
				case '<':
					if b.id == CMake {
						close = '>'
					}
				}
				if close != 0 {
					size = 2
				}
			}
			if b.id == HTTP && c == '{' {
				close = '}'
				size = 2
			}
			if b.id == CMake && close == 0 {
				for _, p := range []string{"$ENV{", "$CACHE{"} {
					if strings.HasPrefix(text[i:], p) {
						size = len(p)
						close = '}'
						break
					}
				}
			}
			s.variableBack = s.mode
			s.variable, s.variableClose, s.variableDepth = true, close, 1
			if b.id == HTTP && c == '{' {
				s.variableDepth = 2
			}
			add(i, i+size, KindVariable, 0)
			i += size
			if close == 0 && b.id == Makefile && i < len(text) && strings.ContainsRune("@%<?^+*|$", rune(text[i])) {
				add(i, i+1, KindVariable, 0)
				i++
				s.variable = false
			}
			continue
		}
		if c == '\\' && b.id != INI && (b.id != Dockerfile || s.mode == configQuote) || b.id == Dockerfile && c == s.dockerEscape {
			finishWord(in.Offset + i)
			s.escape = true
			add(i, i+1, KindEscape, func() Class {
				if s.mode == configQuote {
					return ClassString
				}
				return 0
			}())
			i++
			continue
		}
		if s.mode == configQuote {
			add(i, i+n, KindString, ClassString)
			i += n
			continue
		}
		if s.wordOn {
			end := space(c) || strings.ContainsRune("\"'{}[]();=", rune(c))
			if b.id == HTTP && s.httpStage == 0 {
				end = space(c)
			}
			if (b.id == Makefile || b.id == HTTP && s.httpStage == 1 && !s.value) && c == ':' {
				end = true
			}
			if c == '#' && b.id != HTTP && b.id != Dockerfile {
				end = true
			}
			if !end {
				s.word.add(text[i : i+n])
				add(i, i+n, s.word.kind, 0)
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
		first := !s.seen
		if b.id == Dockerfile && first && c == '#' && strings.HasPrefix(strings.ToLower(text[i:]), "# escape=") && i+9 < len(text) {
			if e := text[i+9]; e == '\\' || e == '`' {
				s.dockerEscape = e
			}
		}
		s.seen = true
		if c == '#' && (b.id != HTTP && b.id != Dockerfile || first) || b.id == INI && c == ';' && first {
			s.mode = configComment
			if b.id == CMake && strings.HasPrefix(text[i:], "#[") {
				s.mode = configBracketOpen
				s.bracketComment = true
				s.bracketEquals = 0
				add(i, i+2, KindComment, ClassComment)
				i += 2
			}
			continue
		}
		if b.id == INI && first && c == '[' {
			s.mode = configSection
			continue
		}
		if b.id == CMake && c == '[' {
			s.mode = configBracketOpen
			s.bracketComment = false
			s.bracketEquals = 0
			add(i, i+1, KindString, ClassString)
			i++
			continue
		}
		if b.id == Dockerfile && strings.HasPrefix(text[i:], "<<") {
			s.dockerShell = &shellState{}
			continue
		}
		if c == '\'' || c == '"' {
			s.mode, s.quote = configQuote, c
			add(i, i+1, KindString, ClassString)
			i++
			continue
		}
		if strings.ContainsRune("{}[]();=:", rune(c)) {
			if c == '=' || c == ':' && b.id != Nginx {
				s.value = true
			}
			if b.id == Nginx && strings.ContainsRune("{};", rune(c)) {
				s.directive = true
			}
			if b.id == CMake {
				if c == '(' {
					s.depth++
				}
				if c == ')' {
					s.depth = max(0, s.depth-1)
				}
			}
			add(i, i+1, KindDelimiter, ClassDelimiter)
			i++
			continue
		}
		if s.prefixAssignment && s.value && b.id == Makefile {
			s.recipePrefix = c
			s.prefixAssignment = false
		}
		kind := KindPlain
		switch b.id {
		case INI:
			if !s.value {
				kind = KindProperty
			}
		case Nginx:
			if s.directive {
				kind = KindKeyword
				s.directive = false
			}
		case CMake:
			if s.depth == 0 {
				kind = KindKeyword
			}
		case Makefile:
			if !s.recipe && !s.value {
				kind = KindProperty
			}
		case Dockerfile:
			if first && !s.continued {
				kind = KindKeyword
			}
			if strings.HasPrefix(text[i:], "--") {
				kind = KindAttribute
			}
		case HTTP:
			if s.httpStage == 0 {
				kind = KindString
			}
			if s.httpStage == 1 && !s.value {
				kind = KindProperty
			}
			if first && c == '@' {
				kind = KindVariable
			}
		}
		if digit(c) && kind == KindPlain {
			kind = KindNumber
		}
		s.word = scanWord{start: in.Offset + i, kind: kind}
		s.wordOn = true
		// The first rune is consumed here so punctuation that belongs to a
		// language word (URLs, options, directive names) cannot stall scanning.
		s.word.add(text[i : i+n])
		add(start, i+n, kind, 0)
		i += n
	}
	if in.LineEnd && i == len(text) {
		finishWord(in.Offset + i)
		if b.id == HTTP && s.httpStage == 1 && !s.seen && s.mode == configCode {
			s.httpStage = 2
		}
		if s.mode == configComment {
			s.mode = configCode
		}
		if s.mode == configQuote && b.id != CMake && b.id != Nginx {
			s.mode = configCode
			s.quote = 0
		}
		if s.mode == configBracketOpen {
			s.mode = configCode
		}
		if b.id == INI && s.mode == configSection {
			s.mode = configCode
		}
		if b.id == HTTP && s.httpRequest && s.httpStage == 0 {
			s.httpStage = 1
		}
		if b.id == Dockerfile && s.dockerShell != nil && s.dockerShell.active == nil && len(s.dockerShell.pending) == 0 && !s.dockerShell.capture {
			s.dockerShell = nil
		}
		if !s.escape {
			s.value = false
			s.continued = false
			s.recipe = false
			s.prefixAssignment = false
		} else {
			s.continued = true
		}
		s.escape = false
		s.seen = false
		s.httpHeader = ""
		s.httpRequest = false
		if s.variable && s.variableClose == 0 {
			s.variable = false
			s.mode = s.variableBack
		}
	}
	if !s.variable {
		s.variableClose = 0
		s.variableDepth = 0
		s.variableBack = configCode
	}
	if s.mode != configBracket && s.mode != configBracketOpen {
		s.bracketComment = false
		s.bracketEquals = 0
		s.bracketMatch = 0
	}
	return w.result(i, s), nil
}

const utf8RuneSelf = 0x80

func lexicalClass(k Kind) Class {
	switch k {
	case KindString, KindEscape, KindEntity:
		return ClassString
	case KindComment:
		return ClassComment
	case KindDelimiter:
		return ClassDelimiter
	}
	return 0
}
