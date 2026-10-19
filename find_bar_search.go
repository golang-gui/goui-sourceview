package sourceview

import "github.com/golang-gui/goui/gui/textedit"

// Capture the range before navigating. A result selection never becomes the
// next search scope, and an empty selection is not an interactive scope.
func (b *FindBar) setSelectionOnly(checked bool) {
	if b.syncing {
		return
	}
	v := b.liveTarget()
	if v == nil {
		return
	}
	selection := v.Selection().Range()
	if checked && selection.Start == selection.End {
		b.refresh()
		return
	}
	state, epoch, query := v.operationState(), b.epoch, v.Search()
	v.cancelPreedit()
	if !state.valid(v) || b.epoch != epoch || b.liveTarget() != v || v.Search() != query {
		return
	}
	var scope *textedit.Range
	if checked {
		scope = &selection
	}
	if err := v.SetSearchRange(scope); err != nil {
		b.fail(err)
		return
	}
	// Search-state subscribers may close/retarget the bar or replace the
	// document/query/selection. Do not navigate their replacement state.
	if !state.valid(v) || b.epoch != epoch || b.liveTarget() != v || v.Search() != query {
		return
	}
	actual, limited := v.SearchRange()
	if limited != checked || checked && actual != selection {
		return
	}
	v.findFrom(selection.Start, false)
	b.refresh()
	b.countStep()
}
