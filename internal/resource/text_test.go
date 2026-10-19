package resource

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/golang-gui/goui/mlang"
)

func TestTranslationsAndFallback(t *testing.T) {
	previous := mlang.Language()
	t.Cleanup(func() { _ = mlang.SetLanguage(previous) })
	getters := []mlang.Getter{Undo, Redo, Cut, Copy, Paste, Delete, MoveLinesUp, MoveLinesDown,
		DuplicateLinesUp, DuplicateLinesDown, DeleteLines, SelectAll, Find, Replacement, Replace,
		ReplaceAll, ToggleReplace, MatchCase, WholeWord, Previous, Next, Close, Counting, QueryTooLong, matches, Fold, Unfold, FoldAll, UnfoldAll}
	if err := mlang.SetLanguage("en"); err != nil {
		t.Fatal(err)
	}
	english := make([]string, len(getters))
	for i, get := range getters {
		english[i] = get()
	}
	for _, tag := range []string{"en-US", "zh-CN", "ja-JP", "ko-KR", "de-DE", "fr-FR", "ru-RU", "ar-SA"} {
		if err := mlang.SetLanguage(tag); err != nil {
			t.Fatal(err)
		}
		for i, get := range getters {
			text := get()
			if text == "" || !utf8.ValidString(text) || tag != "en-US" && text == english[i] {
				t.Errorf("%s: missing/invalid translation for %q: %q", tag, english[i], text)
			}
		}
	}
	for _, tag := range []string{"es", "und"} {
		if err := mlang.SetLanguage(tag); err != nil {
			t.Fatal(err)
		}
		for i, get := range getters {
			if got := get(); got != english[i] {
				t.Errorf("%s: fallback=%q want=%q", tag, got, english[i])
			}
		}
	}
}

func TestMatchCounts(t *testing.T) {
	previous := mlang.Language()
	t.Cleanup(func() { _ = mlang.SetLanguage(previous) })
	for _, tc := range []struct{ tag, format string }{
		{"en", "Matches: %d"}, {"zh-CN", "匹配数：%d"}, {"ja", "一致数: %d"}, {"ko", "일치 항목: %d"},
		{"de", "Treffer: %d"}, {"fr", "Résultats : %d"}, {"ru", "Совпадений: %d"}, {"ar", "عدد النتائج: %d"},
	} {
		if err := mlang.SetLanguage(tc.tag); err != nil {
			t.Fatal(err)
		}
		for _, count := range []int{0, 1, 2, 5, 21, 1000000} {
			if got, want := Matches(count), fmt.Sprintf(tc.format, count); got != want || strings.Contains(got, "%!") {
				t.Errorf("%s count=%d got=%q want=%q", tc.tag, count, got, want)
			}
		}
	}
}
