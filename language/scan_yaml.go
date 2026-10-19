package language

import (
	"regexp"
	"strings"
)

type yamlState struct {
	offset, scalarColumn, keyIndent int
	quote                           byte
	escape                          bool
	escapeLeft                      int
	comment                         bool
	// indent is counted once across fragments; blockIndent is zero until detected.
	lineStart                      bool
	indent, blockBase, blockIndent int
	block, blockHeader             bool
	flow                           int
	scalar                         atom
	scalarOn                       bool
	previous                       byte
	meta                           bool
}

func equalYAML(a, b *yamlState) bool {
	if a == b {
		return true
	}
	return a != nil && b != nil && *a == *b
}

var yamlNumber = regexp.MustCompile(`^[+-]?(?:[0-9]+|0o[0-7]+|0x[0-9a-fA-F]+|(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][+-]?[0-9]+)?|\.(?:inf|Inf|INF))$|^\.(?:nan|NaN|NAN)$`)

func (a atom) yamlKind() Kind {
	text := strings.TrimSpace(a.text)
	if a.significant <= 128 {
		switch text {
		case "true", "True", "TRUE", "false", "False", "FALSE", "null", "Null", "NULL", "~":
			return KindKeyword
		}
		if yamlNumber.MatchString(text) {
			return KindNumber
		}
	} else if a.numeric() && !a.underscores {
		return KindNumber
	}
	return KindPlain
}
func yamlSep(s string, i int) bool {
	return i >= len(s) || space(s[i]) || strings.ContainsRune(",[]{}", rune(s[i]))
}
func scanYAML(state builtinState, input string, limit int, lineEnd bool) ([]lexicalToken, builtinState, int) {
	s := yamlState{lineStart: true, keyIndent: -1}
	if state.yaml != nil {
		s = *state.yaml
	}
	var out []lexicalToken
	i := 0
	finish := func(key bool) {
		if s.scalarOn {
			kind := s.scalar.yamlKind()
			if key {
				kind = KindProperty
				s.keyIndent = s.scalarColumn
			}
			emit(&out, i-s.scalar.n, i, kind)
			s.scalar = atom{}
			s.scalarOn = false
		}
	}
	for i < limit {
		c := input[i]
		if s.lineStart {
			if c == ' ' {
				s.indent++
				i++
				continue
			}
			s.lineStart = false
			if s.block {
				if s.blockIndent == 0 && c != '\r' && s.indent > s.blockBase {
					s.blockIndent = s.indent
				}
				if !(s.indent > s.blockBase && (s.blockIndent == 0 || s.indent >= s.blockIndent)) {
					s.block = false
					s.blockIndent = 0
				}
			}
		}
		if s.block && !s.blockHeader {
			n := limit
			emit(&out, i, n, KindString)
			i = n
			continue
		}
		if s.comment {
			emit(&out, i, limit, KindComment)
			i = limit
			continue
		}
		if s.quote != 0 {
			if s.escapeLeft > 0 {
				if c == s.quote {
					s.escapeLeft = 0
					continue
				}
				n := runeSize(input, i)
				kind := KindEscape
				if !hex(c) {
					kind = KindInvalid
					s.escapeLeft = 0
				} else {
					s.escapeLeft--
				}
				emit(&out, i, i+n, kind)
				i += n
				continue
			}
			if s.escape {
				s.escape = false
				k := KindEscape
				switch c {
				case 'x':
					s.escapeLeft = 2
				case 'u':
					s.escapeLeft = 4
				case 'U':
					s.escapeLeft = 8
				default:
					if !strings.ContainsRune("0abtnvfre \"/\\N_LP\t\r", rune(c)) {
						k = KindInvalid
					}
				}
				n := runeSize(input, i)
				emit(&out, i, i+n, k)
				i += n
				continue
			}
			if c == '\\' && s.quote == '"' {
				emit(&out, i, i+1, KindEscape)
				s.escape = true
				i++
				continue
			}
			if c == s.quote {
				if c == '\'' && i+1 < len(input) && input[i+1] == '\'' {
					emit(&out, i, i+2, KindEscape)
					i += 2
					continue
				}
				emit(&out, i, i+1, KindString)
				s.quote = 0
				i++
				s.previous = c
				continue
			}
			n := runeSize(input, i)
			emit(&out, i, i+n, KindString)
			i += n
			continue
		}
		if s.meta {
			if !space(c) && !strings.ContainsRune(",[]{}", rune(c)) {
				n := runeSize(input, i)
				emit(&out, i, i+n, KindMeta)
				i += n
				continue
			}
			s.meta = false
		}
		if s.scalarOn {
			colon := c == ':' && yamlSep(input, i+1)
			comment := c == '#' && space(s.previous)
			if colon || comment || s.flow > 0 && strings.ContainsRune(",[]{}", rune(c)) {
				finish(colon)
				continue
			}
			s.scalar.add(c)
			s.previous = c
			i++
			continue
		}
		if space(c) {
			s.previous = c
			i++
			continue
		}
		if c == '#' && (s.offset+i == 0 || space(s.previous)) {
			s.comment = true
			continue
		}
		if c == '\'' || c == '"' {
			s.quote = c
			emit(&out, i, i+1, KindString)
			i++
			continue
		}
		if c == '&' || c == '*' || c == '!' || c == '%' && s.indent == 0 && s.offset+i == 0 {
			s.meta = true
			emit(&out, i, i+1, KindMeta)
			i++
			continue
		}
		if (c == '|' || c == '>') && s.flow == 0 {
			s.block = true
			s.blockHeader = true
			s.blockBase = s.indent
			if s.keyIndent >= 0 {
				s.blockBase = s.keyIndent
			}
			s.blockIndent = 0
			n := 1
			for n < 3 && i+n < len(input) && (input[i+n] == '+' || input[i+n] == '-' || input[i+n] >= '1' && input[i+n] <= '9') {
				if digit(input[i+n]) {
					s.blockIndent = s.blockBase + int(input[i+n]-'0')
				}
				n++
			}
			emit(&out, i, i+n, KindMeta)
			i += n
			continue
		}
		if strings.ContainsRune("[]{},", rune(c)) || strings.ContainsRune(":?-", rune(c)) && yamlSep(input, i+1) {
			emit(&out, i, i+1, KindDelimiter)
			if c == '[' || c == '{' {
				s.flow++
			}
			if (c == ']' || c == '}') && s.flow > 0 {
				s.flow--
			}
			s.previous = c
			i++
			continue
		}
		if (strings.HasPrefix(input[i:], "---") || strings.HasPrefix(input[i:], "...")) && s.indent == 0 && yamlSep(input, i+3) {
			emit(&out, i, i+3, KindMeta)
			i += 3
			continue
		}
		s.scalarOn = true
		s.scalarColumn = s.offset + i
		s.scalar = atom{}
	}
	s.offset += i
	if lineEnd && i == len(input) {
		s.offset = 0
		s.scalarColumn = 0
		s.keyIndent = -1
		finish(false)
		s.comment = false
		s.meta = false
		s.escape = false
		s.escapeLeft = 0
		s.lineStart = true
		s.indent = 0
		s.previous = 0
		s.blockHeader = false
	}
	state.yaml = &s
	return out, state, i
}
