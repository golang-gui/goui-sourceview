package language

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

type codeMode uint8

const (
	codeNormal codeMode = iota
	codeQuoted
	codeLineComment
	codeBlockComment
	codeNumber
	codeRegexp
	codeRegexpFlags
	codeDollarOpen
	codeDollarBody
	codeURL
	codeStyleVariable
)

type stringFrame struct {
	parent                 *stringFrame
	quote                  byte
	triple, raw, formatted bool
	braces                 int
}
type parenFrame struct {
	parent  *parenFrame
	control bool
}
type cssFrame struct {
	parent             *cssFrame
	declaration, value bool
}

type styleFrame struct {
	parent                    *styleFrame
	mode                      codeMode
	quote                     byte
	raw                       bool
	braces, blockDepth        int
	css                       *cssFrame
	declaration, value, group bool
}

// codeState contains lexical context only. Language IDs keep shared rules
// isolated; string interpolation, CSS declarations and SQL quoting retain
// their own immutable snapshots.
type codeState struct {
	id                                          ID
	mode                                        codeMode
	quote                                       byte
	triple, raw, formatted, escape, regexpClass bool
	blockDepth, braces                          int
	strings                                     *stringFrame
	parens                                      *parenFrame
	canRegexp, pendingControl, lastDot          bool
	word                                        scanWord
	wordOn                                      bool
	sqlColon                                    bool
	lastNumber                                  byte
	dollar                                      string
	dollarMatch, dollarStart                    int
	dollarCapture                               *scanText
	css                                         *cssFrame
	cssDeclaration, cssValue, cssGroup          bool
	styles                                      *styleFrame
	styleEscapeLeft                             int
	styleEscapeSpace                            bool
	styleVariableBack                           codeMode
}

func (s codeState) Equal(other State) bool {
	t, ok := other.(codeState)
	if !ok {
		return false
	}
	for a, b := s.strings, t.strings; a != b; {
		if a == nil || b == nil || a.quote != b.quote || a.triple != b.triple || a.raw != b.raw || a.formatted != b.formatted || a.braces != b.braces {
			return false
		}
		a, b = a.parent, b.parent
	}
	for a, b := s.parens, t.parens; a != b; {
		if a == nil || b == nil || a.control != b.control {
			return false
		}
		a, b = a.parent, b.parent
	}
	for a, b := s.css, t.css; a != b; {
		if a == nil || b == nil || a.declaration != b.declaration || a.value != b.value {
			return false
		}
		a, b = a.parent, b.parent
	}
	for a, b := s.styles, t.styles; a != b; a, b = a.parent, b.parent {
		if a == nil || b == nil || a.mode != b.mode || a.quote != b.quote || a.raw != b.raw || a.braces != b.braces || a.blockDepth != b.blockDepth || a.declaration != b.declaration || a.value != b.value || a.group != b.group {
			return false
		}
		for x, y := a.css, b.css; x != y; x, y = x.parent, y.parent {
			if x == nil || y == nil || x.declaration != y.declaration || x.value != y.value {
				return false
			}
		}
	}
	if s.dollarCapture != t.dollarCapture && captureText(s.dollarCapture) != captureText(t.dollarCapture) {
		return false
	}
	s.strings, t.strings = nil, nil
	s.parens, t.parens = nil, nil
	s.css, t.css = nil, nil
	s.styles, t.styles = nil, nil
	s.dollarCapture, t.dollarCapture = nil, nil
	return s == t
}

func (s codeState) Context() Context {
	mode := s.mode
	if mode == codeStyleVariable {
		mode = s.styleVariableBack
	}
	return Context{String: mode == codeQuoted || mode == codeRegexp || mode == codeRegexpFlags || mode == codeDollarBody || mode == codeURL,
		Comment: mode == codeLineComment || mode == codeBlockComment, Literal: mode == codeDollarBody || mode == codeQuoted && s.raw && s.id != Starlark}
}

type codeTokenizer struct{ id ID }

func (b codeTokenizer) InitialState() State { return codeState{id: b.id, canRegexp: true} }

