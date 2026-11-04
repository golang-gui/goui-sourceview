package analysis

import (
	"context"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	source "github.com/golang-gui/goui-sourceview"
	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/gui/textedit"
)

func analyze(t *testing.T, text string) Result {
	t.Helper()
	r, err := Analyze(context.Background(), Snapshot{DocumentID: 1, Revision: 2, Text: text})
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func TestTypesChangeAllReferences(t *testing.T) {
	for _, tc := range []struct {
		value string
		kind  language.Kind
	}{{`"hello"`, StringVariable}, {`42`, NumberVariable}} {
		r := analyze(t, "package p\nfunc f(){value := "+tc.value+";println(value)}")
		if len(r.Diagnostics) != 0 {
			t.Fatal(r.Diagnostics)
		}
		count := 0
		for _, span := range r.Spans {
			if r.Snapshot.Text[span.Start:span.End] == "value" {
				count++
				if span.Kind != tc.kind {
					t.Fatalf("引用类型未改变: %+v", span)
				}
			}
		}
		if count != 2 {
			t.Fatalf("声明及引用数量错误: %d", count)
		}
	}
	r := analyze(t, "package p\nfunc f(){println(missing)}")
	if len(r.Diagnostics) == 0 {
		t.Fatal("未报告未定义名称")
	}
	found := false
	for _, span := range r.Spans {
		if span.Kind == Invalid {
			found = true
		}
	}
	if !found {
		t.Fatal("未定义引用没有外部着色结果")
	}
}
func completion(t *testing.T, text string) []source.Completion {
	t.Helper()
	at := strings.Index(text, "|")
	if at < 0 {
		t.Fatal("缺少光标")
	}
	text = strings.Replace(text, "|", "", 1)
	start, end := at, at
	for start > 0 && isASCIIWord(text[start-1]) {
		start--
	}
	for end < len(text) && isASCIIWord(text[end]) {
		end++
	}
	c := source.CompletionContext{DocumentID: 1, Revision: 2, DocumentLength: len(text), Caret: at, Range: textedit.Range{Start: start, End: end}, Prefix: text[start:at]}
	out, err := Complete(context.Background(), Snapshot{DocumentID: 1, Revision: 2, Text: text}, c)
	if err != nil {
		t.Fatal(err)
	}
	return out
}
func isASCIIWord(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_' }
func TestCheckedCompletion(t *testing.T) {
	for _, name := range []string{"Name", "Email"} {
		out := completion(t, "package p\ntype Person struct {"+name+" string; Age int}\nfunc(p Person) Hello(){}\nfunc f(){person:=Person{};person.|}")
		got := map[string]bool{}
		for _, c := range out {
			got[c.Label] = true
			if c.Detail == "" {
				t.Fatal("缺少类型信息")
			}
		}
		if len(out) != 3 || !got[name] || !got["Age"] || !got["Hello"] {
			t.Fatalf("成员结果错误: %+v", out)
		}
	}
	out := completion(t, "package p\nvar visible int\nfunc f(){before:=1;vis|;after:=2;println(before,after)}")
	if len(out) != 1 || out[0].Label != "visible" {
		t.Fatalf("作用域或过滤错误: %+v", out)
	}
	out = completion(t, "package p\nfunc f(){before:=1;af|;after:=2;println(before,after)}")
	if len(out) != 0 {
		t.Fatal("推荐了尚未进入作用域的局部变量")
	}
}
func TestTokenizerFragmentsAndStaleSuffix(t *testing.T) {
	text := "package p\nfunc f(){值:=42;println(值)}\n"
	r := analyze(t, text)
	definition, err := Definition(r)
	if err != nil {
		t.Fatal(err)
	}
	tok := definition.Tokenizer()
	state := tok.InitialState()
	absolute := 0
	seen := 0
	for _, line := range strings.Split(text, "\n") {
		colors := make([]language.Kind, len(line))
		for offset := 0; ; {
			limit := min(3, len(line)-offset)
			for offset+limit < len(line) && !utf8.RuneStart(line[offset+limit]) {
				limit++
			}
			result, err := tok.Scan(language.Input{Text: line[offset:], Offset: offset, Limit: limit, LineEnd: true, DocumentEnd: absolute+len(line) == len(text)}, state)
			if err != nil {
				t.Fatal(err)
			}
			if result.ReplaceFrom != nil {
				clear(colors[*result.ReplaceFrom:])
			}
			for _, token := range result.Tokens {
				if token.Start < 0 || token.End > len(line) || token.Start >= token.End {
					t.Fatal("非法 token 范围")
				}
				for i := token.Start; i < token.End; i++ {
					colors[i] = token.Kind
				}
			}
			state = result.State
			offset += result.Consumed
			if offset == len(line) {
				break
			}
			if result.Consumed == 0 {
				t.Fatal("扫描没有进展")
			}
		}
		for _, kind := range colors {
			if kind == NumberVariable {
				seen++
			}
		}
		absolute += len(line) + 1
	}
	if seen != 2*len("值") {
		t.Fatalf("分片遗漏 Unicode 标识符: %d", seen)
	}
	first, _ := tok.Scan(language.Input{Text: "package q", Limit: 9, LineEnd: true}, tok.InitialState())
	second, _ := tok.Scan(language.Input{Text: "func f(){值:=42;println(值)}", Limit: len("func f(){值:=42;println(值)}"), LineEnd: true}, first.State)
	for _, token := range second.Tokens {
		if token.Kind == NumberVariable {
			t.Fatal("变更后的后缀复用了旧绝对坐标颜色")
		}
	}
	// 词法字符串/括号角色由内置扫描器拥有，语义着色不得覆盖 Class。
	normal := analyze(t, "package p\nfunc f(){println(\"hello\")}")
	definition, _ = Definition(normal)
	state = definition.Tokenizer().InitialState()
	found := false
	for _, line := range strings.Split(normal.Snapshot.Text, "\n") {
		r, _ := definition.Tokenizer().Scan(language.Input{Text: line, Limit: len(line), LineEnd: true}, state)
		state = r.State
		for _, tok := range r.Tokens {
			if tok.Class&language.ClassString != 0 {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("语义适配丢失词法字符串角色")
	}
}
func TestServiceDropsPostedResultsAndCloses(t *testing.T) {
	posted := make(chan func(), 4)
	count := 0
	service := NewService(func(f func()) { posted <- f }, func(r Result, err error) {
		count++
		if err != nil || r.Snapshot.Revision != 2 {
			t.Errorf("发布了旧结果: %+v %v", r, err)
		}
	})
	defer func() { service.Close(); service.Wait() }()
	receive := func() func() {
		t.Helper()
		select {
		case f := <-posted:
			return f
		case <-time.After(3 * time.Second):
			t.Fatal("后台没有返回结果")
			return nil
		}
	}
	service.Submit(Snapshot{1, 1, "package p\nvar x=1"}, 0)
	old := receive()
	service.Submit(Snapshot{2, 2, "package p\nvar x=2"}, 0)
	old()
	receive()()
	if count != 1 {
		t.Fatal("没有丢弃已经投递但过期的结果")
	}
	service.Submit(Snapshot{2, 3, "package p\nvar x=3"}, 0)
	queued := receive()
	service.Close()
	queued()
	if count != 1 {
		t.Fatal("销毁后仍发布结果")
	}
}
func TestCancellationAndLimits(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Analyze(ctx, Snapshot{Text: Sample}); err != context.Canceled {
		t.Fatal("未识别取消")
	}
	if _, err := Analyze(context.Background(), Snapshot{Text: strings.Repeat("x", MaxBytes+1)}); err == nil {
		t.Fatal("未限制分析输入")
	}
	out, err := Complete(context.Background(), Snapshot{DocumentID: 1, Revision: 3, Text: Sample}, source.CompletionContext{DocumentID: 1, Revision: 2})
	if err != nil || len(out) != 0 {
		t.Fatal("旧补全请求没有正常丢弃")
	}
}

func TestExternalContextExcludesCommentsAndStrings(t *testing.T) {
	for _, text := range []string{"package p\nfunc f(){println(\"na|\")}", "package p\n// na|\nfunc f(){}"} {
		if out := completion(t, text); len(out) != 0 {
			t.Fatalf("未知控件词法状态时仍须由外部检查排除字符串/注释: %+v", out)
		}
	}
}
