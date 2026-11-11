package sourceview

import (
	"errors"
	"strings"
	"testing"

	"github.com/golang-gui/goui-sourceview/internal/controls"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui/textedit"
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
	selection := textedit.Selection{Anchor: 43, Caret: 5}
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
	v.SetSelection(textedit.Selection{})
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
	if err := v.document.model.ReplaceAtomic(textedit.Range{}, "x"); err != nil {
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
				v.SetSelection(textedit.Selection{Caret: 3})
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
	v.SetSelection(textedit.Selection{Caret: 5})
	a := saveView(t, v)
	v.SetSelection(textedit.Selection{Caret: 9})
	b := saveView(t, v)
	v.SetSelection(textedit.Selection{})
	v.completion = &completionSession{list: &controls.CompletionList{}, popup: closingPopup{onClose: func() {
		if err := v.RestoreState(b); err != nil {
			t.Error(err)
		}
	}}}
	if err := v.RestoreState(a); !errors.Is(err, ErrOperationChanged) {
		t.Fatal("外层恢复未识别回调替换", err)
	}
	if v.Selection() != b.selection || v.viewRestore == nil || v.viewRestore.snapshot != b {
		t.Fatal("外层恢复覆盖了回调中的新请求")
	}
	v.cancelViewRestore()
}

func TestViewStateUnmeasuredAndPreeditCapture(t *testing.T) {
	v, ctx := stateFixture(t)
	v.context = nil
	v.SetSelection(textedit.Selection{Anchor: 17, Caret: 12})
	v.context = ctx
	s := saveView(t, v)
	if s.anchor.offset != 10 || ctx.created != 0 {
		t.Fatal("未测量视图没有回退到光标所在逻辑行，或分配了原生布局")
	}
	v.anchor = viewportAnchor{offset: 25, inset: 3, x: 7, valid: true}
	v.anchorRevision = v.observed
	projection := textedit.NewProjection(v.document.model, v.Selection())
	projection.Update("uncommitted\n", 2)
	v.projection = projection
	s = saveView(t, v)
	if v.projection != projection || s.anchor != v.anchor || s.selection != v.Selection() || ctx.created != 0 {
		t.Fatal("保存提交或改变了预编辑，或复制了投影坐标")
	}
	v.projection = nil
}
