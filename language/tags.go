package language

import (
	"slices"
	"strings"
)

// TagRules describes conservative XML-like paired elements. Names and lists
// are immutable inside Language. CaseSensitive=false applies ASCII folding.
// Optional-end-tag inference is deliberately outside this lexical policy.
type TagRules struct {
	CaseSensitive                 bool
	VoidElements, RawTextElements []string
}
type NameRange struct{ Start, End int }
type TagMatch struct{ Opening, Closing NameRange }

func XMLTags() *TagRules { return &TagRules{CaseSensitive: true} }
func HTMLTags() *TagRules {
	return &TagRules{VoidElements: strings.Fields("area base br col embed hr img input link meta param source track wbr"), RawTextElements: strings.Fields("script style textarea title")}
}
func (t *TagRules) copy() *TagRules {
	if t == nil {
		return nil
	}
	return &TagRules{t.CaseSensitive, slices.Clone(t.VoidElements), slices.Clone(t.RawTextElements)}
}
func (t *TagRules) key(s string) string {
	if !t.CaseSensitive {
		return strings.ToLower(s)
	}
	return s
}
func (t *TagRules) contains(list []string, s string) bool {
	for _, n := range list {
		if t.key(n) == s {
			return true
		}
	}
	return false
}
func (t *TagRules) tags(text string) (pairs []TagMatch, last markupTag) {
	var stack []markupTag
	for at := 0; at < len(text); {
		n := strings.IndexByte(text[at:], '<')
		if n < 0 {
			break
		}
		at += n
		raw := len(stack) > 0 && t.contains(t.RawTextElements, stack[len(stack)-1].name)
		tag, ok := readMarkupTag(text, at)
		if raw {
			if !ok || !tag.closing || t.key(tag.name) != stack[len(stack)-1].name {
				at++
				continue
			}
		}
		if !raw {
			terminator := ""
			switch {
			case strings.HasPrefix(text[at:], "<!--"):
				terminator = "-->"
			case strings.HasPrefix(text[at:], "<![CDATA["):
				terminator = "]]>"
			case strings.HasPrefix(text[at:], "<?"):
				terminator = "?>"
			}
			if terminator != "" {
				n := strings.Index(text[at+2:], terminator)
				if n < 0 {
					break
				}
				at += 2 + n + len(terminator)
				continue
			}
			if strings.HasPrefix(text[at:], "<!") {
				quote, depth, end := byte(0), 0, at+2
				for ; end < len(text); end++ {
					c := text[end]
					if quote != 0 {
						if c == quote {
							quote = 0
						}
						continue
					}
					if c == '"' || c == '\'' {
						quote = c
					} else if c == '[' {
						depth++
					} else if c == ']' {
						depth = max(0, depth-1)
					} else if c == '>' && depth == 0 {
						break
					}
				}
				if end == len(text) {
					break
				}
				at = end + 1
				continue
			}
		}
		if !ok {
			at++
			continue
		}
		tag.name = t.key(tag.name)
		at = tag.end
		last = tag
		if tag.self || t.contains(t.VoidElements, tag.name) {
			continue
		}
		if !tag.closing {
			stack = append(stack, tag)
			continue
		}
		if len(stack) == 0 {
			continue
		}
		open := stack[len(stack)-1]
		if open.name != tag.name {
			stack = nil
			continue
		}
		stack = stack[:len(stack)-1]
		pairs = append(pairs, TagMatch{Opening: NameRange{open.nameStart, open.nameEnd}, Closing: NameRange{tag.nameStart, tag.nameEnd}})
	}
	return
}

// Match returns a confirmed name pair containing offset (including name edges).
func (t *TagRules) Match(text string, offset int) (TagMatch, bool) {
	if t == nil {
		return TagMatch{}, false
	}
	pairs, _ := t.tags(text)
	for _, p := range pairs {
		if p.Opening.Start <= offset && offset <= p.Opening.End || p.Closing.Start <= offset && offset <= p.Closing.End {
			return p, true
		}
	}
	return TagMatch{}, false
}

// ClosingTag returns the exact closing spelling for a complete last opening tag.
// The prefix must begin at a known document/embedded boundary, not a random slice.
func (t *TagRules) ClosingTag(prefix string) string {
	if t == nil {
		return ""
	}
	_, tag := t.tags(prefix)
	if tag.end != len(prefix) || tag.closing || tag.self || tag.name == "" || t.contains(t.VoidElements, tag.name) {
		return ""
	}
	return "</" + prefix[tag.nameStart:tag.nameEnd] + ">"
}

// ValidName validates a whole tag name for linked editing.
func (t *TagRules) ValidName(name string) bool {
	if name == "" {
		return false
	}
	for at, r := range name {
		if !tagNameRune(r, at == 0) {
			return false
		}
	}
	return true
}
