package editor

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	textcore "github.com/golang-gui/goui-sourceview/internal/text"
)

// CompletionItem is an edit value, with no provider, request or editor state.
type CompletionItem struct {
	Label, Text, Detail string
	Range               *textcore.Range
	AdditionalEdits     []textcore.Edit
	Caret               *int
}

// The root owns its public named struct. Structural conversion at this private
// boundary avoids aliases, callback adapters and unbounded intermediate copies.
type completionValue interface {
	~struct {
		Label, Text, Detail string
		Range               *textcore.Range
		AdditionalEdits     []textcore.Edit
		Caret               *int
	}
}

func SanitizeCompletions[T completionValue](items []T) []T {
	out := make([]T, 0, min(64, len(items)))
	for _, value := range items {
		item := CompletionItem(value)
		if len(item.Label) > 512 || len(item.Detail) > 1024 || len(item.Text) > 64<<10 || len(item.AdditionalEdits) > 128 {
			continue
		}
		item.Label = textcore.Normalize(item.Label)
		item.Detail = strings.Join(strings.Fields(textcore.Normalize(item.Detail)), " ")
		if item.Text == "" {
			item.Text = item.Label
		}
		item.Text = textcore.Normalize(item.Text)
		if item.Label == "" || len(item.Text) > 64<<10 || strings.ContainsAny(item.Label, "\r\n\t\v\f\u0085\u2028\u2029") {
			continue
		}
		total := len(item.Text)
		valid := true
		for _, e := range item.AdditionalEdits {
			total += len(e.Text)
			if len(e.Text) > 64<<10 || total > 1<<20 {
				valid = false
				break
			}
		}
		if !valid {
			continue
		}
		if item.Range != nil {
			r := *item.Range
			item.Range = &r
		}
		if item.Caret != nil {
			at := *item.Caret
			item.Caret = &at
		}
		item.AdditionalEdits = slices.Clone(item.AdditionalEdits)
		out = append(out, T(item))
		if len(out) == 64 {
			break
		}
	}
	return out
}

func NormalizeCompletions[T completionValue](items []T, requestedPrefix string) []T {
	var out []T
	seen := map[string]bool{}
	prefix := strings.ToLower(requestedPrefix)
	for _, value := range items {
		candidate := CompletionItem(value)
		if len(candidate.Label) > 512 || !strings.HasPrefix(strings.ToLower(textcore.Normalize(candidate.Label)), prefix) {
			continue
		}
		clean := SanitizeCompletions([]T{value})
		if len(clean) == 0 {
			continue
		}
		item := CompletionItem(clean[0])
		if item.Text == requestedPrefix || seen[item.Text] {
			continue
		}
		seen[item.Text] = true
		out = append(out, clean[0])
		if len(out) == 64 {
			break
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return CompletionItem(out[i]).Label < CompletionItem(out[j]).Label })
	return out
}

// Candidate identity includes the actual edit, not its transient list index.
func SameCompletion[T completionValue](left, right T) bool {
	a, b := CompletionItem(left), CompletionItem(right)
	if a.Label != b.Label || a.Text != b.Text || !slices.Equal(a.AdditionalEdits, b.AdditionalEdits) {
		return false
	}
	if (a.Range == nil) != (b.Range == nil) || (a.Caret == nil) != (b.Caret == nil) {
		return false
	}
	return (a.Range == nil || *a.Range == *b.Range) && (a.Caret == nil || *a.Caret == *b.Caret)
}

func CompletionEdits[T completionValue](value T, fallback textcore.Range) ([]textcore.Edit, int, error) {
	item := CompletionItem(value)
	r := fallback
	if item.Range != nil {
		r = *item.Range
	}
	caret := len(item.Text)
	if item.Caret != nil {
		caret = *item.Caret
	}
	if caret < 0 || caret > len(item.Text) || !utf8.ValidString(item.Text[:caret]) {
		return nil, 0, fmt.Errorf("sourceview: invalid completion caret")
	}
	edits := slices.Clone(item.AdditionalEdits)
	for _, e := range edits {
		if e.Range.Start < r.End && e.Range.End > r.Start || e.Range.Start == r.Start || e.Range.End == r.Start && e.Range.Start == e.Range.End {
			return nil, 0, fmt.Errorf("sourceview: overlapping completion edits")
		}
	}
	edits = append(edits, textcore.Edit{Range: r, Text: item.Text})
	sort.SliceStable(edits, func(i, j int) bool { return edits[i].Range.Start < edits[j].Range.Start })
	end := r.Start + caret
	for _, e := range item.AdditionalEdits {
		if e.Range.End <= r.Start {
			end += len(textcore.Normalize(e.Text)) - (e.Range.End - e.Range.Start)
		}
	}

	return edits, end, nil
}

func DocumentWords[T any](text string, start, documentLength, rangeStart int, isWord func(rune) bool, build func(string) T) []T {
	end := start + len(text)
	var out []T
	wordStart := -1
	for i, r := range text {
		if isWord(r) {
			if wordStart < 0 {
				wordStart = i
			}
			continue
		}
		if wordStart >= 0 {
			if start+wordStart != rangeStart && (wordStart > 0 || start == 0) {
				out = append(out, build(text[wordStart:i]))
			}
			wordStart = -1
		}
	}
	if wordStart >= 0 && end == documentLength && start+wordStart != rangeStart {
		out = append(out, build(text[wordStart:]))
	}
	return out
}
