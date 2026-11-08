package language

import (
	"context"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestBuiltinLayoutFolding(t *testing.T) {
	for _, id := range []ID{FSharp, Haskell} {
		t.Run(string(id), func(t *testing.T) {
			l, _ := Builtin(id)
			if l.Folding() == nil {
				t.Fatal("builtin has no layout folding provider")
			}
		})
	}
}

func TestLayoutMultilineStrings(t *testing.T) {
	for _, tt := range []struct {
		name  string
		id    ID
		lines []string
		want  []bool
	}{
		{"fsharp normal", FSharp, []string{"let text = \"first", "end let }", "last\"", "let real = 1"}, []bool{true, true, false, false}},
		{"fsharp verbatim", FSharp, []string{"let text = @\"first", "end let }", "last\"", "let real = 1"}, []bool{true, true, false, false}},
		{"haskell gap", Haskell, []string{"text = \"first\\", "", "  \\last\"", "real = 1"}, []bool{true, true, false, false}},
		{"haskell broken", Haskell, []string{"text = \"first", "real = 1"}, []bool{false, false}},
		{"fsharp interpolation close LF", FSharp, []string{"let text = $$\"\"\"first {{", "1", "}", "}", "}}", "last\"\"\""}, []bool{false, false, false, false, true, false}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			l, _ := Builtin(tt.id)
			scanner := l.Tokenizer()
			state := scanner.InitialState()
			for row, line := range tt.lines {
				r, err := scanner.Scan(Input{Text: line, Limit: len(line), LineEnd: true}, state)
				if err != nil {
					t.Fatal(err)
				}
				state = r.State
				if state.Context().String != tt.want[row] {
					t.Fatalf("row %d context = %+v", row, state.Context())
				}
			}
		})
	}
}

func TestLayoutQuotedNameTokens(t *testing.T) {
	l, _ := Builtin(FSharp)
	for _, name := range []string{"``if {match with}``", "``正文 with odd`name``"} {
		text := "let " + name + " = 1"
		r, err := l.Tokenizer().Scan(Input{Text: text, Limit: len(text), LineEnd: true}, l.Tokenizer().InitialState())
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, token := range r.Tokens {
			if token.Start == 4 && token.End == 4+len(name) && token.Kind == KindProperty && token.Class == 0 {
				found = true
			}
			if token.Start >= 4 && token.Start < 4+len(name) && (token.Kind == KindKeyword || token.Class&ClassDelimiter != 0) {
				t.Fatalf("quoted name became code: %v", r.Tokens)
			}
		}
		if !found {
			t.Fatalf("missing quoted name token: %v", r.Tokens)
		}
	}
}

