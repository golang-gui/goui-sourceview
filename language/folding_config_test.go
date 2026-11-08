package language

import (
	"context"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestBuiltinConfigFolding(t *testing.T) {
	for _, id := range []ID{Nginx, GoMod, GoWork, INI, GitConfig, EditorConfig, Systemd, DesktopEntry} {
		t.Run(string(id), func(t *testing.T) {
			l, _ := Builtin(id)
			if l.Folding() == nil {
				t.Fatal("builtin has no basic configuration folding provider")
			}
		})
	}
}

func TestConfigFoldingRanges(t *testing.T) {
	for _, tt := range []struct {
		name string
		id   ID
		text string
		want []FoldRange
	}{
		{"nginx nested", Nginx, "http {\n server {\n  listen 80;\n }\n}", []FoldRange{{0, 4}, {1, 3}}},
		{"nginx literal", Nginx, "server {\n set $text \"first\n } [ ( #\n last\";\n}", []FoldRange{{0, 4}, {1, 3}}},
		{"nginx variables", Nginx, "server {\n set $value ${host};\n set $open \\{;\n # } {\n}", []FoldRange{{0, 4}}},
		{"nginx other punctuation", Nginx, "server {\n location ~ ^/[a-z]+(foo {\n  return 200;\n }\n}", []FoldRange{{0, 4}, {1, 3}}},
		{"nginx closing tail", Nginx, "server {\n listen 80;\n} server {\n listen 81;\n}", []FoldRange{{0, 1}, {2, 4}}},
		{"nginx incomplete", Nginx, "server {\n listen 80;", nil},
		{"nginx unfinished quote", Nginx, "\"first\n } {", nil},
		{"mod groups", GoMod, "require (\n example.com/a v1.0.0\n example.com/b v2.0.0\n)\nreplace (\n example.com/a => ../a\n)", []FoldRange{{0, 3}, {4, 6}}},
		{"mod comments quotes", GoMod, "require (\n // ) (\n \"example.com/a)\" v1.0.0\n)", []FoldRange{{0, 3}}},
		{"mod interval ignored", GoMod, "retract (\n [v1.0.0,\n v1.1.0]\n)", []FoldRange{{0, 3}}},
		{"mod closing tail", GoMod, "require (\n example.com/a v1.0.0\n) tail", []FoldRange{{0, 1}}},
		{"mod incomplete", GoMod, "require (\n example.com/a v1.0.0", nil},
		{"work groups", GoWork, "use (\n ./a\n ./b\n)\nreplace (\n example.com/a => ../a\n)", []FoldRange{{0, 3}, {4, 6}}},
		{"work quotes", GoWork, "use (\n \"./(a)\"\n `./[b]`\n)", []FoldRange{{0, 3}}},
		{"ini sections", INI, "[first]\na=1\nb=2\n\n[second]\nc=3\n", []FoldRange{{0, 2}, {4, 5}}},
		{"ini comments and values", INI, "[main] # title\ntext=\"[fake]\"\n# [comment]\n; [comment]\n[next]\nx=2", []FoldRange{{0, 3}, {4, 5}}},
		{"ini duplicate names", INI, "[main]\nx=1\n[main]\nx=2", []FoldRange{{0, 1}, {2, 3}}},
		{"ini empty section", INI, "[first]\n\n[next]\nx=1", []FoldRange{{2, 3}}},
		{"ini malformed", INI, "[main]\nx=1\n[broken\n[]\n[next] trailing\ny=2", []FoldRange{{0, 5}}},
		{"git quoted subsection", GitConfig, "[remote \"origin]main\"] # ]\nurl = https://example.com\nfetch = value\n[branch \"main\"]\nmerge = refs/heads/main", []FoldRange{{0, 2}, {3, 4}}},
		{"git continuation", GitConfig, "[main]\ntext=first\\\n[fake]\nlast=value\n[next]\nx=1", []FoldRange{{0, 3}, {4, 5}}},
		{"git quoted continuation", GitConfig, "[main]\ntext=\"first\\\n[fake]\"\n[next]\nx=1", []FoldRange{{0, 2}, {3, 4}}},
		{"git invalid header", GitConfig, "[main]\nx=1\n[remote \"broken]\ny=2", []FoldRange{{0, 3}}},
		{"editor glob headers", EditorConfig, "root=true\n[*[0-9].{go,mod}]\nindent_size=4\n[*.txt]\nindent_size=2", []FoldRange{{1, 2}, {3, 4}}},
		{"editor brackets in values", EditorConfig, "[*]\nx=[fake]\n# [comment]\n[*.go] # no inline comments\ny=2\n[*.txt]\nz=3", []FoldRange{{0, 4}, {5, 6}}},
		{"editor glob closing bracket", EditorConfig, "[foo\\].txt]\nx=1\n[[]]\ny=2", []FoldRange{{0, 1}, {2, 3}}},
		{"systemd sections", Systemd, "[Unit]\nDescription=Demo\n[Service]\nExecStart=/usr/bin/demo\n[Install]\nWantedBy=default.target", []FoldRange{{0, 1}, {2, 3}, {4, 5}}},
		{"systemd continuation", Systemd, "[Service]\nExecStart=first \\\n# comment\n[NotASection]\nType=simple\n[Install]\nWantedBy=default.target", []FoldRange{{0, 4}, {5, 6}}},
		{"systemd locale like value", Systemd, "[Service]\nEnvironment=VALUE=[fake]\n[next]\nx=1", []FoldRange{{0, 1}, {2, 3}}},
		{"desktop sections", DesktopEntry, "[Desktop Entry]\nName=Demo\nName[zh_CN]=正文\n[Desktop Action Open]\nName=Open\nExec=demo", []FoldRange{{0, 2}, {3, 5}}},
		{"desktop literals", DesktopEntry, "[Desktop Entry]\nName=[fake]\n# [comment]\n[Desktop Action Next]\nName=Next", []FoldRange{{0, 2}, {3, 4}}},
		{"desktop incomplete header", DesktopEntry, "[Desktop Entry]\nName=Demo\n[broken\nName=Next", []FoldRange{{0, 3}}},
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

func TestConfigFoldingWorkers(t *testing.T) {
	for _, tt := range []struct {
		id          ID
		open, close string
	}{
		{Nginx, "server {", "}"}, {GoMod, "require (", ")"}, {GoWork, "use (", ")"},
		{INI, "[main]", "[next]"}, {GitConfig, "[remote \"main]\"]", "[next]"},
		{EditorConfig, "[*[0-9].go]", "[next]"}, {Systemd, "[Service]", "[Next]"}, {DesktopEntry, "[Desktop Entry]", "[Next]"},
	} {
		t.Run(string(tt.id), func(t *testing.T) {
			l, _ := Builtin(tt.id)
			if l.Folding() == nil {
				t.Fatal("missing folding provider")
			}
			var wg sync.WaitGroup
			for worker := 0; worker < 8; worker++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for at := 4090; at <= 4097; at++ {
						pad := strings.Repeat(" ", at)
						body := "value=" + strings.Repeat("x", 9000)
						text := strings.Repeat("\n", worker) + pad + tt.open + "\n" + body + "\n" + pad + tt.close
						end := worker + 2
						if tt.id != Nginx && tt.id != GoMod && tt.id != GoWork {
							end = worker + 1
						}
						got, err := l.Folding()(context.Background(), text)
						if err != nil || !reflect.DeepEqual(got, []FoldRange{{worker, end}}) {
							t.Errorf("worker %d offset %d = %v, %v", worker, at, got, err)
						}
					}
				}()
			}
			wg.Wait()
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			got, err := l.Folding()(ctx, "[main]\nx=1")
			if err != context.Canceled || got != nil {
				t.Fatalf("cancelled = %v, %v", got, err)
			}
			ctx, cancel = context.WithCancel(context.Background())
			defer cancel()
			scanner := cancelCodeScanner{l.Tokenizer(), cancel}
			fold := sectionFolding(tt.id, scanner)
			if tt.id == Nginx || tt.id == GoMod || tt.id == GoWork {
				fold = structuralFolding(tt.id, scanner)
			}
			got, err = fold(ctx, strings.Repeat("x ", 16000))
			if err != context.Canceled || got != nil {
				t.Fatalf("mid-scan cancellation = %v, %v", got, err)
			}
		})
	}
}
