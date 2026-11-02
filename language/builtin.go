package language

import (
	"context"
	"fmt"
	"slices"
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
		case GraphQL:
			d.Tokenizer = gqlTokenizer{}
			d.Editing.Comments.Line = "#"
			d.Editing.AutoClosingPairs = append(slices.Clone(d.Editing.Brackets), Pair{Open: `"`, Close: `"`, NotInString: true, NotInComment: true})
			d.Editing.SurroundingPairs = slices.Clone(d.Editing.AutoClosingPairs)
			d.Editing.Word = func(r rune) bool { return gqlName(r) || r == '$' || r == '@' }
			d.Editing.AllowPair = func(c PairContext) bool {
				return c.Selected || c.Context.String || c.Typed != `"` || !strings.HasSuffix(c.Before, `""`)
			}
			d.AllowWordSuggestions = codeWordSuggestions
			vocabulary := codeVocabulary(info.ID)
			d.Suggest = func(Context) []string { return vocabulary }
		case HCL:
			d.Tokenizer = hcTokenizer{}
			d.Editing.Comments = Comments{Line: "#", Block: Pair{Open: "/*", Close: "*/"}}
			d.Editing.AutoClosingPairs = append(slices.Clone(d.Editing.Brackets), Pair{Open: `"`, Close: `"`, NotInString: true, NotInComment: true})
			d.Editing.SurroundingPairs = slices.Clone(d.Editing.AutoClosingPairs)
			d.Editing.Word = func(r rune) bool { return IdentifierRune(r) || r == '-' }
			d.AllowWordSuggestions = codeWordSuggestions
			vocabulary := codeVocabulary(info.ID)
			d.Suggest = func(Context) []string { return vocabulary }
		case Batch:
			d.Tokenizer = batTokenizer{}
			d.Editing.Comments.Line = "REM"
			d.Editing.QuoteEscape = '^'
			d.Editing.Brackets = d.Editing.Brackets[:1]
			d.Editing.AutoClosingPairs = []Pair{d.Editing.Brackets[0], {Open: `"`, Close: `"`, NotInString: true, NotInComment: true}}
			d.Editing.SurroundingPairs = d.Editing.AutoClosingPairs
			d.Editing.Word = func(r rune) bool { return IdentifierRune(r) || r == '%' || r == '!' }
			d.AllowWordSuggestions = codeWordSuggestions
			vocabulary := codeVocabulary(info.ID)
			d.Suggest = func(Context) []string { return vocabulary }
		case Diff:
			d.Tokenizer = diffTokenizer{}
			d.Editing = EditingRules{Word: IdentifierRune}
			d.Styles = []Kind{KindAdded, KindRemoved}
			d.AllowWordSuggestions = func(Context, bool) bool { return false }
		case Markdown:
			l, err := newMarkdown(builtinFences(out))
			if err != nil {
				panic(err)
			}
			out[info.ID] = l
			continue
		case JSON, JSONC, JSONLines:
			if info.ID == JSON {
				d.Folding = foldJSON
			} else if info.ID == JSONC {
				d.Editing.Comments = Comments{Line: "//", Block: Pair{Open: "/*", Close: "*/"}}
			}
			d.Editing.AutoClosingPairs = []Pair{{Open: "[", Close: "]", NotInString: true, NotInComment: true}, {Open: "{", Close: "}", NotInString: true, NotInComment: true}, {Open: `"`, Close: `"`, NotInString: true, NotInComment: true}}
			d.Editing.SurroundingPairs = d.Editing.AutoClosingPairs
			d.Editing.Brackets = d.Editing.Brackets[1:]
			d.Suggest = func(Context) []string { return []string{"false", "null", "true"} }
		case XML, HTML:
			html := info.ID == HTML
			d.Editing.Tags = XMLTags()
			if html {
				d.Editing.Tags = HTMLTags()
			}
			d.Folding = func(ctx context.Context, text string) ([]FoldRange, error) { return foldMarkup(ctx, text, html) }
			d.CompletionTriggers = "<"
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
		case HTTP, Makefile, CMake, Nginx, INI, Dockerfile:
			d.Tokenizer = configTokenizer{info.ID}
			d.AllowWordSuggestions = codeWordSuggestions
			d.Editing.Comments.Line = "#"
			d.Editing.Word = func(r rune) bool { return IdentifierRune(r) || r == '-' || r == '.' }
			if info.ID == CMake {
				d.Editing.Comments.Block = Pair{Open: "#[[", Close: "]]"}
			}
			vocabulary := configVocabulary(info.ID)
			d.Suggest = func(Context) []string { return vocabulary }
		case Perl:
			d.Tokenizer = plTokenizer{}
			d.Editing.Comments.Line = "#"
			d.Editing.Word = func(r rune) bool { return IdentifierRune(r) || r == '$' || r == '@' || r == '%' }
			d.AllowWordSuggestions = codeWordSuggestions
			vocabulary := codeVocabulary(info.ID)
			d.Suggest = func(Context) []string { return vocabulary }
		case Scala:
			d.Tokenizer = scalaTokenizer{}
			d.Editing.Comments = Comments{Line: "//", Block: Pair{Open: "/*", Close: "*/"}}
			d.Editing.Word = func(r rune) bool { return IdentifierRune(r) || r == '$' }
			p := Pair{Open: "`", Close: "`", NotInString: true, NotInComment: true}
			d.Editing.AutoClosingPairs = append(d.Editing.AutoClosingPairs, p)
			d.Editing.SurroundingPairs = append(d.Editing.SurroundingPairs, p)
			d.Editing.AllowPair = func(c PairContext) bool {
				return c.Selected || c.Context.String || c.Typed != "'" && (c.Typed != `"` || !strings.HasSuffix(c.Before, `""`))
			}
			d.AllowWordSuggestions = codeWordSuggestions
			vocabulary := codeVocabulary(info.ID)
			d.Suggest = func(Context) []string { return vocabulary }
		case ObjectiveC, ObjectiveCpp:
			d.Tokenizer = cTokenizer{info.ID}
			d.Editing.Word = func(r rune) bool { return IdentifierRune(r) || r == '@' }
			d.Editing.Comments = Comments{Line: "//", Block: Pair{Open: "/*", Close: "*/"}}
			d.AllowWordSuggestions = codeWordSuggestions
			vocabulary := codeVocabulary(info.ID)
			d.Suggest = func(Context) []string { return vocabulary }
		case Zig:
			d.Tokenizer = zigTokenizer{}
			d.Editing.Comments.Line = "//"
			d.AllowWordSuggestions = codeWordSuggestions
			vocabulary := codeVocabulary(info.ID)
			d.Suggest = func(Context) []string { return vocabulary }
		case R:
			d.Tokenizer = rTokenizer{}
			d.Editing.Word = func(r rune) bool { return IdentifierRune(r) || r == '.' }
			d.Editing.Comments.Line = "#"
			p := Pair{Open: "`", Close: "`", NotInString: true, NotInComment: true}
			d.Editing.AutoClosingPairs = append(d.Editing.AutoClosingPairs, p)
			d.Editing.SurroundingPairs = append(d.Editing.SurroundingPairs, p)
			d.Editing.AllowPair = func(c PairContext) bool {
				return c.Selected || c.Context.String || c.Typed != `"` && c.Typed != "'" || !rRawQuotePrefix(c.Before)
			}
			d.AllowWordSuggestions = codeWordSuggestions
			vocabulary := codeVocabulary(info.ID)
			d.Suggest = func(Context) []string { return vocabulary }
		case Dart:
			d.Tokenizer = dartTokenizer{}
			d.Editing.Comments = Comments{Line: "//", Block: Pair{Open: "/*", Close: "*/"}}
			d.Editing.Word = func(r rune) bool { return IdentifierRune(r) || r == '$' }
			d.Editing.AllowPair = func(c PairContext) bool {
				return c.Selected || c.Context.String || c.Typed != `"` && c.Typed != "'" || !strings.HasSuffix(c.Before, c.Typed+c.Typed)
			}
			d.AllowWordSuggestions = codeWordSuggestions
			vocabulary := codeVocabulary(info.ID)
			d.Suggest = func(Context) []string { return vocabulary }
		case Swift:
			d.Tokenizer = swiftTokenizer{}
			d.Editing.Comments = Comments{Line: "//", Block: Pair{Open: "/*", Close: "*/"}}
			for _, pairs := range []*[]Pair{&d.Editing.AutoClosingPairs, &d.Editing.SurroundingPairs} {
				filtered := (*pairs)[:0]
				for _, p := range *pairs {
					if p.Open != "'" {
						filtered = append(filtered, p)
					}
				}
				*pairs = append(filtered, Pair{Open: "`", Close: "`", NotInString: true, NotInComment: true})
			}
			d.Editing.AllowPair = func(c PairContext) bool {
				return c.Selected || c.Context.String || c.Typed != `"` || !strings.HasSuffix(c.Before, `""`) && !strings.HasSuffix(c.Before, "#")
			}
			d.AllowWordSuggestions = codeWordSuggestions
			vocabulary := codeVocabulary(info.ID)
			d.Suggest = func(Context) []string { return vocabulary }
		case Kotlin:
			d.Tokenizer = ktTokenizer{}
			d.Editing.Comments = Comments{Line: "//", Block: Pair{Open: "/*", Close: "*/"}}
			p := Pair{Open: "`", Close: "`", NotInString: true, NotInComment: true}
			d.Editing.AutoClosingPairs = append(d.Editing.AutoClosingPairs, p)
			d.Editing.SurroundingPairs = append(d.Editing.SurroundingPairs, p)
			d.Editing.AllowPair = func(c PairContext) bool {
				return c.Selected || c.Context.String || c.Typed != `"` || !strings.HasSuffix(c.Before, `""`)
			}
			d.AllowWordSuggestions = codeWordSuggestions
			vocabulary := codeVocabulary(info.ID)
			d.Suggest = func(Context) []string { return vocabulary }
		case JSX, TSX:
			d.Tokenizer = jsxTokenizer{info.ID}
			d.Editing.Comments = Comments{Line: "//", Block: Pair{Open: "/*", Close: "*/"}}
			d.Editing.Word = func(r rune) bool { return IdentifierRune(r) || r == '$' }
			p := Pair{Open: "`", Close: "`", NotInString: true, NotInComment: true}
			d.Editing.AutoClosingPairs = append(d.Editing.AutoClosingPairs, p)
			d.Editing.SurroundingPairs = append(d.Editing.SurroundingPairs, p)
			d.Editing.AllowPair = func(c PairContext) bool {
				return !c.Context.Tag || c.Selected || c.Typed != "'" && c.Typed != `"` || c.Context.Value
			}
			d.AllowWordSuggestions = codeWordSuggestions
			base := JavaScript
			if info.ID == TSX {
				base = TypeScript
			}
			vocabulary := codeVocabulary(base)
			html := out[HTML]
			d.Suggest = func(c Context) []string {
				if c.String || c.Comment {
					return nil
				}
				if c.Tag {
					items := html.Suggestions(c)
					if !c.TagName {
						items = append(items, "className", "htmlFor", "key", "ref")
					}
					return items
				}
				return vocabulary
			}
		case PHP:
			htmlDefinition := out[HTML].Definition()
			// Mixed PHP spans are not safe input to the markup-only tag matcher.
			htmlDefinition.Editing.Tags = nil
			htmlDefinition.Folding = nil
			htmlProfile, err := New(htmlDefinition)
			if err != nil {
				panic(err)
			}
			d.Tokenizer = phpTokenizer{html: htmlProfile}
			d.Editing.Comments = Comments{Line: "//", Block: Pair{Open: "/*", Close: "*/"}}
			d.Editing.Word = func(r rune) bool { return IdentifierRune(r) || r == '$' || r == '\\' }
			d.AllowWordSuggestions = codeWordSuggestions
			vocabulary := codeVocabulary(info.ID)
			d.Suggest = func(c Context) []string {
				if c.Embedded != nil && c.Embedded.ID() != PHP {
					return c.Embedded.Suggestions(c)
				}
				return vocabulary
			}
		case Ruby:
			d.Tokenizer = rbTokenizer{}
			d.Editing.Comments = Comments{Line: "#"}
			d.Editing.Word = func(r rune) bool { return IdentifierRune(r) || strings.ContainsRune("@$!?", r) }
			p := Pair{Open: "`", Close: "`", NotInString: true, NotInComment: true}
			d.Editing.AutoClosingPairs = append(d.Editing.AutoClosingPairs, p)
			d.Editing.SurroundingPairs = append(d.Editing.SurroundingPairs, p)
			d.AllowWordSuggestions = codeWordSuggestions
			vocabulary := codeVocabulary(info.ID)
			d.Suggest = func(Context) []string { return vocabulary }
		case PowerShell:
			d.Tokenizer = psTokenizer{}
			d.Editing.QuoteEscape = '`'
			d.Editing.Comments = Comments{Line: "#", Block: Pair{Open: "<#", Close: "#>"}}
			d.Editing.Word = func(r rune) bool { return IdentifierRune(r) || strings.ContainsRune("-$:", r) }
			d.AllowWordSuggestions = codeWordSuggestions
			vocabulary := codeVocabulary(info.ID)
			d.Suggest = func(Context) []string { return vocabulary }
		case C, Cpp, Java, Rust, CSharp, Lua:
			switch info.ID {
			case C, Cpp:
				d.Tokenizer = cTokenizer{info.ID}
			case Java:
				d.Tokenizer = javaTokenizer{}
			case Rust:
				d.Tokenizer = rustTokenizer{}
			case CSharp:
				d.Tokenizer = csTokenizer{}
			case Lua:
				d.Tokenizer = luaTokenizer{}
			}
			d.AllowWordSuggestions = codeWordSuggestions
			d.Editing.Comments = Comments{Line: "//", Block: Pair{Open: "/*", Close: "*/"}}
			if info.ID == Lua {
				d.Editing.Comments = Comments{Line: "--", Block: Pair{Open: "--[[", Close: "]]"}}
			}
			if info.ID == Rust {
				// An apostrophe usually starts a lifetime; selection surrounding
				// remains available for explicitly creating a character literal.
				d.Editing.AllowPair = func(c PairContext) bool { return c.Selected || c.Typed != "'" }
			}
			if info.ID == Java {
				d.Editing.Word = func(r rune) bool { return IdentifierRune(r) || r == '$' }
			}
			if info.ID == Java || info.ID == CSharp {
				d.Editing.AllowPair = func(c PairContext) bool {
					return c.Selected || c.Context.String || c.Typed != `"` || !strings.HasSuffix(c.Before, `""`)
				}
			}
			vocabulary := codeVocabulary(info.ID)
			d.Suggest = func(Context) []string { return vocabulary }
		case SQL, Protobuf, Go, Python, JavaScript, TypeScript, CSS:
			d.Tokenizer = codeTokenizer{info.ID}
			d.AllowWordSuggestions = codeWordSuggestions
			if info.ID == JavaScript || info.ID == TypeScript {
				d.Editing.Word = func(r rune) bool { return IdentifierRune(r) || r == '$' }
			}
			d.Editing.Comments = Comments{Line: "//", Block: Pair{Open: "/*", Close: "*/"}}
			if info.ID == Python {
				d.Editing.Comments = Comments{Line: "#"}
			}
			if info.ID == SQL {
				d.Editing.Comments.Line = "--"
			}
			if info.ID == CSS {
				d.Editing.Comments.Line = ""
				d.Editing.Word = func(r rune) bool { return IdentifierRune(r) || r == '-' }
			}
			if info.ID == Go || info.ID == JavaScript || info.ID == TypeScript || info.ID == SQL {
				p := Pair{Open: "`", Close: "`", NotInString: true, NotInComment: true}
				d.Editing.AutoClosingPairs = append(d.Editing.AutoClosingPairs, p)
				d.Editing.SurroundingPairs = append(d.Editing.SurroundingPairs, p)
			}
			if info.ID == Python {
				d.Editing.AllowPair = func(c PairContext) bool {
					return c.Selected || c.Context.String || !(c.Typed == `"` || c.Typed == "'") || !strings.HasSuffix(c.Before, c.Typed+c.Typed)
				}
			}
			vocabulary := codeVocabulary(info.ID)
			d.Suggest = func(Context) []string { return vocabulary }
		}
		l, err := New(d)
		if err != nil {
			panic(err)
		}
		out[info.ID] = l
	}
	return out
})

