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
		{"CMakeLists.txt", "", CMake}, {"Makefile", "", Makefile}, {"GNUmakefile", "", Makefile},
		{"Makefile.debug", "", Makefile}, {"Dockerfile.dev", "", Dockerfile},
		{"nginx.conf", "", Nginx}, {"site.nginx.conf", "", Nginx}, {"service.conf", "", Plain},
		{"a.JSONC", "", JSONC}, {"request.rest", "", HTTP}, {"project.mk", "", Makefile},
		{"events.JSONL", "", JSONLines}, {"events.ndjson", "true\nnull", JSONLines},
		{"events.jsonl", "#!/bin/sh", JSONLines}, {"events.json", "true\nnull", JSON},
		{"project.cmake", "", CMake}, {"app.ini", "", INI}, {"app.dockerfile", "", Dockerfile},
		{"main.c", "", C}, {"main.C", "", Cpp}, {"lib.H", "", Cpp}, {"module.cppm", "", Cpp},
		{"main.cxx", "", Cpp}, {"header.hpp", "", Cpp}, {"Main.java", "", Java},
		{"main.rs", "", Rust}, {"Program.cs", "", CSharp}, {"script.csx", "", CSharp}, {"init.lua", "", Lua},
		{"script", "#!/usr/bin/env lua5.4", Lua}, {"script", "#!/usr/bin/luajit", Lua},
		{"script", "#!/usr/bin/luatic", Plain}, {"script.c", "#!/bin/lua", C},
		{"run.PS1", "", PowerShell}, {"module.psm1", "", PowerShell}, {"data.psd1", "", PowerShell},
		{"script", "#!/usr/bin/env pwsh", PowerShell},
		{"index.PHP", "", PHP}, {"page.phtml", "", PHP}, {"", "<?php echo 1;", PHP}, {"", "<?= 1 ?>", PHP}, {"", "<?phpfoo", Plain},
		{"app.jsx", "", JSX}, {"app.TSX", "", TSX}, {"app.js", "<A/>", JavaScript}, {"app.ts", "<T,>(x:T)=>x", TypeScript},
		{"hello.kt", "", Kotlin}, {"build.gradle.kts", "", Kotlin}, {"script", "#!/usr/bin/env kotlin", Kotlin},
		{"hello.swift", "", Swift}, {"API.swiftinterface", "", Swift}, {"script", "#!/usr/bin/env swift", Swift},
		{"binary.swiftmodule", "", Plain},
		{"hello.dart", "", Dart}, {"script", "#!/usr/bin/env dart", Dart},
		{"hello.zig", "", Zig},
		{"hello.mm", "", ObjectiveCpp},
		{"hello.scala", "", Scala}, {"build.sbt", "", Scala}, {"script", "#!/usr/bin/env scala-cli", Scala},
		{"hello.pl", "", Perl}, {"Module.pm", "", Perl}, {"script", "#!/usr/bin/env perl", Perl}, {"script", "#!/usr/bin/perl5.40.0", Perl},
		{"change.diff", "", Diff}, {"change.patch", "", Diff},
		{"build.bat", "", Batch}, {"build.CMD", "", Batch},
		{"main.tf", "", HCL}, {"vars.tfvars", "", HCL}, {".terraform.lock.hcl", "", HCL}, {"vars.tfvars.json", "", JSON},
		{"query.graphql", "", GraphQL}, {"schema.graphqls", "", GraphQL}, {"query.gql", "", GraphQL},
		{"theme.scss", "", SCSS}, {"theme.sass", "", Plain},
		{"theme.less", "", Less},
		{"script", "#!/usr/bin/perl5.", Plain}, {"script", "#!/usr/bin/perl5.debug", Plain},
		{"hello.m", "#import <Foundation/Foundation.h>", ObjectiveC}, {"hello.m", "@interface Demo : NSObject", ObjectiveC},
		{"plot.m", "function y=f(x)\ny=x+1;\nend", Plain}, {"empty.m", "", Plain}, {"shared.h", "@interface Demo", Cpp},
		{"hello.R", "", R}, {"hello.r", "", R}, {"hello.Rscript", "", R}, {".Rprofile", "", R},
		{"script", "#!/usr/bin/env -S Rscript --vanilla", R},
		{"report.Rmd", "", Plain}, {"report.qmd", "", Plain}, {"data.RData", "", Plain}, {"build.gradle", "", Plain},
		{"hello.rb", "", Ruby}, {"Gemfile", "", Ruby}, {"Rakefile", "", Ruby}, {"app.gemspec", "", Ruby}, {"config.ru", "", Ruby}, {"script", "#!/usr/bin/env ruby", Ruby},
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

