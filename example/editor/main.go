// Run: go run ./example/editor [-theme system|light|dark] [file].
// Requires a desktop and native typography; Linux Integrated needs a compositor.
// Embedded JSON/XML/HTML/TOML/YAML/Shell documents are editable and retain changes when switched.
// An optional UTF-8 file is read once; edits stay in memory and are never saved.
// Use the toolbar, editor context menu and standard editing shortcuts. Restart
// to reset the samples; close the main window to exit. No development server.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"unicode/utf8"

	source "github.com/golang-gui/goui-sourceview"
	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/layout"
	"github.com/golang-gui/goui/theme/modern"
	"github.com/golang-gui/goui/widgets"
)

func main() {
	runtime.LockOSThread()
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	theme := flag.String("theme", "system", "appearance: system, light, dark")
	flag.Parse()
	if *theme != "system" && *theme != "light" && *theme != "dark" {
		return fmt.Errorf("unknown theme %q: use system, light or dark", *theme)
	}
	if flag.NArg() > 1 {
		return fmt.Errorf("expected at most one UTF-8 filename")
	}
	documents := sampleDocuments()
	if flag.NArg() == 1 {
		path := flag.Arg(0)
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !utf8.Valid(data) {
			return fmt.Errorf("%s is not UTF-8 text", path)
		}
		d := source.NewDocument(string(data))
		if err := d.SetLanguage(language.Guess(path, data)); err != nil {
			return err
		}
		documents = append([]exampleDocument{{name: filepath.Base(path), document: d}}, documents...)
	}
	app, err := gui.NewApplication("org.golanggui.SourceViewExample")
	if err != nil {
		return err
	}
	appearance := *theme
	themeLabel := gui.NewLabel("")
	applyTheme := func() {
		settings := app.Settings()
		dark := settings.ColorScheme() == gui.ColorSchemeDark
		if appearance != "system" {
			dark = appearance == "dark"
		}
		app.SetStyleSheet(modern.Sheet(modern.Options{Dark: dark, AccentColor: settings.AccentColor(), FontFamily: settings.FontFamily(), FontSize: settings.FontSize()}))
		if dark {
			themeLabel.SetText("Light theme")
		} else {
			themeLabel.SetText("Dark theme")
		}
	}
	applyTheme()
	settingsChanged := app.Settings().ConnectChanged(applyTheme)
	defer settingsChanged.Disconnect()
	win, err := app.NewWindow(&gui.WindowOptions{Size: geometry.Size{Width: 1120, Height: 780}, Chrome: gui.WindowChromeIntegrated, Center: true})
	if err != nil {
		return err
	}
	defer win.Destroy()
	if err := win.SetTitle("GOUI SourceView"); err != nil {
		return err
	}
	win.SetMinSize(geometry.Size{Width: 860, Height: 540})
	chrome := win.Chrome().ConnectInfo(func(info gui.ChromeInfo) {
		log.Printf("window chrome: mode=%d enabled=%t controls=%d", info.Mode, info.Enabled, info.Controls)
	})
	defer chrome.Disconnect()

	editor := source.NewSourceView()
	bindings := editor.KeyBindings()
	bindings[source.CommandComplete] = []gui.KeyGesture{{Key: gui.KeyF2}}
	if err := editor.SetKeyBindings(bindings); err != nil {
		return err
	}
	editor.SetID("editor")
	editor.SetAcceptsTab(true)
	scroll := gui.NewScrollView()
	scroll.SetChild(editor)
	viewport := gui.NewOverlay()
	viewport.SetChild(scroll)
	viewport.SetMainWeight(1)
	find := source.NewFindBar()
	find.SetTarget(editor)
	viewport.AddOverlay(find)
	viewport.ConnectOverlayPosition(find, func(available, size geometry.Size, p *geometry.Point) {
		p.X = max(0, available.Width-size.Width-8)
	})

	goTo := source.NewGoToBar()
	goTo.SetTarget(editor)
	viewport.AddOverlay(goTo)
	viewport.ConnectOverlayPosition(goTo, func(available, size geometry.Size, p *geometry.Point) { p.X = max(0, available.Width-size.Width-8) })
	find.ConnectOpen(func(open bool) {
		if open {
			goTo.Close()
		}
	})
	goTo.ConnectOpen(func(open bool) {
		if open {
			find.Close()
		}
	})

	current := documents[0]
	status := gui.NewLabel("")
	status.SetStyleName(modern.MutedText)
	undo, redo := button("Undo", func() { editor.Undo(); win.SetFocusedWidget(editor) }), button("Redo", func() { editor.Redo(); win.SetFocusedWidget(editor) })
	update := func() {
		m := current.document.Model()
		s := editor.Selection()
		row := m.LineAt(s.Caret)
		line, _ := m.LineRange(row)
		prefix, _ := m.Slice(gui.TextRange{Start: line.Start, End: s.Caret})
		mode := "Editable"
		if editor.ReadOnly() {
			mode = "Read only"
		}
		status.SetText(fmt.Sprintf("%s  ·  %s  ·  Ln %d, Col %d  ·  %d lines  ·  %d bytes  ·  %s", current.name, current.document.Language(), row+1, utf8.RuneCountInString(prefix)+1, m.LineCount(), m.Len(), mode))
		undo.SetEnabled(!editor.ReadOnly() && m.CanUndo())
		redo.SetEnabled(!editor.ReadOnly() && m.CanRedo())
	}
	show := func(d exampleDocument) {
		current = d
		editor.SetDocument(d.document)
		update()
		win.SetFocusedWidget(editor)
	}
	group := widgets.NewCheckGroup()
	files := gui.NewWrapBox(layout.DirectionHorizontal)
	files.SetSpacing(4)
	files.SetLineSpacing(4)
	files.SetPadding(8)
	files.SetCrossAlign(layout.CrossCenter)
	for i, d := range documents {
		choice := widgets.NewCheckButton()
		choice.SetGroup(group)
		choice.SetAppearance(widgets.CheckAppearanceButton)
		choice.SetChild(gui.NewLabel(d.name))
		choice.SetChecked(i == 0)
		choice.ConnectChange(func(state widgets.CheckState) {
			if state == widgets.CheckChecked {
				show(d)
			}
		})
		files.AddChild(choice)
	}
	files.AddChild(button("Find / Replace", func() {
		if err := find.Open(true); err != nil {
			status.SetText(err.Error())
		}
	}))
	tools := row(12, 8)
	tools.AddChild(undo)
	tools.AddChild(redo)
	for _, option := range []struct {
		label   string
		checked bool
		change  func(bool)
	}{
		{"Word wrap", false, func(b bool) {
			wrap := source.WrapNone
			if b {
				wrap = source.WrapWordChar
			}
			editor.SetWrapMode(wrap)
		}},
		{"Line numbers", true, editor.SetShowLineNumbers},
		{"Read only", false, func(b bool) { editor.SetReadOnly(b); update() }},
	} {
		choice := widgets.NewCheckButton()
		choice.SetChild(gui.NewLabel(option.label))
		choice.SetChecked(option.checked)
		choice.ConnectChange(func(state widgets.CheckState) { option.change(state == widgets.CheckChecked) })
		tools.AddChild(choice)
	}
	tools.AddChild(spacer())
	hint := gui.NewLabel("F2: complete  ·  F3: next match")
	hint.SetStyleName(modern.MutedText)
	tools.AddChild(hint)

	header := gui.NewHeaderBar()
	header.SetPadding(8)
	title := row(12, 0)
	title.AddChild(gui.NewLabel("GOUI SourceView"))
	subtitle := gui.NewLabel("A code editor for Go")
	subtitle.SetStyleName(modern.MutedText)
	title.AddChild(subtitle)
	title.AddChild(spacer())
	toggleTheme := gui.NewButton()
	toggleTheme.SetStyleName(modern.Flat)
	toggleTheme.SetChild(themeLabel)
	toggleTheme.ConnectClicked(func() {
		dark := appearance == "dark" || appearance == "system" && app.Settings().ColorScheme() == gui.ColorSchemeDark
		appearance = "dark"
		if dark {
			appearance = "light"
		}
		applyTheme()
	})
	title.AddChild(toggleTheme)
	header.SetChild(title)
	footer := row(0, 8)
	footer.AddChild(status)
	content := gui.NewLinearBox(layout.DirectionVertical)
	content.SetCrossAlign(layout.CrossStretch)
	for _, child := range []gui.Widget{header, files, tools, viewport, footer} {
		content.AddChild(child)
	}
	win.SetWidget(content)
	editor.ConnectSelection(func(gui.TextSelection) { update() })
	editor.ConnectChange(func(gui.TextChange) { update() })
	editor.ConnectError(func(err error) { status.SetText(err.Error()) })
	find.ConnectError(func(err error) { status.SetText(err.Error()) })
	show(current)
	if err := win.Show(); err != nil {
		return err
	}
	win.SetFocusedWidget(editor)
	app.Run()
	return nil
}

func button(text string, clicked func()) *gui.Button {
	b := gui.NewButton()
	b.SetChild(gui.NewLabel(text))
	b.ConnectClicked(clicked)
	return b
}

func row(spacing, padding float32) *gui.LinearBox {
	b := gui.NewLinearBox(layout.DirectionHorizontal)
	b.SetSpacing(spacing)
	b.SetPadding(padding)
	b.SetCrossAlign(layout.CrossCenter)
	return b
}

func spacer() *gui.Label {
	w := gui.NewLabel("")
	w.SetMainWeight(1)
	return w
}
