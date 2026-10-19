package markdown

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/golang-gui/goui-sourceview/language"
)

var listPrefix = regexp.MustCompile(`^(\s*)([-+*]|[0-9]+\.)( \[[ xX]\])? +`)

func newline(c language.NewlineContext) (language.NewlineEdit, bool) {
	if strings.HasSuffix(c.Before, "```") && strings.HasPrefix(c.After, "```") {
		text := "\n" + c.Indent + "\n" + c.Indent
		return language.NewlineEdit{Text: text, Caret: 1 + len(c.Indent)}, true
	}
	if !c.Known || c.Context.String || c.Context.Comment {
		return language.NewlineEdit{}, false
	}
	if match := listPrefix.FindStringSubmatch(c.Before); match != nil {
		if len(match[0]) == len(c.Before) {
			return language.NewlineEdit{DeleteBefore: len(match[0]) - len(match[1]), Text: "\n" + match[1], Caret: 1 + len(match[1])}, true
		}
		marker := match[2]
		if strings.HasSuffix(marker, ".") {
			value, err := strconv.ParseUint(marker[:len(marker)-1], 10, 32)
			if err != nil {
				return language.NewlineEdit{}, false
			}
			marker = strconv.FormatUint(value+1, 10) + "."
		}
		text := "\n" + match[1] + marker
		if match[3] != "" {
			text += " [ ]"
		}
		text += " "
		return language.NewlineEdit{Text: text, Caret: len(text)}, true
	}
	if strings.HasPrefix(strings.TrimLeft(c.Before, " \t"), "> ") {
		text := "\n" + c.Indent + "> "
		return language.NewlineEdit{Text: text, Caret: len(text)}, true
	}
	return language.NewlineEdit{}, false
}