func TestJSONLinesRecordState(t *testing.T) {
	l, ok := Builtin(JSONLines)
	if !ok {
		t.Fatal("missing JSON Lines definition")
	}
	if l.Folding() != nil || l.Editing().Comments != (Comments{}) {
		t.Fatal("JSON Lines enabled multiline folding or comments")
	}
	scanner := l.Tokenizer()
	initial := scanner.InitialState()
	for _, text := range []string{"", "{", strings.Repeat("[", 1100), `"unfinished`, "-", "// invalid {", `{"key":true}`, "null"} {
		in := Input{Text: text, Limit: len(text), LineEnd: true}
		r, err := scanner.Scan(in, initial)
		if err != nil {
			t.Fatal(err)
		}
		if err = ValidateResult(in, r); err != nil {
			t.Fatal(err)
		}
		if !r.State.Equal(initial) {
			t.Fatalf("record retained state: %q", text)
		}
		if !initial.Equal(scanner.InitialState()) {
			t.Fatal("initial snapshot changed")
		}
	}
	// 预读到了真实行尾，不代表本片已消费到行尾；软折行也不结束记录。
	text := `{"key":1}`
	in := Input{Text: text, Limit: 1, LineEnd: true}
	r, err := scanner.Scan(in, initial)
	if err != nil || r.Consumed != 1 || r.State.Equal(initial) {
		t.Fatalf("partial record: %+v, %v", r, err)
	}
	partial := r.State
	in = Input{Text: text[1:], Offset: 1, Limit: len(text) - 1, LineEnd: true}
	r, err = scanner.Scan(in, partial)
	if err != nil {
		t.Fatal(err)
	}
	if err = ValidateResult(in, r); err != nil {
		t.Fatal(err)
	}
	if len(r.Tokens) == 0 || r.Tokens[0].Kind != KindProperty || !r.State.Equal(initial) || partial.Equal(initial) {
		t.Fatalf("fragment state: %+v", r)
	}
}

