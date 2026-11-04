package language

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"
)

// syntaxLine is transient worker-owned data. Tokens use logical-line offsets;
// masked text retains byte positions while excluding strings and comments.
type syntaxLine struct {
	row           int
	text, code    string
	tokens        []Token
	before, after State
}

func syntaxLines(ctx context.Context, tokenizer Tokenizer, text string, visit func(syntaxLine)) error {
	state := tokenizer.InitialState()
	for row, at := 0, 0; ; row++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		end := strings.IndexByte(text[at:], '\n')
		if end < 0 {
			end = len(text)
		} else {
			end += at
		}
		line := text[at:end]
		before := state
		var tokens []Token
		for offset := 0; ; {
			if err := ctx.Err(); err != nil {
				return err
			}
			limit := min(4096, len(line)-offset)
			for offset+limit < len(line) && !utf8.RuneStart(line[offset+limit]) {
				limit++
			}
			result, err := tokenizer.Scan(Input{Text: line[offset:], Offset: offset, Limit: limit, LineEnd: true, DocumentEnd: end == len(text)}, state)
			if err != nil {
				return err
			}
			if result.Consumed < 0 || result.Consumed > len(line)-offset || result.Consumed == 0 && offset < len(line) {
				return fmt.Errorf("language: scanner made no progress")
			}
			if result.ReplaceFrom != nil {
				from := *result.ReplaceFrom
				for len(tokens) > 0 && tokens[len(tokens)-1].Start >= from {
					tokens = tokens[:len(tokens)-1]
				}
				if len(tokens) > 0 && tokens[len(tokens)-1].End > from {
					tokens[len(tokens)-1].End = from
				}
			}
			tokens = append(tokens, result.Tokens...)
			state = result.State
			offset += result.Consumed
			if offset == len(line) {
				break
			}
		}
		code := []byte(line)
		for _, t := range tokens {
			if t.Class&(ClassString|ClassComment) != 0 {
				for i := max(0, t.Start); i < min(len(code), t.End); i++ {
					code[i] = ' '
				}
			}
		}
		visit(syntaxLine{row, line, string(code), tokens, before, state})
		if end == len(text) {
			break
		}
		at = end + 1
	}
	return ctx.Err()
}

type structuralFolder struct {
	out                 []FoldRange
	stack               []foldOpen
	literal, comment    int
	section, sectionEnd int
	indent              []indentFold
	last                int
}
type indentFold struct {
	row, column int
	sequence    bool
}

