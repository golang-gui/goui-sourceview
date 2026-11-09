package language

import (
	"context"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestBuiltinSSHRequestFolding(t *testing.T) {
	for _, id := range []ID{SSHConfig, HTTP} {
		l, _ := Builtin(id)
		if l.Folding() == nil {
			t.Errorf("%s: builtin has no SSH/request folding provider", id)
		}
	}
}

func TestSSHRequestFoldingRanges(t *testing.T) {
	for _, tt := range []struct {
		name string
		id   ID
		text string
		want []FoldRange
	}{
		{"ssh sections", SSHConfig, "Host work\n HostName work.example\n User demo\nMatch host *.example\n Port 2222\nHost *\n ServerAliveInterval 60", []FoldRange{{0, 2}, {3, 4}, {5, 6}}},
		{"ssh global prelude", SSHConfig, "Include ~/.ssh/conf.d/*\nUser demo\nHost work\n HostName work.example\n", []FoldRange{{2, 3}}},
		{"ssh case and equals", SSHConfig, "  hOsT = work !test\n User demo\n\tmAtCh=all\n Port 22", []FoldRange{{0, 1}, {2, 3}}},
		{"ssh fake headers", SSHConfig, "Host work\n # Host fake\n ProxyCommand echo Host other\n HostName host\n IdentityFile \"Match all # Host\"\nHost next\n User other", []FoldRange{{0, 4}, {5, 6}}},
		{"ssh quoted argument", SSHConfig, "Host \"work # local\" # title\n User demo\nMatch exec \"echo Host fake\"\n Port 22", []FoldRange{{0, 1}, {2, 3}}},
		{"ssh parameters", SSHConfig, "Host ${NAME} %h\n User demo\nMatch host !*.local user demo\n IdentityFile ~/.ssh/%h", []FoldRange{{0, 1}, {2, 3}}},
		{"ssh empty headers", SSHConfig, "Host\nUser demo\nHost =\nPort 22\nMatch # incomplete\nUser other", nil},
		{"ssh malformed within section", SSHConfig, "Host work\n User demo\nHost\nMatch = # incomplete\n Port 22", []FoldRange{{0, 4}}},
		{"ssh empty sections", SSHConfig, "Host first\n\nHost second\n User demo\n\nHost last", []FoldRange{{2, 3}}},
		{"ssh duplicate sections", SSHConfig, "Host same\n User first\nHost same\n User second", []FoldRange{{0, 1}, {2, 3}}},
		{"ssh only comments", SSHConfig, "# Host fake\n# Match all\n", nil},
		{"http blocks", HTTP, "### first\nGET https://example.com\nAccept: text/plain\n\n### second\nPOST https://example.com\nContent-Type: application/json\n\n{\n \"value\": 1\n}\n", []FoldRange{{0, 2}, {4, 10}}},
		{"http first without separator", HTTP, "GET https://example.com\nAccept: text/plain\n\n### next\nGET https://other.example\nX-Value: 2", []FoldRange{{0, 1}, {3, 5}}},
		{"http implicit get", HTTP, "https://example.com\nAccept: text/plain\n\n### next\nhttp://next.example\nX-Value: 2", []FoldRange{{0, 1}, {3, 5}}},
		{"http url scheme case", HTTP, "HTTPS://example.com\nAccept: text/plain\n### next\nHtTp://next.example\nX-Value: 2", []FoldRange{{0, 1}, {2, 4}}},
		{"http fake url", HTTP, "### first\n\"https://example.com\"\nX-Value: body\n### next\nhttps://\nX-Value: body", nil},
		{"http prelude", HTTP, "@url=https://example.com\n# initial comment\n\nPOST {{url}}\nContent-Type: text/plain\n\nbody", []FoldRange{{3, 6}}},
		{"http verb variable prelude", HTTP, "@method=GET\nGET https://example.com\nAccept: text/plain", []FoldRange{{1, 2}}},
		{"http single line", HTTP, "GET https://example.com", nil},
		{"http separator single request", HTTP, "### first\nGET https://example.com\n", []FoldRange{{0, 1}}},
		{"http empty blocks", HTTP, "### empty\n\n### comments\n# note\n### vars\n@url=https://example.com\n### real\nGET https://example.com", []FoldRange{{6, 7}}},
		{"http body method", HTTP, "GET https://example.com\nContent-Type: text/plain\n\nPOST https://fake.example\nX-Fake: body\n### next\nGET https://next.example", []FoldRange{{0, 4}, {5, 6}}},
		{"http body url", HTTP, "https://example.com\nContent-Type: text/plain\n\nhttp://fake.example\nbody\n### next\nhttps://next.example", []FoldRange{{0, 4}, {5, 6}}},
		{"http embedded body opaque", HTTP, "### json\nPOST https://example.com\nContent-Type: application/json\n\n{\n \"text\": \"### fake\",\n \"list\": [\n  1, 2\n ]\n}\n### next\nGET https://next.example", []FoldRange{{0, 9}, {10, 11}}},
		{"http separator recovers body", HTTP, "### json\nPOST https://example.com\nContent-Type: application/json\n\n{\n \"text\": \"unfinished\n### next\nGET https://next.example\nAccept: text/plain", []FoldRange{{0, 5}, {6, 8}}},
		{"http separator spelling", HTTP, "  ### first\nGET https://example.com\n\t#### next\nGET https://next.example", []FoldRange{{0, 1}, {2, 3}}},
		{"http false separators", HTTP, "### first\nGET https://example.com/###\nX-Value: ###\n\ntext ### inline\n# ### comment\n### next\nGET https://next.example", []FoldRange{{0, 5}, {6, 7}}},
		{"http missing method target", HTTP, "### first\nGET\n### next\nPOST\n", nil},
		{"http quoted method", HTTP, "### first\n\"GET\" https://example.com\nX-Value: body", nil},
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

func TestSSHRequestFoldingWorkers(t *testing.T) {
	for _, id := range []ID{SSHConfig, HTTP} {
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
						body := strings.Repeat("\n", worker) + strings.Repeat(" ", at)
						if id == SSHConfig {
							body += "Host work\n User " + strings.Repeat("x", 9000) + "\nHost next"
						} else {
							body += "### first\nGET https://example.com/" + strings.Repeat("x", 9000) + "\n### next"
							if at%2 == 1 {
								body = strings.Repeat("\n", worker) + strings.Repeat(" ", at) + "https://example.com/" + strings.Repeat("x", 9000) + "\nAccept: text/plain\n### next"
							}
						}
						got, err := l.Folding()(context.Background(), body)
						if err != nil || !reflect.DeepEqual(got, []FoldRange{{worker, worker + 1}}) {
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
			fold := sectionFolding(id, scanner)
			if id == HTTP {
				fold = requestFolding(scanner)
			}
			if got, err := fold(ctx, strings.Repeat("x ", 16000)); err != context.Canceled || got != nil {
				t.Fatalf("mid-scan cancellation = %v, %v", got, err)
			}
		})
	}
}
