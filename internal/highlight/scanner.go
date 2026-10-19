// Package highlight implements stateful, bounded source tokenization.
package highlight

import (
	"strings"
	"unicode/utf8"

	"github.com/golang-gui/goui-sourceview/language"
)

// Token is a UTF-8 byte span. Engine spans are line-relative; Scan spans are
// relative to its input and may revise an earlier range in the current line.
type Token struct {
	Start, End int
	Kind       language.Kind
}

type mode uint8

const (
	data mode = iota
	jsonString
	jsonNumber
	tagName
	tagInside
	attrName
	afterAttr
	beforeValue
	attrSingle
	attrDouble
	attrUnquoted
	comment
	cdata
	pi
	declaration
	entity
	rawText
	rcdata
	scriptData
	plainText
)

type frame struct {
	parent *frame
	array  bool
	expect uint8
	depth  int
}

// State is an immutable snapshot. Stacks, queues and delimiter spellings
// referenced by an earlier snapshot are never mutated.
type State struct {
	mode     mode
	back     mode
	tag      string
	closing  bool
	quote    byte
	depth    int
	stack    *frame
	key      bool
	number   uint8
	overflow int
	script   uint8
	toml     *tomlState
	yaml     *yamlState
	shell    *shellState
}

// Equal compares complete lexical context, not hashes or line numbers.
func (s State) Equal(t State) bool {
	if !equalTOML(s.toml, t.toml) || !equalYAML(s.yaml, t.yaml) || !equalShell(s.shell, t.shell) {
		return false
	}
	s.toml, t.toml = nil, nil
	s.yaml, t.yaml = nil, nil
	s.shell, t.shell = nil, nil
	a, b := s.stack, t.stack
	s.stack, t.stack = nil, nil
	if s != t {
		return false
	}
	for a != b {
		if a == nil || b == nil || a.array != b.array || a.expect != b.expect || a.depth != b.depth {
			return false
		}
		a, b = a.parent, b.parent
	}
	return true
}

func Initial() State { return State{} }

func emit(out *[]Token, start, end int, kind language.Kind) {
	if start >= end || kind == language.KindPlain {
		return
	}
	tokens := *out
	if len(tokens) > 0 && tokens[len(tokens)-1].End == start && tokens[len(tokens)-1].Kind == kind {
		tokens[len(tokens)-1].End = end
	} else {
		tokens = append(tokens, Token{start, end, kind})
	}
	*out = tokens
}
func space(b byte) bool { return b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '\f' }
func alpha(b byte) bool { return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' }
func digit(b byte) bool { return b >= '0' && b <= '9' }
func hex(b byte) bool   { return digit(b) || b >= 'a' && b <= 'f' || b >= 'A' && b <= 'F' }
func lower(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + 32
	}
	return b
}
func asciiPrefix(s, p string) bool {
	if len(s) < len(p) {
		return false
	}
	for i := range len(p) {
		if lower(s[i]) != p[i] {
			return false
		}
	}
	return true
}
func nameStart(r rune) bool {
	return r == ':' || r == '_' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' ||
		r >= 0xc0 && r <= 0xd6 || r >= 0xd8 && r <= 0xf6 || r >= 0xf8 && r <= 0x2ff ||
		r >= 0x370 && r <= 0x37d || r >= 0x37f && r <= 0x1fff || r >= 0x200c && r <= 0x200d ||
		r >= 0x2070 && r <= 0x218f || r >= 0x2c00 && r <= 0x2fef || r >= 0x3001 && r <= 0xd7ff ||
		r >= 0xf900 && r <= 0xfdcf || r >= 0xfdf0 && r <= 0xfffd || r >= 0x10000 && r <= 0xeffff
}
func nameRune(r rune) bool {
	return nameStart(r) || r == '-' || r == '.' || r >= '0' && r <= '9' || r == 0xb7 || r >= 0x300 && r <= 0x36f || r >= 0x203f && r <= 0x2040
}
func (s *State) valueDone() {
	if s.stack != nil {
		p := *s.stack
		p.expect = 3
		s.stack = &p
	}
}
func (s *State) push(array bool) {
	if s.overflow > 0 {
		s.overflow++
		return
	}
	depth := 1
	if s.stack != nil {
		depth = s.stack.depth + 1
	}
	if depth > 1024 {
		s.overflow = 1
		return
	}
	s.stack = &frame{parent: s.stack, array: array, depth: depth}
}
func (s *State) close() {
	if s.overflow > 0 {
		s.overflow--
		if s.overflow == 0 {
			s.valueDone()
		}
		return
	}
	if s.stack != nil {
		s.stack = s.stack.parent
	}
	s.valueDone()
}

