package sourceview

import (
	"fmt"
	"image/color"
	"math/rand"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/golang-gui/goui-sourceview/internal/highlight"
	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui-sourceview/scheme"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/style"
)

func TestHighlightsValidationAndOwnership(t *testing.T) {
	for _, spans := range [][]HighlightSpan{
		{{Range: textedit.Range{Start: -1, End: 1}}},
		{{Range: textedit.Range{Start: 0, End: 0}}},
		{{Range: textedit.Range{Start: 1, End: 2}}},
		{{Range: textedit.Range{Start: 0, End: 7}}},
		{{Range: textedit.Range{Start: 0, End: 3}, Kind: "bad key"}},
		{{Range: textedit.Range{Start: 0, End: 3}}, {Range: textedit.Range{Start: 0, End: 3}}},
	} {
		if _, err := NewHighlights("值\nab", spans); err == nil {
			t.Fatalf("接受非法范围: %+v", spans)
		}
	}
	for _, text := range []string{"a\r\nb", string([]byte{255})} {
		if _, err := NewHighlights(text, nil); err == nil {
			t.Fatal("接受未规范化文本")
		}
	}
	spans := []HighlightSpan{{textedit.Range{Start: 0, End: 4}, language.KindNumber}}
	h, err := NewHighlights("值\nab", spans)
	if err != nil {
		t.Fatal(err)
	}
	spans[0].Kind = language.KindComment
	d := NewDocument("值\nab")
	if ok, err := d.SetHighlights(d.model.Revision(), h); !ok || err != nil {
		t.Fatal(ok, err)
	}
	got := d.highlightSpans(textedit.Range{End: d.model.Len()})
	if len(got) != 1 || got[0].Kind != language.KindNumber {
		t.Fatal("快照没有独立持有数据", got)
	}
	revision := d.model.Revision()
	if ok, err := d.SetHighlights(revision+1, nil); ok || err != nil {
		t.Fatal("旧清除未被忽略")
	}
	wrong, _ := NewHighlights("", nil)
	if ok, err := d.SetHighlights(revision, wrong); ok || err == nil {
		t.Fatal("不同长度没有拒绝")
	}
	if !reflect.DeepEqual(got, d.highlightSpans(textedit.Range{End: d.model.Len()})) {
		t.Fatal("失败发布改动了颜色")
	}
	if ok, err := d.SetHighlights(revision, nil); !ok || err != nil || d.highlightRoot != nil {
		t.Fatal("清除失败")
	}
	// 错误的同长度快照也不能把半个 UTF-8 字符交给原生渲染器。
	wrong, _ = NewHighlights("abcdef", []HighlightSpan{{textedit.Range{Start: 1, End: 2}, language.KindString}})
	d.SetHighlights(revision, wrong)
	if len(d.highlightSpans(textedit.Range{End: 6})) != 0 {
		t.Fatal("绘制传入非法字节边界")
	}
}

func TestHighlightsEditMappingAndIsolation(t *testing.T) {
	for _, tc := range []struct {
		start, end int
		text       string
		want       []HighlightSpan
	}{
		{2, 2, "XX", []HighlightSpan{{textedit.Range{Start: 4, End: 7}, language.KindNumber}}},
		{5, 5, "XX", []HighlightSpan{{textedit.Range{Start: 2, End: 5}, language.KindNumber}}},
		{3, 3, "XX", nil}, {1, 3, "", nil}, {4, 6, "", nil}, {0, 2, "", []HighlightSpan{{textedit.Range{Start: 0, End: 3}, language.KindNumber}}},
	} {
		d := NewDocument("abcdefgh")
		h, _ := NewHighlights(d.model.Text(), []HighlightSpan{{textedit.Range{Start: 2, End: 5}, language.KindNumber}})
		d.SetHighlights(d.model.Revision(), h)
		other := NewDocument(d.model.Text())
		other.SetHighlights(other.model.Revision(), h)
		conn := d.model.ConnectChange(d.changeHighlights)
		if err := d.model.ReplaceAtomic(textedit.Range{Start: tc.start, End: tc.end}, tc.text); err != nil {
			t.Fatal(err)
		}
		conn.Disconnect()
		if got := d.highlightSpans(textedit.Range{End: d.model.Len()}); !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("编辑 %v 后: %v, 预期 %v", tc, got, tc.want)
		}
		if got := other.highlightSpans(textedit.Range{End: 8}); len(got) != 1 || got[0].Range != (textedit.Range{Start: 2, End: 5}) {
			t.Fatal("共享快照被修改")
		}
	}
}

