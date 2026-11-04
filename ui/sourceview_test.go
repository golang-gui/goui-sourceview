package ui

import (
	"testing"

	source "github.com/golang-gui/goui-sourceview"
	"github.com/golang-gui/goui/gui"
	base "github.com/golang-gui/goui/ui"
)

type context struct {
	base.BuildContext
	value any
}

// 标记/执行配置可完全使用 ui 类型；省略选项保留状态，-1 显式清除。
func TestLineMarkConfiguration(t *testing.T) {
	ctx := &context{}
	d := NewDocument("one\ntwo")
	_, err := d.AddLineMark(LineMark{Line: 1, Kind: MarkLogpoint, State: MarkPending})
	if err != nil {
		t.Fatal(err)
	}
	v := SourceView().Document(d).ShowLineMarks(true).ExecutionLine(1)
	w := v.Mount(ctx).(*source.SourceView)
	defer v.Unmount(ctx, w)
	v.Update(ctx, w)
	if line, ok := w.ExecutionLine(); line != 1 || !ok || !w.ShowLineMarks() {
		t.Fatal("configuration missing")
	}
	SourceView().Document(d).Update(ctx, w)
	if line, ok := w.ExecutionLine(); line != 1 || !ok || !w.ShowLineMarks() {
		t.Fatal("uncontrolled state reset")
	}
	SourceView().Document(d).ShowLineMarks(false).ExecutionLine(-1).Update(ctx, w)
	if _, ok := w.ExecutionLine(); ok || w.ShowLineMarks() {
		t.Fatal("explicit clear ignored")
	}
}

func (c *context) State() any         { return c.value }
func (c *context) SetState(value any) { c.value = value }
func TestBindingRestoresDefaultsAndDisconnects(t *testing.T) {
	ctx := &context{}
	initial := SourceView()
	widget := initial.Mount(ctx).(*source.SourceView)
	initial.Update(ctx, widget)
	document := NewDocument("abc")
	events := 0
	configured := SourceView().Document(document).ReadOnly(true).ShowLineNumbers(false).AcceptsTab(true).WrapMode(WrapWordChar).Selection(TextSelection{Caret: 2}).OnSelection(func(TextSelection) { events++ })
	configured.Update(ctx, widget)
	options := SourceView().AutoPairs(false).AutomaticCompletion(false)
	options.Update(ctx, widget)
	if widget.AutoPairs() || widget.AutomaticCompletion() {
		t.Fatal("editor options ignored")
	}
	configured.Update(ctx, widget)
	if !widget.AutoPairs() || !widget.AutomaticCompletion() {
		t.Fatal("editor option defaults not restored")
	}
	if events != 0 || widget.Document() != document || !widget.ReadOnly() || widget.ShowLineNumbers() || !widget.AcceptsTab() {
		t.Fatal("update emitted or lost config")
	}
	widget.SetSelection(TextSelection{Caret: 3})
	if events != 1 {
		t.Fatal(events)
	}
	initial.Update(ctx, widget)
	if widget.Document() == document || widget.ReadOnly() || !widget.ShowLineNumbers() || widget.AcceptsTab() || widget.WrapMode() != WrapNone {
		t.Fatal("defaults not restored")
	}
	configured.Update(ctx, widget)
	configured.Unmount(ctx, gui.Widget(widget))
	widget.SetSelection(TextSelection{})
	if events != 1 {
		t.Fatal("unmount leaked callback", events)
	}
}

func TestUncontrolledSearchSurvivesDeclarativeUpdate(t *testing.T) {
	ctx := &context{}
	v := SourceView()
	w := v.Mount(ctx).(*source.SourceView)
	v.Update(ctx, w)
	w.SetSearch(SearchQuery{Text: "user query", WholeWord: true})
	w.SetHighlightSearchMatches(false)
	v.Update(ctx, w)
	if w.Search() != (SearchQuery{Text: "user query", WholeWord: true}) || w.HighlightSearchMatches() {
		t.Fatal("rebuild cleared interactive query")
	}
	SourceView().HighlightSearchMatches(true).Update(ctx, w)
	if !w.HighlightSearchMatches() {
		t.Fatal("explicit search highlight option ignored")
	}
	SourceView().Search(SearchQuery{}).Update(ctx, w)
	if w.Search() != (SearchQuery{}) {
		t.Fatal("explicit controlled empty query ignored")
	}
	v.Unmount(ctx, w)
}

func TestKeyBindingsAreExplicitAndCopied(t *testing.T) {
	ctx := &context{}
	view := SourceView()
	w := view.Mount(ctx).(*source.SourceView)
	view.Update(ctx, w)
	bindings := KeyBindings{CommandComplete: {{Key: gui.KeyF2}}}
	configured := SourceView().KeyBindings(bindings)
	bindings[CommandComplete][0].Key = gui.KeyF4
	configured.Update(ctx, w)
	if got := w.KeyBindings()[CommandComplete]; len(got) != 1 || got[0].Key != gui.KeyF2 {
		t.Fatal("view retained caller key slices", got)
	}
	view.Update(ctx, w)
	if len(w.KeyBindings()) != 1 {
		t.Fatal("uncontrolled update restored default bindings")
	}
	SourceView().KeyBindings(nil).Update(ctx, w)
	view.Update(ctx, w)
	if len(w.KeyBindings()) != 0 {
		t.Fatal("explicit empty bindings were ignored or reset")
	}
	view.Unmount(ctx, w)
}

