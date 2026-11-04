package sourceview

import (
	"image/color"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/golang-gui/goui-sourceview/internal/highlight"
	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui-sourceview/scheme"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui/textedit"
)

func mapView(text string) *SourceView {
	v := NewSourceView()
	v.SetDocument(NewDocument(text))
	v.viewport = geometry.Size{Width: 800, Height: 400}
	v.lineHeight = 20
	v.resetHeights()
	v.SetShowMinimap(true)
	v.appliedScheme = scheme.Light()
	v.prepareMinimap()
	return v
}
func finishMap(t *testing.T, v *SourceView) {
	t.Helper()
	for n := 0; v.minimapPending(); n++ {
		if n > 256 {
			t.Fatal("地图任务未收敛")
		}
		v.minimapStep(time.Now().Add(time.Second))
	}
}
func TestMinimapConfigurationAndNavigation(t *testing.T) {
	v := mapView(strings.Repeat("abc\n", 1000))
	if v.MinimapOptions() != (MinimapOptions{120, 120}) || v.textRight() != 680 {
		t.Fatal("默认配置或正文宽度错误")
	}
	for _, o := range []MinimapOptions{{Width: 39}, {Width: 321}, {Width: float32(math.NaN())}, {MaxColumns: 257}, {MaxColumns: -1}} {
		if v.SetMinimapOptions(o) == nil || v.MinimapOptions() != (MinimapOptions{120, 120}) {
			t.Fatal("非法配置改变了状态")
		}
	}
	v.mounted = true
	v.selection = textedit.Selection{Anchor: 1, Caret: 2}
	selected := v.Selection()
	var target geometry.Rectangle
	v.ConnectScrollIntoView(func(r geometry.Rectangle) { target = r })
	v.offset.X = 10
	v.beginMinimapDrag(geometry.Point{X: 740, Y: 200})
	want := (v.minimap.total - v.viewport.Height) / 2
	if math.Abs(float64(target.Y-want)) > 1 || target.X != 10 || v.Selection() != selected {
		t.Fatalf("地图点击未保持位置契约 %+v want %v", target, want)
	}
	frozen := v.minimap.dragTotal
	v.setParagraphHeight(0, 1000)
	v.prepareMinimap()
	if !v.minimap.dragging || v.minimap.dragTotal != frozen {
		t.Fatal("异步高度改变了拖动比例")
	}
	v.dragMinimap(geometry.Point{Y: 400})
	if target.Y != frozen-400 {
		t.Fatal("拖到末尾未使用起始总高度")
	}
	v.endMinimapDrag()
	v.viewport.Width = 100
	if v.minimapWidth() != 20 {
		t.Fatal("窄视图没有保留正文宽度")
	}
	v.SetShowMinimap(false)
	if v.minimap != nil || v.textRight() != 100 {
		t.Fatal("关闭后未释放地图或恢复正文")
	}
}

func TestMinimapSamplingAndReuse(t *testing.T) {
	v := mapView("\t甲\"a\": 12\nsecond\n")
	d := v.document
	d.engine = highlight.New(d.model, language.JSON)
	for d.engine.Pending() {
		d.engine.Advance(time.Now().Add(time.Second))
	}
	v.prepareMinimap()
	r, _ := d.model.LineRange(0)
	pixels := v.minimapRow(0, r, 0, v.minimap)
	for i := 0; i < 4; i++ {
		if pixels[i].A != 0 {
			t.Fatal("Tab 未保留空白单元")
		}
	}
	if pixels[4].A == 0 || pixels[5].A == 0 {
		t.Fatal("Unicode 或字符串丢失")
	}
	number := v.colors.Tokens[language.KindNumber]
	number.A = 180
	if pixels[10] != number {
		t.Fatalf("数字颜色=%v want=%v", pixels[10], number)
	}
	again := v.minimapRow(0, r, 0, v.minimap)
	if &pixels[0] != &again[0] {
		t.Fatal("相同片段未复用")
	}
	r2, _ := d.model.LineRange(1)
	preserved := v.minimapRow(1, r2, 0, v.minimap)
	h := d.model.ConnectChange(func(c textedit.Change) { d.engine.Change(c) })
	defer h.Disconnect()
	if err := d.model.Apply(textedit.Edit{Range: textedit.Range{Start: 0, End: 0}, Text: "x"}, textedit.EditOptions{}); err != nil {
		t.Fatal(err)
	}
	v.prepareMinimap()
	r2, _ = d.model.LineRange(1)
	reused := v.minimapRow(1, r2, 0, v.minimap)
	if &preserved[0] != &reused[0] {
		t.Fatal("编辑错误地丢弃了未变行缓存")
	}
	v.appliedScheme = scheme.Dark()
	v.colors = scheme.Dark().Definition()
	v.prepareMinimap()
	if len(v.minimap.cache) != 0 {
		t.Fatal("配色切换仍保留旧缓存")
	}
	other := mapView("different")
	if other.minimap == v.minimap || other.minimap.pixels == v.minimap.pixels {
		t.Fatal("不同视图共享可变地图")
	}
	v.SetDocument(NewDocument("new"))
	if v.minimap != nil {
		t.Fatal("换文档仍保留旧地图")
	}
}

