package language

import "strings"

// hclFoldObserver belongs to a single folding scan. Directive blocks belong to
// their template literal, while interpolation headers can suspend that literal.
// No observation is attached to hcState or its persistent expression frames.
type hclFoldObserver struct {
	templates [][]hclFoldBlock
	headers   []hclFoldHeader
}
type hclFoldBlock struct {
	word string
	row  int
}
type hclFoldHeader struct {
	row, owner    int
	directive     bool
	word          string
	body, invalid bool
}

func (f *foldLexemes) hclEnter(directive bool) {
	if f != nil && f.hcl != nil {
		f.hcl.headers = append(f.hcl.headers, hclFoldHeader{row: f.row, owner: len(f.hcl.templates) - 1, directive: directive})
	}
}
func (f *foldLexemes) hclHead(c byte) {
	if f == nil || f.hcl == nil || len(f.hcl.headers) == 0 {
		return
	}
	h := &f.hcl.headers[len(f.hcl.headers)-1]
	if !h.directive || space(c) || c == '~' || c == '}' {
		return
	}
	if h.word != "" {
		h.body = true
	} else if c < 128 && !codeNameStart(HCL, rune(c)) {
		h.invalid = true
	}
}
func (f *foldLexemes) hclWord(word string) {
	if f == nil || f.hcl == nil || len(f.hcl.headers) == 0 {
		return
	}
	h := &f.hcl.headers[len(f.hcl.headers)-1]
	if h.word == "" {
		h.word = word
	} else {
		h.body = true
	}
}
func (f *foldLexemes) hclLeave(end int) {
	if f == nil || f.hcl == nil || len(f.hcl.headers) == 0 {
		return
	}
	o := f.hcl
	h := o.headers[len(o.headers)-1]
	o.headers = o.headers[:len(o.headers)-1]
	if !h.directive || h.invalid || h.owner < 0 || h.owner >= len(o.templates) {
		return
	}
	blocks := &o.templates[h.owner]
	switch h.word {
	case "if", "for":
		if h.body {
			*blocks = append(*blocks, hclFoldBlock{h.word, h.row})
		}
	case "endif", "endfor":
		if h.body || len(*blocks) == 0 {
			return
		}
		b := (*blocks)[len(*blocks)-1]
		if h.word != "end"+b.word {
			*blocks = nil
			return
		}
		*blocks = (*blocks)[:len(*blocks)-1]
		if b.row < f.row {
			f.closed = append(f.closed, foldLexeme{b.row, end})
		}
	}
}

func indentBlockHeader(id ID, code string) bool {
	if id != Starlark {
		return pythonBlockHeader(code)
	}
	if !strings.HasSuffix(code, ":") {
		return false
	}
	// Starlark reserves Python's other compound-statement words but does not
	// define suites for class, while, try, with, match, or async statements.
	for _, word := range []string{"def", "if", "elif", "else", "for"} {
		if len(code) > len(word) && code[:len(word)] == word && (space(code[len(word)]) || code[len(word)] == ':' || code[len(word)] == '(') {
			return true
		}
	}
	return false
}
