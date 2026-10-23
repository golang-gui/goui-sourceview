package sourceview

import (
	"testing"

	"github.com/golang-gui/goui/gui/textedit"
)

func TestGoToOffsetUsesOneBasedRuneColumns(t *testing.T) {
	m := textedit.NewModel("")
	m.SetText("abc\n中e\u0301x\n")
	for _, tc := range []struct {
		query string
		at    int
		valid bool
	}{{"1", 0, true}, {"2:2", 7, true}, {"2:4", 10, true}, {"3:1", 12, true}, {"0", 0, false}, {"2:6", 0, false}, {"1:0", 0, false}, {"1:2:3", 0, false}, {"no", 0, false}} {
		at, err := gotoOffset(m, tc.query)
		if (err == nil) != tc.valid || tc.valid && at != tc.at {
			t.Fatal(tc, at, err)
		}
	}
}

func TestGoToRetargetHonorsCloseCallback(t *testing.T) {
	bar := NewGoToBar()
	a, b, c := NewSourceView(), NewSourceView(), NewSourceView()
	bar.SetTarget(a)
	bar.open = true
	bar.ConnectOpen(func(open bool) {
		if !open {
			bar.SetTarget(c)
		}
	})
	bar.SetTarget(b)
	if bar.Target() != c {
		t.Fatal("outer setter overwrote callback target")
	}
}