// Scan consumes at least limit bytes, possibly a few lookahead bytes needed
// to finish a delimiter. input excludes LF, is valid UTF-8, and may contain up
// to 32 lookahead bytes. lineEnd is true only at an actual line/document end.
// Returned positions are relative to input. A negative start revises an earlier
// span of this logical line (for example, a key confirmed by a later colon).
// Callers apply tokens in order using appendToken, adding their line offset.
func Scan(id language.ID, state State, input string, limit int, lineEnd bool) ([]Token, State, int) {
	if limit < 0 || limit > len(input) {
		panic("highlight: invalid scan limit")
	}
	if id == language.Plain {
		return nil, state, limit
	}
	if id == language.JSON {
		return scanJSON(state, input, limit, lineEnd)
	}
	switch id {
	case language.TOML:
		return scanTOML(state, input, limit, lineEnd)
	case language.YAML:
		return scanYAML(state, input, limit, lineEnd)
	case language.Shell:
		return scanShell(state, input, limit, lineEnd)
	}
	return scanMarkup(id, state, input, limit, lineEnd)
}

func scanJSON(s State, input string, limit int, lineEnd bool) ([]Token, State, int) {
	var out []Token
	i := 0
	for i < limit {
		start := i
		b := input[i]
		switch s.mode {
		case jsonString:
			kind := language.KindString
			if s.key {
				kind = language.KindProperty
			}
			if b == '"' {
				i++
				emit(&out, start, i, kind)
				s.mode = data
				s.valueDone()
				continue
			}
			if b == '\\' {
				i++
				k := language.KindEscape
				if i == len(input) {
					k = language.KindInvalid
				} else if input[i] == 'u' {
					i++
					n := 0
					for i < len(input) && n < 4 && hex(input[i]) {
						i++
						n++
					}
					if n != 4 {
						k = language.KindInvalid
					}
				} else {
					r, n := utf8.DecodeRuneInString(input[i:])
					i += n
					if !strings.ContainsRune("\"\\/bfnrt", r) {
						k = language.KindInvalid
					}
				}
				emit(&out, start, i, k)
				continue
			}
			_, n := utf8.DecodeRuneInString(input[i:])
			i += n
			if b < 0x20 {
				kind = language.KindInvalid
			}
			emit(&out, start, i, kind)
		case jsonNumber:
			n, next, bad := numberPart(input[i:], s.number)
			if n == 0 {
				s.mode = data
				s.valueDone()
				continue
			}
			i += n
			s.number = next
			atEnd := i == len(input)
			kind := language.KindNumber
			if bad || atEnd && lineEnd && (next == 0 || next == 3 || next == 5 || next == 6) {
				kind = language.KindInvalid
			}
			emit(&out, start, i, kind)
			if !atEnd || lineEnd {
				s.mode = data
				s.valueDone()
			}
		default:
			switch {
			case space(b):
				i++
			case b == '"':
				s.key = s.overflow == 0 && s.stack != nil && !s.stack.array && s.stack.expect == 0
				s.mode = jsonString
				i++
				k := language.KindString
				if s.key {
					k = language.KindProperty
				}
				emit(&out, start, i, k)
			case b == '{' || b == '[':
				s.push(b == '[')
				i++
				emit(&out, start, i, language.KindDelimiter)
			case b == '}' || b == ']':
				s.close()
				i++
				emit(&out, start, i, language.KindDelimiter)
			case b == ',':
				if s.stack != nil {
					p := *s.stack
					p.expect = 0
					s.stack = &p
				}
				i++
				emit(&out, start, i, language.KindDelimiter)
			case b == ':':
				if s.stack != nil {
					p := *s.stack
					p.expect = 2
					s.stack = &p
				}
				i++
				emit(&out, start, i, language.KindDelimiter)
			case b == '-' || digit(b):
				s.mode = jsonNumber
				s.number = 0
			default:
				n := 0
				for _, word := range []string{"true", "false", "null"} {
					if strings.HasPrefix(input[i:], word) && (i+len(word) == len(input) || !nameRune(rune(input[i+len(word)]))) {
						n = len(word)
						break
					}
				}
				if n > 0 {
					i += n
					emit(&out, start, i, language.KindKeyword)
					s.valueDone()
				} else {
					_, n := utf8.DecodeRuneInString(input[i:])
					i += n
					emit(&out, start, i, language.KindInvalid)
				}
			}
		}
	}
	if lineEnd && i == len(input) && s.mode == jsonString {
		s.mode = data
		s.key = false
		s.valueDone()
	}
	if s.mode != jsonString {
		s.key = false
	}
	if s.mode != jsonNumber {
		s.number = 0
	}
	return out, s, i
}

