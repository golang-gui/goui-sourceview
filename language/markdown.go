package language

import (
	"context"
	"fmt"
	"maps"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	KindHeading  Kind = "markdown.heading"
	KindCode     Kind = "markdown.code"
	KindEmphasis Kind = "markdown.emphasis"
	KindLink     Kind = "markdown.link"
	KindFence    Kind = "markdown.fence"
	KindMarker   Kind = "markdown.marker"
)

// NewMarkdown creates a Markdown profile with an explicit fence-name table.
// The map is copied. A nil table uses built-in markup/configuration languages;
// an empty non-nil table disables embedded languages. No global registration.
func NewMarkdown(embedded map[string]*Language) (*Language, error) {
	if embedded == nil {
		embedded = map[string]*Language{}
		for _, id := range []ID{JSON, XML, HTML, TOML, YAML, Shell} {
			l, _ := Builtin(id)
			embedded[string(id)] = l
		}
		embedded["sh"] = embedded["shell"]
		embedded["bash"] = embedded["shell"]
		embedded["yml"] = embedded["yaml"]
	}
	return newMarkdown(embedded)
}
func newMarkdown(embedded map[string]*Language) (*Language, error) {
	table := maps.Clone(embedded)
	for key, l := range table {
		if key == "" || key != strings.ToLower(key) || strings.ContainsAny(key, " \t\r\n") || l == nil {
			return nil, fmt.Errorf("language: invalid Markdown fence %q", key)
		}
	}
	info, _ := Lookup(Markdown)
	rules := defaultEditing()
	rules.Word = func(r rune) bool { return IdentifierRune(r) || r == '-' }
	rules.Newline = markdownNewline
	rules.Comments = Comments{Block: Pair{Open: "<!--", Close: "-->"}}
	rules.AutoClosingPairs = []Pair{{Open: "[", Close: "]", NotInString: true, NotInComment: true}, {Open: "(", Close: ")", NotInString: true, NotInComment: true}, {Open: "`", Close: "`", NotInString: true, NotInComment: true}, {Open: "**", Close: "**", NotInString: true, NotInComment: true}}
	rules.SurroundingPairs = rules.AutoClosingPairs
	styles := []Kind{KindHeading, KindCode, KindEmphasis, KindLink, KindFence, KindMarker}
	for _, l := range table {
		styles = append(styles, l.Styles()...)
	}
	return New(Definition{Info: info, Tokenizer: markdownTokenizer{table}, Folding: foldMarkdown, Editing: rules, Styles: styles})
}

type mdBlock struct {
	fence    byte
	length   int
	language *Language
	child    State
	comment  bool
}

func (a mdBlock) equal(b mdBlock) bool {
	return a.fence == b.fence && a.length == b.length && a.language == b.language && a.comment == b.comment && StatesEqual(a.child, b.child)
}

type markdownState struct {
	block, base mdBlock
	prefix      string
	overflow    bool
	context     Context
}

func (s markdownState) Equal(other State) bool {
	t, ok := other.(markdownState)
	return ok && s.block.equal(t.block) && s.base.equal(t.base) && s.prefix == t.prefix && s.overflow == t.overflow
}
func (s markdownState) Context() Context { return s.context }

type markdownTokenizer struct{ embedded map[string]*Language }

