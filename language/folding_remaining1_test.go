package language

import (
	"context"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestRemainingFirstFolding(t *testing.T) {
	for _, tt := range []struct {
		id         ID
		name, text string
		want       []FoldRange
	}{
		{Makefile, "rules", "all: dep\n\techo first\n\t# note\n\techo last\n\nnext:\n\techo next", []FoldRange{{0, 3}, {5, 6}}},
		{Makefile, "blocks", "ifdef ENABLED\ndefine TEXT\nifeq fake\nendef\nelse\nX=1\nendif", []FoldRange{{0, 6}, {1, 3}}},
		{Makefile, "nested definitions", "override define OUTER\ndefine INNER\nvalue\nendef\nendef", []FoldRange{{0, 4}, {1, 3}}},
		{Makefile, "assignment names", "ifdef = value\ndefine := value\nendif = data", nil},
		{Makefile, "recipe fake directives", "all:\n\tifdef literal\n\tendef\n\tendif", []FoldRange{{0, 3}}},
		{Makefile, "custom prefix", ".RECIPEPREFIX=>\nall:\n>echo first\n>echo last\nnext:\n>echo next", []FoldRange{{1, 3}, {4, 5}}},
		{Makefile, "continuation", "VALUE=one\\\ntwo\nall:\n\techo one \\\n two", []FoldRange{{0, 1}, {2, 4}, {3, 4}}},
		{Makefile, "computed target omitted", "$(TARGET): dep\n\techo one\n\techo two", nil},
		{Makefile, "target not assignment", "URL=http://example.com\n\techo one", nil},
		{Makefile, "unclosed", "ifdef X\ndefine NAME\ntext", nil},
		{Batch, "labels", ":first\necho one\n:: comment\n\n:second\necho two", []FoldRange{{0, 2}, {4, 5}}},
		{Batch, "fake labels", "echo :first\nREM :fake\n::fake\necho one ^\n:continued\n:real\necho two", []FoldRange{{5, 6}}},
		{Batch, "region", ":: #region one\necho one\n@REM #region two\necho two\nREM #endregion\n:: #endregion", []FoldRange{{0, 5}, {2, 4}}},
		{Batch, "empty label", ":\necho one\n:first\n:second\necho two", []FoldRange{{3, 4}}},
		{GoTemplate, "nested", "{{if .Ready}}\n{{range .Items}}\n{{.}}\n{{end}}\n{{else}}\nnone\n{{end}}", []FoldRange{{0, 6}, {1, 3}}},
		{GoTemplate, "fake heads", "{{.if}}\ntext\n{{end}}\n{{printf \"if\"}}\ntext\n{{end}}", nil},
		{GoTemplate, "comments", "{{/* first\n{{if .Fake}}\n*/}}", []FoldRange{{0, 2}}},
		{GoTemplate, "closing tail", "{{if .Ready}}\none\n{{end}} trailing", []FoldRange{{0, 1}}},
		{GoTemplate, "trim", "{{- if .Ready -}}\none\n{{- end -}}", []FoldRange{{0, 2}}},
		{GoTemplate, "missing condition", "{{if /* note */}}\none\n{{end}}", nil},
		{GoTemplate, "unclosed", "{{if .Ready}}\none", nil},
		{Jinja2, "blocks", "{% if ready %}\n{% for x in items %}\n{{x}}\n{% endfor %}\n{% else %}\nnone\n{% endif %}", []FoldRange{{0, 6}, {1, 3}}},
		{Jinja2, "set block", "{% set text %}\nvalue\n{% endset %}\n{% set text = 'value' %}", []FoldRange{{0, 2}}},
		{Jinja2, "raw", "{% raw %}\n{% if fake %}\n{{data}}\n{% endraw %}", []FoldRange{{0, 3}}},
		{Jinja2, "raw quoted fake close", "{% raw %}\n{% \"endraw\" %}\n{% if fake %}\n{% endraw %}", []FoldRange{{0, 3}}},
		{Jinja2, "raw trimmed close", "{%- raw -%}\n{% if fake %}\n{%- endraw -%}", []FoldRange{{0, 2}}},
		{Jinja2, "comments", "{# first\n{% if fake %}\n#}", []FoldRange{{0, 2}}},
		{Jinja2, "expression not statement", "{{if ready}}\none\n{% endif %}", nil},
		{Jinja2, "mismatch", "{% if ready %}\none\n{% endfor %}\n{% endif %}", nil},
		{Jinja2, "trim and multiline", "{%- if (\n ready\n) -%}\none\n{%- endif -%}", []FoldRange{{0, 4}}},
	} {
		t.Run(string(tt.id)+"/"+tt.name, func(t *testing.T) {
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

func TestRemainingTemplateHosts(t *testing.T) {
	for _, id := range []ID{GoTemplate, Jinja2} {
		for _, hostID := range []ID{Plain, HTML, XML, YAML, TOML} {
			host, _ := Builtin(hostID)
			var l *Language
			var err error
			if id == GoTemplate {
				l, err = NewGoTemplate(host)
			} else {
				l, err = NewJinja2(host)
			}
			if err != nil {
				t.Fatal(err)
			}
			text := "{{if .X}}\n<body>\n</body>\n{{end}}"
			if id == Jinja2 {
				text = "{% if x %}\n<body>\n</body>\n{% endif %}"
			}
			if l.Folding() == nil {
				t.Fatal("constructor missing folding")
			}
			got, err := l.Folding()(context.Background(), text)
			if err != nil || !reflect.DeepEqual(got, []FoldRange{{0, 3}}) {
				t.Fatalf("%s host %s: %v, %v", id, hostID, got, err)
			}
		}
	}
}

func TestRemainingFirstWorkers(t *testing.T) {
	for _, tt := range []struct {
		id                ID
		open, body, close string
	}{
		{Makefile, "ifdef X", "VALUE=正文", "endif"}, {Batch, ":first", "echo 正文", ":last"}, {GoTemplate, "{{if .X}}", "正文", "{{end}}"}, {Jinja2, "{% if x %}", "正文", "{% endif %}"},
	} {
		t.Run(string(tt.id), func(t *testing.T) {
			l, _ := Builtin(tt.id)
			var wg sync.WaitGroup
			for w := 0; w < 8; w++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for at := 4090; at <= 4097; at++ {
						text := strings.Repeat("\n", w) + strings.Repeat(" ", at) + tt.open + "\n" + tt.body + strings.Repeat("x", 9000) + "\n" + tt.close
						end := w + 2
						if tt.id == Batch {
							end--
						}
						got, err := l.Folding()(context.Background(), text)
						if err != nil || !reflect.DeepEqual(got, []FoldRange{{w, end}}) {
							t.Errorf("%s worker %d/%d: %v %v", tt.id, w, at, got, err)
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
			var fold FoldingFunc
			switch tt.id {
			case Makefile:
				fold = makeFolding(scan)
			case Batch:
				fold = batchFolding(scan)
			default:
				fold = templateFolding(tt.id, scan)
			}
			if got, err := fold(ctx, strings.Repeat("x ", 16000)); err != context.Canceled || got != nil {
				t.Fatalf("mid scan: %v %v", got, err)
			}
		})
	}
}

func TestRemainingTemplateObservation(t *testing.T) {
	for _, id := range []ID{GoTemplate, Jinja2} {
		l, _ := Builtin(id)
		text := "{{- if .X -}}\n{{/* 正文\n*/}}\n{{printf \"end\"}}\n{{end}}"
		if id == Jinja2 {
			text = "{%- if x -%}\n{% raw %}\n{% \"endraw\" %}\n{%- endraw -%}\n{# 正文\n#}\n{% endif %}"
		}
		for chunk := 1; chunk <= 17; chunk++ {
			lex := &foldLexemes{row: -1, template: &templateFoldObserver{}}
			observed := foldingTokenizer{l.Tokenizer(), lex}
			a, b := l.Tokenizer().InitialState(), l.Tokenizer().InitialState()
			for _, line := range strings.Split(text, "\n") {
				for at := 0; ; {
					in := Input{Text: line[at:], Offset: at, Limit: min(chunk, len(line)-at), LineEnd: true}
					x, err := l.Tokenizer().Scan(in, a)
					if err != nil {
						t.Fatal(err)
					}
					y, err := observed.Scan(in, b)
					if err != nil {
						t.Fatal(err)
					}
					a, b = x.State, y.State
					if !a.Equal(b) {
						t.Fatalf("%s chunk %d changed state", id, chunk)
					}
					y.State = x.State
					if !reflect.DeepEqual(x, y) {
						t.Fatalf("%s chunk %d changed scan result", id, chunk)
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
