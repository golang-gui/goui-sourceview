package editor

import (
	"math/rand"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/golang-gui/goui/gui/textedit"
)

type lineOperation struct {
	name string
	plan func(*textedit.Model, textedit.Selection) (Plan, error)
}

var lineOperations = []lineOperation{
	{"up", func(m *textedit.Model, s textedit.Selection) (Plan, error) { return MoveLines(m, s, false) }},
	{"down", func(m *textedit.Model, s textedit.Selection) (Plan, error) { return MoveLines(m, s, true) }},
	{"copy up", func(m *textedit.Model, s textedit.Selection) (Plan, error) { return DuplicateLines(m, s, false) }},
	{"copy down", func(m *textedit.Model, s textedit.Selection) (Plan, error) { return DuplicateLines(m, s, true) }},
	{"delete", DeleteLines},
}

func TestLineOperationsLiteralBoundariesAndHistory(t *testing.T) {
	cases := []struct {
		name, op, text string
		before         textedit.Selection
		want           string
		after          textedit.Selection
		noop           bool
	}{
		{"move interior up", "up", "aa\n中x\nlast", textedit.Selection{Anchor: 6, Caret: 6}, "中x\naa\nlast", textedit.Selection{Anchor: 3, Caret: 3}, false},
		{"move first down", "down", "a\nb", textedit.Selection{}, "b\na", textedit.Selection{Anchor: 2, Caret: 2}, false},
		{"exclude next line reversed", "down", "a\nb\nc", textedit.Selection{Anchor: 2, Caret: 0}, "b\na\nc", textedit.Selection{Anchor: 4, Caret: 2}, false},
		{"exclude final next line", "down", "a\nb", textedit.Selection{Caret: 2}, "b\na", textedit.Selection{Anchor: 2, Caret: 3}, false},
		{"final empty up", "up", "a\nb\n", textedit.Selection{Anchor: 4, Caret: 4}, "a\n\nb", textedit.Selection{Anchor: 2, Caret: 2}, false},
		{"empty down", "down", "\n中", textedit.Selection{}, "中\n", textedit.Selection{Anchor: 4, Caret: 4}, false},
		{"identical empty down", "down", "\n", textedit.Selection{}, "\n", textedit.Selection{Anchor: 1, Caret: 1}, false},
		{"identical empty up", "up", "\n", textedit.Selection{Anchor: 1, Caret: 1}, "\n", textedit.Selection{}, false},
		{"top boundary", "up", "a\nb", textedit.Selection{Anchor: 1, Caret: 1}, "a\nb", textedit.Selection{Anchor: 1, Caret: 1}, true},
		{"bottom boundary", "down", "a\nb", textedit.Selection{Anchor: 2, Caret: 2}, "a\nb", textedit.Selection{Anchor: 2, Caret: 2}, true},
		{"all rows boundary", "down", "a\nb", textedit.Selection{Caret: 3}, "a\nb", textedit.Selection{Caret: 3}, true},
		{"copy terminal below", "copy down", "a\nb", textedit.Selection{Anchor: 3, Caret: 3}, "a\nb\nb", textedit.Selection{Anchor: 5, Caret: 5}, false},
		{"copy before newline", "copy down", "a\n", textedit.Selection{Caret: 2}, "a\na\n", textedit.Selection{Anchor: 2, Caret: 4}, false},
		{"copy reversed above", "copy up", "a\nb\nc", textedit.Selection{Anchor: 4, Caret: 1}, "a\nb\na\nb\nc", textedit.Selection{Anchor: 4, Caret: 1}, false},
		{"copy empty above", "copy up", "", textedit.Selection{}, "\n", textedit.Selection{}, false},
		{"copy empty below", "copy down", "", textedit.Selection{}, "\n", textedit.Selection{Anchor: 1, Caret: 1}, false},
		{"delete first", "delete", "aa\nbb\ncc", textedit.Selection{Anchor: 1, Caret: 1}, "bb\ncc", textedit.Selection{}, false},
		{"delete final", "delete", "aa\nbb", textedit.Selection{Anchor: 4, Caret: 4}, "aa", textedit.Selection{Anchor: 2, Caret: 2}, false},
		{"delete final empty", "delete", "a\n", textedit.Selection{Anchor: 2, Caret: 2}, "a", textedit.Selection{Anchor: 1, Caret: 1}, false},
		{"delete excluded next", "delete", "a\nb\nc", textedit.Selection{Anchor: 4, Caret: 0}, "c", textedit.Selection{}, false},
		{"delete all", "delete", "中\nlast", textedit.Selection{Caret: 8}, "", textedit.Selection{}, false},
		{"delete empty", "delete", "", textedit.Selection{}, "", textedit.Selection{}, true},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			m := textedit.NewModel(tt.text)
			var operation lineOperation
			for _, op := range lineOperations {
				if op.name == tt.op {
					operation = op
				}
			}
			p, err := operation.plan(m, tt.before)
			if err != nil {
				t.Fatal(err)
			}
			if p.Before != tt.before || p.After != tt.after {
				t.Fatalf("selection: %+v, want %+v", p, tt.after)
			}
			notifications := 0
			m.ConnectChange(func(textedit.Change) { notifications++ })
			if err := p.Apply(m, &textedit.EditSource{}); err != nil {
				t.Fatal(err)
			}
			if m.Text() != tt.want {
				t.Fatalf("text %q, want %q", m.Text(), tt.want)
			}
			if tt.noop {
				if notifications != 0 || m.Revision() != 0 || m.CanUndo() {
					t.Fatal("edge/no-op changed history")
				}
				return
			}
			if notifications != 1 || m.Revision() != 1 {
				t.Fatal("line operation was not atomic", notifications, m.Revision())
			}
			undo := m.UndoWithSelection()
			if !undo.Applied || undo.Selection == nil || *undo.Selection != tt.before || m.Text() != tt.text || m.CanUndo() {
				t.Fatal("undo did not restore one complete operation", undo, m.Text())
			}
			redo := m.RedoWithSelection()
			if !redo.Applied || redo.Selection == nil || *redo.Selection != tt.after || m.Text() != tt.want {
				t.Fatal("redo did not restore text and direction", redo, m.Text())
			}
		})
	}
}

