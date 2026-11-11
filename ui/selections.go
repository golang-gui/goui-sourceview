package ui

import (
	base "github.com/golang-gui/goui/ui"
	"slices"
)

// Selections declares the complete primary-first set and removes either read
// binding and generated writeback. Explicit event callbacks are preserved.
func (v *SourceViewView) Selections(values []TextSelection) *SourceViewView {
	v.selection, v.selectionSource = nil, nil
	if v.fields.Check(writeSelection) {
		v.onSelection = nil
		v.fields.Set(writeSelection, false)
	}
	v.selections = slices.Clone(values)
	v.selectionsSet = true
	v.selectionsSource = nil
	if v.selectionsWrite {
		v.onSelections = nil
		v.selectionsWrite = false
	}
	return v
}
func (v *SourceViewView) BindSelections(source base.BindState[[]TextSelection]) *SourceViewView {
	if source == nil {
		return v
	}
	v.selection, v.selectionSource = nil, nil
	if v.fields.Check(writeSelection) {
		v.onSelection = nil
		v.fields.Set(writeSelection, false)
	}
	v.selections = nil
	v.selectionsSet = false
	v.selectionsSource = source
	v.onSelections = func(values []TextSelection) { source.Set(slices.Clone(values)) }
	v.selectionsWrite = true
	return v
}
func (v *SourceViewView) OnSelections(fn func([]TextSelection)) *SourceViewView {
	v.onSelections = fn
	v.selectionsWrite = false
	return v
}

func (v *SourceViewView) SelectionModifiers(value SelectionModifiers) *SourceViewView {
	v.selectionModifiers = &value
	return v
}
