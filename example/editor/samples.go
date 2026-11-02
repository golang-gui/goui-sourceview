package main

import (
	"embed"
	"strings"

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
		"hello.dart",
		"hello.R",
		"hello.zig",
		"hello.m",
		"hello.mm",
		"hello.scala",
		"hello.pl",
		"change.diff",
		"build.cmd",
		"main.tf",
		"query.graphql",
		"theme.scss",
		"theme.less",
		"go.mod.sample", "go.work.sample", "go.sum.sample",
		"settings.env",
		"messages.properties",
		"rules.gitignore",
		"rules.gitattributes",
		"rules.bzl",
		"build.gradle",
		"page.gohtml", "page.html.j2",
		"editor.service", "editor.desktop",
		"default.nix", "filter.jq", "query.promql",
		"hello.fsx", "hello.ex", "hello.erl", "hello.jl", "hello.clj"} {
		text, err := samples.ReadFile("samples/" + name)
		if err != nil {
			panic(err)
		}
		doc := source.NewDocument(string(text))
		name = strings.TrimSuffix(name, ".sample")
		if err := doc.SetLanguage(language.Guess(name, text)); err != nil {
			panic(err) // Embedded language IDs are fixed, not user input.
		}
		if err := setTemplateHost(doc, name); err != nil {
			panic(err)
		}
		documents = append(documents, exampleDocument{name, doc})
	}
	return documents
}

// Compound filenames choose the example's explicit HTML host.
func setTemplateHost(doc *source.Document, name string) error {
	name = strings.ToLower(name)
	var makeTemplate func(*language.Language) (*language.Language, error)
	if strings.HasSuffix(name, ".gohtml") {
		makeTemplate = language.NewGoTemplate
	}
	if strings.HasSuffix(name, ".html.j2") || strings.HasSuffix(name, ".html.jinja") || strings.HasSuffix(name, ".html.jinja2") {
		makeTemplate = language.NewJinja2
	}
	if makeTemplate == nil {
		return nil
	}
	html, _ := language.Builtin(language.HTML)
	l, err := makeTemplate(html)
	if err != nil {
		return err
	}
	doc.SetLanguageDefinition(l)
	return nil
}
