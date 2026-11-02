package main

import (
	"testing"

	"github.com/golang-gui/goui-sourceview/language"
)

// 每个下拉列表样本都应读取成功，并使用可用的内置语言；不创建 GUI。
func TestSampleDocuments(t *testing.T) {
	documents := sampleDocuments()
	seen := make(map[language.ID]bool)
	associations := map[string]language.ID{"View.xaml": language.XML, "Cargo.lock": language.TOML}
	for _, sample := range documents {
		if sample.document.Model().Len() == 0 || sample.document.Language() == language.Plain {
			t.Fatalf("unrecognized sample: %s", sample.name)
		}
		seen[sample.document.Language()] = true
		if want, ok := associations[sample.name]; ok {
			if sample.document.Language() != want {
				t.Errorf("%s: want %s", sample.name, want)
			}
			delete(associations, sample.name)
		}
	}
	if len(associations) > 0 {
		t.Fatalf("missing associated file samples: %v", associations)
	}
	for _, id := range []language.ID{language.CSV, language.TSV, language.AsciiDoc, language.ReStructuredText,
		language.LaTeX, language.BibTeX, language.Gettext, language.EditorConfig, language.JSON5, language.EDN,
		language.GitConfig, language.SSHConfig, language.Crontab, language.Mermaid, language.DOT} {
		if !seen[id] {
			t.Errorf("missing sample for %s", id)
		}
	}
}
