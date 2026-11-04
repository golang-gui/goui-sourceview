package analysis

import (
	"sort"

	"github.com/golang-gui/goui-sourceview/language"
)

// Definition adapts an immutable result to today's pull-only tokenizer API.
// Replacing this definition invalidates completion and folding in SourceView;
// this deliberately visible limitation is documented by the example.
func Definition(result Result) (*language.Language, error) {
	base, _ := language.Builtin(language.Go)
	d := base.Definition()
	result.Spans = append([]Span(nil), result.Spans...)
	d.Tokenizer = &tokenizer{base: base.Tokenizer(), result: result}
	d.Styles = append(d.Styles, Type, Function, Field, StringVariable, NumberVariable, Variable, Constant, Invalid)
	return language.New(d)
}

type tokenizer struct {
	base   language.Tokenizer
	result Result
}
type state struct {
	base      language.State
	lineStart int
	valid     bool
}

func (s state) Equal(other language.State) bool {
	t, ok := other.(state)
	return ok && s.lineStart == t.lineStart && s.valid == t.valid && s.base.Equal(t.base)
}
func (s state) Context() language.Context { return s.base.Context() }
func (t *tokenizer) InitialState() language.State {
	return state{base: t.base.InitialState(), valid: true}
}
func (t *tokenizer) Scan(in language.Input, previous language.State) (language.Result, error) {
	s := previous.(state)
	result, err := t.base.Scan(in, s.base)
	if err != nil {
		return result, err
	}
	start, end := s.lineStart+in.Offset, s.lineStart+in.Offset+result.Consumed
	// Once an edited fragment differs, preserve lexical colors only for the
	// remaining suffix. Never read a live model or mutate a published snapshot.
	if start < 0 || end > len(t.result.Snapshot.Text) || t.result.Snapshot.Text[start:end] != in.Text[:result.Consumed] {
		s.valid = false
	}
	if in.LineEnd && result.Consumed == len(in.Text) && end < len(t.result.Snapshot.Text) && t.result.Snapshot.Text[end] != '\n' {
		s.valid = false
	}
	if s.valid {
		from := in.Offset
		if result.ReplaceFrom != nil {
			from = *result.ReplaceFrom
		}
		low, high := s.lineStart+from, end
		spans := t.result.Spans
		i := sort.Search(len(spans), func(i int) bool { return spans[i].End > low })
		// Merge the two ordered interval streams. Semantic information only
		// overrides color; string/comment/delimiter classes remain lexical.
		base := result.Tokens
		out := make([]language.Token, 0, len(base)+16)
		b := 0
		for pos := from; pos < high-s.lineStart; {
			absolute := s.lineStart + pos
			for b < len(base) && base[b].End <= pos {
				b++
			}
			for i < len(spans) && spans[i].End <= absolute {
				i++
			}
			end := high - s.lineStart
			kind, class := language.KindPlain, language.Class(0)
			if b < len(base) {
				tok := base[b]
				if tok.Start <= pos {
					kind, class = tok.Kind, tok.Class
					end = min(end, tok.End)
				} else {
					end = min(end, tok.Start)
				}
			}
			if i < len(spans) {
				span := spans[i]
				if span.Start <= absolute {
					kind = span.Kind
					end = min(end, span.End-s.lineStart)
				} else {
					end = min(end, span.Start-s.lineStart)
				}
			}
			if kind != language.KindPlain || class != 0 {
				if n := len(out); n > 0 && out[n-1].End == pos && out[n-1].Kind == kind && out[n-1].Class == class {
					out[n-1].End = end
				} else {
					out = append(out, language.Token{Start: pos, End: end, Kind: kind, Class: class})
				}
			}
			pos = end
		}
		result.Tokens = out
	}
	s.base = result.State
	if in.LineEnd && result.Consumed == len(in.Text) {
		s.lineStart = end + 1
	}
	result.State = s
	return result, nil
}
