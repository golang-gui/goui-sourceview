package text

import (
	"math/rand"
	"testing"
)

func TestHistoryStateIdentityMergeBranchAndReset(t *testing.T) {
	m := NewModel("")
	initial := m.StateID()
	m.Replace(Range{}, "a")
	first := m.StateID()
	m.Replace(Range{1, 1}, "b")
	merged := m.StateID()
	if initial == first || first == merged || !m.Undo() || m.StateID() != initial || m.Text() != "" {
		t.Fatal("merged undo did not restore before identity")
	}
	if !m.Redo() || m.StateID() != merged || m.Text() != "ab" {
		t.Fatal("merged redo did not restore final identity")
	}
	m.BreakUndoGroup()
	m.Replace(Range{2, 2}, "c")
	branch := m.StateID()
	m.Undo()
	m.Replace(Range{2, 2}, "d")
	if m.StateID() <= branch || m.CanRedo() {
		t.Fatal("new branch reused an abandoned state")
	}
	id, revision := m.StateID(), m.Revision()
	m.SetText(m.Text())
	m.ClearHistory()
	m.Replace(Range{}, "")
	m.ApplyBatch(nil, BatchOptions{})
	if m.StateID() != id || m.Revision() != revision {
		t.Fatal("history clearing or no-op changed state identity")
	}
	if err := m.Replace(Range{-1, 0}, "x"); err == nil || m.StateID() != id {
		t.Fatal("invalid edit changed state identity")
	}
	m.SetText("reset")
	if m.StateID() <= id || m.CanUndo() {
		t.Fatal("reset did not allocate a fresh identity")
	}
}

func TestBatchHistoryStateIdentityAndNotifications(t *testing.T) {
	m := NewModel("\n")
	initial := m.StateID()
	owner := &EditSource{}
	before := []SelectionState{{Selection: Selection{}}, {Selection: Selection{1, 1}}}
	after := []SelectionState{{Selection: Selection{1, 1}}, {Selection: Selection{3, 3}}}
	seen := uint64(0)
	h := m.ConnectChange(func(Change) { seen = m.StateID() })
	defer h.Disconnect()
	if err := m.ApplyBatch([]Edit{{Range: Range{}, Text: "a"}, {Range: Range{1, 1}, Text: "a"}}, BatchOptions{Source: owner, BeforeSelections: before, AfterSelections: after, Merge: MergeTyping}); err != nil {
		t.Fatal(err)
	}
	first := m.StateID()
	if seen != first || first == initial {
		t.Fatal("state not published before change callback")
	}
	if err := m.ApplyBatch([]Edit{{Range: Range{1, 1}, Text: "b"}, {Range: Range{3, 3}, Text: "b"}}, BatchOptions{Source: owner, BeforeSelections: after, AfterSelections: []SelectionState{{Selection: Selection{2, 2}}, {Selection: Selection{5, 5}}}, Merge: MergeTyping}); err != nil {
		t.Fatal(err)
	}
	final := m.StateID()
	if final == first || len(m.undoSteps) != 1 || !m.Undo() || m.StateID() != initial || seen != initial || m.Text() != "\n" {
		t.Fatal("merged batch undo lost state identity")
	}
	if !m.Redo() || m.StateID() != final || seen != final || m.Text() != "ab\nab" {
		t.Fatal("merged batch redo lost state identity")
	}
}

func TestHistoryStateRandomAtomicReference(t *testing.T) {
	m := NewModel("")
	type snapshot struct {
		text string
		id   uint64
	}
	states := []snapshot{{"", m.StateID()}}
	at := 0
	unique := map[uint64]bool{m.StateID(): true}
	rng := rand.New(rand.NewSource(812))
	for i := 0; i < 1000; i++ {
		switch rng.Intn(3) {
		case 0:
			value := states[at].text + string(rune('a'+rng.Intn(26)))
			if err := m.ReplaceAtomic(Range{m.Len(), m.Len()}, value[len(value)-1:]); err != nil {
				t.Fatal(err)
			}
			if unique[m.StateID()] {
				t.Fatal("new edit reused identity", i)
			}
			unique[m.StateID()] = true
			states = append(states[:at+1], snapshot{value, m.StateID()})
			at++
		case 1:
			if m.Undo() {
				at--
			} else if at != 0 {
				t.Fatal("missing undo")
			}
		case 2:
			if m.Redo() {
				at++
			} else if at != len(states)-1 {
				t.Fatal("missing redo")
			}
		}
		if m.Text() != states[at].text || m.StateID() != states[at].id {
			t.Fatal("reference history mismatch", i, at)
		}
	}
}
