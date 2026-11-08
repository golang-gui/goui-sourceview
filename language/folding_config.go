package language

import (
	"context"
	"strings"
)

// Sections are physical text regions, not merged configuration objects. Values
// and continuation lines retain the classifications of the existing tokenizer.
func sectionFolding(id ID, t Tokenizer) FoldingFunc {
	return func(ctx context.Context, text string) ([]FoldRange, error) {
		f := structuralFolder{}
		start, last := -1, -1
		err := syntaxLines(ctx, t, text, func(l syntaxLine) {
			if sectionHeaderLine(ctx, id, l) {
				f.add(start, last)
				start, last = l.row, l.row
			} else if start >= 0 && strings.TrimSpace(l.text) != "" {
				last = l.row
			}
		})
		if err != nil {
			return nil, err
		}
		if start >= 0 {
			f.add(start, last)
		}
		return f.out, nil
	}
}

func sectionHeaderToken(id ID, t Token) bool {
	if t.Class&(ClassString|ClassComment) != 0 {
		return false
	}
	switch id {
	case INI:
		return t.Kind == KindTag
	case GitConfig:
		return t.Kind == KindMeta
	case EditorConfig, Systemd, DesktopEntry:
		return t.Kind == KindDelimiter || t.Kind == KindMeta
	}
	return false
}

func sectionHeaderLine(ctx context.Context, id ID, l syntaxLine) bool {
	start := len(l.text) - len(strings.TrimLeft(l.text, " \t"))
	if start == len(l.text) || l.text[start] != '[' {
		return false
	}
	opened, end := false, -1
	for _, t := range l.tokens {
		if t.Start <= start && start < t.End {
			opened = sectionHeaderToken(id, t)
		}
		if !opened || !sectionHeaderToken(id, t) {
			continue
		}
		for i := max(start+1, t.Start); i < t.End; i++ {
			if i%4096 == 0 && ctx.Err() != nil {
				return false
			}
			if l.text[i] == ']' {
				end = i
				if id != EditorConfig {
					break
				}
			}
		}
		// EditorConfig's outer closing bracket can follow glob character
		// classes. Other scanners identify the first real header close.
		if end >= 0 && id != EditorConfig {
			break
		}
	}
	if end < 0 || strings.TrimSpace(l.text[start+1:end]) == "" {
		return false
	}
	// Only lexer-confirmed comments may follow a header. In particular,
	// EditorConfig has no inline comments and quoted suffixes are not headers.
	at := end + 1
	for _, t := range l.tokens {
		if t.Class&ClassComment != 0 && t.Start >= at {
			if strings.TrimSpace(l.text[at:t.Start]) != "" {
				return false
			}
			at = t.End
		}
	}
	return strings.TrimSpace(l.text[at:]) == ""
}
