// Package scheme provides immutable source-code color schemes.
package scheme

import (
	"fmt"
	"image/color"
	"maps"

	"github.com/golang-gui/goui-sourceview/language"
)

// Definition is a mutable input value. New and Scheme.Definition copy its map.
// Colors are ordinary, non-premultiplied sRGB NRGBA values.
type Definition struct {
	Name                        string
	Background                  color.NRGBA
	Foreground                  color.NRGBA
	GutterBackground            color.NRGBA
	GutterForeground            color.NRGBA
	CurrentLine                 color.NRGBA
	SelectionBackground         color.NRGBA
	SelectionForeground         color.NRGBA
	InactiveSelectionBackground color.NRGBA
	SearchBackground            color.NRGBA
	CurrentSearchBackground     color.NRGBA
	// BracketMatchBackground and BracketMatchBorder decorate a confirmed pair.
	// Zero alpha independently disables the fill or border.
	BracketMatchBackground color.NRGBA
	BracketMatchBorder     color.NRGBA
	Caret                  color.NRGBA
	PreeditUnderline       color.NRGBA
	Border                 color.NRGBA
	Completion             CompletionColors
	Tokens                 map[language.Kind]color.NRGBA
}

// CompletionColors styles the suggestion panel independently of text selection.
// Backgrounds and foregrounds are opaque; Border and ScrollThumb may have alpha.
// An entirely zero palette is derived from the scheme's ordinary colors by New.
type CompletionColors struct {
	Background, Foreground, DetailForeground color.NRGBA
	Border                                   color.NRGBA
	SelectedBackground, SelectedForeground   color.NRGBA
	SelectedDetailForeground                 color.NRGBA
	MatchForeground, SelectedMatchForeground color.NRGBA
	ScrollThumb                              color.NRGBA
}

// Scheme is immutable and safe to share.
type Scheme struct{ definition Definition }

// New validates and copies a complete scheme definition.
func New(d Definition) (*Scheme, error) {
	if d.Background.A != 255 || d.Foreground.A != 255 || d.SelectionForeground.A != 255 {
		return nil, fmt.Errorf("scheme: background, foreground and selection foreground must be opaque")
	}
	if d.Completion == (CompletionColors{}) {
		d.Completion = CompletionColors{
			Background: d.Background, Foreground: d.Foreground,
			DetailForeground: mix(d.Foreground, d.Background, 180), Border: d.Border,
			SelectedBackground: mix(d.Foreground, d.Background, 30), SelectedForeground: d.Foreground,
			SelectedDetailForeground: mix(d.Foreground, d.Background, 210),
			MatchForeground:          d.Foreground, SelectedMatchForeground: d.Foreground, ScrollThumb: d.Border,
		}
	}
	for _, c := range []color.NRGBA{d.Completion.Background, d.Completion.Foreground, d.Completion.DetailForeground,
		d.Completion.SelectedBackground, d.Completion.SelectedForeground, d.Completion.SelectedDetailForeground,
		d.Completion.MatchForeground, d.Completion.SelectedMatchForeground} {
		if c.A != 255 {
			return nil, fmt.Errorf("scheme: completion backgrounds and foregrounds must be opaque")
		}
	}
	for kind := range d.Tokens {
		if !kind.Valid() {
			return nil, fmt.Errorf("scheme: unknown token kind %d", kind)
		}
	}
	d.Tokens = maps.Clone(d.Tokens)
	return &Scheme{definition: d}, nil
}

// Definition returns a detached, editable copy.
func (s *Scheme) Definition() Definition {
	d := s.definition
	d.Tokens = maps.Clone(d.Tokens)
	return d
}

// Color returns the token foreground, falling back to the ordinary foreground.
func (s *Scheme) Color(kind language.Kind) color.NRGBA {
	if c, ok := s.definition.Tokens[kind]; ok {
		return c
	}
	return s.definition.Foreground
}

func rgb(v uint32) color.NRGBA {
	return color.NRGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 255}
}

// mix interpolates opaque sRGB colors; it is only a fallback for custom schemes.
func mix(a, b color.NRGBA, weight uint32) color.NRGBA {
	channel := func(x, y uint8) uint8 { return uint8((uint32(x)*weight + uint32(y)*(255-weight) + 127) / 255) }
	return color.NRGBA{R: channel(a.R, b.R), G: channel(a.G, b.G), B: channel(a.B, b.B), A: 255}
}

