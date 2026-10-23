package language

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestLookupDetached(t *testing.T) {
	a, ok := Lookup(XML)
	if !ok || a.Name != "XML" {
		t.Fatal(a, ok)
	}
	a.Extensions[0] = ".broken"
	a.MIMETypes[0] = "broken"
	b, _ := Lookup(XML)
	if b.Extensions[0] != ".xml" || b.MIMETypes[0] != "application/xml" {
		t.Fatal(b)
	}
	if _, ok := Lookup("unknown"); ok {
		t.Fatal("accepted unknown language")
	}
}

func TestGuess(t *testing.T) {
	for _, tc := range []struct {
		name, sample string
		want         ID
	}{
		{"a.JSON", "", JSON}, {"a.xhtml", "<html>", XML},
		{"a.html", "<?xml", HTML}, {"", " \xef\xbb\xbf", Plain},
		{"", "\xef\xbb\xbf <?xml version='1.0'?>", XML},
		{"", " <!DOCTYPE HTML>", HTML}, {"", "{\"a\":1}", JSON},
		{"", "words", Plain},
	} {
		if got := Guess(tc.name, []byte(tc.sample)); got != tc.want {
			t.Errorf("%q %q: %q, want %q", tc.name, tc.sample, got, tc.want)
		}
	}
}

func TestKind(t *testing.T) {
	if KindInvalid.String() != "invalid" || Kind("bad key").Valid() {
		t.Fatal("kind contract")
	}
}

func TestConfigurationAndShellDetection(t *testing.T) {
	for _, tc := range []struct {
		name, sample string
		want         ID
	}{
		{"Cargo.TOML", "", TOML}, {"config.YAML", "", YAML}, {".ci.yml", "", YAML},
		{"run.sh", "", Shell}, {"run.bash", "", Shell}, {"/home/user/.bashrc", "", Shell}, {".profile", "", Shell},
		{"script", "#!/bin/sh\necho hi", Shell}, {"script", "#!/usr/bin/env bash\n", Shell}, {"script", "#!/usr/bin/env -S bash -eu\n", Shell},
		{"script", "#!/usr/bin/env -u bash python", Plain},
		{"script", "#!/usr/bin/env --chdir bash python", Plain},
		{"script", "#!/usr/bin/env -u NAME bash", Shell},
		{"script", "#!/usr/bin/env -- bash", Shell},
		{"script", "#!/usr/bin/env --unknown sh", Plain},
		{"x.json", "#!/bin/sh", JSON}, {"unknown", "#!/usr/bin/env python\n", Plain},
		{"unknown", " #!/bin/sh", Plain}, {".env", "NAME=value", Plain}, {"unknown", "name: value", Plain},
		{"unknown", string(make([]byte, 4096)) + "#!/bin/sh", Plain},
	} {
		if got := Guess(tc.name, []byte(tc.sample)); got != tc.want {
			t.Errorf("%q: got %q want %q", tc.name, got, tc.want)
		}
	}
	for _, id := range []ID{TOML, YAML, Shell} {
		info, ok := Lookup(id)
		if !ok || info.Name == "" || len(info.Extensions) == 0 {
			t.Fatal(id, info, ok)
		}
	}
	if !KindVariable.Valid() || KindVariable.String() != "variable" {
		t.Fatal("variable style missing")
	}
}

func TestBuiltinFolding(t *testing.T) {
	for _, tt := range []struct {
		id   ID
		text string
		want []FoldRange
	}{
		{JSON, "{\n \"a\":[\n \"} ]\",\n 2\n ]\n}", []FoldRange{{0, 5}, {1, 4}}},
		{JSON, "{\n 1\n}", []FoldRange{{0, 2}}},
		{JSON, "[\n {\n } , \"outside\"\n]", []FoldRange{{0, 3}}},
		{JSON, "[\n}", nil},
		{XML, "<a x=\">\">\n <b/>\n <!-- x\n y -->\n</a>", []FoldRange{{0, 4}, {2, 3}}},
		{XML, "<a>\n text\n</a><b/>", []FoldRange{{0, 1}}},
		{XML, "<a>\n</A>", nil},
		{XML, "<![CDATA[\n<a>\n]]>", []FoldRange{{0, 2}}},
		{HTML, "<DIV>\n<br>\n<script>\nconst x='<fake>'; // <!--\n</script>\n</div>", []FoldRange{{0, 5}, {2, 4}}},
	} {
		t.Run(string(tt.id)+tt.text, func(t *testing.T) {
			l, _ := Builtin(tt.id)
			got, err := l.Folding()(context.Background(), tt.text)
			slices.SortFunc(got, func(a, b FoldRange) int { return a.StartLine - b.StartLine })
			if err != nil || !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %v,%v want %v", got, err, tt.want)
			}
		})
	}
}

func TestFoldingCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, id := range []ID{JSON, XML, HTML} {
		l, _ := Builtin(id)
		_, err := l.Folding()(ctx, "{\n<a>\n}")
		if !errors.Is(err, context.Canceled) {
			t.Fatal(id, err)
		}
	}
}

func markdownTokens(t *testing.T, text string, chunk int) ([]Token, State) {
	t.Helper()
	l, _ := Builtin(Markdown)
	scanner := l.Tokenizer()
	state := scanner.InitialState()
	var tokens []Token
	absolute := 0
	for _, line := range strings.Split(text, "\n") {
		var current []Token
		for at := 0; ; {
			n := min(chunk, len(line)-at)
			for at+n < len(line) && !utf8.RuneStart(line[at+n]) {
				n++
			}
			in := Input{Text: line[at : at+n], Offset: at, Limit: n, LineEnd: at+n == len(line)}
			r, err := scanner.Scan(in, state)
			if err != nil {
				t.Fatal(err)
			}
			if err = ValidateResult(in, r); err != nil {
				t.Fatal(err)
			}
			if r.ReplaceFrom != nil {
				cut := 0
				for cut < len(current) && current[cut].End <= *r.ReplaceFrom {
					cut++
				}
				current = current[:cut]
			}
			current = append(current, r.Tokens...)
			state = r.State
			at += r.Consumed
			if at == len(line) {
				break
			}
		}
		for _, tok := range current {
			tok.Start += absolute
			tok.End += absolute
			tokens = append(tokens, tok)
		}
		absolute += len(line) + 1
	}
	return tokens, state
}

func TestMarkdownFragmentReplayAndStyles(t *testing.T) {
	text := "# 标题\n> - [x] item\n**bold** *em* ~~gone~~ `code` [link](url) &amp; \\*\n| a | b |\n| --- | --- |\n<!-- comment\n close -->\n````json\n{\"a\": true}\n```\n# code\n````\n## Next"
	want, _ := markdownTokens(t, text, len(text))
	for _, chunk := range []int{1, 2, 3, 7, 31} {
		got, _ := markdownTokens(t, text, chunk)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("chunk %d differs", chunk)
		}
	}
	for _, tc := range []struct {
		text string
		kind Kind
	}{{"标题", KindHeading}, {"bold", KindEmphasis}, {"gone", KindEmphasis}, {"code`", KindCode}, {"link", KindLink}, {"&amp;", KindEntity}, {"comment", KindComment}, {"\"a\"", KindProperty}, {"true", KindKeyword}, {"# code", KindInvalid}, {"Next", KindHeading}} {
		at := strings.Index(text, tc.text)
		found := false
		for _, tok := range want {
			if tok.Start <= at && at < tok.End {
				found = tok.Kind == tc.kind
				break
			}
		}
		if !found {
			t.Fatal("wrong style", tc)
		}
	}
	l, _ := Builtin(Markdown)
	if Guess("a.markdown", nil) != Markdown {
		t.Fatal("extension")
	}
	r, err := l.Tokenizer().Scan(Input{Text: "```json", Limit: 7, LineEnd: true}, l.Tokenizer().InitialState())
	if err != nil {
		t.Fatal(err)
	}
	r, err = l.Tokenizer().Scan(Input{Text: "{\"x\": ", Limit: 6}, r.State)
	if err != nil || r.State.Context().Embedded == nil || r.State.Context().Embedded.ID() != JSON || r.State.Context().String {
		t.Fatal("embedded context", r, err)
	}
}

func TestMarkdownFoldingAndNewline(t *testing.T) {
	text := "# A\na\n## B\n```json\n# not heading\n```\ntext\n# C\nx"
	got, err := foldMarkdown(context.Background(), text)
	want := []FoldRange{{3, 5}, {2, 6}, {0, 6}, {7, 8}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatal(got, err)
	}
	for _, tc := range []struct {
		before, want string
		remove       int
	}{{"9. item", "\n10. ", 0}, {"- [x] done", "\n- [ ] ", 0}, {"- ", "\n", 2}, {"> quote", "\n> ", 0}, {"> ", "\n", 2}, {"> - item", "\n> - ", 0}} {
		got, ok := markdownNewline(NewlineContext{Before: tc.before, Known: true})
		if !ok || got.Text != tc.want || got.DeleteBefore != tc.remove {
			t.Fatal(tc, got, ok)
		}
	}
}

