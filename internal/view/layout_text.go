package view

import "strings"

// NativeLayoutText supplies a length-preserving display string. Pango truncates
// at NUL even with an explicit positive byte length. Use a measured blank cell
// for that otherwise unrepresentable character on every backend. Model text and
// UTF-8 byte offsets remain unchanged; special-character markers identify NUL.
func NativeLayoutText(text string) string {
	if strings.IndexByte(text, 0) < 0 {
		return text
	}
	return strings.ReplaceAll(text, "\x00", " ")
}
