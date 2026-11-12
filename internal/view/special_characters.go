package view

import (
	"fmt"
	"image/color"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	textcore "github.com/golang-gui/goui-sourceview/internal/text"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/platform/graphics"
)

var c0Names = [...]string{"NUL", "SOH", "STX", "ETX", "EOT", "ENQ", "ACK", "BEL", "BS", "HT", "LF", "VT", "FF", "CR", "SO", "SI", "DLE", "DC1", "DC2", "DC3", "DC4", "NAK", "SYN", "ETB", "CAN", "EM", "SUB", "ESC", "FS", "GS", "RS", "US"}
var c1Names = [...]string{"DEL", "PAD", "HOP", "BPH", "NBH", "IND", "NEL", "SSA", "ESA", "HTS", "HTJ", "VTS", "PLD", "PLU", "RI", "SS2", "SS3", "DCS", "PU1", "PU2", "STS", "CCH", "MW", "SPA", "EPA", "SOS", "SGCI", "SCI", "CSI", "ST", "OSC", "PM", "APC"}
var specialFormats = [...]struct {
	code rune
	name string
}{
	{0x00ad, "SOFT HYPHEN"}, {0x034f, "COMBINING GRAPHEME JOINER"}, {0x061c, "ARABIC LETTER MARK"},
	{0x180e, "MONGOLIAN VOWEL SEPARATOR"}, {0x200b, "ZERO WIDTH SPACE"}, {0x200c, "ZERO WIDTH NON-JOINER"},
	{0x200d, "ZERO WIDTH JOINER"}, {0x200e, "LEFT-TO-RIGHT MARK"}, {0x200f, "RIGHT-TO-LEFT MARK"},
	{0x2028, "LINE SEPARATOR"}, {0x2029, "PARAGRAPH SEPARATOR"}, {0x202a, "LEFT-TO-RIGHT EMBEDDING"},
	{0x202b, "RIGHT-TO-LEFT EMBEDDING"}, {0x202c, "POP DIRECTIONAL FORMATTING"},
	{0x202d, "LEFT-TO-RIGHT OVERRIDE"}, {0x202e, "RIGHT-TO-LEFT OVERRIDE"},
	{0x2060, "WORD JOINER"}, {0x2061, "FUNCTION APPLICATION"}, {0x2062, "INVISIBLE TIMES"},
	{0x2063, "INVISIBLE SEPARATOR"}, {0x2064, "INVISIBLE PLUS"}, {0x2066, "LEFT-TO-RIGHT ISOLATE"},
	{0x2067, "RIGHT-TO-LEFT ISOLATE"}, {0x2068, "FIRST STRONG ISOLATE"}, {0x2069, "POP DIRECTIONAL ISOLATE"},
	{0xfeff, "ZERO WIDTH NO-BREAK SPACE (BOM)"},
}

func specialCharacterID(r rune) int {
	if r >= 0 && r < 32 {
		if r == '\t' || r == '\n' || r == '\r' {
			return -1
		}
		return int(r)
	}
	if r >= 0x7f && r <= 0x9f {
		return 32 + int(r-0x7f)
	}
	if r < 0x7f {
		return -1
	}
	i := sort.Search(len(specialFormats), func(i int) bool { return specialFormats[i].code >= r })
	if i < len(specialFormats) && specialFormats[i].code == r {
		return 65 + i
	}
	return -1
}

func specialCharacterLabel(id int) string {
	if id < 32 {
		return fmt.Sprintf("U+%04X %s", id, c0Names[id])
	}
	if id < 65 {
		return fmt.Sprintf("U+%04X %s", 0x7f+id-32, c1Names[id-32])
	}
	item := specialFormats[id-65]
	return fmt.Sprintf("U+%04X %s", item.code, item.name)
}

// SpecialMarker contains only ordinary values. Cell is the leading native
// caret of the containing cluster, not a fabricated internal cursor stop.
type SpecialMarker struct {
	Cell  geometry.Rectangle
	codes [2]uint64
}

