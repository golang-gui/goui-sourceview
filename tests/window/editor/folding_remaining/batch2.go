package main

import "github.com/golang-gui/goui-sourceview/language"

func batchTwo() []fixture {
	return []fixture{
		{id: language.AsciiDoc, text: "= Doc\n== First\nneedle\n----\n== fake\n----\n== Next\nlast", want: ranges([2]int{0, 7}, [2]int{1, 5}, [2]int{3, 5}, [2]int{6, 7}), after: ranges([2]int{0, 7}, [2]int{1, 7}), keyRow: 4, keyHeader: 3, pointerRow: 1, editRow: 5},
		{id: language.AsciiDoc, text: "====\nneedle\n== fake\n====\n== Real\nbody", want: ranges([2]int{0, 3}, [2]int{4, 5}), after: nil, keyRow: 1, keyHeader: 0, pointerRow: 0, editRow: 3},
		{id: language.ReStructuredText, text: "Title\n=====\nneedle\nChild\n-----\nbody\nNext\n====\nlast", want: ranges([2]int{0, 5}, [2]int{3, 5}, [2]int{6, 8}), after: ranges([2]int{0, 5}, [2]int{6, 8}), keyRow: 5, keyHeader: 3, pointerRow: 0, editRow: 4},
		{id: language.ReStructuredText, text: "Example::\n\n  needle\n  ==== fake\n  body\n\nTail", want: ranges([2]int{0, 4}), after: nil, keyRow: 2, keyHeader: 0, pointerRow: 0, editRow: 0},
		{id: language.LaTeX, text: `\begin{quote}` + "\n" + `\begin{verbatim}` + "\nneedle\n" + `\section{fake}` + "\n" + `\end{verbatim}` + "\n" + `\end{quote}` + "\n" + `\section{Real}` + "\nbody", want: ranges([2]int{0, 5}, [2]int{1, 4}, [2]int{6, 7}), after: ranges([2]int{1, 4}, [2]int{6, 7}), keyRow: 2, keyHeader: 1, pointerRow: 1, editRow: 5},
		{id: language.LaTeX, text: `\section{One}` + "\nneedle\n" + `\subsection{Child}` + "\nbody\n" + `\section{Next}` + "\nlast", want: ranges([2]int{0, 3}, [2]int{2, 3}, [2]int{4, 5}), after: ranges([2]int{0, 5}, [2]int{2, 5}), keyRow: 3, keyHeader: 2, pointerRow: 0, editRow: 4},
	}
}
