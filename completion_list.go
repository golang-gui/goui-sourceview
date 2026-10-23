package sourceview

import (
	"cmp"
	"image/color"
	"math"
	"slices"
	"unicode/utf8"

	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/core/signal"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/layout"
	"github.com/golang-gui/goui/platform/graphics"
	"github.com/golang-gui/goui/platform/typography"
	"github.com/golang-gui/goui/style"
)

const (
	completionRows                = 8
	completionInset       float32 = 6
	completionTextInset   float32 = 12
	completionGap         float32 = 16
	completionScrollWidth float32 = 8
)

// Only visible rows own native layouts. All-candidate measurements are retained
// as scalar metrics, so scrolling never changes the panel width or columns.
type completionList struct {
	gui.WidgetBase
	view                           *SourceView
	session                        *completionSession
	first, visible                 int
	viewport                       geometry.Size
	offset                         geometry.Point
	layoutReady                    bool
	reveal                         signal.Signal1[geometry.Rectangle]
	rowHeight, ascent, descent     float32
	width, labelWidth, detailWidth float32
	measured                       bool
	preparing                      bool
	rows                           map[int]*completionRow
	epoch                          uint64
	err                            error
	colors                         completionColors
	font, secondaryFont            typography.FontInfo
	box                            styleBox
	selectedRadius                 float32
}

type completionRow struct {
	label, detail    *completionText
	styled, selected bool
}

type completionText struct {
	native                  typography.TextLayout
	width, height, baseline float32
	clusters                []typography.TextCluster
	kept                    int // original bytes before an optional ellipsis
}

func newCompletionList(v *SourceView, s *completionSession) *completionList {
	l := &completionList{view: v, session: s}
	l.readAppearance()
	l.ConnectUnmount(l.release)
	click := gui.NewClickEventController()
	click.ConnectClicked(func(ctx gui.EventContext) {
		if p, ok := ctx.Position(); ok {
			if index, hit := l.itemAt(p); hit {
				v.report(v.acceptCompletion(s, index))
				ctx.StopPropagation()
			}
		}
	})
	l.AddEventController(click)
	motion := gui.NewMotionEventController()
	motion.ConnectMotion(func(info gui.MotionInfo) {
		if index, hit := l.itemAt(info.Position); hit && s.index != index {
			s.index = index
			l.RequestPaint()
		}
	})
	l.AddEventController(motion)
	return l
}

func (l *completionList) release() {
	l.epoch++
	rows := l.rows
	l.rows = nil
	for _, row := range rows {
		row.release()
	}
}

func (r *completionRow) release() {
	for _, text := range []*completionText{r.label, r.detail} {
		if text != nil {
			text.native.Destroy()
		}
	}
}

func (l *completionList) refresh() {
	l.reveal.Emit(geometry.Rect(0, float32(l.session.index)*l.rowHeight, l.viewport.Width, l.rowHeight))
	l.RequestPaint()
}

func (l *completionList) ConnectScrollIntoView(fn func(geometry.Rectangle)) signal.Handle {
	return l.reveal.Connect(fn)
}

var _ gui.Scrollable = (*completionList)(nil)

func (l *completionList) detailFont() typography.FontInfo { return l.secondaryFont }

func (l *completionList) StyleChanged() { l.readAppearance() }

