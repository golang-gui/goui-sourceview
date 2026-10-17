package editor

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/golang-gui/goui/gui/textedit"
)

type Query struct {
	Text                  string
	IgnoreCase, WholeWord bool
}

// Search compiles a literal Unicode query and retains original byte coordinates.
type Search struct {
	query   Query
	pattern []rune
	failure []int
}

func Compile(q Query) *Search {
	q.Text = textedit.Normalize(q.Text)
	s := &Search{query: q, pattern: []rune(q.Text)}
	for i, r := range s.pattern {
		if q.IgnoreCase {
			s.pattern[i] = fold(r)
		}
	}
	s.failure = make([]int, len(s.pattern))
	for i, j := 1, 0; i < len(s.pattern); i++ {
		for j > 0 && s.pattern[i] != s.pattern[j] {
			j = s.failure[j-1]
		}
		if s.pattern[i] == s.pattern[j] {
			j++
		}
		s.failure[i] = j
	}
	return s
}
func (s *Search) Empty() bool { return s == nil || len(s.pattern) == 0 }

// CountChunk counts non-overlapping match starts in one source chunk, reading a
// pattern-sized suffix. matchedEnd carries the last accepted range across chunks;
// callers start it at zero and retain the returned value, not a match list.
func (s *Search) CountChunk(m *textedit.Model, start, budget, matchedEnd int) (next, count, after int) {
	next = min(m.Len(), start+max(4, budget))
	for next < m.Len() && !m.IsRuneBoundary(next) {
		next++
	}
	after = matchedEnd
	if s.Empty() || after >= next {
		return next, 0, after
	}
	end := min(m.Len(), next+len(s.pattern)*utf8.UTFMax)
	s.scan(m, textedit.Range{Start: max(start, after), End: end}, func(r textedit.Range) bool {
		if r.Start >= next {
			return false
		}
		if r.Start >= after {
			count++
			after = r.End
		}
		return true
	})
	return next, count, after
}
func fold(r rune) rune {
	lowest := r
	for next := unicode.SimpleFold(r); next != r; next = unicode.SimpleFold(next) {
		if next < lowest {
			lowest = next
		}
	}
	return lowest
}
func word(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsMark(r) || unicode.IsNumber(r)
}

type runePosition struct {
	start, end int
	beforeWord bool
}

// scan visits overlapping matches in a bounded source span. whole-word context
// is read outside the span when necessary. No full document string is built.
func (s *Search) scan(m *textedit.Model, span textedit.Range, visit func(textedit.Range) bool) error {
	if s.Empty() {
		return nil
	}
	start, end := max(0, span.Start), min(m.Len(), span.End)
	for start > 0 && !m.IsRuneBoundary(start) {
		start--
	}
	for end < m.Len() && !m.IsRuneBoundary(end) {
		end++
	}
	ring := make([]runePosition, len(s.pattern))
	seen, matched := 0, 0
	previousWord := false
	if start > 0 {
		before := start - 1
		for before > 0 && !m.IsRuneBoundary(before) {
			before--
		}
		t, _ := m.Slice(textedit.Range{Start: before, End: start})
		r, _ := utf8.DecodeRuneInString(t)
		previousWord = word(r)
	}
	var pending *textedit.Range
	pendingLastWord := false
	for at := start; at < end; {
		next := min(end, at+16*1024)
		for next > at && !m.IsRuneBoundary(next) {
			next--
		}
		text, err := m.Slice(textedit.Range{Start: at, End: next})
		if err != nil {
			return err
		}
		for local, r := range text {
			currentWord := word(r)
			if pending != nil {
				if pendingLastWord != currentWord && !visit(*pending) {
					return nil
				}
				pending = nil
			}
			if s.query.IgnoreCase {
				r = fold(r)
			}
			ring[seen%len(ring)] = runePosition{at + local, at + local + utf8.RuneLen(runeAt(text, local)), previousWord}
			seen++
			for matched > 0 && r != s.pattern[matched] {
				matched = s.failure[matched-1]
			}
			if r == s.pattern[matched] {
				matched++
			}
			if matched == len(s.pattern) {
				first := ring[(seen-len(s.pattern))%len(ring)]
				match := textedit.Range{Start: first.start, End: at + local + utf8.RuneLen(runeAt(text, local))}
				if !s.query.WholeWord {
					if !visit(match) {
						return nil
					}
				} else {
					firstWord := word(runeAtModel(m, match.Start))
					if match.Start == 0 || first.beforeWord != firstWord {
						copy := match
						pending = &copy
						pendingLastWord = currentWord
					}
				}
				matched = s.failure[matched-1]
			}
			previousWord = currentWord
		}
		at = next
	}
	if pending != nil {
		afterWord := false
		if end < m.Len() {
			afterWord = word(runeAtModel(m, end))
		}
		if end == m.Len() || pendingLastWord != afterWord {
			visit(*pending)
		}
	}
	return nil
}
func runeAt(text string, at int) rune { r, _ := utf8.DecodeRuneInString(text[at:]); return r }
func runeAtModel(m *textedit.Model, at int) rune {
	end := min(m.Len(), at+utf8.UTFMax)
	for end < m.Len() && !m.IsRuneBoundary(end) {
		end--
	}
	text, _ := m.Slice(textedit.Range{Start: at, End: end})
	return runeAt(text, 0)
}

