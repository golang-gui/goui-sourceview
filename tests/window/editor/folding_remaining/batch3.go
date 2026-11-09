package main

import "github.com/golang-gui/goui-sourceview/language"

func batchThree() []fixture {
	return []fixture{
		{id: language.BibTeX, text: "@article(key,\n title=\"needle \\\" ) fake\n last\"\n)", want: ranges([2]int{0, 3}), after: nil, keyRow: 1, keyHeader: 0, pointerRow: 0, editRow: 3},
		{id: language.BibTeX, text: "@comment{\n @book{fake}\n}\n@article{key,\n title={needle}\n}", want: ranges([2]int{3, 5}), after: nil, keyRow: 4, keyHeader: 3, pointerRow: 3, editRow: 5},
		{id: language.Gettext, text: "# first\nmsgid \"needle\"\nmsgstr \"one\"\n\nmsgid \"two\"\nmsgstr \"second\"", want: ranges([2]int{0, 2}, [2]int{4, 5}), after: ranges([2]int{4, 5}), keyRow: 1, keyHeader: 0, pointerRow: 0, editRow: 1},
		{id: language.Dotenv, text: "KEY=\"one\nneedle\nlast\"\nOTHER=value", want: ranges([2]int{0, 2}), after: nil, keyRow: 1, keyHeader: 0, pointerRow: 0, editRow: 2},
		{id: language.Properties, text: "key=one\\\n needle\nother=value", want: ranges([2]int{0, 1}), after: nil, keyRow: 1, keyHeader: 0, pointerRow: 0, editRow: 0},
		{id: language.CSV, text: "id,\"one\nneedle\nlast\",other", want: ranges([2]int{0, 1}), after: nil, keyRow: 1, keyHeader: 0, pointerRow: 0, editRow: 2},
		{id: language.CSV, text: "\"one\"\"\nneedle\nlast\"", want: ranges([2]int{0, 2}), after: nil, keyRow: 1, keyHeader: 0, pointerRow: 0, editRow: 2},
	}
}