func TestAddedLanguageContextsAndSnapshots(t *testing.T) {
	for _, tc := range []struct {
		id                    ID
		text                  string
		str, comment, literal bool
	}{
		{JSONC, "/* open", false, true, false}, {HTTP, "# note", false, true, false},
		{Makefile, "# note", false, true, false}, {CMake, "[=[open", true, false, true},
		{Nginx, `set $x "open`, true, false, false}, {INI, `key="open`, true, false, false},
		{Dockerfile, "# note", false, true, false}, {SQL, "$tag$open", true, false, true},
		{Protobuf, "/* open", false, true, false}, {Go, "`raw", true, false, true},
		{Python, `f"{True`, false, false, false}, {JavaScript, "`text ${true", false, false, false},
		{TypeScript, "/* open", false, true, false}, {CSS, `.x { content: "open`, true, false, false},
		{C, "/* open", false, true, false}, {Cpp, `auto s=R"tag(raw`, true, false, true},
		{Java, `\u002f\u002f note`, false, true, false}, {Rust, `r##"raw`, true, false, true},
		{Java, `""`, false, false, false}, {CSharp, `""`, false, false, false},
		{Rust, `'static`, false, false, false}, {CSharp, `$"text {true`, false, false, false},
		{CSharp, `@"raw`, true, false, true}, {CSharp, `$$"""raw`, true, false, true},
		{CSharp, `$$"""{{`, false, false, false}, {CSharp, `"""`, true, false, true},
		{CSharp, `$"outer {$"inner {`, false, false, false},
		{Lua, `[==[raw`, true, false, true}, {Lua, `--[=[comment`, false, true, false},
		{PowerShell, `"hello $name`, true, false, false}, {PowerShell, `'literal`, true, false, true},
		{PowerShell, `"hello $(if`, false, false, false}, {PowerShell, `<# comment`, false, true, false},
		{Ruby, `%q{literal`, true, false, false}, {Ruby, `"hi #{if`, false, false, false}, {Ruby, `%Q{outer #{%Q{inner #{`, false, false, false},
		{PHP, `<?php "hello $name`, true, false, false}, {PHP, `<?php "{$row['key']`, false, false, false}, {PHP, `<?php /* note`, false, true, false},
		{JSX, `const x=<A>text`, true, false, true}, {JSX, `const x=<A title="raw`, true, false, true},
		{JSX, `const x=<A>{true`, false, false, false}, {TSX, `const x=<A>{/* note`, false, true, false},
		{Kotlin, `val s="""raw`, true, false, true}, {Kotlin, `val s=$$"hi $${if`, false, false, false},
		{Swift, `let s=##"raw`, true, false, true}, {Swift, `let s="hi \(true`, false, false, false},
		{Dart, `var s=r'''raw`, true, false, true}, {Dart, `var s='hi ${true`, false, false, false},
		{R, `x <- r"--(raw`, true, false, true}, {R, "`a b", true, false, false},
		{Zig, `const s=\\raw`, true, false, true}, {Zig, `const @"a b`, true, false, false},
		{ObjectiveC, `id s=@"text`, true, false, false},
		{ObjectiveCpp, `auto s=R"tag(raw`, true, false, true},
		{Scala, `val s=s"hi ${true`, false, false, false}, {Scala, `val s="""raw`, true, false, true},
		{Perl, `my $s=q{literal`, true, false, false}, {Perl, "=pod", false, true, false},
		{Batch, `echo "text %name`, true, false, true}, {Batch, "@REM note", false, true, false},
		{HCL, `value="hello ${true`, false, false, false}, {HCL, `value="hello`, true, false, false},
		{GraphQL, `"""body`, true, false, true}, {GraphQL, `"body`, true, false, false},
		{SCSS, `.x {content:"#{true`, false, false, false}, {SCSS, `/* comment`, false, true, false},
		{Less, `.x {content:"@{name`, true, false, false}, {Less, `.x{width:@name`, false, false, false},
	} {
		t.Run(string(tc.id), func(t *testing.T) {
			l, _ := Builtin(tc.id)
			scanner := l.Tokenizer()
			initial := scanner.InitialState()
			input := Input{Text: tc.text, Limit: len(tc.text)}
			r, err := scanner.Scan(input, initial)
			if err != nil {
				t.Fatal(err)
			}
			if err = ValidateResult(input, r); err != nil {
				t.Fatal(err)
			}
			c := r.State.Context()
			if c.String != tc.str || c.Comment != tc.comment || c.Literal != tc.literal {
				t.Fatalf("context %+v", c)
			}
			if !initial.Equal(scanner.InitialState()) {
				t.Fatal("initial snapshot changed")
			}
			_, err = scanner.Scan(Input{Text: `suffix } */ "`, Offset: len(tc.text), Limit: 13, LineEnd: true}, r.State)
			if err != nil {
				t.Fatal(err)
			}
			fresh, err := scanner.Scan(input, scanner.InitialState())
			if err != nil {
				t.Fatal(err)
			}
			if !r.State.Equal(fresh.State) {
				t.Fatal("published snapshot changed")
			}
		})
	}
}

