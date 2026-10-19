package sourceview

import (
	"strings"

	"github.com/golang-gui/goui-sourceview/internal/resource"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/icon/svg"
	"github.com/golang-gui/goui/layout"
	"github.com/golang-gui/goui/mlang"
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
		"selection":   "M3 3h10v10H3zM5 6h6M5 8h6M5 10h4",
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
	b.content = &findSurface{bar: b}
	b.content.SetLayoutManager(&findBarLayout{bar: b})
	b.content.SetVisible(false)
	b.WidgetBase.AddChild(b, b.content)
	add := func(w gui.Widget) { b.content.WidgetBase.AddChild(b.content, w) }
	name := func(w gui.Widget, getter mlang.Getter) {
		b.names[w] = getter
		w.SetName(getter())
	}
	action := func(icon string, getter mlang.Getter, fn func()) *gui.Button {
		w := gui.NewButton()
		w.SetStyleName("sourceview.find.action")
		w.SetPadding(3)
		i := gui.NewIcon(findIcons[icon])
		i.SetStyleName("sourceview.find.icon")
		w.SetChild(i)
		name(w, getter)
		w.ConnectClicked(fn)
		add(w)
		return w
	}
	option := func(getter mlang.Getter, child gui.Widget, fn func(bool)) *widgets.CheckButton {
		w := widgets.NewCheckButton()
		w.SetAppearance(widgets.CheckAppearanceButton)
		w.SetPadding(2)
		w.SetStyleName("sourceview.find.option")
		w.SetChild(child)
		name(w, getter)
		w.ConnectChange(func(state widgets.CheckState) { fn(state == widgets.CheckChecked) })
		add(w)
		return w
	}
	b.toggleIcon = gui.NewIcon(findIcons["right"])
	b.toggleIcon.SetStyleName("sourceview.find.icon")
	b.toggle = option(resource.ToggleReplace, b.toggleIcon, b.SetReplaceMode)
	b.toggle.SetStyleName("sourceview.find.expand")
	b.find = gui.NewTextView()
	b.find.SetPlaceholder(resource.Find())
	name(b.find, resource.Find)
	b.find.SetStyleName("sourceview.find.input")
	b.find.SetPadding(4)
	b.findScroll = gui.NewScrollView()
	b.findScroll.SetChild(b.find)
	add(b.findScroll)
	textOption := func(text string) gui.Widget {
		l := gui.NewLabel(text)
		l.SetStyleName("sourceview.find.option-text")
		return l
	}
	b.matchCase = option(resource.MatchCase, textOption("Aa"), func(checked bool) {
		if v := b.liveTarget(); v != nil {
			q := v.Search()
			q.IgnoreCase = !checked
			v.SetSearch(q)
		}
	})
	b.wholeWord = option(resource.WholeWord, textOption("ab"), func(checked bool) {
		if v := b.liveTarget(); v != nil {
			q := v.Search()
			q.WholeWord = checked
			v.SetSearch(q)
		}
	})
	b.regex = option(resource.RegularExpression, textOption(".*"), func(checked bool) {
		if v := b.liveTarget(); v != nil {
			q := v.Search()
			q.Regex = checked
			v.SetSearch(q)
		}
	})
	selectionIcon := gui.NewIcon(findIcons["selection"])
	selectionIcon.SetStyleName("sourceview.find.icon")
	b.selectionOnly = option(resource.InSelection, selectionIcon, func(checked bool) {
		if b.syncing {
			return
		}
		if v := b.liveTarget(); v != nil {
			if checked {
				r := v.Selection().Range()
				b.fail(v.SetSearchRange(&r))
			} else {
				b.fail(v.SetSearchRange(nil))
			}
		}
	})

	b.status = gui.NewLabel("")
	b.status.SetStyleName("sourceview.find.status")
	add(b.status)
	b.previous = action("previous", resource.Previous, func() { b.navigate(true) })
	b.next = action("next", resource.Next, func() { b.navigate(false) })
	b.close = action("close", resource.Close, b.Close)
	b.replacement = gui.NewTextView()
	b.replacement.SetPlaceholder(resource.Replace())
	name(b.replacement, resource.Replacement)
	b.replacement.SetStyleName("sourceview.find.input")
	b.replacement.SetPadding(4)
	b.replacementScroll = gui.NewScrollView()
	b.replacementScroll.SetChild(b.replacement)
	add(b.replacementScroll)
	b.replace = action("replace", resource.Replace, b.replaceCurrent)
	b.replaceAll = action("replace-all", resource.ReplaceAll, b.replaceEverywhere)
	b.replacementScroll.SetVisible(false)
	b.replacement.SetVisible(false)
	b.replace.SetVisible(false)
	b.replaceAll.SetVisible(false)
	b.replacement.ConnectChange(func(_ gui.TextChange) { b.RequestLayout() })
	b.find.ConnectChange(func(_ gui.TextChange) { b.RequestLayout() })
	b.find.ConnectScrollIntoView(func(geometry.Rectangle) { b.RequestLayout() })
	b.replacement.ConnectScrollIntoView(func(geometry.Rectangle) { b.RequestLayout() })
	b.find.ConnectFocused(func(bool) { b.content.RequestPaint() })
	b.replacement.ConnectFocused(func(bool) { b.content.RequestPaint() })
	b.updateAppearance()
}

type findAppearance struct{ surface, normal, focused, disabled, invalid styleBox }