func TestHighlightsRandomEdits(t *testing.T) {
	// 用朴素数组独立计算预期，覆盖树的懒平移、分裂及多次编辑。
	text := strings.Repeat("x", 600)
	var spans []HighlightSpan
	for i := 0; i < 600; i += 6 {
		spans = append(spans, HighlightSpan{textedit.Range{Start: i, End: i + 3}, language.KindNumber})
	}
	d := NewDocument(text)
	h, _ := NewHighlights(text, spans)
	d.SetHighlights(d.model.Revision(), h)
	c := d.model.ConnectChange(d.changeHighlights)
	defer c.Disconnect()
	rng := rand.New(rand.NewSource(44))
	for step := 0; step < 300; step++ {
		start := rng.Intn(d.model.Len() + 1)
		end := min(d.model.Len(), start+rng.Intn(8))
		insert := strings.Repeat("x", rng.Intn(6))
		if start == end && insert == "" {
			continue
		}
		var want []HighlightSpan
		for _, s := range spans {
			if s.Range.End <= start {
				want = append(want, s)
			} else if s.Range.Start >= end {
				delta := len(insert) - (end - start)
				s.Range.Start += delta
				s.Range.End += delta
				want = append(want, s)
			}
		}
		if err := d.model.ReplaceAtomic(textedit.Range{Start: start, End: end}, insert); err != nil {
			t.Fatal(err)
		}
		spans = want
		if got := d.highlightSpans(textedit.Range{End: d.model.Len()}); !reflect.DeepEqual(got, want) {
			t.Fatalf("第 %d 步结果不符: %v / %v", step, got, want)
		}
	}
}

func TestHighlightsLifecycleAndReentrantPublication(t *testing.T) {
	v := NewSourceView()
	v.SetDocument(NewDocument("abc"))
	d := v.document
	h, _ := NewHighlights("abc", []HighlightSpan{{textedit.Range{End: 3}, language.KindNumber}})
	d.SetHighlights(d.model.Revision(), h)
	// 未挂载时不维持模型订阅；错过修订后挂载必须清除提示。
	d.model.ReplaceAtomic(textedit.Range{}, "x")
	observe(t, v)
	if d.highlightRoot != nil {
		t.Fatal("重新挂载保留了旧坐标")
	}
	h, _ = NewHighlights(d.model.Text(), []HighlightSpan{{textedit.Range{End: 4}, language.KindNumber}})
	d.SetHighlights(d.model.Revision(), h)
	d.model.SetText("new")
	if d.highlightRoot != nil {
		t.Fatal("重载未清除颜色")
	}
	h, _ = NewHighlights("new", []HighlightSpan{{textedit.Range{End: 3}, language.KindNumber}})
	d.SetHighlights(d.model.Revision(), h)
	d.SetLanguage(language.JSON)
	if d.highlightRoot != nil {
		t.Fatal("换语言未清除颜色")
	}
	// 早于 Document 订阅者发布新结果时，迟到的同修订编辑通知不能再映射它。
	d = NewDocument("abc")
	c := d.model.ConnectChange(func(change textedit.Change) {
		h, _ := NewHighlights(d.model.Text(), []HighlightSpan{{textedit.Range{End: d.model.Len()}, language.KindString}})
		d.SetHighlights(change.Revision, h)
	})
	defer c.Disconnect()
	mapChange := d.model.ConnectChange(d.changeHighlights)
	defer mapChange.Disconnect()
	d.model.ReplaceAtomic(textedit.Range{}, "x")
	if got := d.highlightSpans(textedit.Range{End: 4}); len(got) != 1 || got[0].Range.End != 4 {
		t.Fatal("重复映射了新结果", got)
	}
}

