// 共享文档订阅与输入法复位替身；不执行真实挂载，不创建窗口或原生资源。
package sourceview

import (
	"testing"

	"github.com/golang-gui/goui/gui"
)

// Bind only the pure document observer; native mount is covered by tests/window.
func observe(t *testing.T, v *SourceView) {
	t.Helper()
	v.mounted = true
	v.document.acquire(v)
	t.Cleanup(func() {
		if v.mounted {
			v.mounted = false
			v.document.release(v)
		}
	})
}

type resetContext struct {
	gui.IMContext
	reset func()
}

func (c *resetContext) Reset() { c.reset() }