// Number phases: start, zero, integer, fraction-needed, fraction,
// exponent-needed, exponent-sign-needed, exponent.
func numberPart(text string, phase uint8) (int, uint8, bool) {
	i := 0
	bad := false
	for i < len(text) {
		b := text[i]
		switch phase {
		case 0:
			if b == '-' && i == 0 {
				i++
				if i == len(text) {
					return i, 0, true
				}
				b = text[i]
			}
			if b == '0' {
				phase = 1
			} else if b >= '1' && b <= '9' {
				phase = 2
			} else {
				return max(1, i), phase, true
			}
		case 1:
			if digit(b) {
				bad = true
			} else if b == '.' {
				phase = 3
			} else if b == 'e' || b == 'E' {
				phase = 5
			} else {
				return i, phase, bad
			}
		case 2:
			if digit(b) {
			} else if b == '.' {
				phase = 3
			} else if b == 'e' || b == 'E' {
				phase = 5
			} else {
				return i, phase, bad
			}
		case 3:
			if digit(b) {
				phase = 4
			} else {
				return i, phase, true
			}
		case 4:
			if digit(b) {
			} else if b == 'e' || b == 'E' {
				phase = 5
			} else {
				return i, phase, bad
			}
		case 5:
			if b == '+' || b == '-' {
				phase = 6
			} else if digit(b) {
				phase = 7
			} else {
				return i, phase, true
			}
		case 6:
			if digit(b) {
				phase = 7
			} else {
				return i, phase, true
			}
		case 7:
			if !digit(b) {
				return i, phase, bad
			}
		}
		i++
	}
	return i, phase, bad
}

func endTag(text, tag string) bool {
	prefix := "</" + tag
	if !asciiPrefix(text, prefix) {
		return false
	}
	return len(text) == len(prefix) || space(text[len(prefix)]) || text[len(prefix)] == '>' || text[len(prefix)] == '/'
}
func startScript(text string) bool {
	return asciiPrefix(text, "<script") && (len(text) == 7 || space(text[7]) || text[7] == '>' || text[7] == '/')
}
func (s *State) finishTag(id language.ID) {
	s.mode = data
	if id != language.HTML || s.closing {
		return
	}
	switch s.tag {
	case "script":
		s.mode = scriptData
		s.script = 0
	case "style", "xmp", "iframe", "noembed", "noframes":
		s.mode = rawText
	case "title", "textarea":
		s.mode = rcdata
	case "plaintext":
		s.mode = plainText
	}
}

