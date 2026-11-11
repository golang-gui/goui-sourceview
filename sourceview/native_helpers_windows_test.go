package sourceview

import (
	"github.com/golang-gui/goui/platform/typography"
	"github.com/golang-gui/goui/platform/typography/directwrite"
)

func nativeContext() (typography.Context, error) { return directwrite.NewContext() }
