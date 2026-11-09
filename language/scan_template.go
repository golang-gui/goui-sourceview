package language

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

// NewGoTemplate creates a template with an explicit plain-text, HTML, XML, YAML
// or TOML host.
// Nil selects plain text. The host's markup tag matching and folding are disabled.
func NewGoTemplate(host *Language) (*Language, error) { return newTemplate(GoTemplate, host) }

// NewJinja2 creates a template using Jinja's default delimiters and an explicit
// plain-text, HTML, XML, YAML or TOML host. Nil selects plain text.
func NewJinja2(host *Language) (*Language, error) { return newTemplate(Jinja2, host) }

func newTemplate(id ID, host *Language) (*Language, error) {
	if host != nil && host.Tokenizer() != nil {
		b, ok := host.Tokenizer().(builtinTokenizer)
		if !ok || b.id != HTML && b.id != XML && b.id != YAML && b.id != TOML {
			return nil, fmt.Errorf("language: template host must use a plain-text, HTML, XML, YAML or TOML tokenizer")
		}
		d := host.Definition()
		d.Editing.Tags, d.Folding = nil, nil
		var err error
		host, err = New(d)
		if err != nil {
			return nil, err
		}
	}
	info, _ := Lookup(id)
	d := Definition{Info: info, Tokenizer: templateTokenizer{id, host}, Editing: defaultEditing(), AllowWordSuggestions: codeWordSuggestions}
	d.Folding = templateFolding(id, d.Tokenizer)
	d.Editing.Word = func(r rune) bool { return IdentifierRune(r) || r == '$' }
	d.Editing.AllowPair = func(c PairContext) bool { return c.Selected || c.Context.Data == true }
	if id == GoTemplate {
		d.Editing.Comments.Block = Pair{Open: "{{/*", Close: "*/}}"}
	} else {
		d.Editing.Comments.Block = Pair{Open: "{#", Close: "#}"}
	}
	d.Suggest = func(c Context) []string {
		if c.Embedded != nil {
			return c.Embedded.Suggestions(c)
		}
		if c.Data == true {
			return slices.Clone(templateWords(id))
		}
		return nil
	}
	return New(d)
}

func templateWords(id ID) []string {
	if id == GoTemplate {
		return goTemplateWords
	}
	return jinjaWords
}

var goTemplateWords = strings.Fields("if else end range with template block define break continue nil true false and or not len index slice printf print println eq ne lt le gt ge call html js urlquery")
var jinjaWords = strings.Fields("if else elif endif for endfor in not and or is true false none True False None block endblock extends include import from as macro endmacro set endset filter endfilter call endcall with endwith autoescape endautoescape raw endraw recursive")

type templateTokenizer struct {
	id   ID
	host *Language
}
type templateState struct {
	id                         ID
	host                       *Language
	body                       builtinState
	mode, quote                byte
	escape, raw, rawEnd, first bool
	rawPrefix                  bool
	depth                      int
	word                       scanWord
	wordOn                     bool
}

func (s *templateState) hostOffset(at int) {
	if s.host == nil || s.host.Tokenizer() == nil {
		return
	}
	if b, ok := s.host.Tokenizer().(builtinTokenizer); ok && b.id == YAML {
		y := yamlState{lineStart: true, keyIndent: -1}
		if s.body.yaml != nil {
			y = *s.body.yaml
		}
		y.offset = at
		s.body.yaml = &y
	}
}

// A host token cannot span a template hole: later host classification must
// never retract template colors. Keep quote/comment/structural state intact.
func (s *templateState) hostBoundary(at int, w *tokenWriter) {
	s.hostOffset(at)
	if s.body.code != nil && s.body.code.wordOn {
		v := *s.body.code
		finishCodeWord(&v, w, at)
		s.body.code = &v
	}
	if s.body.yaml != nil {
		v := *s.body.yaml
		if v.scalarOn {
			w.add(at-v.scalar.n, at, v.scalar.yamlKind(), 0)
		}
		v.scalar, v.scalarOn = atom{}, false
		if v.lineStart {
			v.beginContent('{')
		}
		v.previous = '}'
		s.body.yaml = &v
	}
	if s.body.toml != nil && s.body.toml.valueOn {
		v := *s.body.toml
		w.add(at-v.value.n, at, v.value.tomlKind(), 0)
		v.value, v.valueOn = atom{}, false
		s.body.toml = &v
	}
}

