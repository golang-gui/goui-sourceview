package language

import (
	"unicode/utf8"
)

// appendToken also applies a late classification of a previously scanned span.
// Revisions stay inside the current line and end at the current scan position.
func appendToken(out *[]lexicalToken, start, end int, kind Kind) {
	for len(*out) > 0 && (*out)[len(*out)-1].End > start {
		last := &(*out)[len(*out)-1]
		if last.Start < start {
			last.End = start
			break
		}
		*out = (*out)[:len(*out)-1]
	}
	emit(out, start, end, kind)
}

// atom retains bounded spelling for literals and a streaming numeric recognizer.
// Its byte length allows classification after arbitrarily many input fragments.
type atom struct {
	significant int
	gap         bool
	// After a bounded prefix, retain a possible date timezone, skipping only
	// the still-unbounded fractional digits before it.
	tail                                     string
	tailBad                                  bool
	n                                        int
	text                                     string
	number                                   uint8
	base                                     byte
	bad, separator, leadingZero, underscores bool
}

func (a *atom) add(c byte) {
	a.n++
	if !space(c) {
		a.significant = a.n
	}
	if len(a.text) < 128 {
		a.text += string(c)
	} else if a.tail != "" || !digit(c) {
		if len(a.tail) < 8 {
			a.tail += string(c)
		} else {
			a.tailBad = true
		}
	}
	if a.bad {
		return
	}
	if space(c) {
		a.gap = true
		return
	}
	if a.gap {
		a.bad = true
		return
	}
	if c == '_' {
		a.underscores = true
		if a.separator || !(a.number == 2 || a.number == 3 || a.number == 5 || a.number == 8 || a.number == 10) {
			a.bad = true
		}
		a.separator = true
		return
	}
	if a.separator && !digit(c) && !(a.number == 10 && a.base == 16 && hex(c)) {
		a.bad = true
		return
	}
	a.separator = false
	switch a.number {
	case 0:
		if c == '+' || c == '-' {
			a.number = 1
		} else {
			a.integerStart(c)
		}
	case 1:
		a.integerStart(c)
	case 2, 3:
		switch {
		case digit(c):
			if a.number == 2 {
				a.leadingZero = true
			}
			a.number = 3
		case c == '.':
			a.number = 4
		case c == 'e' || c == 'E':
			a.number = 6
		case a.n == 2 && a.number == 2 && (c == 'x' || c == 'o' || c == 'b'):
			a.number = 9
			switch c {
			case 'x':
				a.base = 16
			case 'o':
				a.base = 8
			case 'b':
				a.base = 2
			}
		default:
			a.bad = true
		}
	case 4:
		if digit(c) {
			a.number = 5
		} else {
			a.bad = true
		}
	case 5:
		if c == 'e' || c == 'E' {
			a.number = 6
		} else if !digit(c) {
			a.bad = true
		}
	case 6:
		if c == '+' || c == '-' {
			a.number = 7
		} else if digit(c) {
			a.number = 8
		} else {
			a.bad = true
		}
	case 7:
		if digit(c) {
			a.number = 8
		} else {
			a.bad = true
		}
	case 8:
		if !digit(c) {
			a.bad = true
		}
	case 9, 10:
		if a.base == 16 && hex(c) || a.base == 8 && c >= '0' && c <= '7' || a.base == 2 && (c == '0' || c == '1') {
			a.number = 10
		} else {
			a.bad = true
		}
	}
}
func (a *atom) integerStart(c byte) {
	if c == '0' {
		a.number = 2
	} else if digit(c) {
		a.number = 3
	} else {
		a.bad = true
	}
}
func (a atom) numeric() bool {
	return !a.bad && !a.separator && (a.number == 2 || a.number == 3 || a.number == 5 || a.number == 8 || a.number == 10)
}

func runeSize(s string, i int) int { _, n := utf8.DecodeRuneInString(s[i:]); return n }
