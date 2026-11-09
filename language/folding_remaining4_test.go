package language

import (
	"context"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestRemainingMermaidFolding(t *testing.T) {
	for _, tt := range []struct {
		name, text string
		want       []FoldRange
	}{
		{"subgraphs", "flowchart TB\nsubgraph outer\n subgraph inner\n A-->B\n end\nend", []FoldRange{{1, 5}, {2, 4}}},
		{"graph alias", "graph LR\nsubgraph id [Title]\n A-->B\nend", []FoldRange{{1, 3}}},
		{"comment without title", "flowchart TB\nsubgraph %% fake\nA-->B\nend", nil},
		{"compact statements omitted", "sequenceDiagram\nloop work; end\nA->>B: text\nend", nil},
		{"quoted title", "flowchart LR\nsubgraph \"Title\"\n A-->B\nend", []FoldRange{{1, 3}}},
		{"comments", "%% flowchart fake\n%%{init: {\nsubgraph fake\n}}%%\nflowchart TB\nsubgraph Real\n%% end\nA-->B\nend %% comment", []FoldRange{{5, 8}}},
		{"quoted fake", "flowchart TB\nA[\"one\nsubgraph fake\nend\nlast\"]\nsubgraph real\nB-->C\nend", []FoldRange{{5, 7}}},
		{"unquoted fake", "flowchart TB\nA[first\nsubgraph fake\nend\nlast]\nsubgraph real\nB-->C\nend", []FoldRange{{5, 7}}},
		{"frontmatter", "---\ntitle: Flow\n---\nflowchart TB\nsubgraph Real\nA-->B\nend", []FoldRange{{4, 6}}},
		{"incomplete frontmatter", "---\ntitle: Flow\nflowchart TB\nsubgraph fake\nA-->B\nend", nil},
		{"sequence branches", "sequenceDiagram\nloop Every minute\nalt Yes\nA->>B: one\nelse No\nA->>B: two\nend\nend", []FoldRange{{1, 7}, {2, 6}}},
		{"parallel and critical", "sequenceDiagram\npar work\ncritical connect\nA->>B: one\noption timeout\nA->>B: two\nend\nand other\nA->>C: three\nend", []FoldRange{{1, 9}, {2, 6}}},
		{"rect opt break", "sequenceDiagram\nrect rgb(0, 0, 0)\nopt work\nbreak failure\nA->>B: one\nend\nend\nend", []FoldRange{{1, 7}, {2, 6}, {3, 5}}},
		{"semicolon close", "sequenceDiagram\nloop work\nA->>B: end\nend; %% done", []FoldRange{{1, 3}}},
		{"punctuation in messages", "sequenceDiagram\nloop work\nA->>B: unmatched ( text\nend", []FoldRange{{1, 3}}},
		{"participant box", "sequenceDiagram\nbox Aqua Group\nparticipant A\nparticipant B\nend", []FoldRange{{1, 4}}},
		{"message words", "sequenceDiagram\nparticipant loop as Person\nloop ->> A: begin\nA->>loop: end\nend", nil},
		{"keyword node", "flowchart TB\nsubgraph --> A\nA-->B\nend", nil},
		{"incomplete", "flowchart TB\nsubgraph outer\nsubgraph inner\nA-->B\nend", []FoldRange{{2, 4}}},
		{"unknown diagram", "classDiagram\nsubgraph fake\nclass A\nend\nflowchart TB\nsubgraph stillFake\nA-->B\nend", nil},
		{"no diagram declaration", "subgraph fake\nA-->B\nend", nil},
		{"no bracket fallback", "flowchart TB\nA[one\ntwo\nlast]", nil},
		{"no indent fallback", "mindmap\n root\n  child\n  other", nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			l, _ := Builtin(Mermaid)
			if l.Folding() == nil {
				t.Fatal("missing folding")
			}
			got, err := l.Folding()(context.Background(), tt.text)
			if err != nil || !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("%v %v; want %v", got, err, tt.want)
			}
		})
	}
}

func TestRemainingMermaidWorkers(t *testing.T) {
	l, _ := Builtin(Mermaid)
	var wg sync.WaitGroup
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 4090; n <= 4097; n++ {
				text := strings.Repeat("\n", w) + "flowchart TB\n" + strings.Repeat(" ", n) + "subgraph title\nA[" + strings.Repeat("x", 9000) + "]\n" + strings.Repeat(" ", n) + "end"
				got, err := l.Folding()(context.Background(), text)
				if err != nil || !reflect.DeepEqual(got, []FoldRange{{w + 1, w + 3}}) {
					t.Errorf("%d/%d: %v %v", w, n, got, err)
				}
			}
		}()
	}
	wg.Wait()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got, err := l.Folding()(ctx, "text"); err != context.Canceled || got != nil {
		t.Fatalf("cancel: %v %v", got, err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	if got, err := mermaidFolding(cancelCodeScanner{l.Tokenizer(), cancel})(ctx, "flowchart TB\n"+strings.Repeat("x ", 16000)); err != context.Canceled || got != nil {
		t.Fatalf("mid scan: %v %v", got, err)
	}
}

func TestBasicFoldingInventory(t *testing.T) {
	// Deliberate exclusions: no meaningful general block boundary in these
	// profiles. Applications may still provide explicit folds with SetFolds.
	excluded := map[ID]bool{Plain: true, JSONLines: true, GoSum: true, GitIgnore: true, GitAttributes: true, TSV: true, Crontab: true}
	with, without := 0, 0
	for _, info := range builtins {
		l, ok := Builtin(info.ID)
		if !ok {
			t.Fatalf("missing %s", info.ID)
		}
		if l.Folding() != nil {
			with++
			if excluded[info.ID] {
				t.Errorf("unexpected default fold: %s", info.ID)
			}
		} else {
			without++
			if !excluded[info.ID] {
				t.Errorf("missing default fold: %s", info.ID)
			}
		}
	}
	if with != 80 || without != 7 {
		t.Fatalf("inventory %d/%d; want 80/7", with, without)
	}
}
