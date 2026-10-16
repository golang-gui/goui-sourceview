package sourceview

import (
	"runtime"
	"strings"
	"testing"

	"github.com/golang-gui/goui-sourceview/scheme"
	"github.com/golang-gui/goui/platform/typography"
)

func TestNativeCompletionEllipsisAndBaseline(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	ctx, err := nativeContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Destroy()
	v := NewSourceView()
	v.SetScheme(scheme.Light())
	v.context = ctx
	s := &completionSession{items: []Completion{{Label: "example"}}}
	l := newCompletionList(v, s)
	s.list = l
	v.completion = s
	defer l.release()
	for _, family := range []string{"monospace", "sans-serif"} {
		for _, prefix := range []string{"e\u0301", "你好", "👩‍💻", "שלום"} {
			t.Run(family+"/"+prefix, func(t *testing.T) {
				b := l.build()
				defer b.destroy()
				font := typography.FontInfo{Family: family, Size: 12}
				reference, err := b.text(prefix+"…", font, v.colors.Completion.Foreground)
				if err != nil {
					t.Fatal(err)
				}
				width := reference.width
				got, err := b.fit(prefix+strings.Repeat("ABC", 30), font, v.colors.Completion.Foreground, width+.01)
				if err != nil || got == nil {
					t.Fatal("fit", err)
				}
				if got.native.Text() != prefix+"…" || got.width > width+.01 || got.kept != len(prefix) {
					t.Fatalf("%s: got %q at width %g/%g", ctx.Name(), got.native.Text(), got.width, width)
				}
				if got.baseline <= 0 || got.baseline > got.height {
					t.Fatalf("%s: baseline %g outside height %g", ctx.Name(), got.baseline, got.height)
				}
			})
		}
	}
}
