package sourceview

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/golang-gui/goui-sourceview/internal/controls"
	textcore "github.com/golang-gui/goui-sourceview/internal/text"
	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/platform/events"
)

func stateFixture(t *testing.T) (*SourceView, *shapedContext) {
	t.Helper()
	v := NewSourceView()
	v.SetDocument(NewDocument(strings.Repeat("line\n", 100)))
	observe(t, v)
	ctx := &shapedContext{}
	v.context = ctx
	t.Cleanup(v.dropLayouts)
	return v, ctx
}

func TestViewStateFoldingDelayedExactMatchAndIsolation(t *testing.T) {
	v := foldingFixture(t)
	d := v.document
	v.SetSelection(TextSelection{})
	if !v.Fold(1) {
		t.Fatal("初始折叠失败")
	}
	s := saveView(t, v)
	v.UnfoldAll()
	if len(s.folds) != 1 {
		t.Fatal("快照随活视图展开而改变")
	}
	b := NewSourceView()
	b.SetDocument(d)
	observe(t, b)
	// 新视图先没有候选，随后才收到外部分析。
	d.externalFolds, d.foldReady, d.foldSnapshot = true, false, nil
	if err := b.RestoreState(s); err != nil || !b.restoreFoldsPending() {
		t.Fatal("未保留等待分析的折叠意图", err)
	}
	copy := saveView(t, b)
	if !slices.Equal(copy.folds, s.folds) {
		t.Fatal("快速切换标签丢失尚未应用的折叠")
	}
	f, err := NewFolds(d.model.Text(), []FoldRange{{StartLine: 0, EndLine: 5}, {StartLine: 1, EndLine: 4}})
	if err != nil {
		t.Fatal(err)
	}
	d.SetFolds(d.model.Revision(), f)
	finishFoldIntake(t, b)
	finishFoldIntake(t, v)
	if b.restoreFoldsPending() || !b.folds[1].Collapsed || b.folds[0].Collapsed || v.folds[1].Collapsed {
		t.Fatal("端点精确匹配或视图隔离失败", b.folds, v.folds)
	}
	b.cancelViewRestore()
	// 不同终点的候选不可按标题行匹配。
	different, _ := NewFolds(d.model.Text(), []FoldRange{{StartLine: 1, EndLine: 3}})
	d.SetFolds(d.model.Revision(), different)
	finishFoldIntake(t, b)
	if err := b.RestoreState(s); err != nil {
		t.Fatal(err)
	}
	finishFoldIntake(t, b)
	if b.folds[0].Collapsed {
		t.Fatal("不同终点的区域被错误折叠")
	}
	b.cancelViewRestore()
}

func TestViewStateLateFoldsCannotOverrideUserOrHideSelection(t *testing.T) {
	v := foldingFixture(t)
	v.SetSelection(TextSelection{})
	v.Fold(1)
	s := saveView(t, v)
	v.UnfoldAll()
	if err := v.RestoreState(s); err != nil {
		t.Fatal(err)
	}
	v.SetSelection(TextSelection{Anchor: 10, Caret: 10})
	finishFoldIntake(t, v)
	if v.viewRestore != nil || v.folds[1].Collapsed {
		t.Fatal("迟到折叠覆盖了用户选区")
	}
	// 即使旧快照来自较宽视图，也不能将新选区隐藏在区域体内。
	protected := *s
	protected.selection = TextSelection{Anchor: 10, Caret: 10}
	if err := v.RestoreState(&protected); err != nil {
		t.Fatal(err)
	}
	finishFoldIntake(t, v)
	if v.folds[1].Collapsed || v.Selection() != protected.selection {
		t.Fatal("恢复折叠隐藏或移动了选区")
	}
	v.cancelViewRestore()
	v.UnfoldAll()
	if err := v.RestoreState(s); err != nil {
		t.Fatal(err)
	}
	// 即使当前已展开，显式 Unfold 也应阻止待恢复的折叠。
	v.Unfold(1)
	finishFoldIntake(t, v)
	if v.viewRestore != nil || v.folds[1].Collapsed {
		t.Fatal("无变化的展开命令仍被旧恢复请求覆盖")
	}
}

