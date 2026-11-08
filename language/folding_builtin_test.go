package language

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestBuiltinCodeFolding(t *testing.T) {
	for _, id := range []ID{C, Cpp, Java, CSharp, Rust} {
		t.Run(string(id), func(t *testing.T) {
			l, ok := Builtin(id)
			if !ok || l.Folding() == nil {
				t.Fatal("builtin has no folding provider")
			}
			got, err := l.Folding()(context.Background(), "// region outer\n{\n /* block\n  } [\n */\n}\n// endregion\n")
			want := []FoldRange{{0, 6}, {1, 5}, {2, 4}}
			if err != nil || !reflect.DeepEqual(sortedCodeFolds(got), want) {
				t.Fatalf("folds = %v, %v; want %v", got, err, want)
			}
		})
	}
}

func TestCodeFoldingRegions(t *testing.T) {
	tests := []struct {
		name string
		id   ID
		text string
		want []FoldRange
	}{
		{"cpp pragmas", Cpp, "#pragma region outer\n # pragma region inner\n run();\n #pragma endregion\n#pragma endregion", []FoldRange{{0, 4}, {1, 3}}},
		{"c pragmas", C, "#pragma region\n#define CLOSE } \\\n // endregion\n run();\n#pragma endregion", []FoldRange{{0, 4}}},
		{"cs directives", CSharp, "#region outer\n #region inner\n run();\n #endregion\n#endregion", []FoldRange{{0, 4}, {1, 3}}},
		{"java comments", Java, "// #region outer\n run();\n// #endregion", []FoldRange{{0, 2}}},
		{"rust comments", Rust, "// region outer\n run();\n// endregion", []FoldRange{{0, 2}}},
		{"unclosed", Java, "// region\nrun();", nil},
		{"crossing", CSharp, "class C {\n#region r\n work();\n}\n#endregion", []FoldRange{{0, 3}}},
		{"cs fake raw regions", CSharp, "var s = \"\"\"\n#region fake\n#endregion\n\"\"\";", []FoldRange{{0, 3}}},
		{"cpp fake regions", Cpp, "auto s = R\"(\n// region fake\n// endregion\n)\";", []FoldRange{{0, 3}}},
		{"rust fake regions", Rust, "/*\n// region fake\n// endregion\n*/", []FoldRange{{0, 3}}},
		{"unsupported directive", C, "#region r\nrun();\n#endregion", nil},
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

func TestCodeFoldingChunkBoundaries(t *testing.T) {
	for _, tt := range []struct {
		id          ID
		open, close string
	}{
		{C, "/*", "*/"},
		{Cpp, "R\"tag(", ")tag\";"},
		{Java, "\\uu007b", "\\u007d"},
		{CSharp, "$@\"", "\";"},
		{Rust, "r###\"", "\"###;"},
	} {
		t.Run(string(tt.id), func(t *testing.T) {
			l, _ := Builtin(tt.id)
			for at := 4091; at <= 4097; at++ {
				text := strings.Repeat(" ", at) + tt.open + "\n text\n" + strings.Repeat(" ", at) + tt.close
				got, err := l.Folding()(context.Background(), text)
				if err != nil || !reflect.DeepEqual(got, []FoldRange{{0, 2}}) {
					t.Fatalf("offset %d: %v, %v", at, got, err)
				}
			}
		})
	}
}

type cancelCodeScanner struct {
	Tokenizer
	cancel context.CancelFunc
}

func (t cancelCodeScanner) Scan(in Input, state State) (Result, error) {
	r, err := t.Tokenizer.Scan(in, state)
	if in.Offset >= 4096 {
		t.cancel()
	}
	return r, err
}

func TestCodeFoldingCancellationDuringLongLine(t *testing.T) {
	l, _ := Builtin(Cpp)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	scanner := cancelCodeScanner{l.Tokenizer(), cancel}
	fold := structuralFolding(Cpp, scanner)
	text := "{\n" + strings.Repeat("x ", 16000) + "\n}"
	if got, err := fold(ctx, text); err != context.Canceled || got != nil {
		t.Fatalf("mid-scan cancellation = %v, %v", got, err)
	}
}
