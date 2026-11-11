package editor

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	textcore "github.com/golang-gui/goui-sourceview/internal/text"
	"github.com/golang-gui/goui-sourceview/language"
)

func ReindentPlan(ctx context.Context, text string, s textcore.Selection, first, last int, results []language.LineIndent) (Plan, error) {
	p := Plan{Before: s, After: s}
	m := textcore.NewModel(text)
	previous := first - 1
	for _, r := range results {
		if err := ctx.Err(); err != nil {
			return p, err
		}
		if r.Line <= previous || r.Line < first || r.Line > last || len(r.Text) > 64*1024 || strings.Trim(r.Text, " \t") != "" {
			return p, fmt.Errorf("sourceview: invalid line indentation")
		}
		previous = r.Line
		line, _ := m.LineRange(r.Line)
		n := len(text[line.Start:line.End]) - len(strings.TrimLeft(text[line.Start:line.End], " \t"))
		if text[line.Start:line.Start+n] != r.Text {
			p.Edits = append(p.Edits, textcore.Edit{Range: textcore.Range{Start: line.Start, End: line.Start + n}, Text: r.Text})
		}
	}
	p.After = textcore.Selection{Anchor: MapPosition(s.Anchor, p.Edits), Caret: MapPosition(s.Caret, p.Edits)}
	return p, nil
}

func CleanupPlan(ctx context.Context, text string, s textcore.Selection, kind string) (Plan, error) {
	p := Plan{Before: s, After: s}
	m := textcore.NewModel(text)
	first, last, err := SelectedLines(m, s)
	if err != nil {
		return p, err
	}
	if s.Anchor == s.Caret {
		if kind == "trim" {
			first, last = 0, m.LineCount()-1
		} else {
			last = min(first+1, m.LineCount()-1)
		}
	}
	for row := first; row <= last; row++ {
		if err := ctx.Err(); err != nil {
			return p, err
		}
		line, _ := m.LineRange(row)
		if kind == "trim" {
			n := len(strings.TrimRight(text[line.Start:line.End], " \t"))
			if line.Start+n < line.End {
				p.Edits = append(p.Edits, textcore.Edit{Range: textcore.Range{Start: line.Start + n, End: line.End}})
			}
		} else if row < last {
			next, _ := m.LineRange(row + 1)
			n := len(text[next.Start:next.End]) - len(strings.TrimLeft(text[next.Start:next.End], " \t"))
			sep := ""
			if line.End > line.Start && next.Start+n < next.End && text[line.End-1] != ' ' && text[line.End-1] != '\t' {
				sep = " "
			}
			p.Edits = append(p.Edits, textcore.Edit{Range: textcore.Range{Start: line.End, End: next.Start + n}, Text: sep})
		}
	}
	p.After = textcore.Selection{Anchor: MapPosition(s.Anchor, p.Edits), Caret: MapPosition(s.Caret, p.Edits)}
	return p, nil
}

func ShiftedPaste(text, prefix string, o IndentOptions) string {
	text = textcore.Normalize(text)
	lines := strings.Split(text, "\n")
	if len(lines) < 2 {
		return text
	}
	columns := func(s string) int {
		n := 0
		for _, r := range s {
			if r == ' ' {
				n++
			} else if r == '\t' {
				n += o.TabWidth - n%o.TabWidth
			} else {
				break
			}
		}
		return n
	}
	base := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		n := columns(line)
		if base < 0 || n < base {
			base = n
		}
	}
	if base < 0 {
		return text
	}
	target := columns(prefix)
	for i, line := range lines {
		n := len(line) - len(strings.TrimLeft(line, " \t"))
		if n == len(line) {
			continue
		}
		col := max(0, columns(line)-base+target)
		if i == 0 {
			col = max(0, col-target)
		}
		indent := strings.Repeat(" ", col)
		if !o.InsertSpaces {
			indent = strings.Repeat("\t", col/o.TabWidth) + strings.Repeat(" ", col%o.TabWidth)
		}
		lines[i] = indent + line[n:]
	}
	return strings.Join(lines, "\n")
}

func ReindentedPaste(ctx context.Context, text, prefix string, o IndentOptions, l *language.Language) (string, error) {
	text = textcore.Normalize(text)
	t := l.Tokenizer()
	if t == nil {
		return text, nil
	}
	state := t.InitialState()
	for at := 0; at <= len(text); {
		if state.Context().String || state.Context().Comment {
			return text, nil
		}
		end := strings.IndexByte(text[at:], '\n')
		if end < 0 {
			end = len(text)
		} else {
			end += at
		}
		line := text[at:end]
		for offset := 0; ; {
			if err := ctx.Err(); err != nil {
				return "", err
			}
			limit := min(4096, len(line)-offset)
			for offset+limit < len(line) && !utf8.RuneStart(line[offset+limit]) {
				limit++
			}
			result, err := t.Scan(language.Input{Text: line[offset:], Offset: offset, Limit: limit, LineEnd: true, DocumentEnd: end == len(text)}, state)
			if err != nil {
				return text, nil
			}
			if result.Consumed < 0 || result.Consumed > len(line)-offset || result.Consumed == 0 && offset < len(line) {
				return text, nil
			}
			state = result.State
			offset += result.Consumed
			if offset == len(line) {
				break
			}
		}
		if end == len(text) {
			break
		}
		at = end + 1
	}
	if state.Context().String || state.Context().Comment {
		return text, nil
	}
	return ShiftedPaste(text, prefix, o), nil
}

func CasePlan(ctx context.Context, text string, s textcore.Selection, r textcore.Range, upper bool) (Plan, error) {
	p := Plan{Before: s, After: s}
	convert := strings.ToUpper
	if !upper {
		convert = strings.ToLower
	}
	result := convert(text)
	if result == text {
		return p, ctx.Err()
	}
	p.Edits = []textcore.Edit{{Range: r, Text: result}}
	if s.Anchor != s.Caret {
		p.After = textcore.Selection{Anchor: r.Start, Caret: r.Start + len(result)}
		if s.Anchor > s.Caret {
			p.After.Anchor, p.After.Caret = p.After.Caret, p.After.Anchor
		}
	} else {
		at := r.Start + len(convert(text[:s.Caret-r.Start]))
		p.After = textcore.Selection{Anchor: at, Caret: at}
	}
	return p, ctx.Err()
}

func LeadingColumns(text string, tabWidth int) (bytes, columns int) {
	for bytes < len(text) {
		switch text[bytes] {
		case ' ':
			columns++
		case '\t':
			columns += tabWidth - columns%tabWidth
		default:
			return
		}
		bytes++
	}
	return
}
