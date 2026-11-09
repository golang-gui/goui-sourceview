package language

import (
	"context"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestBuiltinDockerFolding(t *testing.T) {
	l, _ := Builtin(Dockerfile)
	if l.Folding() == nil {
		t.Fatal("builtin has no Dockerfile folding provider")
	}
}

func TestDockerFoldingWorkers(t *testing.T) {
	l, _ := Builtin(Dockerfile)
	if l.Folding() == nil {
		t.Fatal("missing folding provider")
	}
	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for at := 4090; at <= 4097; at++ {
				prefix := strings.Repeat("\n", worker) + strings.Repeat(" ", at)
				delimiter := strings.Repeat("X", 9000) + "终"
				for _, tt := range []struct {
					body string
					want []FoldRange
				}{
					{"FROM base\nRUN echo " + strings.Repeat("x", 9000) + " \\\n # ignored\n\n FROM payload\nFROM next\nRUN done", []FoldRange{{worker, worker + 4}, {worker + 1, worker + 4}, {worker + 5, worker + 6}}},
					{"COPY <<'EOF' /out\n" + strings.Repeat("x", 9000) + "\nEOF", []FoldRange{{worker, worker + 2}}},
					{"RUN <<" + delimiter + "\nFROM body\n" + delimiter, []FoldRange{{worker, worker + 2}}},
				} {
					got, err := l.Folding()(context.Background(), prefix+tt.body)
					if err != nil || !reflect.DeepEqual(got, tt.want) {
						t.Errorf("worker %d offset %d: %v, %v; want %v", worker, at, got, err, tt.want)
					}
				}
			}
		}()
	}
	wg.Wait()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got, err := l.Folding()(ctx, "FROM base\nRUN done"); err != context.Canceled || got != nil {
		t.Fatalf("cancelled = %v, %v", got, err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	if got, err := dockerFolding(cancelCodeScanner{l.Tokenizer(), cancel})(ctx, strings.Repeat("x ", 16000)); err != context.Canceled || got != nil {
		t.Fatalf("mid-scan cancellation = %v, %v", got, err)
	}
}

func TestDockerContinuedLines(t *testing.T) {
	for _, tt := range []struct {
		name, text string
		want       []string
	}{
		{"comments and blank", "RUN echo \\\n # comment\n\n FROM payload\nFROM real", []string{"RUN", "", "", "", "FROM"}},
		{"comment escape inert", "# comment \\\nFROM real\nRUN echo done", []string{"", "FROM", "RUN"}},
		{"backtick", "# escape=`\nRUN echo `\n # comment\n\n FROM payload\nFROM real", []string{"", "RUN", "", "", "", "FROM"}},
		{"directive in continuation", "RUN echo \\\n# escape=`\nFROM payload \\\n last\nFROM real", []string{"RUN", "", "", "", "FROM"}},
		{"variable ends continuation", "RUN echo \\\n$VALUE\nFROM real", []string{"RUN", "", "FROM"}},
		{"escaped argument ends continuation", "RUN echo \\\n\\\\\nFROM real", []string{"RUN", "", "FROM"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			l, _ := Builtin(Dockerfile)
			var got []string
			err := syntaxLines(context.Background(), l.Tokenizer(), tt.text, func(line syntaxLine) {
				a, z := leadingFoldKeyword(line)
				got = append(got, line.text[a:z])
			})
			if err != nil || !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("instruction keywords = %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}

func TestDockerFoldingRanges(t *testing.T) {
	for _, tt := range []struct {
		name, text string
		want       []FoldRange
	}{
		{"stages", "FROM base AS build\nRUN echo first\nCOPY . /src\n\nFROM scratch\nCOPY --from=build /out /out\n", []FoldRange{{0, 2}, {4, 5}}},
		{"prelude and case", "# syntax=docker/dockerfile:1\nARG BASE=alpine\n  fRoM ${BASE} AS build\nRUN echo first\n\tFrOm scratch\nCOPY --from=build /out /out", []FoldRange{{2, 3}, {4, 5}}},
		{"fake stages", "FROM base\n# FROM comment\nRUN echo FROM fake\nENV TEXT=\"FROM quoted\"\nONBUILD COPY --from=other /x /x\nFROM final\nRUN echo done", []FoldRange{{0, 4}, {5, 6}}},
		{"nested continuation", "FROM base\nRUN echo first \\\n && echo second \\\n && echo last\nFROM final\nRUN echo done", []FoldRange{{0, 3}, {1, 3}, {4, 5}}},
		{"ignored continuation lines", "FROM base\nRUN echo first \\\n# FROM comment\n\n FROM payload\nRUN echo done", []FoldRange{{0, 5}, {1, 4}}},
		{"comment between stages", "FROM base\nRUN echo first\n# FROM next description\n\nFROM next\nRUN echo second", []FoldRange{{0, 2}, {4, 5}}},
		{"standalone instruction", "RUN echo first \\\n && echo last", []FoldRange{{0, 1}}},
		{"prelude continuation", "ARG VALUE=first\\\nsecond\nFROM base\nRUN echo done", []FoldRange{{0, 1}, {2, 3}}},
		{"multiline from outer wins", "FROM --platform=$BUILDPLATFORM \\\n alpine AS build\nRUN echo done\nFROM final", []FoldRange{{0, 2}}},
		{"backtick instruction", "# escape=`\nFROM base\nRUN echo first `\n && echo last\nFROM next\nRUN echo done", []FoldRange{{1, 3}, {2, 3}, {4, 5}}},
		{"directive in continued body", "FROM base\nRUN echo first \\\n# escape=`\nFROM payload \\\nlast\nFROM final\nRUN echo done", []FoldRange{{0, 4}, {1, 4}, {5, 6}}},
		{"heredoc", "FROM base\nRUN <<EOF\nFROM fake\necho hello\nEOF\nFROM final\nRUN echo done", []FoldRange{{0, 4}, {1, 4}, {5, 6}}},
		{"copy heredoc", "COPY <<'EOF' /file\nFROM fake\nEOF\nFROM real\nRUN echo done", []FoldRange{{0, 2}, {3, 4}}},
		{"tab stripped heredoc", "RUN <<-EOF\n\tFROM fake\n\tEOF\nFROM real\nRUN echo done", []FoldRange{{0, 2}, {3, 4}}},
		{"multiple heredocs", "RUN <<A <<'B'\nFROM fake1\nA\nFROM fake2\nB\nFROM real\nRUN echo done", []FoldRange{{0, 4}, {5, 6}}},
		{"heredoc after continuation", "FROM base\nRUN echo start \\\n && cat <<EOF\nFROM fake\nEOF\nRUN echo done", []FoldRange{{0, 5}, {1, 4}}},
		{"quoted heredoc punctuation", "FROM base\nRUN echo '<<EOF'\nFROM real\nRUN echo done", []FoldRange{{0, 1}, {2, 3}}},
		{"unclosed heredoc", "RUN <<EOF\nFROM fake\necho hello", nil},
		{"unclosed instruction", "RUN echo first \\\n && echo more " + `\`, nil},
		{"unclosed inner stage text", "FROM base\nRUN <<EOF\nFROM fake", []FoldRange{{0, 2}}},
		{"only title", "FROM base", nil},
		{"empty stages", "FROM base\n\nFROM next\nFROM last", nil},
		{"missing stage argument", "FROM\nRUN echo done", nil},
		{"bad header within stage", "FROM base\nRUN echo first\nFROM\nRUN echo last", []FoldRange{{0, 3}}},
		{"no generic bracket fallback", "RUN [\n \"echo\", \"hello\"\n]", nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			l, _ := Builtin(Dockerfile)
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

func TestDockerScannerFragments(t *testing.T) {
	l, _ := Builtin(Dockerfile)
	text := "# escape=`\nFROM base\nRUN echo `\n # FROM ignored\n\n FROM payload\nCOPY <<'正文' /out\nFROM fake\n正文\nFROM final\nRUN echo done"
	var want []syntaxLine
	if err := syntaxLines(context.Background(), l.Tokenizer(), text, func(line syntaxLine) { want = append(want, line) }); err != nil {
		t.Fatal(err)
	}
	for chunk := 1; chunk <= 17; chunk++ {
		s := l.Tokenizer().InitialState()
		for row, line := range strings.Split(text, "\n") {
			var tokens []Token
			for offset := 0; ; {
				r, err := l.Tokenizer().Scan(Input{Text: line[offset:], Offset: offset, Limit: min(chunk, len(line)-offset), LineEnd: true}, s)
				if err != nil {
					t.Fatal(err)
				}
				s = r.State
				// Apply replacement exactly as the documented line-relative contract.
				if r.ReplaceFrom != nil {
					from := *r.ReplaceFrom
					for len(tokens) > 0 && tokens[len(tokens)-1].Start >= from {
						tokens = tokens[:len(tokens)-1]
					}
					if len(tokens) > 0 && tokens[len(tokens)-1].End > from {
						tokens[len(tokens)-1].End = from
					}
				}
				tokens = append(tokens, r.Tokens...)
				offset += r.Consumed
				if offset == len(line) {
					break
				}
				if r.Consumed <= 0 {
					t.Fatal("no progress")
				}
			}
			if !s.Equal(want[row].after) {
				t.Fatalf("chunk %d row %d changed state", chunk, row)
			}
			// Scanner fragments may split adjacent equal tokens; compare byte classes.
			for at := range len(line) {
				kind, class := KindPlain, Class(0)
				wk, wc := KindPlain, Class(0)
				for _, token := range tokens {
					if token.Start <= at && at < token.End {
						kind, class = token.Kind, token.Class
					}
				}
				for _, token := range want[row].tokens {
					if token.Start <= at && at < token.End {
						wk, wc = token.Kind, token.Class
					}
				}
				if kind != wk || class != wc {
					t.Fatalf("chunk %d row %d byte %d = %s/%d; want %s/%d", chunk, row, at, kind, class, wk, wc)
				}
			}
		}
	}
}
