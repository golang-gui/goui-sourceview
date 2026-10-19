package language

import (
	"context"
	"strings"
	"unicode"
	"unicode/utf8"
)

// FoldRange retains StartLine and hides StartLine+1 through EndLine, inclusive.
// Lines are zero-based logical lines. Ranges may nest but must not cross.
type FoldRange struct{ StartLine, EndLine int }

// FoldingFunc analyzes immutable UTF-8/LF text on a worker. It must honor ctx
// and must not access GUI objects or mutable models. Empty results are normal.
type FoldingFunc func(context.Context, string) ([]FoldRange, error)

type foldOpen struct {
	line int
	char byte
}

func foldJSON(ctx context.Context, text string) ([]FoldRange, error) {
	var out []FoldRange
	var stack []foldOpen
	line, quote, escaped := 0, false, false
	for at := 0; at < len(text); at++ {
		if at%16384 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		b := text[at]
		if b == '\n' {
			line++
			quote, escaped = false, false
			continue
		}
		if quote {
			if escaped {
				escaped = false
			} else if b == '\\' {
				escaped = true
			} else if b == '"' {
				quote = false
			}
			continue
		}
		switch b {
		case '"':
			quote = true
		case '{', '[':
			stack = append(stack, foldOpen{line, b})
		case '}', ']':
			if len(stack) == 0 {
				continue
			}
			open := stack[len(stack)-1]
			if open.char == '{' && b != '}' || open.char == '[' && b != ']' {
				stack = nil
				continue
			}
			stack = stack[:len(stack)-1]
			end := foldEndLine(text, at+1, line, true)
			if end > open.line {
				out = append(out, FoldRange{open.line, end})
			}
		}
	}
	return out, ctx.Err()
}

// Keep a closing line visible if it also contains unrelated following text.
func foldEndLine(text string, end, line int, comma bool) int {
	tail, _, _ := strings.Cut(text[end:], "\n")
	tail = strings.TrimSpace(tail)
	if tail != "" && !(comma && tail == ",") {
		return line - 1
	}
	return line
}

type markupTag struct {
	name                           string
	start, end, nameStart, nameEnd int
	closing, self                  bool
}

func tagNameRune(r rune, first bool) bool {
	return r == '_' || r == ':' || unicode.IsLetter(r) || !first && (r == '-' || r == '.' || unicode.IsDigit(r) || unicode.IsMark(r))
}
func readMarkupTag(text string, start int) (markupTag, bool) {
	t := markupTag{start: start}
	if start >= len(text) || text[start] != '<' {
		return t, false
	}
	at := start + 1
	if at < len(text) && text[at] == '/' {
		t.closing = true
		at++
	}
	t.nameStart = at
	for at < len(text) {
		r, n := utf8.DecodeRuneInString(text[at:])
		if !tagNameRune(r, at == t.nameStart) {
			break
		}
		at += n
	}
	t.nameEnd = at
	if at == t.nameStart || at >= len(text) {
		return t, false
	}
	t.name = text[t.nameStart:t.nameEnd]
	if text[at] != '>' && text[at] != '/' && !strings.ContainsRune(" \t\r\n", rune(text[at])) {
		return t, false
	}
	var quote byte
	for ; at < len(text); at++ {
		b := text[at]
		if quote != 0 {
			if b == quote {
				quote = 0
			}
			continue
		}
		if b == '"' || b == '\'' {
			quote = b
			continue
		}
		if b == '<' {
			return t, false
		}
		if b == '>' {
			t.end = at + 1
			t.self = at > start && text[at-1] == '/'
			if t.closing && strings.TrimSpace(text[t.nameEnd:at]) != "" {
				return t, false
			}
			return t, true
		}
	}
	return t, false
}

func htmlVoid(name string) bool {
	switch name {
	case "area", "base", "br", "col", "embed", "hr", "img", "input", "link", "meta", "param", "source", "track", "wbr":
		return true
	}
	return false
}
func rawMarkup(name string) bool {
	return name == "script" || name == "style" || name == "textarea" || name == "title"
}

func foldMarkup(ctx context.Context, text string, html bool) ([]FoldRange, error) {
	type openTag struct {
		tag  markupTag
		line int
	}
	var stack []openTag
	var out []FoldRange
	line, at := 0, 0
	for at < len(text) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		next := strings.IndexByte(text[at:], '<')
		if next < 0 {
			break
		}
		line += strings.Count(text[at:at+next], "\n")
		at += next
		startLine := line
		if len(stack) > 0 && html && rawMarkup(stack[len(stack)-1].tag.name) {
			tag, ok := readMarkupTag(text, at)
			if !ok || !tag.closing || foldTagASCII(tag.name) != stack[len(stack)-1].tag.name {
				at++
				continue
			}
		}
		terminator := ""
		switch {
		case strings.HasPrefix(text[at:], "<!--"):
			terminator = "-->"
		case !html && strings.HasPrefix(text[at:], "<![CDATA["):
			terminator = "]]>"
		case strings.HasPrefix(text[at:], "<?"):
			terminator = "?>"
		}
		if terminator != "" {
			n := strings.Index(text[at+2:], terminator)
			if n < 0 {
				break
			}
			end := at + 2 + n + len(terminator)
			line += strings.Count(text[at:end], "\n")
			last := foldEndLine(text, end, line, false)
			if terminator != "?>" && last > startLine {
				out = append(out, FoldRange{startLine, last})
			}
			at = end
			continue
		}
		if strings.HasPrefix(text[at:], "<!") {
			quote, depth, end := byte(0), 0, at+2
			for ; end < len(text); end++ {
				b := text[end]
				if quote != 0 {
					if b == quote {
						quote = 0
					}
					continue
				}
				if b == '"' || b == '\'' {
					quote = b
				} else if b == '[' {
					depth++
				} else if b == ']' {
					depth = max(0, depth-1)
				} else if b == '>' && depth == 0 {
					end++
					break
				}
			}
			line += strings.Count(text[at:end], "\n")
			at = end
			continue
		}
		tag, ok := readMarkupTag(text, at)
		if !ok {
			at++
			continue
		}
		if html {
			tag.name = foldTagASCII(tag.name)
		}
		if len(stack) > 0 && html && rawMarkup(stack[len(stack)-1].tag.name) && (!tag.closing || tag.name != stack[len(stack)-1].tag.name) {
			at++
			continue
		}
		line += strings.Count(text[at:tag.end], "\n")
		at = tag.end
		if tag.self || html && htmlVoid(tag.name) {
			continue
		}
		if !tag.closing {
			stack = append(stack, openTag{tag, startLine})
			continue
		}
		if len(stack) == 0 {
			continue
		}
		open := stack[len(stack)-1]
		if open.tag.name != tag.name {
			stack = nil
			continue
		}
		stack = stack[:len(stack)-1]
		end := foldEndLine(text, tag.end, line, false)
		if end > open.line {
			out = append(out, FoldRange{open.line, end})
		}
	}
	return out, ctx.Err()
}