func (markdownTokenizer) InitialState() State { return markdownState{} }
func (t markdownTokenizer) Scan(in Input, previous State) (Result, error) {
	s, ok := previous.(markdownState)
	if !ok {
		return Result{}, fmt.Errorf("language: foreign Markdown state")
	}
	n := in.Limit
	for n < len(in.Text) && !utf8.RuneStart(in.Text[n]) {
		n++
	}
	if in.Offset == 0 {
		s.prefix = ""
		s.base = s.block
		s.overflow = false
	}
	// Bounded current-line replay permits delimiter decisions and exact context
	// at partial caret positions without mutating published states.
	if s.overflow || len(s.prefix)+n > 64<<10 {
		s.overflow = true
		s.prefix = ""
		kind := KindPlain
		if s.base.fence != 0 {
			kind = KindCode
		}
		s.context = Context{String: s.base.fence != 0, Literal: s.base.fence != 0}
		result := Result{Consumed: n, State: s}
		if n > 0 && kind != KindPlain {
			result.Tokens = []Token{{Start: in.Offset, End: in.Offset + n, Kind: kind, Class: ClassString}}
		}
		if in.LineEnd && n == len(in.Text) {
			s.block = s.base
			if s.block.fence != 0 {
				s.block.language = nil
				s.block.child = nil
			}
			s.base = mdBlock{}
			s.context = Context{}
			s.overflow = false
			result.State = s
		}
		return result, nil
	}
	s.prefix += in.Text[:n]
	end := in.LineEnd && n == len(in.Text)
	block, tokens, ctx, err := t.line(s.prefix, s.base, end, in.DocumentEnd)
	if err != nil {
		return Result{}, err
	}
	s.block, s.context = block, ctx
	zero := 0
	result := Result{Consumed: n, State: s, Tokens: tokens, ReplaceFrom: &zero}
	if end {
		s.prefix = ""
		s.base = mdBlock{}
		result.State = s
	}
	return result, nil
}
func fencePrefix(line string) (char byte, width int, rest string) {
	text := strings.TrimLeft(line, " ")
	if len(line)-len(text) > 3 || len(text) < 3 || text[0] != '`' && text[0] != '~' {
		return
	}
	char = text[0]
	width = len(text) - len(strings.TrimLeft(text, string(char)))
	if width < 3 {
		return 0, 0, ""
	}
	rest = text[width:]
	return
}
func (t markdownTokenizer) line(text string, b mdBlock, end, documentEnd bool) (mdBlock, []Token, Context, error) {
	char, width, rest := fencePrefix(text)
	if b.fence != 0 {
		if char == b.fence && width >= b.length && strings.TrimSpace(rest) == "" {
			tokens := mdToken(text, KindFence, ClassString)
			if end {
				b = mdBlock{}
			}
			return b, tokens, Context{String: true, Literal: true}, nil
		}
		if b.language != nil && b.language.Tokenizer() != nil {
			result, err := b.language.Tokenizer().Scan(Input{Text: text, Limit: len(text), LineEnd: end, DocumentEnd: documentEnd}, b.child)
			if err != nil {
				return b, nil, Context{}, err
			}
			if err = ValidateResult(Input{Text: text, Limit: len(text), LineEnd: end, DocumentEnd: documentEnd}, result); err != nil {
				return b, nil, Context{}, err
			}
			b.child = result.State
			ctx := result.State.Context()
			ctx.Embedded = b.language
			// Paint plain child text as code, retaining all lexical token boundaries.
			tokens := []Token{}
			at := 0
			for _, tok := range result.Tokens {
				if at < tok.Start {
					tokens = append(tokens, Token{Start: at, End: tok.Start, Kind: KindCode})
				}
				tokens = append(tokens, tok)
				at = tok.End
			}
			if at < len(text) {
				tokens = append(tokens, Token{Start: at, End: len(text), Kind: KindCode})
			}
			return b, tokens, ctx, nil
		}
		return b, mdToken(text, KindCode, ClassString), Context{String: true, Literal: true}, nil
	}
	if !b.comment && char != 0 && !(char == '`' && strings.ContainsRune(rest, '`')) {
		b.fence, b.length = char, width
		fields := strings.Fields(rest)
		if len(fields) > 0 {
			b.language = t.embedded[strings.ToLower(fields[0])]
			if b.language != nil && b.language.Tokenizer() != nil {
				b.child = b.language.Tokenizer().InitialState()
			}
		}
		return b, mdToken(text, KindFence, ClassString), Context{String: true, Literal: true}, nil
	}
	trimmed := strings.TrimLeft(text, " ")
	indent := len(text) - len(trimmed)
	if !b.comment && (indent >= 4 || strings.HasPrefix(text, "\t")) {
		return b, mdToken(text, KindCode, ClassString), Context{String: true, Literal: true}, nil
	}
	base := KindPlain
	if headingLevel(text) > 0 || setextLevel(text) > 0 {
		base = KindHeading
	}
	var out []Token
	ctx := Context{}
	emit := func(a, z int, k Kind, c Class) {
		if a < z && k != KindPlain {
			out = append(out, Token{Start: a, End: z, Kind: k, Class: c})
		}
	}
	for at := 0; at < len(text); {
		start := at
		if b.comment || strings.HasPrefix(text[at:], "<!--") {
			skip := 0
			if !b.comment {
				skip = 4
			}
			n := strings.Index(text[at+skip:], "-->")
			if n < 0 {
				at = len(text)
				b.comment = true
			} else {
				at += skip + n + 3
				b.comment = false
			}
			emit(start, at, KindComment, ClassComment)
			ctx = Context{Comment: b.comment}
			continue
		}
		if text[at] == '\\' && at+1 < len(text) {
			_, n := utf8.DecodeRuneInString(text[at+1:])
			at += 1 + n
			emit(start, at, KindEscape, 0)
			continue
		}
		if text[at] == '`' {
			width := len(text[at:]) - len(strings.TrimLeft(text[at:], "`"))
			marker := text[at : at+width]
			at += width
			close := -1
			for seek := at; seek < len(text); {
				n := strings.Index(text[seek:], marker)
				if n < 0 {
					break
				}
				pos := seek + n
				run := len(text[pos:]) - len(strings.TrimLeft(text[pos:], "`"))
				if run == width {
					close = pos
					break
				}
				seek = pos + run
			}
			if close >= 0 {
				at = close + width
			} else {
				at = len(text)
			}
			emit(start, at, KindCode, ClassString)
			ctx = Context{String: close < 0, Literal: close < 0}
			continue
		}
		if text[at] == '&' {
			if n := strings.IndexByte(text[at:], ';'); n > 1 && n < 32 {
				at += n + 1
				emit(start, at, KindEntity, 0)
				continue
			}
		}
		if strings.ContainsRune("*_~", rune(text[at])) {
			c := text[at]
			run := len(text[at:]) - len(strings.TrimLeft(text[at:], string(c)))
			if c != '~' || run >= 2 {
				marker := text[at : at+run]
				if n := strings.Index(text[at+run:], marker); n > 0 {
					at += run + n + run
					emit(start, at, KindEmphasis, 0)
					continue
				}
			}
		}
		if text[at] == '[' || text[at] == '!' && strings.HasPrefix(text[at:], "![") {
			n := strings.IndexByte(text[at:], ']')
			if n >= 0 {
				at += n + 1
				if at < len(text) && (text[at] == '(' || text[at] == '[') {
					close := byte(')')
					if text[at] == '[' {
						close = ']'
					}
					if n := strings.IndexByte(text[at+1:], close); n >= 0 {
						at += n + 2
					}
				} else if at < len(text) && text[at] == ':' {
					at = len(text)
				}
				emit(start, at, KindLink, 0)
				continue
			}
		}
		if strings.HasPrefix(text[at:], "https://") || strings.HasPrefix(text[at:], "http://") || strings.HasPrefix(text[at:], "<https://") {
			for at < len(text) && !strings.ContainsRune(" \t>", rune(text[at])) {
				at++
			}
			if at < len(text) && text[at] == '>' {
				at++
			}
			emit(start, at, KindLink, 0)
			continue
		}
		k := base
		if k == KindPlain && strings.ContainsRune(">#-+*|", rune(text[at])) {
			k = KindMarker
		}
		_, n := utf8.DecodeRuneInString(text[at:])
		at += n
		emit(start, at, k, 0)
	}
	return b, out, ctx, nil
}
func mdToken(text string, kind Kind, class Class) []Token {
	if text == "" {
		return nil
	}
	return []Token{{End: len(text), Kind: kind, Class: class}}
}
func headingLevel(text string) int {
	trim := strings.TrimLeft(text, " ")
	if len(text)-len(trim) > 3 {
		return 0
	}
	n := len(trim) - len(strings.TrimLeft(trim, "#"))
	if n < 1 || n > 6 || n < len(trim) && trim[n] != ' ' && trim[n] != '\t' {
		return 0
	}
	return n
}
func setextLevel(text string) int {
	trim := strings.TrimSpace(text)
	if trim == "" || len(text)-len(strings.TrimLeft(text, " ")) > 3 {
		return 0
	}
	if strings.Trim(trim, "=") == "" {
		return 1
	}
	if strings.Trim(trim, "-") == "" {
		return 2
	}
	return 0
}
func foldMarkdown(ctx context.Context, text string) ([]FoldRange, error) {
	lines := strings.Split(text, "\n")
	type header struct{ line, level int }
	var headings []header
	var out []FoldRange
	fence, length, start := byte(0), 0, 0
	for row, line := range lines {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		c, n, rest := fencePrefix(line)
		if fence != 0 {
			if c == fence && n >= length && strings.TrimSpace(rest) == "" {
				if row > start {
					out = append(out, FoldRange{start, row})
				}
				fence = 0
			}
			continue
		}
		if c != 0 && !(c == '`' && strings.ContainsRune(rest, '`')) {
			fence, length, start = c, n, row
			continue
		}
		level := headingLevel(line)
		at := row
		if level == 0 && row > 0 && strings.TrimSpace(lines[row-1]) != "" {
			level = setextLevel(line)
			at = row - 1
		}
		if level == 0 {
			continue
		}
		for len(headings) > 0 && headings[len(headings)-1].level >= level {
			h := headings[len(headings)-1]
			headings = headings[:len(headings)-1]
			if at-1 > h.line {
				out = append(out, FoldRange{h.line, at - 1})
			}
		}
		headings = append(headings, header{at, level})
	}
	last := len(lines) - 1
	if last > 0 && lines[last] == "" {
		last--
	}
	if fence != 0 && last > start {
		out = append(out, FoldRange{start, last})
	}
	for _, h := range headings {
		if last > h.line {
			out = append(out, FoldRange{h.line, last})
		}
	}
	return out, nil
}

