package sourceview

import (
	"github.com/golang-gui/goui/platform/typography"
	"github.com/golang-gui/goui/platform/typography/pango"
)

func nativeContext() (typography.Context, error) { return pango.NewContext() }
