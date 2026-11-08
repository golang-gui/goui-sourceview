package language

import "context"

type queryFoldBoundary struct {
	start, end int
	enter      bool
}
type queryFoldObserver struct{ events []queryFoldBoundary }

func (f *foldLexemes) queryBoundary(start, end int, enter bool) {
	if f != nil && f.query != nil {
		f.query.events = append(f.query.events, queryFoldBoundary{start, end, enter})
	}
}

const (
	queryFoldName byte = iota
	queryFoldHeader
	queryFoldThen
	queryFoldBody
	queryFoldLet
)

type queryFoldBlock struct {
	word       string
	row, level int
	phase      byte
}
type queryFoldScope struct {
	groups []byte
	blocks []queryFoldBlock
	last   string
}
type queryFoldPolicy struct {
	id     ID
	scopes []queryFoldScope
}

// Scanner-confirmed interpolation boundaries suspend the enclosing control
// scope. A malformed inner end/in cannot close a definition in the outer scope.
func (p *queryFoldPolicy) line(ctx context.Context, l syntaxLine, events []queryFoldBoundary, f *structuralFolder) {
	at, skip := 0, -1
	for i, a := range keywordFoldAtoms(ctx, l) {
		if i%1024 == 0 && ctx.Err() != nil {
			return
		}
		for at < len(events) && events[at].start <= a.start {
			e := events[at]
			at++
			skip = e.end
			if e.enter {
				p.scopes = append(p.scopes, queryFoldScope{})
			} else if len(p.scopes) > 1 {
				p.scopes = p.scopes[:len(p.scopes)-1]
			}
		}
		if a.start < skip {
			continue
		}
		p.scopes[len(p.scopes)-1].atom(p.id, a, l, f)
	}
}

func (s *queryFoldScope) atom(id ID, a keywordFoldAtom, l syntaxLine, f *structuralFolder) {
	if a.class&ClassComment != 0 {
		return
	}
	value := l.text[a.start:a.end]
	var top *queryFoldBlock
	if n := len(s.blocks); n > 0 {
		top = &s.blocks[n-1]
	}
	if a.class&ClassString != 0 || a.kind == KindVariable || a.kind == KindProperty {
		if top != nil && top.word == "def" && top.phase == queryFoldName {
			s.blocks = s.blocks[:len(s.blocks)-1]
		}
		s.last = "literal"
		return
	}
	if top != nil && top.phase == queryFoldLet {
		if value == "{" {
			s.blocks = s.blocks[:len(s.blocks)-1]
			top = nil
		} else {
			top.phase = queryFoldBody
		}
	}
	if top != nil && top.word == "def" {
		if top.phase == queryFoldName {
			if a.kind == KindPlain && len(value) > 0 && (value[0] == '_' || value[0] >= 'a' && value[0] <= 'z' || value[0] >= 'A' && value[0] <= 'Z') {
				top.phase = queryFoldHeader
			} else {
				s.blocks = s.blocks[:len(s.blocks)-1]
				top = nil
			}
		} else if top.phase == queryFoldHeader && len(s.groups) == top.level && value == ":" {
			top.phase = queryFoldBody
		}
	}
	if a.kind == KindKeyword && s.last != "." {
		switch {
		case id == Nix && value == "let":
			s.blocks = append(s.blocks, queryFoldBlock{"let", l.row, len(s.groups), queryFoldLet})
		case id == Nix && value == "in":
			if top != nil && top.word == "let" && top.level == len(s.groups) {
				f.add(top.row, l.row-1)
				s.blocks = s.blocks[:len(s.blocks)-1]
			}
		case id == JQ && value == "def":
			s.blocks = append(s.blocks, queryFoldBlock{"def", l.row, len(s.groups), queryFoldName})
		case id == JQ && value == "if":
			s.blocks = append(s.blocks, queryFoldBlock{"if", l.row, len(s.groups), queryFoldThen})
		case id == JQ && top != nil && top.word == "if" && top.level == len(s.groups):
			switch value {
			case "then":
				if top.phase == queryFoldThen {
					top.phase = queryFoldBody
				}
			case "elif":
				if top.phase == queryFoldBody {
					top.phase = queryFoldThen
				}
			case "end":
				if top.phase == queryFoldBody {
					f.add(top.row, codeFoldEndLine(l.text, a.end, l.row))
				}
				s.blocks = s.blocks[:len(s.blocks)-1]
			}
		}
	}
	if id == JQ && value == ";" && top != nil && top.word == "def" && top.level == len(s.groups) {
		if top.phase == queryFoldBody {
			f.add(top.row, codeFoldEndLine(l.text, a.end, l.row))
		}
		s.blocks = s.blocks[:len(s.blocks)-1]
	}
	if a.class&ClassDelimiter != 0 && len(value) == 1 {
		c := value[0]
		switch c {
		case '(', '[', '{':
			s.groups = append(s.groups, c)
		case ')', ']', '}':
			if n := len(s.groups); n > 0 {
				o := s.groups[n-1]
				if o == '(' && c == ')' || o == '[' && c == ']' || o == '{' && c == '}' {
					s.groups = s.groups[:n-1]
					for len(s.blocks) > 0 && s.blocks[len(s.blocks)-1].level > len(s.groups) {
						s.blocks = s.blocks[:len(s.blocks)-1]
					}
				} else {
					s.groups, s.blocks = nil, nil
				}
			}
		}
	}
	s.last = value
}
