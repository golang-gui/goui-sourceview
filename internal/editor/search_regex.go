package editor

import (
	"fmt"
	"github.com/golang-gui/goui/gui/textedit"
	"strings"
	"unicode/utf8"
)

func (s *Search) Err() error {
	if s == nil {
		return nil
	}
	return s.err
}
func (s *Search) SetRange(r *textedit.Range) {
	s.scope = nil
	if r != nil {
		copy := *r
		s.scope = &copy
	}
	s.cachedModel = nil
	s.cachedText = ""
	s.cachedMatches = nil
}
func (s *Search) cached() bool {
	return s != nil && (s.query.Regex || strings.ContainsRune(s.query.Text, '\n'))
}

// matches caches one canonical non-overlapping source sequence per query/model
// revision/scope. Regex anchors keep full-document context even with a scope.
// Captures and original text remain private and never escape to GUI callbacks.
func (s *Search) matches(m *textedit.Model) [][]int {
	if s.Empty() {
		return nil
	}
	if s.cachedModel == m && s.cachedRevision == m.Revision() {
		return s.cachedMatches
	}
	s.cachedModel = m
	s.cachedRevision = m.Revision()
	s.cachedMatches = nil
	s.cachedText = ""
	if s.query.Regex {
		s.cachedText = m.Text()
		for _, idx := range s.regex.FindAllStringSubmatchIndex(s.cachedText, -1) {
			if s.scope != nil && (idx[0] < s.scope.Start || idx[1] > s.scope.End) {
				continue
			}
			if s.query.WholeWord && !wholeMatch(s.cachedText, idx[0], idx[1]) {
				continue
			}
			s.cachedMatches = append(s.cachedMatches, idx)
		}
	} else {
		after := 0
		s.scan(m, textedit.Range{End: m.Len()}, func(r textedit.Range) bool {
			if r.Start >= after {
				s.cachedMatches = append(s.cachedMatches, []int{r.Start, r.End})
				after = r.End
			}
			return true
		})
	}
	return s.cachedMatches
}
func wholeMatch(text string, a, b int) bool {
	first, last := false, false
	if a < b {
		r, _ := utf8.DecodeRuneInString(text[a:b])
		first = word(r)
		r, _ = utf8.DecodeLastRuneInString(text[a:b])
		last = word(r)
	}
	if a > 0 {
		r, _ := utf8.DecodeLastRuneInString(text[:a])
		if word(r) == first {
			return false
		}
	}
	if b < len(text) {
		r, _ := utf8.DecodeRuneInString(text[b:])
		if word(r) == last {
			return false
		}
	}
	return true
}
func (s *Search) findCached(m *textedit.Model, start int, backward bool) (textedit.Range, bool) {
	matches := s.matches(m)
	if len(matches) == 0 {
		return textedit.Range{}, false
	}
	var best []int
	if backward {
		for _, idx := range matches {
			if idx[1] <= start {
				best = idx
			}
		}
		if best == nil {
			best = matches[len(matches)-1]
		}
	} else {
		for _, idx := range matches {
			if idx[0] >= start {
				best = idx
				break
			}
		}
		if best == nil {
			best = matches[0]
		}
	}
	return textedit.Range{Start: best[0], End: best[1]}, true
}
func (s *Search) Replacement(m *textedit.Model, r textedit.Range, replacement string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	replacement = textedit.Normalize(replacement)
	if !s.query.Regex {
		return replacement, nil
	}
	for _, idx := range s.matches(m) {
		if idx[0] == r.Start && idx[1] == r.End {
			expanded := s.regex.ExpandString(nil, replacement, s.cachedText, idx)
			if len(expanded) > 32<<20 {
				return "", ErrOperationLimit
			}
			return string(expanded), nil
		}
	}
	return "", fmt.Errorf("sourceview: search match changed")
}
func (s *Search) replaceCached(m *textedit.Model, selection textedit.Selection, replacement string) (Plan, error) {
	p := Plan{Before: selection, After: selection}
	total := 0
	replacement = textedit.Normalize(replacement)
	for _, idx := range s.matches(m) {
		if len(p.Edits) >= 100000 {
			return Plan{}, ErrOperationLimit
		}
		text := replacement
		if s.query.Regex {
			text = string(s.regex.ExpandString(nil, replacement, s.cachedText, idx))
		}
		if len(text) > 32*1024*1024-total {
			return Plan{}, ErrOperationLimit
		}
		total += len(text)
		p.Edits = append(p.Edits, textedit.Edit{Range: textedit.Range{Start: idx[0], End: idx[1]}, Text: text})
	}
	p.After = mapSelection(selection, p.Edits)
	return p, nil
}
