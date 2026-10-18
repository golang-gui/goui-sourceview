<h1 align="center">GOUI SourceView</h1>

<p align="center">A native code editor for Go applications</p>

<p align="center"><a href="doc/README_CN.md">中文</a></p>

GOUI SourceView is an optional code editor extension for [GOUI](https://github.com/golang-gui/goui).
It provides editable source text, incremental JSON / XML / HTML highlighting, completion,
matching brackets, and a floating find-and-replace panel, with native text layout and
Modern light/dark appearances. Use the imperative widget or its thin declarative binding.

The project is under active development. Public APIs are evolving; the current source
requires GOUI APIs newer than its declared release dependency. See [running from source](#run-the-examples).

## See it in action

The [editor example](example/editor) includes three editable documents, undo/redo,
word wrap, line numbers, read-only mode, and appearance switching. Each document
keeps its text and undo history when you switch files. Edits stay in memory.

Modern Light — JSON

<img src="doc/images/editor-light.png" alt="GOUI SourceView showing JSON in a Modern Light Integrated window">

Modern Dark — HTML and the floating find-and-replace panel

<img src="doc/images/editor-dark.png" alt="GOUI SourceView showing HTML and find/replace in a Modern Dark Integrated window">

These are actual screenshots on Linux / X11, Xfwm4 with compositing, Software
rendering at 1x, using `WindowChromeIntegrated`. Native text and window controls
can differ on other desktops.

## What is available

- **Source editing:** Unicode selection, word/line selection and dragging, clipboard,
  undo/redo, indentation, line moving/duplicating/deleting, and familiar editor shortcuts.
- **Language support:** incremental JSON, XML and HTML highlighting, including
  cross-line lexical state. Language names and nearby document words provide completion candidates.
- **Code navigation:** automatic bracket/quote pairs, selection wrapping, matching-bracket
  decoration and jumping, line numbers, and optional word wrap.
- **Search:** Unicode literal search, case and whole-word options, next/previous match,
  current/all replacement, and an optional floating FindBar. Replace-all is one undo step.
- **Integration:** Basic and Modern styles, syntax-preserving selection colors,
  eight-language editor menus and FindBar, shared documents, and declarative views.

The widget is implemented independently on GOUI's public `gui/textedit` and typography
facilities. `Document` owns text, history and language analysis; each `SourceView` owns
its selection, viewport and native layouts. GOUI does not depend on this module.

## Run the examples

Requires **Go 1.24 or newer** and GOUI's [desktop runtime dependencies](https://github.com/golang-gui/goui#platforms-and-dependencies).
For this development snapshot, check out GOUI alongside this repository with commit
`903daf8` or a later version containing the same APIs. The declared `goui v0.1.1`
does not yet include the required batch editing, styling and registration APIs.

From this repository root, create a local workspace if one does not already exist:

```sh
go work init . ../goui
```

For an existing workspace, use `go work use . ../goui` instead. Keep the workspace
local; a machine-specific `replace` is unnecessary.

```sh
CGO_ENABLED=0 go run ./example/editor
CGO_ENABLED=0 go run ./example/declarative

# Choose an initial appearance, or read a local UTF-8 file:
CGO_ENABLED=0 go run ./example/editor -theme dark
CGO_ENABLED=0 go run ./example/editor path/to/file.json
```

On Windows PowerShell, set `$env:CGO_ENABLED='0'` and run the same `go run` commands.
Both examples request Integrated window chrome. Linux needs a compositor to enable
it; GOUI can fall back to native decoration when the required capability is unavailable.
See the [example guide](example/README.md) for controls and screenshot steps.

## Add an editor

The [declarative example](example/declarative/main.go) creates a stable `Document`
outside the build function, composes SourceView with GOUI's ScrollView, and places
the optional FindBar in an Overlay. A complete program:

```go
package main

import (
	"log"

	source "github.com/golang-gui/goui-sourceview"
	"github.com/golang-gui/goui-sourceview/language"
	sourceui "github.com/golang-gui/goui-sourceview/ui"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/theme/modern"
	ui "github.com/golang-gui/goui/ui"
)

func main() {
	document := source.NewDocument("{\n    \"editor\": \"GOUI SourceView\",\n    \"editable\": true\n}\n")
	_ = document.SetLanguage(language.JSON)
	if err := ui.Run("org.golang-gui.SourceViewDeclarative", func(app ui.App) ui.RootView {
		return ui.Root().
			StyleSheet(modern.FromSettings(app.Settings())).
			Windows(
				ui.Window("main").
					Title("Declarative SourceView").
					Chrome(ui.WindowChromeIntegrated).
					Size(900, 600).Center(true).
					Content(
						ui.VBox(
							ui.HeaderBar(ui.Label("Declarative SourceView")).Padding(8),
							ui.Overlay(
								ui.ScrollView(sourceui.SourceView().ID("editor").Document(document).AcceptsTab(true)),
								ui.OverlayItem(sourceui.FindBar("editor")).OnPosition(func(available, size geometry.Size, p *geometry.Point) {
									p.X = max(0, available.Width-size.Width-8)
								}),
							).MainWeight(1),
						),
					),
			)
	}); err != nil {
		log.Fatal(err)
	}
}
```

For imperative code, create `source.NewSourceView()`, call `SetDocument(document)`,
and use `gui.ScrollView.SetChild(editor)`. Compose a separate `source.NewFindBar()`
and call `SetTarget(editor)`. The [editor example](example/editor/main.go) shows the
complete lifecycle and error handling.

## Styles, state and shortcuts

Importing the root package registers Basic and Modern support. Enabling
`modern.FromSettings(app.Settings())` styles the editor, FindBar and completion UI
together. `SetScheme` overrides code colors; `nil` restores the current theme.
Selection backgrounds are translucent and preserve syntax colors by default.
See [style integration](doc/DesignStyles.md).

Document text uses normalized UTF-8 and LF; positions are **byte offsets**.
Share a Document to share text and history between views. Access documents and
mounted widgets sequentially on the GUI thread; post background results through
the application event loop. See [editing contracts](doc/DesignEditing.md).

| Operation | Shortcut |
| --- | --- |
| Find | Ctrl+F / macOS Cmd+F, with FindBar composed |
| Replace | Ctrl+H / macOS Cmd+Option+F, with FindBar composed |
| Next / previous match | F3 / Shift+F3 |
| Completion | Ctrl+Space |
| Move line | Alt+Up/Down / macOS Option+Up/Down |
| Undo / redo | Ctrl+Z / Ctrl+Shift+Z; macOS uses Cmd |

Tab indentation is opt-in through `AcceptsTab(true)`. Press Escape before the next
Tab to return to focus traversal. See the [full shortcut rules](doc/DesignEditing.md#13-常用编辑快捷键)
for platform differences. Menus and FindBar follow `mlang.SetLanguage`; the example's
own toolbar uses English labels.

## Verification and documentation

```sh
go test ./...
go test -race ./...
go vet ./...
```

Desktop scenarios are explicit programs under `tests/window/`; see their entry
comments before running. The current [medium/large-file verification](doc/PlanLargeFiles.md)
covers 10/50 MiB JSON, XML and HTML plus a 100 MiB JSON probe. These results describe
the recorded scenarios, rather than a general latency or memory guarantee.
`GOWORK=off go test ./...` remains blocked by the unreleased GOUI dependency APIs.

- [Architecture and scope](doc/DesignSourceView.md)
- [Completion](doc/DesignCompletion.md) · [Bracket editing](doc/DesignPairs.md) · [FindBar](doc/DesignFindBar.md)
- [Highlighting](doc/DesignHighlighting.md) · [Layout](doc/DesignLayout.md) · [Localization](doc/DesignLocalization.md)
- [Development plan](doc/Plan.md) · [Testing](doc/Testing.md) · [Implementation records](doc/Implementation.md)