func TestBuiltinMarkdownAddedFences(t *testing.T) {
	custom, err := NewMarkdown(nil)
	if err != nil {
		t.Fatal(err)
	}
	builtin, _ := Builtin(Markdown)
	for _, l := range []*Language{builtin, custom} {
		for name, id := range map[string]ID{"jsonc": JSONC, "jsonlines": JSONLines, "jsonl": JSONLines, "ndjson": JSONLines, "http": HTTP, "makefile": Makefile, "cmake": CMake, "nginx": Nginx, "ini": INI, "dockerfile": Dockerfile, "sql": SQL, "proto": Protobuf, "go": Go, "py": Python, "js": JavaScript, "ts": TypeScript, "css": CSS, "c": C, "cpp": Cpp, "c++": Cpp, "cc": Cpp, "java": Java, "rust": Rust, "rs": Rust, "csharp": CSharp, "cs": CSharp, "c#": CSharp, "lua": Lua, "powershell": PowerShell, "ps1": PowerShell, "pwsh": PowerShell, "ps": PowerShell, "ruby": Ruby, "rb": Ruby} {
			scanner := l.Tokenizer()
			fence := "```" + name
			r, err := scanner.Scan(Input{Text: fence, Limit: len(fence), LineEnd: true}, scanner.InitialState())
			if err != nil {
				t.Fatal(err)
			}
			r, err = scanner.Scan(Input{Text: "x", Limit: 1}, r.State)
			if err != nil {
				t.Fatal(err)
			}
			if child := r.State.Context().Embedded; child == nil || child.ID() != id {
				t.Fatalf("%s: embedded %v", name, child)
			}
		}
	}
}