func TestLayoutFoldingRanges(t *testing.T) {
	for _, tt := range []struct {
		name string
		id   ID
		text string
		want []FoldRange
	}{
		{"fsharp bindings", FSharp, "let run x =\n    let y =\n        x + 1\n    y\nlet next = 1", []FoldRange{{0, 3}, {1, 2}}},
		{"fsharp module type members", FSharp, "module Demo =\n    type Item() =\n        member x.Run() =\n            work()\n        member x.Value = 1\n    let other = 2", []FoldRange{{0, 5}, {1, 4}, {2, 3}}},
		{"fsharp multiline header", FSharp, "let run\n    (x: int)\n    (y: int) =\n    x + y\nlet next = 1", []FoldRange{{0, 3}}},
		{"fsharp match arms", FSharp, "let run x =\n    match x with\n    | Some value ->\n        work value\n    | None ->\n        other()\n    finish()\nlet next = 1", []FoldRange{{0, 6}, {1, 5}, {2, 3}, {4, 5}}},
		{"fsharp if branches", FSharp, "let run x =\n    if x then\n        first()\n    elif other then\n        second()\n    else\n        third()\n    finish()\nlet next = 1", []FoldRange{{0, 7}, {1, 6}, {3, 4}, {5, 6}}},
		{"fsharp try branches", FSharp, "let run() =\n    try\n        work()\n    with\n    | Error value ->\n        recover value\n    | _ ->\n        other()\n    finish()", []FoldRange{{0, 8}, {1, 7}, {3, 7}, {4, 5}, {6, 7}}},
		{"fsharp finally", FSharp, "try\n    work()\nfinally\n    cleanup()\nnext()", []FoldRange{{0, 3}, {2, 3}}},
		{"fsharp loops", FSharp, "let run xs =\n    for x in xs do\n        while ready do\n            work x\n    finish()", []FoldRange{{0, 4}, {1, 3}, {2, 3}}},
		{"fsharp recursive and", FSharp, "let rec first x =\n    second x\nand second x =\n    first x", []FoldRange{{0, 1}, {2, 3}}},
		{"fsharp explicit", FSharp, "begin\nlet x = 1;\nwork x\nend", []FoldRange{{0, 3}}},
		{"fsharp verbose loop", FSharp, "for x = 1 to 10 do\n work x\ndone", []FoldRange{{0, 2}}},
		{"fsharp let in", FSharp, "begin\nlet x =\n    work()\nin finish x\nend", []FoldRange{{0, 4}, {1, 2}}},
		{"fsharp groups", FSharp, "let values = [\n1\n2\n]\nlet next = 1", []FoldRange{{0, 3}}},
		{"fsharp normal literal", FSharp, "let text = \"first\nend let }\nlast\"\nlet next = 1", []FoldRange{{0, 2}}},
		{"fsharp verbatim literal", FSharp, "let text = @\"first\n\"\"end let }\nlast\"", []FoldRange{{0, 2}}},
		{"fsharp triple literal", FSharp, "let text = \"\"\"first\n end let }\nlast\"\"\"", []FoldRange{{0, 2}}},
		{"fsharp interpolation", FSharp, "let text = $\"\"\"first {\n    if ready then\n        \"needle\"\n    else\n        \"other\"\n} last\"\"\"", []FoldRange{{0, 5}, {1, 4}, {3, 4}}},
		{"fsharp multi dollar", FSharp, "let text = $$\"\"\"first {{\n    match x with\n    | A ->\n        1\n    | _ ->\n        2\n}} last\"\"\"", []FoldRange{{0, 6}, {1, 5}, {2, 3}, {4, 5}}},
		{"fsharp nested comment", FSharp, "(* outer\n (* inner\n end let }\n *)\n*)", []FoldRange{{0, 4}, {1, 3}}},
		{"fsharp comment strings", FSharp, "(* @\"first\n *) fake\nlast\" \"\"\"\n *) fake\n\"\"\"\n*)", []FoldRange{{0, 5}}},
		{"fsharp comments ignore indentation", FSharp, "let run() =\n    first()\n// comment\n\n    second()\nnext()", []FoldRange{{0, 4}}},
		{"fsharp Tab inside comment", FSharp, "let run() =\n    first()\n(* outer\n\t*) second()\nlet next = 1", []FoldRange{{0, 3}}},
		{"fsharp quoted names", FSharp, "let ``match with`` x =\n    work x\nlet ``{if}`` = 1", []FoldRange{{0, 1}}},
		{"fsharp fake header", FSharp, "value = 1\n    other\nModule.if\n    value", nil},
		{"fsharp no body", FSharp, "let run x =\n// only comment", nil},
		{"fsharp no equals", FSharp, "let run x\n    other", nil},
		{"fsharp ordinary indented use", FSharp, "let text = \"first\n\tlast\"", []FoldRange{{0, 1}}},
		{"fsharp verbose off loop", FSharp, "#light \"off\"\nfor x = 1 to 10 do\nwork x\ndone", []FoldRange{{1, 3}}},
		{"fsharp lambda", FSharp, "let run = fun x ->\n    work x\nlet next = 1", []FoldRange{{0, 1}}},
		{"fsharp function", FSharp, "let run = function\n    | A ->\n        first()\n    | _ ->\n        second()\nlet next = 1", []FoldRange{{0, 4}, {1, 2}, {3, 4}}},
		{"fsharp type cases", FSharp, "type Choice =\n    | First of int\n    | Second of string\nlet next = 1", []FoldRange{{0, 2}}},
		{"fsharp tabs", FSharp, "let run x =\n\tother", nil},
		{"fsharp light off", FSharp, "#light \"off\"\nlet f x =\n    work x\nbegin\nwork()\nend\n#light \"on\"\nlet g x =\n    work x", []FoldRange{{3, 5}, {7, 8}}},
		{"haskell do siblings", Haskell, "run = do\n  first\n  second\nnext = 1", []FoldRange{{0, 2}}},
		{"haskell nested layouts", Haskell, "run = do\n  case value of\n    A -> do\n      first\n      second\n    _ -> other\n  finish\nnext = 1", []FoldRange{{0, 6}, {1, 5}, {2, 4}}},
		{"haskell let in", Haskell, "run = let\n  x = 1\n  y = 2\n in x + y\nnext = 1", []FoldRange{{0, 2}}},
		{"haskell let same line", Haskell, "run = let x = 1\n          y = 2\n      in x + y", []FoldRange{{0, 1}}},
		{"haskell do let declaration", Haskell, "run = do\n  let x = 1\n      y = 2\n  print (x+y)\nnext = 1", []FoldRange{{0, 3}, {1, 2}}},
		{"haskell module where", Haskell, "module Demo where\nfirst = 1\nsecond = do\n  work\n  finish", []FoldRange{{0, 4}, {2, 4}}},
		{"haskell where", Haskell, "run x = helper x\n where\n  helper y = do\n    first y\n    second y\nnext = 1", []FoldRange{{1, 4}, {2, 4}}},
		{"haskell explicit", Haskell, "run = do {\nfirst;\nsecond\n}\nnext = 1", []FoldRange{{0, 3}}},
		{"haskell explicit layout nesting", Haskell, "run = do\n  work (do {\nfirst;\nsecond\n})\n  finish\nnext = 1", []FoldRange{{0, 5}, {1, 4}}},
		{"haskell parenthesis ends layout", Haskell, "run = (do\n  first\n  second\n ) + next", []FoldRange{{0, 2}}},
		{"haskell comma ends layout", Haskell, "values = [\n do\n  first\n , other\n , final\n ]", []FoldRange{{0, 5}, {1, 2}}},
		{"haskell comma before aligned elements", Haskell, "values = [\n do\n  first,\n  other,\n  final\n ]", []FoldRange{{0, 5}, {1, 2}}},
		{"haskell explicit inner let", Haskell, "run = let\n  x = let { y = 1 } in y\n  z = 2\n in x + z", []FoldRange{{0, 2}}},
		{"haskell implicit inner let", Haskell, "run = let\n  x = let\n    y = 1\n   in y\n  z = 2\n in x + z", []FoldRange{{0, 4}, {1, 2}}},
		{"haskell Unicode columns", Haskell, "run = let 中 = 1\n          y = 2\n      in 中 + y", []FoldRange{{0, 1}}},
		{"haskell same line gap", Haskell, "text = \"first\\  \\last\"\nnext = 1", nil},
		{"haskell fake header", Haskell, "value = \"do let where of\"\n    other\nMod.do\n    value", nil},
		{"fsharp mismatch", FSharp, "let values = [\n }\n]", nil},
		{"haskell mismatch", Haskell, "values = [\n }\n]", nil},
		{"fsharp unclosed literal", FSharp, "\"\"\"first\nend let }", nil},
		{"haskell unclosed gap", Haskell, "\"first\\\n   ", nil},
		{"haskell tabs", Haskell, "run = do\n\tfirst\n        second\nnext = 1", []FoldRange{{0, 2}}},
		{"haskell comments", Haskell, "run = do\n  first\n-- dedented\n\n  second\nnext = 1", []FoldRange{{0, 4}}},
		{"haskell nested comment", Haskell, "{- outer\n {- inner\n let do of }\n -}\n-}", []FoldRange{{0, 4}, {1, 3}}},
		{"haskell gap literal", Haskell, "text = \"first\\\n\n  \\last\"\nnext = 1", []FoldRange{{0, 2}}},
		{"haskell broken literal", Haskell, "\"broken\ndo\n  first\n  second", []FoldRange{{1, 3}}},
		{"haskell empty layout", Haskell, "run = do\nnext = 1", nil},
		{"haskell pending at EOF", Haskell, "run = do\n-- only comment", nil},
		{"haskell no general indent", Haskell, "run x =\n  value x", nil},
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

func TestLayoutFoldingWorkers(t *testing.T) {
	for _, id := range []ID{FSharp, Haskell} {
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
						open, close := "let text = \"\"\"", "\"\"\""
						if id == Haskell {
							open, close = "{-", "-}"
						}
						pad := strings.Repeat(" ", at)
						text := strings.Repeat("\n", worker) + pad + open + "\n" + strings.Repeat("x", 9000) + "\n" + pad + close
						got, err := l.Folding()(context.Background(), text)
						if err != nil || !reflect.DeepEqual(got, []FoldRange{{worker, worker + 2}}) {
							t.Errorf("worker %d offset %d: %v, %v", worker, at, got, err)
						}
					}
				}()
			}
			wg.Wait()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			got, err := structuralFolding(id, cancelCodeScanner{l.Tokenizer(), cancel})(ctx, strings.Repeat("x ", 16000))
			if err != context.Canceled || got != nil {
				t.Fatalf("mid-scan cancellation = %v, %v", got, err)
			}
		})
	}
}

