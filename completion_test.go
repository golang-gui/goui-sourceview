package sourceview

import (
	"context"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
)

func TestLanguageAndDocumentCompletions(t *testing.T) {
	for _, tt := range []struct {
		id           language.ID
		text         string
		at           int
		prefix, want string
	}{
		{language.JSON, "tr", 2, "tr", "true"},
		{language.HTML, "<di", 3, "di", "div"},
		{language.HTML, `<a cl`, 5, "cl", "class"},
		{language.XML, `<x xml:l`, 8, "xml:l", "xml:lang"},
		{language.XML, "<widget/>\n<wid", 14, "wid", "widget"},
		{language.Plain, "你好世界 你好", len("你好世界 你好"), "你好", "你好世界"},
	} {
		t.Run(tt.text, func(t *testing.T) {
			v := NewSourceView()
			v.document.model.SetText(tt.text)
			_ = v.document.SetLanguage(tt.id)
			observe(t, v)
			v.document.engine.Advance(time.Time{})
			v.SetSelection(textedit.Selection{Anchor: tt.at, Caret: tt.at})
			c, ok := v.completionContext(false)
			if !ok || c.Prefix != tt.prefix {
				t.Fatal(c, ok)
			}
			items := normalizeCompletions(v.proposals(c), c)
			if !slices.ContainsFunc(items, func(i Completion) bool { return i.Text == tt.want }) {
				t.Fatal(items, tt.want)
			}
		})
	}
}

type closingPopup struct {
	gui.Popover
	onClose func()
}

func (p closingPopup) Destroy() { p.onClose() }

func TestCompletionCloseCallbackCannotApplyStaleRange(t *testing.T) {
	v := NewSourceView()
	observe(t, v)
	v.document.model.SetText("tr")
	v.SetSelection(textedit.Selection{Anchor: 2, Caret: 2})
	original := v.document
	replacement := NewDocument("keep")
	s := &completionSession{state: v.operationState(), range_: textedit.Range{Start: 0, End: 2}, items: []Completion{{Text: "true"}}, list: &completionList{}}
	s.popup = closingPopup{onClose: func() { v.SetDocument(replacement) }}
	v.completion = s
	if err := v.acceptCompletion(s, 0); err != ErrOperationChanged {
		t.Fatal(err)
	}
	if v.document != replacement || replacement.model.Text() != "keep" || original.model.Text() != "tr" {
		t.Fatal("stale completion modified document")
	}
}

func TestCompletionReplacesWholeWordAndBoundsResults(t *testing.T) {
	v := NewSourceView()
	v.document.model.SetText("alpha tail")
	v.SetSelection(textedit.Selection{Anchor: 2, Caret: 2})
	c, ok := v.completionContext(true)
	if !ok || c.Prefix != "al" || c.Range != (textedit.Range{Start: 0, End: 5}) {
		t.Fatal(c, ok)
	}
	items := normalizeCompletions([]Completion{{Label: "alpha"}, {Label: "alpha"}, {Label: "alpine"}, {Label: "al\nbad"}, {Label: "z"}}, c)
	if len(items) != 2 || items[0].Text != "alpha" || items[1].Text != "alpine" {
		t.Fatal(items)
	}
	v.SetSelection(textedit.Selection{Caret: 2})
	if _, ok := v.completionContext(true); ok {
		t.Fatal("selection offered completion")
	}
	if v.ShowCompletion() != ErrNotMounted {
		t.Fatal("unmounted completion")
	}
}

func TestCompletionLabelsStayOnOneNativeLine(t *testing.T) {
	for _, separator := range []string{"\r", "\n", "\t", "\v", "\f", "\u0085", "\u2028", "\u2029"} {
		items := normalizeCompletions([]Completion{{Label: "a" + separator + "b"}}, CompletionContext{})
		if len(items) != 0 {
			t.Fatalf("accepted native line separator %q", separator)
		}
	}
}

func TestConfigurationAndShellCompletions(t *testing.T) {
	for _, tc := range []struct {
		id         language.ID
		text, want string
	}{
		{language.TOML, "enabled = tr", "true"}, {language.YAML, "enabled: nu", "null"}, {language.Shell, "pri", "printf"},
	} {
		v := NewSourceView()
		v.document.model.SetText(tc.text)
		_ = v.document.SetLanguage(tc.id)
		observe(t, v)
		v.document.engine.Advance(time.Time{})
		v.SetSelection(textedit.Selection{Anchor: len(tc.text), Caret: len(tc.text)})
		c, ok := v.completionContext(true)
		if !ok {
			t.Fatal(tc)
		}
		items := normalizeCompletions(v.proposals(c), c)
		if !slices.ContainsFunc(items, func(i Completion) bool { return i.Text == tc.want }) {
			t.Fatal(tc, items)
		}
	}
	for _, tc := range []struct {
		id   language.ID
		text string
	}{
		{language.TOML, "# true tr"}, {language.TOML, `s = "true tr`}, {language.YAML, "text: |\n  true tr"}, {language.Shell, "cat <<END\nthen th"},
	} {
		v := NewSourceView()
		v.document.model.SetText(tc.text)
		_ = v.document.SetLanguage(tc.id)
		observe(t, v)
		v.document.engine.Advance(time.Time{})
		v.SetSelection(textedit.Selection{Anchor: len(tc.text), Caret: len(tc.text)})
		c, ok := v.completionContext(true)
		if !ok {
			t.Fatal(tc)
		}
		if got := v.proposals(c); len(got) != 0 {
			t.Fatal("suggestions inside literal", tc, got)
		}
	}
}

