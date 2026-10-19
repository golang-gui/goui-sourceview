// Package resource contains SourceView-owned presentation text. Source content,
// stable identifiers, provider proposals and diagnostic errors are not translated.
package resource

import (
	"fmt"

	"github.com/golang-gui/goui/mlang"
)

var (
	Undo = mlang.String(mlang.En("Undo"),
		mlang.ZhCN("撤销"), mlang.Ja("元に戻す"), mlang.Ko("실행 취소"),
		mlang.De("Rückgängig"), mlang.Fr("Annuler"), mlang.Ru("Отменить"), mlang.Ar("تراجع"))
	Redo = mlang.String(mlang.En("Redo"),
		mlang.ZhCN("重做"), mlang.Ja("やり直す"), mlang.Ko("다시 실행"),
		mlang.De("Wiederholen"), mlang.Fr("Rétablir"), mlang.Ru("Повторить"), mlang.Ar("إعادة"))
	Cut = mlang.String(mlang.En("Cut"),
		mlang.ZhCN("剪切"), mlang.Ja("切り取り"), mlang.Ko("잘라내기"),
		mlang.De("Ausschneiden"), mlang.Fr("Couper"), mlang.Ru("Вырезать"), mlang.Ar("قص"))
	Copy = mlang.String(mlang.En("Copy"),
		mlang.ZhCN("复制"), mlang.Ja("コピー"), mlang.Ko("복사"),
		mlang.De("Kopieren"), mlang.Fr("Copier"), mlang.Ru("Копировать"), mlang.Ar("نسخ"))
	Paste = mlang.String(mlang.En("Paste"),
		mlang.ZhCN("粘贴"), mlang.Ja("貼り付け"), mlang.Ko("붙여넣기"),
		mlang.De("Einfügen"), mlang.Fr("Coller"), mlang.Ru("Вставить"), mlang.Ar("لصق"))
	Delete = mlang.String(mlang.En("Delete"),
		mlang.ZhCN("删除"), mlang.Ja("削除"), mlang.Ko("삭제"),
		mlang.De("Löschen"), mlang.Fr("Supprimer"), mlang.Ru("Удалить"), mlang.Ar("حذف"))
	MoveLinesUp = mlang.String(mlang.En("Move Lines Up"),
		mlang.ZhCN("上移行"), mlang.Ja("行を上へ移動"), mlang.Ko("줄 위로 이동"),
		mlang.De("Zeilen nach oben verschieben"), mlang.Fr("Déplacer les lignes vers le haut"),
		mlang.Ru("Переместить строки вверх"), mlang.Ar("نقل الأسطر لأعلى"))
	MoveLinesDown = mlang.String(mlang.En("Move Lines Down"),
		mlang.ZhCN("下移行"), mlang.Ja("行を下へ移動"), mlang.Ko("줄 아래로 이동"),
		mlang.De("Zeilen nach unten verschieben"), mlang.Fr("Déplacer les lignes vers le bas"),
		mlang.Ru("Переместить строки вниз"), mlang.Ar("نقل الأسطر لأسفل"))
	DuplicateLinesUp = mlang.String(mlang.En("Duplicate Lines Up"),
		mlang.ZhCN("向上复制行"), mlang.Ja("行を上へ複製"), mlang.Ko("줄 위로 복제"),
		mlang.De("Zeilen nach oben duplizieren"), mlang.Fr("Dupliquer les lignes vers le haut"),
		mlang.Ru("Дублировать строки выше"), mlang.Ar("تكرار الأسطر لأعلى"))
	DuplicateLinesDown = mlang.String(mlang.En("Duplicate Lines Down"),
		mlang.ZhCN("向下复制行"), mlang.Ja("行を下へ複製"), mlang.Ko("줄 아래로 복제"),
		mlang.De("Zeilen nach unten duplizieren"), mlang.Fr("Dupliquer les lignes vers le bas"),
		mlang.Ru("Дублировать строки ниже"), mlang.Ar("تكرار الأسطر لأسفل"))
	DeleteLines = mlang.String(mlang.En("Delete Lines"),
		mlang.ZhCN("删除整行"), mlang.Ja("行を削除"), mlang.Ko("줄 삭제"),
		mlang.De("Zeilen löschen"), mlang.Fr("Supprimer les lignes"), mlang.Ru("Удалить строки"), mlang.Ar("حذف الأسطر"))
	SelectAll = mlang.String(mlang.En("Select All"),
		mlang.ZhCN("全选"), mlang.Ja("すべて選択"), mlang.Ko("모두 선택"),
		mlang.De("Alles auswählen"), mlang.Fr("Tout sélectionner"), mlang.Ru("Выделить всё"), mlang.Ar("تحديد الكل"))
	Find = mlang.String(mlang.En("Find"),
		mlang.ZhCN("查找"), mlang.Ja("検索"), mlang.Ko("찾기"),
		mlang.De("Suchen"), mlang.Fr("Rechercher"), mlang.Ru("Найти"), mlang.Ar("بحث"))
	Replacement = mlang.String(mlang.En("Replacement"),
		mlang.ZhCN("替换内容"), mlang.Ja("置換後の文字列"), mlang.Ko("바꿀 내용"),
		mlang.De("Ersetzung"), mlang.Fr("Texte de remplacement"), mlang.Ru("Текст замены"), mlang.Ar("نص الاستبدال"))
	Replace = mlang.String(mlang.En("Replace"),
		mlang.ZhCN("替换"), mlang.Ja("置換"), mlang.Ko("바꾸기"),
		mlang.De("Ersetzen"), mlang.Fr("Remplacer"), mlang.Ru("Заменить"), mlang.Ar("استبدال"))
	ReplaceAll = mlang.String(mlang.En("Replace all"),
		mlang.ZhCN("全部替换"), mlang.Ja("すべて置換"), mlang.Ko("모두 바꾸기"),
		mlang.De("Alle ersetzen"), mlang.Fr("Tout remplacer"), mlang.Ru("Заменить всё"), mlang.Ar("استبدال الكل"))
	ToggleReplace = mlang.String(mlang.En("Toggle replace"),
		mlang.ZhCN("展开或收起替换"), mlang.Ja("置換欄の表示を切り替え"), mlang.Ko("바꾸기 표시 전환"),
		mlang.De("Ersetzungsfeld ein-/ausblenden"), mlang.Fr("Afficher ou masquer le remplacement"),
		mlang.Ru("Показать или скрыть замену"), mlang.Ar("إظهار أو إخفاء الاستبدال"))
	MatchCase = mlang.String(mlang.En("Match case"),
		mlang.ZhCN("区分大小写"), mlang.Ja("大文字と小文字を区別"), mlang.Ko("대/소문자 구분"),
		mlang.De("Groß-/Kleinschreibung beachten"), mlang.Fr("Respecter la casse"),
		mlang.Ru("Учитывать регистр"), mlang.Ar("مطابقة حالة الأحرف"))
	WholeWord = mlang.String(mlang.En("Whole word"),
		mlang.ZhCN("整词"), mlang.Ja("単語単位"), mlang.Ko("단어 단위"),
		mlang.De("Ganzes Wort"), mlang.Fr("Mot entier"), mlang.Ru("Слово целиком"), mlang.Ar("كلمة كاملة"))
	Previous = mlang.String(mlang.En("Previous"),
		mlang.ZhCN("上一项"), mlang.Ja("前へ"), mlang.Ko("이전"),
		mlang.De("Vorheriger Treffer"), mlang.Fr("Précédent"), mlang.Ru("Предыдущее совпадение"), mlang.Ar("السابق"))
	Next = mlang.String(mlang.En("Next"),
		mlang.ZhCN("下一项"), mlang.Ja("次へ"), mlang.Ko("다음"),
		mlang.De("Nächster Treffer"), mlang.Fr("Suivant"), mlang.Ru("Следующее совпадение"), mlang.Ar("التالي"))
	Close = mlang.String(mlang.En("Close"),
		mlang.ZhCN("关闭"), mlang.Ja("閉じる"), mlang.Ko("닫기"),
		mlang.De("Schließen"), mlang.Fr("Fermer"), mlang.Ru("Закрыть"), mlang.Ar("إغلاق"))
	Counting = mlang.String(mlang.En("Counting…"),
		mlang.ZhCN("计数中…"), mlang.Ja("集計中…"), mlang.Ko("계산 중…"),
		mlang.De("Wird gezählt…"), mlang.Fr("Comptage en cours…"), mlang.Ru("Подсчёт…"), mlang.Ar("جارٍ العد…"))
	QueryTooLong = mlang.String(mlang.En("Query too long to count"),
		mlang.ZhCN("查询过长，无法计数"), mlang.Ja("検索文字列が長すぎるため集計できません"), mlang.Ko("검색어가 너무 길어 개수를 셀 수 없습니다"),
		mlang.De("Suchtext zum Zählen zu lang"), mlang.Fr("Recherche trop longue pour le comptage"),
		mlang.Ru("Запрос слишком длинный для подсчёта"), mlang.Ar("نص البحث طويل جدًا للعد"))
	// A noun label followed by the count avoids language-dependent plural forms.
	matches = mlang.String(mlang.En("Matches: %d"),
		mlang.ZhCN("匹配数：%d"), mlang.Ja("一致数: %d"), mlang.Ko("일치 항목: %d"),
		mlang.De("Treffer: %d"), mlang.Fr("Résultats : %d"), mlang.Ru("Совпадений: %d"), mlang.Ar("عدد النتائج: %d"))
)