func TestViewStateLanguageAndFoldingConfiguration(t *testing.T) {
	for _, change := range []string{"language", "disabled"} {
		t.Run(change, func(t *testing.T) {
			v := foldingFixture(t)
			v.SetSelection(TextSelection{})
			v.Fold(1)
			s := saveView(t, v)
			if change == "language" {
				if err := v.document.SetLanguage(language.XML); err != nil {
					t.Fatal(err)
				}
			} else {
				v.SetFoldingEnabled(false)
			}
			if err := v.RestoreState(s); err != nil || v.restoreFoldsPending() || len(v.collapsedFolds) != 0 {
				t.Fatal("恢复改变了语言或折叠配置", err)
			}
			v.cancelViewRestore()
		})
	}
}

func TestViewStateCollapsedIndexTracksEditsAndReplacement(t *testing.T) {
	v := foldingFixture(t)
	v.SetSelection(TextSelection{})
	v.Fold(1)
	old := saveView(t, v)
	if err := v.document.model.ReplaceAtomic(TextRange{}, "prefix\n"); err != nil {
		t.Fatal(err)
	}
	s := saveView(t, v)
	if len(s.folds) != 1 || s.folds[0].Start != old.folds[0].Start+7 || s.folds[0].End != old.folds[0].End+7 {
		t.Fatal("编辑映射没有更新已折叠索引", s.folds)
	}
	f, _ := NewFolds(v.document.model.Text(), nil)
	v.document.SetFolds(v.document.model.Revision(), f)
	finishFoldIntake(t, v)
	if len(saveView(t, v).folds) != 0 || len(v.collapsedFolds) != 0 {
		t.Fatal("替换候选后快照仍含已移除的折叠")
	}
}

func TestViewStateEmptyIntentClearsMappedExternalFolds(t *testing.T) {
	v := foldingFixture(t)
	d := v.document
	f, err := NewFolds(d.model.Text(), []FoldRange{{StartLine: 1, EndLine: 4}})
	if err != nil {
		t.Fatal(err)
	}
	d.SetFolds(d.model.Revision(), f)
	finishFoldIntake(t, v)
	v.SetSelection(TextSelection{})
	v.Fold(1)
	if err := d.model.ReplaceAtomic(TextRange{}, "prefix\n"); err != nil {
		t.Fatal(err)
	}
	if !d.foldReady || d.foldSnapshot != nil || len(v.collapsedFolds) != 1 {
		t.Fatal("未建立外部分析失效、映射折叠仍可用的状态")
	}
	b := NewSourceView()
	b.SetDocument(d)
	observe(t, b)
	empty := saveView(t, b)
	if err := v.RestoreState(empty); err != nil {
		t.Fatal(err)
	}
	finishFoldIntake(t, v)
	if len(v.collapsedFolds) != 0 {
		t.Fatal("空折叠意图未清空映射后保留的折叠")
	}
	v.cancelViewRestore()
}

func TestViewStateReentrantSearchNotificationKeepsNewestRestore(t *testing.T) {
	v, _ := stateFixture(t)
	a := saveView(t, v)
	v.SetSelection(TextSelection{Caret: 9})
	b := saveView(t, v)
	called := false
	h := v.searchState.Connect(func() {
		if called {
			return
		}
		called = true
		if err := v.RestoreState(b); err != nil {
			t.Error(err)
		}
	})
	defer h.Disconnect()
	if err := v.RestoreState(a); !errors.Is(err, ErrOperationChanged) || v.viewRestore == nil || v.viewRestore.snapshot.selection != b.selection {
		t.Fatal("搜索通知回调中的新恢复被外层覆盖", err)
	}
	v.cancelViewRestore()
}

