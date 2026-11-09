package main

import "github.com/golang-gui/goui-sourceview/language"

func batchFour() []fixture {
	return []fixture{
		{id: language.Mermaid, text: "flowchart TB\nsubgraph outer\n subgraph inner\n A[needle]-->B\n end\nend", want: ranges([2]int{1, 5}, [2]int{2, 4}), after: ranges([2]int{2, 4}), keyRow: 3, keyHeader: 2, pointerRow: 2, editRow: 5},
		{id: language.Mermaid, text: "flowchart TB\nA[first\nsubgraph fake\nend\nlast]\nsubgraph real\nB[needle]-->C\nend", want: ranges([2]int{5, 7}), after: nil, keyRow: 6, keyHeader: 5, pointerRow: 5, editRow: 7},
		{id: language.Mermaid, text: "sequenceDiagram\nloop work\nalt Yes\nA->>B: needle\nelse No\nA->>B: text\nend\nend", want: ranges([2]int{1, 7}, [2]int{2, 6}), after: ranges([2]int{2, 6}), keyRow: 3, keyHeader: 2, pointerRow: 2, editRow: 7},
		{id: language.Mermaid, text: "sequenceDiagram\npar work\ncritical connect\nA->>B: needle\noption timeout\nA->>B: text\nend\nand other\nA->>C: last\nend", want: ranges([2]int{1, 9}, [2]int{2, 6}), after: ranges([2]int{2, 9}), keyRow: 3, keyHeader: 2, pointerRow: 2, editRow: 6},
		{id: language.Mermaid, text: "sequenceDiagram\nbox Aqua Group\nparticipant A as needle\nparticipant B\nend", want: ranges([2]int{1, 4}), after: nil, keyRow: 2, keyHeader: 1, pointerRow: 1, editRow: 1},
	}
}