func (f *structuralFolder) add(start, end int) {
	if end > start {
		f.out = append(f.out, FoldRange{start, end})
	}
}
func (f *structuralFolder) delimiters(l syntaxLine, skip bool) {
	if skip {
		return
	}
	for _, t := range l.tokens {
		if t.Class&ClassDelimiter == 0 {
			continue
		}
		for i := t.Start; i < t.End; i++ {
			c := l.text[i]
			switch c {
			case '{', '[', '(':
				f.stack = append(f.stack, foldOpen{l.row, c})
			case '}', ']', ')':
				if len(f.stack) == 0 {
					continue
				}
				o := f.stack[len(f.stack)-1]
				if o.char == '{' && c != '}' || o.char == '[' && c != ']' || o.char == '(' && c != ')' {
					f.stack = nil
					continue
				}
				f.stack = f.stack[:len(f.stack)-1]
				f.add(o.line, foldEndLine(l.text, i+1, l.row, true))
			}
		}
	}
}
func structuralFolding(id ID, t Tokenizer) FoldingFunc {
	return func(ctx context.Context, text string) ([]FoldRange, error) {
		var regions []FoldRange
		var regionStack []int
		var pythonContinuation bool
		var pythonHeader *indentFold
		f := structuralFolder{literal: -1, comment: -1, section: -1, sectionEnd: -1, last: -1}
		err := syntaxLines(ctx, t, text, func(l syntaxLine) {
			before, after := l.before.Context(), l.after.Context()
			if marker := foldRegionMarker(id, l); marker != 0 {
				if marker > 0 {
					regionStack = append(regionStack, l.row)
				} else if len(regionStack) > 0 {
					start := regionStack[len(regionStack)-1]
					regionStack = regionStack[:len(regionStack)-1]
					if l.row > start {
						regions = append(regions, FoldRange{start, l.row})
					}
				}
			}
			trim := strings.TrimSpace(l.code)
			raw := strings.TrimSpace(l.text)
			column := len(l.text) - len(strings.TrimLeft(l.text, " "))
			// State identifies multiline literals, including a closing line. Per-line
			// strings are still excluded by token classes in delimiter processing.
			if id != YAML && !before.String && after.String {
				f.literal = l.row
			}
			if id != YAML && before.String && !after.String && f.literal >= 0 {
				f.add(f.literal, literalEnd(l, ClassString))
				f.literal = -1
			}
			if !before.Comment && after.Comment {
				f.comment = l.row
			}
			if before.Comment && !after.Comment && f.comment >= 0 {
				f.add(f.comment, literalEnd(l, ClassComment))
				f.comment = -1
			}
			table := tomlTableLine(id, l)
			if table {
				f.add(f.section, f.sectionEnd)
				f.section, f.sectionEnd = l.row, l.row
			} else if id == TOML && f.section >= 0 && (trim != "" || before.String) {
				f.sectionEnd = l.row
			}
			continued := len(f.stack) > 0 || id == Python && pythonContinuation
			f.delimiters(l, table)
			if id == Python {
				pythonContinuation = strings.HasSuffix(trim, "\\")
				if !continued && !strings.HasSuffix(trim, ":") && pythonBlockHeader(trim+":") && (len(f.stack) > 0 || pythonContinuation) {
					pythonHeader = &indentFold{row: l.row, column: column}
				}
				if continued {
					f.last = l.row
					if len(f.stack) == 0 && !pythonContinuation {
						if pythonHeader != nil && strings.HasSuffix(trim, ":") {
							f.indent = append(f.indent, *pythonHeader)
						}
						pythonHeader = nil
					}
					return
				}
			}
			if id == YAML && continued {
				f.last = l.row
				return
			}
			if id != YAML && id != Python {
				return
			}
			blockContent := false
			if id == YAML {
				s := l.after.(builtinState).yaml
				blockContent = s != nil && s.block && !s.blockHeader && column > s.blockBase
				// A block scalar header ends in a String context, but its body must
				// participate in its parent's extent, never in delimiter matching.
				if !before.String && (raw == "---" || raw == "...") {
					f.stack = nil
					for _, o := range f.indent {
						f.add(o.row, f.last)
					}
					f.indent = nil
					f.last = -1
					return
				}
			}
			if raw == "" || strings.HasPrefix(raw, "#") {
				if blockContent {
					f.last = l.row
				}
				return
			}
			quoted := before.String
			if id == YAML {
				s := l.before.(builtinState).yaml
				quoted = s != nil && s.quote != 0
			}
			if quoted || blockContent {
				f.last = l.row
				return
			}
			if strings.Contains(l.text[:column], "\t") || strings.HasPrefix(l.text[column:], "\t") {
				return
			}
			seq := id == YAML && (strings.HasPrefix(trim, "- ") || trim == "-")
			for len(f.indent) > 0 {
				o := f.indent[len(f.indent)-1]
				if column > o.column || id == YAML && seq && column == o.column && !o.sequence {
					break
				}
				f.add(o.row, f.last)
				f.indent = f.indent[:len(f.indent)-1]
			}
			f.last = l.row
			header := false
			if id == Python {
				header = pythonBlockHeader(trim) && len(f.stack) == 0
			} else {
				s := l.after.(builtinState).yaml
				header = s != nil && s.block || strings.HasSuffix(trim, ":") || seq
			}
			if header {
				f.indent = append(f.indent, indentFold{l.row, column, seq})
			}
		})
		if err != nil {
			return nil, err
		}
		if f.section >= 0 {
			f.add(f.section, f.sectionEnd)
		}
		for _, o := range f.indent {
			f.add(o.row, f.last)
		}
		for _, r := range regions {
			crossing := false
			for i, b := range f.out {
				if i%1024 == 0 {
					if err := ctx.Err(); err != nil {
						return nil, err
					}
				}
				if b.StartLine < r.StartLine && r.StartLine <= b.EndLine && b.EndLine < r.EndLine || r.StartLine < b.StartLine && b.StartLine <= r.EndLine && r.EndLine < b.EndLine {
					crossing = true
					break
				}
			}
			if !crossing {
				f.out = append(f.out, r)
			}
		}
		return f.out, nil
	}
}
func pythonBlockHeader(code string) bool {
	if !strings.HasSuffix(code, ":") {
		return false
	}
	fields := strings.Fields(strings.TrimSuffix(code, ":"))
	if len(fields) == 0 {
		return false
	}
	if fields[0] == "async" && len(fields) > 1 {
		fields = fields[1:]
	}
	switch fields[0] {
	case "if", "elif", "else", "for", "while", "try", "except", "finally", "with", "def", "class", "match", "case":
		return true
	}
	return false
}

func tomlTableLine(id ID, l syntaxLine) bool {
	if id != TOML {
		return false
	}
	s := l.before.(builtinState).toml
	if s != nil && (s.frames != nil || s.quote != 0) {
		return false
	}
	trim := strings.TrimSpace(l.code)
	return strings.HasPrefix(trim, "[") && strings.HasSuffix(trim, "]")
}
func literalEnd(l syntaxLine, class Class) int {
	end := 0
	for _, t := range l.tokens {
		if t.Class&class != 0 {
			end = t.End
		} else if end > 0 {
			break
		}
	}
	return foldEndLine(l.text, end, l.row, true)
}

func foldRegionMarker(id ID, l syntaxLine) int {
	if l.before.Context().String || l.before.Context().Comment {
		return 0
	}
	raw := strings.TrimSpace(l.text)
	switch id {
	case Go, JavaScript, TypeScript:
		if !strings.HasPrefix(raw, "//") {
			return 0
		}
		raw = strings.TrimSpace(raw[2:])
	case Python:
		if !strings.HasPrefix(raw, "#") {
			return 0
		}
		raw = strings.TrimSpace(raw[1:])
	case CSS:
		if !strings.HasPrefix(raw, "/*") || !strings.HasSuffix(raw, "*/") {
			return 0
		}
		raw = strings.TrimSpace(raw[2 : len(raw)-2])
	default:
		return 0
	}
	fields := strings.Fields(raw)
	if len(fields) == 0 {
		return 0
	}
	marker := strings.TrimPrefix(fields[0], "#")
	if marker == "region" {
		return 1
	}
	if marker == "endregion" {
		return -1
	}
	return 0
}
