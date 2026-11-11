package view

import (
	"slices"
	"testing"

	textcore "github.com/golang-gui/goui-sourceview/internal/text"
	"github.com/golang-gui/goui/platform/typography"
)

type releaseLayout struct {
	typography.TextLayout
	name  string
	order *[]string
}

func (l *releaseLayout) Destroy() { *l.order = append(*l.order, l.name) }

func TestParagraphReleaseOrder(t *testing.T) {
	var order []string
	p := ParagraphResources{
		Layout: &releaseLayout{name: "body", order: &order},
		Number: &releaseLayout{name: "number", order: &order},
	}
	p.Destroy()
	if !slices.Equal(order, []string{"body", "number"}) {
		t.Fatalf("release order: %v", order)
	}
	var empty ParagraphResources
	empty.Destroy()
}

type previewContext struct {
	typography.Context
	t *testing.T
}

func (c *previewContext) NewTextLayout(string, typography.TextFormat, float32, float32) (typography.TextLayout, error) {
	c.t.Fatal("preview prepared a missing native layout")
	return nil, nil
}

func TestPreviewBorrowsMeasuredGeometry(t *testing.T) {
	var absent *ParagraphResources
	if g, h := absent.PreviewGeometry(20); g != nil || h != 20 {
		t.Fatal("missing paragraph lost fallback height", g, h)
	}
	p := &ParagraphResources{}
	if g, h := p.PreviewGeometry(20); g != nil || h != 20 {
		t.Fatal("unmeasured paragraph exposed geometry", g, h)
	}
	p.Geometry = textcore.NewGeometry([]typography.TextLine{{Length: 1, Width: 10, Height: 18}}, nil, 1, 20)
	if g, h := p.PreviewGeometry(20); g != &p.Geometry || h != 18 || p.GeometryAt(0) != g {
		t.Fatal("preview and caret must share measured geometry", g, h)
	}
	// A visible page elsewhere in a long paragraph cannot substitute for its
	// missing first page, nor cause the source reader or native layout to run.
	p.Paged = NewParagraphLayout(&previewContext{t: t}, typography.TextFormat{}, 200, 100000, func(textcore.Range) (string, error) {
		t.Fatal("preview read unmeasured source")
		return "", nil
	})
	defer p.Paged.Destroy()
	if g, h := p.PreviewGeometry(20); g != nil || h != 20 {
		t.Fatal("missing first page used unrelated geometry", g, h)
	}
}
