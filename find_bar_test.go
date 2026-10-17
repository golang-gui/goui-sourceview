package sourceview

import (
	"errors"
	"testing"
)

func TestDetachedFindBarBorrowsTargetWithoutSubscriptions(t *testing.T) {
	a, b := NewSourceView(), NewSourceView()
	bar := NewFindBar()
	initial := len(a.EventControllers())
	bar.SetTarget(a)
	bar.SetReplacement("replacement")
	bar.SetReplaceMode(true)
	if bar.Opened() || bar.Target() != a || bar.Replacement() != "replacement" || !bar.ReplaceMode() {
		t.Fatal("detached properties")
	}
	if err := bar.Open(false); !errors.Is(err, ErrNotMounted) {
		t.Fatal(err)
	}
	if len(a.EventControllers()) != initial || a.document.connection != nil || len(bar.attached) != 0 || len(bar.lifetime) != 0 {
		t.Fatal("detached companion subscribed")
	}
	bar.SetTarget(b)
	bar.SetTarget(nil)
	if bar.Target() != nil || len(a.EventControllers()) != initial {
		t.Fatal("retarget retained editor")
	}
}
