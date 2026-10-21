package highlight

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/gui/textedit"
)

// Optional pure-analysis benchmark. Never downloads files or creates a GUI.
// Reading and model construction are outside timing; metadata building and
// complete initial tokenization are inside. Cached native rendering is excluded.
func BenchmarkCorpusAnalysis(b *testing.B) {
	dir := os.Getenv("GOUI_CORPUS_DIR")
	if dir == "" {
		b.Skip("optional corpus benchmark: set GOUI_CORPUS_DIR to the downloaded directory")
	}
	for _, name := range []string{"war-and-peace.txt", "github-api.json", "opengl.xml", "war-and-peace.html", "rust-releases.md"} {
		b.Run(name, func(b *testing.B) {
			data, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				b.Fatal(err)
			}
			model := textedit.NewModel(string(data))
			id := language.Guess(name, data[:min(len(data), 4096)])
			b.ReportAllocs()
			b.SetBytes(int64(model.Len()))
			b.ResetTimer()
			for range b.N {
				engine := New(model, id)
				engine.Advance(time.Time{})
				if engine.Err() != nil || engine.Pending() || !engine.Ready() || engine.LineCount() != model.LineCount() {
					b.Fatal("analysis incomplete", engine.Err(), engine.LineCount())
				}
			}
		})
	}
}
