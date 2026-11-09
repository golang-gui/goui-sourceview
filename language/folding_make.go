package language

import (
	"context"
	"strings"
)

type makeFoldBlock struct {
	row  int
	kind string
}

func makeFolding(t Tokenizer) FoldingFunc {
	return func(ctx context.Context, text string) ([]FoldRange, error) {
		f := structuralFolder{}
		var blocks []makeFoldBlock
		target, recipe, logical := -1, -1, -1
		flush := func() {
			if target >= 0 {
				f.add(target, recipe)
			}
			target, recipe = -1, -1
		}
		err := syntaxLines(ctx, t, text, func(l syntaxLine) {
			before, after := l.before.(configState), l.after.(configState)
			isRecipe := len(l.text) > 0 && l.text[0] == before.recipePrefix || before.recipe && before.continued
			trim := strings.TrimSpace(l.code)
			inDefine := len(blocks) > 0 && blocks[len(blocks)-1].kind == "define"
			if !inDefine {
				if !before.continued && after.continued {
					logical = l.row
				}
				if logical >= 0 && !after.continued {
					f.add(logical, l.row)
					logical = -1
				}
				if isRecipe {
					if target >= 0 {
						recipe = l.row
					}
					return
				}
				if trim == "" {
					return
				}
				flush()
			}
			if isRecipe || before.continued || before.variable || before.mode != configCode {
				return
			}
			a, z := leadingFoldKeyword(l)
			head := ""
			tail := ""
			if z > a && (z == len(l.text) || space(l.text[z])) {
				head = l.text[a:z]
				tail = strings.TrimSpace(l.code[z:])
				for head == "override" || head == "export" || head == "private" {
					if tail == "" {
						head = ""
						break
					}
					word := strings.Fields(tail)[0]
					head, tail = word, strings.TrimSpace(tail[len(word):])
				}
			}
			assignment := strings.HasPrefix(tail, "=") || strings.HasPrefix(tail, ":=") || strings.HasPrefix(tail, "+=") || strings.HasPrefix(tail, "?=") || strings.HasPrefix(tail, "!=")
			if !assignment {
				switch head {
				case "define":
					if tail != "" {
						blocks = append(blocks, makeFoldBlock{l.row, "define"})
					}
					return
				case "ifdef", "ifndef", "ifeq", "ifneq":
					if !inDefine && tail != "" {
						blocks = append(blocks, makeFoldBlock{l.row, "if"})
					}
					return
				case "endef", "endif":
					if tail == "" && len(blocks) > 0 {
						want := "if"
						if head == "endef" {
							want = "define"
						}
						top := blocks[len(blocks)-1]
						if top.kind == want {
							f.add(top.row, l.row)
							blocks = blocks[:len(blocks)-1]
						}
					}
					return
				case "else":
					return
				}
			}
			if inDefine {
				return
			}
			// Only literal target headers, with no assignment or expansion before
			// their real colon, own immediately following recipe lines.
			for _, token := range l.tokens {
				if token.Class&(ClassString|ClassComment) != 0 || token.Kind == KindVariable || token.Kind == KindEscape || token.Kind == KindKeyword {
					return
				}
				for at := token.Start; at < token.End; at++ {
					switch l.text[at] {
					case '=':
						return
					case ':':
						if token.Class&ClassDelimiter != 0 && strings.TrimSpace(l.text[:at]) != "" && !(at+1 < len(l.text) && l.text[at+1] == '=') {
							target = l.row
						}
						return
					}
				}
			}
		})
		if err != nil {
			return nil, err
		}
		flush()
		return noncrossingMixedFolds(ctx, f.out)
	}
}
