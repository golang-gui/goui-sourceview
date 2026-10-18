package main

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"io"
	"os"
	"testing"
	"unicode/utf8"

	"github.com/golang-gui/goui-sourceview/language"
)

func TestGeneratedSamples(t *testing.T) {
	for _, id := range []language.ID{language.JSON, language.XML, language.HTML} {
		s, err := generateSample(t.TempDir(), id, 4096)
		if err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(s.path)
		if err != nil {
			t.Fatal(err)
		}
		if !utf8.Valid(data) || bytes.ContainsRune(data, '\r') || bytes.Count(data, []byte(searchText)) != 3 {
			t.Fatal(id, "invalid encoding or sparse markers")
		}
		for i, marker := range markers {
			if bytes.Count(data, []byte(marker)) != 1 || !bytes.HasPrefix(data[s.positions[i]:], []byte(marker)) {
				t.Fatal(id, "incorrect marker position", marker)
			}
		}
		if s.middleStart >= s.positions[1] || s.middleEnd <= s.positions[1] || s.followingEnd <= s.middleEnd || data[s.middleEnd-1] != '\n' {
			t.Fatal(id, "incorrect independent line ranges")
		}
		switch id {
		case language.JSON:
			var rows []struct {
				Row, Text string
				Flag      bool
			}
			if err := json.Unmarshal(data, &rows); err != nil || len(rows) != s.rows || !rows[0].Flag {
				t.Fatal("invalid JSON fixture", err)
			}
		case language.XML, language.HTML:
			// The generated HTML deliberately uses a well-formed XML-compatible
			// subset, so the standard parser can check nesting and exact row count.
			rowTag := "row"
			if id == language.HTML {
				rowTag = "p"
			}
			d, rows := xml.NewDecoder(bytes.NewReader(data)), 0
			for {
				token, err := d.Token()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatal(err)
				}
				if start, ok := token.(xml.StartElement); ok && start.Name.Local == rowTag {
					rows++
				}
			}
			if rows != s.rows {
				t.Fatal(id, "invalid fixture row count")
			}
		}
	}
}