func (l *completionList) readAppearance() bool {
	resolve := func(part string) style.Style {
		return gui.ResolveWidgetStyle(l.view, "sourceview.completion", part, style.Normal)
	}
	root := resolve("")
	selected := resolve("selected")
	selectedRadius, _ := selected.Radius()
	if math.IsNaN(float64(selectedRadius)) || math.IsInf(float64(selectedRadius), 0) {
		selectedRadius = 0
	}
	selectedRadius = max(0, selectedRadius)
	border, _ := root.BorderColor()
	colors := completionColors{Background: background(root), Foreground: foreground(root), Border: nrgba(border), DetailForeground: foreground(resolve("detail")),
		SelectedBackground: background(selected), SelectedForeground: foreground(selected), SelectedDetailForeground: foreground(resolve("selected-detail")),
		MatchForeground: foreground(resolve("match")), SelectedMatchForeground: foreground(resolve("selected-match"))}
	font, detail, box := styleFont(root), styleFont(resolve("detail")), boxOf(root)
	if colors == l.colors && font == l.font && detail == l.secondaryFont && box == l.box && selectedRadius == l.selectedRadius {
		return false
	}
	l.release()
	l.measured = false
	l.layoutReady = false
	l.colors, l.font, l.secondaryFont, l.box = colors, font, detail, box
	l.selectedRadius = selectedRadius
	l.RequestLayout()
	l.RequestPaint()
	return true
}

// A build transaction owns every layout until its rows are published. Native
// callbacks can cancel/unmount the panel without releasing an in-flight probe.
type completionBuild struct {
	list    *completionList
	context typography.Context
	epoch   uint64
	live    []typography.TextLayout
}

func (l *completionList) build() *completionBuild {
	return &completionBuild{list: l, context: l.view.context, epoch: l.epoch}
}

func (b *completionBuild) valid() bool {
	l := b.list
	return b.context != nil && l.epoch == b.epoch && l.view.completion == l.session && l.view.context == b.context
}

func (b *completionBuild) discard(native typography.TextLayout) {
	at := slices.Index(b.live, native)
	b.live = slices.Delete(b.live, at, at+1)
	native.Destroy()
}

func (b *completionBuild) destroy() {
	for _, native := range b.live {
		native.Destroy()
	}
}

func (b *completionBuild) text(text string, font typography.FontInfo, foreground color.NRGBA) (*completionText, error) {
	if !b.valid() {
		return nil, ErrOperationChanged
	}
	native, err := b.context.NewTextLayout(text, typography.TextFormat{Font: font, TextColor: foreground, WrapMode: typography.WrapNone}, 1<<20, 1<<16)
	if native != nil {
		b.live = append(b.live, native)
	}
	if !b.valid() {
		return nil, ErrOperationChanged
	}
	if err != nil {
		return nil, err
	}
	w, h := native.MeasureSize()
	if !b.valid() {
		return nil, ErrOperationChanged
	}
	lines, clusters := native.MeasureMetrics()
	if !b.valid() {
		return nil, ErrOperationChanged
	}
	baseline := h * .8
	if len(lines) > 0 && lines[0].Baseline > 0 {
		baseline = lines[0].Baseline
	}
	return &completionText{native: native, width: w, height: h, baseline: baseline, clusters: clusters, kept: len(text)}, nil
}

func (l *completionList) fail(b *completionBuild, err error) {
	if err == nil || !b.valid() {
		return
	}
	l.err = err
	if l.session.shown {
		l.view.closeCompletion()
		l.view.report(err)
	}
}

func (l *completionList) measureItems() {
	if l.measured || l.err != nil || l.preparing {
		return
	}
	l.preparing = true
	defer func() { l.preparing = false }()
	b := l.build()
	defer b.destroy()
	var label, detail, ascent, descent float32
	for _, item := range l.session.items {
		for column, value := range []string{item.Label, item.Detail} {
			if value == "" {
				continue
			}
			font := l.font
			if column == 1 {
				font = l.detailFont()
			}
			text, err := b.text(value, font, l.colors.Foreground)
			if err != nil {
				l.fail(b, err)
				return
			}
			if column == 0 {
				label = max(label, text.width)
			} else {
				detail = max(detail, text.width)
			}
			ascent, descent = max(ascent, text.baseline), max(descent, text.height-text.baseline)
			b.discard(text.native)
			if !b.valid() {
				return
			}
		}
	}
	if !b.valid() {
		return
	}
	l.labelWidth, l.detailWidth = min(260, label), min(220, detail)
	l.ascent, l.descent, l.rowHeight = ascent, descent, max(26, ascent+descent+8)
	width := 2*completionTextInset + l.labelWidth
	if detail > 0 {
		width += completionGap + l.detailWidth
	}
	if len(l.session.items) > completionRows {
		width += completionScrollWidth
	}
	l.width, l.measured = max(240, min(520, width)), true
}

