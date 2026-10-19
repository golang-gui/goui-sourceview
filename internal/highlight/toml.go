package highlight

import (
	"github.com/golang-gui/goui-sourceview/language"
	"regexp"
	"strings"
)

var tomlNumber = regexp.MustCompile(`^[+-]?(?:inf|nan|(?:0|[1-9](?:_?[0-9])*)(?:\.[0-9](?:_?[0-9])*)?(?:[eE][+-]?[0-9](?:_?[0-9])*)?)$|^0(?:x[0-9a-fA-F](?:_?[0-9a-fA-F])*|o[0-7](?:_?[0-7])*|b[01](?:_?[01])*)$`)
var tomlDate = regexp.MustCompile(`^(?:[0-9]{4}-[0-9]{2}-[0-9]{2}(?:[Tt ][0-9]{2}:[0-9]{2}(?::[0-9]{2}(?:\.[0-9]+)?)?(?:[Zz]|[+-][0-9]{2}:[0-9]{2})?)?|[0-9]{2}:[0-9]{2}(?::[0-9]{2}(?:\.[0-9]+)?)?)$`)

func (a atom) tomlKind() language.Kind {
	if a.n <= 128 {
		if a.text == "true" || a.text == "false" {
			return language.KindKeyword
		}
		if tomlNumber.MatchString(a.text) {
			return language.KindNumber
		}
		if tomlDate.MatchString(a.text) {
			return language.KindMeta
		}
	} else {
		if a.numeric() && !a.leadingZero {
			return language.KindNumber
		}
		if !a.tailBad && strings.Contains(a.text, ".") && tomlDate.MatchString(a.text+a.tail) {
			return language.KindMeta
		}
	}
	return language.KindInvalid
}

type tomlFrame struct {
	parent *tomlFrame
	table  bool
}
type tomlState struct {
	quote                                   byte
	triple, key, stringKey, comment, escape bool
	escapeLeft                              int
	value                                   atom
	valueOn                                 bool
	frames                                  *tomlFrame
	table                                   bool
}

func equalTOML(a, b *tomlState) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	x, y := *a, *b
	af, bf := x.frames, y.frames
	x.frames, y.frames = nil, nil
	if x != y {
		return false
	}
	for af != bf {
		if af == nil || bf == nil || af.table != bf.table {
			return false
		}
		af, bf = af.parent, bf.parent
	}
	return true
}
func scanTOML(state State, input string, limit int, lineEnd bool) ([]Token, State, int) {
	s := tomlState{key: true}
	if state.toml != nil {
		s = *state.toml
	}
	var out []Token
	i := 0
	finish := func() {
		if s.valueOn {
			emit(&out, i-s.value.n, i, s.value.tomlKind())
			s.value = atom{}
			s.valueOn = false
		}
	}
	for i < limit {
		c := input[i]
		if s.comment {
			n := min(limit, len(input))
			emit(&out, i, n, language.KindComment)
			i = n
			continue
		}
		if s.quote != 0 {
			kind := language.KindString
			if s.stringKey {
				kind = language.KindProperty
			}
			if s.escapeLeft > 0 {
				if c == s.quote {
					s.escapeLeft = 0
					continue
				}
				n := runeSize(input, i)
				kind := language.KindEscape
				if !hex(c) {
					kind = language.KindInvalid
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
				k := language.KindEscape
				switch c {
				case 'u':
					s.escapeLeft = 4
				case 'U':
					s.escapeLeft = 8
				case 'x':
					s.escapeLeft = 2
				default:
					if !strings.ContainsRune("befnrt\"\\", rune(c)) && !(s.triple && space(c)) {
						k = language.KindInvalid
					}
				}
				n := runeSize(input, i)
				emit(&out, i, i+n, k)
				i += n
				continue
			}
			if c == '\\' && s.quote == '"' {
				emit(&out, i, i+1, language.KindEscape)
				s.escape = true
				i++
				continue
			}
			if c == s.quote {
				if !s.triple {
					emit(&out, i, i+1, kind)
					s.quote = 0
					i++
					continue
				}
				if strings.HasPrefix(input[i:], strings.Repeat(string(c), 3)) {
					n := 3
					for n < 5 && i+n < len(input) && input[i+n] == c {
						n++
					}
					emit(&out, i, i+n, kind)
					i += n
					s.quote = 0
					s.triple = false
					continue
				}
			}
			n := runeSize(input, i)
			emit(&out, i, i+n, kind)
			i += n
			continue
		}
		if s.valueOn {
			// A date/time may contain one separating space.
			dateSpace := c == ' ' && s.value.n == 10 && len(input[i:]) >= 3 && digit(input[i+1]) && digit(input[i+2])
			if !space(c) && !strings.ContainsRune(",]}#", rune(c)) || dateSpace {
				s.value.add(c)
				i++
				continue
			}
			finish()
			continue
		}
		if space(c) {
			i++
			continue
		}
		if c == '#' {
			s.comment = true
			continue
		}
		if c == '\'' || c == '"' {
			s.quote = c
			s.stringKey = s.key
			n := 1
			s.triple = !s.key && strings.HasPrefix(input[i:], strings.Repeat(string(c), 3))
			if s.triple {
				n = 3
			}
			kind := language.KindString
			if s.key {
				kind = language.KindProperty
			}
			emit(&out, i, i+n, kind)
			i += n
			continue
		}
		if strings.ContainsRune("[]=,.{}", rune(c)) {
			emit(&out, i, i+1, language.KindDelimiter)
			i++
			switch c {
			case '=':
				s.key = false
			case '[':
				if s.key && s.frames == nil {
					s.table = true
				} else {
					s.frames = &tomlFrame{parent: s.frames}
					s.key = false
				}
			case '{':
				s.frames = &tomlFrame{parent: s.frames, table: true}
				s.key = true
			case ']', '}':
				if !s.table && s.frames != nil {
					s.frames = s.frames.parent
				}
				s.key = false
			case ',':
				s.key = s.frames != nil && s.frames.table
			}
			continue
		}
		if s.key {
			n := runeSize(input, i)
			kind := language.KindProperty
			if !(alpha(c) || digit(c) || c == '_' || c == '-') {
				kind = language.KindInvalid
			}
			emit(&out, i, i+n, kind)
			i += n
			continue
		}
		s.valueOn = true
		s.value = atom{}
	}
	if lineEnd && i == len(input) {
		finish()
		s.comment = false
		s.table = false
		if !s.triple {
			s.quote = 0
			s.escape = false
			s.escapeLeft = 0
		}
		// Escaped LF in multiline basic strings trims following whitespace.
		if s.triple && s.escape {
			s.escape = false
		}
		if s.frames == nil {
			s.key = true
		}
	}
	state.toml = &s
	return out, state, i
}