func (m SpecialMarker) Bounds() geometry.Rectangle {
	return geometry.Rect(m.Cell.X-2, m.Cell.Y+max(0, m.Cell.Height-5), 5, min(4, m.Cell.Height))
}

func (m SpecialMarker) Tooltip() string {
	var labels []string
	for id := 0; id < 65+len(specialFormats); id++ {
		if m.codes[id/64]&(uint64(1)<<uint(id%64)) != 0 {
			labels = append(labels, specialCharacterLabel(id))
		}
	}
	return strings.Join(labels, "\n")
}

// SpecialLayout indexes a bounded, already measured paragraph window. The
// owner must discard it when the borrowed Geometry or viewport changes.
type SpecialLayout struct {
	Version, Revision uint64
	Text              string
	TextStart         int
	Clip              geometry.Rectangle
	geometry          textcore.Geometry
	at                int
	markers           []SpecialMarker
	positions         map[geometry.Rectangle]int
}

func NewSpecialLayout(text string, start int, g textcore.Geometry, clip geometry.Rectangle) *SpecialLayout {
	return &SpecialLayout{Text: text, TextStart: start, geometry: g, Clip: clip, positions: make(map[geometry.Rectangle]int)}
}
func (s *SpecialLayout) Pending() bool { return s.at < len(s.Text) }

// Advance scans at most 4096 runes and respects the shared frame deadline.
// Geometry queries use measured values; no native call or text edit occurs.
func (s *SpecialLayout) Advance(deadline time.Time) {
	for work := 0; work < 4096 && s.Pending(); work++ {
		if work%64 == 0 && !deadline.IsZero() && !time.Now().Before(deadline) {
			return
		}
		r, n := utf8.DecodeRuneInString(s.Text[s.at:])
		at := s.TextStart + s.at
		s.at += n
		id := specialCharacterID(r)
		if id < 0 {
			continue
		}
		offset := s.geometry.Snap(at, false)
		cell := s.geometry.Caret(textcore.Position{Offset: offset})
		m := SpecialMarker{Cell: cell}
		if !WhitespaceIntersects(m.Bounds(), s.Clip) {
			continue
		}
		index, ok := s.positions[cell]
		if !ok {
			index = len(s.markers)
			s.positions[cell] = index
			s.markers = append(s.markers, m)
		}
		s.markers[index].codes[id/64] |= uint64(1) << uint(id%64)
	}
	if !s.Pending() {
		s.positions = nil
	}
}

func (s *SpecialLayout) Visit(fn func(SpecialMarker)) {
	for _, m := range s.markers {
		fn(m)
	}
}
func (s *SpecialLayout) Tooltip(point geometry.Point) string {
	if point.X < s.Clip.X || point.X >= s.Clip.X+s.Clip.Width || point.Y < s.Clip.Y || point.Y >= s.Clip.Y+s.Clip.Height {
		return ""
	}
	for _, m := range s.markers {
		r := geometry.Rect(m.Cell.X-3, m.Cell.Y, 7, m.Cell.Height)
		if point.X >= r.X && point.X < r.X+r.Width && point.Y >= r.Y && point.Y < r.Y+r.Height {
			return m.Tooltip()
		}
	}
	return ""
}

func PaintSpecialCharacter(draw gui.Painter, m SpecialMarker, origin geometry.Point, c color.NRGBA) {
	r := m.Bounds().Translate(origin)
	brush := graphics.ColorOf(c)
	a, b, c0, d := geometry.Point{X: r.X + r.Width/2, Y: r.Y}, geometry.Point{X: r.X + r.Width, Y: r.Y + r.Height/2}, geometry.Point{X: r.X + r.Width/2, Y: r.Y + r.Height}, geometry.Point{X: r.X, Y: r.Y + r.Height/2}
	draw.DrawLine(a, b, 1, brush)
	draw.DrawLine(b, c0, 1, brush)
	draw.DrawLine(c0, d, 1, brush)
	draw.DrawLine(d, a, 1, brush)
}
