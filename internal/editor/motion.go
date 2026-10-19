package editor

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/golang-gui/goui/gui/textedit"
)

// Code words use Monaco's default ASCII separators. Other non-whitespace
// Unicode characters stay together; the view rounds destinations to Clusters.
const wordSeparators = "`~!@#$%^&*()-=+[{]}\\|;:'\",.<>/?"

func wordClass(r rune) int {
	if unicode.IsSpace(r) {
		return 0
	}
	if strings.ContainsRune(wordSeparators, r) {
		return 1
	}
	return 2
}

// wordCursor bounds transient copies even on a very long logical line.
type wordCursor struct {
	m              *textedit.Model
	at, start, end int
	text           string
	word           func(rune) bool
}

func (c *wordCursor) peek(forward bool) (rune, int) {
	if forward && c.at == c.m.Len() || !forward && c.at == 0 {
		return 0, 0
	}
	if forward && (c.at < c.start || c.at >= c.end) || !forward && (c.at <= c.start || c.at > c.end) || c.text == "" {
		c.start, c.end = c.at, c.at
		if forward {
			c.end = min(c.m.Len(), c.at+4096)
			for !c.m.IsRuneBoundary(c.end) {
				c.end--
			}
		} else {
			c.start = max(0, c.at-4096)
			for !c.m.IsRuneBoundary(c.start) {
				c.start++
			}
		}
		c.text, _ = c.m.Slice(textedit.Range{Start: c.start, End: c.end})
	}
	if forward {
		return utf8.DecodeRuneInString(c.text[c.at-c.start:])
	}
	return utf8.DecodeLastRuneInString(c.text[:c.at-c.start])
}

func (c *wordCursor) class(r rune) int {
	if c.word == nil {
		return wordClass(r)
	}
	if unicode.IsSpace(r) {
		return 0
	}
	if c.word(r) {
		return 2
	}
	return 1
}

// WordRange selects a language word, whitespace run, or single punctuation rune.
func WordRange(m *textedit.Model, at int, word func(rune) bool) textedit.Range {
	c := wordCursor{m: m, at: at, word: word}
	r, n := c.peek(true)
	if n == 0 {
		return textedit.Range{Start: at, End: at}
	}
	if c.class(r) == 1 || r == '\n' {
		return textedit.Range{Start: at, End: at + n}
	}
	class := c.class(r)
	c.consume(false, class)
	start := c.at
	c.at = at
	c.consume(true, class)
	return textedit.Range{Start: start, End: c.at}
}

func (c *wordCursor) consume(forward bool, class int) int {
	count := 0
	for {
		r, size := c.peek(forward)
		if size == 0 || r == '\n' || c.class(r) != class {
			return count
		}
		if forward {
			c.at += size
		} else {
			c.at -= size
		}
		count++
	}
}

// WordBoundary moves to a code-word start on the left or end on the right.
// Navigation crosses at most one LF and skips adjacent whitespace. Deletion
// stops after a whitespace run of two or more, and treats a line join separately.
func WordBoundary(m *textedit.Model, at int, forward, deleting bool) int {
	return WordBoundaryWith(m, at, forward, deleting, nil)
}

// WordBoundaryWith uses the language word predicate; nil preserves default separators.
func WordBoundaryWith(m *textedit.Model, at int, forward, deleting bool, word func(rune) bool) int {
	c := wordCursor{m: m, at: at, word: word}
	r, n := c.peek(forward)
	if n == 0 {
		return at
	}
	if r == '\n' {
		if forward {
			c.at++
		} else {
			c.at--
		}
		if deleting {
			if forward {
				c.consume(true, 0)
			}
			return c.at
		}
	}
	spaces := c.consume(forward, 0)
	if deleting && spaces > 1 {
		return c.at
	}
	r, n = c.peek(forward)
	if n == 0 || r == '\n' {
		return c.at
	}
	class := c.class(r)
	wordEnd := c.at
	count := c.consume(forward, class)
	if !deleting && class == 1 && count == 1 {
		// Fast navigation over one separator adjoining the next word.
		if forward {
			if next, size := c.peek(true); size > 0 && c.class(next) == 2 {
				c.consume(true, 2)
			}
		} else if wordEnd < m.Len() && c.class(runeAtModel(m, wordEnd)) == 2 {
			c.consume(false, 0)
			if prev, size := c.peek(false); size > 0 && prev != '\n' {
				c.consume(false, c.class(prev))
			}
		}
	}
	return c.at
}

// InsertLine preserves the caret's logical line and inserts an indented blank
// line beside it. It does not replace a selection or split the current line.
func InsertLine(m *textedit.Model, s textedit.Selection, below bool) (Plan, error) {
	if _, _, err := SelectedLines(m, s); err != nil {
		return Plan{}, err
	}
	r, _ := m.LineRange(m.LineAt(s.Caret))
	c := wordCursor{m: m, at: r.Start}
	var indent strings.Builder
	for c.at < r.End {
		ch, size := c.peek(true)
		if ch != ' ' && ch != '\t' {
			break
		}
		if indent.Len() >= 32*1024*1024-1 {
			return Plan{}, ErrOperationLimit
		}
		indent.WriteByte(byte(ch))
		c.at += size
	}
	at, text := r.Start, indent.String()+"\n"
	caret := at + indent.Len()
	if below {
		at, text = r.End, "\n"+indent.String()
		caret = at + len(text)
	}
	return Plan{Before: s, After: textedit.Selection{Anchor: caret, Caret: caret}, Edits: []textedit.Edit{{Range: textedit.Range{Start: at, End: at}, Text: text}}}, nil
}