var goKeywords = words("break default func interface select case defer go map struct chan else goto package switch const fallthrough if range type continue for import return var bool byte complex64 complex128 error float32 float64 int int8 int16 int32 int64 rune string uint uint8 uint16 uint32 uint64 uintptr any comparable true false iota nil")
var pythonKeywords = words("False None True and as assert async await break class continue def del elif else except finally for from global if import in is lambda nonlocal not or pass raise return try while with yield match case type")
var jsKeywords = words("as async await break case catch class const continue debugger default delete do else export extends false finally for from function get if import in instanceof let new null of return set static super switch this throw true try typeof undefined var void while with yield")
var tsKeywords = words("abstract accessor any asserts bigint boolean constructor declare enum global implements infer interface intrinsic is keyof module namespace never number object out override private protected public readonly require satisfies string symbol type unique unknown using")
var protoKeywords = words("syntax edition import weak public package option optional required repeated oneof map reserved extensions to max enum message service rpc returns stream extend group double float int32 int64 uint32 uint64 sint32 sint64 fixed32 fixed64 sfixed32 sfixed64 bool string bytes true false inf nan")
var sqlKeywords = words("ADD ALL ALTER ANALYZE AND ANY AS ASC BEGIN BETWEEN BIGINT BINARY BOOLEAN BOTH BY CALL CASCADE CASE CAST CHAR CHARACTER CHECK COLLATE COLUMN COMMIT CONSTRAINT CREATE CROSS CURRENT_DATE CURRENT_TIME CURRENT_TIMESTAMP DATABASE DATE DECIMAL DECLARE DEFAULT DELETE DESC DISTINCT DO DOUBLE DROP ELSE END ESCAPE EXCEPT EXISTS EXPLAIN FALSE FETCH FILTER FLOAT FOLLOWING FOR FOREIGN FROM FULL FUNCTION GRANT GROUP HAVING IF ILIKE IN INDEX INNER INSERT INT INTEGER INTERSECT INTERVAL INTO IS JOIN KEY LATERAL LEADING LEFT LIKE LIMIT MATERIALIZED NATURAL NOT NULL NULLS NUMERIC OFFSET ON ONLY OR ORDER OUTER OVER PARTITION PRECEDING PRIMARY PROCEDURE RANGE REAL RECURSIVE REFERENCES RETURN RETURNING REVOKE RIGHT ROLLBACK ROW ROWS SCHEMA SELECT SET SMALLINT TABLE TEMP TEMPORARY TEXT THEN TIME TIMESTAMP TO TRAILING TRANSACTION TRIGGER TRUE TRUNCATE UNION UNIQUE UPDATE USER USING VALUES VARCHAR VIEW WHEN WHERE WINDOW WITH")

func codeKeyword(id ID, word string) bool {
	switch id {
	case Go:
		return goKeywords[word]
	case Python:
		return pythonKeywords[word]
	case Starlark:
		return starlarkKeywords[word]
	case JavaScript:
		return jsKeywords[word]
	case TypeScript:
		return jsKeywords[word] || tsKeywords[word]
	case Protobuf:
		return protoKeywords[word]
	case SQL:
		return sqlKeywords[strings.ToUpper(word)]
	case SCSS:
		return scssKeywords[word]
	case Less:
		return lessKeywords[word]
	}
	return false
}
func cssLanguage(id ID) bool    { return id == CSS || id == SCSS || id == Less }
func pythonLanguage(id ID) bool { return id == Python || id == Starlark }

var starlarkKeywords = words("and break continue def elif else for if in lambda load not or pass return as assert async await class del except finally from global import is nonlocal raise try while with yield False True None")

func codeStringPrefix(id ID, text string) bool {
	if id == Python {
		return pythonStringPrefix(text)
	}
	return id == Starlark && (text == "r" || text == "b" || text == "br" || text == "rb")
}

var scssKeywords = words("true false null and or not from through to in as with using !default !global !optional")
var lessKeywords = words("when and not true false iscolor isnumber isstring iskeyword isurl ispixel ispercentage isem lighten darken saturate desaturate fade mix spin round ceil floor percentage")
var lessAtRules = words("@import @plugin @charset @namespace @media @supports @container @layer @document @keyframes @-webkit-keyframes @font-face @page @property @scope @starting-style @counter-style @font-feature-values")