func TestProgrammingLanguageClasses(t *testing.T) {
	for _, tc := range []struct {
		id           ID
		text, needle string
		class        Class
	}{
		{C, `#include <stdio.h>`, "stdio", ClassString},
		{Cpp, `R"tag({ /* literal */ })tag"`, "{", ClassString},
		{Java, `\u002f\u002f comment`, "comment", ClassComment},
		{Java, `"\u0022; {}`, "{", ClassDelimiter},
		{Java, `"\\u0022 still"`, "still", ClassString},
		{Rust, `/* outer /* inner */ still */`, "still", ClassComment},
		{Rust, `r###"{ literal }"###`, "{", ClassString},
		{Rust, `fn f<'a>(x: &'a str) {}`, "'a", 0},
		{CSharp, `$"text {true}"`, "true", 0},
		{CSharp, `$"text {true}"`, "{", ClassDelimiter},
		{CSharp, `$$"""literal {brace} {{true}}"""`, "{brace}", ClassString},
		{CSharp, `$$"""literal {brace} {{true}}"""`, "true", 0},
		{CSharp, `$"{1:0.00}"`, "0.00", ClassString},
		{Lua, `[=[{ literal }]=]`, "{", ClassString},
		{Lua, `--[=[{ comment }]=]`, "{", ClassComment},
		{PowerShell, `"hello $(if ($true) { 1 })"`, "if", 0},
		{PowerShell, `"hello $name"`, "$name", ClassString},
		{PowerShell, `<# { comment } #>`, "{", ClassComment},
		{Ruby, `"hi #{true}"`, "true", 0}, {Ruby, `%q{#{literal}}`, "literal", ClassString}, {Ruby, `%Q{#@value}`, "@value", ClassString},
		{PHP, `<p title="<?= $name ?>">text</p>`, "$name", 0},
		{PHP, `<?php "hi $name"`, "$name", ClassString},
		{PHP, `<?php "{$row['key']}"`, "$row", 0},
		{JSX, `const x=<A>if true</A>;`, "true", ClassString},
		{JSX, `const x=<A title="{raw}">{true}</A>;`, "{raw}", ClassString},
		{JSX, `const x=<A>{true}</A>;`, "true", 0},
		{TSX, `const x=<A>{true}</A>;`, "{true}", ClassDelimiter},
		{Kotlin, `val s="hi $name ${true}"`, "$name", ClassString}, {Kotlin, `val s="hi $name ${true}"`, "true", 0},
		{Swift, `let s=#"hi \#(true)"#`, "true", 0}, {Swift, `let n=10 / 2`, "/", 0},
		{Swift, `let r=#/[(]/#`, "[(]", ClassString},
		{Dart, `var s=r'${true}';`, "{true}", ClassString}, {Dart, `var s='${true}';`, "true", 0},
		{R, `x <- r"--[{raw}]--";`, "{raw}", ClassString}, {R, "x %[% y", "[", 0},
		{Zig, `const s=\\{raw}`, "{raw}", ClassString},
		{ObjectiveC, `id s=@"{raw}";`, "{raw}", ClassString},
		{ObjectiveCpp, `auto s=R"({raw})";`, "{raw}", ClassString},
		{Scala, `val s=s"${true}"`, "true", 0},
		{Perl, `my $r=qr/[()]/;`, "[()]", ClassString},
		{Batch, `echo "%name% (literal)"`, "(literal)", ClassString},
		{HCL, `value="${true}"`, "true", 0}, {HCL, `value="$${true}"`, "true", ClassString},
		{GraphQL, `query Q { field(x:"[()]") }`, "[()]", ClassString},
		{SCSS, `.x {content:"[()]"}`, "[()]", ClassString}, {SCSS, `.x {content:"#{true}"}`, "true", 0},
		{Less, `.x {content:"@{name}"}`, "name", ClassString}, {Less, `.@{name}{}`, "name", 0},
	} {
		l, _ := Builtin(tc.id)
		for _, chunk := range []int{1, 2, 3, 7, 31, 16384} {
			scanner := l.Tokenizer()
			state := scanner.InitialState()
			var tokens []Token
			for at := 0; at < len(tc.text); {
				limit := min(chunk, len(tc.text)-at)
				for at+limit < len(tc.text) && !utf8.RuneStart(tc.text[at+limit]) {
					limit++
				}
				end := min(len(tc.text), at+limit+32)
				for end < len(tc.text) && !utf8.RuneStart(tc.text[end]) {
					end++
				}
				in := Input{Text: tc.text[at:end], Offset: at, Limit: limit, LineEnd: end == len(tc.text), DocumentEnd: end == len(tc.text)}
				r, err := scanner.Scan(in, state)
				if err != nil {
					t.Fatal(err)
				}
				if err = ValidateResult(in, r); err != nil {
					t.Fatalf("%s chunk %d: %v", tc.id, chunk, err)
				}
				if r.ReplaceFrom != nil {
					cut := *r.ReplaceFrom
					for len(tokens) > 0 && tokens[len(tokens)-1].End > cut {
						last := &tokens[len(tokens)-1]
						if last.Start < cut {
							last.End = cut
							break
						}
						tokens = tokens[:len(tokens)-1]
					}
				}
				tokens = append(tokens, r.Tokens...)
				at += r.Consumed
				state = r.State
			}
			at := strings.Index(tc.text, tc.needle)
			got := Class(0)
			for _, tok := range tokens {
				if tok.Start <= at && at < tok.End {
					got = tok.Class
					break
				}
			}
			if got != tc.class {
				t.Fatalf("%s chunk %d %q: class %v want %v; %v", tc.id, chunk, tc.text, got, tc.class, tokens)
			}
		}
	}
}

