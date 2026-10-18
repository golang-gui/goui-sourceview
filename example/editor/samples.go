package main

import (
	_ "embed"

	source "github.com/golang-gui/goui-sourceview"
	"github.com/golang-gui/goui-sourceview/language"
)

// Original samples are embedded so the binary can run from any directory.
//
//go:embed samples/settings.json
var jsonSample string

//go:embed samples/layout.xml
var xmlSample string

//go:embed samples/index.html
var htmlSample string

type exampleDocument struct {
	name     string
	document *source.Document
}

func sampleDocuments() []exampleDocument {
	var documents []exampleDocument
	for _, sample := range []struct {
		name, text string
		language   language.ID
	}{
		{"settings.json", jsonSample, language.JSON},
		{"layout.xml", xmlSample, language.XML},
		{"index.html", htmlSample, language.HTML},
	} {
		doc := source.NewDocument(sample.text)
		if err := doc.SetLanguage(sample.language); err != nil {
			panic(err) // Embedded language IDs are fixed, not user input.
		}
		documents = append(documents, exampleDocument{sample.name, doc})
	}
	return documents
}
