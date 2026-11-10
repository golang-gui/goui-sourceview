package controls

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
		"selection":   "M2 5V2h3M11 2h3v3M14 11v3h-3M5 14H2v-3M5 5h6M5 8h6M5 11h4",
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

// FindActions keep editor actions and reentry guards in the root package.
type FindActions struct {
	ReplaceMode                                func(bool)
	MatchCase, WholeWord, Regex, SelectionOnly func(bool)
	Previous, Next, Close, Replace, ReplaceAll func()
	Replacing                                  func() bool
	Message                                    func() string
	QueryError                                 func() error
}

type FindUI struct {
	Owner                                              gui.Widget
	Actions                                            FindActions
	Content                                            *FindSurface
	Appearance                                         FindAppearance
	Inset                                              float32
	ToggleIcon                                         *gui.Icon
	Narrow, Reduced                                    bool
	FieldRect, ReplacementRect                         geometry.Rectangle
	Find, ReplacementInput                             *gui.TextView
	FindScroll, ReplacementScroll                      *gui.ScrollView
	Previous, Next, Replace, ReplaceAll, CloseButton   *gui.Button
	MatchCase, WholeWord, Regex, SelectionOnly, Toggle *widgets.CheckButton
	Status                                             *gui.Label
	Names                                              map[gui.Widget]mlang.Getter
}

func NewFindUI(owner gui.Widget, actions FindActions) *FindUI {
	b := &FindUI{Owner: owner, Actions: actions, Names: make(map[gui.Widget]mlang.Getter)}
	b.buildSurface()
	return b
}
func FindIcon(name string) gui.IconSource { return findIcons[name] }

func (b *FindUI) buildSurface() {
	b.Content = &FindSurface{bar: b}
	b.Content.SetLayoutManager(&findBarLayout{bar: b})
	b.Content.SetVisible(false)
	add := func(w gui.Widget) { b.Content.WidgetBase.AddChild(b.Content, w) }
	name := func(w gui.Widget, getter mlang.Getter) {
		b.Names[w] = getter
		w.SetName(getter())
	}
	action := func(icon string, getter mlang.Getter, fn func()) *gui.Button {
		w := gui.NewButton()
		w.SetStyleName("sourceview.find.action")
		w.SetPadding(geometry.UniformInsets(3))
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
		w.SetPadding(geometry.UniformInsets(2))
		w.SetStyleName("sourceview.find.option")
		w.SetChild(child)
		name(w, getter)
		w.ConnectChange(func(state widgets.CheckState) { fn(state == widgets.CheckChecked) })
		add(w)
		return w
	}
	b.ToggleIcon = gui.NewIcon(findIcons["right"])
	b.ToggleIcon.SetStyleName("sourceview.find.icon")
	b.Toggle = option(resource.ToggleReplace, b.ToggleIcon, b.Actions.ReplaceMode)
	b.Toggle.SetStyleName("sourceview.find.expand")
	b.Find = gui.NewTextView()
	b.Find.SetPlaceholder(resource.Find())
	name(b.Find, resource.Find)
	b.Find.SetStyleName("sourceview.find.input")
	b.Find.SetPadding(geometry.UniformInsets(4))
	b.FindScroll = gui.NewScrollView()
	b.FindScroll.SetDirection(gui.ScrollVertical)
	b.FindScroll.SetMainWeight(0)
	b.FindScroll.SetMaxSize(geometry.Size{Height: 200})
	b.FindScroll.SetChild(b.Find)
	add(b.FindScroll)
	textOption := func(text string) gui.Widget {
		l := gui.NewLabel(text)
		l.SetStyleName("sourceview.find.option-text")
		return l
	}
	b.MatchCase = option(resource.MatchCase, textOption("Aa"), b.Actions.MatchCase)
	b.WholeWord = option(resource.WholeWord, textOption("ab"), b.Actions.WholeWord)
	b.Regex = option(resource.RegularExpression, textOption(".*"), b.Actions.Regex)
	selectionIcon := gui.NewIcon(findIcons["selection"])
	selectionIcon.SetStyleName("sourceview.find.icon")
	b.SelectionOnly = option(resource.InSelection, selectionIcon, b.Actions.SelectionOnly)

	b.Status = gui.NewLabel("")
	b.Status.SetStyleName("sourceview.find.status")
	add(b.Status)
	b.Previous = action("previous", resource.Previous, b.Actions.Previous)
	b.Next = action("next", resource.Next, b.Actions.Next)
	b.CloseButton = action("close", resource.Close, b.Actions.Close)
	b.ReplacementInput = gui.NewTextView()
	b.ReplacementInput.SetPlaceholder(resource.Replace())
	name(b.ReplacementInput, resource.Replacement)
	b.ReplacementInput.SetStyleName("sourceview.find.input")
	b.ReplacementInput.SetPadding(geometry.UniformInsets(4))
	b.ReplacementScroll = gui.NewScrollView()
	b.ReplacementScroll.SetDirection(gui.ScrollVertical)
	b.ReplacementScroll.SetMainWeight(0)
	b.ReplacementScroll.SetMaxSize(geometry.Size{Height: 200})
	b.ReplacementScroll.SetChild(b.ReplacementInput)
	add(b.ReplacementScroll)
	b.Replace = action("replace", resource.Replace, b.Actions.Replace)
	b.ReplaceAll = action("replace-all", resource.ReplaceAll, b.Actions.ReplaceAll)
	b.ReplacementScroll.SetVisible(false)
	b.ReplacementInput.SetVisible(false)
	b.Replace.SetVisible(false)
	b.ReplaceAll.SetVisible(false)
	b.ReplacementInput.ConnectChange(func(_ gui.TextChange) { b.Content.RequestLayout() })
	b.Find.ConnectChange(func(_ gui.TextChange) { b.Content.RequestLayout() })
	b.Find.ConnectScrollIntoView(func(geometry.Rectangle) { b.Content.RequestLayout() })
	b.ReplacementInput.ConnectScrollIntoView(func(geometry.Rectangle) { b.Content.RequestLayout() })
	b.Find.ConnectFocused(func(bool) { b.Content.RequestPaint() })
	b.ReplacementInput.ConnectFocused(func(bool) { b.Content.RequestPaint() })
	b.ReadAppearance()
}

