package language

import (
	"context"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestBuiltinDataFolding(t *testing.T) {
	for _, id := range []ID{Zig, R, GraphQL, HCL, Protobuf, Starlark} {
		t.Run(string(id), func(t *testing.T) {
			l, _ := Builtin(id)
			if l.Folding() == nil {
				t.Fatal("builtin has no data folding provider")
			}
		})
	}
}

func TestDataFoldingRanges(t *testing.T) {
	for _, tt := range []struct {
		name string
		id   ID
		text string
		want []FoldRange
	}{
		{"zig nesting", Zig, "pub fn main() void {\n const a = [_]u8{\n  1, 2,\n };\n}", []FoldRange{{0, 4}, {1, 3}}},
		{"zig line string", Zig, "const s =\n \\\\{ // text\n \\\\} [\n;", []FoldRange{{1, 2}}},
		{"zig line string EOF", Zig, "\\\\first\n\\\\last", []FoldRange{{0, 1}}},
		{"zig line string break", Zig, "\\\\first\n\n\\\\second", nil},
		{"zig separate expressions", Zig, "x = \\\\first\ny = \\\\second", nil},
		{"zig quoted names", Zig, "fn @\"{name}\"() void {\n const c = '}';\n // }\n}", []FoldRange{{0, 3}}},
		{"r code", R, "f <- function(x) {\n if (x) {\n  print(\"}\")\n }\n}", []FoldRange{{0, 4}, {1, 3}}},
		{"r raw delimiters", R, "x <- r\"--[\n )-\" } #\n]--\"", []FoldRange{{0, 2}}},
		{"r ordinary literal", R, "x <- 'first\n } # last'", []FoldRange{{0, 1}}},
		{"r adjacent literals", R, "x <- '\n'; y <- '\n'", []FoldRange{{1, 2}}},
		{"r backtick name", R, "`name\nwith }` <- 1", nil},
		{"r custom operator", R, "x %fake{\n}fake% y", nil},
		{"graphql nested", GraphQL, "query Q(\n $id: ID!\n) {\n user(id: $id) {\n  id\n }\n}", []FoldRange{{0, 1}, {2, 6}, {3, 5}}},
		{"graphql block string", GraphQL, "\"\"\"description\n \\\"\"\" } #\nend\"\"\"", []FoldRange{{0, 2}}},
		{"graphql adjacent block strings", GraphQL, "\"\"\"a\n\"\"\" \"\"\"b\n\"\"\"", []FoldRange{{1, 2}}},
		{"graphql invalid ordinary string", GraphQL, "\"broken\nquery Q {\n field\n}", []FoldRange{{1, 3}}},
		{"hcl nesting", HCL, "resource \"x\" \"y\" {\n values = [\n  { a = \"}\" },\n ]\n}", []FoldRange{{0, 4}, {1, 3}}},
		{"hcl interpolation", HCL, "x = \"outer ${\n join(\"\", [\n  \"a\", \"b\"\n ])\n} end\"", []FoldRange{{0, 4}, {1, 3}}},
		{"hcl heredoc", HCL, "x = <<-EOF\n } # \\\n  EOF", []FoldRange{{0, 2}}},
		{"hcl comments", HCL, "/* first\n*/ value = 1 /* second\n*/", []FoldRange{{1, 2}}},
		{"hcl template blocks", HCL, "x = <<EOF\n%{ if enabled }\n%{ for item in items ~}\n${item}\n%{ endfor }\n%{ else }\nnone\n%{ endif }\nEOF", []FoldRange{{0, 8}, {1, 7}, {2, 4}}},
		{"hcl directive owner", HCL, "x = \"%{ if enabled }${\n \"%{ endif }\"\n} outer\"", []FoldRange{{0, 2}}},
		{"hcl escaped directives", HCL, "x = <<EOF\n%%{ if x }\n$${fake { }\n%%{ endif }\nEOF", []FoldRange{{0, 4}}},
		{"hcl empty directive", HCL, "x = <<EOF\n%{ if }\nbody\n%{ endif }\nEOF", []FoldRange{{0, 4}}},
		{"hcl wrong directive", HCL, "x = <<EOF\n%{ if enabled }\nbody\n%{ endfor }\nEOF", []FoldRange{{0, 4}}},
		{"hcl commented directives", HCL, "x = <<EOF\n%{ if /* comment */ enabled }\nbody\n%{ endif /* comment */ }\nEOF", []FoldRange{{0, 4}, {1, 3}}},
		{"hcl comment is not condition", HCL, "x = <<EOF\n%{ if /* comment */ }\nbody\n%{ endif }\nEOF", []FoldRange{{0, 4}}},
		{"hcl unknown directive", HCL, "x = <<EOF\n%{ 1 if enabled }\nbody\n%{ endif }\nEOF", []FoldRange{{0, 4}}},
		{"hcl directive closing content", HCL, "x = <<EOF\n%{ if enabled }\nbody\n%{ endif } rest\nEOF", []FoldRange{{0, 4}, {1, 2}}},
		{"hcl multiline directive", HCL, "x = <<EOF\n%{ if (\n enabled\n) }\nbody\n%{ endif }\nEOF", []FoldRange{{0, 6}, {1, 5}}},
		{"hcl adjacent templates", HCL, "a = \"\n\"; b = \"\n\"", []FoldRange{{1, 2}}},
		{"hcl unclosed heredoc", HCL, "x = <<EOF\n } #\n EO", nil},
		{"proto declarations", Protobuf, "message Outer {\n oneof value {\n  string text = 1;\n }\n}\nservice API {\n rpc Run(Outer) returns (Outer) {\n  option deprecated = true;\n }\n}", []FoldRange{{0, 4}, {1, 3}, {5, 9}, {6, 8}}},
		{"proto comments", Protobuf, "/* first\n*/ option x = \"{\"; /* second\n*/", []FoldRange{{1, 2}}},
		{"proto quoted syntax", Protobuf, "message M {\n string x = 1 [default = \"} /*\"];\n}", []FoldRange{{0, 2}}},
		{"starlark suites", Starlark, "def f(x):\n    if x:\n        return 1\n    else:\n        return 2\nvalue = f(True)", []FoldRange{{0, 4}, {1, 2}, {3, 4}}},
		{"starlark multiline header", Starlark, "def f(\n    x,\n):\n    return x\nvalue = f(1)", []FoldRange{{0, 3}}},
		{"starlark continuation", Starlark, "if ready and \\\n   enabled:\n    work()\ntail()", []FoldRange{{0, 2}}},
		{"starlark containers", Starlark, "load(\n \"@repo//pkg:defs.bzl\",\n \"rule\",\n)\nvalues = [\n x for x in source if x\n]", []FoldRange{{0, 3}, {4, 6}}},
		{"starlark literal", Starlark, "def f():\n    text = r\"\"\"start\nnot a suite:\n } #\nend\"\"\"\n    return text", []FoldRange{{0, 5}, {1, 4}}},
		{"starlark adjacent literals", Starlark, "x = \"\"\"a\n\"\"\"; y = br\"\"\"b\n\"\"\"", []FoldRange{{1, 2}}},
		{"starlark reserved suites", Starlark, "class A:\n    value = 1\nwhile ready:\n    work()\ntry:\n    work()", nil},
		{"starlark compact condition", Starlark, "if(True):\n    work()\ntail()", []FoldRange{{0, 1}}},
		{"starlark tabs rejected", Starlark, "if ready:\n\twork()", nil},
		{"starlark no indentation", Starlark, "if ready:\nwork()", nil},
		{"starlark dictionary colon", Starlark, "values = {\n \"if\":\n  1,\n}", []FoldRange{{0, 3}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			l, _ := Builtin(tt.id)
			if l.Folding() == nil {
				t.Fatal("missing folding provider")
			}
			got, err := l.Folding()(context.Background(), tt.text)
			if err != nil || !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("folds = %v, %v; want %v", got, err, tt.want)
			}
		})
	}
}

