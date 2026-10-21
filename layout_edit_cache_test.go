package sourceview

import (
	"testing"
	"time"

	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/gui/textedit"
)

func TestUncollapsedFoldCandidatesPreserveLayoutAfterExternalEdit(t *testing.T) {
	for _, insertion := range []string{"共享", "new\n"} {
		t.Run(insertion, func(t *testing.T) {
			v, scroll := revealFixture(t)
			v.document.engine.Advance(time.Time{})
			v.acceptFolds([]language.FoldRange{{StartLine: 0, EndLine: 199}})
			v.frameLayout = true
			settle := func() {
				for range 20 {
					scroll.Arrange(scroll.Rect())
					v.layoutStep(time.Time{})
				}
				if v.layoutPending || v.reveal != nil || v.pendingAnchor {
					t.Fatal("external edit/reveal did not settle")
				}
			}
			r, _ := v.Document().Model().LineRange(180)
			v.SetSelection(textedit.Selection{Anchor: r.Start, Caret: r.Start})
			settle()
			paragraph := v.cachedParagraph(180)
			if paragraph == nil {
				t.Fatal("destination not prepared")
			}
			// This offscreen measured height must survive an inserted LF too;
			// visible cache hits alone must not conceal a full height reset.
			v.paragraph(100)
			if err := v.Document().Model().ReplaceAtomic(textedit.Range{}, insertion); err != nil {
				t.Fatal(err)
			}
			shift := 0
			if insertion == "new\n" {
				shift = 1
			}
			if v.cachedParagraph(180+shift) != paragraph {
				t.Fatal("uncollapsed fold metadata discarded an unchanged suffix layout")
			}
			settle()
			if got := v.paragraphTop(101+shift) - v.paragraphTop(100+shift); got != 32 {
				t.Fatal("line-count mapping reset an offscreen measured height", got)
			}
			caret, ok := v.cachedCaretRect()
			if !ok || caret.Y < 0 || caret.Y+caret.Height > v.viewport.Height {
				t.Fatal("external edit moved the settled caret out of view", caret, v.viewport)
			}
		})
	}
}
