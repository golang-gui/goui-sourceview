package folding

import (
	"slices"
	"testing"

	"github.com/golang-gui/goui-sourceview/internal/highlight"
	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/gui/textedit"
)

func TestSnapshotUsesNormalizedByteBoundaries(t *testing.T) {
	ranges := []language.FoldRange{{StartLine: 0, EndLine: 1}, {StartLine: 0, EndLine: 2}}
	snapshot, err := BuildSnapshot("a\r\nb\rc\n", ranges)
	want := []Entry{{Region: Region{StartLine: 0, EndLine: 2}, Start: 0, End: 5}}
	if err != nil || snapshot.Length != 6 || snapshot.Lines != 4 || !slices.Equal(snapshot.Entries, want) {
		t.Fatalf("normalized snapshot: %+v, err %v", snapshot, err)
	}
	ranges[1].EndLine = 3
	if !slices.Equal(snapshot.Entries, want) {
		t.Fatal("snapshot retained mutable input ranges")
	}
	if _, err := BuildSnapshot("a\nb\nc\nd", []language.FoldRange{{StartLine: 0, EndLine: 2}, {StartLine: 1, EndLine: 3}}); err == nil {
		t.Fatal("crossing ranges accepted")
	}
	empty, err := BuildSnapshot("", nil)
	if err != nil || empty.Length != 0 || empty.Lines != 1 || len(empty.Entries) != 0 {
		t.Fatal("empty document snapshot", empty, err)
	}
}

func TestEntryMappingKeepsSequentialCoordinatesAndBoundaries(t *testing.T) {
	initial := Entry{Region: Region{StartLine: 2, EndLine: 4, Collapsed: true}, Start: 10, End: 20}
	shifted := Entry{Region: Region{StartLine: 3, EndLine: 5, Collapsed: true}, Start: 11, End: 21}
	lineAdded := highlight.Splice{Removed: 1, Added: 2}
	for _, tc := range []struct {
		name    string
		edits   []textedit.Edit
		splices []highlight.Splice
		want    []Entry
	}{
		{"before", []textedit.Edit{{Text: "\n"}}, []highlight.Splice{lineAdded}, []Entry{shifted}},
		{"newline-at-header", []textedit.Edit{{Range: textedit.Range{Start: 10, End: 10}, Text: "\n"}}, []highlight.Splice{lineAdded}, []Entry{shifted}},
		{"text-at-header", []textedit.Edit{{Range: textedit.Range{Start: 10, End: 10}, Text: "x"}}, []highlight.Splice{{}}, nil},
		{"end-boundary", []textedit.Edit{{Range: textedit.Range{Start: 20, End: 21}}}, []highlight.Splice{{}}, nil},
		{"descending-batch", []textedit.Edit{{Range: textedit.Range{Start: 50, End: 50}, Text: "x"}, {Text: "\n"}}, []highlight.Splice{{}, lineAdded}, []Entry{shifted}},
		{"ascending-sequential-batch", []textedit.Edit{{Text: "\n"}, {Range: textedit.Range{Start: 51, End: 51}, Text: "x"}}, []highlight.Splice{lineAdded, {}}, []Entry{shifted}},
		{"arbitrary-order-fallback", []textedit.Edit{{Range: textedit.Range{Start: 30, End: 30}, Text: "x"}, {Text: "\n"}, {Range: textedit.Range{Start: 50, End: 50}, Text: "y"}}, []highlight.Splice{{}, lineAdded, {}}, []Entry{shifted}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := MapEntries([]Entry{initial}, textedit.Change{Edits: tc.edits}, tc.splices, true, func(int) int {
				t.Fatal("incremental mapping unexpectedly queried model rows")
				return 0
			})
			if !slices.Equal(got, tc.want) {
				t.Fatalf("mapped %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestMappingWithoutSplicesUsesCurrentModelRows(t *testing.T) {
	m := textedit.NewModel("0\n1\n2\n3\n4\n5")
	initial := Entry{Region: Region{StartLine: 2, EndLine: 4, Collapsed: true}, Start: 4, End: 9}
	edit := textedit.Edit{Text: "X\n"}
	if err := m.Replace(edit.Range, edit.Text); err != nil {
		t.Fatal(err)
	}
	got := MapEntries([]Entry{initial}, textedit.Change{Edits: []textedit.Edit{edit}}, nil, false, m.LineAt)
	want := []Entry{{Region: Region{StartLine: 3, EndLine: 5, Collapsed: true}, Start: 6, End: 11}}
	if !slices.Equal(got, want) {
		t.Fatalf("mapped %+v, want %+v", got, want)
	}
}

func TestReconcileKeepsIntentWithoutSharingMutableEntries(t *testing.T) {
	m := textedit.NewModel("a\nb\nc\nd")
	old := []Entry{{Region: Region{StartLine: 0, EndLine: 2, Collapsed: true}, Start: 0, End: 5}}
	got := Reconcile(old, []language.FoldRange{{StartLine: 0, EndLine: 2}}, m.LineRange)
	if !slices.Equal(got, old) {
		t.Fatal("identical endpoints lost collapse intent", got)
	}
	got[0].Collapsed = false
	if !old[0].Collapsed {
		t.Fatal("reconciled view mutated the original entries")
	}
	got = Reconcile(old, []language.FoldRange{{StartLine: 1, EndLine: 3}}, m.LineRange)
	if len(got) != 1 || got[0].Collapsed || got[0].Start != 2 || got[0].End != 7 {
		t.Fatal("different endpoints inherited collapse intent", got)
	}
}

func TestHiddenRangesMergeNestedEntries(t *testing.T) {
	got := Hidden([]Entry{
		{Region: Region{StartLine: 0, EndLine: 4, Collapsed: true}},
		{Region: Region{StartLine: 1, EndLine: 2, Collapsed: true}},
		{Region: Region{StartLine: 5, EndLine: 7, Collapsed: true}},
		{Region: Region{StartLine: 7, EndLine: 9}},
	})
	if !slices.Equal(got, []Interval{{Start: 1, End: 5}, {Start: 6, End: 8}}) {
		t.Fatal("nested or expanded entries changed hidden ranges", got)
	}
}