func TestDataFoldingIncompleteAndTrailingCode(t *testing.T) {
	for _, id := range []ID{Zig, R, GraphQL, HCL, Protobuf, Starlark} {
		t.Run(string(id), func(t *testing.T) {
			l, _ := Builtin(id)
			fold := l.Folding()
			for _, text := range []string{"value = {\n x", "value = {\n ]\n}"} {
				got, err := fold(context.Background(), text)
				if err != nil || len(got) != 0 {
					t.Fatalf("incomplete %q = %v, %v", text, got, err)
				}
			}
			got, err := fold(context.Background(), "value = {\n x\n} tail")
			if err != nil || !reflect.DeepEqual(got, []FoldRange{{0, 1}}) {
				t.Fatalf("trailing source = %v, %v", got, err)
			}
		})
	}
}

func TestHCLFoldingLongHeaders(t *testing.T) {
	l, _ := Builtin(HCL)
	label := "TEXT" + strings.Repeat("x", 5000)
	for at := 4090; at <= 4097; at++ {
		pad := strings.Repeat(" ", at)
		text := "value=<<-" + label + "\n" + pad + "%{ if enabled }\nbody\n" + pad + "%{ endif }\n" + pad + label
		got, err := l.Folding()(context.Background(), text)
		if err != nil || !reflect.DeepEqual(got, []FoldRange{{0, 4}, {1, 3}}) {
			t.Fatalf("offset %d: %v, %v", at, got, err)
		}
	}
}

