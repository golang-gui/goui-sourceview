package highlight

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/gui/textedit"
)

func TestLineStoreBlockAggregatesMatchEveryBytePosition(t *testing.T) {
	var parts []string
	for row := 0; row < 4096; row++ {
		parts = append(parts, strings.Repeat("x", row%37)+"中")
	}
	parts = append(parts, "") // The trailing empty paragraph is a legal endpoint.
	model := textedit.NewModel(strings.Join(parts, "\n"))
	engine := New(model, language.Plain)
	engine.Advance(time.Time{})
	if engine.Err() != nil || engine.Pending() || engine.LineCount() != len(parts) {
		t.Fatal("metadata incomplete", engine.Err())
	}
	start := 0
	for row, part := range parts {
		if got := engine.lines.offset(row); got != start {
			t.Fatalf("row %d offset=%d want=%d", row, got, start)
		}
		end := start + len(part)
		if row+1 < len(parts) {
			end++
		}
		for at := start; at < end; at++ {
			if got := engine.lines.lineAt(at); got != row {
				t.Fatalf("byte %d row=%d want=%d", at, got, row)
			}
		}
		start = end
	}
	if engine.lines.lineAt(model.Len()) != len(parts)-1 || engine.lines.offset(len(parts)) != model.Len() {
		t.Fatal("exclusive block/document endpoint mapping")
	}
}
