//go:build ignore

package main

// Multi-cursor walkthrough (all edits stay in memory):
//  1. Click a repeated field name below, then use Selection > Select all occurrences.
//  2. Type a new name. Undo restores every replacement in one step.
//  3. Select several rows and use Selection > Split selection into lines.
//  4. Try the column-drag gesture shown below the toolbar, or add a cursor below.
//     Short rows clamp to their real end; Tabs and combined characters stay whole.
var stock = []struct {
	product string
	amount  int
}{
	{product: "apples", amount: 12},
	{product: "bananas", amount: 24},
	{product: "中文 é", amount: 36},
}

// Drag a column over these aligned numbers, replace all three, then Undo.
var (
	north = 100
	south = 200
	west  = 300
)
