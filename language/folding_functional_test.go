package language

import (
	"context"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestBuiltinFunctionalFolding(t *testing.T) {
	for _, id := range []ID{Elixir, Erlang, Julia, OCaml} {
		t.Run(string(id), func(t *testing.T) {
			l, _ := Builtin(id)
			if l.Folding() == nil {
				t.Fatal("builtin has no functional folding provider")
			}
		})
	}
}

func TestFunctionalFoldingRanges(t *testing.T) {
	for _, tt := range []struct {
		name string
		id   ID
		text string
		want []FoldRange
	}{
		{"elixir blocks", Elixir, "defmodule Demo do\n def run(x) do\n  if x do\n   :ok\n  else\n   :other\n  end\n end\nend", []FoldRange{{0, 8}, {1, 7}, {2, 6}}},
		{"elixir multiline head", Elixir, "def run(\n x\n) do\n x\nend", []FoldRange{{0, 4}}},
		{"elixir short do", Elixir, "defmodule Demo do\n def run(x), do: x\n def other(), do: :ok\nend", []FoldRange{{0, 3}}},
		{"elixir fn and branches", Elixir, "try do\n values |> Enum.map(fn x ->\n  x + 1\n end)\nrescue\n _ -> :error\nafter\n cleanup()\nend", []FoldRange{{0, 8}, {1, 3}}},
		{"elixir atoms and members", Elixir, "defmodule Demo do\n x = [:do, :end, do: 1, fn: 2]\n Module.end()\nend", []FoldRange{{0, 3}}},
		{"elixir heredoc interpolation", Elixir, "text = \"\"\"\nstart #{\n if ready do\n  \"inner\nlast\"\n end\n} finish\n\"\"\"", []FoldRange{{0, 7}, {1, 5}, {2, 5}, {3, 4}}},
		{"elixir sigil modifiers", Elixir, "text = ~r{first\n {nested} /\nlast}ims", []FoldRange{{0, 2}}},
		{"elixir raw sigil", Elixir, "text = ~S\"\"\"\n#{if ready do end} } [\n\"\"\"", []FoldRange{{0, 2}}},
		{"elixir interpolation scope", Elixir, "defmodule Demo do\n text = \"#{end}\"\nend", []FoldRange{{0, 2}}},
		{"erlang cases", Erlang, "case Value of\n ok -> begin\n  work()\n end;\n _ -> other()\nend", []FoldRange{{0, 5}, {1, 3}}},
		{"erlang clauses", Erlang, "receive\n {ok, X} ->\n  case X of\n   1 -> one;\n   _ -> other\n  end\nafter\n 100 -> timeout\nend", []FoldRange{{0, 8}, {2, 5}}},
		{"erlang fun", Erlang, "Fun = fun Fact(X) when X > 0 ->\n Fact(X - 1);\n Fact(_) -> done\nend", []FoldRange{{0, 3}}},
		{"erlang references", Erlang, "case X of\n 1 -> fun local/1;\n 2 -> fun module:remote/2;\n _ -> fun M:F/A\nend", []FoldRange{{0, 4}}},
		{"erlang binaries", Erlang, "Value = <<\n <<\n  1, 2\n >>,\n 3\n>>", []FoldRange{{0, 5}, {1, 3}}},
		{"erlang adjacent binaries", Erlang, "Value = <<<<\n 1\n>>>>", []FoldRange{{0, 2}}},
		{"elixir bit shifts", Elixir, "def run(x) do\n x <<< 2\n x >>> 1\nend", []FoldRange{{0, 3}}},
		{"erlang quoted atoms", Erlang, "begin\n X = 'end begin';\n Y = \"case of\nend\"\nend", []FoldRange{{0, 4}, {2, 3}}},
		{"erlang missing of", Erlang, "case X\n work()\nend", nil},
		{"erlang maybe", Erlang, "maybe\n {ok, X} ?= read(),\n X\nelse\n _ -> error\nend", []FoldRange{{0, 5}}},
		{"julia declarations", Julia, "module Demo\n mutable struct Item\n  value::Int\n end\n function run(x)\n  if x\n   work()\n  else\n   other()\n  end\n end\nend", []FoldRange{{0, 11}, {1, 3}, {4, 10}, {5, 9}}},
		{"julia types", Julia, "abstract type Base\nend\nprimitive type Bits 8\nend", []FoldRange{{0, 1}, {2, 3}}},
		{"julia loops", Julia, "function run(xs)\n for x in xs\n  while ready\n   work(x)\n  end\n end\nend", []FoldRange{{0, 6}, {1, 5}, {2, 4}}},
		{"julia comprehensions", Julia, "function run(xs)\n a = [x for x in xs if x > 0]\n b = (f(x) for x in xs)\n c = A[begin+1:end-1]\n return a\nend", []FoldRange{{0, 5}}},
		{"julia conditional comprehension", Julia, "a = [if x > 0\n x\nelse\n 0\nend for x in xs]", []FoldRange{{0, 4}}},
		{"julia block in index", Julia, "function run(A)\n return A[begin\n  work()\n  1\n end]\nend", []FoldRange{{0, 5}, {1, 4}}},
		{"julia loop in begin", Julia, "values = [begin\n for x in xs\n  work(x)\n end\n result\nend]", []FoldRange{{0, 5}, {1, 3}}},
		{"julia do block", Julia, "map(values) do x\n x + 1\nend", []FoldRange{{0, 2}}},
		{"julia short function", Julia, "function run(x)\n f(y) = y + 1\n :end\n obj.end\n return A[end]\nend", []FoldRange{{0, 5}}},
		{"julia literal interpolation", Julia, "text = \"\"\"start $(\n if ready\n  \"inner\nlast\"\n else\n  \"other\"\n end\n) finish\"\"\"", []FoldRange{{0, 7}, {1, 6}, {2, 3}}},
		{"julia regex modifiers", Julia, "text = r\"\"\"first\n end if } [\nlast\"\"\"ims", []FoldRange{{0, 2}}},
		{"julia nested comment", Julia, "#= outer\n #= inner\n end function\n =#\n=#", []FoldRange{{0, 4}, {1, 3}}},
		{"julia interpolation scope", Julia, "function run()\n text = \"$(end)\"\nend", []FoldRange{{0, 2}}},
		{"ocaml blocks", OCaml, "module Demo = struct\n let run x = begin\n  while ready do\n   work x\n  done\n end\nend", []FoldRange{{0, 6}, {1, 5}, {2, 4}}},
		{"ocaml signature object", OCaml, "module type S = sig\n val run : int -> int\nend\nclass demo = object\n method run = 1\nend", []FoldRange{{0, 2}, {3, 5}}},
		{"ocaml multiline loop", OCaml, "for i = 0\n to 10\ndo\n work i\ndone", []FoldRange{{0, 4}}},
		{"ocaml loop mismatch", OCaml, "while ready do\n work()\nend", nil},
		{"ocaml raw literal", OCaml, "let text = {tag|\n end done } |wrong}\n|tag}", []FoldRange{{0, 2}}},
		{"ocaml nested comments", OCaml, "(* outer\n (* inner\n done end\n *)\n*)", []FoldRange{{0, 4}, {1, 3}}},
		{"ocaml comment literals", OCaml, "(* outer \"*)\n text\" {tag|\n *) fake\n|tag} tail\n*)", []FoldRange{{0, 4}}},
		{"ocaml raw close LF", OCaml, "let text = {tag|\nbody |ta\ng}\nend done\n|tag}", []FoldRange{{0, 4}}},
		{"ocaml ordinary let", OCaml, "let run x =\n match x with\n | 1 -> one\n | _ -> other", nil},
		{"elixir incomplete", Elixir, "def run(x) do\n x", nil},
		{"erlang incomplete", Erlang, "fun(X) ->\n X", nil},
		{"julia incomplete", Julia, "function run()\n value", nil},
		{"ocaml incomplete", OCaml, "module M = struct\n let x = 1", nil},
		{"elixir closing tail", Elixir, "if ready do\n :ok\nend |> next()", []FoldRange{{0, 1}}},
		{"erlang closing tail", Erlang, "begin\n work()\nend + 1", []FoldRange{{0, 1}}},
		{"julia closing tail", Julia, "begin\n work()\nend + 1", []FoldRange{{0, 1}}},
		{"ocaml closing tail", OCaml, "begin\n work()\nend + 1", []FoldRange{{0, 1}}},
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

func TestFunctionalTokenizerObservation(t *testing.T) {
	for _, tt := range []struct {
		id   ID
		text string
	}{
		{Elixir, "def run(x) do\n ~r{正文 #{if x do \"value\" end}}ims\n ~S\"\"\"\nraw #{end}\n\"\"\"\nend"},
		{Erlang, "begin\n F = fun Fact(X) when X > 0 -> Fact(X-1) end,\n <<1, 2>>, '正文'\nend"},
		{Julia, "function run(A)\n #= outer #= inner =#\n =#\n x = \"正文 $(if true \"value\" end)\"\n r\"\"\"first\nlast\"\"\"ims\n A[begin+1:end-1]\nend"},
		{OCaml, "module M = struct\n (* \"*) 正文\n \" {tag|*)|tag} *)\n let text = {tag|\n |ta\ng}\n |tag}\nend"},
		{FSharp, "let text = $\"正文 {1}\"\n(* outer (* inner *) *)"},
		{Haskell, "let x = \"正文\"\n{- outer {- inner -} -}"},
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

func TestFunctionalMultilineLexicalBoundaries(t *testing.T) {
	for _, tt := range []struct {
		id                 ID
		lines              []string
		comments, literals []bool
	}{
		{Julia, []string{"#= outer", "#= inner", "=#", "end function", "=#", "function run()"}, []bool{true, true, true, true, false, false}, nil},
		{OCaml, []string{"{tag|", "body |ta", "g}", "end done", "|tag}", "begin"}, nil, []bool{true, true, true, true, false, false}},
		{OCaml, []string{"(* {tag|", "|ta", "g} *)", "|tag}", "*)", "begin"}, []bool{true, true, true, true, false, false}, nil},
	} {
		t.Run(string(tt.id), func(t *testing.T) {
			l, _ := Builtin(tt.id)
			scanner := l.Tokenizer()
			state := scanner.InitialState()
			for row, line := range tt.lines {
				before := state
				r, err := scanner.Scan(Input{Text: line, Limit: len(line), LineEnd: true}, state)
				if err != nil {
					t.Fatal(err)
				}
				// Saved states must remain reusable after the following line scan.
				again, err := scanner.Scan(Input{Text: line, Limit: len(line), LineEnd: true}, before)
				if err != nil || !r.State.Equal(again.State) {
					t.Fatal("saved state mutated")
				}
				state = r.State
				c := state.Context()
				if tt.comments != nil && c.Comment != tt.comments[row] || tt.literals != nil && c.String != tt.literals[row] {
					t.Fatalf("%s row %d context = %+v", tt.id, row, c)
				}
			}
		})
	}
}

func TestFunctionalFoldingRawLabel(t *testing.T) {
	label := strings.Repeat("a", 5000)
	l, _ := Builtin(OCaml)
	got, err := l.Folding()(context.Background(), "let text = {"+label+"|\n end done\n|"+label+"}")
	if err != nil || !reflect.DeepEqual(got, []FoldRange{{0, 2}}) {
		t.Fatalf("long raw label = %v, %v", got, err)
	}
}

func TestFunctionalFoldingWorkers(t *testing.T) {
	for _, tt := range []struct {
		id          ID
		open, close string
	}{
		{Elixir, "text = ~S\"\"\"", "\"\"\""}, {Erlang, "begin", "end"}, {Julia, "#=", "=#"}, {OCaml, "let text = {tag|", "|tag}"},
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

func TestFunctionalFoldingModifierFragments(t *testing.T) {
	for _, id := range []ID{Elixir, Julia} {
		l, _ := Builtin(id)
		open := "text = ~r\"\"\""
		if id == Julia {
			open = "text = r\"\"\""
		}
		for at := 4090; at <= 4097; at++ {
			text := open + "\n end } [\n" + strings.Repeat(" ", at) + "\"\"\"ims"
			got, err := l.Folding()(context.Background(), text)
			if err != nil || !reflect.DeepEqual(got, []FoldRange{{0, 2}}) {
				t.Fatalf("%s modifier offset %d = %v, %v", id, at, got, err)
			}
		}
	}
}
