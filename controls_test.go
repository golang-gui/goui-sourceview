package sourceview

import (
	"errors"
	"image/color"
	"testing"

	"github.com/golang-gui/goui-sourceview/scheme"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/mlang"
	"github.com/golang-gui/goui/platform/graphics"
	"github.com/golang-gui/goui/style"
	"github.com/golang-gui/goui/theme/basic"
	"github.com/golang-gui/goui/theme/modern"
)

func TestDetachedFindBarBorrowsTargetWithoutSubscriptions(t *testing.T) {
	a, b := NewSourceView(), NewSourceView()
	bar := NewFindBar()
	initial := len(a.EventControllers())
	bar.SetTarget(a)
	bar.SetReplacement("replacement")
	bar.SetReplaceMode(true)
	if bar.Opened() || bar.Target() != a || bar.Replacement() != "replacement" || !bar.ReplaceMode() {
		t.Fatal("detached properties")
	}
	if err := bar.Open(false); !errors.Is(err, ErrNotMounted) {
		t.Fatal(err)
	}
	if len(a.EventControllers()) != initial || a.document.connection != nil || len(bar.attached) != 0 || len(bar.lifetime) != 0 {
		t.Fatal("detached companion subscribed")
	}
	bar.SetTarget(b)
	bar.SetTarget(nil)
	if bar.Target() != nil || len(a.EventControllers()) != initial {
		t.Fatal("retarget retained editor")
	}
}

func TestGoToOffsetUsesOneBasedRuneColumns(t *testing.T) {
	m := textedit.NewModel("")
	m.SetText("abc\n中e\u0301x\n")
	for _, tc := range []struct {
		query string
		at    int
		valid bool
	}{{"1", 0, true}, {"2:2", 7, true}, {"2:4", 10, true}, {"3:1", 12, true}, {"0", 0, false}, {"2:6", 0, false}, {"1:0", 0, false}, {"1:2:3", 0, false}, {"no", 0, false}} {
		at, err := gotoOffset(m, tc.query)
		if (err == nil) != tc.valid || tc.valid && at != tc.at {
			t.Fatal(tc, at, err)
		}
	}
}

func TestGoToRetargetHonorsCloseCallback(t *testing.T) {
	bar := NewGoToBar()
	a, b, c := NewSourceView(), NewSourceView(), NewSourceView()
	bar.SetTarget(a)
	bar.open = true
	bar.ConnectOpen(func(open bool) {
		if !open {
			bar.SetTarget(c)
		}
	})
	bar.SetTarget(b)
	if bar.Target() != c {
		t.Fatal("outer setter overwrote callback target")
	}
}

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

func TestCompletionShellRulesAndSafeContent(t *testing.T) {
	for _, tc := range []struct {
		name   string
		sheet  style.StyleSheet
		radius float32
		border float32
	}{
		{"basic", basic.Sheet(), 4, 1},
		{"light", modern.Sheet(modern.Options{}), 8, 1},
		{"dark", modern.Sheet(modern.Options{Dark: true}), 8, 1},
		{"custom", style.Sheet(append(modern.Rules(modern.Options{}), style.Name("sourceview.completion").Radius(32).BorderWidth(2))...), 32, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l, _ := completionFixture(t, []Completion{{Label: "true", Detail: "JSON"}})
			l.view.completion = nil
			themedScope(l.view, tc.sheet)
			l.readAppearance()
			l.view.completion = l.session
			if l.box.radius != tc.radius || l.box.width != tc.border || l.colors.Background.A != 255 {
				t.Fatal("incomplete styled shell", l.box)
			}
			menu := boxOf(tc.sheet.Resolve(style.Sel{Name: "menu"}))
			if l.box.shadow != menu.shadow {
				t.Fatal("completion shadow diverged from the official menu")
			}
			arrangeCompletion(l)
			p := l.session.panel
			r := p.scroll.Rect()
			// The content's leading corner must be inside the rounded border,
			// independently of the implementation's padding formula.
			dx, dy, radius := max(0, tc.radius-r.X), max(0, tc.radius-r.Y), tc.radius-l.box.width
			if dx*dx+dy*dy > radius*radius {
				t.Fatal("scroll content covered a rounded shell corner", r)
			}
		})
	}
}

// The former scheme UI palette tests live here, where those colors are owned.
func TestControlPalettesAreIndependentOfCodeSchemes(t *testing.T) {
	for _, dark := range []bool{false, true} {
		v := NewSourceView()
		sheet := modern.Sheet(modern.Options{Dark: dark, FontFamily: "ui-font", FontSize: 19, AccentColor: rgb(0x357f40)})
		themedScope(v, sheet)
		l := newCompletionList(v, &completionSession{})
		b := NewFindBar()
		// A companion uses its own explicit tree location; SetTarget does not
		// transfer a stylesheet from an unrelated subtree.
		themedScope(b, sheet)
		b.StyleChanged()
		before, find := l.colors, b.appearance
		for _, c := range []color.NRGBA{before.Background, before.Foreground, before.DetailForeground, before.SelectedForeground, before.SelectedDetailForeground} {
			if c.A != 255 {
				t.Fatal("unreadable built-in popup surface/text", c)
			}
		}
		if l.font.Family != "ui-font" || l.font.Size != 19 || l.detailFont().Size != 19*.9 || b.appearance.surface.radius != 8 || b.appearance.focused.width != 2 {
			t.Fatal("Modern font, radius or focus metrics were not used")
		}
		if got := foreground(gui.ResolveWidgetStyle(b.find, "sourceview.find.input", "", style.Normal)); got.A != 255 || background(gui.ResolveWidgetStyle(b.find, "sourceview.find.input", "frame", style.Normal)).A != 255 {
			t.Fatal("unreadable built-in find surface/text")
		}
		d := scheme.Dark().Definition()
		d.SelectionBackground, d.SelectionForeground = rgb(0xff0000), rgb(0x00ff00)
		s, err := scheme.New(d)
		if err != nil {
			t.Fatal(err)
		}
		v.SetScheme(s)
		l.readAppearance()
		b.SetTarget(v)
		b.StyleChanged()
		if l.colors != before || b.appearance != find {
			t.Fatal("code selection changed UI palettes")
		}
		if got := foreground(sheet.Resolve(style.Sel{Name: "icon"})); got != foreground(modern.Sheet(modern.Options{Dark: dark}).Resolve(style.Sel{Name: "icon"})) {
			t.Fatal("SourceView registration modified the global icon style")
		}
	}
}

