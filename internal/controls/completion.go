package controls

import (
	"cmp"
	"image/color"
	"math"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/core/signal"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/layout"
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
type CompletionList struct {
	gui.WidgetBase
	host                           CompletionHost
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
	colors                         CompletionColors
	font, secondaryFont            typography.FontInfo
	box                            Box
	selectedRadius                 geometry.CornerRadius
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

// CompletionState is borrowed from the root-owned completion session.
type CompletionState struct {
	Count, Index   int
	Prefix         string
	Pending, Shown bool
}
type CompletionItem struct{ Label, Detail string }

// CompletionHost retains editing and session identity at the owning editor.
// Accessors are sequential, allocation-free reads; callbacks may invalidate it.
type CompletionHost struct {
	Owner   gui.Widget
	Context func() typography.Context
	Current func() bool
	State   func() CompletionState
	Item    func(int) CompletionItem
	Select  func(int)
	Accept  func(int)
	Fail    func(error)
	Changed error
}

func NewCompletionList(host CompletionHost) *CompletionList {
	l := &CompletionList{host: host}
	l.ReadAppearance()
	l.ConnectUnmount(l.Release)
	click := gui.NewClickEventController()
	click.ConnectClicked(func(ctx gui.EventContext, info gui.ClickInfo) {
		if index, hit := l.itemAt(info.Position); hit {
			host.Accept(index)
			ctx.StopPropagation()
		}
	})
	l.AddEventController(click)
	motion := gui.NewMotionEventController()
	motion.ConnectMotion(func(_ gui.EventContext, info gui.MotionInfo) {
		if index, hit := l.itemAt(info.Position); hit && host.State().Index != index {
			host.Select(index)
			l.RequestPaint()
		}
	})
	l.AddEventController(motion)
	return l
}

func (l *CompletionList) Release() {
	l.epoch++
	rows := l.rows
	l.rows = nil
	for _, row := range rows {
		row.Release()
	}
}

func (r *completionRow) Release() {
	for _, text := range []*completionText{r.label, r.detail} {
		if text != nil {
			text.native.Destroy()
		}
	}
}

func (l *CompletionList) Refresh() {
	l.reveal.Emit(geometry.Rect(0, float32(l.host.State().Index)*l.rowHeight, l.viewport.Width, l.rowHeight))
	l.RequestPaint()
}

func (l *CompletionList) ConnectScrollIntoView(fn func(geometry.Rectangle)) signal.Handle {
	return l.reveal.Connect(fn)
}

var _ gui.Scrollable = (*CompletionList)(nil)

func (l *CompletionList) detailFont() typography.FontInfo { return l.secondaryFont }

func (l *CompletionList) StyleChanged() { l.ReadAppearance() }

func (l *CompletionList) ReadAppearance() bool {
	resolve := func(part string) style.Style {
		return gui.ResolveWidgetStyle(l.host.Owner, "sourceview.completion", part, style.Normal)
	}
	root := resolve("")
	selected := resolve("selected")
	selectedRadius, _ := selected.CornerRadius()
	selectedRadius = cleanCornerRadius(selectedRadius)
	border, _ := root.BorderColor()
	colors := CompletionColors{Background: background(root), Foreground: foreground(root), Border: nrgba(border), DetailForeground: foreground(resolve("detail")),
		SelectedBackground: background(selected), SelectedForeground: foreground(selected), SelectedDetailForeground: foreground(resolve("selected-detail")),
		MatchForeground: foreground(resolve("match")), SelectedMatchForeground: foreground(resolve("selected-match"))}
	font, detail, box := styleFont(root), styleFont(resolve("detail")), BoxOf(root)
	if colors == l.colors && font == l.font && detail == l.secondaryFont && box == l.box && selectedRadius == l.selectedRadius {
		return false
	}
	l.Release()
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
	list    *CompletionList
	context typography.Context
	epoch   uint64
	live    []typography.TextLayout
}

func (l *CompletionList) build() *completionBuild {
	return &completionBuild{list: l, context: l.host.Context(), epoch: l.epoch}
}

func (b *completionBuild) valid() bool {
	l := b.list
	return b.context != nil && l.epoch == b.epoch && l.host.Current() && l.host.Context() == b.context
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
		return nil, b.list.host.Changed
	}
	native, err := b.context.NewTextLayout(text, typography.TextFormat{Font: font, TextColor: foreground, WrapMode: typography.WrapNone}, 1<<20, 1<<16)
	if native != nil {
		b.live = append(b.live, native)
	}
	if !b.valid() {
		return nil, b.list.host.Changed
	}
	if err != nil {
		return nil, err
	}
	w, h := native.MeasureSize()
	if !b.valid() {
		return nil, b.list.host.Changed
	}
	lines, clusters := native.MeasureMetrics()
	if !b.valid() {
		return nil, b.list.host.Changed
	}
	baseline := h * .8
	if len(lines) > 0 && lines[0].Baseline > 0 {
		baseline = lines[0].Baseline
	}
	return &completionText{native: native, width: w, height: h, baseline: baseline, clusters: clusters, kept: len(text)}, nil
}

func (l *CompletionList) fail(b *completionBuild, err error) {
	if err == nil || !b.valid() {
		return
	}
	l.err = err
	if l.host.State().Shown {
		l.host.Fail(err)
	}
}

func (l *CompletionList) measureItems() {
	if l.measured || l.err != nil || l.preparing {
		return
	}
	l.preparing = true
	defer func() { l.preparing = false }()
	b := l.build()
	defer b.destroy()
	var label, detail, ascent, descent float32
	for i := range l.host.State().Count {
		item := l.host.Item(i)
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
	if l.host.State().Count > completionRows {
		width += completionScrollWidth
	}
	l.width, l.measured = max(240, min(520, width)), true
}

func (l *CompletionList) Measure(c layout.Constraint) layout.Measurement {
	l.measureItems()
	return layout.Measured(c.Clamp(geometry.Size{Width: max(0, l.width-2*completionInset), Height: float32(min(completionRows, l.host.State().Count)) * l.rowHeight}))
}

func (l *CompletionList) ContentSize() geometry.Size {
	return geometry.Size{Width: l.viewport.Width, Height: float32(l.host.State().Count) * l.rowHeight}
}

func (l *CompletionList) LayoutVisible(viewport geometry.Size, offset geometry.Point) {
	if l.layoutReady && viewport == l.viewport && offset == l.offset {
		return
	}
	if viewport.Width != l.viewport.Width {
		l.Release()
	}
	l.epoch++
	l.viewport, l.offset = viewport, offset
	l.layoutReady = true
	l.first, l.visible = 0, 0
	if l.rowHeight > 0 && viewport.Width > 0 && viewport.Height > 0 {
		l.first = min(l.host.State().Count, max(0, int(offset.Y/l.rowHeight)))
		end := min(l.host.State().Count, int(math.Ceil(float64((offset.Y+viewport.Height)/l.rowHeight))))
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
		row.Release()
	}
	l.RequestPaint()
}

func (l *CompletionList) columns() (labelWidth, detailX, detailWidth float32) {
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
		return nil, b.list.host.Changed
	}
	best.clusters = nil
	return best, nil
}

func (l *CompletionList) ensureRows() {
	if l.err != nil || !l.measured || l.preparing || len(l.rows) == l.visible {
		return
	}
	l.preparing = true
	defer func() { l.preparing = false }()
	b := l.build()
	defer b.destroy()
	labelWidth, _, detailWidth := l.columns()
	rows := make(map[int]*completionRow)
	for i := l.first; i < min(l.host.State().Count, l.first+l.visible); i++ {
		if l.rows[i] != nil {
			continue
		}
		item := l.host.Item(i)
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

func (l *CompletionList) rowRect(slot int) geometry.Rectangle {
	return geometry.Rect(0, float32(l.first+slot)*l.rowHeight-l.offset.Y, l.viewport.Width, l.rowHeight)
}

func (l *CompletionList) itemAt(p geometry.Point) (int, bool) {
	if !l.host.Current() || l.host.State().Pending || l.rowHeight <= 0 || p.X < 0 || p.X >= l.viewport.Width || p.Y < 0 || p.Y >= l.viewport.Height {
		return 0, false
	}
	index := int((p.Y + l.offset.Y) / l.rowHeight)
	if index >= l.host.State().Count {
		return 0, false
	}
	return index, true
}

func (l *CompletionList) styleRow(row *completionRow, index int, b *completionBuild) bool {
	selected := index == l.host.State().Index
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
		if end := min(row.label.kept, completionPrefixEnd(l.host.Item(index).Label, l.host.State().Prefix)); end > 0 {
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

func (l *CompletionList) Paint(p gui.Painter) {
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
				gui.PaintStyledBackground(p, fill, style.Rule{}.BackgroundColor(colors.SelectedBackground).
					CornerRadius(l.selectedRadius).Style)
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

func (l *CompletionList) Snapshot() gui.WidgetInfo {
	info := l.WidgetBase.Snapshot()
	info.Role = gui.Role("completion-list")
	info.SetAttribute("sourceview.completion.pending", l.host.State().Pending)
	info.ItemCount, info.VisibleStart, info.VisibleEnd = l.host.State().Count, l.first, l.first+l.visible
	for slot := 0; slot < l.visible; slot++ {
		i := l.first + slot
		item := l.host.Item(i)
		r := l.rowRect(slot)
		top, bottom := max(0, r.Y), min(l.viewport.Height, r.Y+r.Height)
		r.Y, r.Height = top, max(0, bottom-top)
		row := gui.WidgetInfo{Role: gui.Role("completion-item"), Text: item.Label, Visible: true, Enabled: info.Enabled && !l.host.State().Pending, Selected: i == l.host.State().Index, Bounds: r.Translate(info.Bounds.Pos)}
		if row.Enabled {
			row.Actions = []gui.Action{gui.ActionClick}
		}
		row.SetAttribute("sourceview.completion.detail", item.Detail)
		info.Children = append(info.Children, row)
	}
	return info
}

// The panel owns finite popup geometry and padding. ScrollView owns scrolling;
// CompletionList owns only the candidate layouts intersecting that viewport.
type CompletionPanel struct {
	gui.WidgetBase
	list   *CompletionList
	scroll *gui.ScrollView
}

func NewCompletionPanel(list *CompletionList) *CompletionPanel {
	p := &CompletionPanel{list: list, scroll: gui.NewScrollView()}
	p.scroll.SetDirection(gui.ScrollVertical)
	p.scroll.SetMainWeight(0)
	p.scroll.SetChild(list)
	p.WidgetBase.AddChild(p, p.scroll)
	return p
}

func (p *CompletionPanel) Measure(c layout.Constraint) layout.Measurement {
	l := p.list
	l.measureItems()
	inset := p.Inset()
	size := c.Clamp(geometry.Size{Width: min(520, l.width+2*(inset-completionInset)), Height: 2*inset + float32(min(completionRows, l.host.State().Count))*l.rowHeight})
	p.scroll.SetLineHeight(l.rowHeight)
	p.scroll.Measure(layout.Tight(size.Inset(inset)))
	return layout.Measured(size)
}

func (p *CompletionPanel) Arrange(r geometry.Rectangle) {
	p.WidgetBase.Arrange(r)
	p.scroll.Arrange(geometry.Rect(0, 0, r.Width, r.Height).Inset(p.Inset()))
}

// Reserve a conservative safe interior for the styled Popover's rounded
// corners. The shell, shadow and border are drawn only by Popover itself.
func (p *CompletionPanel) Inset() float32 {
	return max(completionInset, float32(math.Ceil(float64(p.list.box.Radius.Max()/2)))+p.list.box.Width)
}

func (p *CompletionPanel) Snapshot() gui.WidgetInfo {
	info := p.WidgetBase.Snapshot()
	info.Role = "completion-panel"
	return info
}

// Reset invalidates content metrics after the owner replaces the candidate set.
func (l *CompletionList) Reset() {
	l.measured, l.layoutReady = false, false
	l.first, l.visible = 0, 0
	l.measureItems()
}
func (l *CompletionList) Error() error { return l.err }

// Prefix matching folds Unicode runes, whose UTF-8 lengths can change (İ -> i).
// Convert the folded prefix length back to a boundary in the original label.
func completionPrefixEnd(label, prefix string) int {
	folded := strings.ToLower(prefix)
	if folded == "" || !strings.HasPrefix(strings.ToLower(label), folded) {
		return 0
	}
	length := 0
	for at, r := range label {
		length += utf8.RuneLen(unicode.ToLower(r))
		if length >= len(folded) {
			return at + utf8.RuneLen(r)
		}
	}
	return 0
}

// CompletionAppearance is a comparable snapshot used by the owner to invalidate
// the native popup when UI appearance changes independently of the code scheme.
type CompletionAppearance struct {
	Colors           CompletionColors
	Font, DetailFont typography.FontInfo
	Box              Box
	SelectedRadius   geometry.CornerRadius
}

func (l *CompletionList) Appearance() CompletionAppearance {
	return CompletionAppearance{l.colors, l.font, l.secondaryFont, l.box, l.selectedRadius}
}

const CompletionTextInset = completionTextInset
const CompletionInset = completionInset
