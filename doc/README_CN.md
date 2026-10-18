<h1 align="center">GOUI SourceView</h1>

<p align="center">面向 Go 桌面应用的原生代码编辑器</p>

<p align="center"><a href="../README.md">English</a></p>

GOUI SourceView 是 [GOUI](https://github.com/golang-gui/goui) 的可选代码编辑器扩展，
提供源码编辑、JSON / XML / HTML 增量高亮、补全、括号匹配和浮动查找替换面板，
使用原生文字排版及 Modern 亮暗外观。可以使用命令式控件，也可以使用薄声明式绑定。

项目仍在开发，公共 API 尚在演进。当前源码依赖 GOUI 尚未发布的通用接口，
运行方式见[运行示例](#运行示例)。

## 体验示例

[编辑器示例](../example/editor) 提供三份可编辑文档，可使用撤销/重做、折行、行号、
只读模式和亮暗外观。切换文档时保留各自的内容与撤销历史；修改仅保存在内存中。

Modern 浅色 — JSON

<img src="images/editor-light.png" alt="GOUI SourceView 在 Modern 浅色 Integrated 窗口中编辑 JSON">

Modern 深色 — HTML 与浮动查找替换面板

<img src="images/editor-dark.png" alt="GOUI SourceView 在 Modern 深色 Integrated 窗口中编辑 HTML 并使用查找替换">

以上为本机真实截图：Linux / X11、开启合成的 Xfwm4、Software 绘制、1x 缩放，
使用 `WindowChromeIntegrated`。不同桌面的原生文字及窗口控制按钮可能有所不同。

## 当前能力

- **源码编辑：** Unicode 选区、按词/整行选择与拖选、剪贴板、撤销/重做、缩进，
  以及行移动/复制/删除和常用编辑快捷键。
- **语言支持：** JSON、XML、HTML 增量高亮及跨行词法状态；语言名称和附近文档词提供补全候选。
- **代码导航：** 括号/引号自动配对、选区包裹、匹配装饰与跳转、行号和可选折行。
- **搜索替换：** Unicode 字面量搜索、大小写/整词选项、前后查找、当前/全部替换，
  以及可选的浮动 FindBar；全部替换可一次撤销。
- **集成：** Basic / Modern 样式、保留语法色的选区、八语言编辑菜单和 FindBar、共享文档及声明式绑定。

控件通过 GOUI 公开的 `gui/textedit` 和 typography 设施独立实现。
Document 管理文本、历史与语言分析；每个 SourceView 管理自己的选区、视口与原生排版。
GOUI 不反向依赖本模块。

## 运行示例

需要 **Go 1.24 或更新版本**及 GOUI 的[桌面运行依赖](https://github.com/golang-gui/goui#platforms-and-dependencies)。
当前开发快照需将包含 `903daf8` 或之后相同接口的 GOUI 检出放在相邻 `../goui`。
`go.mod` 声明的 `goui v0.1.1` 尚不具备所需批量编辑、样式和注册接口。

在本仓库根目录执行；尚未创建本地工作区时：

```sh
go work init . ../goui
```

已有工作区时改用 `go work use . ../goui`。工作区仅供本地使用，无需机器专属 `replace`。

```sh
CGO_ENABLED=0 go run ./example/editor
CGO_ENABLED=0 go run ./example/declarative

# 指定初始外观，或读取本地 UTF-8 文件：
CGO_ENABLED=0 go run ./example/editor -theme dark
CGO_ENABLED=0 go run ./example/editor path/to/file.json
```

Windows PowerShell 先设置 `$env:CGO_ENABLED='0'`，再运行相同的 `go run` 命令。
两个示例均请求 Integrated 窗口装饰；Linux 需要合成器，能力不满足时 GOUI 可回退到原生装饰。
操作及截图复现步骤见[示例指南](../example/README.md)。

## 接入编辑器

[声明式示例](../example/declarative/main.go) 在 build 函数外创建稳定 Document，
将 SourceView 放入 GOUI ScrollView，再用 Overlay 组合可选 FindBar。完整程序如下：

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

命令式接入使用 `source.NewSourceView()`、`SetDocument(document)` 和
`gui.ScrollView.SetChild(editor)`。可选查找栏另行创建并调用 `SetTarget(editor)`；
[命令式示例](../example/editor/main.go) 展示完整组合、生命周期和错误处理。

## 样式、状态与快捷键

导入根包即登记 Basic 和 Modern 支持。应用启用 `modern.FromSettings(app.Settings())`，
编辑区、查找栏和补全列表会一起适配。`SetScheme` 仅覆盖代码配色，传 `nil` 恢复当前主题。
选区默认使用真实半透明背景并保留语法前景，详见[样式接入](DesignStyles.md)。

文档采用规范化 UTF-8 与 LF，位置使用**字节偏移**。多个视图可共享同一 Document 的内容与历史。
文档及已挂载控件在 GUI 线程顺序访问，后台结果通过应用事件循环投递，详见[编辑契约](DesignEditing.md)。

| 操作 | 快捷键 |
| --- | --- |
| 查找 | Ctrl+F / macOS Cmd+F，需组合 FindBar |
| 替换 | Ctrl+H / macOS Cmd+Option+F，需组合 FindBar |
| 下一项 / 上一项 | F3 / Shift+F3 |
| 补全 | Ctrl+Space |
| 移动行 | Alt+上/下 / macOS Option+上/下 |
| 撤销 / 重做 | Ctrl+Z / Ctrl+Shift+Z；macOS 使用 Cmd |

`AcceptsTab(true)` 显式开启 Tab 缩进；按 Escape 后的下一次 Tab 恢复焦点遍历。
其他命令与平台差异见[完整快捷键规则](DesignEditing.md#13-常用编辑快捷键)。
编辑菜单和查找栏跟随 `mlang.SetLanguage`；示例自身工具栏使用英文标签。

## 验证与文档

```sh
go test ./...
go test -race ./...
go vet ./...
```

真实桌面场景放在 `tests/window/` 的独立程序中，执行前阅读入口注释。
当前[中大文件验证](PlanLargeFiles.md) 覆盖 10/50 MiB JSON、XML、HTML 及 100 MiB JSON 探测，
只代表记录中的功能场景，不构成通用延迟或内存上限承诺。
`GOWORK=off go test ./...` 仍受 GOUI 依赖接口尚未发布的条件阻塞。

- [定位与架构](DesignSourceView.md)
- [补全](DesignCompletion.md) · [括号编辑](DesignPairs.md) · [查找栏](DesignFindBar.md)
- [增量高亮](DesignHighlighting.md) · [排版与滚动](DesignLayout.md) · [多语言](DesignLocalization.md)
- [开发计划](Plan.md) · [测试矩阵](Testing.md) · [实现记录](Implementation.md)