// This reference operates on slices of whole strings and reconstructs offsets
// from line/byte-column pairs. It does not reuse the production edit algorithm.
func referenceLineOperation(text string, s textedit.Selection, op string) (string, textedit.Selection) {
	rows := strings.Split(text, "\n")
	r := s.Range()
	first, last := strings.Count(text[:r.Start], "\n"), strings.Count(text[:r.End], "\n")
	if r.End > r.Start && text[r.End-1] == '\n' {
		last--
	}
	target := first
	var result []string
	switch op {
	case "up":
		if first == 0 {
			return text, s
		}
		result = append(result, rows[:first-1]...)
		result = append(result, rows[first:last+1]...)
		result = append(result, rows[first-1])
		result = append(result, rows[last+1:]...)
		target--
	case "down":
		if last == len(rows)-1 {
			return text, s
		}
		result = append(result, rows[:first]...)
		result = append(result, rows[last+1])
		result = append(result, rows[first:last+1]...)
		result = append(result, rows[last+2:]...)
		target++
	case "copy up", "copy down":
		at := first
		if op == "copy down" {
			at = last + 1
			target = at
		}
		result = append(result, rows[:at]...)
		result = append(result, rows[first:last+1]...)
		result = append(result, rows[at:]...)
	case "delete":
		result = append(result, rows[:first]...)
		result = append(result, rows[last+1:]...)
		remaining := strings.Join(result, "\n")
		at := 0
		for i := 0; i < first && i < len(result); i++ {
			at += len(result[i]) + 1
		}
		at = min(at, len(remaining))
		return remaining, textedit.Selection{Anchor: at, Caret: at}
	}
	value := strings.Join(result, "\n")
	position := func(old int) int {
		row := strings.Count(text[:old], "\n")
		start := strings.LastIndex(text[:old], "\n") + 1
		wantedRow := target + row - first
		at := 0
		for i := 0; i < wantedRow && i < len(result); i++ {
			at += len(result[i]) + 1
		}
		return min(len(value), at+old-start)
	}
	return value, textedit.Selection{Anchor: position(s.Anchor), Caret: position(s.Caret)}
}
func TestLineOperationsAgainstWholeLineReference(t *testing.T) {
	rng := rand.New(rand.NewSource(71931))
	samples := []string{"", "a", "中文", "e\u0301", "\t foo", "👩‍💻"}
	for i := 0; i < 600; i++ {
		rows := make([]string, 1+rng.Intn(8))
		for j := range rows {
			rows[j] = samples[rng.Intn(len(samples))]
		}
		text := strings.Join(rows, "\n")
		boundaries := []int{len(text)}
		for at := range text {
			boundaries = append(boundaries, at)
		}
		slices.Sort(boundaries)
		s := textedit.Selection{Anchor: boundaries[rng.Intn(len(boundaries))], Caret: boundaries[rng.Intn(len(boundaries))]}
		for _, op := range lineOperations {
			m := textedit.NewModel(text)
			p, err := op.plan(m, s)
			if err != nil {
				t.Fatal(err)
			}
			want, selection := referenceLineOperation(text, s, op.name)
			if err := p.Apply(m, nil); err != nil {
				t.Fatalf("%s %q %+v: %v", op.name, text, s, err)
			}
			if m.Text() != want || p.After != selection || !utf8.ValidString(m.Text()) {
				t.Fatalf("%s %q %+v: got %q %+v; want %q %+v", op.name, text, s, m.Text(), p.After, want, selection)
			}
		}
	}
}
func TestLineOperationValidationAndInsertBudget(t *testing.T) {
	m := textedit.NewModel("中\nx")
	for _, s := range []textedit.Selection{{Anchor: -1}, {Caret: 9}, {Anchor: 1, Caret: 3}} {
		for _, op := range lineOperations {
			if _, err := op.plan(m, s); err == nil {
				t.Fatal("accepted invalid selection", op.name, s)
			}
		}
	}
	huge := textedit.NewModel(strings.Repeat("x", 32<<20) + "\nz")
	for _, plan := range []func() (Plan, error){
		func() (Plan, error) { return DuplicateLines(huge, textedit.Selection{}, true) },
		func() (Plan, error) {
			return MoveLines(huge, textedit.Selection{Anchor: huge.Len(), Caret: huge.Len()}, false)
		},
	} {
		if _, err := plan(); err != ErrOperationLimit {
			t.Fatal("missing operation budget", err)
		}
	}
	if huge.Revision() != 0 || huge.CanUndo() {
		t.Fatal("rejected operation mutated model")
	}
}
