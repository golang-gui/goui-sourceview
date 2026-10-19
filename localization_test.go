package sourceview

import (
	"testing"

	"github.com/golang-gui/goui/mlang"
)

func preserveUILanguage(t *testing.T) {
	t.Helper()
	previous := mlang.Language()
	t.Cleanup(func() {
		if err := mlang.SetLanguage(previous); err != nil {
			t.Fatal(err)
		}
	})
}

func TestEditorCommandTranslations(t *testing.T) {
	preserveUILanguage(t)
	for _, tc := range []struct{ tag, copy, move string }{
		{"en-US", "Copy", "Move Lines Up"}, {"zh-CN", "复制", "上移行"},
		{"ja-JP", "コピー", "行を上へ移動"}, {"ko-KR", "복사", "줄 위로 이동"},
		{"de-DE", "Kopieren", "Zeilen nach oben verschieben"}, {"fr-FR", "Copier", "Déplacer les lignes vers le haut"},
		{"ru-RU", "Копировать", "Переместить строки вверх"}, {"ar-SA", "نسخ", "نقل الأسطر لأعلى"},
	} {
		if err := mlang.SetLanguage(tc.tag); err != nil {
			t.Fatal(err)
		}
		if commandLabels[CommandCopy]() != tc.copy || commandLabels[CommandMoveLinesUp]() != tc.move {
			t.Errorf("%s: copy=%q move=%q", tc.tag, commandLabels[CommandCopy](), commandLabels[CommandMoveLinesUp]())
		}
	}
}

func TestFindBarTranslatedInputsAndHints(t *testing.T) {
	preserveUILanguage(t)
	for _, tc := range []struct{ tag, find, replacement, replace string }{
		{"en-US", "Find", "Replacement", "Replace"}, {"zh-CN", "查找", "替换内容", "替换"},
		{"ja-JP", "検索", "置換後の文字列", "置換"}, {"ko-KR", "찾기", "바꿀 내용", "바꾸기"},
		{"de-DE", "Suchen", "Ersetzung", "Ersetzen"}, {"fr-FR", "Rechercher", "Texte de remplacement", "Remplacer"},
		{"ru-RU", "Найти", "Текст замены", "Заменить"}, {"ar-SA", "بحث", "نص الاستبدال", "استبدال"},
	} {
		if err := mlang.SetLanguage(tc.tag); err != nil {
			t.Fatal(err)
		}
		bar := NewFindBar()
		if bar.find.Name() != tc.find || bar.replacement.Name() != tc.replacement || bar.find.Placeholder() != tc.find || bar.replacement.Placeholder() != tc.replace {
			t.Errorf("%s: names=%q/%q hints=%q/%q", tc.tag, bar.find.Name(), bar.replacement.Name(), bar.find.Placeholder(), bar.replacement.Placeholder())
		}
		setFindText(bar.find, "query")
		bar.SetReplacement("replacement")
		bar.updateLabels()
		if bar.find.Model().Text() != "query" || bar.Replacement() != "replacement" {
			t.Fatal("translating hints changed text values")
		}
	}
}

func TestFindBarTranslatedStatus(t *testing.T) {
	preserveUILanguage(t)
	bar := NewFindBar()
	bar.count.query = SearchQuery{Text: "x"}
	for _, tc := range []struct{ tag, counting, tooLong, count string }{
		{"en", "Counting…", "Query too long to count", "Matches: 2"},
		{"zh-CN", "计数中…", "查询过长，无法计数", "匹配数：2"},
		{"ja", "集計中…", "検索文字列が長すぎるため集計できません", "一致数: 2"},
		{"ko", "계산 중…", "검색어가 너무 길어 개수를 셀 수 없습니다", "일치 항목: 2"},
		{"de", "Wird gezählt…", "Suchtext zum Zählen zu lang", "Treffer: 2"},
		{"fr", "Comptage en cours…", "Recherche trop longue pour le comptage", "Résultats : 2"},
		{"ru", "Подсчёт…", "Запрос слишком длинный для подсчёта", "Совпадений: 2"},
		{"ar", "جارٍ العد…", "نص البحث طويل جدًا للعد", "عدد النتائج: 2"},
	} {
		if err := mlang.SetLanguage(tc.tag); err != nil {
			t.Fatal(err)
		}
		bar.count.available, bar.count.done = true, false
		bar.updateStatus()
		if bar.status.Text() != tc.counting {
			t.Errorf("%s counting=%q", tc.tag, bar.status.Text())
		}
		bar.count.available = false
		bar.updateStatus()
		if bar.status.Text() != tc.tooLong {
			t.Errorf("%s too long=%q", tc.tag, bar.status.Text())
		}
		bar.count.available, bar.count.done, bar.count.total = true, true, 2
		bar.updateStatus()
		if bar.status.Text() != tc.count {
			t.Errorf("%s count=%q", tc.tag, bar.status.Text())
		}
		bar.message = "application diagnostic"
		bar.updateStatus()
		if bar.status.Text() != bar.message {
			t.Fatal("diagnostic error was translated")
		}
		bar.message = ""
	}
}
