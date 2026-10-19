package language

import (
	"fmt"
	"strings"
	"sync"
	"unicode/utf8"
)

var builtinLanguages = sync.OnceValue(func() map[ID]*Language {
	out := make(map[ID]*Language)
	for _, info := range builtins {
		d := Definition{Info: info, Editing: defaultEditing()}
		if info.ID != Plain {
			d.Tokenizer = builtinTokenizer{info.ID}
		}
		switch info.ID {
		case JSON:
			d.Editing.AutoClosingPairs = []Pair{{Open: "[", Close: "]", NotInString: true, NotInComment: true}, {Open: "{", Close: "}", NotInString: true, NotInComment: true}, {Open: `"`, Close: `"`, NotInString: true, NotInComment: true}}
			d.Editing.SurroundingPairs = d.Editing.AutoClosingPairs
			d.Editing.Brackets = d.Editing.Brackets[1:]
			d.Suggest = func(Context) []string { return []string{"false", "null", "true"} }
		case XML, HTML:
			d.Editing.Word = func(r rune) bool { return IdentifierRune(r) || strings.ContainsRune("-:.", r) }
			d.Editing.Brackets = append(d.Editing.Brackets, Pair{Open: "<", Close: ">"})
			p := Pair{Open: "<", Close: ">", NotInString: true, NotInComment: true}
			d.Editing.AutoClosingPairs = append(d.Editing.AutoClosingPairs, p)
			d.Editing.SurroundingPairs = append(d.Editing.SurroundingPairs, p)
			d.Editing.AllowPair = func(c PairContext) bool {
				if c.Selected {
					return true
				}
				if c.Typed == `"` || c.Typed == "'" {
					return c.Context.Value
				}
				return c.Typed != "<" || !c.Context.Tag
			}
			d.Editing.Comments.Block = Pair{Open: "<!--", Close: "-->"}
			if info.ID == HTML {
				d.Suggest = func(c Context) []string {
					if !c.Tag {
						return nil
					}
					if !c.TagName {
						return []string{"alt", "class", "disabled", "href", "id", "lang", "name", "rel", "role", "src", "style", "title", "type", "value"}
					}
					return []string{"a", "article", "body", "button", "div", "footer", "form", "h1", "head", "header", "html", "img", "input", "label", "li", "link", "main", "meta", "nav", "p", "script", "section", "span", "style", "table", "tbody", "td", "textarea", "th", "thead", "title", "tr", "ul"}
				}
			} else {
				d.Suggest = func(c Context) []string {
					if c.Tag && !c.TagName {
						return []string{"xmlns", "xml:lang", "xml:space"}
					}
					return nil
				}
			}
		case TOML, YAML:
			d.AllowWordSuggestions = codeWordSuggestions
			d.Editing.Word = func(r rune) bool { return IdentifierRune(r) || r == '-' }
			d.Editing.Comments.Line = "#"
			id := info.ID
			d.Editing.AllowPair = func(c PairContext) bool {
				if c.Selected || c.Context.String {
					return true
				}
				if id == TOML && (c.Typed == `"` || c.Typed == "'") {
					return !strings.HasSuffix(c.Before, c.Typed+c.Typed)
				}
				if id == YAML && c.Typed == "'" {
					return !strings.HasSuffix(c.Before, "'")
				}
				return true
			}
			if info.ID == TOML {
				d.Suggest = func(c Context) []string {
					if c.Value {
						return []string{"false", "true", "inf", "nan"}
					}
					return nil
				}
			} else {
				d.Suggest = func(Context) []string { return []string{"false", "null", "true"} }
			}
		case Shell:
			d.AllowWordSuggestions = codeWordSuggestions
			d.Editing.Comments.Line = "#"
			d.Suggest = func(Context) []string {
				return []string{"if", "then", "else", "elif", "fi", "for", "while", "until", "do", "done", "case", "esac", "in", "function", "select", "time", "coproc", "echo", "printf", "export", "readonly", "local", "return", "exit", "unset", "set", "test", "source", "read", "trap", "shift", "getopts"}
			}
		}
		l, err := New(d)
		if err != nil {
			panic(err)
		}
		out[info.ID] = l
	}
	return out
})

// Builtin returns an immutable built-in definition. Custom definitions are
// passed directly to a document; no registration or name matching is needed.
func Builtin(id ID) (*Language, bool) { l, ok := builtinLanguages()[id]; return l, ok }
func defaultEditing() EditingRules {
	brackets := []Pair{{Open: "(", Close: ")", NotInString: true, NotInComment: true}, {Open: "[", Close: "]", NotInString: true, NotInComment: true}, {Open: "{", Close: "}", NotInString: true, NotInComment: true}}
	pairs := append(append([]Pair(nil), brackets...), Pair{Open: `"`, Close: `"`, NotInString: true, NotInComment: true}, Pair{Open: "'", Close: "'", NotInString: true, NotInComment: true})
	return EditingRules{Word: IdentifierRune, AutoClosingPairs: pairs, SurroundingPairs: append([]Pair(nil), pairs...), Brackets: brackets, AutoCloseBefore: ")]},:;>"}
}

type builtinTokenizer struct{ id ID }

func (b builtinTokenizer) InitialState() State { return initialBuiltin() }
func (b builtinTokenizer) Scan(in Input, state State) (Result, error) {
	s, ok := state.(builtinState)
	if !ok {
		return Result{}, fmt.Errorf("language: foreign state for %s", b.id)
	}
	if in.Limit < 0 || in.Limit > len(in.Text) || in.Offset < 0 {
		return Result{}, fmt.Errorf("language: invalid input")
	}
	s.pendingTag = false
	limit := in.Limit
	for limit < len(in.Text) && !utf8.RuneStart(in.Text[limit]) {
		limit++
	}
	raw, next, n := scanBuiltin(b.id, s, in.Text, limit, in.LineEnd)
	if (b.id == XML || b.id == HTML) && next.mode == data && n == in.Limit && n > 0 && in.Text[n-1] == '<' && (n == len(in.Text) || in.Text[n] == '>') && !(in.LineEnd && n == len(in.Text)) {
		next.pendingTag = true
	}
	result := Result{Consumed: n, State: next}
	for _, t := range raw {
		tok := Token{Start: in.Offset + t.Start, End: in.Offset + t.End, Kind: t.Kind}
		switch t.Kind {
		case KindString, KindEscape, KindEntity:
			tok.Class = ClassString
		case KindComment:
			tok.Class = ClassComment
		case KindDelimiter:
			tok.Class = ClassDelimiter
		}
		if tok.Start < in.Offset && (result.ReplaceFrom == nil || tok.Start < *result.ReplaceFrom) {
			start := tok.Start
			result.ReplaceFrom = &start
		}
		result.Tokens = append(result.Tokens, tok)
	}
	return result, nil
}

func codeWordSuggestions(c Context, known bool) bool { return known && !c.Comment && !c.String }