type FindAppearance struct{ Surface, Normal, Focused, Disabled, Invalid Box }

func (b *FindUI) ReadAppearance() {
	name := b.Owner.StyleName()
	if name == "" {
		name = "sourceview.find"
	}
	next := FindAppearance{Surface: BoxOf(gui.ResolveWidgetStyle(b.Owner, name, "", style.Normal)),
		Normal:   BoxOf(gui.ResolveWidgetStyle(b.Owner, "sourceview.find.input", "frame", style.Normal)),
		Focused:  BoxOf(gui.ResolveWidgetStyle(b.Owner, "sourceview.find.input", "frame", style.Focused)),
		Invalid:  BoxOf(gui.ResolveWidgetStyle(b.Owner, "sourceview.find.input", "error-frame", style.Normal)),
		Disabled: BoxOf(gui.ResolveWidgetStyle(b.Owner, "sourceview.find.input", "frame", style.Disabled))}
	if b.Appearance == next {
		return
	}
	b.Appearance = next
	b.Inset = max(float32(2), next.Surface.Outset())
	b.Content.RequestLayout()
	b.Content.RequestPaint()
}

// A surface's shadow is reserved inside ordinary widget bounds.
type FindSurface struct {
	gui.WidgetBase
	bar *FindUI
}

func (s *FindSurface) Paint(p gui.Painter) {
	b := s.bar
	b.Appearance.Surface.Paint(p, geometry.Rect(0, 0, s.Rect().Width, s.Rect().Height).Inset(b.Inset))
	input := func(r geometry.Rectangle, w *gui.TextView) {
		skin := b.Appearance.Normal
		if !gui.IsEnabled(w) {
			skin = b.Appearance.Disabled
		} else if w.Focused() {
			skin = b.Appearance.Focused
		}
		if w == b.Find {
			if b.Actions.QueryError() != nil {
				skin = b.Appearance.Invalid
			}
		}
		skin.Paint(p, r)
	}
	input(b.FieldRect, b.Find)
	if b.Actions.Replacing() {
		input(b.ReplacementRect, b.ReplacementInput)
	}
}

// Layout addresses ordinary visible children by identity.
type findBarLayout struct {
	bar               *FindUI
	rowHeight         float32
	replacementHeight float32
	controlsHeight    float32
	statusWidth       float32
	optionSizes       [4]geometry.Size
}