// Find locates a match outside the current one, wrapping at most once.
func (s *Search) Find(m *textedit.Model, start int, backward bool) (textedit.Range, bool) {
	var first, last, best textedit.Range
	have, found := false, false
	matchedEnd := 0
	s.scan(m, textedit.Range{Start: 0, End: m.Len()}, func(r textedit.Range) bool {
		if r.Start < matchedEnd {
			return true
		}
		matchedEnd = r.End
		if !have {
			first = r
			have = true
		}
		last = r
		if backward {
			if r.End <= start {
				best = r
				found = true
			}
			return true
		}
		if r.Start >= start {
			best = r
			found = true
			return false
		}
		return true
	})
	if found {
		return best, true
	}
	if have {
		if backward {
			return last, true
		}
		return first, true
	}
	return textedit.Range{}, false
}

// InRange returns matches intersecting a visible span, with bounded decorations.
func (s *Search) InRange(m *textedit.Model, span textedit.Range, limit int) []textedit.Range {
	if s.Empty() || limit <= 0 {
		return nil
	}
	context := len(s.pattern) * utf8.UTFMax
	source := textedit.Range{Start: max(0, span.Start-context), End: min(m.Len(), span.End+context)}
	singleLine := !strings.ContainsRune(s.query.Text, '\n')
	if singleLine {
		// A single-line pattern cannot overlap a preceding logical line. The
		// painter already supplies whole paragraphs; this is also a stable
		// anchor if a caller requests only part of one line.
		line, _ := m.LineRange(m.LineAt(span.Start))
		source.Start = line.Start
	}
	var matches []textedit.Range
	matchedEnd := 0
	s.scan(m, source, func(r textedit.Range) bool {
		if singleLine && r.Start < matchedEnd {
			return true
		}
		matchedEnd = r.End
		if r.End > span.Start && r.Start < span.End {
			matches = append(matches, r)
		}
		return len(matches) < limit
	})
	return matches
}
func (s *Search) Matches(m *textedit.Model, r textedit.Range) bool {
	if s.Empty() {
		return false
	}
	found := false
	s.scan(m, r, func(actual textedit.Range) bool {
		if actual == r {
			found = true
		}
		return true
	})
	return found
}

// ReplaceAll plans non-overlapping replacements against one original revision.
func (s *Search) ReplaceAll(m *textedit.Model, selection textedit.Selection, replacement string) (Plan, error) {
	p := Plan{Before: selection, After: selection}
	text := textedit.Normalize(replacement)
	lastEnd, total := 0, 0
	budget := false
	err := s.scan(m, textedit.Range{Start: 0, End: m.Len()}, func(r textedit.Range) bool {
		if r.Start < lastEnd {
			return true
		}
		if len(p.Edits) >= 100000 || len(text) > 32*1024*1024-total {
			budget = true
			return false
		}
		p.Edits = append(p.Edits, textedit.Edit{Range: r, Text: text})
		lastEnd = r.End
		total += len(text)
		return true
	})
	if err != nil {
		return Plan{}, err
	}
	if budget {
		return Plan{}, ErrOperationLimit
	}
	p.After = mapSelection(selection, p.Edits)
	return p, nil
}
