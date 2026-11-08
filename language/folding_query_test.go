package language

import (
	"context"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestBuiltinQueryReaderFolding(t *testing.T) {
	for _, id := range []ID{Nix, JQ, PromQL, Clojure, EDN} {
		t.Run(string(id), func(t *testing.T) {
			l, _ := Builtin(id)
			if l.Folding() == nil {
				t.Fatal("builtin has no query/reader folding provider")
			}
		})
	}
}

func TestQueryReaderFoldingRanges(t *testing.T) {
	for _, tt := range []struct {
		name string
		id   ID
		text string
		want []FoldRange
	}{
		{"nix collections", Nix, "{\n values = [\n  { text = \"}\"; }\n ];\n}", []FoldRange{{0, 4}, {1, 3}}},
		{"nix bindings", Nix, "let\n x = 1;\n y = 2;\nin x + y", []FoldRange{{0, 2}}},
		{"nix nested bindings", Nix, "let\n x = let\n  y = 1;\n in y;\nin x", []FoldRange{{0, 3}, {1, 2}}},
		{"nix legacy let", Nix, "let\n x = let {\n  body = 1;\n };\nin x", []FoldRange{{0, 3}, {1, 3}}},
		{"nix member names", Nix, "{\n x = attrs.let;\n y = attrs.in;\n}", []FoldRange{{0, 3}}},
		{"nix literal", Nix, "x = ''first\n } /* ''$ fake\nlast'';", []FoldRange{{0, 2}}},
		{"nix adjacent literals", Nix, "x = \"first\n\"; y = \"second\n\";", []FoldRange{{1, 2}}},
		{"nix interpolation", Nix, "x = \"outer ${\n let\n  text = \"inner\n last\";\n in text\n} end\";", []FoldRange{{0, 5}, {1, 3}, {2, 3}}},
		{"nix path", Nix, "{\n path = ./foo/bar;\n search = <nixpkgs>;\n}", []FoldRange{{0, 3}}},
		{"nix comments", Nix, "/* first\n*/ x; /* second\n*/", []FoldRange{{1, 2}}},
		{"nix escaped interpolation", Nix, "x = ''first\n ''${ let in }\nlast'';", []FoldRange{{0, 2}}},
		{"nix interpolation scope", Nix, "let\n x = \"${in}\";\n y = 1;\nin x", []FoldRange{{0, 2}}},
		{"jq conditions", JQ, "if .ready then\n if .value then\n  1\n else\n  2\n end\nelif .other then\n 3\nelse\n 4\nend", []FoldRange{{0, 10}, {1, 5}}},
		{"jq functions", JQ, "def outer(f; g):\n def inner(x):\n  x + 1;\n f | g | inner(.);\nouter(.; .)", []FoldRange{{0, 3}, {1, 2}}},
		{"jq argument semicolons", JQ, "def demo:\n reduce .[] as $x (\n  0; . + $x\n );\ndemo", []FoldRange{{0, 3}, {1, 3}}},
		{"jq function condition", JQ, "def demo:\n if .ready then\n  1\n else\n  0\n end;", []FoldRange{{0, 5}, {1, 5}}},
		{"jq member keywords", JQ, "{\n value: .if,\n other: .end,\n def: 1\n}", []FoldRange{{0, 4}}},
		{"jq missing then", JQ, "if .ready\n .value\nend", nil},
		{"jq strings", JQ, "\"outer \\(\n if .ready then\n  \"inner\nlast\"\n else\n  \"other\"\n end\n) end\"", []FoldRange{{0, 7}, {1, 6}, {2, 3}}},
		{"jq interpolation scope", JQ, "if .ready then\n \"\\(end)\"\nelse\n 0\nend", []FoldRange{{0, 4}}},
		{"jq adjacent strings", JQ, "\"first\n\" + \"second\n\"", []FoldRange{{1, 2}}},
		{"jq literal fake code", JQ, "\"if then end {\n} def x;\"", []FoldRange{{0, 1}}},
		{"jq trailing code", JQ, "if .ready then\n 1\nend | .other", []FoldRange{{0, 1}}},
		{"prom aggregation", PromQL, "sum by (job) (\n rate(\n  requests_total{\n   status=~\"5..\"\n  }[5m]\n )\n)", []FoldRange{{0, 6}, {1, 5}, {2, 3}}},
		{"prom raw string", PromQL, "label_replace(up, \"x\", `first\n } # \\\nlast`, \"job\", \".*\")", []FoldRange{{0, 2}}},
		{"prom standalone raw", PromQL, "`first\n } # \\\nlast`", []FoldRange{{0, 2}}},
		{"prom invalid quote", PromQL, "\"broken\nsum(\n up\n)", []FoldRange{{1, 3}}},
		{"clojure collections", Clojure, "(defn demo [x]\n (let [values [\n  x 1\n ]]\n  {:values values}))", []FoldRange{{0, 4}, {1, 3}}},
		{"clojure separate closing lines", Clojure, "(defn demo [x]\n (let [values [\n  x 1\n ]]\n  {:values values}\n )\n)", []FoldRange{{0, 6}, {1, 5}}},
		{"clojure regex", Clojure, "(def pattern #\"first\n } [ ;\nlast\")", []FoldRange{{0, 2}}},
		{"clojure characters", Clojure, "(vector\n \\) \\] \\{\n)", []FoldRange{{0, 2}}},
		{"clojure conditional", Clojure, "#?(:clj (\n first\n)\n:cljs [\n second\n])", []FoldRange{{0, 5}, {3, 4}}},
		{"clojure conditional separate close", Clojure, "#?(:clj (\n first\n)\n:cljs [\n second\n]\n)", []FoldRange{{0, 6}, {3, 5}}},
		{"clojure discard", Clojure, "#_ (defn ignored []\n {broken [\n 1]})\n(defn real []\n 1)", []FoldRange{{0, 2}, {3, 4}}},
		{"clojure discard tail", Clojure, "#_ [\n 1\n] (real)", []FoldRange{{0, 1}}},
		{"clojure nested discard", Clojure, "#_ #_ [\n ignored\n]\n[\n discarded\n]", []FoldRange{{0, 5}}},
		{"clojure discard metadata", Clojure, "#_ ^{:tag X}\n [\n value\n]", []FoldRange{{0, 3}}},
		{"clojure discarded literal", Clojure, "#_ \"first\n } [\nlast\"\n[\n real\n]", []FoldRange{{0, 2}, {3, 5}}},
		{"clojure adjacent strings", Clojure, "\"first\n\" \"second\n\"", []FoldRange{{1, 2}}},
		{"edn data", EDN, "{:items [\n {:value #{\n  1 2\n }}\n]}", []FoldRange{{0, 4}, {1, 3}}},
		{"edn tagged discard", EDN, "#_ #custom/tag\n {\n :ignored 1\n}\n[\n :real\n]", []FoldRange{{0, 3}, {4, 6}}},
		{"edn literal", EDN, "{:text \"first\n } [ ;\nlast\"}", []FoldRange{{0, 2}}},
		{"edn unterminated discard", EDN, "#_ [\n \"text\"", nil},
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

func TestQueryReaderFoldingRecovery(t *testing.T) {
	for _, id := range []ID{Nix, JQ, PromQL, Clojure, EDN} {
		t.Run(string(id), func(t *testing.T) {
			l, _ := Builtin(id)
			for _, text := range []string{"[\n value", "[\n }\n]"} {
				got, err := l.Folding()(context.Background(), text)
				if err != nil || len(got) != 0 {
					t.Fatalf("incomplete %q: %v, %v", text, got, err)
				}
			}
			got, err := l.Folding()(context.Background(), "[\n value\n] tail")
			if err != nil || !reflect.DeepEqual(got, []FoldRange{{0, 1}}) {
				t.Fatalf("trailing code = %v, %v", got, err)
			}
		})
	}
	for _, tt := range []struct {
		id   ID
		text string
		want []FoldRange
	}{
		{Nix, "let\n x = 1;", nil},
		{JQ, "def demo:\n .value", nil},
		{JQ, "def :\n .value;", nil},
		{JQ, "def demo;\n .value", nil},
		{JQ, "if .x then\n 1\nelif .y\n 2\nend", nil},
		{JQ, "def outer:\n \"\\(def inner:\n .;\ninner)\"\n | .;\nouter", []FoldRange{{0, 4}, {1, 3}}},
		{Clojure, "#_ #tag\n \"first\nlast\"\n(real)", []FoldRange{{0, 2}}},
		{EDN, "#_\n [\n :ignored\n]\n[:real]", []FoldRange{{0, 3}}},
	} {
		l, _ := Builtin(tt.id)
		got, err := l.Folding()(context.Background(), tt.text)
		if err != nil || !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s %q = %v, %v; want %v", tt.id, tt.text, got, err, tt.want)
		}
	}
}

func TestQueryReaderTokenizerObservation(t *testing.T) {
	for _, tt := range []struct {
		id   ID
		text string
	}{
		{Nix, "let\n x = ''正文 ${let y=\"inner\"; in y}'';\n y = ./path/to/file;\nin x\n/* comment */"},
		{JQ, "def demo(x; y):\n \"正文 \\(if .ready then x else y end)\";\ndemo(.; .)"},
		{PromQL, "sum(rate(up{job=~\"prod\"}[5m]))\n`正文\n } # \\\nlast`"},
		{Clojure, "#_ ^{:tag X}\n [#tag \"正文\n\" #_ [:ignore]]\n#?(:clj [1] :cljs #{2})\n#\"regex\n }\""},
		{EDN, "#_ #custom/tag\n {:value \"正文\n\"}\n[:real \\] #inst \"2026-11-08\"]"},
		{FSharp, "let text = $\"value {1}\"\n(* comment *)"},
		{Elixir, "value = \"#{inspect(1)}\"\n~S\"\"\"\ntext\n\"\"\""},
		{Julia, "value = \"$(call(1))\"\n#= nested #= comment =# =#"},
	} {
		t.Run(string(tt.id), func(t *testing.T) {
			l, _ := Builtin(tt.id)
			public := l.Tokenizer()
			for chunk := 1; chunk <= 17; chunk++ {
				lex := &foldLexemes{row: -1, query: &queryFoldObserver{}}
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

func TestQueryReaderFoldingWorkers(t *testing.T) {
	for _, tt := range []struct {
		id          ID
		open, close string
	}{
		{Nix, "x = ''", "'';"}, {JQ, "def demo:", ";"}, {PromQL, "`", "`"}, {Clojure, "#_ [", "]"}, {EDN, "[", "]"},
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
						text := strings.Repeat("\n", i) + pad + tt.open + "\n" + strings.Repeat("x", 9000) + "\n" + pad + tt.close
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
