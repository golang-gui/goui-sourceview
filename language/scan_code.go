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

// codeState contains lexical context only. JS/TS share a scanner with separate
// vocabularies; Python interpolation and SQL quoting have explicit states.
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
	if s.dollarCapture != t.dollarCapture && captureText(s.dollarCapture) != captureText(t.dollarCapture) {
		return false
	}
	s.strings, t.strings = nil, nil
	s.parens, t.parens = nil, nil
	s.css, t.css = nil, nil
	s.dollarCapture, t.dollarCapture = nil, nil
	return s == t
}

func (s codeState) Context() Context {
	return Context{String: s.mode == codeQuoted || s.mode == codeRegexp || s.mode == codeRegexpFlags || s.mode == codeDollarBody,
		Comment: s.mode == codeLineComment || s.mode == codeBlockComment, Literal: s.mode == codeDollarBody || s.mode == codeQuoted && s.raw}
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
	case JavaScript:
		return jsKeywords[word]
	case TypeScript:
		return jsKeywords[word] || tsKeywords[word]
	case Protobuf:
		return protoKeywords[word]
	case SQL:
		return sqlKeywords[strings.ToUpper(word)]
	}
	return false
}
func codeNameStart(id ID, r rune) bool {
	return IdentifierRune(r) && !(r >= '0' && r <= '9') || r == '$' && (id == JavaScript || id == TypeScript || id == SQL) || r == '-' && id == CSS
}
func codeNamePart(id ID, r rune) bool {
	return IdentifierRune(r) || r == '$' && (id == JavaScript || id == TypeScript || id == SQL) || r == '-' && id == CSS
}

func (b codeTokenizer) Scan(in Input, state State) (Result, error) {
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
	var capture strings.Builder
	flushCapture := func() {
		if capture.Len() > 0 {
			s.dollarCapture = &scanText{previous: s.dollarCapture, text: capture.String()}
			capture.Reset()
		}
	}
	finishWord := func(end int) {
		if !s.wordOn {
			return
		}
		word := s.word
		kind := word.kind
		if !s.lastDot && codeKeyword(b.id, word.text) {
			kind = KindKeyword
		}
		if b.id == CSS && strings.HasPrefix(word.text, "@") {
			kind = KindMeta
			switch strings.ToLower(word.text) {
			case "@media", "@supports", "@layer", "@container", "@keyframes", "@-webkit-keyframes", "@document", "@scope", "@starting-style":
				s.cssGroup = true
			}
		}
		if js {
			s.canRegexp = kind == KindKeyword && wordsBeforeExpression[word.text]
			s.pendingControl = controlKeywords[word.text]
		}
		w.add(word.start, end, kind, 0)
		s.word, s.wordOn = scanWord{}, false
		s.lastDot = false
	}
	startQuote := func(quote byte, raw, formatted bool) {
		s.mode, s.quote, s.raw, s.formatted, s.escape = codeQuoted, quote, raw, formatted, false
		s.triple = b.id == Python && strings.HasPrefix(text[i:], strings.Repeat(string(quote), 3))
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
		switch s.mode {
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
			if digit(c) || strings.ContainsRune("abcdefABCDEFxXoOpPeEjJnNiI_.", r) || (c == '+' || c == '-') && strings.ContainsRune("eEpP", rune(s.lastNumber)) || b.id == CSS && (alpha(c) || c == '%') {
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
				if s.raw && b.id == Python {
					kind = KindString
				}
				add(i, i+n, kind, ClassString)
				i += n
				continue
			}
			if c == '\\' && (!s.raw || b.id == Python) {
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
			if codeNamePart(b.id, r) {
				s.word.add(text[i : i+n])
				add(i, i+n, s.word.kind, 0)
				i += n
				continue
			}
			if b.id == Python && (c == '\'' || c == '"') && pythonStringPrefix(s.word.text) {
				prefix := strings.ToLower(s.word.text)
				w.add(s.word.start, in.Offset+i, KindString, ClassString)
				s.word, s.wordOn = scanWord{}, false
				startQuote(c, strings.Contains(prefix, "r"), strings.ContainsAny(prefix, "ft"))
				continue
			}
			finishWord(in.Offset + i)
			continue
		}
		if space(c) {
			s.sqlColon = false
			i += n
			continue
		}
		if b.id == Python && c == '#' || b.id == SQL && strings.HasPrefix(text[i:], "--") || js && in.Offset+i == 0 && strings.HasPrefix(text[i:], "#!") {
			s.mode = codeLineComment
			continue
		}
		if b.id != Python && b.id != CSS && b.id != SQL && strings.HasPrefix(text[i:], "//") {
			s.mode = codeLineComment
			continue
		}
		if b.id != Python && strings.HasPrefix(text[i:], "/*") {
			s.mode = codeBlockComment
			s.blockDepth = 1
			add(i, i+2, KindComment, ClassComment)
			i += 2
			continue
		}
		if c == '"' || c == '\'' || c == '`' && (b.id == Go || js || b.id == SQL) {
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
		if digit(c) || (c == '.' || b.id == CSS && (c == '-' || c == '+')) && i+1 < len(text) && digit(text[i+1]) {
			s.mode = codeNumber
			s.canRegexp = false
			s.lastNumber = c
			add(i, i+1, KindNumber, 0)
			i++
			continue
		}
		if codeNameStart(b.id, r) || b.id == Python && c == '@' || b.id == CSS && (c == '@' || c == '#' || c == '.') {
			s.sqlColon = false
			kind := KindPlain
			if b.id == Python && c == '@' {
				kind = KindMeta
			}
			if b.id == CSS {
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
			if f := s.strings; f != nil && s.braces == f.braces {
				s.mode, s.quote, s.triple, s.raw, s.formatted = codeQuoted, f.quote, f.triple, f.raw, f.formatted
				s.strings = f.parent
				add(i, i+1, KindDelimiter, ClassDelimiter)
				i++
				continue
			}
			s.braces = max(0, s.braces-1)
		}
		if b.id == CSS {
			switch c {
			case '{':
				s.css = &cssFrame{parent: s.css, declaration: s.cssDeclaration, value: s.cssValue}
				s.cssDeclaration = !s.cssGroup
				s.cssValue = false
				s.cssGroup = false
			case '}':
				if s.css != nil {
					s.cssDeclaration, s.cssValue = s.css.declaration, s.css.value
					s.css = s.css.parent
				}
			case ':':
				if s.cssDeclaration {
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