func (l *completionList) Measure(c layout.Constraint) layout.Measurement {
	l.measureItems()
	return layout.Measured(c.Clamp(geometry.Size{Width: max(0, l.width-2*completionInset), Height: float32(min(completionRows, len(l.session.items))) * l.rowHeight}))
}

func (l *completionList) ContentSize() geometry.Size {
	return geometry.Size{Width: l.viewport.Width, Height: float32(len(l.session.items)) * l.rowHeight}
}

func (l *completionList) LayoutVisible(viewport geometry.Size, offset geometry.Point) {
	if l.layoutReady && viewport == l.viewport && offset == l.offset {
		return
	}
	if viewport.Width != l.viewport.Width {
		l.release()
	}
	l.epoch++
	l.viewport, l.offset = viewport, offset
	l.layoutReady = true
	l.first, l.visible = 0, 0
	if l.rowHeight > 0 && viewport.Width > 0 && viewport.Height > 0 {
		l.first = min(len(l.session.items), max(0, int(offset.Y/l.rowHeight)))
		end := min(len(l.session.items), int(math.Ceil(float64((offset.Y+viewport.Height)/l.rowHeight))))
		l.visible = max(0, end-l.first)
	}
	var removed []*completionRow
	for index, row := range l.rows {
		if index < l.first || index >= l.first+l.visible {
			delete(l.rows, index)
			removed = append(removed, row)
		}
	}
	for _, row := range removed {
		row.release()
	}
	l.RequestPaint()
}

func (l *completionList) columns() (labelWidth, detailX, detailWidth float32) {
	inset := completionTextInset - completionInset
	right := max(inset, l.viewport.Width-inset)
	available := right - inset
	if l.detailWidth == 0 {
		return available, right, 0
	}
	labelWidth = min(l.labelWidth, max(0, available-completionGap-min(l.detailWidth, available*.45)))
	detailX = inset + labelWidth + completionGap
	return labelWidth, detailX, max(0, right-detailX)
}

// Logical cut positions must not bisect a shaping cluster, including overlapping
// native clusters. The replacement is reshaped and measured with its ellipsis.
func completionCuts(text string, clusters []typography.TextCluster) []int {
	cuts := []int{0}
	ordered := slices.Clone(clusters)
	slices.SortFunc(ordered, func(a, b typography.TextCluster) int { return cmp.Compare(a.Start, b.Start) })
	appendCut := func(end int) {
		if end > 0 && (end == len(text) || utf8.RuneStart(text[end])) {
			cuts = append(cuts, end)
		}
	}
	end := 0
	for _, c := range ordered {
		if c.Start < 0 || c.Length <= 0 || c.Start > len(text) || c.Length > len(text)-c.Start {
			continue
		}
		if c.Start >= end {
			appendCut(end)
		}
		end = max(end, c.Start+c.Length)
	}
	appendCut(end)
	cuts = append(cuts, len(text))
	return slices.Compact(cuts)
}

func (b *completionBuild) fit(value string, font typography.FontInfo, foreground color.NRGBA, width float32) (*completionText, error) {
	if value == "" || width <= 0 {
		return nil, nil
	}
	full, err := b.text(value, font, foreground)
	if err != nil {
		return nil, err
	}
	if full.width <= width {
		full.clusters = nil
		return full, nil
	}
	cuts := completionCuts(value, full.clusters)
	b.discard(full.native)
	best, err := b.text("…", font, foreground)
	if err != nil {
		return nil, err
	}
	best.kept = 0
	if best.width > width {
		b.discard(best.native)
		return nil, nil
	}
	for lo, hi := 1, len(cuts)-2; lo <= hi; {
		mid := lo + (hi-lo)/2
		candidate, err := b.text(value[:cuts[mid]]+"…", font, foreground)
		if err != nil {
			return nil, err
		}
		if candidate.width <= width {
			b.discard(best.native)
			best = candidate
			best.kept = cuts[mid]
			lo = mid + 1
		} else {
			b.discard(candidate.native)
			hi = mid - 1
		}
	}
	if !b.valid() {
		return nil, ErrOperationChanged
	}
	best.clusters = nil
	return best, nil
}

