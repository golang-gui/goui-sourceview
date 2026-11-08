package language

import (
	"context"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestBuiltinInterpolatedFolding(t *testing.T) {
	for _, id := range []ID{Kotlin, Swift, Dart, Scala, Groovy, PowerShell} {
		t.Run(string(id), func(t *testing.T) {
			l, ok := Builtin(id)
			if !ok || l.Folding() == nil {
				t.Fatal("builtin has no folding provider")
			}
			text := "{\n {\n  work();\n }\n}\n"
			got, err := l.Folding()(context.Background(), text)
			if err != nil || !reflect.DeepEqual(sortedCodeFolds(got), []FoldRange{{0, 4}, {1, 3}}) {
				t.Fatalf("folds = %v, %v", got, err)
			}
		})
	}
}

func TestInterpolatedFoldingBoundaries(t *testing.T) {
	lines := func(s ...string) string { return strings.Join(s, "\n") }
	tests := []struct {
		name string
		id   ID
		text string
		want []FoldRange
	}{
		{"kotlin triple", Kotlin, lines(`val s = """`, `} [ /*`, `"""`), []FoldRange{{0, 2}}},
		{"kotlin multi dollar nested", Kotlin, lines(`val s = $$"""`, `$${`, ` """`, ` } [`, ` """`, `}`, `"""`), []FoldRange{{0, 6}, {1, 5}, {2, 4}}},
		{"kotlin adjacent", Kotlin, lines(`val a = """`, `"""; val b = """`, `"""`), []FoldRange{{1, 2}}},
		{"kotlin aborted nested", Kotlin, lines(`val s = """`, `${`, ` "broken`, `}`, `"""`), []FoldRange{{0, 4}, {1, 3}}},
		{"swift extended triple", Swift, lines(`let s = ##"""`, ` } """ /* [`, `"""##`), []FoldRange{{0, 2}}},
		{"swift nested interpolation", Swift, lines(`let s = #"""`, `\#(`, ` """`, ` } [`, ` """`, `)`, `"""#`), []FoldRange{{0, 6}, {1, 5}, {2, 4}}},
		{"swift extended regex", Swift, lines(`let r = #/`, ` [}](abc)`, `/#`), []FoldRange{{0, 2}}},
		{"swift adjacent", Swift, lines(`let a = """`, `"""; let b = """`, `"""`), []FoldRange{{1, 2}}},
		{"swift aborted nested", Swift, lines(`let s = """`, `\(`, ` "broken`, `)`, `"""`), []FoldRange{{0, 4}, {1, 3}}},
		{"dart raw triple", Dart, lines(`var s = r'''`, `${ } [ /*`, `''';`), []FoldRange{{0, 2}}},
		{"dart nested interpolation", Dart, lines(`var s = """`, `${`, ` r'''`, ` } [`, ` '''`, `}`, `""";`), []FoldRange{{0, 6}, {1, 5}, {2, 4}}},
		{"dart adjacent", Dart, lines(`var a = '''`, `'''; var b = '''`, `''';`), []FoldRange{{1, 2}}},
		{"dart aborted nested", Dart, lines(`var s = """`, `${`, ` "broken`, `}`, `""";`), []FoldRange{{0, 4}, {1, 3}}},
		{"scala triple", Scala, lines(`val s = """`, ` } [ /*`, `"""`), []FoldRange{{0, 2}}},
		{"scala nested interpolation", Scala, lines(`val s = raw"""`, `${`, ` s"""`, ` } [`, ` """`, `}`, `"""`), []FoldRange{{0, 6}, {1, 5}, {2, 4}}},
		{"scala adjacent", Scala, lines(`val a = """`, `"""; val b = """`, `"""`), []FoldRange{{1, 2}}},
		{"scala aborted nested", Scala, lines(`val s = s"""`, `${`, ` "broken`, `}`, `"""`), []FoldRange{{0, 4}, {1, 3}}},
		{"scala indentation scope", Scala, lines(`object Main:`, ` def f =`, `  work()`), nil},
		{"groovy slashy", Groovy, lines(`def s = /`, ` } [`, `/`), []FoldRange{{0, 2}}},
		{"groovy dollar slashy", Groovy, lines(`def s = $/`, ` } $$ $/ [`, `/$`), []FoldRange{{0, 2}}},
		{"groovy nested interpolation", Groovy, lines(`def s = """`, `${`, ` '''`, ` } [`, ` '''`, `}`, `"""`), []FoldRange{{0, 6}, {1, 5}, {2, 4}}},
		{"groovy adjacent", Groovy, lines(`def a = '''`, `'''; def b = '''`, `'''`), []FoldRange{{1, 2}}},
		{"groovy continued quote", Groovy, "def s = \"a\\\n } [\\\nb\"", []FoldRange{{0, 2}}},
		{"powershell here string", PowerShell, lines(`$s = @'`, ` } [ $( #region`, `'@`), []FoldRange{{0, 2}}},
		{"powershell ordinary multiline", PowerShell, lines(`$s = '`, ` } [ ''`, `'`), []FoldRange{{0, 2}}},
		{"powershell nested interpolation", PowerShell, lines(`$s = @"`, `$(`, `@"`, ` } [`, `"@`, `)`, `"@`), []FoldRange{{0, 6}, {1, 5}, {2, 4}}},
		{"powershell adjacent", PowerShell, lines(`$a = '`, `'; $b = '`, `'`), []FoldRange{{1, 2}}},
		{"powershell invalid here opener", PowerShell, lines(`$s = @' invalid`, `{`, ` work()`, `}`), []FoldRange{{1, 3}}},
		{"powershell indented terminator", PowerShell, lines(`$s = @"`, ` "@`, ` } [`, `"@`), []FoldRange{{0, 3}}},
		{"powershell block comments", PowerShell, lines(`<#`, ` } [ #region`, `#>`), []FoldRange{{0, 2}}},
		{"powershell nonnested comments", PowerShell, lines(`<#`, ` <#`, `#>`, `{`, ` work()`, `}`), []FoldRange{{0, 2}, {3, 5}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l, _ := Builtin(tt.id)
			got, err := l.Folding()(context.Background(), tt.text)
			if err != nil || !reflect.DeepEqual(sortedCodeFolds(got), tt.want) {
				t.Fatalf("folds = %v, %v; want %v", got, err, tt.want)
			}
		})
	}
}

func TestInterpolatedFoldingRegionsAndIncompleteInput(t *testing.T) {
	for _, id := range []ID{Kotlin, Swift, Dart, Scala, Groovy, PowerShell} {
		t.Run(string(id), func(t *testing.T) {
			l, _ := Builtin(id)
			open, close := "// #region name", "// #endregion"
			if id == PowerShell {
				open, close = "#ReGiOn name", "#EnDrEgIoN"
			}
			for _, tt := range []struct {
				name, text string
				want       []FoldRange
			}{
				{"nested", open + "\n" + open + "\n work();\n" + close + "\n" + close, []FoldRange{{0, 4}, {1, 3}}},
				{"crossing", "{\n" + open + "\n work();\n}\n" + close, []FoldRange{{0, 3}}},
				{"unclosed region", open + "\nwork();", nil},
				{"unclosed block", "{\n work();", nil},
				{"mismatched block", "{\n ]\n}", nil},
				{"closing statement", "{\n work();\n} next();", []FoldRange{{0, 1}}},
				{"single line", "{ work(); }", nil},
			} {
				t.Run(tt.name, func(t *testing.T) {
					got, err := l.Folding()(context.Background(), tt.text)
					if err != nil || !reflect.DeepEqual(sortedCodeFolds(got), tt.want) {
						t.Fatalf("folds = %v, %v; want %v", got, err, tt.want)
					}
				})
			}
		})
	}
}

func TestInterpolatedFoldingChunksAndFakeMarkers(t *testing.T) {
	for _, tt := range []struct {
		id                ID
		open, close, body string
	}{
		{Kotlin, `$$"""`, `"""`, "// region\n// endregion"},
		{Swift, `##"""`, `"""##`, "// region\n// endregion"},
		{Dart, `r'''`, `'''`, "// region\n// endregion"},
		{Scala, `raw"""`, `"""`, "// region\n// endregion"},
		{Groovy, `$/`, `/$`, "// region\n// endregion"},
		{PowerShell, `@'`, `'@`, "#region\n#endregion"},
	} {
		t.Run(string(tt.id), func(t *testing.T) {
			l, _ := Builtin(tt.id)
			for at := 4090; at <= 4097; at++ {
				close := strings.Repeat(" ", at) + tt.close
				if tt.id == PowerShell {
					// A here-string terminator must be in column zero. Exercise
					// long closing-line offsets with an ordinary multiline quote.
					close = tt.close
				}
				text := strings.Repeat(" ", at) + tt.open + "\n" + tt.body + "\n" + close
				got, err := l.Folding()(context.Background(), text)
				if err != nil || !reflect.DeepEqual(got, []FoldRange{{0, 3}}) {
					t.Fatalf("offset %d: %v, %v", at, got, err)
				}
			}
			blockOpen, blockClose := "/*", "*/"
			if tt.id == PowerShell {
				blockOpen, blockClose = "<#", "#>"
			}
			got, err := l.Folding()(context.Background(), blockOpen+"\n"+tt.body+"\n"+blockClose)
			if err != nil || !reflect.DeepEqual(got, []FoldRange{{0, 3}}) {
				t.Fatalf("comment markers = %v, %v", got, err)
			}
			got, err = l.Folding()(context.Background(), tt.open+"\n"+tt.body)
			if err != nil || got != nil {
				t.Fatalf("unclosed literal = %v, %v", got, err)
			}
		})
	}
	// A smart quote terminator occupies multiple source bytes at a chunk edge.
	l, _ := Builtin(PowerShell)
	text := "'\n" + strings.Repeat(" ", 4095) + "’"
	got, err := l.Folding()(context.Background(), text)
	if err != nil || !reflect.DeepEqual(got, []FoldRange{{0, 1}}) {
		t.Fatalf("PowerShell source offsets = %v, %v", got, err)
	}
}

func TestInterpolatedNestedCommentFolding(t *testing.T) {
	for _, id := range []ID{Kotlin, Swift, Dart, Scala} {
		t.Run(string(id), func(t *testing.T) {
			l, _ := Builtin(id)
			text := "/* outer\n /* inner\n  } [\n */\n*/"
			got, err := structuralFolding(id, l.Tokenizer())(context.Background(), text)
			if err != nil || !reflect.DeepEqual(sortedCodeFolds(got), []FoldRange{{0, 4}, {1, 3}}) {
				t.Fatalf("nested comments = %v, %v", got, err)
			}
		})
	}
}

func TestInterpolatedFoldingIsolation(t *testing.T) {
	for _, tt := range []struct {
		id          ID
		open, close string
	}{
		{Kotlin, `"""`, `"""`},
		{Swift, `#"""`, `"""#`},
		{Dart, `r'''`, `'''`},
		{Scala, `raw"""`, `"""`},
		{Groovy, `$/`, `/$`},
		{PowerShell, `@"`, `"@`},
	} {
		t.Run(string(tt.id), func(t *testing.T) {
			l, _ := Builtin(tt.id)
			fold := l.Folding()
			var workers sync.WaitGroup
			for i := 0; i < 8; i++ {
				workers.Add(1)
				go func(i int) {
					defer workers.Done()
					text := strings.Repeat("\n", i) + tt.open + "\n" + strings.Repeat("x", 8200) + "\n" + tt.close
					got, err := fold(context.Background(), text)
					if err != nil || !reflect.DeepEqual(got, []FoldRange{{i, i + 2}}) {
						t.Errorf("worker %d: %v, %v", i, got, err)
					}
				}(i)
			}
			workers.Wait()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			scanner := cancelCodeScanner{l.Tokenizer(), cancel}
			got, err := structuralFolding(tt.id, scanner)(ctx, "{\n"+strings.Repeat("x ", 16000)+"\n}")
			if err != context.Canceled || got != nil {
				t.Fatalf("mid-scan cancellation = %v, %v", got, err)
			}
		})
	}
}