func TestMinimapBoundedLongLineAndFolds(t *testing.T) {
	v := mapView(strings.Repeat("甲 ", 300000))
	v.SetWrapMode(WrapChar)
	v.setParagraphHeight(0, 100000)
	v.prepareMinimap()
	s := v.minimap
	v.minimapStep(time.Now().Add(-time.Second))
	if s.next != 0 {
		t.Fatal("过期预算仍执行采样")
	}
	v.minimapStep(time.Now().Add(time.Second))
	if s.next != 16 {
		t.Fatalf("单回合采样数 %d", s.next)
	}
	finishMap(t, v)
	if len(s.pixels.Pix) > 256*2048*4 || len(s.cache) > 2048 {
		t.Fatal("超过资源上限")
	}
	if s.pixels.NRGBAAt(0, 0) == (color.NRGBA{}) || s.pixels.NRGBAAt(0, 200) == (color.NRGBA{}) {
		t.Fatal("长折行只有首行内容")
	}
	if len(v.layouts) != 0 {
		t.Fatal("地图创建了原生文本布局")
	}
	v.viewport.Height = 9000
	v.prepareMinimap()
	if v.minimap.pixels.Rect.Dy() != 2048 {
		t.Fatal("超高视图未限制栅格高度")
	}

	folded := mapView("head\nhidden\ntail")
	folded.hiddenLines = []lineInterval{{1, 2}}
	folded.resetHeights()
	folded.prepareMinimap()
	finishMap(t, folded)
	if folded.paragraphAt(21) != 2 || folded.visibleLines.count != 2 {
		t.Fatal("折叠后的地图没有复用正文行索引")
	}
	// 显示首行后必须紧接 tail，而不是再采样隐藏行。
	r, _ := folded.document.model.LineRange(2)
	tail := folded.minimapRow(2, r, 0, folded.minimap)
	if tail[0].A == 0 || tail[4].A != 0 {
		t.Fatal("折叠尾行采样错误")
	}
}

// 地图占据固定右侧区域后，横向显露目标必须包含地图宽度。
func TestMinimapCaretRevealAndHorizontalScroll(t *testing.T) {
	v, scroll := revealFixture(t)
	v.SetShowMinimap(true)
	v.document.model.SetText(strings.Repeat("a", 2000))
	v.SetSelection(textedit.Selection{Caret: 1900})
	v.RevealSelection()
	for range 12 {
		scroll.Arrange(scroll.Rect())
	}
	r, ok := v.cachedCaretRect()
	if !ok || r.X+r.Width > v.textRight() || r.X < v.gutter {
		t.Fatalf("光标被地图遮住: %v bodyRight=%v offset=%v", r, v.textRight(), v.offset)
	}
	if v.reveal != nil {
		t.Fatal("横向显露未收敛")
	}
	v.prepareMinimap()
	before := v.minimapRect()
	v.offset.X += 50
	if v.minimapRect() != before {
		t.Fatal("地图随正文横向滚动")
	}
}

// 下一个绘制帧之前的编辑也必须让旧拖动失效，不能滚动新修订。
func TestMinimapDragRevisionAndUnmount(t *testing.T) {
	v := mapView(strings.Repeat("text\n", 100))
	v.mounted = true
	calls := 0
	v.ConnectScrollIntoView(func(geometry.Rectangle) { calls++ })
	v.beginMinimapDrag(geometry.Point{Y: 200})
	before := calls
	v.document.model.SetText("replacement")
	v.dragMinimap(geometry.Point{Y: 300})
	if calls != before || v.minimap.dragging {
		t.Fatal("旧修订的拖动仍在滚动新文档")
	}
	v.unmount()
	if v.minimap != nil {
		t.Fatal("卸载后保留了地图缓存")
	}
}

func TestMinimapSamplingOptionsKeepGeometry(t *testing.T) {
	v := mapView("text")
	epoch, total := v.layoutEpoch, v.heights.Total()
	if err := v.SetMinimapOptions(MinimapOptions{MaxColumns: 80}); err != nil {
		t.Fatal(err)
	}
	if v.layoutEpoch != epoch || v.heights.Total() != total {
		t.Fatal("地图采样配置使正文排版失效")
	}
	if err := v.SetMinimapOptions(MinimapOptions{Width: 100}); err != nil {
		t.Fatal(err)
	}
	if v.layoutEpoch == epoch || v.heights.Total() != total {
		t.Fatal("地图宽度变化未保留高度估计或未更新布局")
	}
}