func TestMarkdownExplicitEmbeddingCopiesMap(t *testing.T) {
	json, _ := Builtin(JSON)
	table := map[string]*Language{"custom": json}
	l, err := NewMarkdown(table)
	if err != nil {
		t.Fatal(err)
	}
	delete(table, "custom")
	scanner := l.Tokenizer()
	r, _ := scanner.Scan(Input{Text: "```custom", Limit: 9, LineEnd: true}, scanner.InitialState())
	r, _ = scanner.Scan(Input{Text: "true", Limit: 4}, r.State)
	if r.State.Context().Embedded != json {
		t.Fatal("map aliased")
	}
}

func TestTagPolicyConservativeMatching(t *testing.T) {
	for _, tc := range []struct {
		text  string
		rules *TagRules
		close string
	}{{`<ns:a x=">">`, XMLTags(), `</ns:a>`}, {`<!-- <x>`, XMLTags(), ""}, {`<![CDATA[<x>`, XMLTags(), ""}, {`<x/>`, XMLTags(), ""}, {`<BR>`, HTMLTags(), ""}, {`<script> '<x>`, HTMLTags(), ""}, {`<!doctype html><DIV>`, HTMLTags(), `</DIV>`}} {
		if got := tc.rules.ClosingTag(tc.text); got != tc.close {
			t.Fatal(tc.text, got)
		}
	}
	text := `<root><a title=">"></a></root>`
	pair, ok := XMLTags().Match(text, 8)
	if !ok || text[pair.Opening.Start:pair.Opening.End] != "a" || text[pair.Closing.Start:pair.Closing.End] != "a" {
		t.Fatal(pair, ok)
	}
	if _, ok := XMLTags().Match(`<A></a>`, 2); ok {
		t.Fatal("XML case mismatch paired")
	}
	if _, ok := HTMLTags().Match(`<A></a>`, 2); !ok {
		t.Fatal("HTML case mismatch")
	}
}

func TestMarkdownCommentDoesNotCreateHeadingFolds(t *testing.T) {
	got, err := foldMarkdown(context.Background(), "<!--\n# not a heading\ntext\n-->\n# real\nbody")
	if err != nil || !reflect.DeepEqual(got, []FoldRange{{4, 5}}) {
		t.Fatal(got, err)
	}
}

func TestMarkdownLongCodeLineKeepsOuterFenceBoundary(t *testing.T) {
	text := "```json\n" + strings.Repeat("x", 70<<10) + "\n# still code\n```\n# real"
	tokens, _ := markdownTokens(t, text, 4096)
	for _, tc := range []struct {
		needle string
		kind   Kind
	}{{"# still code", KindCode}, {"# real", KindHeading}} {
		at := strings.Index(text, tc.needle)
		found := false
		for _, tok := range tokens {
			if tok.Start <= at && at < tok.End {
				found = tok.Kind == tc.kind
				break
			}
		}
		if !found {
			t.Fatal(tc)
		}
	}
}

func TestMarkdownLongCommentRetainsCrossLineState(t *testing.T) {
	text := "<!--" + strings.Repeat("x", 70<<10) + "\n# still comment\n-->\n# real"
	tokens, _ := markdownTokens(t, text, 4096)
	for _, tc := range []struct {
		needle string
		kind   Kind
	}{{"# still comment", KindComment}, {"# real", KindHeading}} {
		at := strings.Index(text, tc.needle)
		found := false
		for _, tok := range tokens {
			if tok.Start <= at && at < tok.End {
				found = tok.Kind == tc.kind
				break
			}
		}
		if !found {
			t.Fatal(tc)
		}
	}
}

func TestMarkdownOverflowDoesNotOverlapOpeningCommentMarker(t *testing.T) {
	text := strings.Repeat("x", (64<<10)-4) + "<!-->\n# still comment\n-->\n# real"
	tokens, _ := markdownTokens(t, text, 4096)
	at := strings.Index(text, "# still comment")
	found := false
	for _, tok := range tokens {
		if tok.Start <= at && at < tok.End {
			found = tok.Kind == KindComment
			break
		}
	}
	if !found {
		t.Fatal("overlapped the opening comment marker across overflow")
	}
}

func TestMarkupNameCasePolicyPreservesNonASCII(t *testing.T) {
	if _, ok := HTMLTags().Match("<X-Node></x-node>", 2); !ok {
		t.Fatal("ASCII case was not folded")
	}
	if _, ok := HTMLTags().Match("<Φ></φ>", 2); ok {
		t.Fatal("non-ASCII tag spelling was folded")
	}
	ranges, err := foldMarkup(context.Background(), "<Φ>\ntext\n</φ>", true)
	if err != nil || len(ranges) != 0 {
		t.Fatal(ranges, err)
	}
	ranges, err = foldMarkup(context.Background(), "<SCRIPT>\n</ſcript>\n</script>", true)
	if err != nil || !reflect.DeepEqual(ranges, []FoldRange{{0, 2}}) {
		t.Fatal(ranges, err)
	}
}
