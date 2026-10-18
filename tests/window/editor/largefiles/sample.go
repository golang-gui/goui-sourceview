package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/golang-gui/goui-sourceview/language"
)

const searchText = "SV_MATCH_2718"

var markers = [...]string{"SV_HEAD", "SV_MIDDLE", "SV_TAIL"}

type sample struct {
	path                                 string
	rows                                 int
	positions                            [3]int
	middleStart, middleEnd, followingEnd int
}

// Generated text is original project data. Every payload has the same UTF-8
// byte length, with sparse independent markers and no encoded line breaks.
func payload(marker string) string {
	text := marker + " 中文 é 👩‍💻 "
	if marker != "filler" {
		text += searchText + " "
	}
	return text + strings.Repeat("x", 160-len(text))
}

func generateSample(dir string, id language.ID, target int) (result sample, err error) {
	var header, footer string
	row := func(i int, content string, last bool) string {
		switch id {
		case language.JSON:
			comma := ","
			if last {
				comma = ""
			}
			return fmt.Sprintf("  {\"row\":\"%08d\",\"text\":\"%s\",\"flag\":true}%s\n", i, content, comma)
		case language.XML:
			return fmt.Sprintf("  <row id=\"%08d\"><text>%s</text><flag>true</flag></row>\n", i, content)
		default:
			return fmt.Sprintf("  <p data-row=\"%08d\">%s</p>\n", i, content)
		}
	}
	switch id {
	case language.JSON:
		header, footer = "[\n", "]\n"
	case language.XML:
		header, footer = "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<rows>\n", "</rows>\n"
	case language.HTML:
		header, footer = "<!doctype html>\n<html><body>\n", "</body></html>\n"
	default:
		return result, fmt.Errorf("unsupported sample language %q", id)
	}
	result.rows = max(5, (target-len(header)-len(footer))/len(row(0, payload("filler"), false)))
	f, err := os.CreateTemp(dir, "sourceview-*."+string(id))
	if err != nil {
		return result, err
	}
	result.path = f.Name()
	defer func() {
		if closeErr := f.Close(); err == nil {
			err = closeErr
		}
	}()
	w := bufio.NewWriter(f)
	write := func(text string) {
		if err == nil {
			_, err = w.WriteString(text)
		}
	}
	write(header)
	at := len(header)
	for i := 0; i < result.rows && err == nil; i++ {
		marker, slot := "filler", -1
		switch i {
		case 0:
			slot = 0
		case result.rows / 2:
			slot = 1
		case result.rows - 1:
			slot = 2
		}
		if slot >= 0 {
			marker = markers[slot]
		}
		line := row(i, payload(marker), i == result.rows-1)
		if slot >= 0 {
			result.positions[slot] = at + strings.Index(line, marker)
		}
		if slot == 1 {
			result.middleStart, result.middleEnd = at, at+len(line)
		}
		at += len(line)
		if i == result.rows/2+1 {
			result.followingEnd = at
		}
		write(line)
	}
	write(footer)
	if err == nil {
		err = w.Flush()
	}
	return result, err
}
