package sourceview

import (
	"github.com/golang-gui/goui/platform/typography"
	"github.com/golang-gui/goui/platform/typography/coretext"
)

func nativeContext() (typography.Context, error) { return coretext.NewContext() }
