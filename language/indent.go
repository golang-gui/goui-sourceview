package language

import (
	"context"
	"strings"
)

// ReindentContext is an immutable normalized snapshot and an inclusive logical
// line scope. Providers run on a worker and must honor cancellation.
type ReindentContext struct {
	Text                  string
	StartLine, EndLine    int
	TabWidth, IndentWidth int
	InsertSpaces          bool
}

// LineIndent replaces only one logical line's leading ASCII spaces and tabs.
// Providers return unique ascending lines within the requested scope. Omitted
// lines are unchanged, including literal data and ambiguous syntax.
type LineIndent struct {
	Line int
	Text string
}
type ReindentFunc func(context.Context, ReindentContext) ([]LineIndent, error)

func indentation(columns, tabWidth int, spaces bool) string {
	if spaces {
		return strings.Repeat(" ", columns)
	}
	return strings.Repeat("\t", columns/tabWidth) + strings.Repeat(" ", columns%tabWidth)
}
func structuralReindent(id ID, t Tokenizer) ReindentFunc {
	return func(ctx context.Context, c ReindentContext) ([]LineIndent, error) {
		var out []LineIndent
		var stack []byte
		var bases []int
		invalid := false
		err := syntaxLines(ctx, t, c.Text, func(l syntaxLine) {
			if invalid {
				return
			}
			before := l.before.Context()
			code := strings.TrimSpace(l.code)
			if tomlTableLine(id, l) {
				if l.row >= c.StartLine && l.row <= c.EndLine {
					out = append(out, LineIndent{l.row, ""})
				}
				return
			}
			level := len(stack)
			// Closing delimiters at the start align with their opener, not its body.
			for i := 0; i < len(code) && strings.ContainsRune("}])", rune(code[i])); i++ {
				level = max(0, level-1)
			}
			if l.row >= c.StartLine && l.row <= c.EndLine && !before.String && !before.Comment && strings.TrimSpace(l.text) != "" {
				columns := level * c.IndentWidth
				if id == Python {
					if len(bases) == 0 {
						columns = -1
					} else {
						columns = bases[len(bases)-1] + c.IndentWidth
						if level < len(stack) {
							columns = bases[max(0, level)]
						}
					}
				}
				if columns >= 0 {
					out = append(out, LineIndent{l.row, indentation(columns, c.TabWidth, c.InsertSpaces)})
				}
			}
			for _, tok := range l.tokens {
				if tok.Class&ClassDelimiter == 0 {
					continue
				}
				for i := tok.Start; i < tok.End; i++ {
					ch := l.text[i]
					switch ch {
					case '{', '[', '(':
						stack = append(stack, ch)
						col := 0
						for _, r := range l.text {
							if r == ' ' {
								col++
							} else if r == '\t' {
								col += c.TabWidth - col%c.TabWidth
							} else {
								break
							}
						}
						bases = append(bases, col)
					case '}', ']', ')':
						if len(stack) == 0 {
							invalid = true
							return
						}
						open := stack[len(stack)-1]
						if open == '{' && ch != '}' || open == '[' && ch != ']' || open == '(' && ch != ')' {
							invalid = true
							return
						}
						stack = stack[:len(stack)-1]
						bases = bases[:len(bases)-1]
					}
				}
			}
		})
		if err != nil {
			return nil, err
		}
		if invalid {
			return nil, nil
		}
		return out, nil
	}
}

func assistedNewline(id ID, t Tokenizer) func(NewlineContext) (NewlineEdit, bool) {
	return func(c NewlineContext) (NewlineEdit, bool) {
		if !c.Known || c.Context.String {
			return NewlineEdit{}, false
		}
		before, after := strings.TrimSpace(c.Before), strings.TrimSpace(c.After)
		result := func(s string) (NewlineEdit, bool) { return NewlineEdit{Text: s, Caret: len(s)}, true }
		if c.Context.Comment {
			if strings.HasPrefix(before, "/*") || strings.HasPrefix(before, "*") && !strings.HasPrefix(before, "*/") {
				if !strings.Contains(before, "*/") {
					prefix := c.Indent
					if strings.HasPrefix(before, "/*") {
						prefix += " "
					}
					return result("\n" + prefix + "* ")
				}
			}
			return NewlineEdit{}, false
		}
		yamlFlow := false
		if data, ok := c.Context.Data.(yamlEditingContext); ok {
			yamlFlow = data.flow > 0
		}
		if id == YAML && !yamlFlow {
			if before == "-" {
				return NewlineEdit{DeleteBefore: len(c.Before) - len(c.Indent), Text: "\n" + c.Indent, Caret: 1 + len(c.Indent)}, true
			}
		}
		// The bounded line prefix is scanned with the same lexical rules so colons
		// and delimiters inside complete strings/comments cannot trigger indentation.
		code := before
		depth := 0
		yamlBlock := false
		if t != nil {
			_ = syntaxLines(context.Background(), t, c.Before, func(l syntaxLine) {
				code = strings.TrimSpace(l.code)
				if id == YAML {
					s := l.after.(builtinState).yaml
					yamlBlock = s != nil && s.block
				}
				for _, tok := range l.tokens {
					if tok.Class&ClassDelimiter == 0 {
						continue
					}
					for _, r := range l.text[tok.Start:tok.End] {
						if strings.ContainsRune("([{", r) {
							depth++
						}
						if strings.ContainsRune(")]}", r) {
							depth--
						}
					}
				}
			})
		}
		if id == Python && depth == 0 && pythonBlockHeader(code) {
			return result("\n" + c.Indent + c.Unit)
		}
		if id == YAML && !yamlFlow && depth == 0 && (strings.HasSuffix(code, ":") || yamlBlock) {
			return result("\n" + c.Indent + c.Unit)
		}
		if id == YAML && !yamlFlow && strings.HasPrefix(before, "- ") && after == "" && depth == 0 {
			return result("\n" + c.Indent + "- ")
		}
		for _, p := range []Pair{{Open: "{", Close: "}"}, {Open: "[", Close: "]"}, {Open: "(", Close: ")"}} {
			if strings.HasSuffix(code, p.Open) {
				if strings.HasPrefix(after, p.Close) {
					return NewlineEdit{}, false
				}
				return result("\n" + c.Indent + c.Unit)
			}
		}
		return NewlineEdit{}, false
	}
}

