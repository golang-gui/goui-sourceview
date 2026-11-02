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
		"hello.fsx", "hello.ex", "hello.erl", "hello.jl", "hello.clj", "Main.hs", "hello.ml",
		"export.csv",
		"export.tsv",
		"guide.adoc",
		"guide.rst",
		"paper.tex",
		"references.bib",
		"messages.po",
		"settings.editorconfig",
		"settings.json5",
		"data.edn",
		"settings.gitconfig",
		"ssh_config",
		"jobs.crontab",
		"flow.mmd",
		"graph.dot",
		"View.xaml", "Cargo.lock",
		"page-yaml.md", "page-toml.md",
		"deployment.yaml.gotmpl", "deployment.yaml.j2", "settings.toml.j2", "view.xml.gotmpl",
	} {
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

// Compound filenames choose an explicit supported host for this example.
func setTemplateHost(doc *source.Document, name string) error {
	name = strings.ToLower(name)
	var makeTemplate func(*language.Language) (*language.Language, error)
	stem := name
	hostID := language.Plain
	if strings.HasSuffix(name, ".gohtml") {
		makeTemplate = language.NewGoTemplate
		hostID = language.HTML
	}
	if strings.HasSuffix(name, ".gotmpl") {
		makeTemplate = language.NewGoTemplate
		stem = strings.TrimSuffix(name, ".gotmpl")
	}
	for _, suffix := range []string{".j2", ".jinja", ".jinja2"} {
		if strings.HasSuffix(name, suffix) {
			makeTemplate = language.NewJinja2
			stem = strings.TrimSuffix(name, suffix)
		}
	}
	if makeTemplate == nil {
		return nil
	}
	if hostID == language.Plain {
		hostID = language.Guess(stem, nil)
	}
	switch hostID {
	case language.HTML, language.XML, language.YAML, language.TOML:
	default:
		return nil
	}
	host, _ := language.Builtin(hostID)
	l, err := makeTemplate(host)
	if err != nil {
		return err
	}
	doc.SetLanguageDefinition(l)
	return nil
}
