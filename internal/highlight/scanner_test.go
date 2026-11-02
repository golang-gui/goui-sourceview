package highlight

import (
	"fmt"
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
		// 内嵌内容现在由 JS/CSS 着色；这些字符仍不开始 HTML 标签。
		{language.HTML, "<script>x</scriptx> <b> y </script><p>z</p>", "b", language.KindString},
		{language.HTML, "<style><b>not a tag</b></STYLE><p>x</p>", "b", language.KindTag},
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
	// 显式撤回也可能没有替代 token（例如未知 Docker 指令恢复 Plain）。
	// 旧夹具适配器用零长度 Plain 操作复用 appendStyled 的范围截断。
	if result.ReplaceFrom != nil {
		tokens = append([]Token{{Start: *result.ReplaceFrom, End: *result.ReplaceFrom}}, tokens...)
	}
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
		{language.JSONC, "/* open\n */ true", "*/", "", "true", language.KindComment},
		{language.JSONLines, "{\n\"root\"\n{\"next\":true}", "{\n", "{", "root", language.KindProperty},
		{language.JSONLines, "{\"key\":1}", "{", "{\n", "key", language.KindString},
		{language.JSONLines, "{\"key\":1}\n\"root\"", "1}", "1", "root", language.KindString},
		{language.CMake, "set(x [=[open\nbody\n]=])\nmessage(done)", "]=]", "", "message", language.KindString},
		{language.Go, "/* open\n body\n */\nvar n=1", "/*", "//", "body", language.KindPlain},
		{language.Python, "x = '''open\nbody\n'''\ny = True", "'''", "'", "body", language.KindPlain},
		{language.JavaScript, "const x = `open\nbody\n`;\nlet y=1", "`", "\"", "body", language.KindPlain},
		{language.SQL, "SELECT $tag$open\nbody\n$tag$;", "$tag$", "'", "body", language.KindString},
		{language.C, "/* open\n body\n */\nreturn 1;", "/*", "//", "body", language.KindPlain},
		{language.C, "// note \\\ncontinued\nint n;", "\\", "", "continued", language.KindPlain},
		{language.Cpp, "auto x = R\"tag(open\nbody\n)tag\";\nreturn 1;", ")tag", ")miss", "return", language.KindString},
		{language.Java, "String x = \"\"\"\nbody\n\"\"\";\nreturn;", "\"\"\"", "\"", "body", language.KindPlain},
		{language.Java, `\u002f\u002f note\u000aint n;`, "000a", "0009", "int", language.KindComment},
		{language.Rust, "/* outer /* inner */\nbody\n*/\nlet x=1;", "/* inner", "inner", "body", language.KindPlain},
		{language.Rust, "let s=r##\"open\nbody\n\"##;\nlet n=1;", "\"##;", "\"#;", "let n", language.KindString},
		{language.CSharp, "var x = \"\"\"\nbody\n\"\"\";\nreturn;", "\"\"\"", "\"", "body", language.KindPlain},
		{language.CSharp, `var s=$$"""raw {{true}}""";`, "$$", "$$$", "true", language.KindString},
		{language.Lua, "local x=[=[open\nbody\n]=]\nreturn x", "]=]", "]==]", "return", language.KindString},
		{language.PowerShell, "$x=@'\nbody\n'@\nfunction f {}", "'@", " '@", "function", language.KindString},
		{language.Ruby, "x=<<~TXT\nbody\n TXT\nclass Demo; end", " TXT", " MISS", "class", language.KindString},
		{language.PHP, "<?php $x=<<<TXT\nbody\nTXT;\nreturn 1;", "TXT;", "MISS;", "return", language.KindString},
		{language.JSX, "const x=<A/>;\nconst n=1;", "/>", ">", "const n", language.KindPlain},
		{language.TSX, "const f=<T,>(x:T)=>x;\nconst n=1;", ",", "", "const n", language.KindPlain},
		{language.Kotlin, "val s=\"\"\"open\nbody\n\"\"\"\nval n=1", "\"\"\"", "\"", "body", language.KindPlain},
		{language.Zig, "const s=\\\\raw\nconst real=true", "\\\\", "/", "raw", language.KindPlain},
		{language.ObjectiveC, "/*open\nbody\n*/\n@end", "/*", "", "body", language.KindPlain},
		{language.ObjectiveCpp, "auto x=R\"(\nbody\n)\";", "R\"", "\"", "body", language.KindPlain},
		{language.Scala, "val s=\"\"\"open\nbody\n\"\"\"", "\"\"\"", "\"", "body", language.KindPlain},
		{language.Perl, "my $x=<<'TXT';\nbody\nTXT\nsub real", "<<'TXT'", "1", "body", language.KindPlain},
		{language.Perl, "=pod\nbody\n=cut\nsub real", "=pod", "", "body", language.KindPlain},
		{language.Swift, "let s=#\"\"\"\nbody\n\"\"\"#\nlet n=1", "\"\"\"", "\"", "body", language.KindPlain},
		{language.Dart, "var s=r'''\nbody\n''';\nvar n=1", "'''", "'", "body", language.KindPlain},
		{language.R, "x <- r\"(body)\"\nreturn(1)", ")\"", ")-\"", "return", language.KindString},
		{language.Diff, "@@ -1 +1 @@\n-old\n+new\n+++ b/next", "-1 +1", "-1,2 +1,2", "+++ b/next", language.KindAdded},
		{language.Batch, "echo ^\nrem body", "^", "", "body", language.KindComment},
		{language.HCL, "value=<<END\nbody\nEND\nresource real {}", "<<END", "1", "body", language.KindPlain},
		{language.GraphQL, "\"\"\"open\nbody\n\"\"\"\ntype Real {}", "\"\"\"", "\"", "body", language.KindProperty},
		{language.SCSS, "/* open\nbody\n*/\n.x{}", "/*", "", "body", language.KindTag},
		{language.Less, "/* open\nbody\n*/\n.x{}", "/*", "", "body", language.KindTag},
	} {
		m := textedit.NewModel(tc.text)
		e := New(m, tc.id)
		e.SetLanguage(tc.id)
		drain(t, e)
		at := strings.Index(tc.text, tc.old)
		if at < 0 {
			t.Fatalf("missing edit fixture %q", tc.old)
		}
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

func TestAdditionalConfigurationKinds(t *testing.T) {
	for _, tc := range []struct {
		id           language.ID
		text, needle string
		kind         language.Kind
	}{
		{language.JSONC, "{\"name\": 1, /* } */\n // note\n \"ok\": true,}", "name", language.KindProperty},
		{language.JSONC, "/* multiline\n comment */ true", "comment", language.KindComment},
		{language.JSONC, "// comment\ntrue", "true", language.KindKeyword},
		{language.JSONLines, "{\n\"root\"", "root", language.KindString},
		{language.JSONLines, "// comment\ntrue", "/", language.KindInvalid},
		{language.JSONLines, "{\"key\":true}\n[1,null]\n42", "key", language.KindProperty},
		{language.Markdown, "```jsonl\n{\n\"root\"\n```", "root", language.KindString},
		{language.HTTP, "POST / HTTP/1.1\nContent-Type: application/x-ndjson\n\n{\n\"root\"", "root", language.KindString},
		{language.JSON, "// comment", "comment", language.KindInvalid},
		{language.CMake, "project(example LANGUAGES CXX)", "project", language.KindKeyword},
		{language.CMake, "set(x [==[a\n${literal}]==])", "${literal}", language.KindString},
		{language.CMake, "#[=[ comment\n more ]=]\nset(x ${HOME})", "more", language.KindComment},
		{language.CMake, "set(x $<IF:$<BOOL:${ON}>,yes,no>)", "$<IF", language.KindVariable},
		{language.Makefile, "include common.mk\nall: build\n\techo $@ $(FLAGS)", "include", language.KindKeyword},
		{language.Makefile, "all: build\n\techo $@ $(FLAGS)", "all", language.KindProperty},
		{language.Makefile, "all: build\n\techo $@ $(FLAGS)", "$(FLAGS)", language.KindVariable},
		{language.Makefile, ".RECIPEPREFIX := >\nall:\n>include recipe-argument", "include", language.KindPlain},
		{language.Nginx, "server {\n listen 8080;\n location / { proxy_pass http://$host; }\n}", "proxy_pass", language.KindKeyword},
		{language.Nginx, "set $name \"hello $host\"; # note", "$host", language.KindVariable},
		{language.INI, "[server]\nport = 8080\n; comment", "server", language.KindTag},
		{language.INI, "[server]\nport = 8080\n; comment", "port", language.KindProperty},
		{language.Dockerfile, "FROM alpine:3\nCOPY --from=build /app /app\nRUN echo $HOME", "COPY", language.KindKeyword},
		{language.Dockerfile, "FROM alpine\nRUN <<'EOF'\n$literal\nEOF\nCMD [\"app\"]", "$literal", language.KindString},
		{language.Dockerfile, "RUN <<EOF\n${HOME}\nEOF\nCMD [\"app\"]", "CMD", language.KindKeyword},
		{language.HTTP, "@host = example.test\n### first\nPOST https://{{host}}/ HTTP/1.1\nContent-Type: application/json\n\n{\"enabled\":true}\n### next\nGET https://example.test/", "POST", language.KindKeyword},
		{language.HTTP, "POST https://example.test/ HTTP/1.1\nContent-Type: application/json\n\n{\"enabled\":true}", "enabled", language.KindProperty},
	} {
		t.Run(string(tc.id)+tc.needle, func(t *testing.T) {
			tokens, _ := scanAll(tc.id, tc.text, 7)
			at := strings.Index(tc.text, tc.needle)
			if got := kindAt(tokens, at); got != tc.kind {
				t.Fatalf("%q: got %s want %s; %v", tc.text, got, tc.kind, tokens)
			}
		})
	}
}

func TestAdditionalConfigurationFragments(t *testing.T) {
	fixtures := map[language.ID][]string{
		language.JSONC:      {"{/*a\n b*/\"name\":\"value\",// }\n\"n\":1,}", "/* incomplete", "{\"long\":\"" + strings.Repeat("名", ChunkSize) + "\"}"},
		language.JSONLines:  {"{\r\n\"root\"\r\ntrue\r\n[1,null]\r\n{\"key\":false}\r\n\n\"escaped\\ntext\"", "{\"long\":\"" + strings.Repeat("名", ChunkSize) + "\"}\n\"root\"\n42"},
		language.CMake:      {"#[==[ comment\n${literal} ]==]\nset(x [=[hello\nworld]=])\nmessage(\"${x}\" $<CONFIG:Debug>)", "[" + strings.Repeat("=", 90) + "[text]" + strings.Repeat("=", 90) + "]"},
		language.Makefile:   {".RECIPEPREFIX := >\nall: a \\\n b\n>echo $(patsubst %.c,%.o,$(SRC)) $$HOME $@\nifdef DEBUG\ninclude debug.mk\nendif"},
		language.Nginx:      {"server {\n listen 8080;\n set $value \"hello ${host}\";\n location / { proxy_pass http://localhost; } # comment\n}"},
		language.INI:        {"; note\n[section]\nkey = \"value\"\nnumber=123\nlong=" + strings.Repeat("a", ChunkSize*2)},
		language.Dockerfile: {"# escape=`\nFROM alpine\nRUN echo hello `\n world\nCOPY --from=build /app /app\nRUN <<A <<-'B'\n${HOME}\nA\n\t$literal\n\tB\nCMD [\"app\"]"},
		language.HTTP:       {"# comment\n@host=example.test\n### first\nPOST https://{{host}}/ HTTP/1.1\nContent-Type: application/json\n\n{\"ok\":true}\n### next\nGET https://example.test/\nAccept: */*"},
	}
	for id, texts := range fixtures {
		for _, text := range texts {
			want, ws := scanAll(id, text, ChunkSize*8)
			for _, chunk := range []int{1, 2, 3, 7, 31, ChunkSize} {
				if len(text) > ChunkSize && chunk < 31 {
					continue
				}
				got, gs := scanAll(id, text, chunk)
				if !reflect.DeepEqual(got, want) || !gs.Equal(ws) {
					t.Fatalf("%s chunk %d: tokens equal=%v state equal=%v\ngot %v\nwant %v", id, chunk, reflect.DeepEqual(got, want), gs.Equal(ws), got, want)
				}
			}
		}
	}
}

func TestCodeLanguageKinds(t *testing.T) {
	for _, tc := range []struct {
		id           language.ID
		text, needle string
		kind         language.Kind
	}{
		{language.Go, "package main\nvar value = `raw\ntext` // note", "package", language.KindKeyword},
		{language.Go, "x := 0x1.fp+2i; r := '\\n'", "0x1", language.KindNumber},
		{language.Go, "var x = `raw\ntext`", "text", language.KindString},
		{language.Protobuf, "syntax = \"proto3\";\nmessage Item { optional string name = 1; }", "optional", language.KindKeyword},
		{language.Python, "@decorator\nasync def hello(name):\n return f\"Hello {name.upper()}!\"", "@decorator", language.KindMeta},
		{language.Python, "x = r'''raw\ntext'''\ny=True", "text", language.KindString},
		{language.Python, "x = f\"{True} {{literal}}\"", "True", language.KindKeyword},
		{language.JavaScript, "const pattern = /[/]+\\w/gu; let ratio = 10 / 2;", "/[/]", language.KindString},
		{language.JavaScript, "if (ready) /ok/.test(value);", "/ok/", language.KindString},
		{language.JavaScript, "const x = `hello ${true ? `nested ${1}` : name}`;", "true", language.KindKeyword},
		{language.JavaScript, "let ratio = 10 / 2;", "/", language.KindDelimiter},
		{language.TypeScript, "interface Item<T> { readonly value: T; }\ntype Name = string;", "readonly", language.KindKeyword},
		{language.JavaScript, "let interfaceName = 1; interface", "interface", language.KindPlain},
		{language.SQL, "select \"name\" from items where id = :id AND title = 'it''s fine'", "select", language.KindKeyword},
		{language.SQL, "SELECT $tag$first\n'raw' /* text */\n$tag$;", "/* text", language.KindString},
		{language.SQL, "/* nested /* comment */ still */ SELECT 1;", "still", language.KindComment},
		{language.SQL, "SELECT value::text, :name FROM items", "text", language.KindKeyword},
		{language.CSS, "@media (min-width: 600px) { .card:hover { --accent: #ffaa00; color: var(--accent); margin: -10px; } }", "@media", language.KindMeta},
		{language.CSS, ".card { border-radius: 6px; color: #ffaa00; }", "border-radius", language.KindProperty},
		{language.CSS, ".card { --accent: #ffaa00; }", "--accent", language.KindVariable},
		{language.CSS, ".card { margin: -10px; }", "-10px", language.KindNumber},
		{language.HTML, "<script>const n = 42; // note\nlet x = `hi ${true}`;</script><style>.x { color: red; }</style>", "const", language.KindKeyword},
		{language.HTML, "<script>const s = \"</script>\";<p>outside</p>", "outside", language.KindPlain},
		{language.HTML, "<style>.x { color: red; }</style>", "color", language.KindProperty},
		{language.HTML, "<script type='application/json'>true</script>", "true", language.KindPlain},
		{language.HTML, "<script TYPE=module>const n=1</script>", "const", language.KindKeyword},
	} {
		t.Run(string(tc.id)+tc.needle, func(t *testing.T) {
			tokens, _ := scanAll(tc.id, tc.text, 3)
			at := strings.Index(tc.text, tc.needle)
			if tc.needle == "interface" {
				at = strings.LastIndex(tc.text, tc.needle)
			}
			if got := kindAt(tokens, at); got != tc.kind {
				t.Fatalf("%q: got %s want %s; %v", tc.text, got, tc.kind, tokens)
			}
		})
	}
}

func TestCodeLanguageFragments(t *testing.T) {
	fixtures := map[language.ID][]string{
		language.Go:         {"package main\n/* block\n comment */\nfunc main(){var x=0x1.fp-2i; s:=`raw\ntext`;r:='\\u1234'}"},
		language.Protobuf:   {"syntax='proto3';\n// note\nmessage 名 { repeated string value=1; /*c*/ }"},
		language.Python:     {"@decorator\nasync def hi(name):\n return rf'''raw\n{name + f\"{True}\"} {{literal}}''' # note\nx=0xFF + 1.2e-3j", "x='''" + strings.Repeat("名", ChunkSize) + "'''\ny=True"},
		language.JavaScript: {"const pattern=/[/]+\\w/gu; let x=42 / 2;\nif (ready) /a/.test(x);\nconst text=`outer ${true ? `inner ${1}` : name} end`; /*x\ny*/", "const " + strings.Repeat("x", ChunkSize*2) + "=true;"},
		language.TypeScript: {"interface Item<T> { readonly value: T; }\nconst x: string = `hi ${true}`; type X = keyof Item<number>;"},
		language.SQL:        {"SELECT \"name\", [column], `other`, 'it''s fine', $tag$raw\n${literal} 'quotes'\n$tag$ FROM items WHERE id=:id AND n=$1; /* outer /*inner*/ end */", "SELECT $" + strings.Repeat("x", 90) + "$body$" + strings.Repeat("x", 90) + "$;"},
		language.CSS:        {"@media (min-width: 600px) { .card:hover { --accent: #ffaa00; color: var(--accent); margin: -10px; background: url(https://example.test/a); } } /*c\nd*/"},
		language.HTML:       {"<script>const s=`a ${true}`; /* block\n comment */\nlet x=1 / 2; if(x) /a/.test(s);</script><style>@media (width>1px){.x{color:red}}</style>", "<script>\"</script>\"<p>x</p>", "<script><!--\nvar n=1;\n<script>double</script>\n-->\n</script>"},
	}
	for id, texts := range fixtures {
		for _, text := range texts {
			want, ws := scanAll(id, text, ChunkSize*8)
			for _, chunk := range []int{1, 2, 3, 7, 31, ChunkSize} {
				if len(text) > ChunkSize && chunk < 31 {
					continue
				}
				got, gs := scanAll(id, text, chunk)
				if !reflect.DeepEqual(got, want) || !gs.Equal(ws) {
					t.Fatalf("%s chunk %d: tokens equal=%v state equal=%v\ngot %v\nwant %v", id, chunk, reflect.DeepEqual(got, want), gs.Equal(ws), got, want)
				}
			}
		}
	}
}

func TestAddedLanguageMalformedFragments(t *testing.T) {
	random := rand.New(rand.NewSource(20261027))
	alphabet := []rune("abc012_'\"`\\$#&|><-+!?[]{}():,;=/ \t\n你好")
	for _, id := range []language.ID{language.JSONC, language.JSONLines, language.HTTP, language.Makefile, language.CMake, language.Nginx, language.INI, language.Dockerfile, language.SQL, language.Protobuf, language.Go, language.Python, language.JavaScript, language.TypeScript, language.CSS, language.HTML} {
		for round := 0; round < 100; round++ {
			var b strings.Builder
			for n := random.Intn(400); n > 0; n-- {
				b.WriteRune(alphabet[random.Intn(len(alphabet))])
			}
			text := b.String()
			want, ws := scanAll(id, text, 1024)
			got, gs := scanAll(id, text, 1+random.Intn(17))
			if !reflect.DeepEqual(got, want) || !gs.Equal(ws) {
				t.Fatalf("%s malformed %q: got %v want %v states equal=%v\ngot state %#v\nwant state %#v", id, text, got, want, gs.Equal(ws), gs.raw, ws.raw)
			}
			previous := 0
			for _, token := range got {
				if token.Start < previous || token.End > len(text) || token.Start >= token.End || !utf8.ValidString(text[token.Start:token.End]) {
					t.Fatalf("%s invalid span %v in %q", id, token, text)
				}
				previous = token.End
			}
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
		{language.Ruby, "puts(<<A, <<B)\n#{true", "}"},
		{language.JSX, "const x=<A>{true ? <B title={", "1}/> : null}"},
		{language.Kotlin, `val s="outer ${"inner ${`, "true}"},
		{language.Scala, `val s=s"outer ${s"inner ${`, "true}"},
		{language.Perl, "print <<A, <<'B';\nbody", "A"},
		{language.Perl, "my $x=<<'long", "tail'"},
		{language.HCL, `value="outer ${"inner ${`, "true}"},
		{language.SCSS, `.x{content:"outer #{"inner #{`, "true}"},
		{language.Less, `.x{content:"@{long`, "name}"},
		{language.Swift, `let s="outer \("inner \(`, "true)"},
		{language.Dart, `var s='outer ${"inner ${`, "true}"},
		{language.R, `x <- r"--[open`, "body]--\""},
	} {
		_, before := scanAll(tc.id, tc.prefix, 3)
		_, _, _ = Scan(tc.id, before, tc.next, len(tc.next), true)
		_, reference := scanAll(tc.id, tc.prefix, 7)
		if !before.Equal(reference) {
			t.Fatalf("%s scan mutated incoming snapshot", tc.id)
		}
	}
}

func TestFifthBatchKinds(t *testing.T) {
	for _, tc := range []struct {
		id           language.ID
		text, needle string
		kind         language.Kind
	}{
		{language.Diff, "--- a/file\n+++ b/file\n@@ -1 +1 @@\n--- old\n+++ new\n--- a/next", "--- old", language.KindRemoved},
		{language.Diff, "--- a/file\n+++ b/file\n@@ -1 +1 @@\n--- old\n+++ new\n--- a/next", "+++ new", language.KindAdded},
		{language.Diff, "@@ -1 +1 @@\n-old\n+new\n--- a/next", "--- a/next", language.KindHeading},
		{language.Diff, "@@ -0,0 +1,2 @@\n+new\n+again\n+++ b/next", "+++ b/next", language.KindHeading},
		{language.Diff, "diff --git a/a b/a", "diff", language.KindMeta},
		{language.Diff, "\\ No newline at end of file", "No newline", language.KindComment},
		{language.Batch, "@EcHo off\nREM comment\necho rem argument", "EcHo", language.KindKeyword},
		{language.Batch, "@EcHo off\nREM comment\necho rem argument", "comment", language.KindComment},
		{language.Batch, "echo rem argument", "rem", language.KindPlain},
		{language.Batch, "echo ^& rem argument", "rem", language.KindPlain},
		{language.Batch, "echo ^\nrem continued\nrem actual", "continued", language.KindPlain},
		{language.Batch, "echo ^\nrem continued\nrem actual", "actual", language.KindComment},
		{language.Batch, ":build\n:: note\necho \"%NAME% !count!\"", ":build", language.KindTag},
		{language.Batch, "echo \"%NAME% !count!\"", "%NAME%", language.KindVariable},
		{language.Batch, "for %%i in (*) do echo %%~fi %~dp0 %1 %*", "%%~fi", language.KindVariable},
		{language.Batch, "for %%i in (*) do echo %%~fi %~dp0 %1 %*", "%~dp0", language.KindVariable},
		{language.Batch, "echo \"unclosed\nrem actual", "actual", language.KindComment},
		{language.HCL, `resource "local_file" "x" { content = "${true ? "yes" : "no"}" }`, "content", language.KindProperty},
		{language.HCL, `value="${true ? "yes" : "no"}"`, "true", language.KindKeyword},
		{language.HCL, `value="$${true} %%{if false}"`, "true", language.KindString},
		{language.HCL, `value="%{ if true }yes%{ endif }"`, "endif", language.KindKeyword},
		{language.HCL, "value=<<END\n\\n $${true}\nEND\nresource real {}", "true", language.KindString},
		{language.HCL, "value=<<END\n\\n $${true}\nEND\nresource real {}", "resource", language.KindKeyword},
		{language.HCL, "value=<<-END\n${jsonencode({x=true})}\n  END  \nresource real {}", "true", language.KindKeyword},
		{language.HCL, "value=<<-END\n${jsonencode({x=true})}\n  END  \nresource real {}", "resource", language.KindKeyword},
		{language.HCL, "value=<<OUTER\n${join(\"\",[<<INNER\nbody\nINNER\n])}\nOUTER\nresource real {}", "resource", language.KindKeyword},
		{language.GraphQL, `query Q($id: ID!) { user(id:$id) @skip(if:false) { ...Details } }`, "$id", language.KindVariable},
		{language.GraphQL, `query Q($id: ID!) { user(id:$id) @skip(if:false) { ...Details } }`, "@skip", language.KindMeta},
		{language.GraphQL, `query Q { user { __typename } }`, "__typename", language.KindProperty},
		{language.GraphQL, "\"\"\"description \\\"\"\" literal\nbody\n\"\"\"\ntype User { id:ID! }", "body", language.KindString},
		{language.GraphQL, "\"\"\"description \\\"\"\" literal\nbody\n\"\"\"\ntype User { id:ID! }", "type", language.KindKeyword},
		{language.GraphQL, `query Q { field(text:"\u{1F600}\u0041") }`, "1F600", language.KindEscape},
		{language.GraphQL, "# comment\nquery Q { field(x:-1.2e+3) }", "-1.2e+3", language.KindNumber},
		{language.GraphQL, "\"unfinished\nquery Q {}", "query", language.KindKeyword},
		{language.SCSS, `$accent: #336699; @mixin box($size){width:$size}`, "$accent", language.KindVariable},
		{language.SCSS, `$accent: #336699; @mixin box($size){width:$size}`, "@mixin", language.KindMeta},
		{language.SCSS, `.x {content:"literal $name #{true}"}`, "$name", language.KindString},
		{language.SCSS, `.x {content:"literal $name #{true}"}`, "true", language.KindKeyword},
		{language.SCSS, `.x {content:"\#{true}"}`, "true", language.KindString},
		{language.SCSS, `.x {content:"\1F600 smile"}`, "1F600", language.KindEscape},
		{language.SCSS, `.x { background:url(http://example.test/#{$name}.png); color:red }`, "http://", language.KindString},
		{language.SCSS, `.x { background:url(http://example.test/#{$name}.png); color:red }`, "$name", language.KindVariable},
		{language.SCSS, `.x { background:url(http://example.test/#{$name}.png); color:red }`, "color", language.KindProperty},
		{language.SCSS, `/* comment #{true} tail */ .x{color:red}`, "tail", language.KindComment},
		{language.SCSS, `/* comment #{1 /*inner*/ + 2} tail */ .x{color:red}`, "color", language.KindProperty},
		{language.SCSS, `.x{.child:hover{color:red} padding:1.5rem;}`, "padding", language.KindProperty},
		{language.Less, `@color:#369; .x {color:@color; width:10px}`, "@color", language.KindVariable},
		{language.Less, `@import "theme.less"; @media screen {.x {color:red}}`, "@import", language.KindMeta},
		{language.Less, `@import "theme.less"; @media screen {.x {color:red}}`, "@media", language.KindMeta},
		{language.Less, `.@{name}{content:~"Hello @{name}";color:@@ref}`, "@{name}", language.KindVariable},
		{language.Less, `.@{name}{content:~"Hello @{name}";color:@@ref}`, "@@ref", language.KindVariable},
		{language.Less, `.x{width:$height; background:url(http://example.test/@{name}.png); color:red}`, "$height", language.KindVariable},
		{language.Less, `.x{background:url(http://example.test/@{name}.png); color:red}`, "http://", language.KindString},
		{language.Less, ".x{width:`(1 + 2)`; color:red}", "(1 + 2)", language.KindString},
		{language.Less, `.x{.child:hover{color:red} padding:1rem;}`, "padding", language.KindProperty},
	} {
		for _, chunk := range []int{1, 3, 31, ChunkSize} {
			tokens, _ := scanAll(tc.id, tc.text, chunk)
			if got := kindAt(tokens, strings.Index(tc.text, tc.needle)); got != tc.kind {
				t.Fatalf("%s %q chunk %d: %s want %s", tc.id, tc.needle, chunk, got, tc.kind)
			}
		}
	}
}

func TestFifthBatchFragments(t *testing.T) {
	fixtures := map[language.ID][]string{
		language.Less:    {"@name: editor; @ref: name; .@{name} {content:~\"Hello @{name}\"; color:@@ref; background:url(http://example.test/@{name}.png); .child:hover{color:red} padding:1rem; width:`1 + 2`}\n@media screen {.x{height:2px}} // line", ".@{" + strings.Repeat("v", ChunkSize) + "}{content:\"" + strings.Repeat("字", ChunkSize) + "@{name}\"}", "/* note */ @" + strings.Repeat("v", ChunkSize) + ": 1; .x{content:\"\\1F600 smile\"}"},
		language.SCSS:    {"$name: editor; @mixin box($x){width:$x} .#{$name}{content:\"#{if(true,\"#{1}\",\"no\")}\"; background:url(http://example.test/#{$name}.png); .child:hover{color:#abc} padding:1.5rem} /* comment #{1 /*nested*/ + 2} tail */ // line\n.x {content:\"\\1F600 smile\"}", ".x {content:\"" + strings.Repeat("字", ChunkSize) + "#{true}\"; background:url(" + strings.Repeat("a", ChunkSize) + "/#{$name}.png)}", "$" + strings.Repeat("v", ChunkSize) + ": 1; @" + strings.Repeat("m", ChunkSize) + " {}"},
		language.GraphQL: {"query Q($id:ID!){ user(id:$id) @skip(if:false){ ...Details } }\nfragment Details on User { name }\n\"\"\"description \\\"\"\" literal\nbody\n\"\"\"\ntype User { id:ID! }", "query Q { field(text:\"\\u{1F600}\\u0041\") }\n\"\"\"" + strings.Repeat("字\\", ChunkSize) + "\"\"\"\nquery Q {}", "query " + strings.Repeat("q", ChunkSize) + "($" + strings.Repeat("v", ChunkSize) + ":Int){}"},
		language.HCL:     {"resource \"local_file\" \"x\" {\nname-with-dash = \"${true ? \"${1}\" : \"no\"}\"\nbody = <<-END\n$${literal} %{ if true ~}\n${jsonencode({a=\"b\"})}\n%{ endif }\n END\n}\n/* end */", "value=<<OUTER\n${join(\"\",[<<INNER\nbody\nINNER\n])}\nOUTER\nresource real {}", "value=<<" + strings.Repeat("字", ChunkSize) + "\n${true}\n" + strings.Repeat("字", ChunkSize) + "\nresource real {}", strings.Repeat("k", ChunkSize) + " = \"" + strings.Repeat("字", ChunkSize) + "${true}\""},
		language.Batch:   {"@echo off\nsetlocal EnableDelayedExpansion\nREM note\n:build\n:: note\nfor %%i in (*.txt) do (\n echo \"%%~fi %~dp0 %NAME:~1,2% !count!\"\n)\necho ^\nrem continuation\nrem actual", "echo \"" + strings.Repeat("字", ChunkSize) + "%NAME%\"\n" + strings.Repeat("r", ChunkSize) + " argument\nrem final"},
		language.Diff:    {"diff --git a/a b/a\n--- a/a\n+++ b/a\n@@ -1,2 +1,2 @@ function\n--- old\n+++ new\n same\n--- a/b\n+++ b/b\n@@ -0,0 +1 @@\n+new\n\\ No newline at end of file", "@@ -1 +1 @@ " + strings.Repeat("字", ChunkSize) + "\n-" + strings.Repeat("x", ChunkSize*2) + "\n+new", "@@ -1," + strings.Repeat("9", ChunkSize) + " +1 @@\n+new"},
	}
	for id, texts := range fixtures {
		for _, text := range texts {
			want, ws := scanAll(id, text, ChunkSize*8)
			for _, chunk := range []int{1, 2, 3, 7, 31, ChunkSize} {
				got, gs := scanAll(id, text, chunk)
				if !reflect.DeepEqual(got, want) || !gs.Equal(ws) {
					t.Fatalf("%s chunk %d: tokens=%v states=%v", id, chunk, reflect.DeepEqual(got, want), gs.Equal(ws))
				}
			}
		}
	}
}

func TestFourthBatchKinds(t *testing.T) {
	for _, tc := range []struct {
		id           language.ID
		text, needle string
		kind         language.Kind
	}{
		{language.Zig, `const x: u4096 = @intCast(1);`, "u4096", language.KindKeyword},
		{language.ObjectiveC, `@interface Demo : NSObject @end`, "@interface", language.KindKeyword},
		{language.Scala, `val s="literal $name";`, "$name", language.KindString},
		{language.Perl, `my $s="hi $name ${other}";`, "$name", language.KindVariable},
		{language.Perl, `my $s=q{literal $name};`, "$name", language.KindString},
		{language.Perl, `my $s=qq{hi $name};`, "$name", language.KindVariable},
		{language.Perl, `$x =~ s{[{}]}{qq $name}g; my $n=10 / 2;`, "$name", language.KindVariable},
		{language.Perl, `$x =~ s{[{}]}{qq $name}g; my $n=10 / 2;`, "/ 2", language.KindDelimiter},
		{language.Perl, `my $x=qr'no $name';`, "$name", language.KindString},
		{language.Perl, `my $r=qr/^[\$]foo$/; sub real {}`, "sub", language.KindKeyword},
		{language.Perl, `my $r=qr{[}]}; sub real {}`, "sub", language.KindKeyword},
		{language.Perl, `my $r=qr/$]/;`, "$]", language.KindVariable},
		{language.Perl, `my $r=qr{foo$|bar$}; sub real {}`, "sub", language.KindKeyword},
		{language.Perl, `my $s=q§a\§b§; sub real {}`, "sub", language.KindKeyword},
		{language.Perl, `sub q { return 1; } $obj->m(2);`, "return", language.KindKeyword},
		{language.Perl, `my %h=(q => 1); $h{m};`, "q", language.KindPlain},
		{language.Perl, "=pod\nsub fake\n=cut\nsub real", "fake", language.KindComment},
		{language.Perl, "__DATA__\nsub fake", "fake", language.KindString},
		{language.Perl, "my $x=<<'END';\n$name literal\nEND\nsub real", "$name", language.KindString},
		{language.Perl, "my $x=<<~END;\n  $name\n  END\nsub real", "$name", language.KindVariable},
		{language.Perl, `my $x=qq{outer {nested} $name}; sub real {}`, "sub", language.KindKeyword},
		{language.Perl, `$x =~ s/old/new $name/g; sub real {}`, "sub", language.KindKeyword},
		{language.Perl, `$x =~ s{old} [new $name]g; sub real {}`, "sub", language.KindKeyword},
		{language.Perl, `$x =~ tr[a-z][A-Z]; sub real {}`, "sub", language.KindKeyword},
		{language.Perl, `$x =~ y/abc/def/; sub real {}`, "sub", language.KindKeyword},
		{language.Perl, `$x =~ s'old'new $name'g;`, "$name", language.KindString},
		{language.Perl, `my $x=qq'hello $name';`, "$name", language.KindVariable},
		{language.Perl, "print <<A, <<~'B';\n$name\nA\n literal $name\n B\nsub real {}", "sub", language.KindKeyword},
		{language.Perl, "my $x=q # note\n {literal}; sub real {}", "sub", language.KindKeyword},
		{language.Perl, "my $x=<< 1; sub real {}", "sub", language.KindKeyword},
		{language.Scala, `val s=s"hello $name ${if (true) s"${1}" else "no"}"`, "if", language.KindKeyword},
		{language.Scala, `val s=raw"\${false}";`, "false", language.KindKeyword},
		{language.Scala, `val s=s"quote $" dollar $$"; val x=1`, "val x", language.KindKeyword},
		{language.Scala, "val symbol='name; val c='字'; val code='{true}", "true", language.KindKeyword},
		{language.Scala, "val `class`=1; /* outer /* inner */ still */", "class", language.KindVariable},
		{language.Scala, "/* outer /* inner */ still */ val x=1", "still", language.KindComment},
		{language.ObjectiveCpp, `template<class T> id box(T x){return @(x);} auto s=R"tag(@interface literal)tag";`, "template", language.KindKeyword},
		{language.ObjectiveCpp, `auto s=R"tag(@interface literal)tag"; @implementation Demo @end`, "literal", language.KindString},
		{language.ObjectiveCpp, `auto s=R"tag(@interface literal)tag"; @implementation Demo @end`, "@implementation", language.KindKeyword},
		{language.ObjectiveC, `id s=@"a {literal}"; NSArray *a=@[@42,@YES];`, "literal", language.KindString},
		{language.ObjectiveC, `id s=@"a {literal}"; NSArray *a=@[@42,@YES];`, "@42", language.KindNumber},
		{language.ObjectiveC, `NSArray *a=@[@-42,@YES];`, "@YES", language.KindKeyword},
		{language.ObjectiveC, "#import <Foundation/Foundation.h>", "Foundation.h", language.KindString},
		{language.Zig, `const @"a b" = @import("std");`, "a b", language.KindVariable},
		{language.Zig, `const x=@import("std");`, "@import", language.KindMeta},
		{language.Zig, "const x=\\\\${raw} \\n\nconst y=1;", "${raw}", language.KindString},
		{language.Zig, "/* not a comment */ const y=1;", "const", language.KindKeyword},
		{language.Zig, `const x="\u{1F600} \x41";`, "1F600", language.KindEscape},
		{language.Zig, "//! documentation\nconst x=0x1.fp+2;", "documentation", language.KindComment},
	} {
		for _, chunk := range []int{1, 3, 31, ChunkSize} {
			tokens, _ := scanAll(tc.id, tc.text, chunk)
			if got := kindAt(tokens, strings.Index(tc.text, tc.needle)); got != tc.kind {
				t.Fatalf("%s %q chunk %d: %s want %s", tc.id, tc.needle, chunk, got, tc.kind)
			}
		}
	}
}

func TestFourthBatchFragments(t *testing.T) {
	fixtures := map[language.ID][]string{
		language.Perl:         {"use strict; my %h=(q => 1); sub q { return 1; } $obj->m(2); my $s=qq{hi $name ${other}}; my $r=qr{[{}]\\d+}i; $s =~ s{old}{new $name}g; $s =~ tr/a-z/A-Z/; my $x=10 / 2; my $y=//;", "print <<A, <<~'B';\n$name\nA\n literal $name\n B\n=pod\nignored\n=cut\nsub real {}\n__END__\ndata", "my $s=qq{" + strings.Repeat("字\\}", ChunkSize) + " $name}; my $r=m/[\\]{}]+/;", "my $s=<<'" + strings.Repeat("字", ChunkSize) + "';\nbody\n" + strings.Repeat("字", ChunkSize) + "\nsub real;", "my $s=q" + strings.Repeat(" ", ChunkSize*2) + "{body}; $s =~ s{old} # comment\n {new $name}g;"},
		language.Scala:        {"@deprecated(\"note\")\n/* outer /* nested */ still */\nval s=s\"hi $name ${if (true) s\"${1}\" else raw\"\\${false}\"}\"; val t=s\"\"\"\n${List(1,2)} $$ $\"\n\"\"\"\"\"; val `type`='symbol; val ch='字'; val code='{true}", "val s=custom\"\"\"" + strings.Repeat("字\\", ChunkSize) + "${true}" + strings.Repeat("\"", 50) + "; val x=42.toString"},
		language.ObjectiveCpp: {"#import <Foundation/Foundation.h>\n#include <vector>\ntemplate<class T> id box(T x){return @(x);} auto s=u8R\"tag(raw\n@interface fake )wrong\" still)tag\"; @interface Real : NSObject @end", "auto s=R\"0123456789abcdef(" + strings.Repeat("字@\"", ChunkSize) + ")0123456789abcdef\"; @end"},
		language.ObjectiveC:   {"#import <Foundation/Foundation.h>\n@interface Demo : NSObject\n@property(copy) NSString *name;\n@end\n@implementation Demo\n- (id)value { @autoreleasepool { return @{ @\"key\": @[@42,@-1,@YES,@(1+2)] }; } }\n@end", "id s=@\"" + strings.Repeat("字\\n", ChunkSize) + "\"; /* comment */ return nil;"},
		language.Zig:          {"//! module\nconst @\"name 字\"=\\\\body ${raw} \\n\n\\\\continued\nconst n:u12=0x1.fp+2; const c='\\u{1234}'; const s=\"\\x41\";", "const s=\\\\" + strings.Repeat("字\\{}", ChunkSize) + "\nconst yes=true;"},
	}
	for id, texts := range fixtures {
		for _, text := range texts {
			want, ws := scanAll(id, text, ChunkSize*8)
			for _, chunk := range []int{1, 2, 3, 7, 31, ChunkSize} {
				got, gs := scanAll(id, text, chunk)
				if !reflect.DeepEqual(got, want) || !gs.Equal(ws) {
					t.Fatalf("%s chunk %d: tokens=%v states=%v\ngot %v\nwant %v\ngot %#v want %#v", id, chunk, reflect.DeepEqual(got, want), gs.Equal(ws), got, want, gs.raw, ws.raw)
				}
			}
		}
	}
}

func TestProgrammingLanguageKinds(t *testing.T) {
	for _, tc := range []struct {
		id           language.ID
		text, needle string
		kind         language.Kind
	}{
		{language.C, "#include <stdio.h>\nint main(void) { return 0x1.fp+2f; }", "include", language.KindMeta},
		{language.C, "#include <stdio.h>", "stdio", language.KindString},
		{language.C, "const char *s = u8\"hello\";", "u8", language.KindString},
		{language.C, "unsigned n=0b1010'0011u;", "0b1010'0011u", language.KindNumber},
		{language.C, "// note \\\\\ncontinued\nint x;", "continued", language.KindComment},
		{language.C, "#define SUM(a,b) \\\n ((a)+(b))\nint n;", "int", language.KindKeyword},
		{language.Cpp, "constexpr auto text = u8R\"tag(raw \"text\"\n)no\" still)tag\";", "still", language.KindString},
		{language.Cpp, "auto x = R\"(raw)\"; return 1'000ULL;", "return", language.KindKeyword},
		{language.Cpp, "auto x = R\"(raw)\"; return 1'000ULL;", "1'000", language.KindNumber},
		{language.Java, "@Override public String text() { return \"hi\"; }", "@Override", language.KindMeta},
		{language.Java, "cl\\u0061ss Demo {}", "cl", language.KindKeyword},
		{language.Java, "double n=\\u002e5e-2;", "\\u002e", language.KindNumber},
		{language.Java, "\\u002f\\uu002f comment\\u000aint n;", "comment", language.KindComment},
		{language.Java, "\\u002f\\uu002f comment\\u000aint n;", "int", language.KindKeyword},
		{language.Java, "String x=\"\"\"\nraw \" quoted\n\"\"\"; return;", "quoted", language.KindString},
		{language.Java, "String x=\"\\\\u0022 still\";", "still", language.KindString},
		{language.Rust, "fn get<'a>(x: &'a str) -> &'a str { x }", "'a", language.KindVariable},
		{language.Rust, "let ch='字'; let raw=r#type;", "字", language.KindString},
		{language.Rust, "let raw=r#type;", "type", language.KindPlain},
		{language.Rust, "let x = cr##\"raw \"# still\"##;", "still", language.KindString},
		{language.Rust, "/* outer /* inner */ still */ let x=1;", "still", language.KindComment},
		{language.Rust, "println!(\"ok\");", "println", language.KindMeta},
		{language.Rust, "let x=1..2;", "..", language.KindDelimiter},
		{language.CSharp, "#nullable enable\npublic class Demo {}", "nullable", language.KindMeta},
		{language.CSharp, "var @class = @\"a\"\"b\";", "class", language.KindPlain},
		{language.CSharp, "var s = $@\"Hi {true ? $\"{1}\" : @\"x\"}!\";", "true", language.KindKeyword},
		{language.CSharp, "var s = $$\"\"\"literal {brace} {{{true}}}\"\"\";", "true", language.KindKeyword},
		{language.CSharp, "var s = $$\"\"\"literal {brace} {{{true}}}\"\"\";", "brace", language.KindString},
		{language.CSharp, "var s = \"\"\"\"\nraw \"\"\" text\n\"\"\"\"; return;", "text", language.KindString},
		{language.CSharp, "var s = $\"{1:0.00}\";", "0.00", language.KindString},
		{language.CSharp, "var s = $\"{value ?? 1:0.00} {global::System.String.Empty}\";", "0.00", language.KindString},
		{language.CSharp, "var range=1..2;", "..", language.KindDelimiter},
		{language.Lua, "local text=[==[raw ]=] still\n]==]\nreturn text", "still", language.KindString},
		{language.Lua, "--[=[ note\nbody\n]=]\nlocal n=0x1.fp+2", "body", language.KindComment},
		{language.Lua, "local x='a\\z \n  b'\nreturn x", "b", language.KindString},
		{language.Lua, "local n=0x1.fp+2; return n", "0x1", language.KindNumber},
		{language.Lua, "local x=1..2; return x", "..", language.KindDelimiter},
		{language.PowerShell, "FuNcTiOn Test { param($Name) if ($Name -eq 'x') {} }", "FuNcTiOn", language.KindKeyword},
		{language.PowerShell, "if ($n -eq 1) { Write-Output -Name $env:PATH }", "-eq", language.KindKeyword},
		{language.PowerShell, "Write-Output -Name $env:PATH", "-Name", language.KindAttribute},
		{language.PowerShell, "Write-Output -Name $env:PATH", "$env:PATH", language.KindVariable},
		{language.PowerShell, "$x=\"hello $name$(if ($true) { \"nested $value\" })\"", "if", language.KindKeyword},
		{language.PowerShell, "$x=@'\n$name $(if)\n'@", "$name", language.KindString},
		{language.PowerShell, "$x=@\"\n$name $(if ($true) { 1 })\n\"@", "if", language.KindKeyword},
		{language.PowerShell, "$x=@'\n '@\nstill\n'@", "still", language.KindString},
		{language.PowerShell, "<# outer <# inner #> function f {}", "function", language.KindKeyword},
		{language.PowerShell, "$x='first\nsecond'; function f {}", "second", language.KindString},
		{language.PowerShell, "Write-Output ‘foo#bar’; function f {}", "foo", language.KindString},
		{language.Ruby, "class Demo\n def hello(name)\n  \"hi #{true ? name : 'x'}\"\n end\nend", "class", language.KindKeyword},
		{language.Ruby, "x=%q{literal #{true} {nested}}", "true", language.KindString},
		{language.Ruby, "x=%Q{interpolate #{true} {nested}}", "true", language.KindKeyword},
		{language.Ruby, "x=%r{[/]+\\w{2}}im; y=10 / 2", "[/]", language.KindString},
		{language.Ruby, "x=10 / 2", "/", language.KindDelimiter},
		{language.Ruby, "puts(<<A, <<-'B')\n#{true}\nA\n#{false}\n B\nclass Demo;end", "true", language.KindKeyword},
		{language.Ruby, "puts(<<A, <<-'B')\n#{true}\nA\n#{false}\n B\nclass Demo;end", "false", language.KindString},
		{language.Ruby, "puts(<<A, <<-'B')\n#{true}\nA\n#{false}\n B\nclass Demo;end", "class", language.KindKeyword},
		{language.Ruby, "x=<<A\n#{puts <<B}\ninner\nB\nouter\nA\nclass Demo;end", "inner", language.KindString},
		{language.Ruby, "x=<<A\n#{puts <<B}\ninner\nB\nouter\nA\nclass Demo;end", "class", language.KindKeyword},
		{language.Ruby, "=begin\nclass Fake\n=end\nclass Real;end", "Fake", language.KindComment},
		{language.Ruby, "__END__\nclass data", "class", language.KindString},
		{language.Ruby, "x=:name; @@value=$name; x=1..2", "name", language.KindProperty},
		{language.Ruby, `x=?\u{5b57}; x!=y`, `?\u{5b57}`, language.KindString},
		{language.PHP, `<?php readonly class Demo { public function f() { return true; } }`, "readonly", language.KindKeyword},
		{language.PHP, `<p title="<?= $name ?>">Hi</p>`, "$name", language.KindVariable},
		{language.PHP, `<script>const<?php echo ' '; ?> value=1;</script>`, "const", language.KindKeyword},
		{language.PHP, `<script>const<?php echo ' '; ?> value=1;</script>`, "echo", language.KindKeyword},
		{language.PHP, `<script>let text="<?= $name ?>";</script>`, "$name", language.KindVariable},
		{language.PHP, `<!-- <?= $name ?> -->`, "$name", language.KindVariable},
		{language.PHP, `<?php // note ?><b>outside</b>`, "outside", language.KindPlain},
		{language.PHP, `<?php /* ?> still */ return;`, "still", language.KindComment},
		{language.PHP, "<?php $x=<<<TXT\n$name {$row['key']}\n TXT, true;", "true", language.KindKeyword},
		{language.PHP, "<?php $x=<<<'TXT'\n$name {$row['key']}\n TXT;", "$name", language.KindString},
		{language.PHP, "<?php $x=<<<TXT\nTXT_MORE\nstill\nTXT;", "still", language.KindString},
		{language.PHP, "<?php #[Route('/')] function f() {}", "Route", language.KindPlain},
		{language.Markdown, "```php\nreturn true;\n```", "return", language.KindKeyword},
		{language.JSX, `const x=<Panel title="a>b" enabled>{true ? <Item/> : null}</Panel>;`, "Panel", language.KindTag},
		{language.JSX, `const x=<Panel title="a>b" enabled>{true ? <Item/> : null}</Panel>;`, "title", language.KindAttribute},
		{language.JSX, `const x=<Panel title="a>b" enabled>{true ? <Item/> : null}</Panel>;`, "a>b", language.KindString},
		{language.JSX, `const x=<Panel title="a>b" enabled>{true ? <Item/> : null}</Panel>;`, "true", language.KindKeyword},
		{language.JSX, `const x=a < b; const y=<A/> / 2;`, "b;", language.KindPlain},
		{language.JSX, `const y=<A/> / 2;`, "/ 2", language.KindDelimiter},
		{language.JSX, `const x=<A>if true // text &amp;</A>;`, "true", language.KindPlain},
		{language.JSX, `const x=<A>if true // text &amp;</A>;`, "&amp;", language.KindEntity},
		{language.JSX, `const x=<UI.Button aria-label="go"><svg:path/></UI.Button>;`, "UI.Button", language.KindTag},
		{language.JSX, "const x=<A>{/[}]/.test(x) ? `hi ${<B/>}` : null}</A>;", "B", language.KindTag},
		{language.TSX, `const f=<T,>(x:T)=><A value={x}/>;`, "T,", language.KindPlain},
		{language.TSX, `const f=<T extends unknown>(x:T)=><A/>;`, "extends", language.KindKeyword},
		{language.TSX, `const f=<T=string,>(x:T)=>x;`, "string", language.KindKeyword},
		{language.TSX, `const x=<T extends={true}/>;`, "extends", language.KindAttribute},
		{language.TSX, `interface Props { readonly name: string; }`, "readonly", language.KindKeyword},
		{language.Kotlin, `@file:JvmName("Demo") fun main() { val name="Kotlin" }`, "@file:JvmName", language.KindMeta},
		{language.Kotlin, `val s="hi $name ${if (true) "ok" else "no"}"`, "if", language.KindKeyword},
		{language.Kotlin, `val s="""raw \n $name ${true}"""`, "name", language.KindVariable},
		{language.Kotlin, `val s=$$"""literal $name; $${true}"""`, "$name", language.KindString},
		{language.Kotlin, `val s=$$"""literal $name; $${true}"""`, "true", language.KindKeyword},
		{language.Kotlin, `/* outer /* inner */ still */ val x=1`, "still", language.KindComment},
		{language.Kotlin, "val `class`=1; loop@ for (x in 1..<3) break@loop", "class", language.KindVariable},
		{language.Kotlin, `val x=42.toString(); val n=0x2e+3`, "toString", language.KindPlain},
		{language.Kotlin, `val x=42.toString(); val n=0x2e+3`, "+", language.KindDelimiter},
		{language.Swift, `@MainActor func f(){ let s=#"hi \#(true ? "ok" : "no")"# }`, "true", language.KindKeyword},
		{language.Swift, `let s=##"literal \#(true) and \##(false)"##`, "true", language.KindString},
		{language.Swift, `let s=##"literal \#(true) and \##(false)"##`, "false", language.KindKeyword},
		{language.Swift, `let r = /[a-z\/]+/; let n = 10 / 2`, "a-z", language.KindString},
		{language.Swift, `let r = /[a-z\/]+/; let n = 10 / 2`, "/ 2", language.KindDelimiter},
		{language.Swift, "let r=#/a / b\n[c]/#; return true", "true", language.KindKeyword},
		{language.Swift, "/* outer /* inner */ still */ let `class` = $0", "still", language.KindComment},
		{language.Swift, "let `class` = $0", "$0", language.KindVariable},
		{language.Swift, "let `class` = $0", "class", language.KindVariable},
		{language.Dart, "@override String greet()=> 'Hi $name ${true ? 'ok' : 'no'}';", "override", language.KindMeta},
		{language.Dart, "var s='Hi $name ${true ? 'ok' : 'no'}';", "name", language.KindVariable},
		{language.Dart, "var s='Hi $name ${true ? 'ok' : 'no'}';", "true", language.KindKeyword},
		{language.Dart, "var s=r'${true} \\n';", "true", language.KindString},
		{language.Dart, "var s='\\u{1F600}';", "1F600", language.KindEscape},
		{language.Dart, "var s=#library.name; /* outer /* inner */ still */", "#library.name", language.KindMeta},
		{language.Dart, "/* outer /* inner */ still */ var n=1", "still", language.KindComment},
		{language.Dart, "var n=42.toString(); var x=0x2e+3;", "toString", language.KindPlain},
		{language.Dart, "var n=42.toString(); var x=0x2e+3;", "+", language.KindDelimiter},
		{language.R, "x <- r\"--[raw ]-\" still]--\"; if (TRUE) x", "still", language.KindString},
		{language.R, "x <- R'---{raw}---'; if (TRUE) x", "if", language.KindKeyword},
		{language.R, "`if` <- 1L; T <- FALSE", "if", language.KindVariable},
		{language.R, "T <- FALSE", "T", language.KindPlain},
		{language.R, "values %in% c(1, NA_real_, Inf)", "NA_real_", language.KindKeyword},
		{language.R, "x %custom[op]% y |> f()", "custom", language.KindDelimiter},
		{language.R, "x <- 0x1.fp+2 + 3i", "0x1.fp+2", language.KindNumber},
		{language.R, `x <- "\u{1234}" # comment`, "1234", language.KindEscape},
	} {
		t.Run(string(tc.id)+"/"+tc.needle, func(t *testing.T) {
			for _, chunk := range []int{1, 3, 31, ChunkSize} {
				tokens, _ := scanAll(tc.id, tc.text, chunk)
				at := strings.Index(tc.text, tc.needle)
				if at < 0 || kindAt(tokens, at) != tc.kind {
					t.Fatalf("chunk %d: %q want %s: %v", chunk, tc.text, tc.kind, tokens)
				}
			}
		})
	}
}

func TestProgrammingLanguageFragments(t *testing.T) {
	fixtures := map[language.ID][]string{
		language.R:      {"# comment\nf <- function(x) { `a b` <- r\"--[body ]-\" \\n\n]--\"; y=R'({raw})'; x %custom[op]% y |> sum(); NA_integer_ }", "x <- r\"" + strings.Repeat("-", ChunkSize*2) + "(" + strings.Repeat("字", ChunkSize) + ")" + strings.Repeat("-", ChunkSize*2) + "\"; if (TRUE) 1L", "x <- '\\x41 \\123'; y <- \"\\u{1234} \\U0001F600\"; x=0x1.fp+2 + 2i"},
		language.Dart:   {"@override void main(){ var s='Hi $name ${true ? \"nested ${1}\" : r\"raw\"}'; var t=r'''\n${false} \\n\n'''; /* outer /* inner */ still */ var sym=#library.name; var n=1..toString(); }", "var s='''" + strings.Repeat("字", ChunkSize) + "${{'name': 'value'}}''';", "var s='\\u{1F600} \\u1234 \\x4A'; var x=0x2e+3;"},
		language.C:      {"#include <stdio.h>\n#define SUM(a,b) \\\n ((a)+(b))\n// note \\\\\nstill\nconst char *x=u8\"abc\\n\"; wchar_t ch=L'字';", "/*" + strings.Repeat("字", ChunkSize) + "*/ int n;"},
		language.Cpp:    {"template<class T> concept Value = requires(T t) { t.x; };\nauto x=u8R\"0123456789abcdef(raw\n)wrong\" )0123456789abcdef\"; return 1'000ULL;", "auto x=R\"(" + strings.Repeat("\"\\字", ChunkSize) + ")\";"},
		language.Java:   {"@Override public String get(){ return \"\"\"\ntext \\\"\"\"\n\"\"\"; }", "cl\\u0061ss X { \\u002f\\u002a comment \\u002a\\u002f int n; }", "// note\\u000areturn;", "\\" + strings.Repeat("u", ChunkSize*2) + "0069nt n;", "String x=\"\\\\u0022\\u005c\\u006e\";"},
		language.Rust:   {"fn borrow<'a>(x: &'a str) -> &'a str { /* outer /*inner*/ still */ let r#type=br###\"raw \"## still\"###; println!(\"{}\",'字'); x }", "let x=r" + strings.Repeat("#", 255) + "\"" + strings.Repeat("字", ChunkSize) + "\"" + strings.Repeat("#", 255) + "; let y=1..2;"},
		language.CSharp: {"#nullable enable\nclass Demo { string s=$@\"Hi {true ? $\"{1}\" : @\"x\"}!\"; string r=$$\"\"\"literal {brace} {{{true}}}\"\"\"; }", "var s=\"\"\"\"\nraw \"\"\" text\n\"\"\"\"; var @class = @\"a\"\"b\";", "var s=$\"{1:0.00} {true ? 1 : 0}\";", "var s=" + strings.Repeat("$", 90) + strings.Repeat("\"", 100) + "raw " + strings.Repeat("{", 90) + "true" + strings.Repeat("}", 90) + strings.Repeat("\"", 100) + ";"},
		language.Lua:    {"local x=[==[raw ]=] text\n]==]\n--[=[ comment\n]=]\nlocal s='a\\z \n b'; return 0x1.fp+2", "local x=[" + strings.Repeat("=", ChunkSize*2) + "[body]" + strings.Repeat("=", ChunkSize*2) + "]; return x"},
	}
	fixtures[language.PowerShell] = []string{"function Greet { param($name) \"hi $name$($(\"nested\")) ${env:PATH}\"; if ($true -AND $false) { 1..3 } }\n$x=@\"\nbody $env:PATH $(if (1) { \"x\" })\n\"@\n$x=@'\nliteral $name\n '@\n'@", "$x=‘it’’s’\n<# a <# b #> function f {}\n$x=\"a`\"b\"", "$x=@'\n" + strings.Repeat("字", ChunkSize) + "\n'@"}
	fixtures[language.Ruby] = []string{"class Demo\n def hi(name)\n \"#{true ? %Q{nested #{1}} : name} #@value\"\n end\nend\nx=%r[[/]]im; y=10 / 2\nputs(<<A, <<-'B')\n#{true}\nA\n#{false}\n B\nclass Real;end", "x=<<A\n#{puts <<B}\ninner\nB\nouter\nA\nclass Demo;end", "x=<<'" + strings.Repeat("字", ChunkSize) + "'\nbody\n" + strings.Repeat("字", ChunkSize) + "\nclass Demo;end", "x=\"#@" + strings.Repeat("name", ChunkSize) + "\""}
	fixtures[language.PHP] = []string{"<p title=\"<?= $name ?>\">Hi</p><script>const<?php echo ' '; ?> value=\"<?= $name ?>\";</script><!-- <?= $name ?> -->\n<?php #[Route('/')] readonly class Demo { function f() { return \"{$row['key']} ${name}\"; } } // ?> <b>text</b>", "<?php $x=<<<TXT\n$name {$row['key']}\n TXT, true; $x=<<<'END'\n$name\nENDING\nEND; return;", "<?php $x=<<<\"" + strings.Repeat("字", ChunkSize) + "\"\nbody\n" + strings.Repeat("字", ChunkSize) + "; return;"}
	fixtures[language.JSX] = []string{"const x=<><UI.Button aria-label=\"a>b &gt;\" {...props}>{items.map(x=><Item key={x.id}/>)}</UI.Button>{/[}]/.test(x) ? `hi ${<B/>}` : null}</>; const y=a < b; const ratio=<A/> / 2;", "const x=<A title=\"\nbody\\\n\"/>;", "const x=<A>" + strings.Repeat("字", ChunkSize) + " &amp;</A>;"}
	fixtures[language.TSX] = []string{"interface Props {readonly name:string;}\nconst f=<T,>(x:T):T=>x; const g=<T extends unknown>(x:T)=><A value={x}/>; const x=<T extends={true}/>;", "const f=<T" + strings.Repeat(" ", ChunkSize*2) + ",>(x:T)=><A/>;"}
	fixtures[language.Kotlin] = []string{"@file:JvmName(\"Demo\")\n/* outer /* inner */ still */\nfun main(){ val s=\"hi $name ${if (true) \"${1}\" else name}\"; val raw=$$\"\"\"$literal $${true} $$$name\nraw\\n\"\"\"; val `class`=42.toString(); loop@ for(x in 1..<3)break@loop }", "val s=" + strings.Repeat("$", ChunkSize*2) + "\"" + strings.Repeat("$", ChunkSize*2) + "{true}\"", "val s=\"\"\"" + strings.Repeat("字", ChunkSize) + strings.Repeat("\"", 100) + "; val n=1"}
	fixtures[language.Swift] = []string{"@MainActor func f(){ let s=##\"raw \\##(true ? \"nested \\(1)\" : \"no\")\"##; let m=\"\"\"\nbody\n\"\"\"; let r=/[a-z\\/]+/; let n=10 / 2; /* outer /* inner */ still */ #if DEBUG\n#endif }", "let s=" + strings.Repeat("#", ChunkSize*2) + "\"body\\" + strings.Repeat("#", ChunkSize*2) + "(true)\"" + strings.Repeat("#", ChunkSize*2) + "; let x=1", "let r=##/a / b\n[()]/##; let x=0x1.fp+2"}
	for id, texts := range fixtures {
		for _, text := range texts {
			want, ws := scanAll(id, text, ChunkSize*8)
			for _, chunk := range []int{1, 2, 3, 7, 31, ChunkSize} {
				got, gs := scanAll(id, text, chunk)
				if !reflect.DeepEqual(got, want) || !gs.Equal(ws) {
					t.Fatalf("%s chunk %d: tokens equal=%v states equal=%v\ngot %v\nwant %v\ngot state %#v\nwant state %#v", id, chunk, reflect.DeepEqual(got, want), gs.Equal(ws), got, want, gs.raw, ws.raw)
				}
			}
		}
	}
}

func TestToolingLanguageMalformedFragments(t *testing.T) {
	random := rand.New(rand.NewSource(20261102))
	alphabet := []rune("Rremurl0129_'\"`\\$#@%!.*+-[]{}():,;=/~ \t\n你好")
	for _, id := range []language.ID{language.Diff, language.Batch, language.HCL, language.GraphQL, language.SCSS, language.Less} {
		for round := 0; round < 300; round++ {
			var b strings.Builder
			for n := random.Intn(500); n > 0; n-- {
				b.WriteRune(alphabet[random.Intn(len(alphabet))])
			}
			text := b.String()
			want, ws := scanAll(id, text, 1024)
			got, gs := scanAll(id, text, 1+random.Intn(17))
			if !reflect.DeepEqual(got, want) || !gs.Equal(ws) {
				t.Fatalf("%s malformed %q: tokens=%v states=%v\ngot state %#v\nwant state %#v", id, text, reflect.DeepEqual(got, want), gs.Equal(ws), gs.raw, ws.raw)
			}
		}
	}
}

func TestProgrammingLanguageMalformedFragments(t *testing.T) {
	random := rand.New(rand.NewSource(20261102))
	alphabet := []rune("Rurcb0129_'\"\\$#@!*+-[]{}():,;=/ \t\n你好")
	for _, id := range []language.ID{language.C, language.Cpp, language.Java, language.Rust, language.CSharp, language.Lua, language.PowerShell, language.Ruby, language.PHP, language.JSX, language.TSX, language.Kotlin, language.Swift, language.Dart, language.R, language.Zig, language.ObjectiveC, language.ObjectiveCpp, language.Scala, language.Perl} {
		for round := 0; round < 300; round++ {
			var b strings.Builder
			for n := random.Intn(500); n > 0; n-- {
				b.WriteRune(alphabet[random.Intn(len(alphabet))])
			}
			text := b.String()
			if id == language.PHP {
				text = "<?php " + text
			}
			want, ws := scanAll(id, text, 1024)
			got, gs := scanAll(id, text, 1+random.Intn(17))
			if !reflect.DeepEqual(got, want) || !gs.Equal(ws) {
				t.Fatalf("%s malformed %q: got %v want %v\ngot state %#v\nwant state %#v", id, text, got, want, gs.raw, ws.raw)
			}
		}
	}
}

func TestRubyQueuedHeredocs(t *testing.T) {
	var text strings.Builder
	text.WriteString("puts(")
	for i := 0; i < 63; i++ {
		if i > 0 {
			text.WriteString(",")
		}
		fmt.Fprintf(&text, "<<E%d", i)
	}
	text.WriteString(")\n")
	for i := 0; i < 63; i++ {
		fmt.Fprintf(&text, "body%d\nE%d\n", i, i)
	}
	text.WriteString("class Real; end")
	for _, chunk := range []int{1, 7, ChunkSize} {
		tokens, _ := scanAll(language.Ruby, text.String(), chunk)
		if kindAt(tokens, strings.LastIndex(text.String(), "class")) != language.KindKeyword {
			t.Fatal("heredoc queue did not restore code")
		}
		for i := 0; i < 63; i++ {
			if kindAt(tokens, strings.Index(text.String(), fmt.Sprintf("body%d\n", i))) != language.KindString {
				t.Fatalf("heredoc %d lost", i)
			}
		}
	}
}
