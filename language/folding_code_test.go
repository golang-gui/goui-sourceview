package language

import (
	"context"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
)

func sortedCodeFolds(ranges []FoldRange) []FoldRange {
	sort.Slice(ranges, func(i, j int) bool {
		if ranges[i].StartLine != ranges[j].StartLine {
			return ranges[i].StartLine < ranges[j].StartLine
		}
		return ranges[i].EndLine < ranges[j].EndLine
	})
	return ranges
}

func TestCodeFoldingLexicalBoundaries(t *testing.T) {
	tests := []struct {
		name string
		id   ID
		text string
		want []FoldRange
	}{
		{"c block", C, "void f() {\n  puts(\"} [\"); // {\n}\n", []FoldRange{{0, 2}}},
		{"c macro braces", C, "void f() {\n#define CLOSE } \\\n  ]\n  run();\n}\n", []FoldRange{{0, 4}}},
		{"c continued string", C, "char *s = \"a\\\n}\\\nb\";\n", []FoldRange{{0, 2}}},
		{"c aborted string", C, "char *s = \"a\\\nb\nvoid f() {\n  work();\n}\n", []FoldRange{{2, 4}}},
		{"cpp raw", Cpp, "auto s = u8R\"tag(\n} /* {\n)tag\";\n", []FoldRange{{0, 2}}},
		{"cpp adjacent raw", Cpp, "auto a = R\"x(\n)x\"; auto b = R\"x(\n)x\";\n", []FoldRange{{1, 2}}},
		{"cpp adjacent comments", Cpp, "/* first\n*/ int a; /* second\n*/\n", []FoldRange{{1, 2}}},
		{"java text block", Java, "String s = \"\"\"\n} /* {\n\"\"\";\n", []FoldRange{{0, 2}}},
		{"java escaped delimiters", Java, "class C \\uu007b\n  void f() \\u007b\n    run();\n  \\u007d\n\\u007d\n", []FoldRange{{0, 4}, {1, 3}}},
		{"java escaped comment", Java, "class C {\n\\u002f\\u002a\n} [\n\\u002a\\u002f\n}\n", []FoldRange{{0, 4}, {1, 3}}},
		{"java escaped string", Java, "class C {\n String s = \\u0022} [\\u0022;\n}\n", []FoldRange{{0, 2}}},
		{"java physical lines", Java, "class C \\u007b\\u000a\n x();\n\\u007d next();\n", []FoldRange{{0, 1}}},
		{"cs verbatim", CSharp, "var s = @\"\n} /* \"\" [\n\";\n", []FoldRange{{0, 2}}},
		{"cs raw", CSharp, "var s = \"\"\"\n} [ /*\n\"\"\";\n", []FoldRange{{0, 2}}},
		{"cs adjacent verbatim", CSharp, "var a = @\"\n\"; var b = @\"\n\";\n", []FoldRange{{1, 2}}},
		{"cs interpolation", CSharp, "var s = $@\"start {\n  F(@\"nested\ntext\")\n} end\";\n", []FoldRange{{0, 3}, {0, 2}, {1, 2}}},
		{"cs nested literals", CSharp, "var s = $@\"start {\n F(@\"nested\n } /*\n \"\n )\n} end\";\n", []FoldRange{{0, 5}, {0, 4}, {1, 3}, {1, 4}}},
		{"cs aborted nested literal", CSharp, "var s = $@\"start {\n @name;\n \"broken\n}\nend\";\n", []FoldRange{{0, 4}, {0, 3}}},
		{"cs raw interpolation", CSharp, "var s = $$\"\"\"\n{{\n F(\"}\")\n}}\n\"\"\";\n", []FoldRange{{0, 4}, {1, 3}, {1, 2}}},
		{"rust raw", Rust, "let s = br##\"\n} /* \"#\n\"##;\n", []FoldRange{{0, 2}}},
		{"rust nested comments", Rust, "/* outer\n /* inner\n } {\n */\n*/\n", []FoldRange{{0, 4}, {1, 3}}},
		{"rust lifetime macro", Rust, "fn f<'a>(x: &'a str) {\n 'outer: loop {\n  vec![\n   r#type, '{', \"}\"\n  ];\n }\n}\n", []FoldRange{{0, 6}, {1, 5}, {2, 4}}},
		{"unclosed", Cpp, "void f() {\n auto x = R\"(\n }\n", nil},
		{"single line", CSharp, "class C {}\nvar s = \"x\";\n/* c */\n", nil},
		{"mismatch", Rust, "fn f() {\n ]\n}\n", nil},
		{"closing statement", Java, "class C {\n run();\n} next();\n", []FoldRange{{0, 1}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l, _ := Builtin(tt.id)
			got, err := structuralFolding(tt.id, l.Tokenizer())(context.Background(), tt.text)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(sortedCodeFolds(got), sortedCodeFolds(tt.want)) {
				t.Fatalf("folds = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCodeFoldingWorkerIsolation(t *testing.T) {
	l, _ := Builtin(CSharp)
	fold := structuralFolding(CSharp, l.Tokenizer())
	var workers sync.WaitGroup
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func(i int) {
			defer workers.Done()
			text := strings.Repeat("\n", i) + "var s = @\"\n" + strings.Repeat("x", 8200) + "\n\";"
			got, err := fold(context.Background(), text)
			if err != nil || !reflect.DeepEqual(got, []FoldRange{{i, i + 2}}) {
				t.Errorf("worker %d: %v, %v", i, got, err)
			}
		}(i)
	}
	workers.Wait()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got, err := fold(ctx, "class C {\n}\n"); err != context.Canceled || got != nil {
		t.Fatalf("cancelled = %v, %v", got, err)
	}
}
