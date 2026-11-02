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
// The map is copied. A nil table uses the built-in languages and fence aliases;
// an empty non-nil table disables embedded languages. No global registration.
func NewMarkdown(embedded map[string]*Language) (*Language, error) {
	if embedded == nil {
		embedded = builtinFences(builtinLanguages())
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
	rules.AutoClosingPairs = append(rules.AutoClosingPairs, Pair{Open: "```", Close: "```", NotInComment: true})
	rules.AllowPair = func(c PairContext) bool {
		if c.Selected {
			return true
		}
		text := c.Before + c.Typed
		if strings.HasSuffix(text, "```") {
			trim := strings.TrimLeft(text, " ")
			return len(text)-len(trim) <= 3 && trim == "```" && strings.TrimSpace(c.After) == ""
		}
		return true
	}
	rules.SurroundingPairs = rules.AutoClosingPairs
	styles := []Kind{KindHeading, KindCode, KindEmphasis, KindLink, KindFence, KindMarker}
	for _, l := range table {
		styles = append(styles, l.Styles()...)
	}
	return New(Definition{Info: info, Tokenizer: markdownTokenizer{table}, Folding: foldMarkdown, Editing: rules, Styles: styles})
}

type mdBlock struct {
	frontMatter bool
	fence       byte
	length      int
	language    *Language
	child       State
	comment     bool
}

func (a mdBlock) equal(b mdBlock) bool {
	return a.frontMatter == b.frontMatter && a.fence == b.fence && a.length == b.length && a.language == b.language && a.comment == b.comment && StatesEqual(a.child, b.child)
}

type markdownState struct {
	first, frontCandidate bool
	frontMarker           byte
	frontRun              int
	block, base           mdBlock
	prefix                string
	overflow              bool
	context               Context
}

func (s markdownState) Equal(other State) bool {
	t, ok := other.(markdownState)
	return ok && s.first == t.first && s.frontCandidate == t.frontCandidate && s.frontMarker == t.frontMarker && s.frontRun == t.frontRun && s.block.equal(t.block) && s.base.equal(t.base) && s.prefix == t.prefix && s.overflow == t.overflow
}
func (s markdownState) Context() Context { return s.context }

type markdownTokenizer struct{ embedded map[string]*Language }

func (markdownTokenizer) InitialState() State { return markdownState{first: true} }
func (t markdownTokenizer) Scan(in Input, previous State) (Result, error) {
	s, ok := previous.(markdownState)
	if !ok {
		return Result{}, fmt.Errorf("language: foreign Markdown state")
	}
	n, err := scanLimit(in)
	if err != nil {
		return Result{}, err
	}
	if s.block.frontMatter {
		return t.frontMatter(in, s)
	}
	if s.first {
		if in.Offset == 0 {
			s.frontCandidate, s.frontMarker, s.frontRun = true, 0, 0
		}
		s.frontPrefix(in.Text[:n], in.Offset, true)
		if in.LineEnd && n == len(in.Text) && s.frontCandidate && s.frontRun == 3 {
			name := "yaml"
			if s.frontMarker == '+' {
				name = "toml"
			}
			s.block = mdBlock{frontMatter: true, fence: s.frontMarker, language: t.embedded[name]}
			if s.block.language != nil && s.block.language.Tokenizer() != nil {
				s.block.child = s.block.language.Tokenizer().InitialState()
			}
			s.context = Context{Embedded: s.block.language}
			s.first, s.frontCandidate, s.frontMarker, s.frontRun = false, false, 0, 0
			s.base, s.prefix, s.overflow = mdBlock{}, "", false
			zero := 0
			return Result{Consumed: n, State: s, Tokens: []Token{{Start: 0, End: in.Offset + n, Kind: KindFence}}, ReplaceFrom: &zero}, nil
		}
	}
	if in.Offset == 0 {
		s.prefix = ""
		s.base = s.block
		s.overflow = false
	}
	// Bounded current-line replay permits delimiter decisions and exact context
	// at partial caret positions without mutating published states.

	if s.overflow || len(s.prefix)+n > 64<<10 {
		tail := s.prefix + in.Text[:n]
		if !s.overflow {
			s.block.comment = s.base.comment
		}
		s.overflow = true
		consumedMarker := 0
		kind, class := KindPlain, Class(0)
		if s.block.fence != 0 {
			kind, class = KindCode, ClassString
			s.block.language = nil
			s.block.child = nil
		} else {
			wasComment := s.block.comment
			for at := 0; at < len(tail); {
				marker := "<!--"
				if s.block.comment {
					marker = "-->"
				}
				n := strings.Index(tail[at:], marker)
				if n < 0 {
					break
				}
				s.block.comment = !s.block.comment
				at += n + len(marker)
				consumedMarker = at
			}
			if wasComment || s.block.comment {
				kind, class = KindComment, ClassComment
			}
		}
		s.prefix = tail[max(consumedMarker, len(tail)-3):]
		s.context = Context{String: s.block.fence != 0, Literal: s.block.fence != 0, Comment: s.block.comment}
		result := Result{Consumed: n, State: s}
		if n > 0 && kind != KindPlain {
			result.Tokens = []Token{{Start: in.Offset, End: in.Offset + n, Kind: kind, Class: class}}
		}
		if in.LineEnd && n == len(in.Text) {
			s.first = false
			s.frontCandidate, s.frontMarker, s.frontRun = false, 0, 0
			s.base = mdBlock{}
			s.prefix = ""
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
		s.first = false
		s.frontCandidate, s.frontMarker, s.frontRun = false, 0, 0
		s.prefix = ""
		s.base = mdBlock{}
		result.State = s
	}
	return result, nil
}

// Metadata is streamed directly into its child tokenizer. Closing candidates
// are tracked independently, so even a very long line never needs replay.
func (t markdownTokenizer) frontMatter(in Input, s markdownState) (Result, error) {
	if in.Offset == 0 {
		s.frontCandidate, s.frontMarker, s.frontRun = true, 0, 0
	}
	n, err := scanLimit(in)
	if err != nil {
		return Result{}, err
	}
	w := tokenWriter{base: in.Offset}
	s.context = Context{String: true, Literal: true}
	if s.block.language != nil && s.block.language.Tokenizer() != nil {
		r, err := s.block.language.Tokenizer().Scan(in, s.block.child)
		if err != nil {
			return Result{}, err
		}
		if err = ValidateResult(in, r); err != nil {
			return Result{}, err
		}
		n, s.block.child = r.Consumed, r.State
		w.merge(r)
		s.context = r.State.Context()
		s.context.Embedded = s.block.language
	}
	s.frontPrefix(in.Text[:n], in.Offset, false)
	if in.LineEnd && n == len(in.Text) {
		if s.frontCandidate && s.frontRun == 3 {
			w.add(0, in.Offset+n, KindFence, 0)
			s.block, s.context = mdBlock{}, Context{}
		}
		s.frontCandidate, s.frontMarker, s.frontRun = false, 0, 0
	}
	return w.result(n, s), nil
}

func (s *markdownState) frontPrefix(text string, offset int, opening bool) {
	for i, c := range text {
		if !s.frontCandidate {
			break
		}
		if opening && offset+i == 0 && c == '\ufeff' {
			continue
		}
		if s.frontRun == 0 {
			valid := c == rune(s.block.fence) || s.block.fence == '-' && c == '.'
			if opening {
				valid = c == '-' || c == '+'
			}
			if !valid {
				s.frontCandidate = false
				break
			}
			s.frontMarker = byte(c)
		}
		if s.frontRun < 3 {
			if c != rune(s.frontMarker) {
				s.frontCandidate = false
				break
			}
			s.frontRun++
		} else if c != ' ' && c != '\t' && c != '\r' {
			s.frontCandidate = false
		}
	}
}

func frontMatterMarker(text string) byte {
	switch strings.TrimRight(text, " \t\r") {
	case "---":
		return '-'
	case "+++":
		return '+'
	}
	return 0
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
	comment := false
	front := byte(0)
	for row, line := range lines {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if row == 0 {
			front = frontMatterMarker(strings.TrimPrefix(line, "\ufeff"))
			if front != 0 {
				continue
			}
		}
		if front != 0 {
			trim := strings.TrimRight(line, " \t\r")
			if trim == strings.Repeat(string(front), 3) || front == '-' && trim == "..." {
				out = append(out, FoldRange{0, row})
				front = 0
			}
			continue
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

		if comment {
			if strings.Contains(line, "-->") {
				comment = false
			}
			continue
		}
		if strings.Contains(line, "<!--") {
			after := line[strings.Index(line, "<!--")+4:]
			comment = !strings.Contains(after, "-->")
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
	if front != 0 && last > 0 {
		out = append(out, FoldRange{0, last})
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
	if c.Known && !c.Context.Comment {
		char, width, _ := fencePrefix(c.Before)
		if char != 0 && strings.HasPrefix(c.After, strings.Repeat(string(char), width)) {
			text := "\n" + c.Indent + "\n" + c.Indent
			return NewlineEdit{Text: text, Caret: 1 + len(c.Indent)}, true
		}
	}

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