func TestDataFoldingTokenizerObservation(t *testing.T) {
	for _, tt := range []struct {
		id   ID
		text string
	}{
		{Zig, "const s =\n \\\\正文 }\n \\\\body\n;\nfn f() void {}"},
		{R, "x <- r\"--[\n正文 }\n]--\"; y <- 'a\nb'"},
		{GraphQL, "\"\"\"正文\n\\\"\"\" }\n\"\"\"\nquery Q { field }"},
		{HCL, "x = <<EOF\n%{ if ready }\n${join(\"\", [\"a\", \"b\"])}\n%{ endif }\nEOF\n/* c */"},
		{Protobuf, "message M {\n /* 正文 }\n */ string x = 1;\n}"},
		{Starlark, "def f(x):\n    text = br\"\"\"正文\n } #\n\"\"\"\n    return x"},
	} {
		t.Run(string(tt.id), func(t *testing.T) {
			l, _ := Builtin(tt.id)
			public := l.Tokenizer()
			for chunk := 1; chunk <= 17; chunk++ {
				lex := &foldLexemes{row: -1}
				if tt.id == HCL {
					lex.hcl = &hclFoldObserver{}
				}
				observed := foldingTokenizer{public, lex}
				a, b := public.InitialState(), public.InitialState()
				for _, line := range strings.Split(tt.text, "\n") {
					for offset := 0; ; {
						in := Input{Text: line[offset:], Offset: offset, Limit: min(chunk, len(line)-offset), LineEnd: true}
						x, err := public.Scan(in, a)
						if err != nil {
							t.Fatal(err)
						}
						y, err := observed.Scan(in, b)
						if err != nil {
							t.Fatal(err)
						}
						a, b = x.State, y.State
						if !a.Equal(b) {
							t.Fatalf("chunk %d offset %d changed state", chunk, offset)
						}
						y.State = x.State
						if !reflect.DeepEqual(x, y) {
							t.Fatalf("chunk %d offset %d changed tokens: %v / %v", chunk, offset, x, y)
						}
						offset += x.Consumed
						if offset == len(line) {
							break
						}
						if x.Consumed <= 0 {
							t.Fatal("no progress")
						}
					}
				}
			}
		})
	}
}

func TestDataFoldingFragmentsAndWorkers(t *testing.T) {
	for _, tt := range []struct {
		id          ID
		open, close string
	}{
		{Zig, "\\\\first", "\\\\last"}, {R, "x <- r\"--[", "]--\""},
		{GraphQL, "\"\"\"", "\"\"\""}, {HCL, "x = <<EOF", "EOF"},
		{Protobuf, "/*", "*/"}, {Starlark, "x = br\"\"\"", "\"\"\""},
	} {
		t.Run(string(tt.id), func(t *testing.T) {
			l, _ := Builtin(tt.id)
			if l.Folding() == nil {
				t.Fatal("missing folding provider")
			}
			fold := l.Folding()
			var wg sync.WaitGroup
			for i := 0; i < 8; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					for at := 4090; at <= 4097; at++ {
						pad := strings.Repeat(" ", at)
						body := strings.Repeat("x", 9000)
						if tt.id == Zig {
							body = "\\\\" + body
						}
						text := strings.Repeat("\n", i) + pad + tt.open + "\n" + body + "\n" + pad + tt.close
						got, err := fold(context.Background(), text)
						if err != nil || !reflect.DeepEqual(got, []FoldRange{{i, i + 2}}) {
							t.Errorf("worker %d offset %d: %v, %v", i, at, got, err)
						}
					}
				}(i)
			}
			wg.Wait()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			got, err := structuralFolding(tt.id, cancelCodeScanner{l.Tokenizer(), cancel})(ctx, strings.Repeat("x ", 16000))
			if err != context.Canceled || got != nil {
				t.Fatalf("mid-scan cancellation = %v, %v", got, err)
			}
		})
	}
}