func (l *completionList) ensureRows() {
	if l.err != nil || !l.measured || l.preparing || len(l.rows) == l.visible {
		return
	}
	l.preparing = true
	defer func() { l.preparing = false }()
	b := l.build()
	defer b.destroy()
	labelWidth, _, detailWidth := l.columns()
	rows := make(map[int]*completionRow)
	for i := l.first; i < min(len(l.session.items), l.first+l.visible); i++ {
		if l.rows[i] != nil {
			continue
		}
		item := l.session.items[i]
		label, err := b.fit(item.Label, l.font, l.colors.Foreground, labelWidth)
		if err != nil {
			l.fail(b, err)
			return
		}
		detail, err := b.fit(item.Detail, l.detailFont(), l.colors.DetailForeground, detailWidth)
		if err != nil {
			l.fail(b, err)
			return
		}
		rows[i] = &completionRow{label: label, detail: detail}
	}
	if !b.valid() {
		return
	}
	if l.rows == nil {
		l.rows = make(map[int]*completionRow)
	}
	for index, row := range rows {
		l.rows[index] = row
	}
	b.live = nil
}

func (l *completionList) rowRect(slot int) geometry.Rectangle {
	return geometry.Rect(0, float32(l.first+slot)*l.rowHeight-l.offset.Y, l.viewport.Width, l.rowHeight)
}

func (l *completionList) itemAt(p geometry.Point) (int, bool) {
	if l.view.completion != l.session || l.rowHeight <= 0 || p.X < 0 || p.X >= l.viewport.Width || p.Y < 0 || p.Y >= l.viewport.Height {
		return 0, false
	}
	index := int((p.Y + l.offset.Y) / l.rowHeight)
	if index >= len(l.session.items) {
		return 0, false
	}
	return index, true
}

func (l *completionList) styleRow(row *completionRow, index int, b *completionBuild) bool {
	selected := index == l.session.index
	if row.styled && row.selected == selected {
		return b.valid()
	}
	colors := l.colors
	foreground, detail, match := colors.Foreground, colors.DetailForeground, colors.MatchForeground
	if selected {
		foreground, detail, match = colors.SelectedForeground, colors.SelectedDetailForeground, colors.SelectedMatchForeground
	}
	if row.label != nil {
		native := row.label.native
		native.SetTextColor(0, len(native.Text()), foreground)
		if !b.valid() {
			return false
		}
		if end := min(row.label.kept, completionPrefixEnd(l.session.items[index].Label, l.session.prefix)); end > 0 {
			native.SetTextColor(0, end, match)
			if !b.valid() {
				return false
			}
		}
	}
	if row.detail != nil {
		row.detail.native.SetTextColor(0, len(row.detail.native.Text()), detail)
		if !b.valid() {
			return false
		}
	}
	row.styled, row.selected = true, selected
	return true
}