func (b *FindBar) StyleChanged() { b.updateAppearance() }
func (b *FindBar) updateAppearance() {
	name := b.StyleName()
	if name == "" {
		name = "sourceview.find"
	}
	next := findAppearance{surface: boxOf(gui.ResolveWidgetStyle(b, name, "", style.Normal)),
		normal:   boxOf(gui.ResolveWidgetStyle(b, "sourceview.find.input", "frame", style.Normal)),
		focused:  boxOf(gui.ResolveWidgetStyle(b, "sourceview.find.input", "frame", style.Focused)),
		invalid:  boxOf(gui.ResolveWidgetStyle(b, "sourceview.find.input", "error-frame", style.Normal)),
		disabled: boxOf(gui.ResolveWidgetStyle(b, "sourceview.find.input", "frame", style.Disabled))}
	if b.appearance == next {
		return
	}
	b.appearance = next
	b.inset = max(float32(2), next.surface.outset())
	b.RequestLayout()
	b.RequestPaint()
}

// A surface's shadow is reserved inside ordinary widget bounds.
type findSurface struct {
	gui.WidgetBase
	bar *FindBar
}

func (s *findSurface) Paint(p gui.Painter) {
	b := s.bar
	b.appearance.surface.paint(p, geometry.Rect(0, 0, s.Rect().Width, s.Rect().Height).Inset(b.inset))
	input := func(r geometry.Rectangle, w *gui.TextView) {
		skin := b.appearance.normal
		if !gui.IsEnabled(w) {
			skin = b.appearance.disabled
		} else if w.Focused() {
			skin = b.appearance.focused
		}
		if w == b.find {
			if v := b.liveTarget(); v != nil && v.SearchError() != nil {
				skin = b.appearance.invalid
			}
		}
		skin.paint(p, r)
	}
	input(b.fieldRect, b.find)
	if b.replaceMode {
		input(b.replacementRect, b.replacement)
	}
}

// Layout addresses ordinary visible children by identity.
type findBarLayout struct {
	bar         *FindBar
	rowHeight   float32
	statusWidth float32
	optionSizes [4]geometry.Size
}

// Density changes precede WidgetBase's visible-child snapshot.
func (s *findSurface) Measure(c layout.Constraint) layout.Measurement {
	b := s.bar
	width := min(float32(512), c.Max.Width)
	narrow, reduced := width < 285, width < 400
	if b.narrow != narrow || b.reduced != reduced {
		b.narrow, b.reduced = narrow, reduced
		b.status.SetVisible(!reduced)
		for _, w := range []gui.Widget{b.matchCase, b.wholeWord, b.regex, b.selectionOnly, b.previous, b.next} {
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
	width := min(float32(512), c.Max.Width)
	slots := l.slots(children)

	// Read public content extents after framework style/measurement. This avoids
	// estimating native line height from a font's point size.
	fieldHeight := func(scroll *gui.ScrollView, field *gui.TextView) float32 {
		slots[scroll].Measure(layout.Loose(geometry.Size{Width: width, Height: 200}))
		height := field.ContentSize().Height
		if count := field.Model().LineCount(); count > 5 {
			height = (height-2*field.Padding())*5/float32(count) + 2*field.Padding()
		}
		return min(float32(200), height+2)
	}
	l.rowHeight = max(float32(28), fieldHeight(b.findScroll, b.find))
	if b.replaceMode {
		l.rowHeight = max(l.rowHeight, fieldHeight(b.replacementScroll, b.replacement))
	}

	if !b.reduced {
		l.statusWidth = max(64, slots[b.status].Measure(layout.Unbounded()).Width)
		if b.message != "" {
			l.statusWidth = min(144, l.statusWidth)
		}
	}
	if !b.narrow {
		for i, w := range []gui.Widget{b.matchCase, b.wholeWord, b.regex, b.selectionOnly} {
			m := slots[w].Measure(layout.Unbounded())
			l.optionSizes[i] = geometry.Size{Width: max(22, m.Width), Height: min(l.rowHeight, max(22, m.Height))}
		}
	}
	rows := float32(1)
	if b.replaceMode {
		rows = 2
	}
	height := 2*b.inset + 10 + rows*l.rowHeight + (rows-1)*4
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
	widgets := []gui.Widget{b.toggle, b.findScroll, b.matchCase, b.wholeWord, b.status, b.previous, b.next, b.close, b.replacementScroll, b.replace, b.replaceAll, b.regex, b.selectionOnly}
	edge := b.inset + 5
	x, y, right := edge, edge, max(edge, r.Width-edge)
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
		right -= l.statusWidth + 2
		height := slots[b.status].Measure(layout.Loose(geometry.Size{Width: l.statusWidth, Height: h})).Height
		place(4, right, y+(h-height)/2, l.statusWidth, height)
		right -= 4
	}
	place(0, x, y+(h-22)/2, 18, 22)
	x += 21
	b.fieldRect = geometry.Rect(x, y, max(0, right-x), h)
	inputRight := right
	if !b.narrow {

		total := float32(0)
		for _, size := range l.optionSizes {
			total += size.Width + 3
		}
		inputRight -= total
		cursor := inputRight + 1
		for i, index := range []int{2, 3, 11, 12} {
			size := l.optionSizes[i]
			place(index, cursor, y+(h-size.Height)/2, size.Width, size.Height)
			cursor += size.Width + 3
		}

	}
	place(1, x+1, y+1, max(0, inputRight-x-2), h-2)
	if b.replaceMode {
		y += h + 4
		b.replacementRect = geometry.Rect(x, y, b.fieldRect.Width, h)
		place(8, x+1, y+1, max(0, b.fieldRect.Width-2), h-2)
		// Replace actions align with the leading edge of the count/action area.
		if !b.narrow {
			place(9, right+4, y+(h-22)/2, 22, 22)
			place(10, right+28, y+(h-22)/2, 22, 22)
		}
	}
}
