package sourceview

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/gui/textedit"
)

// CRLF 只能算一个逻辑换行；文件末尾的换行数量与输出格式独立保留。
func TestDocumentLineEndingRoundTrips(t *testing.T) {
	for _, tc := range []struct {
		name, input, normalized, output string
		ending                          LineEnding
		mixed                           bool
		lines                           int
	}{
		{"empty", "", "", "", LineEndingLF, false, 1},
		{"no newline", "abc", "abc", "abc", LineEndingLF, false, 1},
		{"LF", "a\nb\n", "a\nb\n", "a\nb\n", LineEndingLF, false, 3},
		{"CRLF no final newline", "a\r\nb", "a\nb", "a\r\nb", LineEndingCRLF, false, 2},
		{"CRLF final newline", "abc\r\n", "abc\n", "abc\r\n", LineEndingCRLF, false, 2},
		{"CRLF final empty lines", "abc\r\n\r\n", "abc\n\n", "abc\r\n\r\n", LineEndingCRLF, false, 3},
		{"CRLF only", "\r\n", "\n", "\r\n", LineEndingCRLF, false, 2},
		{"CR", "中\r文\r", "中\n文\n", "中\r文\r", LineEndingCR, false, 3},
		{"LF majority", "a\r\nb\nc\n", "a\nb\nc\n", "a\nb\nc\n", LineEndingLF, true, 4},
		{"CRLF majority", "a\nb\r\nc\r\n", "a\nb\nc\n", "a\r\nb\r\nc\r\n", LineEndingCRLF, true, 4},
		{"tie first CRLF", "a\r\nb\n", "a\nb\n", "a\r\nb\r\n", LineEndingCRLF, true, 3},
		{"tie first LF", "a\nb\r\n", "a\nb\n", "a\nb\n", LineEndingLF, true, 3},
		{"dominant tie first CR", "a\nb\rc\r\nd\re\r\n", "a\nb\nc\nd\ne\n", "a\rb\rc\rd\re\r", LineEndingCR, true, 6},
		{"CR before CRLF", "a\r\r\n", "a\n\n", "a\r\r", LineEndingCR, true, 3},
		{"other Unicode separators", "a\u0085b\u2028c", "a\u0085b\u2028c", "a\u0085b\u2028c", LineEndingLF, false, 1},
		{"invalid UTF8", "\xff\r\n", "\ufffd\n", "\ufffd\r\n", LineEndingCRLF, false, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ending, mixed := DetectLineEnding(tc.input)
			if ending != tc.ending || mixed != tc.mixed {
				t.Fatal("detection", ending, mixed)
			}
			d := NewDocument(tc.input)
			if d.LineEnding() != tc.ending || d.Model().Text() != tc.normalized || d.Model().LineCount() != tc.lines {
				t.Fatal("normalized model", d.LineEnding(), d.Model().Text(), d.Model().LineCount())
			}
			var out bytes.Buffer
			n, err := d.WriteTo(&out)
			if err != nil || n != int64(len(tc.output)) || out.String() != tc.output {
				t.Fatal("output", n, err, out.String())
			}
			if d.Model().Revision() != 0 || d.Model().CanUndo() {
				t.Fatal("serialization changed model")
			}
		})
	}
}