func TestApplicationOverridesAndPopupOwnerScope(t *testing.T) {
	rules := modern.Rules(modern.Options{Dark: true})
	rules = append(rules,
		style.Name("sourceview.completion").BorderColor(color.NRGBA{}).FontFamily("owner-font").FontSize(16),
		style.Name("sourceview.completion").Shadow(style.Shadow{Color: color.NRGBA{R: 200, A: 80}}),
		style.Name("sourceview.find").Shadow(style.Shadow{}),
		style.Name("sourceview.find.option").Part("button-checked").BorderColor(color.NRGBA{R: 90, A: 80}),
	)
	v := NewSourceView()
	owner := themedScope(v, style.Sheet(rules...))
	l := newCompletionList(v, &completionSession{})
	// Popup roots may have a completely different application sheet. Resolution
	// is explicitly tied to the owner, before and after attachment to that root.
	themedScope(l, basic.Sheet())
	l.StyleChanged()
	if l.font.Family != "owner-font" || l.font.Size != 16 || l.colors.Border.A != 0 || l.box.shadow.Color.A != graphics.ColorOf(color.NRGBA{R: 200, A: 80}).A {
		t.Fatal("popup lost its owner scope or decorative alpha")
	}
	b := NewFindBar()
	themedScope(b, owner.StyleSheet())
	b.StyleChanged()
	border, _ := gui.ResolveWidgetStyle(b.matchCase, "sourceview.find.option", "button-checked", style.Normal).BorderColor()
	if b.appearance.surface.shadow.Color.A != 0 || nrgba(border).A != 80 || b.inset != 2 {
		t.Fatal("find overrides lost explicit alpha or retained shadow space")
	}
}

func TestCompletionThemeChangesReleaseOwnedLayouts(t *testing.T) {
	l, ctx := completionFixture(t, []Completion{{Label: "transform", Detail: "JSON"}})
	owner := themedScope(l.view, modern.Sheet(modern.Options{}))
	l.StyleChanged()
	arrangeCompletion(l)
	l.Paint(&completionPainter{})
	created, destroyed, epoch := ctx.created, ctx.destroyed, l.epoch
	l.StyleChanged()
	if ctx.created != created || ctx.destroyed != destroyed || l.epoch != epoch {
		t.Fatal("unchanged appearance released native resources")
	}
	owner.SetStyleSheet(modern.Sheet(modern.Options{Dark: true, FontSize: 18}))
	l.StyleChanged()
	if len(l.rows) != 0 || l.measured || ctx.destroyed != created {
		t.Fatal("theme change retained native layouts or old metrics")
	}
	arrangeCompletion(l)
	l.Paint(&completionPainter{})
	if l.rows[0].label.native.(*shapedLayout).format.Font.Size != 18 || l.colors.Background != rgb(0x28282d) {
		t.Fatal("popup rebuild used stale font or palette")
	}
}

func TestUnchangedStyleNotificationsKeepCompletion(t *testing.T) {
	l, ctx := completionFixture(t, []Completion{{Label: "transform", Detail: "JSON"}})
	owner := themedScope(l.view, modern.Sheet(modern.Options{}))
	l.readAppearance()
	arrangeCompletion(l)
	l.Paint(&completionPainter{})
	created, destroyed := ctx.created, ctx.destroyed
	closed := 0
	l.session.popup = closingPopup{onClose: func() { closed++ }}
	// Hover/focus-state invalidation can call this without changing any of the
	// code or popup appearance values. The current session must remain usable.
	l.view.StyleChanged()
	if l.view.completion != l.session || closed != 0 || ctx.created != created || ctx.destroyed != destroyed {
		t.Fatal("unchanged style notification closed or rebuilt completion")
	}
	// The fixture has an explicit code scheme. UI-only changes still close and
	// release the old native popup so reopening uses the new UI font/colors.
	owner.SetStyleSheet(modern.Sheet(modern.Options{Dark: true, FontSize: 18}))
	l.view.StyleChanged()
	if l.view.completion != nil || closed != 1 || ctx.destroyed != created {
		t.Fatal("UI-only theme change retained the old completion popup")
	}
}

func TestCompletionSelectionRadiusChangeClosesOldSession(t *testing.T) {
	l, ctx := completionFixture(t, []Completion{{Label: "true", Detail: "JSON"}})
	owner := themedScope(l.view, modern.Sheet(modern.Options{}))
	l.readAppearance()
	arrangeCompletion(l)
	l.Paint(&completionPainter{})
	created := ctx.created
	closed := 0
	l.session.popup = closingPopup{onClose: func() { closed++ }}
	owner.SetStyleSheet(style.Sheet(append(modern.Rules(modern.Options{}), style.Name("sourceview.completion").Part("selected").Radius(0))...))
	l.view.StyleChanged()
	if l.view.completion != nil || closed != 1 || ctx.destroyed != created {
		t.Fatal("selection-radius-only change retained a stale popup or native resources")
	}
}
