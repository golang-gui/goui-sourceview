package language

import (
	"context"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestBuiltinDiagramDiffFolding(t *testing.T) {
	for _, id := range []ID{DOT, Diff} {
		l, _ := Builtin(id)
		if l.Folding() == nil {
			t.Errorf("%s: builtin has no basic diagram/diff folding provider", id)
		}
	}
}

func TestDiagramDiffFoldingRanges(t *testing.T) {
	for _, tt := range []struct {
		name string
		id   ID
		text string
		want []FoldRange
	}{
		{"dot nested", DOT, "digraph G {\n subgraph cluster {\n  a -> b;\n }\n}", []FoldRange{{0, 4}, {1, 3}}},
		{"dot attributes", DOT, "node [\n label=\"[ }\"\n color=red\n];", []FoldRange{{0, 3}}},
		{"dot closing tail", DOT, "subgraph {\n a;\n} b;\n[\n color=red\n] tail", []FoldRange{{0, 1}, {3, 4}}},
		{"dot comments", DOT, "/* first\n { [ fake\n*/\n/* second\n*/ tail;", []FoldRange{{0, 2}}},
		{"dot adjacent comments", DOT, "/* first\n*/ tail; /* second\n*/", []FoldRange{{1, 2}}},
		{"dot continued quote", DOT, "label=\"first\\\n } [ fake\\\nlast\";", []FoldRange{{0, 2}}},
		{"dot invalid quote recovery", DOT, "label=\"broken\nsubgraph {\n a;\n}", []FoldRange{{1, 3}}},
		{"dot html opaque", DOT, "digraph G {\n a [label=<\n  <TABLE><TR><TD> } [ </TD></TR>\n  <TR><TD HREF=\"a>b\">hello</TD></TR></TABLE>\n >];\n}", []FoldRange{{0, 5}, {1, 4}}},
		{"dot html comment", DOT, "label=<\n <TABLE><!-- > } [\n fake -->\n <TR><TD>value</TD></TR></TABLE>\n>;", []FoldRange{{0, 4}}},
		{"dot unfinished html", DOT, "label=<\n <TABLE> } [", nil},
		{"dot unfinished quote", DOT, "label=\"first\\\nlast\\", nil},
		{"dot unfinished comment", DOT, "/* first\n } [", nil},
		{"dot unmatched outer", DOT, "digraph G {\n subgraph {\n  a;\n }", []FoldRange{{1, 3}}},
		{"dot mismatched", DOT, "{\n [\n }\n]", nil},
		{"git files and hunks", Diff, "diff --git a/a b/a\nindex 123..456 100644\n--- a/a\n+++ b/a\n@@ -1 +1 @@\n-old\n+new\n@@ -3 +3 @@ name\n-old2\n+new2\ndiff --git a/b b/b\n--- a/b\n+++ b/b\n@@ -0,0 +1 @@\n+next", []FoldRange{{0, 9}, {4, 6}, {7, 9}, {10, 14}, {13, 14}}},
		{"unified files", Diff, "--- old\n+++ new\n@@ -1 +1 @@\n-old\n+new\n--- other\n+++ next\n@@ -0,0 +1 @@\n+line\n", []FoldRange{{0, 4}, {2, 4}, {5, 8}, {7, 8}}},
		{"diff body headings", Diff, "--- a\n+++ b\n@@ -1,2 +1,2 @@\n--- removed\n+++ added\n context", []FoldRange{{0, 5}, {2, 5}}},
		{"diff body markers", Diff, "diff --git a/a b/a\n@@ -1,2 +1,2 @@\n-@@ fake\n+diff --git fake\n diff --git context", []FoldRange{{0, 4}, {1, 4}}},
		{"diff missing newline", Diff, "@@ -1 +1 @@\n-old\n\\ No newline at end of file\n+new\n\\ No newline at end of file", []FoldRange{{0, 4}}},
		{"diff space context", Diff, "@@ -1 +1 @@\n \n", []FoldRange{{0, 1}}},
		{"diff zero count", Diff, "@@ -0,0 +1,2 @@\n+one\n+two", []FoldRange{{0, 2}}},
		{"diff empty hunk", Diff, "@@ -0,0 +0,0 @@", nil},
		{"diff malformed hunk", Diff, "@@ -x +1 @@\n+one\n@@ -1 +1 wrong\n+two", nil},
		{"diff orphan headers", Diff, "--- old\nmetadata\n+++ new\n+one", nil},
		{"diff orphan new header", Diff, "+++ new\n+one", nil},
		{"diff quoted paths", Diff, "diff --git \"a/文件 x\" \"b/文件 x\"\n--- \"a/文件 x\"\n+++ \"b/文件 x\"\n@@ -1 +1 @@\n-old\n+new", []FoldRange{{0, 5}, {3, 5}}},
		{"diff binary opaque", Diff, "diff --git a/a b/a\nGIT binary patch\nliteral 20\n@@ -1 +1 @@\n--- fake\n+++ fake\n+body\ndiff --git a/b b/b\nBinary files a/b and b/b differ", []FoldRange{{0, 6}, {7, 8}}},
		{"diff rename metadata", Diff, "diff --git a/a b/b\nsimilarity index 100%\nrename from a\nrename to b", []FoldRange{{0, 3}}},
		{"diff combined unsupported", Diff, "diff --cc file\n@@@ -1,1 -1,1 +1,1 @@@\n--- fake\n+++ fake\n@@ -1 +1 @@\n+body", nil},
		{"diff standalone marker", Diff, "diff --git a/a b/a", nil},
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

func TestDiagramDiffTokenizerObservation(t *testing.T) {
	for _, tt := range []struct {
		id   ID
		text string
	}{
		{DOT, "digraph G {\n node [正文=\"first\\\n } \\\" last\"]; /* one\n*/ /* two\n*/\n label=<\n <TABLE TITLE=\"a>b\"><!-- > [\n --><TR><TD>正文</TD></TR></TABLE>\n >;\n \"broken\n a -> b;\n}"},
		{Diff, "diff --git a/正文 b/正文\n--- a/正文\n+++ b/正文\n@@ -1,2 +1,2 @@ name\n--- removed\n+++ added\n context\n\\ No newline at end of file\n@@ -0,0 +1 @@\n+new\n@@ -x +1 @@\n+bad\nGIT binary patch"},
	} {
		t.Run(string(tt.id), func(t *testing.T) {
			l, _ := Builtin(tt.id)
			public := l.Tokenizer()
			for chunk := 1; chunk <= 17; chunk++ {
				observed := foldingTokenizer{public, &foldLexemes{row: -1, diff: &diffFoldObserver{}}}
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

func TestDotLiteralChunkBoundaries(t *testing.T) {
	l, _ := Builtin(DOT)
	for at := 4090; at <= 4097; at++ {
		for _, body := range []string{
			"label=\"first\\\n } [ 正文\\\nlast\";",
			"label=<\n <TABLE TITLE=\"a>b\"><TR><TD>正文 } [</TD></TR></TABLE>\n>;",
			"/* first\n 正文 } [\n*/",
		} {
			text := strings.Repeat(" ", at) + body
			got, err := l.Folding()(context.Background(), text)
			if err != nil || !reflect.DeepEqual(got, []FoldRange{{0, 2}}) {
				t.Fatalf("offset %d %q: %v, %v", at, body, got, err)
			}
		}
	}
}

func TestDiagramDiffFoldingWorkers(t *testing.T) {
	for _, id := range []ID{DOT, Diff} {
		t.Run(string(id), func(t *testing.T) {
			l, _ := Builtin(id)
			if l.Folding() == nil {
				t.Fatal("missing folding provider")
			}
			var wg sync.WaitGroup
			for worker := 0; worker < 8; worker++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for at := 4090; at <= 4097; at++ {
						text := strings.Repeat("\n", worker) + strings.Repeat(" ", at) + "/*\n" + strings.Repeat("x", 9000) + "\n*/"
						if id == Diff {
							text = strings.Repeat("\n", worker) + "@@ -1 +1 @@ " + strings.Repeat("x", at) + "\n-" + strings.Repeat("x", 9000) + "\n+new"
						}
						got, err := l.Folding()(context.Background(), text)
						if err != nil || !reflect.DeepEqual(got, []FoldRange{{worker, worker + 2}}) {
							t.Errorf("worker %d offset %d = %v, %v", worker, at, got, err)
						}
					}
				}()
			}
			wg.Wait()
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if got, err := l.Folding()(ctx, "text"); err != context.Canceled || got != nil {
				t.Fatalf("cancelled = %v, %v", got, err)
			}
			ctx, cancel = context.WithCancel(context.Background())
			defer cancel()
			scanner := cancelCodeScanner{l.Tokenizer(), cancel}
			fold := structuralFolding(id, scanner)
			if id == Diff {
				fold = diffFolding(scanner)
			}
			if got, err := fold(ctx, strings.Repeat("x ", 16000)); err != context.Canceled || got != nil {
				t.Fatalf("mid-scan cancellation = %v, %v", got, err)
			}
		})
	}
}