func TestLayoutTokenizerObservation(t *testing.T) {
	for _, tt := range []struct {
		id   ID
		text string
	}{
		{Elixir, "def run(x) do\n ~r{正文 #{if x do \"value\" end}}ims\n ~S\"\"\"\nraw #{end}\n\"\"\"\nend"},
		{Erlang, "begin\n F = fun Fact(X) when X > 0 -> Fact(X-1) end,\n <<1, 2>>, '正文'\nend"},
		{Julia, "function run(A)\n #= outer #= inner =#\n =#\n x = \"正文 $(if true \"value\" end)\"\n r\"\"\"first\nlast\"\"\"ims\n A[begin+1:end-1]\nend"},
		{OCaml, "module M = struct\n (* \"*) 正文\n \" {tag|*)|tag} *)\n let text = {tag|\n |ta\ng}\n |tag}\nend"},
		{FSharp, "let ``match with`` x =\n work x\nlet text = $$\"\"\"正文 {{\n if ready then\n  1\n else\n  2\n}} last\"\"\"\n(* @\"*)\nlast\" \"\"\"*)\"\"\" *)"},
		{Haskell, "module Demo where\nrun = let 中 = 1\n          y = 2\n      in 中+y\ntext = \"first\\\n  \\last\"\n{- outer {- inner -} -}"},
		{Clojure, "#_ [\"正文\n\"]\n#\"regex\n\" (real)"},
		{EDN, "#_ #tag {:text \"正文\n\"}\n[:real]"},
	} {
		t.Run(string(tt.id), func(t *testing.T) {
			l, _ := Builtin(tt.id)
			public := l.Tokenizer()
			for chunk := 1; chunk <= 17; chunk++ {
				lex := &foldLexemes{row: -1, functional: &functionalFoldObserver{}}
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