// 输出格式是文档共享配置，不创建文本撤销项；无原始文本的模型不能反推 CRLF。
func TestDocumentLineEndingConfigurationAndReentry(t *testing.T) {
	m := textedit.NewModel("a\r\nb")
	d := NewDocumentWithModel(m)
	if d.LineEnding() != LineEndingLF {
		t.Fatal("normalized model guessed its old line ending")
	}
	first, second := NewSourceView(), NewSourceView()
	first.SetDocument(d)
	second.SetDocument(d)
	first.SetSelection(textedit.Selection{Anchor: 1, Caret: 1})
	before, revision := first.Selection(), m.Revision()
	var received []LineEnding
	h := d.ConnectLineEndingChanged(func(e LineEnding) { received = append(received, e) })
	if err := d.SetLineEnding(LineEndingCRLF); err != nil {
		t.Fatal(err)
	}
	if err := d.SetLineEnding(LineEndingCRLF); err != nil {
		t.Fatal(err)
	}
	if d.SetLineEnding(99) == nil || !slices.Equal(received, []LineEnding{LineEndingCRLF}) || d.LineEnding() != LineEndingCRLF {
		t.Fatal("invalid/same option changed configuration", received)
	}
	if first.Document().LineEnding() != second.Document().LineEnding() || first.Selection() != before || m.Revision() != revision || m.CanUndo() || m.Text() != "a\nb" {
		t.Fatal("format configuration edited a view or model")
	}
	h.Disconnect()
	once := true
	var observed []LineEnding
	h = d.ConnectLineEndingChanged(func(e LineEnding) {
		if once {
			once = false
			_ = d.SetLineEnding(LineEndingCRLF)
			_ = d.SetLineEnding(LineEndingLF)
		}
	})
	defer h.Disconnect()
	other := d.ConnectLineEndingChanged(func(e LineEnding) { observed = append(observed, e) })
	defer other.Disconnect()
	if err := d.SetLineEnding(LineEndingLF); err != nil {
		t.Fatal(err)
	}
	if d.LineEnding() != LineEndingLF || !slices.Equal(observed, []LineEnding{LineEndingCRLF, LineEndingLF}) {
		t.Fatal("reentrant setter published an obsolete event", observed)
	}
	if NewDocumentWithModel(nil).Model().Text() != "" {
		t.Fatal("nil model")
	}
}

type documentWriterFunc func([]byte) (int, error)

func (fn documentWriterFunc) Write(p []byte) (int, error) { return fn(p) }

// 输出计数、短写和底层错误按 io.Writer 契约返回；回调改文本或格式后立即停止。
func TestDocumentWriteFailureAndMutation(t *testing.T) {
	d := NewDocument(strings.Repeat("x\r\n", documentWriteChunk))
	sentinel := errors.New("writer failed")
	for _, tc := range []struct {
		name      string
		count     int
		err, want error
	}{
		{"short", 3, nil, io.ErrShortWrite}, {"zero", 0, nil, io.ErrShortWrite},
		{"partial error", 2, sentinel, sentinel},
	} {
		t.Run(tc.name, func(t *testing.T) {
			n, err := d.WriteTo(documentWriterFunc(func(p []byte) (int, error) { return tc.count, tc.err }))
			if n != int64(tc.count) || !errors.Is(err, tc.want) {
				t.Fatal(n, err)
			}
		})
	}
	for _, format := range []bool{false, true} {
		t.Run(map[bool]string{false: "model mutation", true: "format ABA mutation"}[format], func(t *testing.T) {
			d := NewDocument(strings.Repeat("x\r\n", documentWriteChunk))
			calls := 0
			n, err := d.WriteTo(documentWriterFunc(func(p []byte) (int, error) {
				calls++
				if format {
					_ = d.SetLineEnding(LineEndingLF)
					_ = d.SetLineEnding(LineEndingCRLF)
				} else {
					d.Model().SetText("replacement")
				}
				return len(p), nil
			}))
			if !errors.Is(err, ErrOperationChanged) || n <= 0 || calls != 1 {
				t.Fatal(n, err, calls)
			}
		})
	}
	var calls int
	n, err := NewDocument("").WriteTo(documentWriterFunc(func(p []byte) (int, error) { calls++; return 0, sentinel }))
	if n != 0 || err != nil || calls != 0 {
		t.Fatal("empty document wrote data", n, err, calls)
	}
}