func codeNameStart(id ID, r rune) bool {
	return IdentifierRune(r) && !(r >= '0' && r <= '9') || r == '$' && (id == JavaScript || id == TypeScript || id == SQL || id == SCSS || id == Less) || r == '-' && cssLanguage(id)
}
func codeNamePart(id ID, r rune) bool {
	return IdentifierRune(r) || r == '$' && (id == JavaScript || id == TypeScript || id == SQL) || r == '-' && cssLanguage(id) || id == Less && r == '@'
}

func finishCodeWord(s *codeState, w *tokenWriter, end int) {
	if !s.wordOn {
		return
	}
	word := s.word
	kind := word.kind
	if !s.lastDot && codeKeyword(s.id, word.text) {
		kind = KindKeyword
	}
	if cssLanguage(s.id) && strings.HasPrefix(word.text, "@") {
		kind = KindMeta
		if s.id == Less && !lessAtRules[strings.ToLower(word.text)] {
			kind = KindVariable
		}
		switch strings.ToLower(word.text) {
		case "@media", "@supports", "@layer", "@container", "@keyframes", "@-webkit-keyframes", "@document", "@scope", "@starting-style":
			s.cssGroup = true
		}
	}
	if s.id == JavaScript || s.id == TypeScript {
		s.canRegexp = kind == KindKeyword && wordsBeforeExpression[word.text]
		s.pendingControl = controlKeywords[word.text]
	}
	w.add(word.start, end, kind, 0)
	s.word, s.wordOn, s.lastDot = scanWord{}, false, false
}

func (b codeTokenizer) Scan(in Input, state State) (Result, error) {
	return b.scan(in, state, nil)
}

