package editor

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	textcore "github.com/golang-gui/goui-sourceview/internal/text"
)

func GoToOffset(m *textcore.Model, query string) (int, error) {
	parts := strings.Split(strings.TrimSpace(query), ":")
	if len(parts) < 1 || len(parts) > 2 {
		return 0, fmt.Errorf("sourceview: invalid line/column")
	}
	line, err := strconv.Atoi(parts[0])
	if err != nil || line < 1 || line > m.LineCount() {
		return 0, fmt.Errorf("sourceview: line out of range")
	}
	column := 1
	if len(parts) == 2 {
		column, err = strconv.Atoi(parts[1])
		if err != nil || column < 1 {
			return 0, fmt.Errorf("sourceview: column out of range")
		}
	}
	r, _ := m.LineRange(line - 1)
	text, _ := m.Slice(r)
	at := 0
	for n := 1; n < column; n++ {
		if at == len(text) {
			return 0, fmt.Errorf("sourceview: column out of range")
		}
		_, size := utf8.DecodeRuneInString(text[at:])
		at += size
	}
	return r.Start + at, nil
}
