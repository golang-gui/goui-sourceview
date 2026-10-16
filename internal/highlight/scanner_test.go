package highlight

import (
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/golang-gui/goui-sourceview/language"
)

func scanAll(id language.ID, text string, chunk int) ([]Token, State) {
	s := Initial()
	var out []Token
	offset := 0
	lines := strings.Split(text, "\n")
	for line, part := range lines {
		at := 0
		for at < len(part) {
			limit := min(chunk, len(part)-at)
			for !utf8.RuneStart(part[at+limit-1]) && at+limit < len(part) && !utf8.RuneStart(part[at+limit]) {
				limit--
			}
			if limit == 0 {
				_, limit = utf8.DecodeRuneInString(part[at:])
			}
			end := min(len(part), at+limit+32)
			for end < len(part) && !utf8.RuneStart(part[end]) {
				end++
			}
			tokens, next, n := Scan(id, s, part[at:end], limit, end == len(part))
			if n <= 0 || n > end-at {
				panic("scanner failed progress")
			}
			for _, tok := range tokens {
				emit(&out, offset+at+tok.Start, offset+at+tok.End, tok.Kind)
			}
			at += n
			s = next
		}
		if len(part) == 0 {
			_, s, _ = Scan(id, s, "", 0, true)
		}
		offset += len(part)
		if line < len(lines)-1 {
			offset++
		}
	}
	return out, s
}

func kindAt(tokens []Token, offset int) language.Kind {
	for _, t := range tokens {
		if t.Start <= offset && offset < t.End {
			return t.Kind
		}
	}
	return language.KindPlain
}
func TestLexicalFixtures(t *testing.T) {
	for _, tc := range []struct {
		id           language.ID
		text, needle string
		kind         language.Kind
	}{
		{language.JSON, "{\"name\": \"value\", \"n\":12, \"b\":true}", "name", language.KindProperty},
		{language.JSON, "{\"name\": \"value\"}", "value", language.KindString},
		{language.JSON, "{\"n\":12}", "12", language.KindNumber},
		{language.JSON, "{\"b\":true}", "true", language.KindKeyword},
		{language.JSON, "{\"bad\":01}", "01", language.KindInvalid},
		{language.XML, "<名 a='v'>&amp;<!-- c --><![CDATA[<x>]]></名>", "名", language.KindTag},
		{language.XML, "<x a='v'/>", "a", language.KindAttribute},
		{language.XML, "<x>&amp;</x>", "&amp;", language.KindEntity},
		{language.XML, "<!-- c -->", "c", language.KindComment},
		{language.HTML, "<INPUT disabled value=ok>", "disabled", language.KindAttribute},
		{language.HTML, "<INPUT disabled value=ok>", "ok", language.KindString},
		{language.HTML, "<script>\"</script>\"<b>yes</b>", "b", language.KindTag},
		{language.HTML, "<script>x</scriptx> <b> y </script><p>z</p>", "b", language.KindPlain},
		{language.HTML, "<style><b>not a tag</b></STYLE><p>x</p>", "b", language.KindPlain},
		{language.XML, "<x a='line\ncontinued'>z</x>", "continued", language.KindString},
	} {
		tokens, _ := scanAll(tc.id, tc.text, 16384)
		at := strings.Index(tc.text, tc.needle)
		if got := kindAt(tokens, at); got != tc.kind {
			t.Errorf("%s %q %q: %s, want %s; %+v", tc.id, tc.text, tc.needle, got, tc.kind, tokens)
		}
	}
}
func TestFragmentationInvariant(t *testing.T) {
	texts := map[language.ID]string{
		language.JSON: "{\n\"名\": \"a\\u1234\\\"b\", \"x\": [1.2e-3,false,null]\n}",
		language.XML:  "<?xml version='1.0'?><名 a='a&amp;b\nc'><!-- c --><![CDATA[x]]></名>",
		language.HTML: "<!DOCTYPE html><SCRIPT>\"</scriptx>\"<!--<script>x</script>--></script><p a=b>ok</p>",
	}
	for id, text := range texts {
		want, ws := scanAll(id, text, 16384)
		for _, chunk := range []int{1, 2, 3, 7, 16} {
			got, gs := scanAll(id, text, chunk)
			if !reflect.DeepEqual(got, want) || !gs.Equal(ws) {
				t.Errorf("%s chunk=%d:\n%+v\n%+v", id, chunk, got, want)
			}
			for _, tok := range got {
				if tok.Start < 0 || tok.End > len(text) || tok.Start >= tok.End || !utf8.ValidString(text[tok.Start:tok.End]) {
					t.Fatal(tok)
				}
			}
		}
	}
}
func TestCrossLineContext(t *testing.T) {
	for _, id := range []language.ID{language.XML, language.HTML} {
		tokens, _ := scanAll(id, "<!-- open\nstill comment\n--><a>text</a>", 2)
		if kindAt(tokens, strings.Index("<!-- open\nstill comment\n--><a>text</a>", "still")) != language.KindComment {
			t.Fatal(tokens)
		}
	}
	tokens, _ := scanAll(language.JSON, "{\"key\"\n:1}", 2)
	if kindAt(tokens, 2) != language.KindProperty {
		t.Fatal(tokens)
	}
}
