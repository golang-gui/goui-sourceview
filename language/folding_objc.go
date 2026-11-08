package language

import (
	"context"
	"unicode/utf8"
)

type objcFoldBlock struct {
	row             int
	protocol, named bool
}
type objcFoldPolicy struct {
	groups          []byte
	pending, active *objcFoldBlock
}

// Only lexer-confirmed directives at declaration depth participate. A protocol
// name stays tentative until the next token distinguishes a definition from a
// comma/semicolon forward declaration; @protocol(...) is an expression.
func (p *objcFoldPolicy) line(ctx context.Context, l syntaxLine, f *structuralFolder) {
	for i, a := range keywordFoldAtoms(ctx, l) {
		if i%1024 == 0 && ctx.Err() != nil {
			return
		}
		if a.class&(ClassString|ClassComment) != 0 {
			continue
		}
		value := l.text[a.start:a.end]
		if b := p.pending; b != nil {
			if !b.named {
				r, _ := utf8.DecodeRuneInString(value)
				if a.class == 0 && value[0] != '@' && codeNameStart(C, r) && (a.kind == KindPlain || a.kind == KindKeyword) {
					b.named = true
					if !b.protocol {
						p.active, p.pending = b, nil
					}
				} else {
					p.pending = nil
				}
			} else {
				if value != ";" && value != "," && value != "(" {
					p.active = b
				}
				p.pending = nil
			}
		}
		if len(p.groups) == 0 && a.kind == KindKeyword {
			switch value {
			case "@interface", "@implementation", "@protocol":
				// A protocol expression/forward declaration must not discard the
				// surrounding implementation before its next token is known.
				if value != "@protocol" {
					p.active = nil
				}
				p.pending = &objcFoldBlock{row: l.row, protocol: value == "@protocol"}
			case "@end":
				if p.active != nil {
					f.add(p.active.row, codeFoldEndLine(l.text, a.end, l.row))
					p.active = nil
				}
			}
		}
		if a.class&ClassDelimiter == 0 || len(value) != 1 {
			continue
		}
		c := value[0]
		switch c {
		case '(', '[', '{':
			p.groups = append(p.groups, c)
		case ')', ']', '}':
			if n := len(p.groups); n > 0 {
				o := p.groups[n-1]
				if o == '(' && c == ')' || o == '[' && c == ']' || o == '{' && c == '}' {
					p.groups = p.groups[:n-1]
				} else {
					p.groups, p.pending, p.active = nil, nil, nil
				}
			}
		}
	}
}