func Matches(count int) string { return fmt.Sprintf(matches(), count) }

var (
	Fold      = mlang.String(mlang.En("Fold"), mlang.ZhCN("折叠"), mlang.Ja("折りたたむ"), mlang.Ko("접기"), mlang.De("Einklappen"), mlang.Fr("Replier"), mlang.Ru("Свернуть"), mlang.Ar("طي"))
	Unfold    = mlang.String(mlang.En("Unfold"), mlang.ZhCN("展开"), mlang.Ja("展開"), mlang.Ko("펼치기"), mlang.De("Ausklappen"), mlang.Fr("Déplier"), mlang.Ru("Развернуть"), mlang.Ar("توسيع"))
	FoldAll   = mlang.String(mlang.En("Fold all"), mlang.ZhCN("全部折叠"), mlang.Ja("すべて折りたたむ"), mlang.Ko("모두 접기"), mlang.De("Alles einklappen"), mlang.Fr("Tout replier"), mlang.Ru("Свернуть всё"), mlang.Ar("طي الكل"))
	UnfoldAll = mlang.String(mlang.En("Unfold all"), mlang.ZhCN("全部展开"), mlang.Ja("すべて展開"), mlang.Ko("모두 펼치기"), mlang.De("Alles ausklappen"), mlang.Fr("Tout déplier"), mlang.Ru("Развернуть всё"), mlang.Ar("توسيع الكل"))
)