func TestHighlightsForegroundComposition(t *testing.T) {
	v := NewSourceView()
	base, lexical, semantic, selected := color.NRGBA{1, 1, 1, 255}, color.NRGBA{2, 2, 2, 255}, color.NRGBA{3, 3, 3, 255}, color.NRGBA{4, 4, 4, 255}
	def := scheme.Light().Definition()
	def.Foreground = base
	def.SelectionForeground = selected
	def.Tokens["external"] = semantic
	def.Tokens[language.KindString] = lexical
	s, _ := scheme.New(def)
	v.SetScheme(s)
	got := v.paragraphColors(8, paragraphStyle{selection: textedit.Range{Start: 3, End: 5}, focused: true}, []highlight.Token{{Start: 0, End: 8, Kind: language.KindString}}, HighlightSpan{textedit.Range{Start: 2, End: 6}, "external"}, HighlightSpan{textedit.Range{Start: 6, End: 7}, language.KindPlain})
	want := []paragraphColor{{0, 2, lexical}, {2, 3, semantic}, {3, 5, selected}, {5, 6, semantic}, {7, 8, lexical}}
	if !reflect.DeepEqual(got, want) {
		t.Fatal("前景优先级不正确", got)
	}
}

func TestHighlightsReuseNativeGeometry(t *testing.T) {
	v, ctx := layoutWorkFixture(t, 1)
	v.SetScheme(scheme.Light())
	v.LayoutVisible(geometry.Size{Width: 220, Height: 200}, geometry.Point{})
	v.layoutStep(time.Time{})
	p := v.cachedParagraph(0)
	created := ctx.created
	d := v.document
	revision := d.model.Revision()
	definition := d.definition
	engine := d.engine
	h, _ := NewHighlights(d.model.Text(), []HighlightSpan{{textedit.Range{End: 3}, language.KindNumber}})
	d.SetHighlights(revision, h)
	v.Paint(&recordingPainter{})
	v.layoutStep(time.Time{})
	if ctx.created != created || v.cachedParagraph(0) != p || d.model.Revision() != revision || d.definition != definition || d.engine != engine {
		t.Fatal("颜色提交影响几何或语言状态")
	}
	if len(p.colors) == 0 {
		t.Fatal("没有更新颜色")
	}
	calls := p.layout.(*shapedLayout).colorCalls
	d.SetHighlights(revision, h)
	v.Paint(&recordingPainter{})
	v.layoutStep(time.Time{})
	if p.layout.(*shapedLayout).colorCalls != calls {
		t.Fatal("相同有效颜色重复修改原生布局")
	}
}

func BenchmarkHighlightsPublishAndEdit(b *testing.B) {
	for _, count := range []int{1000, 100000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			text := strings.Repeat("abc ", count)
			spans := make([]HighlightSpan, count)
			for i := range spans {
				spans[i] = HighlightSpan{textedit.Range{Start: i * 4, End: i*4 + 3}, language.KindNumber}
			}
			h, _ := NewHighlights(text, spans)
			d := NewDocument(text)
			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				d.SetHighlights(d.model.Revision(), h)
				d.changeHighlights(textedit.Change{Revision: d.model.Revision() + 1, Edits: []textedit.Edit{{Range: textedit.Range{Start: count * 2, End: count * 2}, Text: "x"}}})
			}
		})
	}
}

