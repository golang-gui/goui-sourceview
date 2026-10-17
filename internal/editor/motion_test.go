package editor

import (
	"strings"
	"testing"

	"github.com/golang-gui/goui/gui/textedit"
)

func TestCodeWordBoundaries(t *testing.T) {
	for _, tt := range []struct {
		text            string
		at              int
		right, deleting bool
		want            int
	}{
		{"foo.bar", 3, true, false, 7}, {"foo.bar", 4, false, false, 0},
		{"foo.bar", 4, false, true, 3}, {"foo.bar", 3, true, true, 4},
		{"foo  bar", 3, true, false, 8}, {"foo  bar", 3, true, true, 5},
		{"foo  bar", 5, false, false, 0}, {"foo  bar", 5, false, true, 3},
		{"foo bar", 4, false, true, 0}, {"foo bar", 3, true, true, 7},
		{"a\n  beta", 1, true, false, 8}, {"a\n  beta", 1, true, true, 4},
		{"a\nbeta", 2, false, false, 0}, {"a\nbeta", 2, false, true, 1},
		{"a\n\nb", 1, true, false, 2}, {"a\n\nb", 3, false, false, 2},
		{"中文_name e\u0301", 0, true, false, 11}, {"中文_name e\u0301", 15, false, false, 12},
		{"", 0, true, false, 0}, {"a", 0, false, true, 0}, {"a", 1, true, true, 1},
	} {
		m := textedit.NewModel(tt.text)
		if got := WordBoundary(m, tt.at, tt.right, tt.deleting); got != tt.want {
			t.Errorf("%q at %d right=%v delete=%v: got %d want %d", tt.text, tt.at, tt.right, tt.deleting, got, tt.want)
		}
	}
	m := textedit.NewModel(strings.Repeat("中", 5000) + " end")
	if got := WordBoundary(m, 0, true, false); got != 15000 {
		t.Fatal("chunked forward", got)
	}
	if got := WordBoundary(m, 15000, false, false); got != 0 {
		t.Fatal("chunked backward", got)
	}
}

func TestInsertLineRetainsTextAndHistory(t *testing.T) {
	for _, tt := range []struct {
		text  string
		s     textedit.Selection
		below bool
		want  string
		caret int
	}{
		{"  alpha\nbeta", textedit.Selection{Anchor: 9, Caret: 4}, false, "  \n  alpha\nbeta", 2},
		{"\talpha", textedit.Selection{Anchor: 3, Caret: 3}, true, "\talpha\n\t", 8},
		{"a\n", textedit.Selection{Anchor: 2, Caret: 2}, true, "a\n\n", 3},
		{"", textedit.Selection{}, false, "\n", 0},
	} {
		m := textedit.NewModel(tt.text)
		p, err := InsertLine(m, tt.s, tt.below)
		if err != nil {
			t.Fatal(err)
		}
		if err = p.Apply(m, nil); err != nil {
			t.Fatal(err)
		}
		if m.Text() != tt.want || p.After != (textedit.Selection{Anchor: tt.caret, Caret: tt.caret}) {
			t.Fatal(m.Text(), p.After)
		}
		undo := m.UndoWithSelection()
		if !undo.Applied || m.Text() != tt.text || *undo.Selection != tt.s {
			t.Fatal("undo", undo)
		}
		redo := m.RedoWithSelection()
		if !redo.Applied || m.Text() != tt.want || *redo.Selection != p.After {
			t.Fatal("redo", redo)
		}
	}
}
