package main

import (
	"embed"

	source "github.com/golang-gui/goui-sourceview"
	"github.com/golang-gui/goui-sourceview/language"
)

// Original samples are embedded so the binary can run from any directory.
//
//go:embed samples/*
var samples embed.FS

type exampleDocument struct {
	name     string
	document *source.Document
}

func sampleDocuments() []exampleDocument {
	var documents []exampleDocument
	for _, name := range []string{
		"settings.json", "layout.xml", "index.html", "config.toml", "config.yaml", "script.sh", "guide.md",
		"settings.jsonc", "requests.http", "service.proto", "Makefile", "CMakeLists.txt", "nginx.conf", "settings.ini",
		"Dockerfile", "query.sql", "hello.go", "hello.py", "app.js", "app.ts", "theme.css", "events.jsonl",
		"hello.c", "hello.cpp", "Hello.java", "hello.rs", "Hello.cs", "hello.lua",
		"hello.ps1",
		"hello.rb",
		"hello.php",
		"app.jsx", "app.tsx",
		"hello.kt",
		"hello.swift",
	} {
		text, err := samples.ReadFile("samples/" + name)
		if err != nil {
			panic(err)
		}
		doc := source.NewDocument(string(text))
		if err := doc.SetLanguage(language.Guess(name, text)); err != nil {
			panic(err) // Embedded language IDs are fixed, not user input.
		}
		documents = append(documents, exampleDocument{name, doc})
	}
	return documents
}
