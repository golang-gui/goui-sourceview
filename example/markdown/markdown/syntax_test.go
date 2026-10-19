package markdown

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	source "github.com/golang-gui/goui-sourceview"
	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/gui/textedit"
)

func scan(t *testing.T, text string, chunk int) []language.Token {
	t.Helper()
	scanner := tokenizer{}
	var snapshot language.State = scanner.InitialState()
	var tokens []language.Token
	absolute := 0
	appendToken := func(tok language.Token) {
		if len(tokens) > 0 {
			last := &tokens[len(tokens)-1]
			if last.End == tok.Start && last.Kind == tok.Kind && last.Class == tok.Class {
				last.End = tok.End
				return
			}
		}
		tokens = append(tokens, tok)
	}
	for _, line := range strings.Split(text, "\n") {
		at := 0
		for {
			limit := min(chunk, len(line)-at)
			for at+limit < len(line) && !utf8.RuneStart(line[at+limit]) {
				limit++
			}
			end := min(len(line), at+limit+32)
			for end < len(line) && !utf8.RuneStart(line[end]) {
				end++
			}
			in := language.Input{Text: line[at:end], Offset: at, Limit: limit, LineEnd: end == len(line)}
			result, err := scanner.Scan(in, snapshot)
			if err != nil {
				t.Fatal(err)
			}
			if err = language.ValidateResult(in, result); err != nil {
				t.Fatalf("at %d: %v", at, err)
			}
			for _, tok := range result.Tokens {
				tok.Start += absolute
				tok.End += absolute
				appendToken(tok)
			}
			at += result.Consumed
			snapshot = result.State
			if at == len(line) {
				break
			}
		}
		absolute += len(line) + 1
	}
	return tokens
}
func TestMarkdownCategoriesAndFragments(t *testing.T) {
	text := "# 标题\n**bold** `code [ignored]` [link](https://example.com)\n```go\n[a](b)\n```\n<!-- one\n two -->\nafter\n" + "# " + strings.Repeat("中文", 9000) + "\n```go\ncode\n```" + strings.Repeat(" ", 100) + "invalid\nstill code\n```"
	want := scan(t, text, len(text))
	for _, chunk := range []int{1, 2, 3, 7, 31, 4096} {
		if got := scan(t, text, chunk); !reflect.DeepEqual(got, want) {
			t.Fatalf("fragment-dependent syntax, chunk %d: got %v want %v", chunk, got[:min(30, len(got))], want[:min(30, len(want))])
		}
	}
	for _, tc := range []struct {
		needle string
		kind   language.Kind
		class  language.Class
	}{
		{"标题", Heading, 0}, {"bold", Emphasis, 0}, {"ignored", Code, language.ClassString}, {"https://", Link, language.ClassString}, {"[a]", Code, language.ClassString}, {"two", language.KindComment, language.ClassComment}, {"[link", language.KindDelimiter, language.ClassDelimiter},
	} {
		at := strings.Index(text, tc.needle)
		found := false
		for _, tok := range want {
			if tok.Start <= at && at < tok.End {
				found = tok.Kind == tc.kind && tok.Class == tc.class
				break
			}
		}
		if !found {
			t.Fatalf("%q missing %s class %d", tc.needle, tc.kind, tc.class)
		}
	}
}
func TestMarkdownNewline(t *testing.T) {
	for _, tc := range []struct {
		before, after, want string
		remove              int
	}{
		{"- item", "", "\n- ", 0}, {"9. item", "", "\n10. ", 0}, {"  - [x] done", "", "\n  - [ ] ", 0}, {"- ", "", "\n", 2}, {"> quote", "", "\n> ", 0}, {"```", "```", "\n\n", 0},
	} {
		result, handled := newline(language.NewlineContext{Before: tc.before, After: tc.after, Indent: strings.Repeat(" ", len(tc.before)-len(strings.TrimLeft(tc.before, " "))), Known: true})
		if !handled || result.Text != tc.want || result.DeleteBefore != tc.remove {
			t.Fatalf("%q: %+v %t", tc.before, result, handled)
		}
	}
}
func TestMarkdownProviderOwnsRangesAndAdditionalEdits(t *testing.T) {
	definition, err := NewLanguage()
	if err != nil {
		t.Fatal(err)
	}
	c := source.CompletionContext{Definition: definition, ContextKnown: true, Caret: 1, Range: textedit.Range{Start: 1, End: 1}, Before: "[", After: "]", DocumentLength: 2, Trigger: "["}
	items, err := complete(context.Background(), c)
	if err != nil || len(items) != 1 || items[0].Range == nil || *items[0].Range != (textedit.Range{Start: 0, End: 2}) || items[0].Caret == nil || *items[0].Caret != 8 {
		t.Fatal(items, err)
	}
	c.Before = "@"
	c.After = ""
	c.DocumentLength = 1
	items, err = complete(context.Background(), c)
	if err != nil || len(items) != 1 || len(items[0].AdditionalEdits) != 1 || items[0].AdditionalEdits[0].Range.Start != 1 {
		t.Fatal(items, err)
	}
	c.Lexical.Comment = true
	if items, err = complete(context.Background(), c); err != nil || len(items) != 0 {
		t.Fatal("comment completion", items, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = complete(ctx, c); err != context.Canceled {
		t.Fatal(err)
	}
}