func markupReindent(rules *TagRules) ReindentFunc {
	return func(ctx context.Context, c ReindentContext) ([]LineIndent, error) {
		// Full tags, comments and raw-text regions are parsed once. Only lines that
		// begin at a confirmed tag boundary are eligible; literal text is untouched.
		type boundary struct{ at, depth int }
		var points []boundary
		var stack []string
		for at := 0; at < len(c.Text); {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			n := strings.IndexByte(c.Text[at:], '<')
			if n < 0 {
				break
			}
			at += n
			raw := len(stack) > 0 && rules.contains(rules.RawTextElements, stack[len(stack)-1])
			if !raw {
				term := ""
				switch {
				case strings.HasPrefix(c.Text[at:], "<!--"):
					term = "-->"
				case strings.HasPrefix(c.Text[at:], "<![CDATA["):
					term = "]]>"
				case strings.HasPrefix(c.Text[at:], "<?"):
					term = "?>"
				}
				if term != "" {
					n := strings.Index(c.Text[at+2:], term)
					if n < 0 {
						break
					}
					at += 2 + n + len(term)
					continue
				}
			}
			tag, ok := readMarkupTag(c.Text, at)
			if !ok {
				at++
				continue
			}
			name := rules.key(tag.name)
			if raw && (!tag.closing || name != stack[len(stack)-1]) {
				at++
				continue
			}
			depth := len(stack)
			if tag.closing {
				if len(stack) == 0 || stack[len(stack)-1] != name {
					return nil, nil
				}
				stack = stack[:len(stack)-1]
				depth--
			}
			if !raw {
				points = append(points, boundary{at, depth})
			}
			if !tag.closing && !tag.self && !rules.contains(rules.VoidElements, name) {
				stack = append(stack, name)
			}
			at = tag.end
		}
		var out []LineIndent
		index := 0
		for row, at := 0, 0; at <= len(c.Text); row++ {
			end := strings.IndexByte(c.Text[at:], '\n')
			if end < 0 {
				end = len(c.Text)
			} else {
				end += at
			}
			n := len(c.Text[at:end]) - len(strings.TrimLeft(c.Text[at:end], " \t"))
			start := at + n
			for index < len(points) && points[index].at < start {
				index++
			}
			if row >= c.StartLine && row <= c.EndLine && index < len(points) && points[index].at == start {
				out = append(out, LineIndent{row, indentation(points[index].depth*c.IndentWidth, c.TabWidth, c.InsertSpaces)})
			}
			if end == len(c.Text) {
				break
			}
			at = end + 1
		}
		return out, ctx.Err()
	}
}

func markupNewline(rules *TagRules) func(NewlineContext) (NewlineEdit, bool) {
	return func(c NewlineContext) (NewlineEdit, bool) {
		if !c.Known || c.Context.String || c.Context.Comment || c.Context.Tag {
			return NewlineEdit{}, false
		}
		before := strings.TrimRight(c.Before, " \t")
		_, tag := rules.tags(before)
		if tag.end != len(before) || tag.name == "" || tag.closing || tag.self || rules.contains(rules.VoidElements, tag.name) || rules.contains(rules.RawTextElements, tag.name) {
			return NewlineEdit{}, false
		}
		text := "\n" + c.Indent + c.Unit
		caret := len(text)
		after := strings.TrimLeft(c.After, " \t")
		closing, ok := readMarkupTag(after, 0)
		remove := 0
		if ok && closing.closing && rules.key(closing.name) == tag.name {
			text += "\n" + c.Indent
			remove = len(c.After) - len(after)
		}
		return NewlineEdit{Text: text, Caret: caret, DeleteAfter: remove}, true
	}
}