func TestHighlightsSharedViewsUndoAndMinimap(t *testing.T) {
	v := mapView("abc def")
	d := v.document
	observe(t, v)
	other := NewSourceView()
	other.SetDocument(d)
	observe(t, other)
	v.SetScheme(scheme.Light())
	other.SetScheme(scheme.Dark())
	h, _ := NewHighlights(d.model.Text(), []HighlightSpan{{textedit.Range{Start: 4, End: 7}, language.KindNumber}})
	d.SetHighlights(d.model.Revision(), h)
	v.prepareMinimap()
	finishMap(t, v)
	pixels := v.minimap.pixels
	r, _ := d.model.LineRange(0)
	want := scheme.Light().Color(language.KindNumber)
	want.A = 180
	if got := v.minimapRow(0, r, 0, v.minimap)[4]; got != want {
		t.Fatal("地图遗漏外部颜色", got, want)
	}
	if v.highlightColor(language.KindNumber) == other.highlightColor(language.KindNumber) {
		t.Fatal("共享文档污染视图配色")
	}
	d.model.ReplaceAtomic(textedit.Range{}, "x")
	if got := d.highlightSpans(textedit.Range{End: 8}); len(got) != 1 || got[0].Range.Start != 5 {
		t.Fatal("共享视图重复映射")
	}
	if !d.model.Undo() {
		t.Fatal("撤销失败")
	}
	if got := d.highlightSpans(textedit.Range{End: 7}); len(got) != 1 || got[0].Range.Start != 4 {
		t.Fatal("撤销提示坐标不正确")
	}
	if !d.model.Redo() {
		t.Fatal("重做失败")
	}
	if got := d.highlightSpans(textedit.Range{End: 8}); len(got) != 1 || got[0].Range.Start != 5 {
		t.Fatal("重做提示坐标不正确")
	}
	d.SetHighlights(d.model.Revision(), nil)
	v.prepareMinimap()
	if v.minimap.pixels != pixels {
		t.Fatal("颜色替换清空了整个地图位图")
	}
	r, _ = d.model.LineRange(0)
	want = v.colors.Foreground
	want.A = 180
	if got := v.minimapRow(0, r, 0, v.minimap)[5]; got != want {
		t.Fatal("地图保留旧语义颜色", got, want)
	}
}

func TestHighlightsCustomStyleWithoutLanguageRegistration(t *testing.T) {
	v := NewSourceView()
	first, second := color.NRGBA{50, 100, 150, 255}, color.NRGBA{150, 100, 50, 255}
	sheet := func(c color.NRGBA) style.StyleSheet {
		return style.Sheet(style.Name("source-view").Part("token-analysis.value").ForegroundColor(c))
	}
	owner := themedScope(v, sheet(first))
	v.StyleChanged()
	if got := v.highlightColor("analysis.value"); got != first {
		t.Fatal("未解析独立样式键", got)
	}
	owner.SetStyleSheet(sheet(second))
	v.StyleChanged()
	if got := v.highlightColor("analysis.value"); got != second {
		t.Fatal("主题更新未清理外部颜色缓存", got)
	}
}

func TestHighlightsPreeditProjection(t *testing.T) {
	v := NewSourceView()
	v.SetDocument(NewDocument("abc\ndef"))
	h, _ := NewHighlights(v.document.model.Text(), []HighlightSpan{{textedit.Range{End: 7}, language.KindNumber}})
	v.document.SetHighlights(v.document.model.Revision(), h)
	v.projection = textedit.NewProjection(v.document.model, textedit.Selection{Anchor: 2, Caret: 5})
	v.projection.Update("中文", 3)
	got := v.displayHighlights(textedit.Range{End: 10})
	want := []HighlightSpan{{textedit.Range{End: 2}, language.KindNumber}, {textedit.Range{Start: 8, End: 10}, language.KindNumber}}
	if !reflect.DeepEqual(got, want) {
		t.Fatal("预编辑影响了其他文本着色", got)
	}
}
