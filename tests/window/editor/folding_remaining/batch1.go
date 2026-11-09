package main

import "github.com/golang-gui/goui-sourceview/language"

func batchOne() []fixture {
	html, _ := language.Builtin(language.HTML)
	goHost, err := language.NewGoTemplate(html)
	must(err)
	jinjaHost, err := language.NewJinja2(html)
	must(err)
	return []fixture{
		{id: language.Makefile, text: "ifdef X\ndefine TEXT\nneedle\nendef\nall:\n\techo value\nendif", want: ranges([2]int{0, 6}, [2]int{1, 3}, [2]int{4, 5}), after: ranges([2]int{1, 3}, [2]int{4, 5}), keyRow: 2, keyHeader: 1, pointerRow: 1, editRow: 0},
		{id: language.Makefile, text: ".RECIPEPREFIX=>\nall:\n>echo needle\n>echo next\nnext:\n>echo last", want: ranges([2]int{1, 3}, [2]int{4, 5}), after: nil, keyRow: 2, keyHeader: 1, pointerRow: 1, editRow: 0},
		{id: language.Batch, text: ":first\necho needle\nREM :fake\n:next\necho done", want: ranges([2]int{0, 2}, [2]int{3, 4}), after: ranges([2]int{3, 4}), keyRow: 1, keyHeader: 0, pointerRow: 0, editRow: 0},
		{id: language.Batch, text: ":: #region main\nREM #region inner\necho needle\nREM #endregion\n:: #endregion", want: ranges([2]int{0, 4}, [2]int{1, 3}), after: ranges([2]int{1, 3}), keyRow: 2, keyHeader: 1, pointerRow: 1, editRow: 4},
		{id: language.GoTemplate, text: "{{if .X}}\n{{range .Items}}\nneedle\n{{end}}\n{{else}}\nnone\n{{end}}", want: ranges([2]int{0, 6}, [2]int{1, 3}), after: ranges([2]int{1, 3}), keyRow: 2, keyHeader: 1, pointerRow: 1, editRow: 6},
		{id: language.Jinja2, text: "{% if x %}\n{% raw %}\nneedle {% endfor fake %}\n{% endraw %}\n{% else %}\nnone\n{% endif %}", want: ranges([2]int{0, 6}, [2]int{1, 3}), after: ranges([2]int{1, 3}), keyRow: 2, keyHeader: 1, pointerRow: 1, editRow: 6},
		{id: language.GoTemplate, definition: goHost, text: "<main>\n{{if .X}}\nneedle\n{{end}}\n</main>", want: ranges([2]int{1, 3}), after: nil, keyRow: 2, keyHeader: 1, pointerRow: 1, editRow: 3},
		{id: language.Jinja2, definition: jinjaHost, text: "<main>\n{% if x %}\nneedle\n{% endif %}\n</main>", want: ranges([2]int{1, 3}), after: nil, keyRow: 2, keyHeader: 1, pointerRow: 1, editRow: 3},
	}
}