// scan can pause at a confirmed code boundary for an enclosing JSX profile.
// Only the private caller may receive zero consumption at that boundary.
func (b codeTokenizer) scan(in Input, state State, boundary func(codeState, int) bool) (Result, error) {
	s, ok := state.(codeState)
	if !ok || s.id != b.id {
		return Result{}, fmt.Errorf("language: foreign state for %s", b.id)
	}
	limit, err := scanLimit(in)
	if err != nil {
		return Result{}, err
	}
	w := tokenWriter{base: in.Offset}
	text := in.Text
	i := 0
	add := func(a, z int, k Kind, c Class) { w.add(in.Offset+a, in.Offset+z, k, c) }
	js := b.id == JavaScript || b.id == TypeScript
	css := cssLanguage(b.id)
	preprocessor := css && b.id != CSS
	var capture strings.Builder
	flushCapture := func() {
		if capture.Len() > 0 {
			s.dollarCapture = &scanText{previous: s.dollarCapture, text: capture.String()}
			capture.Reset()
		}
	}
	finishWord := func(end int) { finishCodeWord(&s, &w, end) }
	startQuote := func(quote byte, raw, formatted bool) {
		s.mode, s.quote, s.raw, s.formatted, s.escape = codeQuoted, quote, raw, formatted, false
		s.triple = pythonLanguage(b.id) && strings.HasPrefix(text[i:], strings.Repeat(string(quote), 3))
		n := 1
		if s.triple {
			n = 3
		}
		add(i, i+n, KindString, ClassString)
		i += n
		s.canRegexp = false
	}
	for i < limit {
		c := text[i]
		r, n := utf8.DecodeRuneInString(text[i:])
		if preprocessor {
			class := Class(0)
			if s.mode == codeQuoted || s.mode == codeURL {
				class = ClassString
			}
			if s.escape && hex(c) {
				s.escape = false
				s.styleEscapeLeft, s.styleEscapeSpace = 5, true
				add(i, i+1, KindEscape, class)
				i++
				continue
			}
			if s.styleEscapeLeft > 0 {
				if hex(c) {
					s.styleEscapeLeft--
					add(i, i+1, KindEscape, class)
					i++
					continue
				}
				s.styleEscapeLeft = 0
			}
			if s.styleEscapeSpace {
				s.styleEscapeSpace = false
				if space(c) {
					add(i, i+1, KindEscape, class)
					i++
					continue
				}
			}
		}
		if b.id == SCSS && !s.wordOn && !s.escape && strings.HasPrefix(text[i:], "#{") && (s.mode == codeNormal || s.mode == codeQuoted || s.mode == codeURL || s.mode == codeBlockComment) {
			s.styles = &styleFrame{parent: s.styles, mode: s.mode, quote: s.quote, raw: s.raw, braces: s.braces, blockDepth: s.blockDepth, css: s.css, declaration: s.cssDeclaration, value: s.cssValue, group: s.cssGroup}
			s.mode, s.quote, s.raw, s.css, s.cssDeclaration, s.cssValue, s.cssGroup = codeNormal, 0, false, nil, false, true, false
			add(i, i+2, KindDelimiter, ClassDelimiter)
			i += 2
			continue
		}
		if b.id == Less && !s.wordOn && !s.escape && s.quote != '`' && strings.HasPrefix(text[i:], "@{") && (s.mode == codeNormal || s.mode == codeQuoted || s.mode == codeURL) {
			class := Class(0)
			if s.mode != codeNormal {
				class = ClassString
			}
			s.styleVariableBack, s.mode = s.mode, codeStyleVariable
			add(i, i+2, KindVariable, class)
			i += 2
			continue
		}
		switch s.mode {
		case codeStyleVariable:
			class := Class(0)
			if s.styleVariableBack == codeQuoted || s.styleVariableBack == codeURL {
				class = ClassString
			}
			add(i, i+n, KindVariable, class)
			i += n
			if c == '}' {
				s.mode, s.styleVariableBack = s.styleVariableBack, codeNormal
			}
			continue
		case codeURL:
			if s.escape {
				s.escape = false
				add(i, i+n, KindEscape, ClassString)
				i += n
				continue
			}
			if c == '\\' {
				s.escape = true
				add(i, i+1, KindEscape, ClassString)
				i++
				continue
			}
			if s.quote == 0 && c == ')' {
				s.mode = codeNormal
				add(i, i+1, KindDelimiter, ClassDelimiter)
				i++
				continue
			}
			if s.quote != 0 && c == s.quote {
				s.quote = 0
			} else if s.quote == 0 && (c == '"' || c == '\'') {
				s.quote = c
			}
			add(i, i+n, KindString, ClassString)
			i += n
			continue
		case codeLineComment:
			add(i, i+n, KindComment, ClassComment)
			i += n
			continue
		case codeBlockComment:
			if b.id == SQL && strings.HasPrefix(text[i:], "/*") {
				s.blockDepth++
				add(i, i+2, KindComment, ClassComment)
				i += 2
				continue
			}
			if strings.HasPrefix(text[i:], "*/") {
				s.blockDepth--
				if s.blockDepth == 0 {
					s.mode = codeNormal
				}
				add(i, i+2, KindComment, ClassComment)
				i += 2
				continue
			}
			add(i, i+n, KindComment, ClassComment)
			i += n
			continue
		case codeDollarOpen:
			if c == '$' {
				capture.WriteByte(c)
				flushCapture()
				s.dollar = captureText(s.dollarCapture)
				s.dollarCapture = nil
				w.add(s.dollarStart, in.Offset+i+1, KindString, ClassString)
				s.dollarStart = 0
				s.mode = codeDollarBody
				s.dollarMatch = 0
				i++
				continue
			}
			if !IdentifierRune(r) {
				flushCapture()
				s.dollarCapture = nil
				s.dollarStart = 0
				s.mode = codeNormal
				continue
			}
			capture.WriteString(text[i : i+n])
			add(i, i+n, KindVariable, 0)
			i += n
			continue
		case codeDollarBody:
			for j := i; j < i+n; j++ {
				if text[j] == s.dollar[s.dollarMatch] {
					s.dollarMatch++
				} else {
					s.dollarMatch = 0
					if text[j] == '$' {
						s.dollarMatch = 1
					}
				}
				if s.dollarMatch == len(s.dollar) {
					s.mode = codeNormal
					s.dollar = ""
					s.dollarMatch = 0
					break
				}
			}
			add(i, i+n, KindString, ClassString)
			i += n
			continue
		case codeRegexpFlags:
			if alpha(c) {
				add(i, i+1, KindString, ClassString)
				i++
				continue
			}
			s.mode = codeNormal
			continue
		case codeRegexp:
			if s.escape {
				s.escape = false
				add(i, i+n, KindEscape, ClassString)
				i += n
				continue
			}
			if c == '\\' {
				s.escape = true
				add(i, i+1, KindEscape, ClassString)
				i++
				continue
			}
			if c == '[' {
				s.regexpClass = true
			}
			if c == ']' {
				s.regexpClass = false
			}
			if c == '/' && !s.regexpClass {
				s.mode = codeRegexpFlags
				s.canRegexp = false
			}
			add(i, i+n, KindString, ClassString)
			i += n
			continue
		case codeNumber:
			if digit(c) || strings.ContainsRune("abcdefABCDEFxXoOpPeEjJnNiI_.", r) || (c == '+' || c == '-') && strings.ContainsRune("eEpP", rune(s.lastNumber)) || css && (alpha(c) || c == '%') {
				add(i, i+n, KindNumber, 0)
				s.lastNumber = c
				i += n
				continue
			}
			s.mode = codeNormal
			s.lastNumber = 0
			continue
		case codeQuoted:
			if s.escape {
				s.escape = false
				kind := KindEscape
				if s.raw && pythonLanguage(b.id) {
					kind = KindString
				}
				add(i, i+n, kind, ClassString)
				i += n
				continue
			}
			if c == '\\' && (!s.raw || pythonLanguage(b.id)) {
				s.escape = true
				kind := KindEscape
				if s.raw {
					kind = KindString
				}
				add(i, i+1, kind, ClassString)
				i++
				continue
			}
			if b.id == SQL && c == s.quote && i+1 < len(text) && text[i+1] == s.quote {
				add(i, i+2, KindString, ClassString)
				i += 2
				continue
			}
			if c == s.quote {
				count := 1
				if s.triple {
					count = 3
				}
				if !s.triple || strings.HasPrefix(text[i:], strings.Repeat(string(c), count)) {
					add(i, i+count, KindString, ClassString)
					i += count
					s.mode = codeNormal
					s.quote = 0
					s.raw = false
					s.triple = false
					s.formatted = false
					continue
				}
			}
			if s.formatted && (js && strings.HasPrefix(text[i:], "${") || b.id == Python && c == '{') {
				if b.id == Python && strings.HasPrefix(text[i:], "{{") {
					add(i, i+2, KindString, ClassString)
					i += 2
					continue
				}
				s.strings = &stringFrame{parent: s.strings, quote: s.quote, triple: s.triple, raw: s.raw, formatted: s.formatted, braces: s.braces}
				s.mode = codeNormal
				s.quote = 0
				s.raw = false
				s.triple = false
				s.formatted = false
				s.canRegexp = true
				size := 1
				if js {
					size = 2
				}
				add(i, i+size, KindDelimiter, ClassDelimiter)
				i += size
				continue
			}
			if b.id == Python && s.formatted && strings.HasPrefix(text[i:], "}}") {
				add(i, i+2, KindString, ClassString)
				i += 2
				continue
			}
			add(i, i+n, KindString, ClassString)
			i += n
			continue
		}
		if s.wordOn {
			if b.id == Less && c == '@' && (!strings.HasPrefix(s.word.text, "@") || strings.HasPrefix(text[i:], "@{")) {
				finishWord(in.Offset + i)
				continue
			}
			if codeNamePart(b.id, r) {
				s.word.add(text[i : i+n])
				add(i, i+n, s.word.kind, 0)
				i += n
				continue
			}
			if (c == '\'' || c == '"') && codeStringPrefix(b.id, s.word.text) {
				prefix := strings.ToLower(s.word.text)
				w.add(s.word.start, in.Offset+i, KindString, ClassString)
				s.word, s.wordOn = scanWord{}, false
				startQuote(c, strings.Contains(prefix, "r"), b.id == Python && strings.ContainsAny(prefix, "ft"))
				continue
			}
			url := preprocessor && strings.EqualFold(s.word.text, "url") && c == '('
			finishWord(in.Offset + i)
			if url {
				s.mode, s.quote = codeURL, 0
				add(i, i+1, KindDelimiter, ClassDelimiter)
				i++
				continue
			}
			continue
		}
		if boundary != nil && s.mode == codeNormal && boundary(s, i) {
			break
		}
		if preprocessor && s.escape {
			s.escape = false
			add(i, i+n, KindEscape, 0)
			i += n
			continue
		}
		if preprocessor && c == '\\' {
			s.escape = true
			add(i, i+1, KindEscape, 0)
			i++
			continue
		}
		if space(c) {
			s.sqlColon = false
			i += n
			continue
		}
		if pythonLanguage(b.id) && c == '#' || b.id == SQL && strings.HasPrefix(text[i:], "--") || js && in.Offset+i == 0 && strings.HasPrefix(text[i:], "#!") {
			s.mode = codeLineComment
			continue
		}
		if !pythonLanguage(b.id) && b.id != CSS && b.id != SQL && strings.HasPrefix(text[i:], "//") {
			s.mode = codeLineComment
			continue
		}
		if !pythonLanguage(b.id) && strings.HasPrefix(text[i:], "/*") {
			s.mode = codeBlockComment
			s.blockDepth = 1
			add(i, i+2, KindComment, ClassComment)
			i += 2
			continue
		}
		if c == '"' || c == '\'' || c == '`' && (b.id == Go || js || b.id == SQL || b.id == Less) {
			startQuote(c, c == '`' && (b.id == Go || js), c == '`' && js)
			// Template strings still process escapes, unlike Go raw strings.
			if js {
				s.raw = false
			}
			continue
		}
		if b.id == SQL && c == '[' {
			startQuote(']', true, false)
			continue
		}
		if b.id == SQL && c == '$' && i+1 < len(text) && (text[i+1] == '$' || alpha(text[i+1]) || text[i+1] == '_') {
			s.mode = codeDollarOpen
			s.dollarStart = in.Offset + i
			capture.WriteByte('$')
			add(i, i+1, KindVariable, 0)
			i++
			continue
		}
		if b.id == SQL && (c == ':' || c == '$' || c == '@') && i+1 < len(text) && text[i+1] != c && !(c == ':' && s.sqlColon) {
			s.word = scanWord{start: in.Offset + i, kind: KindVariable}
			s.wordOn = true
			s.word.add(string(c))
			add(i, i+1, KindVariable, 0)
			i++
			continue
		}
		if js && c == '/' && s.canRegexp {
			s.mode = codeRegexp
			s.regexpClass = false
			s.escape = false
			add(i, i+1, KindString, ClassString)
			i++
			continue
		}
		if digit(c) || (c == '.' || css && (c == '-' || c == '+')) && i+1 < len(text) && digit(text[i+1]) {
			s.mode = codeNumber
			s.canRegexp = false
			s.lastNumber = c
			add(i, i+1, KindNumber, 0)
			i++
			continue
		}
		if codeNameStart(b.id, r) || b.id == Python && c == '@' || css && (c == '@' || c == '#' || c == '.') {
			s.sqlColon = false
			kind := KindPlain
			if b.id == Python && c == '@' {
				kind = KindMeta
			}
			if css {
				kind = KindTag
				if s.cssDeclaration && !s.cssValue {
					kind = KindProperty
				}
				if s.cssValue {
					kind = KindKeyword
				}
				if c == '@' {
					kind = KindMeta
				}
				if strings.HasPrefix(text[i:], "--") {
					kind = KindVariable
				}
				if c == '#' && s.cssValue {
					kind = KindNumber
				}
				if preprocessor && (c == '.' || c == '#' && !s.cssValue) {
					kind = KindTag
				}
				if b.id == SCSS && c == '$' {
					kind = KindVariable
				}
				if b.id == Less && (c == '@' || c == '$') {
					kind = KindVariable
				}
			}
			s.word = scanWord{start: in.Offset + i, kind: kind}
			s.wordOn = true
			s.word.add(text[i : i+n])
			add(i, i+n, kind, 0)
			i += n
			continue
		}
		if c == '{' {
			s.braces++
		}
		if c == '}' {
			if f := s.styles; f != nil && s.braces == f.braces {
				s.mode, s.quote, s.raw, s.blockDepth = f.mode, f.quote, f.raw, f.blockDepth
				s.css, s.cssDeclaration, s.cssValue, s.cssGroup = f.css, f.declaration, f.value, f.group
				s.styles = f.parent
				add(i, i+1, KindDelimiter, ClassDelimiter)
				i++
				continue
			}
			if f := s.strings; f != nil && s.braces == f.braces {
				s.mode, s.quote, s.triple, s.raw, s.formatted = codeQuoted, f.quote, f.triple, f.raw, f.formatted
				s.strings = f.parent
				add(i, i+1, KindDelimiter, ClassDelimiter)
				i++
				continue
			}
			s.braces = max(0, s.braces-1)
		}
		if css {
			switch c {
			case '{':
				value := s.cssValue
				if preprocessor {
					value = false
				}
				s.css = &cssFrame{parent: s.css, declaration: s.cssDeclaration, value: value}
				s.cssDeclaration = !s.cssGroup
				s.cssValue = false
				s.cssGroup = false
			case '}':
				if s.css != nil {
					s.cssDeclaration, s.cssValue = s.css.declaration, s.css.value
					s.css = s.css.parent
				}
			case ':':
				if s.cssDeclaration || preprocessor {
					s.cssValue = true
				}
			case ';':
				s.cssValue = false
				s.cssGroup = false
			}
		}
		if js {
			if c == '(' {
				s.parens = &parenFrame{parent: s.parens, control: s.pendingControl}
				s.pendingControl = false
			}
			if c == ')' {
				s.canRegexp = s.parens != nil && s.parens.control
				if s.parens != nil {
					s.parens = s.parens.parent
				}
			} else {
				s.canRegexp = c != ']' && c != '}' && c != '.'
				if (c == '+' || c == '-') && i+1 < len(text) && text[i+1] == c {
					add(i, i+2, KindDelimiter, 0)
					i += 2
					s.canRegexp = false
					continue
				}
			}
			s.lastDot = c == '.'
		}
		class := Class(0)
		s.sqlColon = b.id == SQL && c == ':'
		if strings.ContainsRune("()[]{}", r) {
			class = ClassDelimiter
		}
		add(i, i+n, KindDelimiter, class)
		i += n
	}
	flushCapture()
	if in.LineEnd && i == len(text) {
		finishWord(in.Offset + i)
		if s.mode == codeStyleVariable {
			s.mode, s.styleVariableBack = s.styleVariableBack, codeNormal
		}
		if s.mode == codeLineComment || s.mode == codeNumber || s.mode == codeRegexp || s.mode == codeRegexpFlags || s.mode == codeDollarOpen {
			s.mode = codeNormal
			s.lastNumber = 0
			s.dollarCapture = nil
			s.dollarStart = 0
			s.regexpClass = false
		}
		if s.mode == codeQuoted && !s.triple && s.quote != '`' && b.id != SQL && !s.escape {
			s.mode = codeNormal
			s.quote = 0
			s.raw = false
			s.formatted = false
		}
		if s.mode == codeQuoted && s.escape {
			s.escape = false
		}
		if preprocessor {
			s.escape, s.styleEscapeLeft, s.styleEscapeSpace = false, 0, false
		}
	}
	return w.result(i, s), nil
}

var wordsBeforeExpression = words("return throw case delete void typeof new in of instanceof yield await else do")
var controlKeywords = words("if while for with switch catch")

func pythonStringPrefix(s string) bool {
	switch strings.ToLower(s) {
	case "r", "u", "b", "f", "br", "rb", "fr", "rf", "t", "tr", "rt":
		return true
	}
	return false
}