// 多字节字符恰跨读取块边界，CRLF 也必须完整写出；数据落在测试临时目录中。
func TestDocumentWriteChunksAndEditedRoundTrip(t *testing.T) {
	text := strings.Repeat("x", documentWriteChunk-1) + "中\r\n" + strings.Repeat("👩‍💻\r\n", 12000) + "last"
	d := NewDocument(text)
	var output bytes.Buffer
	calls := 0
	n, err := d.WriteTo(documentWriterFunc(func(p []byte) (int, error) {
		calls++
		if len(p) > 2*documentWriteChunk || !utf8.Valid(p) || p[len(p)-1] == '\r' {
			t.Fatal("invalid output chunk", len(p))
		}
		return output.Write(p)
	}))
	if err != nil || n != int64(len(text)) || output.String() != text || calls < 2 {
		t.Fatal("chunked round trip", n, err, calls)
	}
	m := d.Model()
	if err := m.ReplaceAtomic(textedit.Range{Start: m.Len(), End: m.Len()}, "\r\n"); err != nil {
		t.Fatal(err)
	}
	if !m.Undo() || !m.Redo() {
		t.Fatal("newline undo/redo")
	}
	file, err := os.Create(filepath.Join(t.TempDir(), "source.txt"))
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := d.WriteTo(file)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		t.Fatal(writeErr, closeErr)
	}
	data, err := os.ReadFile(file.Name())
	if err != nil || string(data) != text+"\r\n" {
		t.Fatal("edited file output", err)
	}
	if d.LineEnding() != LineEndingCRLF || strings.ContainsRune(m.Text(), '\r') {
		t.Fatal("serialization leaked into model")
	}
}

// 选中模型里的一个 LF 就能删除整个 CRLF；撤销恢复格式和末尾换行。
func TestCRLFLineBreakDeletionAndUndo(t *testing.T) {
	d := NewDocument("a\r\nb\r\n")
	v := NewSourceView()
	v.SetDocument(d)
	observe(t, v)
	v.SetSelection(textedit.Selection{Anchor: 1, Caret: 2})
	if err := v.ReplaceSelection(""); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	_, err := d.WriteTo(&out)
	if err != nil || out.String() != "ab\r\n" || v.Selection() != (textedit.Selection{Anchor: 1, Caret: 1}) {
		t.Fatal("CRLF deletion", out.String(), err, v.Selection())
	}
	if !v.Undo() {
		t.Fatal("missing deletion undo")
	}
	out.Reset()
	_, err = d.WriteTo(&out)
	if err != nil || out.String() != "a\r\nb\r\n" || v.Selection() != (textedit.Selection{Anchor: 1, Caret: 2}) {
		t.Fatal("CRLF deletion undo", out.String(), err, v.Selection())
	}
}

func TestIndependentDocumentsAndViews(t *testing.T) {
	d := NewDocument("abc\nxyz")
	a, b := NewSourceView(), NewSourceView()
	a.SetDocument(d)
	b.SetDocument(d)
	a.SetSelection(textedit.Selection{Anchor: 1, Caret: 2})
	if d.connection != nil || d.engine != nil {
		t.Fatal("constructor subscribed to external model")
	}
	observe(t, a)
	observe(t, b)
	if len(d.views) != 2 {
		t.Fatal("shared subscriptions")
	}
	b.SetSelection(textedit.Selection{Anchor: 5, Caret: 5})
	if err := a.ReplaceSelection("ABC"); err != nil {
		t.Fatal(err)
	}
	if got := d.Model().Text(); got != "aABCc\nxyz" {
		t.Fatal(got)
	}
	if a.Selection() != (textedit.Selection{Anchor: 4, Caret: 4}) || b.Selection().Caret != 7 {
		t.Fatalf("selections %v / %v", a.Selection(), b.Selection())
	}
	if !a.Undo() || d.Model().Text() != "abc\nxyz" || a.Selection() != (textedit.Selection{Anchor: 1, Caret: 2}) {
		t.Fatal("undo selection", a.Selection())
	}
	b.SetReadOnly(true)
	if !errors.Is(b.ReplaceSelection("bad"), ErrReadOnly) || b.Undo() {
		t.Fatal("read only edited")
	}
	a.mounted = false
	d.release(a)
	if d.engine == nil {
		t.Fatal("one mounted view still needs analysis")
	}
	b.mounted = false
	d.release(b)
	if d.engine != nil || d.connection != nil {
		t.Fatal("last unmount retained observer")
	}
}

