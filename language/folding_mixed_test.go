package language

import (
	"context"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestBuiltinMixedFolding(t *testing.T) {
	for _, id := range []ID{JSX, TSX, PHP} {
		t.Run(string(id), func(t *testing.T) {
			l, _ := Builtin(id)
			if l.Folding() == nil {
				t.Fatal("builtin has no mixed folding provider")
			}
			got, err := l.Folding()(context.Background(), "<div>\n text\n</div>")
			if err != nil || !reflect.DeepEqual(got, []FoldRange{{0, 2}}) {
				t.Fatalf("folds = %v, %v", got, err)
			}
		})
	}
}

func TestJSXMixedFolding(t *testing.T) {
	for _, id := range []ID{JSX, TSX} {
		t.Run(string(id), func(t *testing.T) {
			l, _ := Builtin(id)
			fold := mixedFolding(id, l.Tokenizer())
			for _, tt := range []struct {
				name, text string
				want       []FoldRange
			}{
				{"tags and fragments", "const x = <>\n <UI.Panel>\n  <svg:path />\n </UI.Panel>\n</>;", []FoldRange{{0, 4}, {1, 3}}},
				{"code in tags", "const x = <Panel>\n {items.map(x => (\n  <Row>\n   {x}\n  </Row>\n ))}\n</Panel>;", []FoldRange{{0, 6}, {1, 5}, {2, 4}}},
				{"nested attribute JSX", "const x = <Outer attr={\n <Inner>\n  value\n </Inner>\n}>\n text\n</Outer>;", []FoldRange{{0, 6}, {1, 3}}},
				{"template interpolation", "const x = <Panel>\n {`start\n ${\n  `inner\n  body`\n }\n end`}\n</Panel>;", []FoldRange{{0, 7}, {1, 6}, {2, 5}, {3, 4}}},
				{"JSX inside template", "const s = `start\n${\n <A>\n text\n </A>\n}\nend`;", []FoldRange{{0, 6}, {1, 5}, {2, 4}}},
				{"attribute quote", "const x = <Panel\n title=\"first\n second\"\n>\n body\n</Panel>;", []FoldRange{{0, 5}, {1, 2}}},
				{"fake tags in code", "function f() {\n const s = '<Fake>';\n const r = /<Fake>/;\n return 1;\n}", []FoldRange{{0, 4}}},
				{"comment tags", "/* <Fake>\n </Fake> */", []FoldRange{{0, 1}}},
				{"text braces", "const x = <Panel>\n [ ( ) ]\n</Panel>;", []FoldRange{{0, 2}}},
				{"self closing", "const x = <Panel\n title=\"body\"\n/>;", nil},
				{"case mismatch", "const x = <Panel>\n body\n</panel>;", nil},
				{"nest mismatch", "const x = <A>\n <B>\n </A>\n</B>;", nil},
				{"unclosed", "const x = <A>\n text", nil},
				{"closing statement", "const x = <A>\n text\n</A>; other();", []FoldRange{{0, 1}}},
				{"code regions", "// region\nconst x = <A>\n text\n</A>;\n// endregion", []FoldRange{{0, 4}, {1, 3}}},
				{"text fake regions", "const x = <A>\n // region\n // endregion\n</A>;", []FoldRange{{0, 3}}},
			} {
				t.Run(tt.name, func(t *testing.T) {
					got, err := fold(context.Background(), tt.text)
					if err != nil || !reflect.DeepEqual(got, tt.want) {
						t.Fatalf("folds = %v, %v; want %v", got, err, tt.want)
					}
				})
			}
		})
	}
	l, _ := Builtin(TSX)
	for _, text := range []string{"const f = <T,>(x: T) => {\n return x;\n};", "const f = <T extends object>(x: T) => {\n return x;\n};", "if (a < b) {\n work();\n}"} {
		got, err := mixedFolding(TSX, l.Tokenizer())(context.Background(), text)
		if err != nil || !reflect.DeepEqual(got, []FoldRange{{0, 2}}) {
			t.Fatalf("TSX code %q = %v, %v", text, got, err)
		}
	}
}

func TestPHPMixedFolding(t *testing.T) {
	l, _ := Builtin(PHP)
	fold := mixedFolding(PHP, l.Tokenizer())
	for _, tt := range []struct {
		name, text string
		want       []FoldRange
	}{
		{"HTML and PHP braces", "<DIV>\n<?php if ($ok) { ?>\n <span>\n  <?= $value ?>\n </span>\n<?php } ?>\n</div>", []FoldRange{{0, 6}, {1, 4}, {2, 4}}},
		{"alternative templates", "<main>\n<?php if ($ok): ?>\n <section>\n <?php foreach ($xs as $x): ?>\n  <span>\n   <?= $x ?>\n  </span>\n <?php endforeach; ?>\n </section>\n<?php endif; ?>\n</main>", []FoldRange{{0, 10}, {1, 8}, {2, 8}, {3, 6}, {4, 6}}},
		{"heredoc", "<?php\n$x = <<<TEXT\n<fake> } [\nTEXT;", []FoldRange{{1, 3}}},
		{"nowdoc", "<?php\n$x = <<<'TEXT'\n<?php <fake> } [\n  TEXT;", []FoldRange{{1, 3}}},
		{"nested literal", "<?php\n$x = \"start {$a[\n 'inner\n text'\n]} end\";", []FoldRange{{1, 4}, {2, 3}}},
		{"adjacent strings", "<?php\n$a = '\n'; $b = '\n';", []FoldRange{{2, 3}}},
		{"comments", "<div>\n<?php /* <fake>\n } [ */ ?>\n</div>", []FoldRange{{0, 3}}},
		{"HTML comment", "<!-- <?php echo '-->'; ?>\n </fake>\n-->", []FoldRange{{0, 2}}},
		{"static attributes", "<div title=\"<?= '\"<fake>' ?>\">\n body\n</div>", []FoldRange{{0, 2}}},
		{"dynamic names", "<h<?= $level ?>>\n body\n</h<?= $level ?>>", nil},
		{"raw script", "<script>\n const x = '<fake>';\n <?php echo '</script>'; ?>\n</script>", []FoldRange{{0, 3}}},
		{"line comment switch", "<?php // comment ?> <div>\n<?php if ($x) { ?>\n text\n<?php } ?>\n</div>", []FoldRange{{0, 4}, {1, 2}}},
		{"implicit closing semicolon", "<?php if ($x): ?>\n body\n<?php endif ?>", []FoldRange{{0, 1}}},
		{"regions", "<?php\n#region\n$x = 1;\n#endregion", []FoldRange{{1, 3}}},
		{"HTML fake regions", "#region\n<div>\n text\n</div>\n#endregion", []FoldRange{{1, 3}}},
		{"unclosed", "<div>\n<?php if ($x): ?>\n text", nil},
		{"invalid heredoc", "<?php\n$x = <<<;\n{\n work();\n}", []FoldRange{{2, 4}}},
		{"unrelated closing text", "<div>\n body\n</div> next", []FoldRange{{0, 1}}},
		{"crossing template", "<?php if ($x): ?>\n <div>\n<?php endif; ?>\n text\n</div>", []FoldRange{{0, 1}}},
		{"branch and nested condition", "<?php IF (test($x ? f() : g())): ?>\n one\n<?php elseif ($other): ?>\n two\n<?php else: ?>\n three\n<?php ENDIF; ?>", []FoldRange{{0, 5}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := fold(context.Background(), tt.text)
			if err != nil || !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("folds = %v, %v; want %v", got, err, tt.want)
			}
		})
	}
	for _, pair := range [][2]string{{"if", "endif"}, {"while", "endwhile"}, {"for", "endfor"}, {"foreach", "endforeach"}, {"switch", "endswitch"}, {"declare", "enddeclare"}} {
		text := "<?php " + pair[0] + " (\n value()\n): ?>\n body\n<?php " + pair[1] + "; ?>"
		got, err := fold(context.Background(), text)
		if err != nil || !reflect.DeepEqual(got, []FoldRange{{0, 3}}) {
			t.Errorf("alternative %s: %v, %v", pair[0], got, err)
		}
	}
	for _, text := range []string{"<?php $o->if($v): ?>\n body\n<?php endif; ?>", "<?php A::if($v): ?>\n body\n<?php endif; ?>", "<?php if ($v) work(); ?>\n body\n<?php endif; ?>", "<?php if ($v): ?>\n body\n<?php endwhile; ?>"} {
		got, err := fold(context.Background(), text)
		if err != nil || got != nil {
			t.Errorf("false control %q: %v, %v", text, got, err)
		}
	}
}