// Density changes precede WidgetBase's visible-child snapshot.
func (s *FindSurface) Measure(c layout.Constraint) layout.Measurement {
	b := s.bar
	width := min(float32(512), c.Max.Width)
	narrow, reduced := width < 285, width < 400
	if b.Narrow != narrow || b.Reduced != reduced {
		b.Narrow, b.Reduced = narrow, reduced
		b.Status.SetVisible(!reduced)
		for _, w := range []gui.Widget{b.MatchCase, b.WholeWord, b.Regex, b.SelectionOnly, b.Previous, b.Next} {
			w.SetVisible(!narrow)
		}
		b.Replace.SetVisible(b.Actions.Replacing() && !narrow)
		b.ReplaceAll.SetVisible(b.Actions.Replacing() && !narrow)
	}
	return s.WidgetBase.Measure(c)
}
func (l *findBarLayout) slots(children []layout.Child) map[gui.Widget]layout.Child {
	slots := make(map[gui.Widget]layout.Child, len(children))
	i := 0
	for _, w := range l.bar.Content.Children() {
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
	l.controlsHeight = 28

	if !b.Reduced {
		m := slots[b.Status].Measure(layout.Unbounded())
		l.statusWidth = max(64, m.Width)
		l.controlsHeight = max(l.controlsHeight, m.Height+6)
		if b.Actions.Message() != "" {
			l.statusWidth = min(144, l.statusWidth)
		}
	}
	optionsWidth := float32(0)
	if !b.Narrow {
		for i, w := range []gui.Widget{b.MatchCase, b.WholeWord, b.Regex, b.SelectionOnly} {
			m := slots[w].Measure(layout.Unbounded())
			l.optionSizes[i] = geometry.Size{Width: max(22, m.Width), Height: max(22, m.Height)}
			l.controlsHeight = max(l.controlsHeight, l.optionSizes[i].Height+6)
			optionsWidth += l.optionSizes[i].Width + 3
		}
	}
	rows := float32(1)
	if b.Actions.Replacing() {
		rows = 2
	}
	chrome := 2*b.Inset + 10 + (rows-1)*4
	maximum := min(float32(202), max(0, (c.Max.Height-chrome)/rows))
	// Measure at the same input widths used by Arrange. TextView and ScrollView
	// own content measurement; no logical-line count or font estimate is needed.
	fieldWidth := max(0, width-2*(b.Inset+5)-21-24)
	if !b.Narrow {
		fieldWidth = max(0, fieldWidth-48)
	}
	if !b.Reduced {
		fieldWidth = max(0, fieldWidth-l.statusWidth-6)
	}
	fieldHeight := func(scroll *gui.ScrollView, width float32) float32 {
		m := slots[scroll].Measure(layout.Loose(geometry.Size{Width: max(0, width), Height: max(0, maximum-2)}))
		return min(maximum, max(l.controlsHeight, m.Height+2))
	}
	l.rowHeight = fieldHeight(b.FindScroll, fieldWidth-optionsWidth-2)
	l.replacementHeight = 0
	if b.Actions.Replacing() {
		l.replacementHeight = fieldHeight(b.ReplacementScroll, fieldWidth-2)
	}
	height := chrome + l.rowHeight + l.replacementHeight
	// Measure all visible descendants through the framework before arrangement.
	for _, child := range children {
		if child == slots[b.FindScroll] || child == slots[b.ReplacementScroll] {
			continue
		}
		child.Measure(layout.Loose(geometry.Size{Width: width, Height: height}))
	}
	return layout.Measured(c.Clamp(geometry.Size{Width: width, Height: height}))
}
func (l *findBarLayout) Arrange(children []layout.Child, r geometry.Rectangle) {
	b := l.bar
	h := l.rowHeight
	slots := l.slots(children)
	widgets := []gui.Widget{b.Toggle, b.FindScroll, b.MatchCase, b.WholeWord, b.Status, b.Previous, b.Next, b.CloseButton, b.ReplacementScroll, b.Replace, b.ReplaceAll, b.Regex, b.SelectionOnly}
	edge := b.Inset + 5
	x, y, right := edge, edge, max(edge, r.Width-edge)
	// Controls occupy a compact strip at the top of each input frame. Only the
	// input viewport grows with its content; scrolling never moves the strip.
	controlY := func(height float32) float32 {
		return y + max(0, (min(h, l.controlsHeight)-height)/2)
	}
	place := func(i int, x, y, w, h float32) {
		if child := slots[widgets[i]]; child != nil {
			child.Arrange(geometry.Rect(x, y, max(0, w), max(0, h)))
		}
	}
	// Allocate from the right so actions retain their targets at narrow widths.
	take := func(i int) {
		right -= 22
		place(i, max(x, right), controlY(22), min(float32(22), max(0, right+22-x)), 22)
		right -= 2
	}
	take(7)
	if !b.Narrow {
		take(6)
		take(5)
	}
	if !b.Reduced {
		right -= l.statusWidth + 2
		height := slots[b.Status].Measure(layout.Loose(geometry.Size{Width: l.statusWidth, Height: min(h, l.controlsHeight)})).Height
		place(4, right, controlY(height), l.statusWidth, height)
		right -= 4
	}
	place(0, x, controlY(22), 18, 22)
	x += 21
	b.FieldRect = geometry.Rect(x, y, max(0, right-x), h)
	inputRight := right
	if !b.Narrow {

		total := float32(0)
		for _, size := range l.optionSizes {
			total += size.Width + 3
		}
		inputRight -= total
		cursor := inputRight + 1
		for i, index := range []int{2, 3, 11, 12} {
			size := l.optionSizes[i]
			place(index, cursor, controlY(size.Height), size.Width, size.Height)
			cursor += size.Width + 3
		}

	}
	place(1, x+1, y+1, max(0, inputRight-x-2), h-2)
	if b.Actions.Replacing() {
		y += h + 4
		h = l.replacementHeight
		b.ReplacementRect = geometry.Rect(x, y, b.FieldRect.Width, h)
		place(8, x+1, y+1, max(0, b.FieldRect.Width-2), h-2)
		// Replace actions align with the leading edge of the count/action area.
		if !b.Narrow {
			place(9, right+4, controlY(22), 22, 22)
			place(10, right+28, controlY(22), 22, 22)
		}
	}
}