func TestCompletionEditsAndHistory(t *testing.T) {
	v := NewSourceView()
	observe(t, v)
	m := v.document.model
	m.SetText("use\nfo tail")
	before := textedit.Selection{Anchor: 6, Caret: 6}
	v.SetSelection(before)
	r := textedit.Range{Start: 4, End: 6}
	caret := 3
	session := &completionSession{state: v.operationState(), range_: r, items: []Completion{{Text: "foo()", Range: &r, Caret: &caret, AdditionalEdits: []textedit.Edit{{Range: textedit.Range{}, Text: "import x\n"}}}}, list: &completionList{}, popup: closingPopup{onClose: func() {}}}
	v.completion = session
	if err := v.acceptCompletion(session, 0); err != nil {
		t.Fatal(err)
	}
	if m.Text() != "import x\nuse\nfoo() tail" || v.Selection().Caret != 16 {
		t.Fatal(m.Text(), v.Selection())
	}
	if !v.Undo() || m.Text() != "use\nfo tail" || v.Selection() != before {
		t.Fatal("completion was not one history unit", m.Text(), v.Selection())
	}
	if !v.Redo() || v.Selection().Caret != 16 {
		t.Fatal("completion redo")
	}
}

func TestProviderOwnsOrderingAndFiltering(t *testing.T) {
	items := sanitizeCompletions([]Completion{{Label: "zebra"}, {Label: "alpha"}, {Label: "zebra"}})
	if len(items) != 3 || items[0].Text != "zebra" || items[1].Text != "alpha" {
		t.Fatal(items)
	}
	if got := sanitizeCompletions(nil); len(got) != 0 {
		t.Fatal(got)
	}
}

func TestCompletionWorkerSerializesAndReleasesCancelledState(t *testing.T) {
	v := NewSourceView()
	q := &completionQueue{owner: v}
	posts := make(chan func(), 4)
	post := func(f func()) { posts <- f }
	gate := make(chan struct{})
	started := make(chan struct{}, 2)
	var calls atomic.Int32
	fn := func(ctx context.Context, c CompletionContext) ([]Completion, error) {
		calls.Add(1)
		started <- struct{}{}
		if c.Prefix == "first" {
			<-gate
		}
		return []Completion{{Label: c.Prefix}}, nil
	}
	job := func(prefix string) *completionJob {
		ctx, cancel := context.WithCancel(context.Background())
		return &completionJob{state: v.operationState(), request: CompletionContext{Prefix: prefix}, complete: fn, context: ctx, cancel: cancel}
	}
	first := job("first")
	q.start(first, post)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("worker not started")
	}
	q.cancel()
	if q.active != nil || first.context.Err() != context.Canceled {
		t.Fatal("cancel retained operation state")
	}
	q.next = job("latest")
	if calls.Load() != 1 {
		t.Fatal("concurrent worker")
	}
	close(gate)
	select {
	case f := <-posts:
		f()
	case <-time.After(time.Second):
		t.Fatal("no completion post")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("latest not started")
	}
	select {
	case f := <-posts:
		f()
	case <-time.After(time.Second):
		t.Fatal("latest did not finish")
	}
	if calls.Load() != 2 || q.busy || q.pending() {
		t.Fatal("queue did not drain")
	}
}

func TestStableProviderPreservesSessionAndReplacementCancels(t *testing.T) {
	v := NewSourceView()
	first, second := &CompletionProvider{}, &CompletionProvider{}
	v.SetCompletionProvider(first)
	closed := 0
	session := &completionSession{popup: closingPopup{onClose: func() { closed++ }}, list: &completionList{}}
	v.completion = session
	v.SetCompletionProvider(first)
	if v.completion != session || closed != 0 {
		t.Fatal("stable configuration closed session")
	}
	v.SetCompletionProvider(second)
	if v.completion != nil || closed != 1 {
		t.Fatal("replacement retained session")
	}
}

func TestDefaultWordPolicyBelongsToDefinition(t *testing.T) {
	l, err := language.New(language.Definition{Info: language.Info{ID: language.Shell, Name: "Application language"}})
	if err != nil {
		t.Fatal(err)
	}
	c := CompletionContext{Language: language.Shell, Definition: l, Lexical: language.Context{Comment: true}, ContextKnown: true, NearbyText: "foobar fo", DocumentLength: 9, Caret: 9, Range: textedit.Range{Start: 7, End: 9}, Prefix: "fo"}
	if items := DefaultCompletions(c); len(items) != 1 || items[0].Text != "foobar" {
		t.Fatal("inferred language policy from ID", items)
	}
	c.Definition, _ = language.Builtin(language.Shell)
	if items := DefaultCompletions(c); len(items) != 0 {
		t.Fatal("builtin literal policy lost", items)
	}
}