func TestMixedFoldingChunksAndWorkers(t *testing.T) {
	for _, id := range []ID{JSX, TSX, PHP} {
		t.Run(string(id), func(t *testing.T) {
			l, _ := Builtin(id)
			fold := mixedFolding(id, l.Tokenizer())
			name := "Panel" + strings.Repeat("x", 5000)
			for at := 4090; at <= 4097; at++ {
				text := strings.Repeat(" ", at) + "<" + name + ">\n text\n</" + name + ">"
				got, err := fold(context.Background(), text)
				if err != nil || !reflect.DeepEqual(got, []FoldRange{{0, 2}}) {
					t.Fatalf("long name/offset %d = %v, %v", at, got, err)
				}
			}
			var group sync.WaitGroup
			for i := 0; i < 8; i++ {
				group.Add(1)
				go func(i int) {
					defer group.Done()
					text := strings.Repeat("\n", i) + "<A>\n" + strings.Repeat("x", 9000) + "\n</A>"
					want := []FoldRange{{i, i + 2}}
					if id == PHP {
						text = strings.Repeat("\n", i) + "<A>\n<?php $x = 'start\n" + strings.Repeat("x", 9000) + "\n'; ?>\n</A>"
						want = []FoldRange{{i, i + 4}, {i + 1, i + 2}}
					}
					got, err := fold(context.Background(), text)
					if err != nil || !reflect.DeepEqual(got, want) {
						t.Errorf("worker %d = %v, %v", i, got, err)
					}
				}(i)
			}
			group.Wait()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			scanner := cancelCodeScanner{l.Tokenizer(), cancel}
			got, err := mixedFolding(id, scanner)(ctx, "<A>\n"+strings.Repeat("x ", 16000)+"\n</A>")
			if err != context.Canceled || got != nil {
				t.Fatalf("mid-scan cancellation = %v, %v", got, err)
			}
		})
	}
}

func TestMixedLiteralChunkBoundaries(t *testing.T) {
	for _, id := range []ID{JSX, TSX, PHP} {
		t.Run(string(id), func(t *testing.T) {
			l, _ := Builtin(id)
			label := "TEXT" + strings.Repeat("x", 5000)
			for at := 4090; at <= 4097; at++ {
				pad := strings.Repeat(" ", at)
				text := "const x = <A>\n" + pad + "{`hello\n" + pad + "body`}\n</A>;"
				want := []FoldRange{{0, 3}, {1, 2}}
				if id == PHP {
					text = "<main>\n" + pad + "<?php $x = <<<'" + label + "'\n } <fake>\n" + pad + label + ";\n?>\n</main>"
					want = []FoldRange{{0, 5}, {1, 3}}
				}
				got, err := l.Folding()(context.Background(), text)
				if err != nil || !reflect.DeepEqual(got, want) {
					t.Fatalf("offset %d = %v, %v; want %v", at, got, err, want)
				}
			}
		})
	}
}
