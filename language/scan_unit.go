package language

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

type unitTokenizer struct{ id ID }
type unitState struct {
	id                                                  ID
	phase                                               byte
	value, continued, comment, locale, escape, variable bool
	quote, last                                         byte
	key                                                 string
	word                                                scanWord
	wordOn                                              bool
	escapeLeft                                          int
	octal                                               bool
}

func (s unitState) Equal(other State) bool { t, ok := other.(unitState); return ok && s == t }
func (s unitState) Context() Context {
	return Context{Comment: s.comment, String: s.value && !s.comment, Value: s.value, Data: !s.value && !s.comment}
}
func (b unitTokenizer) InitialState() State { return unitState{id: b.id} }
func unitWords(id ID) []string {

	return strings.Fields("Unit Service Install Socket Timer Path Mount Automount Swap Target Slice Scope Description Documentation After Before Wants Requires Type ExecStart ExecStop ExecReload WorkingDirectory Environment EnvironmentFile User Group Restart RestartSec WantedBy RequiredBy OnCalendar OnBootSec OnUnitActiveSec Persistent")

}
func (b unitTokenizer) Scan(in Input, state State) (Result, error) {
	s, ok := state.(unitState)
	if !ok || s.id != b.id {
		return Result{}, fmt.Errorf("language: foreign unit state")
	}
	limit, err := scanLimit(in)
	if err != nil {
		return Result{}, err
	}
	w := tokenWriter{base: in.Offset}
	text, i := in.Text, 0
	add := func(a, z int, k Kind, c Class) { w.add(in.Offset+a, in.Offset+z, k, c) }
	finish := func(end int) {
		if !s.wordOn {
			return
		}
		k := KindString
		if s.word.text == "true" || s.word.text == "false" || wordsBoolean[s.word.text] {
			k = KindKeyword
		} else if s.word.text != "" && digit(s.word.text[0]) {
			k = KindNumber
		}
		w.add(s.word.start, end, k, ClassString)
		s.word, s.wordOn = scanWord{}, false
	}
	for i < limit {
		c := text[i]
		r, n := utf8.DecodeRuneInString(text[i:])
		if s.comment {
			add(i, i+n, KindComment, ClassComment)
			i += n
			continue
		}
		if s.phase == 0 {
			if space(c) {
				i += n
				continue
			}
			if c == '#' || c == ';' {
				s.comment = true
				continue
			}
			if s.continued {
				s.phase = 2
				s.value = true
			} else if c == '[' {
				s.phase = 3
				add(i, i+1, KindDelimiter, 0)
				i++
				continue
			} else {
				s.phase = 1
				s.key = ""
			}
		}
		if s.phase == 3 {
			if c == ']' {
				add(i, i+1, KindDelimiter, 0)
				s.phase = 4
			} else {
				add(i, i+n, KindMeta, 0)
			}
			i += n
			continue
		}
		if s.phase == 4 {
			i += n
			continue
		}
		if s.phase == 1 {
			if c == '=' && !s.locale {
				s.phase = 2
				s.value = true
				add(i, i+1, KindDelimiter, 0)
				i++
				continue
			}
			if c == '[' {
				s.locale = true
			} else if c == ']' {
				s.locale = false
			}
			if !space(c) {
				add(i, i+n, KindProperty, 0)
				if !s.locale && c != ']' && len(s.key) < 128 {
					s.key += text[i : i+n]
				}
			}
			i += n
			continue
		}
		if s.wordOn {
			if IdentifierRune(r) || c == '.' || c == '-' {
				s.word.add(text[i : i+n])
				s.last = c
				i += n
				continue
			}
			finish(in.Offset + i)
			continue
		}
		if c == '\r' && i+1 == len(text) {
			i++
			continue
		}
		if s.variable {
			if IdentifierRune(r) || c == '}' {
				add(i, i+n, KindVariable, ClassString)
				i += n
				if c == '}' {
					s.variable = false
				}
				s.last = c
				continue
			}
			s.variable = false
		}
		if s.escape {
			k := KindEscape
			allowed := "sntr\\;"

			allowed = "abfnrtv\\\"'sxuU01234567"

			if !strings.ContainsRune(allowed, r) {
				k = KindInvalid
			}
			add(i, i+n, k, ClassString)

			switch c {
			case 'x':
				s.escapeLeft = 2
			case 'u':
				s.escapeLeft = 4
			case 'U':
				s.escapeLeft = 8
			case '0', '1', '2', '3', '4', '5', '6', '7':
				s.escapeLeft, s.octal = 2, true
			}

			s.escape = false
			s.last = c
			i += n
			continue
		}
		if s.escapeLeft > 0 {
			valid := hex(c)
			if s.octal {
				valid = c >= '0' && c <= '7'
			}
			k := KindEscape
			if !valid {
				k = KindInvalid
			}
			add(i, i+n, k, ClassString)
			i += n
			s.escapeLeft--
			if s.escapeLeft == 0 {
				s.octal = false
			}
			s.last = c
			continue
		}
		if c == '\\' {
			add(i, i+1, KindEscape, ClassString)
			s.escape = true
			s.last = c
			i++
			continue
		}
		if c == '%' && i+1 < len(text) {
			k := KindVariable
			if text[i+1] == '%' {
				k = KindEscape
			}
			z := i + 1 + runeSize(text, i+1)
			add(i, z, k, ClassString)
			s.last = text[z-1]
			i = z
			continue
		}
		if c == '$' && strings.HasPrefix(s.key, "Exec") {
			z := i + 1
			if z < len(text) && text[z] == '$' {
				add(i, z+1, KindEscape, ClassString)
				i = z + 1
				continue
			}
			if z < len(text) && text[z] == '{' {
				z++
				s.variable = true
			} else {
				s.variable = true
			}
			add(i, z, KindVariable, ClassString)
			i = z
			s.last = c
			continue
		}

		if s.quote != 0 {
			if c == s.quote {
				s.quote = 0
			}
		} else if c == '"' || c == '\'' {
			s.quote = c
		}

		if IdentifierRune(r) {
			s.word = scanWord{start: in.Offset + i}
			s.wordOn = true
			s.word.add(text[i : i+n])
			s.last = c
			i += n
			continue
		}
		k := KindString

		add(i, i+n, k, ClassString)
		s.last = c
		i += n
	}
	if in.LineEnd && i == len(text) {
		finish(in.Offset + i)
		if !s.comment {
			s.continued = s.value && s.last == '\\'
			if !s.continued {
				s.value = false
				s.key = ""
				s.quote = 0
			}
		}
		s.phase, s.comment, s.locale, s.escape, s.variable, s.last = 0, false, false, false, false, 0
		s.escapeLeft, s.octal = 0, false
	}
	return w.result(i, s), nil
}

var wordsBoolean = words("yes no on off")
