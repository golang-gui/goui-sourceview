package language

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"
)

func TestBuiltinFolding(t *testing.T) {
	for _, tt := range []struct {
		id   ID
		text string
		want []FoldRange
	}{
		{JSON, "{\n \"a\":[\n \"} ]\",\n 2\n ]\n}", []FoldRange{{0, 5}, {1, 4}}},
		{JSON, "{\n 1\n}", []FoldRange{{0, 2}}},
		{JSON, "[\n {\n } , \"outside\"\n]", []FoldRange{{0, 3}}},
		{JSON, "[\n}", nil},
		{XML, "<a x=\">\">\n <b/>\n <!-- x\n y -->\n</a>", []FoldRange{{0, 4}, {2, 3}}},
		{XML, "<a>\n text\n</a><b/>", []FoldRange{{0, 1}}},
		{XML, "<a>\n</A>", nil},
		{XML, "<![CDATA[\n<a>\n]]>", []FoldRange{{0, 2}}},
		{HTML, "<DIV>\n<br>\n<script>\nconst x='<fake>'; // <!--\n</script>\n</div>", []FoldRange{{0, 5}, {2, 4}}},
	} {
		t.Run(string(tt.id)+tt.text, func(t *testing.T) {
			l, _ := Builtin(tt.id)
			got, err := l.Folding()(context.Background(), tt.text)
			slices.SortFunc(got, func(a, b FoldRange) int { return a.StartLine - b.StartLine })
			if err != nil || !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %v,%v want %v", got, err, tt.want)
			}
		})
	}
}
func TestFoldingCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, id := range []ID{JSON, XML, HTML} {
		l, _ := Builtin(id)
		_, err := l.Folding()(ctx, "{\n<a>\n}")
		if !errors.Is(err, context.Canceled) {
			t.Fatal(id, err)
		}
	}
}
