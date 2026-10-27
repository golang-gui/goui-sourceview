//go:build ignore

package main

import "fmt"

// Document holds a small editable source.
type Document struct {
	Title string
	Tags  []string
}

func main() {
	doc := Document{Title: "SourceView", Tags: []string{"native", "Go"}}
	for index, tag := range doc.Tags {
		fmt.Printf("%d: %s — %s\n", index, doc.Title, tag)
	}
	fmt.Println(`A raw string can span
multiple lines.`)
}