func builtin(d Definition) *Scheme {
	s, err := New(d)
	if err != nil {
		panic(err)
	}
	return s
}

var light = builtin(Definition{
	Name: "SourceView Light", Background: rgb(0xffffff), Foreground: rgb(0x24292f),
	GutterBackground: rgb(0xf6f8fa), GutterForeground: rgb(0x6e7781), CurrentLine: rgb(0xf3f6fa),
	SelectionBackground: rgb(0xb8d7ff), SelectionForeground: rgb(0x111111), InactiveSelectionBackground: rgb(0xd8dee8),
	SearchBackground: rgb(0xffe58f), CurrentSearchBackground: rgb(0xffd166),
	BracketMatchBackground: rgb(0xe6eef7), BracketMatchBorder: rgb(0x9aafc4),
	Caret: rgb(0x24292f), PreeditUnderline: rgb(0x24292f), Border: rgb(0xd0d7de),
	Completion: CompletionColors{
		Background: rgb(0xf8fafc), Foreground: rgb(0x24292f), DetailForeground: rgb(0x626d7a), Border: rgb(0xb9c3cf),
		SelectedBackground: rgb(0xe3edf9), SelectedForeground: rgb(0x172b44), SelectedDetailForeground: rgb(0x4e627b),
		MatchForeground: rgb(0x005fb8), SelectedMatchForeground: rgb(0x0055a5), ScrollThumb: rgb(0xa6b3c2),
	},
	Tokens: map[language.Kind]color.NRGBA{
		language.KindProperty: rgb(0x005cc5), language.KindString: rgb(0x0a7f39), language.KindEscape: rgb(0x6f42c1),
		language.KindNumber: rgb(0x0550ae), language.KindKeyword: rgb(0x8250df), language.KindDelimiter: rgb(0x57606a),
		language.KindTag: rgb(0x116329), language.KindAttribute: rgb(0x953800), language.KindEntity: rgb(0x8250df),
		language.KindMeta: rgb(0x8250df), language.KindComment: rgb(0x6e7781), language.KindInvalid: rgb(0xcf222e),
	},
})

var dark = builtin(Definition{
	Name: "SourceView Dark", Background: rgb(0x1e1e1e), Foreground: rgb(0xd4d4d4),
	GutterBackground: rgb(0x252526), GutterForeground: rgb(0x858585), CurrentLine: rgb(0x2a2d2e),
	SelectionBackground: rgb(0x264f78), SelectionForeground: rgb(0xffffff), InactiveSelectionBackground: rgb(0x3a3d41),
	SearchBackground: rgb(0x5c4b00), CurrentSearchBackground: rgb(0x805e00),
	BracketMatchBackground: rgb(0x35414d), BracketMatchBorder: rgb(0x667a8d),
	Caret: rgb(0xd4d4d4), PreeditUnderline: rgb(0xd4d4d4), Border: rgb(0x454545),
	Completion: CompletionColors{
		Background: rgb(0x25292f), Foreground: rgb(0xdce2e9), DetailForeground: rgb(0xa0aab7), Border: rgb(0x505b69),
		SelectedBackground: rgb(0x35465e), SelectedForeground: rgb(0xf0f5fc), SelectedDetailForeground: rgb(0xc0ccdc),
		MatchForeground: rgb(0x79bbff), SelectedMatchForeground: rgb(0x96cbff), ScrollThumb: rgb(0x677588),
	},
	Tokens: map[language.Kind]color.NRGBA{
		language.KindProperty: rgb(0x9cdcfe), language.KindString: rgb(0xce9178), language.KindEscape: rgb(0xd7ba7d),
		language.KindNumber: rgb(0xb5cea8), language.KindKeyword: rgb(0x569cd6), language.KindDelimiter: rgb(0xd4d4d4),
		language.KindTag: rgb(0x569cd6), language.KindAttribute: rgb(0x9cdcfe), language.KindEntity: rgb(0xd7ba7d),
		language.KindMeta: rgb(0xc586c0), language.KindComment: rgb(0x6a9955), language.KindInvalid: rgb(0xf44747),
	},
})

// Light returns the shared immutable light scheme.
func Light() *Scheme { return light }

// Dark returns the shared immutable dark scheme.
func Dark() *Scheme { return dark }