func (l *completionList) Paint(p gui.Painter) {
	l.ensureRows()
	b := l.build()
	if !b.valid() || l.err != nil {
		return
	}
	colors := l.colors
	labelWidth, detailX, detailWidth := l.columns()
	for slot := 0; slot < l.visible; slot++ {
		row := l.rows[l.first+slot]
		if row == nil || !l.styleRow(row, l.first+slot, b) {
			return
		}
		r := l.rowRect(slot)
		if row.selected {
			// Round the full row before ScrollView clips it. A partially visible
			// row must not acquire a new rounded edge at the viewport boundary.
			fill := geometry.Rect(r.X+2, r.Y+1, max(0, r.Width-4), max(0, r.Height-2))
			if fill.Width > 0 && fill.Height > 0 {
				radius := min(l.selectedRadius, min(fill.Width, fill.Height)/2)
				if radius == 0 {
					p.FillRect(fill, graphics.ColorOf(colors.SelectedBackground))
				} else {
					p.FillRoundRect(fill, radius, graphics.ColorOf(colors.SelectedBackground))
				}
			}
		}
		baseline := r.Y + (l.rowHeight-l.ascent-l.descent)/2 + l.ascent
		for column, text := range []*completionText{row.label, row.detail} {
			if text == nil {
				continue
			}
			x, width := completionTextInset-completionInset, labelWidth
			if column == 1 {
				x, width = detailX, detailWidth
			}
			p.Save()
			p.SetClipRect(geometry.Rect(x, r.Y, width, r.Height))
			if column == 1 {
				x += max(0, width-text.width)
			}
			p.DrawTextLayout(geometry.Point{X: x, Y: baseline - text.baseline}, text.native)
			p.Restore()
			if !b.valid() {
				return
			}
		}
	}
}

func (l *completionList) Snapshot() gui.WidgetInfo {
	info := l.WidgetBase.Snapshot()
	info.Role = gui.Role("completion-list")
	info.ItemCount, info.VisibleStart, info.VisibleEnd = len(l.session.items), l.first, l.first+l.visible
	for slot := 0; slot < l.visible; slot++ {
		i := l.first + slot
		item := l.session.items[i]
		r := l.rowRect(slot)
		top, bottom := max(0, r.Y), min(l.viewport.Height, r.Y+r.Height)
		r.Y, r.Height = top, max(0, bottom-top)
		row := gui.WidgetInfo{Role: gui.Role("completion-item"), Text: item.Label, Visible: true, Enabled: true, Selected: i == l.session.index, Actions: []gui.Action{gui.ActionClick}, Bounds: r.Translate(info.Bounds.Pos)}
		row.SetAttribute("sourceview.completion.detail", item.Detail)
		info.Children = append(info.Children, row)
	}
	return info
}

// The panel owns finite popup geometry and padding. ScrollView owns scrolling;
// completionList owns only the candidate layouts intersecting that viewport.
type completionPanel struct {
	gui.WidgetBase
	list   *completionList
	scroll *gui.ScrollView
}

func newCompletionPanel(list *completionList) *completionPanel {
	p := &completionPanel{list: list, scroll: gui.NewScrollView()}
	p.scroll.SetDirection(gui.ScrollVertical)
	p.scroll.SetMainWeight(0)
	p.scroll.SetChild(list)
	p.WidgetBase.AddChild(p, p.scroll)
	return p
}

func (p *completionPanel) Measure(c layout.Constraint) layout.Measurement {
	l := p.list
	l.measureItems()
	inset := p.inset()
	size := c.Clamp(geometry.Size{Width: min(520, l.width+2*(inset-completionInset)), Height: 2*inset + float32(min(completionRows, len(l.session.items)))*l.rowHeight})
	p.scroll.SetLineHeight(l.rowHeight)
	p.scroll.Measure(layout.Tight(size.Inset(inset)))
	return layout.Measured(size)
}

func (p *completionPanel) Arrange(r geometry.Rectangle) {
	p.WidgetBase.Arrange(r)
	p.scroll.Arrange(geometry.Rect(0, 0, r.Width, r.Height).Inset(p.inset()))
}

// Reserve a conservative safe interior for the styled Popover's rounded
// corners. The shell, shadow and border are drawn only by Popover itself.
func (p *completionPanel) inset() float32 {
	return max(completionInset, float32(math.Ceil(float64(p.list.box.radius/2)))+p.list.box.width)
}

func (p *completionPanel) Snapshot() gui.WidgetInfo {
	info := p.WidgetBase.Snapshot()
	info.Role = "completion-panel"
	return info
}
