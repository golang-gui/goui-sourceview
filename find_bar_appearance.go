package sourceview

import (
	"image/color"
	"strings"

	"github.com/golang-gui/goui-sourceview/scheme"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/icon/svg"
	"github.com/golang-gui/goui/layout"
	"github.com/golang-gui/goui/mlang"
	"github.com/golang-gui/goui/platform/graphics"
	"github.com/golang-gui/goui/style"
	"github.com/golang-gui/goui/widgets"
)

// Original, monochrome line drawings; no icon font or external asset resources.
var findIcons = func() map[string]gui.IconSource {
	paths := map[string]string{
		"right": "M6 3l5 5-5 5", "down": "M3 6l5 5 5-5",
		"previous": "M8 13V3M4 7l4-4 4 4", "next": "M8 3v10M4 9l4 4 4-4",
		"close":       "M4 4l8 8M12 4l-8 8",
		"replace":     "M3 3h7a3 3 0 0 1 0 6H4m3-3L4 9l3 3M10 12h3v2h-3z",
		"replace-all": "M2 3h8a3 3 0 0 1 0 6H3m3-3L3 9l3 3M8 12h2v2H8zM12 12h2v2h-2z",
	}
	icons := make(map[string]gui.IconSource, len(paths))
	for name, path := range paths {
		icon, err := svg.Parse(strings.NewReader(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><path d="` + path + `" fill="none" stroke="currentColor" stroke-width="1.25" stroke-linecap="round" stroke-linejoin="round"/></svg>`))
		if err != nil {
			panic(err)
		}
		icons[name] = icon
	}
	return icons
}()

func (b *FindBar) buildSurface() {
	b.scope = gui.NewStyleScope()
	b.content = &findSurface{bar: b}
	b.content.SetLayoutManager(&findBarLayout{bar: b})
	b.content.SetVisible(false)
	b.scope.SetChild(b.content)
	b.WidgetBase.AddChild(b, b.scope)
	add := func(w gui.Widget) { b.content.WidgetBase.AddChild(b.content, w) }
	name := func(w gui.Widget, en, zh string) {
		getter := mlang.String(mlang.En(en), mlang.ZhCN(zh))
		b.names[w] = getter
		w.SetName(getter())
	}
	hint := func(en, zh string) *gui.Label {
		getter := mlang.String(mlang.En(en), mlang.ZhCN(zh))
		l := gui.NewLabel(getter())
		l.SetStyleName("sourceview.find.hint")
		b.labels[l] = getter
		add(l)
		return l
	}
	b.findHint = hint("Find", "查找")
	b.replaceHint = hint("Replace", "替换")
	action := func(icon, en, zh string, fn func()) *gui.Button {
		w := gui.NewButton()
		w.SetStyleName("sourceview.find.action")
		w.SetPadding(3)
		w.SetChild(gui.NewIcon(findIcons[icon]))
		name(w, en, zh)
		w.ConnectClicked(fn)
		add(w)
		return w
	}
	option := func(en, zh string, child gui.Widget, fn func(bool)) *widgets.CheckButton {
		w := widgets.NewCheckButton()
		w.SetAppearance(widgets.CheckAppearanceButton)
		w.SetPadding(2)
		w.SetStyleName("sourceview.find.option")
		w.SetChild(child)
		name(w, en, zh)
		w.ConnectChange(func(state widgets.CheckState) { fn(state == widgets.CheckChecked) })
		add(w)
		return w
	}
	b.toggleIcon = gui.NewIcon(findIcons["right"])
	b.toggle = option("Toggle replace", "展开替换", b.toggleIcon, b.SetReplaceMode)
	b.toggle.SetStyleName("sourceview.find.expand")
	b.find = gui.NewTextInput()
	b.find.SetName("Find")
	b.find.SetStyleName("sourceview.find.input")
	b.find.SetPadding(4)
	add(b.find)
	textOption := func(text string) gui.Widget {
		l := gui.NewLabel(text)
		l.SetStyleName("sourceview.find.option-text")
		return l
	}
	b.matchCase = option("Match case", "区分大小写", textOption("Aa"), func(checked bool) {
		if v := b.liveTarget(); v != nil {
			q := v.Search()
			q.IgnoreCase = !checked
			v.SetSearch(q)
		}
	})
	b.wholeWord = option("Whole word", "整词", textOption("ab"), func(checked bool) {
		if v := b.liveTarget(); v != nil {
			q := v.Search()
			q.WholeWord = checked
			v.SetSearch(q)
		}
	})
	b.status = gui.NewLabel("")
	b.status.SetStyleName("sourceview.find.status")
	add(b.status)
	b.previous = action("previous", "Previous", "上一项", func() { b.navigate(true) })
	b.next = action("next", "Next", "下一项", func() { b.navigate(false) })
	b.close = action("close", "Close", "关闭", b.Close)
	b.replacement = gui.NewTextInput()
	b.replacement.SetName("Replacement")
	b.replacement.SetStyleName("sourceview.find.input")
	b.replacement.SetPadding(4)
	add(b.replacement)
	b.replace = action("replace", "Replace", "替换", b.replaceCurrent)
	b.replaceAll = action("replace-all", "Replace all", "全部替换", b.replaceEverywhere)
	b.replacement.SetVisible(false)
	b.replace.SetVisible(false)
	b.replaceAll.SetVisible(false)
	b.replaceHint.SetVisible(false)
	b.replacement.ConnectText(func(text string) { b.replaceHint.SetVisible(b.replaceMode && text == "") })
	b.find.ConnectFocused(func(bool) { b.content.RequestPaint() })
	b.replacement.ConnectFocused(func(bool) { b.content.RequestPaint() })
	b.updateAppearance(nil)
}

func (b *FindBar) updateAppearance(v *SourceView) {
	s := scheme.Light()
	if v != nil {
		s = v.colorScheme()
	}
	if s == b.paletteScheme {
		return
	}
	b.paletteScheme = s
	d := s.Definition()
	c := d.Find
	b.findColors = c
	// A complete sheet is confined to this explicitly owned subtree. Layout and
	// behavior remain on the native controls; application styles stay unchanged.
	rules := gui.DefaultStyleRules()
	rules = append(rules,
		style.Name("sourceview.find.input").FontSize(9.75).ForegroundColor(c.Foreground).BackgroundColor(transparent).BorderWidth(0),
		style.Name("sourceview.find.input").State(style.Focused).BorderWidth(0),
		style.Name("sourceview.find.input").State(style.Disabled).ForegroundColor(c.MutedForeground),
		style.Name("sourceview.find.input").Part("selection").BackgroundColor(d.SelectionBackground).ForegroundColor(d.SelectionForeground),
		style.Name("sourceview.find.input").Part("caret").ForegroundColor(c.Foreground),
		style.Name("sourceview.find.action").BackgroundColor(transparent).BorderWidth(0).Radius(3),
		style.Name("sourceview.find.action").State(style.Hovered).BackgroundColor(c.HoverBackground),
		style.Name("sourceview.find.action").State(style.Pressed).BackgroundColor(c.OptionBackground),
		style.Name("sourceview.find.action").Part("focus").State(style.Focused).BackgroundColor(transparent).BorderWidth(0),
		style.Name("sourceview.find.action").Part("focus").State(style.FocusVisible).BorderColor(c.FocusBorder).BorderWidth(1).Radius(3),
		style.Name("icon").ForegroundColor(c.Foreground), style.Name("icon").State(style.Disabled).ForegroundColor(c.MutedForeground),
		style.Name("sourceview.find.option-text").FontSize(9).ForegroundColor(c.Foreground),
		style.Name("sourceview.find.option-text").State(style.Disabled).ForegroundColor(c.MutedForeground),
		style.Name("sourceview.find.status").FontSize(9).ForegroundColor(c.MutedForeground),
		style.Name("sourceview.find.hint").FontSize(9.75).ForegroundColor(c.MutedForeground),
		style.Name("sourceview.find.error").FontSize(9).ForegroundColor(c.ErrorForeground),
	)
	for _, name := range []string{"sourceview.find.option", "sourceview.find.expand"} {
		for _, part := range []string{"button", "button-checked"} {
			bg, border := transparent, transparent
			if part == "button-checked" && name != "sourceview.find.expand" {
				bg = c.OptionBackground
				border = c.OptionBorder
			}
			rules = append(rules, style.Name(name).Part(part).BackgroundColor(bg).BorderColor(border).BorderWidth(1).Radius(3).FontSize(9),
				style.Name(name).Part(part).State(style.Hovered).BackgroundColor(c.HoverBackground),
				style.Name(name).Part(part).State(style.Pressed).BackgroundColor(c.OptionBackground))
		}
		rules = append(rules, style.Name(name).Part("focus").State(style.FocusVisible).BorderColor(c.FocusBorder).BorderWidth(1).Radius(3))
	}
	b.scope.SetStyleSheet(style.Sheet(rules...))
	b.content.RequestPaint()
}

var transparent = color.NRGBA{} // Premultiplied zero also implements color.Color.

// The surface reserves eight DIP for a soft shadow, inside its ordinary clip.
// Its children remain ordinary widgets in the main window's event/focus tree.
type findSurface struct {
	gui.WidgetBase
	bar *FindBar
}

func (s *findSurface) Paint(p gui.Painter) {
	b := s.bar
	c := b.findColors
	panel := geometry.Rect(0, 0, s.Rect().Width, s.Rect().Height).Inset(8)
	p.DrawBoxShadow(panel, 4, graphics.BoxShadow{Color: graphics.ColorOf(c.Shadow), Offset: geometry.Point{Y: 2}, BlurRadius: 6})
	p.FillRoundRect(panel, 4, graphics.ColorOf(c.Background))
	p.DrawRoundRect(panel.Inset(.5), 3.5, 1, graphics.ColorOf(c.Border))
	input := func(r geometry.Rectangle, focused bool) {
		if r.Width <= 0 || r.Height <= 0 {
			return
		}
		border := c.InputBorder
		if focused {
			border = c.FocusBorder
		}
		p.FillRoundRect(r, 2, graphics.ColorOf(c.InputBackground))
		p.DrawRoundRect(r.Inset(.5), 1.5, 1, graphics.ColorOf(border))
	}
	input(b.fieldRect, b.find.Focused())
	if b.replaceMode {
		input(b.replacementRect, b.replacement.Focused())
	}
}

// Layout addresses ordinary visible children by identity. Placeholder labels
// paint behind the native inputs and never intercept their pointer input.
type findBarLayout struct {
	bar       *FindBar
	rowHeight float32
}

// Density changes precede WidgetBase's visible-child snapshot.
func (s *findSurface) Measure(c layout.Constraint) layout.Measurement {
	b := s.bar
	width := min(float32(448), c.Max.Width)
	narrow, reduced := width < 285, width < 400
	if b.narrow != narrow || b.reduced != reduced {
		b.narrow, b.reduced = narrow, reduced
		b.status.SetVisible(!reduced)
		for _, w := range []gui.Widget{b.matchCase, b.wholeWord, b.previous, b.next} {
			w.SetVisible(!narrow)
		}
		b.replace.SetVisible(b.replaceMode && !narrow)
		b.replaceAll.SetVisible(b.replaceMode && !narrow)
	}
	return s.WidgetBase.Measure(c)
}
func (l *findBarLayout) slots(children []layout.Child) map[gui.Widget]layout.Child {
	slots := make(map[gui.Widget]layout.Child, len(children))
	i := 0
	for _, w := range l.bar.content.Children() {
		if w.Visible() {
			slots[w] = children[i]
			i++
		}
	}
	return slots
}
func (l *findBarLayout) Measure(children []layout.Child, c layout.Constraint) layout.Measurement {
	b := l.bar
	width := min(float32(448), c.Max.Width)
	slots := l.slots(children)
	l.rowHeight = max(float32(26), slots[b.find].Measure(layout.Loose(geometry.Size{Width: max(0, width-80), Height: layout.Inf})).Height)
	if b.replaceMode {
		l.rowHeight = max(l.rowHeight, slots[b.replacement].Measure(layout.Loose(geometry.Size{Width: max(0, width-80), Height: layout.Inf})).Height)
	}
	rows := float32(1)
	if b.replaceMode {
		rows = 2
	}
	height := 16 + 10 + rows*l.rowHeight + (rows-1)*4
	// Measure all visible descendants through the framework before arrangement.
	for _, child := range children {
		child.Measure(layout.Loose(geometry.Size{Width: width, Height: height}))
	}
	return layout.Measured(c.Clamp(geometry.Size{Width: width, Height: height}))
}
func (l *findBarLayout) Arrange(children []layout.Child, r geometry.Rectangle) {
	b := l.bar
	h := l.rowHeight
	slots := l.slots(children)
	widgets := []gui.Widget{b.toggle, b.find, b.matchCase, b.wholeWord, b.status, b.previous, b.next, b.close, b.replacement, b.replace, b.replaceAll, b.findHint, b.replaceHint}
	x, y, right := float32(13), float32(13), max(float32(13), r.Width-13)
	place := func(i int, x, y, w, h float32) {
		if child := slots[widgets[i]]; child != nil {
			child.Arrange(geometry.Rect(x, y, max(0, w), max(0, h)))
		}
	}
	// Allocate from the right so actions retain their targets at narrow widths.
	take := func(i int) {
		right -= 22
		place(i, max(x, right), y+(h-22)/2, min(float32(22), max(0, right+22-x)), 22)
		right -= 2
	}
	take(7)
	if !b.narrow {
		take(6)
		take(5)
	}
	if !b.reduced {
		right -= 76
		height := slots[b.status].Measure(layout.Loose(geometry.Size{Width: 74, Height: h})).Height
		place(4, right, y+(h-height)/2, 74, height)
		right -= 4
	}
	place(0, x, y+(h-22)/2, 18, 22)
	x += 21
	b.fieldRect = geometry.Rect(x, y, max(0, right-x), h)
	inputRight := right
	if !b.narrow {
		inputRight -= 46
		place(2, inputRight+1, y+(h-22)/2, 22, 22)
		place(3, inputRight+23, y+(h-22)/2, 22, 22)
	}
	place(1, x+1, y+1, max(0, inputRight-x-2), h-2)
	place(11, x+5, y+5, max(0, inputRight-x-8), h-8)
	if b.replaceMode {
		y += h + 4
		b.replacementRect = geometry.Rect(x, y, b.fieldRect.Width, h)
		place(8, x+1, y+1, max(0, b.fieldRect.Width-2), h-2)
		place(12, x+5, y+5, max(0, b.fieldRect.Width-8), h-8)
		// Replace actions align with the leading edge of the count/action area.
		if !b.narrow {
			place(9, right+4, y+(h-22)/2, 22, 22)
			place(10, right+28, y+(h-22)/2, 22, 22)
		}
	}
}
