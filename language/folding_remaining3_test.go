package language

import (
	"context"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestRemainingThirdFolding(t *testing.T) {
	for _, tt := range []struct {
		id         ID
		name, text string
		want       []FoldRange
	}{
		{BibTeX, "entry", "@article{key,\n title={one {nested} value},\n year=2026\n}", []FoldRange{{0, 3}}},
		{BibTeX, "parentheses", "@book(key,\n title=\"text )\",\n year=2026\n)", []FoldRange{{0, 3}}},
		{BibTeX, "multiline head", "@article\n{key,\n title={text}\n}", []FoldRange{{0, 3}}},
		{BibTeX, "incomplete", "@article{key,\n title={text}", nil},
		{BibTeX, "closing tail", "@book{key,\n title={text}\n} @misc{next, title={short}}", []FoldRange{{0, 1}}},
		{BibTeX, "string and preamble", "@string{\n name=\"one\" #\n \"two\"\n}\n@preamble{\n \"text\"\n}", []FoldRange{{0, 3}, {4, 6}}},
		{BibTeX, "escaped quote", "@article{key,\n title=\"a \\\" } fake\n b\"\n}", []FoldRange{{0, 3}}},
		{BibTeX, "escaped brace", "@article{key,\n title={a \\} }\n}", []FoldRange{{0, 2}}},
		{BibTeX, "opaque comment", "@comment{\n @article{fake,\n title={fake}\n}\n}\n@book{real,\n title={text}\n}", []FoldRange{{5, 7}}},
		{Gettext, "records", "# first\nmsgctxt \"context\"\nmsgid \"one\"\nmsgstr \"first\"\n\n#: source:1\nmsgid \"two\"\nmsgstr \"second\"\n", []FoldRange{{0, 3}, {5, 7}}},
		{Gettext, "no separator", "msgid \"one\"\nmsgstr \"first\"\n# second\nmsgid \"two\"\nmsgstr \"second\"", []FoldRange{{0, 1}, {2, 4}}},
		{Gettext, "plural and fake word", "msgid \"one\"\nmsgid_plural \"many\"\nmsgstr[0] \"msgid fake\"\nmsgstr[1] \"text\"", []FoldRange{{0, 3}}},
		{Gettext, "continued text", "msgid \"\"\n\"one\"\n\"two\"\nmsgstr \"text\"", []FoldRange{{0, 3}}},
		{Gettext, "comments only", "# a\n#: b\n# c", nil},
		{Gettext, "orphan translation", "msgstr \"a\"\n\"b\"", nil},
		{Dotenv, "double quote", "KEY=\"one\ntext\nlast\"\nOTHER=value", []FoldRange{{0, 2}}},
		{Dotenv, "single quote", "export KEY='one\ntext\nlast'", []FoldRange{{0, 2}}},
		{Dotenv, "backtick", "KEY=`one\ntext\nlast`", []FoldRange{{0, 2}}},
		{Dotenv, "closing comment", "KEY=\"one\ntext\nlast\" # tail", []FoldRange{{0, 1}}},
		{Dotenv, "unclosed", "KEY=\"one\ntext", nil},
		{Dotenv, "fake quotes", "# KEY=\"fake\nKEY=bare\nOTHER=value", nil},
		{Properties, "continuations", "key=one\\\n  two\\\n  last\nother=value", []FoldRange{{0, 2}}},
		{Properties, "continued key", "long\\\n key=value", []FoldRange{{0, 1}}},
		{Properties, "even slashes and comments", "key=one\\\\\nnext=value\n# fake\\\n! fake\\", nil},
		{Properties, "unfinished", "key=one\\\n  two\\", nil},
		{Properties, "blank end", "key=one\\\n\nother=value", []FoldRange{{0, 1}}},
		{CSV, "multiline field", "id,\"one\ntext\nlast\"", []FoldRange{{0, 2}}},
		{CSV, "closing columns", "id,\"one\ntext\nlast\",other", []FoldRange{{0, 1}}},
		{CSV, "empty last column", "id,\"one\ntext\nlast\",", []FoldRange{{0, 1}}},
		{CSV, "escaped quotes", "\"one\"\"\ntext\nlast\"", []FoldRange{{0, 2}}},
		{CSV, "unfinished", "\"one\ntext\nlast", nil},
		{CSV, "quote in bare field", "bad\"quote\ntext\nlast", nil},
	} {
		t.Run(string(tt.id)+"/"+tt.name, func(t *testing.T) {
			l, _ := Builtin(tt.id)
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

func TestRemainingThirdWorkers(t *testing.T) {
	for _, id := range []ID{BibTeX, Gettext, Dotenv, Properties, CSV} {
		t.Run(string(id), func(t *testing.T) {
			l, _ := Builtin(id)
			var wg sync.WaitGroup
			for w := 0; w < 8; w++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for n := 4090; n <= 4097; n++ {
						pad, body := strings.Repeat(" ", n), strings.Repeat("x", 9000)
						text := strings.Repeat("\n", w)
						switch id {
						case BibTeX:
							text += "@article" + pad + "{key,\n title={" + body + "}\n}"
						case Gettext:
							text += pad + "msgid \"one\"\n\"" + body + "\"\nmsgstr \"text\""
						case Dotenv:
							text += "KEY=" + pad + "\"one\n" + body + "\nlast" + pad + "\""
						case Properties:
							text += "key=" + pad + "one\\\n " + body + "\\\n last"
						case CSV:
							text += "\"" + pad + "one\n" + body + "\nlast" + pad + "\""
						}
						got, err := l.Folding()(context.Background(), text)
						if err != nil || !reflect.DeepEqual(got, []FoldRange{{w, w + 2}}) {
							t.Errorf("%s %d/%d: %v %v", id, w, n, got, err)
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
			scan := cancelCodeScanner{l.Tokenizer(), cancel}
			fold := recordLiteralFolding(id, scan)
			if id == Gettext {
				fold = poFolding(scan)
			}
			if got, err := fold(ctx, strings.Repeat("x ", 16000)); err != context.Canceled || got != nil {
				t.Fatalf("mid scan: %v %v", got, err)
			}
		})
	}
}

func TestRemainingRecordObservation(t *testing.T) {
	for _, sample := range []struct {
		id   ID
		text string
	}{
		{BibTeX, "@article\n{key,\n title={文\\} 字}\n}\n@comment{\n@book{fake}\n}"},
		{Dotenv, "KEY=\"一\\n\n二\" # tail\nOTHER='text'"},
		{Properties, "key=文\\\n 字\\\n end\n# comment"},
		{CSV, "\"一\"\"\n二\",next\nrow,value"},
	} {
		l, _ := Builtin(sample.id)
		for chunk := 1; chunk <= 17; chunk++ {
			obs := foldingTokenizer{l.Tokenizer(), &foldLexemes{row: -1}}
			a, b := l.Tokenizer().InitialState(), l.Tokenizer().InitialState()
			for _, line := range strings.Split(sample.text, "\n") {
				for at := 0; ; {
					in := Input{Text: line[at:], Offset: at, Limit: min(chunk, len(line)-at), LineEnd: true}
					x, err := l.Tokenizer().Scan(in, a)
					if err != nil {
						t.Fatal(err)
					}
					y, err := obs.Scan(in, b)
					if err != nil {
						t.Fatal(err)
					}
					a, b = x.State, y.State
					if !a.Equal(b) {
						t.Fatal("state changed")
					}
					y.State = x.State
					if !reflect.DeepEqual(x, y) {
						t.Fatalf("%s chunk %d result changed", sample.id, chunk)
					}
					at += x.Consumed
					if at == len(line) {
						break
					}
					if x.Consumed <= 0 {
						t.Fatal("no progress")
					}
				}
			}
		}
	}
}
