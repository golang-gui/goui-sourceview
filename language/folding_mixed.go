package language

import (
	"context"
	"sort"
	"strings"
)

// All mixed-language observations belong to one immutable-text worker scan.
// Tag names retain their full spelling, independently of bounded keyword data.
type mixedFoldObserver struct {
	pending []*mixedFoldTag
	tags    []mixedFoldElement
	closed  []foldLexeme
	php     []foldSpan
}
type mixedFoldTag struct {
	name    strings.Builder
	row     int
	closing bool
}
type mixedFoldElement struct {
	name string
	row  int
}
type foldSpan struct{ start, end int }

func (f *foldLexemes) startTag(closing bool) {
	if f != nil && f.mixed != nil {
		f.mixed.pending = append(f.mixed.pending, &mixedFoldTag{row: f.row, closing: closing})
	}
}
func (f *foldLexemes) tagName(part string) {
	if f != nil && f.mixed != nil && len(f.mixed.pending) > 0 {
		f.mixed.pending[len(f.mixed.pending)-1].name.WriteString(part)
	}
}
func (f *foldLexemes) abortTag() {
	if f != nil && f.mixed != nil && len(f.mixed.pending) > 0 {
		f.mixed.pending = f.mixed.pending[:len(f.mixed.pending)-1]
	}
}
func (f *foldLexemes) endTag(end int, self bool) {
	if f == nil || f.mixed == nil || len(f.mixed.pending) == 0 {
		return
	}
	m := f.mixed
	t := m.pending[len(m.pending)-1]
	name, row, closing := t.name.String(), t.row, t.closing
	m.pending = m.pending[:len(m.pending)-1]
	if self {
		return
	}
	if !closing {
		m.tags = append(m.tags, mixedFoldElement{name, row})
		return
	}
	if len(m.tags) == 0 {
		return
	}
	open := m.tags[len(m.tags)-1]
	if open.name != name {
		m.tags = nil
		return
	}
	m.tags = m.tags[:len(m.tags)-1]
	if f.row > open.row {
		m.closed = append(m.closed, foldLexeme{open.row, end})
	}
}
func (f *foldLexemes) phpSpan(start, end int) {
	if f == nil || f.mixed == nil || end <= start {
		return
	}
	p := &f.mixed.php
	if n := len(*p); n > 0 && (*p)[n-1].end == start {
		(*p)[n-1].end = end
	} else {
		*p = append(*p, foldSpan{start, end})
	}
}

func mixedFolding(id ID, t Tokenizer) FoldingFunc {
	return func(ctx context.Context, text string) ([]FoldRange, error) {
		observed := &mixedFoldObserver{}
		lexemes := &foldLexemes{row: -1, mixed: observed}
		f := structuralFolder{}
		var html strings.Builder
		var alt phpAlternativeFolds
		var regions []int
		err := syntaxLines(ctx, foldingTokenizer{t, lexemes}, text, func(l syntaxLine) {
			if ctx.Err() != nil {
				return
			}
			if id == PHP {
				// Retain byte positions and LF. '?' prevents a PHP island in a
				// tag name from accidentally manufacturing a static HTML name.
				projection := []byte(l.text)
				for _, span := range observed.php {
					for at := span.start; at < span.end; at++ {
						if at%4096 == 0 && ctx.Err() != nil {
							return
						}
						projection[at] = ' '
					}
					projection[span.start] = '?'
				}
				if l.row > 0 {
					html.WriteByte('\n')
				}
				html.Write(projection)
				l.tokens = phpFoldTokens(l.tokens, observed.php)
				observed.php = observed.php[:0]
				alt.line(ctx, l, &f)
			}
			if ctx.Err() != nil {
				return
			}
			f.delimiters(l, false, id)
			for _, closed := range lexemes.closed {
				f.add(closed.start, codeFoldEndLine(l.text, closed.end, l.row))
			}
			lexemes.closed = lexemes.closed[:0]
			for _, closed := range observed.closed {
				f.add(closed.start, codeFoldEndLine(l.text, closed.end, l.row))
			}
			observed.closed = observed.closed[:0]
			if marker := mixedFoldRegion(l, id); marker > 0 {
				regions = append(regions, l.row)
			} else if marker < 0 && len(regions) > 0 {
				f.add(regions[len(regions)-1], l.row)
				regions = regions[:len(regions)-1]
			}
		})
		if err != nil {
			return nil, err
		}
		if id == PHP {
			ranges, err := foldMarkup(ctx, html.String(), true)
			if err != nil {
				return nil, err
			}
			f.out = append(f.out, ranges...)
		}
		return noncrossingMixedFolds(ctx, f.out)
	}
}

func phpFoldTokens(tokens []Token, spans []foldSpan) []Token {
	var out []Token
	span := 0
	for _, t := range tokens {
		for span < len(spans) && spans[span].end <= t.Start {
			span++
		}
		for i := span; i < len(spans) && spans[i].start < t.End; i++ {
			part := t
			part.Start, part.End = max(t.Start, spans[i].start), min(t.End, spans[i].end)
			out = append(out, part)
		}
	}
	return out
}