func builtinFences(definitions map[ID]*Language) map[string]*Language {
	table := make(map[string]*Language)
	for id, l := range definitions {
		if id != Plain && id != Markdown {
			if id == PHP {
				d := l.Definition()
				scanner := d.Tokenizer.(phpTokenizer)
				scanner.fragment = true
				d.Tokenizer = scanner
				var err error
				l, err = New(d)
				if err != nil {
					panic(err)
				}
			}
			table[string(id)] = l
		}
	}
	for alias, id := range map[string]ID{
		"patch": Diff,
		"batch": Batch, "cmd": Batch,
		"terraform": HCL, "tf": HCL,
		"gql": GraphQL,
		"sh":  Shell, "bash": Shell, "yml": YAML, "js": JavaScript, "ts": TypeScript, "py": Python, "golang": Go,
		"proto": Protobuf, "make": Makefile, "docker": Dockerfile, "jsonl": JSONLines, "ndjson": JSONLines,
		"c++": Cpp, "cc": Cpp, "rs": Rust, "cs": CSharp, "c#": CSharp,
		"ps1": PowerShell, "pwsh": PowerShell, "ps": PowerShell,
		"rb":              Ruby,
		"javascriptreact": JSX, "typescriptreact": TSX,
		"kt": Kotlin, "kts": Kotlin,
		"objc": ObjectiveC, "objectivec": ObjectiveC,
		"pl":     Perl,
		"objcpp": ObjectiveCpp, "objc++": ObjectiveCpp, "objective-c++": ObjectiveCpp,
	} {
		table[alias] = definitions[id]
	}
	return table
}

