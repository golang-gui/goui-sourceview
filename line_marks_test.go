package sourceview

import (
	"testing"

	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
)

// 未挂载文档也必须跟踪；删除失效与业务状态不同，撤销不猜测恢复。
func TestLineMarksTrackDetachedDocument(t *testing.T) {
	d := NewDocument("甲\nb\nc")
	id, err := d.AddLineMark(LineMark{Line: 1, Label: "stop"})
	if err != nil {
		t.Fatal(err)
	}
	apply := func(start, end int, text string) {
		t.Helper()
		if err := d.Model().Apply(textedit.Edit{Range: textedit.Range{Start: start, End: end}, Text: text}, textedit.EditOptions{Atomic: true}); err != nil {
			t.Fatal(err)
		}
	}
	check := func(line int, valid bool) {
		t.Helper()
		got := d.LineMarks()
		if len(got) != 1 || got[0].ID != id || got[0].Line != line || got[0].Valid != valid {
			t.Fatalf("marks=%+v want line=%d valid=%t", got, line, valid)
		}
	}
	apply(4, 4, "new\n") // 锚点右重力，跟随原来的 b。
	check(2, true)
	d.Model().Undo()
	check(1, true)
	d.Model().Redo()
	check(2, true)
	apply(8, 10, "")
	check(-1, false)
	d.Model().Undo()
	check(-1, false)
	if err := d.UpdateLineMark(id, LineMark{Line: 2, Kind: MarkLogpoint, State: MarkPending}); err != nil {
		t.Fatal(err)
	}
	check(2, true)
	d.Model().SetText("new file")
	check(-1, false)
	if !d.RemoveLineMark(id) || d.RemoveLineMark(id) || d.markConnection != nil {
		t.Fatal("remove did not release subscription")
	}
	next, _ := d.AddLineMark(LineMark{})
	if next <= id {
		t.Fatal("reused ID")
	}
}

// 通知重入时存在尚未分发的变更；新建锚点不能被旧通知重复移动。
func TestLineMarksReentrantChanges(t *testing.T) {
	d := NewDocument("a\nb\nc")
	var created LineMarkID
	h := d.Model().ConnectChange(func(c textedit.Change) {
		if c.Revision == 1 {
			_ = d.Model().Apply(textedit.Edit{Range: textedit.Range{}, Text: "y\n"}, textedit.EditOptions{})
			created, _ = d.AddLineMark(LineMark{Line: 3})
		}
	})
	defer h.Disconnect()
	id, _ := d.AddLineMark(LineMark{Line: 1})
	_ = d.Model().Apply(textedit.Edit{Range: textedit.Range{}, Text: "x\n"}, textedit.EditOptions{})
	for _, got := range d.LineMarks() {
		if !got.Valid || got.Line != 3 || got.ID != id && got.ID != created {
			t.Fatalf("bad reentrant mark %+v", got)
		}
	}
	changes := 0
	s := d.ConnectLineMarksChanged(func() {
		changes++
		if changes == 1 {
			d.RemoveLineMark(id)
		}
	})
	defer s.Disconnect()
	_ = d.UpdateLineMark(id, LineMark{Line: 0})
	if changes != 2 || len(d.LineMarks()) != 1 {
		t.Fatal("reentrant removal")
	}
}

func TestLineMarkValidationAndSnapshots(t *testing.T) {
	d := NewDocument("one\ntwo")
	before := d.Model().Revision()
	for _, mark := range []LineMark{{Line: -1}, {Line: 2}, {Kind: 255}, {State: 255}} {
		if _, err := d.AddLineMark(mark); err == nil {
			t.Fatalf("accepted %+v", mark)
		}
	}
	id, _ := d.AddLineMark(LineMark{Line: 1, Kind: MarkBookmark})
	if err := d.UpdateLineMark(id, LineMark{Line: 8}); err == nil {
		t.Fatal("accepted invalid update")
	}
	if err := d.UpdateLineMark(999, LineMark{}); err == nil {
		t.Fatal("accepted unknown ID")
	}
	a := d.LineMarks()
	a[0].Line = 0
	if d.LineMarks()[0].Line != 1 || d.Model().Revision() != before || d.Model().CanUndo() {
		t.Fatal("snapshot or history polluted")
	}
}