var markdownList = regexp.MustCompile(`^([ \t]*(?:> ?)*)([-+*]|[0-9]{1,9}[.)])( \[[ xX]\])?[ \t]+`)

func markdownNewline(c NewlineContext) (NewlineEdit, bool) {
	if !c.Known || c.Context.String || c.Context.Comment {
		return NewlineEdit{}, false
	}
	if m := markdownList.FindStringSubmatch(c.Before); m != nil {
		if len(m[0]) == len(c.Before) {
			return NewlineEdit{DeleteBefore: len(m[0]) - len(m[1]), Text: "\n" + m[1], Caret: 1 + len(m[1])}, true
		}
		marker := m[2]
		last := marker[len(marker)-1]
		if last == '.' || last == ')' {
			n, _ := strconv.Atoi(marker[:len(marker)-1])
			marker = strconv.Itoa(n+1) + string(last)
		}
		text := "\n" + m[1] + marker
		if m[3] != "" {
			text += " [ ]"
		}
		text += " "
		return NewlineEdit{Text: text, Caret: len(text)}, true
	}
	prefix := strings.TrimLeft(c.Before, " \t")
	if strings.HasPrefix(prefix, "> ") || prefix == ">" {
		base := c.Before[:len(c.Before)-len(prefix)]
		if prefix == "> " || prefix == ">" {
			return NewlineEdit{DeleteBefore: len(prefix), Text: "\n" + base, Caret: 1 + len(base)}, true
		}
		end := 0
		for end < len(prefix) && prefix[end] == '>' {
			end++
			if end < len(prefix) && prefix[end] == ' ' {
				end++
			}
		}
		text := "\n" + base + prefix[:end]
		return NewlineEdit{Text: text, Caret: len(text)}, true
	}
	return NewlineEdit{}, false
}
