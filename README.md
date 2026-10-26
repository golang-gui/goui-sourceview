<h1 align="center">GOUI SourceView</h1>

<p align="center">A native code editor for Go applications</p>

<p align="center"><a href="doc/README_CN.md">中文</a></p>

GOUI SourceView brings source code editing to [GOUI](https://github.com/golang-gui/goui)
applications, with syntax highlighting, completion, matching brackets, and Modern
light/dark appearances. It supports both imperative and declarative integration.

The project is under active development.

## Installation

Requires Go 1.24 or newer. The current source depends on GOUI v0.4.0;
Go downloads the matching dependency for the version you install.

```sh
go get github.com/golang-gui/goui-sourceview@v0.1.0
```

## Features

- **Syntax highlighting** for JSON, XML, HTML, Markdown, TOML, YAML and Shell.
- **Folding and navigation** for structured documents, Markdown headings and code fences, with line/column navigation.
- **Editing assistance** with smart indentation, whitespace markers, indent guides and rulers.
- **Code editing** with Unicode selection, clipboard, undo/redo, indentation and line operations.
- **Completion and brackets** with suggestions, automatic pairs, matching-bracket navigation and linked tag names.
- **Custom languages** with application-defined highlighting, editing rules and completion.
- **Find and replace** in a floating panel, with multiline queries, regular expressions, capture replacements and selection scopes.
- **Light and dark appearances** with syntax colors preserved in selections.
- **Multilingual controls** with editor menus and find/replace available in eight languages.

## Screenshots

Modern Light — JSON

<img src="doc/images/editor-light.png" alt="GOUI SourceView editing JSON in Modern Light">

Modern Dark — HTML

<img src="doc/images/editor-dark.png" alt="GOUI SourceView editing HTML in Modern Dark">

Screenshots from the example running on Linux.

## Build with an AI coding assistant

The [SourceView application development Skill](skills/goui-sourceview-app/SKILL.md)
helps your AI integrate the editor and customize highlighting, editing rules, and completion.
To install it, tell your AI:

> Install the SourceView application development skill following https://github.com/golang-gui/goui-sourceview/blob/main/skills/INSTALL.md.

The [installation guide](skills/INSTALL.md) uses your tool's native skill mechanism.
Tools without Skill support can read the development guide directly. Installing the Skill does
not install or upgrade your application's Go dependencies.

## Try the example

Requires Go 1.24 or newer. Follow the [example guide](example/README.md) to prepare
the development environment, then run from the repository root:

```sh
go run ./example/editor
```

The example lets you edit JSON, XML, HTML, Markdown, TOML, YAML and Shell, try search and completion, and switch
between light and dark appearances. A [declarative example](example/declarative) and a [custom Markdown editor](example/markdown) are also available.

## License

[MIT](LICENSE)
