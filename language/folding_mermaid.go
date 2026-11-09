package language

import (
	"context"
	"strings"
)

// Only explicit blocks in flowcharts and sequence diagrams participate. This
// is not a Mermaid parser: labels, messages and other diagrams have no generic
// brace/indent fallback. Branches remain inside their enclosing block.
func mermaidFolding(t Tokenizer) FoldingFunc {
	return func(ctx context.Context, text string) ([]FoldRange, error) {
		f := structuralFolder{}
		var blocks []int
		var brackets []byte
		mode, frontmatter := "", false
		err := syntaxLines(ctx, t, text, func(l syntaxLine) {
			code := strings.TrimSpace(l.code)
			if mode == "" {
				if code == "" {
					return
				}
				if code == "---" {
					if frontmatter {
						mode = "header"
					} else {
						frontmatter = true
					}
					return
				}
				if frontmatter {
					return
				}
			}
			if mode == "" || mode == "header" {
				if code == "" {
					return
				}
				a, z := leadingFoldKeyword(l)
				mode = "other"
				if z > a && (z == len(l.text) || space(l.text[z]) || l.text[z] == ';') {
					switch l.text[a:z] {
					case "flowchart", "flowchart-elk", "graph":
						mode = "flowchart"
					case "sequenceDiagram":
						mode = "sequence"
					}
				}
				return
			}
			if mode == "other" {
				return
			}
			simple := !strings.Contains(strings.TrimSuffix(code, ";"), ";")
			if !simple {
				blocks = nil
			}
			if simple && len(brackets) == 0 {
				a, z := leadingFoldKeyword(l)
				if z > a && (z == len(l.text) || space(l.text[z]) || l.text[z] == ';') {
					word, tail := l.text[a:z], strings.TrimSpace(l.code[z:])
					if word == "end" && (tail == "" || tail == ";") {
						if len(blocks) > 0 {
							start := blocks[len(blocks)-1]
							blocks = blocks[:len(blocks)-1]
							f.add(start, l.row)
						}
					} else {
						open := mode == "flowchart" && word == "subgraph" && mermaidFoldDescription(l, z)
						if mode == "sequence" {
							switch word {
							case "loop", "alt", "opt", "par", "rect", "critical", "break", "box":
								open = true
							}
						}
						// A keyword can also name a message sender or a flowchart
						// node. A leading connector is not a block description.
						if tail != "" && strings.ContainsRune("-=<>!~:", rune(tail[0])) {
							open = false
						}
						if open {
							blocks = append(blocks, l.row)
						}
					}
				}
			}
			// Track punctuation only to keep unquoted multiline node labels
			// opaque. Never turn their brackets into folding candidates.
			for _, token := range l.tokens {
				if mode != "flowchart" || token.Class&ClassDelimiter == 0 {
					continue
				}
				for at := token.Start; at < token.End; at++ {
					if at%4096 == 0 && ctx.Err() != nil {
						return
					}
					c := l.text[at]
					switch c {
					case '[', '{', '(':
						brackets = append(brackets, c)
					case ']', '}', ')':
						if len(brackets) > 0 {
							top := brackets[len(brackets)-1]
							if top == '[' && c == ']' || top == '{' && c == '}' || top == '(' && c == ')' {
								brackets = brackets[:len(brackets)-1]
							} else {
								brackets = nil
							}
						}
					}
				}
			}
		})
		if err != nil {
			return nil, err
		}
		return noncrossingMixedFolds(ctx, f.out)
	}
}

// Quoted descriptions count, lexer-confirmed trailing comments do not.
func mermaidFoldDescription(l syntaxLine, start int) bool {
	if strings.TrimSpace(l.code[start:]) != "" {
		return true
	}
	for _, token := range l.tokens {
		if token.End > start && token.Class&ClassComment == 0 && token.Kind != KindMeta && strings.TrimSpace(l.text[max(start, token.Start):token.End]) != "" {
			return true
		}
	}
	return false
}