func TestViewStateManualLineCollapseAndNarrowDestination(t *testing.T) {
	v, ctx := collapsedLineFixture(t, strings.Repeat("word ", 300)+"\ntail")
	if !v.CollapseLine(0) {
		t.Fatal("未收起长行")
	}
	v.SetSelection(TextSelection{Anchor: 30, Caret: 30})
	s := saveView(t, v)
	if len(s.lines) != 1 {
		t.Fatal("快照遗漏手动长行")
	}
	v.ExpandLine(0)
	created := ctx.created
	if err := v.RestoreState(s); err != nil || ctx.created != created || !v.manuallyCollapsed(0) {
		t.Fatal("恢复丢失长行或同步测量", err)
	}
	v.LayoutVisible(geometry.Size{Width: 400, Height: 180}, geometry.Point{})
	if v.viewRestore != nil || !v.manuallyCollapsed(0) {
		t.Fatal("相同宽度没有完成恢复")
	}
	v.ExpandLine(0)
	v.Arrange(geometry.Rect(0, 0, 100, 180))
	if err := v.RestoreState(s); err != nil {
		t.Fatal(err)
	}
	for range 4 {
		v.protectRestoredSelection(func(row int) bool { return v.paragraph(row) != nil })
		v.LayoutVisible(geometry.Size{Width: 100, Height: 180}, geometry.Point{})
	}
	if v.manuallyCollapsed(0) || v.Selection() != s.selection || v.viewRestore != nil {
		t.Fatal("较窄宽度留下隐藏选区或移动了光标")
	}
}

func TestViewStateFoldLimitAndBudgetedIntake(t *testing.T) {
	const count = 100000
	d := NewDocument(strings.Repeat("a\nb\n", count))
	ranges := make([]FoldRange, count)
	for i := range ranges {
		ranges[i] = FoldRange{StartLine: i * 2, EndLine: i*2 + 1}
	}
	f, err := NewFolds(d.model.Text(), ranges)
	if err != nil {
		t.Fatal(err)
	}
	v := NewSourceView()
	v.SetDocument(d)
	observe(t, v)
	d.SetFolds(d.model.Revision(), f)
	finishFoldIntake(t, v)
	v.Fold(100)
	s := saveView(t, v)
	if len(s.folds) != 1 {
		t.Fatal("候选全集被写入快照")
	}
	v.UnfoldAll()
	if err := v.RestoreState(s); err != nil || len(v.foldIntake.entries) != 0 {
		t.Fatal("恢复同步遍历全部候选", err)
	}
	v.foldingIntakeStep()
	if v.foldIntake == nil || len(v.foldIntake.entries) > 256 {
		t.Fatal("单步匹配没有遵守候选预算")
	}
	finishFoldIntake(t, v)
	if len(v.collapsedFolds) != 1 || !v.folds[50].Collapsed {
		t.Fatal("分批匹配丢失折叠意图")
	}
	v.cancelViewRestore()
	// 批量建立边界状态，再用正式折叠命令跨过上限，验证索引一致性。
	for i := range v.folds {
		v.folds[i].Collapsed = i < maxSavedFolds
	}
	v.refreshFoldGeometry()
	if s, err := v.SaveState(); err != nil || len(s.folds) != maxSavedFolds {
		t.Fatal("4096 个折叠被拒绝", err)
	}
	v.Fold(maxSavedFolds * 2)
	if _, err := v.SaveState(); !errors.Is(err, ErrOperationLimit) {
		t.Fatal("超限快照没有明确失败", err)
	}
}