// 未声明显示选项的重建保留交互状态，显式 None/false 仍可关闭。
func TestUncontrolledWhitespaceSurvivesDeclarativeUpdate(t *testing.T) {
	ctx := &context{}
	view := SourceView()
	w := view.Mount(ctx).(*source.SourceView)
	defer view.Unmount(ctx, w)
	view.Update(ctx, w)
	if w.WhitespaceMode() != WhitespaceNone || w.ShowLineEndings() {
		t.Fatal("new view defaults")
	}
	if err := w.SetWhitespaceMode(WhitespaceBoundary); err != nil {
		t.Fatal(err)
	}
	w.SetShowLineEndings(true)
	view.Update(ctx, w)
	if w.WhitespaceMode() != WhitespaceBoundary || !w.ShowLineEndings() {
		t.Fatal("rebuild reset interactive state")
	}
	SourceView().WhitespaceMode(WhitespaceNone).Update(ctx, w)
	if w.WhitespaceMode() != WhitespaceNone || !w.ShowLineEndings() {
		t.Fatal("explicit None or independent EOL option ignored")
	}
	SourceView().WhitespaceMode(WhitespaceAll).ShowLineEndings(false).Update(ctx, w)
	view.Update(ctx, w)
	if w.WhitespaceMode() != WhitespaceAll || w.ShowLineEndings() {
		t.Fatal("explicit false or retained mode ignored")
	}
}

// 绑定声明不读取状态，避免在父 ComposeView 中错误建立整个子树的读取依赖。
func TestSelectionBindingDefersReadsAndHonorsModifierOrder(t *testing.T) {
	s := &selectionState{value: TextSelection{Caret: 2}}
	v := SourceView().BindSelection(s)
	if s.reads != 0 || v.selectionSource != s {
		t.Fatal("绑定声明提前读取了状态")
	}
	v.onSelection(TextSelection{Caret: 3})
	if s.value.Caret != 3 || s.writes != 1 {
		t.Fatal("绑定未回写用户选区")
	}
	v.Selection(TextSelection{Caret: 1})
	if v.selectionSource != nil || v.onSelection != nil || v.selection.Caret != 1 {
		t.Fatal("Selection 未取消读绑定和生成的回写")
	}
	callbacks := 0
	v.BindSelection(s).OnSelection(func(TextSelection) { callbacks++ })
	if v.selectionSource != s || v.fields.Check(writeSelection) {
		t.Fatal("OnSelection 应只替换回写，保留读绑定")
	}
	v.Selection(TextSelection{}).onSelection(TextSelection{})
	if callbacks != 1 || s.writes != 1 {
		t.Fatal("Selection 覆盖了显式回调或残留回写")
	}
	v.BindSelection(s).BindSelection(nil)
	if v.selectionSource != s || s.reads != 0 {
		t.Fatal("nil 绑定应为无操作，且声明阶段不能读取状态")
	}
}

type selectionState struct {
	base.State[TextSelection]
	value         TextSelection
	reads, writes int
}

func (s *selectionState) Get() TextSelection { s.reads++; return s.value }
func (s *selectionState) Set(value TextSelection) {
	s.writes++
	s.value = value
}

func TestMinimapConfiguration(t *testing.T) {
	ctx := &context{}
	v := SourceView().ShowMinimap(true).MinimapOptions(MinimapOptions{Width: 100, MaxColumns: 80})
	w := v.Mount(ctx).(*source.SourceView)
	defer v.Unmount(ctx, w)
	v.Update(ctx, w)
	if !w.ShowMinimap() || w.MinimapOptions().Width != 100 {
		t.Fatal("声明式地图配置丢失")
	}
	SourceView().Update(ctx, w)
	if !w.ShowMinimap() || w.MinimapOptions().MaxColumns != 80 {
		t.Fatal("省略配置错误地覆盖现状")
	}
	SourceView().ShowMinimap(false).MinimapOptions(MinimapOptions{}).Update(ctx, w)
	if w.ShowMinimap() || w.MinimapOptions().Width != 120 {
		t.Fatal("显式关闭或默认配置未生效")
	}
}

// 快照和文档均由 ui 暴露，不要求声明式使用方混用命令式包类型。
func TestExternalHighlightsAliases(t *testing.T) {
	d := NewDocument("value")
	var span HighlightSpan
	span.Range = TextRange{End: 5}
	span.Kind = "analysis.value"
	var h *Highlights
	h, err := NewHighlights("value", []HighlightSpan{span})
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := d.SetHighlights(d.Model().Revision(), h); !ok || err != nil {
		t.Fatal(ok, err)
	}
}