func TestHistoryIndentSearchAndPrivateDocument(t *testing.T) {
	v := NewSourceView()
	original := v.Document()
	m := original.Model()
	m.SetText("  alpha\n  alpha\nlast")
	v.SetSelection(textedit.Selection{Caret: 15})
	observe(t, v)
	if err := v.Indent(false); err != nil {
		t.Fatal(err)
	}
	if m.Text() != "      alpha\n      alpha\nlast" {
		t.Fatal(m.Text())
	}
	if !v.Undo() || m.Text() != "  alpha\n  alpha\nlast" {
		t.Fatal("atomic indent undo")
	}
	v.SetSearch(SearchQuery{Text: "alpha"})
	n, err := v.ReplaceAll("β")
	if err != nil || n != 2 || m.Text() != "  β\n  β\nlast" {
		t.Fatal(n, err, m.Text())
	}
	if !v.Undo() || m.Text() != "  alpha\n  alpha\nlast" {
		t.Fatal("atomic replace undo")
	}
	r, ok := v.FindNext(false)
	if !ok || r != (textedit.Range{Start: 2, End: 7}) {
		t.Fatal(r, ok)
	}
	if ok, err := v.ReplaceCurrent("x"); !ok || err != nil || m.Text() != "  x\n  alpha\nlast" {
		t.Fatal(ok, err, m.Text())
	}
	other := NewDocument("shared")
	v.SetDocument(other)
	v.SetDocument(nil)
	if v.Document() != original || m.Text() != "  x\n  alpha\nlast" {
		t.Fatal("private document lost")
	}
}

func TestNotificationReentryDoesNotOverwriteNewSelection(t *testing.T) {
	d := NewDocument("abc")
	v := NewSourceView()
	v.SetDocument(d)
	// This listener runs before the mounted view and sees a newer revision
	// while the older notification is still being delivered.
	fired := false
	handle := d.Model().ConnectChange(func(c textedit.Change) {
		if fired {
			return
		}
		fired = true
		if err := d.Model().Replace(textedit.Range{Start: 0, End: 0}, "Ω"); err != nil {
			t.Fatal(err)
		}
		v.SetSelection(textedit.Selection{Anchor: 2, Caret: 2})
	})
	defer handle.Disconnect()
	observe(t, v)
	v.SetSelection(textedit.Selection{Anchor: 3, Caret: 3})
	if err := v.ReplaceSelection("!"); err != nil {
		t.Fatal(err)
	}
	if got := v.Selection(); got != (textedit.Selection{Anchor: 2, Caret: 2}) {
		t.Fatal("older edit overwrote reentrant selection", got)
	}
	if v.observed != d.Model().Revision() {
		t.Fatal("notifications not drained")
	}
}

func TestResetAndLanguageValidation(t *testing.T) {
	d := NewDocument("abc")
	v := NewSourceView()
	v.SetDocument(d)
	observe(t, v)
	v.SetSelection(textedit.Selection{Anchor: 3, Caret: 3})
	d.Model().SetText("new")
	if v.Selection() != (textedit.Selection{}) {
		t.Fatal("reset selection")
	}
	if err := d.SetLanguage(language.JSON); err != nil {
		t.Fatal(err)
	}
	engine := d.engine
	if err := d.SetLanguage(language.ID("missing")); !errors.Is(err, ErrUnknownLanguage) || d.Language() != language.JSON || d.engine != engine {
		t.Fatal("invalid language mutated document")
	}
	info := v.Snapshot()
	if info.Role != "sourceview" || info.TextEditing == nil || info.TextEditing.Length != 3 {
		t.Fatalf("%+v", info)
	}
}