func saveView(t *testing.T, v *SourceView) *ViewState {
	t.Helper()
	s, err := v.SaveState()
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestViewStateSelectionAndViewportRoundTrip(t *testing.T) {
	v, ctx := stateFixture(t)
	selection := TextSelection{Anchor: 43, Caret: 5}
	v.SetSelection(selection)
	v.upstream = true
	size := geometry.Size{Width: 300, Height: 100}
	v.LayoutVisible(size, geometry.Point{})
	for row := 0; row < v.document.model.LineCount(); row++ {
		v.paragraph(row)
	}
	v.LayoutVisible(size, geometry.Point{Y: 407})
	created := ctx.created
	s := saveView(t, v)
	if ctx.created != created || s.selection != selection || !s.upstream || !s.anchor.valid {
		t.Fatal("保存改变了布局或未保留精确文本锚点", s.anchor, s.selection, ctx.created, created)
	}
	v.SetSelection(TextSelection{})
	v.LayoutVisible(size, geometry.Point{})
	v.SetReadOnly(true)
	created = ctx.created
	beforeControllers := len(v.EventControllers())
	if err := v.RestoreState(s); err != nil {
		t.Fatal(err)
	}
	if ctx.created != created || v.Selection() != selection || !v.upstream || !v.pendingAnchor || v.reveal != nil {
		t.Fatal("恢复同步测量了正文或丢失反向选区/亲和性")
	}
	if len(v.EventControllers()) != beforeControllers+1 {
		t.Fatal("恢复未持有输入取消连接")
	}
	var target geometry.Rectangle
	h := v.ConnectScrollIntoView(func(r geometry.Rectangle) { target = r })
	defer h.Disconnect()
	v.LayoutVisible(size, geometry.Point{})
	if target.Y != 407 || v.offset.Y != 407 || v.pendingAnchor || v.viewRestore != nil || len(v.EventControllers()) != beforeControllers {
		t.Fatal("恢复未定位原视口或未释放恢复任务", target, v.offset)
	}
	if v.document.model.Revision() != 0 || v.document.model.CanUndo() || v.Focused() {
		t.Fatal("恢复修改了正文历史或获取焦点")
	}
}

func TestViewStateEmptyAndHorizontalViewport(t *testing.T) {
	for _, text := range []string{"", strings.Repeat("x", 120)} {
		t.Run(fmt.Sprintf("bytes=%d", len(text)), func(t *testing.T) {
			v, _ := stateFixture(t)
			v.SetDocument(NewDocument(text))
			size := geometry.Size{Width: 300, Height: 100}
			v.LayoutVisible(size, geometry.Point{})
			x := float32(0)
			if text != "" {
				x = 70
			}
			v.LayoutVisible(size, geometry.Point{X: x})
			s := saveView(t, v)
			v.LayoutVisible(size, geometry.Point{})
			if err := v.RestoreState(s); err != nil {
				t.Fatal(err)
			}
			v.LayoutVisible(size, geometry.Point{})
			if v.viewRestore != nil || v.offset.X != x {
				t.Fatal("空文档或横向位置没有完成恢复", v.offset)
			}
			if err := v.RestoreState(s); err != nil {
				t.Fatal(err)
			}
			v.LayoutVisible(geometry.Size{Width: 2000, Height: 100}, geometry.Point{})
			if v.viewRestore != nil || v.offset.X != 0 {
				t.Fatal("横向位置没有按新视口钳制", v.offset)
			}
		})
	}
}

func TestViewStateRejectsInvalidAndStaleWithoutSideEffects(t *testing.T) {
	unmounted := NewSourceView()
	if _, err := unmounted.SaveState(); !errors.Is(err, ErrNotMounted) {
		t.Fatal("未挂载视图保存成功", err)
	}
	v, _ := stateFixture(t)
	s := saveView(t, v)
	if err := unmounted.RestoreState(s); !errors.Is(err, ErrNotMounted) {
		t.Fatal(err)
	}
	if err := v.RestoreState(s); err != nil {
		t.Fatal(err)
	}
	request := v.viewRestore
	for _, invalid := range []*ViewState{nil, {}, {document: s.document + 1}, {document: s.document, revision: s.revision + 1}} {
		if err := v.RestoreState(invalid); err == nil || v.viewRestore != request {
			t.Fatal("无效请求替换了已有恢复任务", err)
		}
	}
	if err := v.document.model.ReplaceAtomic(TextRange{}, "x"); err != nil {
		t.Fatal(err)
	}
	before := v.Selection()
	if err := v.RestoreState(s); !errors.Is(err, ErrOperationChanged) || v.Selection() != before || v.viewRestore != nil {
		t.Fatal("过期快照覆盖了当前状态", err)
	}
}

func TestViewStateExplicitActionsCancelPendingRestore(t *testing.T) {
	for _, action := range []string{"selection", "reveal", "key", "document", "unmount"} {
		t.Run(action, func(t *testing.T) {
			v, _ := stateFixture(t)
			s := saveView(t, v)
			before := len(v.EventControllers())
			if err := v.RestoreState(s); err != nil {
				t.Fatal(err)
			}
			switch action {
			case "selection":
				v.SetSelection(TextSelection{Caret: 3})
			case "reveal":
				v.RevealSelection()
			case "key":
				v.inputKey(events.KeyEvent{EventType: events.KeyDown, Key: events.KeyEscape})
			case "document":
				v.SetDocument(NewDocument("other"))
			case "unmount":
				v.unmount()
			}
			if v.viewRestore != nil || v.pendingAnchor || len(v.EventControllers()) != before {
				t.Fatal("显式操作仍保留旧恢复任务或控制器")
			}
		})
	}
}

func TestViewStateReentrantCompletionCloseKeepsNewestRestore(t *testing.T) {
	v, _ := stateFixture(t)
	v.SetSelection(TextSelection{Caret: 5})
	a := saveView(t, v)
	v.SetSelection(TextSelection{Caret: 9})
	b := saveView(t, v)
	v.SetSelection(TextSelection{})
	v.completion = &completionSession{list: &controls.CompletionList{}, popup: closingPopup{onClose: func() {
		if err := v.RestoreState(b); err != nil {
			t.Error(err)
		}
	}}}
	if err := v.RestoreState(a); !errors.Is(err, ErrOperationChanged) {
		t.Fatal("外层恢复未识别回调替换", err)
	}
	if v.Selection() != b.selection || v.viewRestore == nil || v.viewRestore.snapshot.selection != b.selection {
		t.Fatal("外层恢复覆盖了回调中的新请求")
	}
	v.cancelViewRestore()
}

func TestViewStateRequestOwnsSubmittedValue(t *testing.T) {
	v, _ := stateFixture(t)
	v.SetSelection(TextSelection{Caret: 7})
	s := saveView(t, v)
	if err := v.RestoreState(s); err != nil {
		t.Fatal(err)
	}
	*s = ViewState{}
	if v.viewRestore.snapshot.document != v.document.ID() || v.viewRestore.snapshot.selection.Caret != 7 {
		t.Fatal("调用方重置快照变量改变了已接受的请求")
	}
	v.cancelViewRestore()
}

func TestViewStateUnmeasuredAndPreeditCapture(t *testing.T) {
	v, ctx := stateFixture(t)
	v.context = nil
	v.SetSelection(TextSelection{Anchor: 17, Caret: 12})
	v.context = ctx
	s := saveView(t, v)
	if s.anchor.offset != 10 || ctx.created != 0 {
		t.Fatal("未测量视图没有回退到光标所在逻辑行，或分配了原生布局")
	}
	v.anchor = viewportAnchor{offset: 25, inset: 3, x: 7, valid: true}
	v.anchorRevision = v.observed
	projection := textcore.NewProjection(v.document.model, v.Selection())
	projection.Update("uncommitted\n", 2)
	v.projection = projection
	s = saveView(t, v)
	if v.projection != projection || s.anchor != v.anchor || s.selection != v.Selection() || ctx.created != 0 {
		t.Fatal("保存提交或改变了预编辑，或复制了投影坐标")
	}
	v.projection = nil
}