func TestPHPProfilesAndSuspension(t *testing.T) {
	l, _ := Builtin(PHP)
	for _, tc := range []struct {
		text     string
		embedded ID
		str      bool
	}{
		{`<p title="`, HTML, true},
		{`<p title="<?= $name`, PHP, false},
		{`<script>const name="<?= $name ?> tail`, JavaScript, true},
	} {
		scanner := l.Tokenizer()
		in := Input{Text: tc.text, Limit: len(tc.text)}
		r, err := scanner.Scan(in, scanner.InitialState())
		if err != nil {
			t.Fatal(err)
		}
		if err = ValidateResult(in, r); err != nil {
			t.Fatal(err)
		}
		c := r.State.Context()
		id := PHP
		if c.Embedded != nil {
			id = c.Embedded.ID()
		}
		if id != tc.embedded || c.String != tc.str {
			t.Fatalf("%q: %+v", tc.text, c)
		}
		if id == HTML && c.Embedded.Editing().Tags != nil {
			t.Fatal("mixed HTML enabled markup-only tag matching")
		}
	}
	markdown, _ := Builtin(Markdown)
	custom, err := NewMarkdown(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, md := range []*Language{markdown, custom} {
		scanner := md.Tokenizer()
		r, err := scanner.Scan(Input{Text: "```php", Limit: 6, LineEnd: true}, scanner.InitialState())
		if err != nil {
			t.Fatal(err)
		}
		r, err = scanner.Scan(Input{Text: "return true;", Limit: 12, LineEnd: true}, r.State)
		if err != nil {
			t.Fatal(err)
		}
		if len(r.Tokens) == 0 || r.Tokens[0].Kind != KindKeyword || r.State.Context().Embedded.ID() != PHP {
			t.Fatalf("bare PHP fence: %+v", r)
		}
	}
}

func TestJSXFencesAndEditing(t *testing.T) {
	md, _ := Builtin(Markdown)
	custom, err := NewMarkdown(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range []*Language{md, custom} {
		for name, id := range map[string]ID{"jsx": JSX, "tsx": TSX, "javascriptreact": JSX, "typescriptreact": TSX} {
			scanner := l.Tokenizer()
			fence := "```" + name
			r, err := scanner.Scan(Input{Text: fence, Limit: len(fence), LineEnd: true}, scanner.InitialState())
			if err != nil {
				t.Fatal(err)
			}
			text := "const x=<A/>;"
			r, err = scanner.Scan(Input{Text: text, Limit: len(text), LineEnd: true}, r.State)
			if err != nil {
				t.Fatal(err)
			}
			if r.State.Context().Embedded.ID() != id {
				t.Fatalf("%s: wrong fence profile", name)
			}
			found := false
			for _, tok := range r.Tokens {
				if tok.Start == 9 && tok.End == 10 && tok.Kind == KindTag {
					found = true
				}
			}
			if !found {
				t.Fatalf("%s: JSX tag not highlighted: %v", name, r.Tokens)
			}
		}
	}
	for _, id := range []ID{JSX, TSX} {
		l, _ := Builtin(id)
		if l.Editing().Tags != nil {
			t.Fatal("JSX enabled markup-only tag matcher")
		}
		for _, p := range l.Editing().Brackets {
			if p.Open == "<" {
				t.Fatal("JSX guesses generic angle pairs")
			}
		}
	}
}

func TestToolingLanguageFences(t *testing.T) {
	md, _ := Builtin(Markdown)
	for _, tc := range []struct {
		name string
		id   ID
		text string
		kind Kind
	}{
		{"diff", Diff, "+new", KindAdded}, {"patch", Diff, "@@", KindMeta},
		{"bat", Batch, "echo", KindKeyword}, {"batch", Batch, "set", KindKeyword}, {"cmd", Batch, "call", KindKeyword},
		{"hcl", HCL, "resource", KindKeyword}, {"terraform", HCL, "variable", KindKeyword}, {"tf", HCL, "module", KindKeyword},
		{"graphql", GraphQL, "query", KindKeyword}, {"gql", GraphQL, "schema", KindKeyword},
		{"scss", SCSS, "@mixin", KindMeta},
		{"less", Less, "@import", KindMeta},
	} {
		scanner := md.Tokenizer()
		fence := "```" + tc.name
		r, err := scanner.Scan(Input{Text: fence, Limit: len(fence), LineEnd: true}, scanner.InitialState())
		if err != nil {
			t.Fatal(err)
		}
		r, err = scanner.Scan(Input{Text: tc.text, Limit: len(tc.text), LineEnd: true}, r.State)
		if err != nil {
			t.Fatal(err)
		}
		if r.State.Context().Embedded.ID() != tc.id || len(r.Tokens) != 1 || r.Tokens[0].Kind != tc.kind || r.Tokens[0].End != len(tc.text) {
			t.Fatalf("%s: %+v", tc.name, r)
		}
	}
}

func TestProgrammingLanguageFences(t *testing.T) {
	builtin, _ := Builtin(Markdown)
	custom, err := NewMarkdown(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, md := range []*Language{builtin, custom} {
		for _, tc := range []struct {
			name    string
			id      ID
			keyword string
		}{
			{"kotlin", Kotlin, "fun"}, {"kt", Kotlin, "val"}, {"kts", Kotlin, "class"},
			{"swift", Swift, "func"},
			{"dart", Dart, "void"},
			{"r", R, "function"}, {"R", R, "TRUE"},
			{"zig", Zig, "comptime"},
			{"objc", ObjectiveC, "@interface"}, {"objective-c", ObjectiveC, "@end"},
			{"objcpp", ObjectiveCpp, "namespace"}, {"objc++", ObjectiveCpp, "@end"}, {"objective-cpp", ObjectiveCpp, "template"},
			{"scala", Scala, "given"},
			{"perl", Perl, "package"}, {"pl", Perl, "sub"},
		} {
			scanner := md.Tokenizer()
			fence := "```" + tc.name
			r, err := scanner.Scan(Input{Text: fence, Limit: len(fence), LineEnd: true}, scanner.InitialState())
			if err != nil {
				t.Fatal(err)
			}
			r, err = scanner.Scan(Input{Text: tc.keyword, Limit: len(tc.keyword), LineEnd: true}, r.State)
			if err != nil {
				t.Fatal(err)
			}
			if r.State.Context().Embedded.ID() != tc.id || len(r.Tokens) != 1 || r.Tokens[0].Kind != KindKeyword || r.Tokens[0].Start != 0 || r.Tokens[0].End != len(tc.keyword) {
				t.Fatalf("%s: %+v", tc.name, r)
			}
		}
	}
}

func TestProgrammingLanguageEditingRules(t *testing.T) {
	for _, id := range []ID{C, Cpp, Java, Rust, CSharp, Lua} {
		l, _ := Builtin(id)
		rules := l.Editing()
		line, block := "//", "/*"
		if id == Lua {
			line, block = "--", "--[["
		}
		if rules.Comments.Line != line || rules.Comments.Block.Open != block {
			t.Fatalf("%s: comments %+v", id, rules.Comments)
		}
		for _, pair := range rules.Brackets {
			if pair.Open == "<" {
				t.Fatalf("%s guesses generic angle brackets", id)
			}
		}
		if len(l.Suggestions(Context{})) == 0 {
			t.Fatalf("%s missing vocabulary", id)
		}
	}
	l, _ := Builtin(Rust)
	allow := l.Editing().AllowPair
	if allow(PairContext{Typed: "'"}) || !allow(PairContext{Typed: "'", Selected: true}) {
		t.Fatal("Rust lifetime quote policy")
	}
}

func TestHTTPNestedBodyContext(t *testing.T) {
	l, _ := Builtin(HTTP)
	scanner := l.Tokenizer()
	state := scanner.InitialState()
	lines := []string{"POST / HTTP/1.1", "Content-Type: text/html", "", "<script>const text = `open"}
	for i, line := range lines {
		r, err := scanner.Scan(Input{Text: line, Limit: len(line), LineEnd: i < len(lines)-1}, state)
		if err != nil {
			t.Fatal(err)
		}
		state = r.State
	}
	c := state.Context()
	if c.Embedded == nil || c.Embedded.ID() != JavaScript || !c.String {
		t.Fatalf("nested body context: %+v", c)
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
		{"script", "#!/usr/bin/env -u bash python", Python},
		{"script", "#!/usr/bin/env --chdir bash python", Python},
		{"script", "#!/usr/bin/env -u NAME bash", Shell},
		{"script", "#!/usr/bin/env -- bash", Shell},
		{"script", "#!/usr/bin/env --unknown sh", Plain},
		{"x.json", "#!/bin/sh", JSON}, {"unknown", "#!/usr/bin/env python\n", Python},
		{"script", "#!/usr/bin/env -S python3 -u", Python},
		{"script", "#!/usr/bin/pythonic", Plain},
		{"main.go", "", Go}, {"main.PYI", "", Python}, {"main.mjs", "", JavaScript},
		{"main.cts", "", TypeScript}, {"style.css", "", CSS}, {"service.proto", "", Protobuf}, {"query.SQL", "", SQL},
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