// Builtin returns an immutable built-in definition. Custom definitions are
// passed directly to a document; no registration or name matching is needed.
func Builtin(id ID) (*Language, bool) { l, ok := builtinLanguages()[id]; return l, ok }

func defaultEditing() EditingRules {
	brackets := []Pair{{Open: "(", Close: ")", NotInString: true, NotInComment: true}, {Open: "[", Close: "]", NotInString: true, NotInComment: true}, {Open: "{", Close: "}", NotInString: true, NotInComment: true}}
	pairs := append(append([]Pair(nil), brackets...), Pair{Open: `"`, Close: `"`, NotInString: true, NotInComment: true}, Pair{Open: "'", Close: "'", NotInString: true, NotInComment: true})
	return EditingRules{Word: IdentifierRune, AutoClosingPairs: pairs, SurroundingPairs: append([]Pair(nil), pairs...), Brackets: brackets, AutoCloseBefore: ")]},:;>"}
}

func codeVocabulary(id ID) []string {
	var table map[string]bool
	switch id {
	case GraphQL:
		table = gqlKeywords
	case HCL:
		table = hcKeywords
	case Batch:
		table = batKeywords
	case Perl:
		table = plKeywords
	case Scala:
		table = scalaKeywords
	case ObjectiveC, ObjectiveCpp:
		out := codeVocabulary(cBaseID(id))
		for word := range objcKeywords {
			out = append(out, word)
		}
		for word := range objcDirectives {
			out = append(out, "@"+word)
		}
		slices.Sort(out)
		return out
	case Kotlin:
		table = ktKeywords
	case Swift:
		table = swiftKeywords
	case Dart:
		table = dartKeywords
	case R:
		table = rKeywords
	case Zig:
		table = zigKeywords
	case PowerShell:
		table = psKeywords
	case Ruby:
		table = rbKeywords
	case PHP:
		table = phpKeywords
	case C:
		table = cKeywords
	case Cpp:
		table = cppKeywords
	case Java:
		table = javaKeywords
	case Rust:
		table = rustKeywords
	case CSharp:
		table = csKeywords
	case Lua:
		table = luaKeywords
	case Go:
		table = goKeywords
	case Python:
		table = pythonKeywords
	case JavaScript:
		table = jsKeywords
	case TypeScript:
		table = tsKeywords
	case Protobuf:
		table = protoKeywords
	case SQL:
		table = sqlKeywords
	case CSS:
		return strings.Fields("color background background-color border border-radius display flex grid font-family font-size gap height margin padding position width var calc url @media @supports")
	}
	var out []string
	for word := range table {
		out = append(out, word)
	}
	if id == TypeScript {
		for word := range jsKeywords {
			out = append(out, word)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func configVocabulary(id ID) []string {
	switch id {
	case HTTP:
		return strings.Fields("GET HEAD POST PUT DELETE CONNECT OPTIONS TRACE PATCH Content-Type Authorization Accept")
	case Makefile:
		return strings.Fields("include define endef ifdef ifndef ifeq ifneq else endif export override .PHONY")
	case CMake:
		return strings.Fields("cmake_minimum_required project add_executable add_library target_link_libraries target_include_directories set option if else endif foreach endforeach find_package include message")
	case Nginx:
		return strings.Fields("http server location listen server_name root index proxy_pass upstream include events worker_connections")
	case Dockerfile:
		return strings.Fields("ADD ARG CMD COPY ENTRYPOINT ENV EXPOSE FROM HEALTHCHECK LABEL ONBUILD RUN SHELL STOPSIGNAL USER VOLUME WORKDIR")
	case INI:
		return strings.Fields("true false yes no on off")
	}
	return nil
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
	raw, next, n := scanBuiltin(b.id, s, in.Text, limit, in.LineEnd, in.Offset)
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

func (s builtinState) Context() Context {
	if s.code != nil {
		c := s.code.Context()
		c.Embedded, _ = Builtin(s.code.id)
		return c
	}
	if t := s.toml; t != nil {
		return Context{String: t.quote != 0, Comment: t.comment, Literal: t.quote == '\'', Value: !t.key}
	}
	if y := s.yaml; y != nil {
		return Context{String: y.quote != 0 || y.block && !y.blockHeader, Comment: y.comment, Literal: y.quote == '\''}
	}
	if sh := s.shell; sh != nil {
		mode := shTop(sh.stack)
		return Context{String: sh.active != nil || sh.capture || mode == shSingle || mode == shDouble || mode == shANSI, Comment: sh.comment, Literal: mode == shSingle || sh.active != nil && sh.active.literal}
	}

	return Context{
		String:  s.mode == jsonString || s.mode == attrSingle || s.mode == attrDouble,
		Comment: s.mode == comment || s.mode == cdata || s.mode == pi || s.mode == declaration || s.mode == scriptData || s.mode == rawText || s.mode == plainText,
		Tag:     s.pendingTag || s.mode == tagName || s.mode == tagInside || s.mode == attrName || s.mode == afterAttr || s.mode == beforeValue,
		TagName: s.pendingTag || s.mode == tagName,
		Value:   s.mode == beforeValue,
	}
}