func (s templateState) Equal(other State) bool {
	t, ok := other.(templateState)
	if !ok || !s.body.Equal(t.body) {
		return false
	}
	s.body, t.body = builtinState{}, builtinState{}
	return s == t
}
func (s templateState) Context() Context {
	if s.mode == 0 || s.raw {
		c := s.body.Context()
		if c.Embedded == nil {
			c.Embedded = s.host
		}
		return c
	}
	return Context{String: s.quote != 0, Literal: s.quote == 0x60, Comment: s.mode == '#', Data: true}
}
func (b templateTokenizer) InitialState() State {
	return templateState{id: b.id, host: b.host, body: initialBuiltin()}
}
func (b templateTokenizer) Scan(in Input, state State) (Result, error) {
	return b.scan(in, state, nil)
}

func (b templateTokenizer) scan(in Input, state State, folds *foldLexemes) (Result, error) {
	s, ok := state.(templateState)
	if !ok || s.id != b.id || s.host != b.host {
		return Result{}, fmt.Errorf("language: foreign template state")
	}
	limit, err := scanLimit(in)
	if err != nil {
		return Result{}, err
	}
	w := tokenWriter{base: in.Offset}
	text, i := in.Text, 0
	add := func(a, z int, k Kind, c Class) { w.add(in.Offset+a, in.Offset+z, k, c) }
	finish := func(end int) {
		if !s.wordOn {
			return
		}
		k := s.word.kind
		if slices.Contains(templateWords(s.id), s.word.text) {
			k = KindKeyword
		}
		if s.raw && s.word.text != "endraw" {
			k = KindPlain
		}
		folds.templateWord(s.word.text, k, s.first)
		w.add(s.word.start, end, k, 0)
		if s.first && s.mode == '%' {
			if s.word.text == "raw" && !s.raw {
				s.rawEnd = true
			}
			if s.word.text == "endraw" && s.raw {
				s.rawEnd = true
			}
		}
		s.first = false
		s.word, s.wordOn = scanWord{}, false
	}
	for i < limit {
		c := text[i]
		r, n := utf8.DecodeRuneInString(text[i:])
		if s.mode == 0 {
			open := strings.HasPrefix(text[i:], "{{") || s.id == Jinja2 && (strings.HasPrefix(text[i:], "{%") || strings.HasPrefix(text[i:], "{#"))
			if open && (!s.raw || strings.HasPrefix(text[i:], "{%")) {
				folds.templateOpen(in.Offset+i, text[i+1], s.raw)
				s.hostBoundary(in.Offset+i, &w)
				s.mode, s.first, s.rawEnd = text[i+1], true, false
				s.rawPrefix = false
				add(i, i+2, KindMeta, 0)
				i += 2
				continue
			}
			end := i + n
			for end < len(text) && !(text[end] == '{' && end+1 < len(text) && (text[end+1] == '{' && !s.raw || s.id == Jinja2 && (text[end+1] == '%' || text[end+1] == '#' && !s.raw))) {
				end += runeSize(text, end)
			}
			used := min(end-i, limit-i)
			if s.host != nil && s.host.Tokenizer() != nil {
				s.hostOffset(in.Offset + i)
				// The physical line is finalized once, after all template pieces.
				part := Input{Text: text[i:end], Offset: in.Offset + i, Limit: used}
				result, e := s.host.Tokenizer().Scan(part, s.body)
				if e != nil {
					return Result{}, e
				}
				w.merge(result)
				s.body = result.State.(builtinState)
				used = result.Consumed
			} else {
				for i+used < end && !utf8.RuneStart(text[i+used]) {
					used++
				}
			}
			i += used
			continue
		}
		if s.mode == '#' {
			close := "#}"
			if s.id == GoTemplate {
				close = "*/"
			}
			if strings.HasPrefix(text[i:], close) {
				add(i, i+2, KindComment, ClassComment)
				i += 2
				if s.id == GoTemplate {
					s.mode = '{'
				} else {
					folds.templateClose(s.id, in.Offset+i)
					s.mode = 0
				}
			} else {
				add(i, i+n, KindComment, ClassComment)
				i += n
			}
			continue
		}
		if s.quote != 0 {
			k := KindString
			if s.escape {
				k = KindEscape
				s.escape = false
			} else if c == '\\' && s.quote != 0x60 {
				s.escape = true
				k = KindEscape
			} else if c == s.quote {
				s.quote = 0
			}
			add(i, i+n, k, ClassString)
			i += n
			continue
		}
		if s.wordOn {
			if IdentifierRune(r) || c == '$' {
				s.word.add(text[i : i+n])
				i += n
				continue
			}
			finish(in.Offset + i)
			continue
		}
		close := "}}"
		if s.mode == '%' {
			close = "%}"
		}
		if s.depth == 0 && strings.HasPrefix(text[i:], close) {
			folds.templateClose(s.id, in.Offset+i+2)
			add(i, i+2, KindMeta, 0)
			i += 2
			s.mode = 0
			if s.rawEnd {
				s.raw = !s.raw
			}
			continue
		}
		if s.raw {
			if IdentifierRune(r) {
				s.word = scanWord{start: in.Offset + i}
				s.wordOn = true
				s.word.add(text[i : i+n])
				i += n
				continue
			}
			trim := s.first && !s.rawPrefix && (c == '-' || c == '+')
			if !space(c) {
				folds.templateSymbol(c, in.Offset+i, trim)
				if trim {
					s.rawPrefix = true
				} else {
					s.first = false
				}
			}
			i += n
			continue
		}
		if s.id == GoTemplate && strings.HasPrefix(text[i:], "/*") {
			if folds != nil && folds.template != nil && !folds.template.action.seen {
				folds.template.action.comment = true
			}
			s.mode = '#'
			add(i, i+2, KindComment, ClassComment)
			i += 2
			continue
		}
		if c == '"' || c == '\'' || s.id == GoTemplate && c == 0x60 {
			folds.templateSymbol(c, in.Offset+i, false)
			s.quote = c
			add(i, i+1, KindString, ClassString)
			i++
			continue
		}
		if IdentifierRune(r) || c == '$' {
			k := KindPlain
			if digit(c) {
				k = KindNumber
			} else if c == '$' {
				k = KindVariable
			}
			s.word = scanWord{start: in.Offset + i, kind: k}
			s.wordOn = true
			s.word.add(text[i : i+n])
			i += n
			continue
		}
		if strings.ContainsRune("([{", r) {
			s.depth++
		}
		if strings.ContainsRune(")]}", r) && s.depth > 0 {
			s.depth--
		}
		if !space(c) {
			trim := (c == '-' || s.id == Jinja2 && c == '+') && (strings.HasPrefix(text[i+1:], close) || s.id == Jinja2 || i+1 < len(text) && space(text[i+1]))
			folds.templateSymbol(c, in.Offset+i, trim)
			class := Class(0)
			if strings.ContainsRune("()[]{}", r) {
				class = ClassDelimiter
			}
			add(i, i+n, KindDelimiter, class)
		}
		i += n
	}
	if in.LineEnd && i == len(text) {
		finish(in.Offset + i)
		if s.host != nil && s.host.Tokenizer() != nil {
			s.hostOffset(in.Offset + i)
			end := Input{Offset: in.Offset + i, LineEnd: true, DocumentEnd: in.DocumentEnd}
			r, e := s.host.Tokenizer().Scan(end, s.body)
			if e != nil {
				return Result{}, e
			}
			w.merge(r)
			s.body = r.State.(builtinState)
		}
	}
	return w.result(i, s), nil
}