func mixedFoldRegion(l syntaxLine, id ID) int {
	raw := strings.TrimSpace(l.text)
	if strings.HasPrefix(raw, "//") {
		raw = strings.TrimSpace(raw[2:])
	} else if id == PHP && strings.HasPrefix(raw, "#") {
		raw = strings.TrimSpace(raw[1:])
	} else {
		return 0
	}
	at := len(l.text) - len(strings.TrimLeft(l.text, " \t\r"))
	for _, t := range l.tokens {
		if t.Start <= at && at < t.End {
			if t.Class&ClassComment == 0 || l.before.Context().Comment {
				return 0
			}
			fields := strings.Fields(raw)
			if len(fields) > 0 {
				switch strings.TrimPrefix(fields[0], "#") {
				case "region":
					return 1
				case "endregion":
					return -1
				}
			}
			break
		}
	}
	return 0
}

// Source lines cannot represent crossing language extents. Keep the earlier
// enclosing candidate, omit crossing candidates and same-header inner ranges.
func noncrossingMixedFolds(ctx context.Context, ranges []FoldRange) ([]FoldRange, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(ranges, func(i, j int) bool {
		if ranges[i].StartLine != ranges[j].StartLine {
			return ranges[i].StartLine < ranges[j].StartLine
		}
		return ranges[i].EndLine > ranges[j].EndLine
	})
	var out []FoldRange
	var ends []int
	for i, r := range ranges {
		if i%1024 == 0 && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if len(out) > 0 && out[len(out)-1].StartLine == r.StartLine {
			continue
		}
		for len(ends) > 0 && ends[len(ends)-1] < r.StartLine {
			ends = ends[:len(ends)-1]
		}
		if len(ends) > 0 && r.EndLine > ends[len(ends)-1] {
			continue
		}
		out = append(out, r)
		ends = append(ends, r.EndLine)
	}
	return out, ctx.Err()
}

type phpAlternativeFolds struct {
	stack          []phpFoldControl
	header         *phpFoldHeader
	ending         *FoldRange
	parens, arrays int
	previous, last string
}
type phpFoldControl struct {
	kind string
	row  int
}
type phpFoldHeader struct {
	phpFoldControl
	stage, base int
}

var phpFoldEnds = map[string]string{"endif": "if", "endwhile": "while", "endfor": "for", "endforeach": "foreach", "endswitch": "switch", "enddeclare": "declare"}

func (p *phpAlternativeFolds) confirmEnd(f *structuralFolder) {
	if p.ending != nil {
		f.add(p.ending.StartLine, p.ending.EndLine)
		p.stack = p.stack[:len(p.stack)-1]
		p.ending = nil
	}
}
func (p *phpAlternativeFolds) line(ctx context.Context, l syntaxLine, f *structuralFolder) {
	remember := func(v string) { p.previous, p.last = p.last, v }
	for i, t := range l.tokens {
		if i%1024 == 0 && ctx.Err() != nil {
			return
		}
		value := l.text[t.Start:t.End]
		if t.Class&ClassComment != 0 {
			continue
		}
		if t.Kind == KindMeta {
			if strings.Contains(value, "?>") {
				p.confirmEnd(f)
			}
			continue
		}
		if t.Class&ClassString != 0 {
			p.ending = nil
			if p.header != nil && p.header.stage != 1 {
				p.header = nil
			}
			remember("literal")
			continue
		}
		if t.Kind != KindDelimiter {
			p.ending = nil
			if p.header != nil && p.header.stage != 1 {
				p.header = nil
			}
			word := strings.ToLower(value)
			member := p.last == "\\" || p.last == "function" || p.last == ">" && p.previous == "-" || p.last == ":" && p.previous == ":"
			if t.Kind == KindKeyword && !member && p.parens == 0 && p.arrays == 0 {
				switch word {
				case "if", "while", "for", "foreach", "switch", "declare":
					p.header = &phpFoldHeader{phpFoldControl: phpFoldControl{word, l.row}}
				default:
					if kind := phpFoldEnds[word]; kind != "" && len(p.stack) > 0 {
						open := p.stack[len(p.stack)-1]
						if open.kind == kind {
							p.ending = &FoldRange{open.row, codeFoldEndLine(l.text, t.End, l.row)}
						} else {
							p.stack = nil
						}
					}
				}
			}
			remember(word)
			continue
		}
		for _, c := range value {
			if c == ' ' || c == '\t' || c == '\r' {
				continue
			}
			if p.ending != nil {
				if c == ';' {
					p.confirmEnd(f)
				} else {
					p.ending = nil
				}
			}
			if h := p.header; h != nil {
				switch h.stage {
				case 0:
					if c == '(' {
						h.stage, h.base = 1, p.parens
					} else {
						p.header = nil
					}
				case 1:
					if c == ')' && p.parens == h.base+1 {
						h.stage = 2
					}
				case 2:
					if c == ':' {
						p.stack = append(p.stack, h.phpFoldControl)
					}
					p.header = nil
				}
			}
			switch c {
			case '(':
				p.parens++
			case ')':
				p.parens = max(0, p.parens-1)
			case '[':
				p.arrays++
			case ']':
				p.arrays = max(0, p.arrays-1)
			}
			remember(string(c))
		}
	}
}
