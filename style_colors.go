package sourceview

import "image/color"

type completionColors struct {
	Background, Foreground, DetailForeground color.NRGBA
	Border                                   color.NRGBA
	SelectedBackground, SelectedForeground   color.NRGBA
	SelectedDetailForeground                 color.NRGBA
	MatchForeground, SelectedMatchForeground color.NRGBA
	ScrollThumb                              color.NRGBA
}

type findColors struct {
	Background, Foreground, MutedForeground           color.NRGBA
	Border, InputBackground, InputBorder, FocusBorder color.NRGBA
	HoverBackground, OptionBackground, OptionBorder   color.NRGBA
	ErrorForeground, Shadow                           color.NRGBA
}
