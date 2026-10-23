package highlight

import (
	"math/rand"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/gui/textedit"
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
				appendToken(&out, offset+at+tok.Start, offset+at+tok.End, tok.Kind)
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

// The original scanner regression corpus now exercises the public streaming
// contract. Keep its fragment-relative harness and independent expectations.
type State struct {
	raw    language.State
	id     language.ID
	offset int
}

func Initial() State { return State{} }

func (s State) Equal(t State) bool {
	if s.raw == nil && t.raw == nil {
		return true
	}
	if s.raw == nil {
		l, _ := language.Builtin(t.id)
		s.raw = l.Tokenizer().InitialState()
	}
	if t.raw == nil {
		l, _ := language.Builtin(s.id)
		t.raw = l.Tokenizer().InitialState()
	}
	return language.StatesEqual(s.raw, t.raw)
}

func Scan(id language.ID, s State, text string, limit int, lineEnd bool) ([]Token, State, int) {
	l, _ := language.Builtin(id)
	scanner := l.Tokenizer()
	if scanner == nil {
		return nil, State{raw: language.EmptyState{}, id: id}, limit
	}
	if s.raw == nil {
		s.raw = scanner.InitialState()
	}
	in := language.Input{Text: text, Offset: s.offset, Limit: limit, LineEnd: lineEnd}
	result, err := scanner.Scan(in, s.raw)
	if err != nil {
		panic(err)
	}
	if err = language.ValidateResult(in, result); err != nil {
		panic(err)
	}
	tokens := result.Tokens
	for i := range tokens {
		tokens[i].Start -= s.offset
		tokens[i].End -= s.offset
	}
	next := State{raw: result.State, id: id, offset: s.offset + result.Consumed}
	if lineEnd && result.Consumed == len(text) {
		next.offset = 0
	}
	return tokens, next, result.Consumed
}

func emit(out *[]Token, start, end int, kind language.Kind) {
	appendStyled(out, Token{Start: start, End: end, Kind: kind})
}

func appendToken(out *[]Token, start, end int, kind language.Kind) { emit(out, start, end, kind) }

// Original fixtures below exercise lexical categories, not parser validation.
func TestConfigurationAndShellKinds(t *testing.T) {
	for _, tc := range []struct {
		id           language.ID
		text, needle string
		kind         language.Kind
	}{
		{language.TOML, `[owner.name]`, "owner", language.KindProperty},
		{language.TOML, `"quoted key" = true`, "quoted key", language.KindProperty},
		{language.TOML, `a = { sub = 0xFE, enabled = false }`, "sub", language.KindProperty},
		{language.TOML, `a = { sub = 0xFE, enabled = false }`, "0xFE", language.KindNumber},
		{language.TOML, `time = 1979-05-27 07:32:00Z`, "1979", language.KindMeta},
		{language.TOML, `str = "\e\x1b\u1234"`, "\\x1b", language.KindEscape},
		{language.TOML, "s = '''raw\\\ntext'''\na = true", "text", language.KindString},
		{language.TOML, "s = \"\"\"hello\nworld\"\"\"\na = true", "true", language.KindKeyword},
		{language.TOML, `n = 1_000.5e-2 # note`, "note", language.KindComment},
		{language.TOML, `s = "\你"`, "你", language.KindInvalid},
		{language.YAML, "s: \"\\u12\"\nnext: true", "next", language.KindProperty},
		{language.TOML, `n = 01`, "01", language.KindInvalid},
		{language.YAML, "---\nname: SourceView", "name", language.KindProperty},
		{language.YAML, "url: http://example.com/a#frag", "frag", language.KindPlain},
		{language.YAML, "list: [true, null, 12, on, yes]", "true", language.KindKeyword},
		{language.YAML, "list: [true, null, 12, on, yes]", "on", language.KindPlain},
		{language.YAML, "base: &base {value: 42}\nnext: *base", "&base", language.KindMeta},
		{language.YAML, "type: !!str value # comment", "comment", language.KindComment},
		{language.YAML, "text: 'it''s \\raw'", "''", language.KindEscape},
		{language.YAML, "text: |2-\n  # literal\nnext: true", "# literal", language.KindString},
		{language.YAML, "text: >\n  hello\n\n  world\nnext: true", "next", language.KindProperty},
		{language.YAML, "- key: |\n    text\n  next: true", "next", language.KindProperty},
		{language.YAML, "- key: |\n  next: true", "next", language.KindProperty},
		{language.Shell, `if [ "$USER" = root ]; then echo yes; fi`, "then", language.KindKeyword},
		{language.Shell, `PATH="$HOME/bin"`, "PATH", language.KindVariable},
		{language.Shell, `echo 'literal $HOME'`, "$HOME", language.KindString},
		{language.Shell, "cat <<END\n${HOME}\nEND", "HOME", language.KindVariable},
		{language.Shell, `echo "$HOME"`, "$HOME", language.KindVariable},
		{language.Shell, `echo foo#bar # comment`, "bar", language.KindPlain},
		{language.Shell, `echo foo#bar # comment`, "comment", language.KindComment},
		{language.Shell, "echo \"$(# comment\npwd)\"", "comment", language.KindComment},
		{language.Shell, "echo foo\\\n#suffix", "suffix", language.KindPlain},
		{language.Shell, "echo \\\n#comment", "comment", language.KindComment},
		{language.Shell, `echo $HOME#suffix`, "suffix", language.KindPlain},
		{language.Shell, `echo ${HOME}#suffix`, "suffix", language.KindPlain},
		{language.Shell, `echo "$(echo hi)"#suffix`, "suffix", language.KindPlain},
		{language.Shell, `echo \a#suffix`, "suffix", language.KindPlain},
		{language.Shell, `echo "text"#suffix`, "suffix", language.KindPlain},
		{language.Shell, "cat <<EOF\nbody $USER\nEOF\necho done", "EOF\necho", language.KindMeta},
		{language.Shell, "cat <<'EOF'\nbody $USER\nEOF", "$USER", language.KindString},
		{language.Shell, "cat <<A <<-\"B\"\nfirst\nA\n\tsecond\n\tB\necho done", "second", language.KindString},
		{language.Shell, "cat <<A <<-\"B\"\nfirst\nA\n\tsecond\n\tB\necho done", "echo", language.KindPlain},
		{language.Shell, "echo $((1 << 2))\necho after", "after", language.KindPlain},
		{language.Shell, "((1 << 2))\necho after", "after", language.KindPlain},
		{language.Shell, "cat <<\"E\\q\"\nbody\nE\\q\necho after", "after", language.KindPlain},
		{language.Shell, "cat <<<word\necho after", "after", language.KindPlain},
		{language.Shell, "echo \"$(cat <<END\nbody\nEND\n)\"\necho after", "after", language.KindPlain},
	} {
		t.Run(string(tc.id)+"/"+tc.needle, func(t *testing.T) {
			tokens, _ := scanAll(tc.id, tc.text, 16384)
			at := strings.Index(tc.text, tc.needle)
			if at < 0 {
				t.Fatal("bad fixture")
			}
			if got := kindAt(tokens, at); got != tc.kind {
				t.Fatalf("%q: %s, want %s; %+v", tc.text, got, tc.kind, tokens)
			}
		})
	}
}

func TestNewLanguageFragments(t *testing.T) {
	fixtures := map[language.ID][]string{
		language.TOML:  {"a.b = [1, true, { a = 'x' }] # hi\nb = \"\"\"test\\\"\nmore\\u1234\"\"\"\nc = 1979-05-27 07:32:00Z", "[\"name\".section]\nkey = '''literal\n''''x'\nkey = 3", "a=" + strings.Repeat("1", ChunkSize*3)},
		language.YAML:  {"---\nbase: &base [true, 12, null]\ntext: |2- # hint\n  first\n\n  last\nnext: *base", "- key: >+\n    content\n  next: 'it''s \\raw'\nurl: http://example.org/#x", strings.Repeat("名", ChunkSize) + ": true"},
		language.Shell: {"if [ \"$HOME\" ]; then\ncat <<A <<-'B'\n$USER\nA\n\t$literal\n\tB\nfi", "echo \"$(echo ${USER:-$(whoami)})\" $((1 << 2)) $'a\\n' `date`\n# comment", "cat <<END\n${HOME} $HOME \\$HOME\nEND", "cat <<<word\necho foo#bar \"text\"#suffix", "cat <<" + strings.Repeat("x", ChunkSize*3) + "\nbody\n" + strings.Repeat("x", ChunkSize*3) + "\necho after"},
	}
	for id, texts := range fixtures {
		for _, text := range texts {
			want, ws := scanAll(id, text, ChunkSize*8)
			for _, chunk := range []int{1, 2, 3, 7, 31, ChunkSize} {
				// Tiny chunks on the giant heredoc fixture exercise the same transitions;
				// use 31+ there to avoid quadratic test fixture bookkeeping.
				if len(text) > ChunkSize && chunk < 31 {
					continue
				}
				got, gs := scanAll(id, text, chunk)
				if !reflect.DeepEqual(got, want) || !gs.Equal(ws) {
					t.Fatalf("%s chunk=%d len=%d tokens equal=%v states equal=%v\ngot %v\nwant %v", id, chunk, len(text), reflect.DeepEqual(got, want), gs.Equal(ws), got, want)
				}
			}
		}
	}
}

func TestNewLanguageIncrementalRecovery(t *testing.T) {
	for _, tc := range []struct {
		id                             language.ID
		text, old, replacement, needle string
		kind                           language.Kind
	}{
		{language.TOML, "s = \"\"\"open\ntext\nend\nnext = true", `"""`, `"`, "next", language.KindProperty},
		{language.YAML, "key: |\n  body\nnext: true", "|", "plain", "body", language.KindPlain},
		{language.Shell, "cat <<END\n$USER\nEND", "<<END", "<<'END'", "$USER", language.KindString},
		{language.Shell, "cat <<END\nbody\nEND\nafter\nEND\necho done", "END\nafter", "miss\nafter", "after", language.KindString},
	} {
		m := textedit.NewModel(tc.text)
		e := New(m, tc.id)
		e.SetLanguage(tc.id)
		drain(t, e)
		at := strings.Index(tc.text, tc.old)
		change := textedit.Change{}
		m.ConnectChange(func(c textedit.Change) { change = c; e.Change(c) })
		if err := m.Replace(textedit.Range{Start: at, End: at + len(tc.old)}, tc.replacement); err != nil {
			t.Fatal(err)
		}
		drain(t, e)
		checkIndex(t, e, m)
		if change.Revision == 0 {
			t.Fatal("no edit")
		}
		target := strings.Index(m.Text(), tc.needle)
		row := m.LineAt(target)
		r, _ := m.LineRange(row)
		tokens, _, ready := e.Tokens(row)
		if !ready || kindAt(tokens, target-r.Start) != tc.kind {
			t.Fatalf("%s recovery: %v", tc.id, tokens)
		}
		fresh := New(m, tc.id)
		fresh.SetLanguage(tc.id)
		drain(t, fresh)
		for row := 0; row < e.LineCount(); row++ {
			a, _, _ := e.Tokens(row)
			b, _, _ := fresh.Tokens(row)
			if !reflect.DeepEqual(a, b) || !e.Line(row).out.Equal(fresh.Line(row).out) {
				t.Fatalf("%s row %d diverged", tc.id, row)
			}
		}
	}
}

func TestLateClassificationAcrossEngineChunks(t *testing.T) {
	for _, tc := range []struct {
		id     language.ID
		text   string
		offset int
		kind   language.Kind
	}{
		{language.YAML, strings.Repeat("k", ChunkSize*3) + ": true", ChunkSize, language.KindProperty},
		{language.Shell, "cat <<" + strings.Repeat("x", ChunkSize*3) + "\nbody\n" + strings.Repeat("x", ChunkSize*3), ChunkSize*3 + len("cat <<\nbody\n"), language.KindMeta},
	} {
		m := textedit.NewModel(tc.text)
		e := New(m, tc.id)
		e.SetLanguage(tc.id)
		drain(t, e)
		row := m.LineAt(tc.offset)
		r, _ := m.LineRange(row)
		tokens, _, ok := e.Tokens(row)
		if !ok || kindAt(tokens, tc.offset-r.Start) != tc.kind {
			t.Fatalf("%s late classification: %v", tc.id, tokens)
		}
		e.Advance(time.Time{})
		if e.Err() != nil {
			t.Fatal(e.Err())
		}
	}
}

func TestNewLanguageMalformedFragments(t *testing.T) {
	random := rand.New(rand.NewSource(1741))
	alphabet := []rune("abc012_'\"\\$#&|><-+!?[]{}():,= \t\n你好")
	for _, id := range []language.ID{language.TOML, language.YAML, language.Shell} {
		for round := 0; round < 300; round++ {
			var b strings.Builder
			for n := random.Intn(400); n > 0; n-- {
				b.WriteRune(alphabet[random.Intn(len(alphabet))])
			}
			text := b.String()
			want, ws := scanAll(id, text, 1024)
			got, gs := scanAll(id, text, 1+random.Intn(17))
			if !reflect.DeepEqual(got, want) || !gs.Equal(ws) {
				t.Fatalf("%s malformed input %q: got %v want %v state equal %v", id, text, got, want, gs.Equal(ws))
			}
			previous := 0
			for _, token := range got {
				if token.Start < previous || token.End > len(text) || token.Start >= token.End || !utf8.ValidString(text[token.Start:token.End]) {
					t.Fatalf("%s invalid span: %v in %q", id, token, text)
				}
				previous = token.End
			}
		}
	}
}

func TestLongConfigurationLiterals(t *testing.T) {
	for _, tc := range []struct {
		id   language.ID
		text string
		kind language.Kind
	}{
		{language.TOML, "n = 1979-05-27T07:32:00." + strings.Repeat("1", ChunkSize*2) + "+08:00", language.KindMeta},
		{language.TOML, "n = 0x" + strings.Repeat("f", ChunkSize*2), language.KindNumber},
		{language.TOML, "n = 0b" + strings.Repeat("1", ChunkSize*2), language.KindNumber},
		{language.TOML, "n = " + strings.Repeat("1", ChunkSize*2) + "__2", language.KindInvalid},
		{language.TOML, "n = " + strings.Repeat("1", ChunkSize*2) + "e+", language.KindInvalid},
		{language.YAML, "n: " + strings.Repeat("1", ChunkSize*2), language.KindNumber},
		{language.YAML, "n: " + strings.Repeat("1", ChunkSize*2) + "  # comment", language.KindNumber},
		{language.YAML, "n: true" + strings.Repeat(" ", ChunkSize) + "# comment", language.KindKeyword},
		{language.YAML, "n: " + strings.Repeat("1", ChunkSize) + " 2", language.KindPlain},
	} {
		tokens, _ := scanAll(tc.id, tc.text, ChunkSize)
		if got := kindAt(tokens, 5); got != tc.kind {
			t.Fatalf("%s long literal: got %s want %s", tc.id, got, tc.kind)
		}
	}
}

func TestNewLanguageSnapshotsRemainImmutable(t *testing.T) {
	for _, tc := range []struct {
		id           language.ID
		prefix, next string
	}{
		{language.TOML, "a = [1, { b = ", "true }, 2]"},
		{language.YAML, "key: |\n  first", "next: true"},
		{language.Shell, "cat <<A <<'B'\nfirst", "A"},
	} {
		_, before := scanAll(tc.id, tc.prefix, 3)
		_, _, _ = Scan(tc.id, before, tc.next, len(tc.next), true)
		_, reference := scanAll(tc.id, tc.prefix, 7)
		if !before.Equal(reference) {
			t.Fatalf("%s scan mutated incoming snapshot", tc.id)
		}
	}
}