var (
	GoTo            = mlang.String(mlang.En("Go to line/column"), mlang.ZhCN("跳转行列"), mlang.Ja("行・列へ移動"), mlang.Ko("줄/열로 이동"), mlang.De("Zu Zeile/Spalte"), mlang.Fr("Aller à la ligne/colonne"), mlang.Ru("Перейти к строке/столбцу"), mlang.Ar("انتقال إلى سطر/عمود"))
	GoToPlaceholder = mlang.String(mlang.En("Line[:column]"), mlang.ZhCN("行号[:列号]"), mlang.Ja("行[:列]"), mlang.Ko("줄[:열]"), mlang.De("Zeile[:Spalte]"), mlang.Fr("Ligne[:colonne]"), mlang.Ru("Строка[:столбец]"), mlang.Ar("سطر[:عمود]"))
	InvalidPosition = mlang.String(mlang.En("Invalid line or column"), mlang.ZhCN("行号或列号无效"), mlang.Ja("行または列が無効です"), mlang.Ko("잘못된 줄 또는 열"), mlang.De("Ungültige Zeile oder Spalte"), mlang.Fr("Ligne ou colonne invalide"), mlang.Ru("Недопустимая строка или столбец"), mlang.Ar("سطر أو عمود غير صالح"))
)

var (
	RegularExpression = mlang.String(mlang.En("Regular expression"), mlang.ZhCN("正则表达式"), mlang.Ja("正規表現"), mlang.Ko("정규식"), mlang.De("Regulärer Ausdruck"), mlang.Fr("Expression régulière"), mlang.Ru("Регулярное выражение"), mlang.Ar("تعبير نمطي"))
	InSelection       = mlang.String(mlang.En("Find in selection"), mlang.ZhCN("在选区中查找"), mlang.Ja("選択範囲内を検索"), mlang.Ko("선택 영역에서 찾기"), mlang.De("In Auswahl suchen"), mlang.Fr("Chercher dans la sélection"), mlang.Ru("Искать в выделении"), mlang.Ar("بحث في التحديد"))
	InvalidExpression = mlang.String(mlang.En("Invalid expression"), mlang.ZhCN("表达式无效"), mlang.Ja("無効な正規表現"), mlang.Ko("잘못된 정규식"), mlang.De("Ungültiger Ausdruck"), mlang.Fr("Expression invalide"), mlang.Ru("Недопустимое выражение"), mlang.Ar("تعبير غير صالح"))
)