func scanMarkup(id language.ID, s State, input string, limit int, lineEnd bool) ([]Token, State, int) {
	var out []Token
	i := 0
	for i < limit {
		start := i
		b := input[i]
		r, n := utf8.DecodeRuneInString(input[i:])
		switch s.mode {
		case plainText:
			i = limit
		case comment:
			if strings.HasPrefix(input[i:], "-->") {
				i += 3
				s.mode = s.back
				s.back = data
			} else {
				i += n
			}
			emit(&out, start, i, language.KindComment)
		case cdata:
			if strings.HasPrefix(input[i:], "]]>") {
				i += 3
				s.mode = data
				emit(&out, start, i, language.KindMeta)
			} else {
				i += n
			}
		case pi, declaration:
			k := language.KindMeta
			switch {
			case s.quote == 0 && s.mode == declaration && strings.HasPrefix(input[i:], "<!--"):
				i += 4
				s.back, s.mode = declaration, comment
			case s.quote != 0:
				if b == s.quote {
					s.quote = 0
				}
				i += n
			case b == '\'' || b == '"':
				s.quote = b
				i++
			case s.mode == pi && strings.HasPrefix(input[i:], "?>"):
				i += 2
				s.mode = data
			case s.mode == declaration && b == '[':
				s.depth++
				i++
			case s.mode == declaration && b == ']':
				s.depth = max(0, s.depth-1)
				i++
			case s.mode == declaration && b == '>' && s.depth == 0:
				i++
				s.mode = data
			default:
				i += n
			}
			emit(&out, start, i, k)
		case entity:
			if b == ';' {
				i++
				emit(&out, start, i, language.KindEntity)
				s.mode = s.back
			} else if nameRune(r) || b == '#' {
				i += n
				emit(&out, start, i, language.KindEntity)
			} else {
				s.mode = s.back
			}
		case rawText, rcdata, scriptData:
			if s.mode == scriptData && strings.HasPrefix(input[i:], "<!--") && s.script == 0 {
				s.script = 1
				i += 4
			} else if s.mode == scriptData && strings.HasPrefix(input[i:], "-->") && s.script > 0 {
				s.script = 0
				i += 3
			} else if s.mode == scriptData && s.script == 1 && startScript(input[i:]) {
				s.script = 2
				i += 7
			} else if endTag(input[i:], s.tag) {
				if s.mode == scriptData && s.script == 2 {
					s.script = 1
					i += len(s.tag) + 2
				} else {
					s.mode = tagName
					s.closing = true
					s.tag = ""
					i += 2
					emit(&out, start, i, language.KindDelimiter)
				}
			} else if s.mode == rcdata && b == '&' {
				s.back = s.mode
				s.mode = entity
				i++
				emit(&out, start, i, language.KindEntity)
			} else {
				i += n
			}
		case tagName:
			valid := nameRune(r)
			if id == language.HTML {
				valid = !space(b) && b != '>' && b != '/' && b != '=' && b != '<'
			}
			if valid {
				if len(s.tag) < 16 {
					s.tag += strings.ToLower(input[i : i+n])
				}
				i += n
				emit(&out, start, i, language.KindTag)
			} else {
				s.mode = tagInside
			}
		case tagInside:
			switch {
			case space(b):
				i++
			case b == '>':
				i++
				emit(&out, start, i, language.KindDelimiter)
				s.finishTag(id)
			case strings.HasPrefix(input[i:], "/>"):
				i += 2
				emit(&out, start, i, language.KindDelimiter)
				s.mode = data
			case b == '<':
				s.mode = data
			case b == '/':
				i++
				emit(&out, start, i, language.KindInvalid)
			default:
				s.mode = attrName
			}
		case attrName:
			if !space(b) && b != '=' && b != '>' && b != '/' && b != '<' {
				i += n
				emit(&out, start, i, language.KindAttribute)
			} else {
				s.mode = afterAttr
			}
		case afterAttr:
			if space(b) {
				i++
			} else if b == '=' {
				i++
				emit(&out, start, i, language.KindDelimiter)
				s.mode = beforeValue
			} else {
				s.mode = tagInside
			}
		case beforeValue:
			if space(b) {
				i++
			} else if b == '"' || b == '\'' {
				s.mode = attrDouble
				if b == '\'' {
					s.mode = attrSingle
				}
				i++
				emit(&out, start, i, language.KindString)
			} else if b == '>' || b == '<' {
				s.mode = tagInside
			} else {
				s.mode = attrUnquoted
			}
		case attrSingle, attrDouble:
			quote := byte('"')
			if s.mode == attrSingle {
				quote = '\''
			}
			if b == quote {
				i++
				emit(&out, start, i, language.KindString)
				s.mode = tagInside
			} else if b == '&' {
				s.back = s.mode
				s.mode = entity
				i++
				emit(&out, start, i, language.KindEntity)
			} else {
				i += n
				emit(&out, start, i, language.KindString)
			}
		case attrUnquoted:
			if space(b) || b == '>' || b == '<' {
				s.mode = tagInside
			} else if b == '&' {
				s.back = s.mode
				s.mode = entity
				i++
				emit(&out, start, i, language.KindEntity)
			} else {
				i += n
				k := language.KindString
				if id == language.XML {
					k = language.KindInvalid
				}
				emit(&out, start, i, k)
			}
		default:
			switch {
			case strings.HasPrefix(input[i:], "<!--"):
				i += 4
				s.mode = comment
				emit(&out, start, i, language.KindComment)
			case id == language.XML && strings.HasPrefix(input[i:], "<![CDATA["):
				i += 9
				s.mode = cdata
				emit(&out, start, i, language.KindMeta)
			case strings.HasPrefix(input[i:], "<!"):
				i += 2
				s.mode = declaration
				s.depth = 0
				s.quote = 0
				emit(&out, start, i, language.KindMeta)
			case strings.HasPrefix(input[i:], "<?"):
				i += 2
				s.mode = pi
				s.quote = 0
				emit(&out, start, i, language.KindMeta)
			case strings.HasPrefix(input[i:], "</"):
				i += 2
				s.mode = tagName
				s.tag = ""
				s.closing = true
				emit(&out, start, i, language.KindDelimiter)
			case b == '<' && i+1 < len(input):
				next, _ := utf8.DecodeRuneInString(input[i+1:])
				if nameStart(next) {
					i++
					s.mode = tagName
					s.tag = ""
					s.closing = false
					emit(&out, start, i, language.KindDelimiter)
				} else {
					i += n
				}
			case b == '&':
				s.back = data
				s.mode = entity
				i++
				emit(&out, start, i, language.KindEntity)
			default:
				i += n
			}
		}
	}
	if lineEnd && i == len(input) {
		switch s.mode {
		case tagName:
			s.mode = tagInside
		case attrName:
			s.mode = afterAttr
		case attrUnquoted:
			s.mode = tagInside
		case entity:
			s.mode = s.back
		}
	}
	if s.mode == data {
		s = State{}
	}
	if s.mode != entity && s.mode != comment {
		s.back = data
	}
	return out, s, i
}