// 批处理坐标按顺序变化；覆盖终点不属于删除区间。
func TestLineAnchorSequentialEdits(t *testing.T) {
	c := textedit.Change{Edits: []textedit.Edit{
		{Range: textedit.Range{Start: 0, End: 2}, Text: ""},
		{Range: textedit.Range{Start: 0, End: 0}, Text: "abc"},
	}}
	if offset, valid := mapLineAnchor(2, true, c); offset != 3 || !valid {
		t.Fatalf("got %d %t", offset, valid)
	}
}

// 同文档视图共享行标记，执行行及选区仍相互独立。
func TestLineMarkViewsAndExecution(t *testing.T) {
	d := NewDocument("a\nb\nc\nd")
	a, b := NewSourceView(), NewSourceView()
	a.SetDocument(d)
	b.SetDocument(d)
	observe(t, a)
	observe(t, b)
	a.SetShowLineMarks(true)
	a.SetShowLineNumbers(false)
	a.viewport = geometry.Size{Width: 240, Height: 160}
	a.resetHeights()
	a.updateGutter()
	if a.gutter != a.markColumn || a.gutter < 24 || b.ShowLineMarks() {
		t.Fatal("independent gutter options")
	}
	id, _ := d.AddLineMark(LineMark{Line: 1})
	if len(b.Document().LineMarks()) != 1 {
		t.Fatal("marks not shared")
	}
	if err := a.SetExecutionLine(1); err != nil {
		t.Fatal(err)
	}
	if _, ok := b.ExecutionLine(); ok {
		t.Fatal("execution leaked")
	}
	_ = d.Model().Apply(textedit.Edit{Range: textedit.Range{}, Text: "x\n"}, textedit.EditOptions{})
	if line, ok := a.ExecutionLine(); !ok || line != 2 {
		t.Fatal("execution anchor", line, ok)
	}
	selection := a.Selection()
	events := 0
	h := a.ConnectLineMarkAction(func(e LineMarkEvent) {
		events++
		if e.Line != 2 || e.Document != d || len(e.Marks) != 1 || e.Marks[0].ID != id {
			t.Fatalf("event=%+v", e)
		}
	})
	a.requestLineMark(2, LineMarkToggle, geometry.Point{})
	h.Disconnect()
	a.requestLineMark(2, LineMarkToggle, geometry.Point{})
	if events != 1 || a.Selection() != selection || len(d.LineMarks()) != 1 {
		t.Fatal("request mutated state or leaked connection")
	}
	point := a.lineMarkRect(2).Pos
	point.X = 4
	point.Y += 3
	if row, ok := a.lineMarkHit(point); !ok || row != 2 || a.cursorAt(point) != gui.CursorPointing {
		t.Fatal("mark hit/cursor")
	}
	a.hiddenLines = []lineInterval{{start: 1, end: 4}}
	a.resetHeights()
	if marks := a.marksOnRow(0); len(marks) != 1 || marks[0].Line != 2 {
		t.Fatal("fold lost original mark line")
	}
	a.SetDocument(NewDocument("other"))
	if _, ok := a.ExecutionLine(); ok {
		t.Fatal("document swap retained execution")
	}
}

// 无窗口期间错过的变更不能在下一次通知中错误复活执行位置。
func TestExecutionAnchorMissedRevision(t *testing.T) {
	v := NewSourceView()
	v.Document().Model().SetText("a\nb")
	_ = v.SetExecutionLine(1)
	_ = v.Document().Model().Apply(textedit.Edit{Range: textedit.Range{}, Text: "x\n"}, textedit.EditOptions{})
	observe(t, v)
	_ = v.Document().Model().Apply(textedit.Edit{Range: textedit.Range{}, Text: "y\n"}, textedit.EditOptions{})
	if _, ok := v.ExecutionLine(); ok {
		t.Fatal("revived stale anchor")
	}
}
