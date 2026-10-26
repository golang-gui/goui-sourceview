# SourceView examples / 示例

The examples use Modern and request
`WindowChromeIntegrated`; Linux needs a compositor for Integrated decoration.

示例均采用 Modern 与 Integrated 窗口，Linux 的 Integrated 需要桌面合成器。
示例工具栏为英文；编辑菜单及查找栏使用 GOUI 界面语言资源。

## Setup / 环境准备

Requires Go 1.24 or newer and GOUI's [desktop runtime dependencies](https://github.com/golang-gui/goui#platforms-and-dependencies).
The module depends on GOUI v0.4.0; Go downloads it automatically. A separate GOUI
checkout or local workspace is not required to build or run these examples.

需要 Go 1.24 或更新版本及 GOUI 的桌面运行依赖。模块直接依赖 GOUI v0.4.0，由 Go
自动下载；构建和运行示例无需另行检出 GOUI 或创建本地工作区。

## Run / 启动

```sh
go run ./example/editor
go run ./example/editor -theme light
go run ./example/editor -theme dark path/to/file.html
go run ./example/declarative
go run ./example/markdown
go run ./example/markdown -dark
```

The examples also build with `CGO_ENABLED=0`. In Bash, prefix a command with
`CGO_ENABLED=0`; in Windows PowerShell, set `$env:CGO_ENABLED='0'` first.
If Linux cannot enable Integrated decoration, GOUI may fall back to native decoration.

示例支持 `CGO_ENABLED=0` 构建。Bash 可在命令前加 `CGO_ENABLED=0`；
Windows PowerShell 先设置 `$env:CGO_ENABLED='0'`。
Linux 不满足 Integrated 所需能力时，GOUI 可回退到原生装饰。

Both editor examples explicitly bind F2 for completion; the control has no default manual-completion shortcut.
两个编辑示例显式绑定 F2 触发补全；控件本身不预设手动补全快捷键。

## Editor

`editor` embeds original JSON/XML/HTML/Markdown/TOML/YAML/Shell samples, so the binary can run from any
directory. Each sample has a separate Document. Switching preserves content and
undo history; selection and scroll position belong to the current view. An optional
UTF-8 file is read once and gets its own file selector. All edits stay in memory.
Restart resets the samples. Closing the main window exits. No DevServer is started.

`editor` 内嵌本项目原创 JSON/XML/HTML/Markdown/TOML/YAML/Shell 样本，可从任意目录运行构建后的程序。
每份样本拥有独立 Document，切换保留内容与撤销历史；选区和滚动属于当前视图。
可选参数读取一个 UTF-8 文件并增加对应选择按钮，修改仅保存在内存中。
重启恢复样本，关闭主窗口退出，不启动开发服务器。

- Click filenames to switch; edit one, switch away and back, then Undo.
  点击文件名切换，编辑后切换回来，确认内容和撤销历史保留。
- Toggle Word wrap, Line numbers and Read only; the footer shows language, LF/CRLF/CR,
  line, Unicode character column, line count, output byte count and editing mode. The displayed
  column counts Unicode code points, rather than bytes or visual tab columns.
  切换折行、行号及只读；底部显示语言、LF/CRLF/CR、行列、行数、输出字节数和模式。
  显示列按 Unicode 码点计数，不是模型字节偏移或 Tab 展开后的视觉列。
- Use Find / Replace or platform shortcuts; try `editor` in JSON or `feature`
  in HTML. Replace one/all and Undo. Escape closes the floating panel.
  打开查找替换，在 JSON 搜索 `editor`、HTML 搜索 `feature`，尝试替换及撤销，Escape 关闭浮层。
- Open View to choose Off, Selection, Trailing, Boundary or All whitespace markers,
  and toggle logical line endings independently. These options change display only.
  从 View 菜单选择空白显示模式，独立切换逻辑换行标记；这些选项只改变显示。
- Right-click for the editor menu. F2 opens completion; type brackets,
  quotes, or select text and type an opening bracket. Try line movement shortcuts.
  右键打开菜单；F2 请求补全，输入括号/引号或包裹选区，尝试按快捷键移动整行。
- The header button switches light/dark. `-theme system` follows system appearance
  until you choose a theme; settings connections end with the application.
  标题栏按钮切换亮暗；`-theme system` 在手动切换前跟随系统外观，订阅随应用结束释放。

Clipboard commands change the real clipboard only when requested. File loading
reads the explicitly supplied file; edits stay in memory. Check real IME and
system-reserved shortcuts in the user's desktop environment.

只有显式执行复制/剪切等命令才改变真实剪贴板。文件参数只读取指定文件，修改保存在内存中。
真实输入法和系统占用快捷键需在对应桌面环境验证。

## Declarative

`declarative` is a minimal `ui.Run` composition: stable Document, HeaderBar,
ScrollView and SourceView with built-in find, replace and go-to UI. Its stylesheet follows system settings.
Use Ctrl/Cmd+F to open the built-in find UI; editing uses the same widget as the imperative example.

`declarative` 展示最小声明式组合及稳定 Document，样式随系统设置重建。
Ctrl/Cmd+F 打开查找栏；编辑行为由同一个命令式控件提供。

Configuration types, document construction and key bindings use `sourceui` directly;
GOUI key constants use `base`. This example explicitly inserts real Tabs and shows
Boundary whitespace markers. Existing sample indentation remains spaces.

配置类型、文档构造和快捷键统一使用 `sourceui`，GOUI 按键常量使用 `base`，
无需导入命令式根包或 gui。此例显式配置真实 Tab 输入并显示 Boundary 空白标记；
初始样本中的缩进仍是普通空格。


See [editor shortcuts](../doc/DesignEditing.md#13-常用编辑快捷键) for the complete
platform-specific bindings, and [style integration](../doc/DesignStyles.md) for customization.

完整平台快捷键见[编辑规则](../doc/DesignEditing.md#13-常用编辑快捷键)，
外观定制见[样式接入](../doc/DesignStyles.md)。

## README screenshots / 截图复现

The published screenshots use the real Linux desktop window, Software at 1x with
Integrated enabled, cropped to the application boundary. No UI elements are drawn
into the image. Sample data comes from the embedded files in `editor/samples/`.

1. Run `GOUI_PLAT_PAINTER=software GOUI_PLAT_SCALE=1 go run ./example/editor -theme light`.
2. Capture the initial JSON document after highlighting settles.
3. Click `index.html`, switch to dark, open Find / Replace, and enter `feature`.
4. Capture the window with search highlights and the floating panel visible.
5. Close the example to reset; screenshots are documentation assets in `doc/images/`.

截图来自本机真实窗口，仅裁剪应用边界，不往图片绘制控件。
按上述步骤先捕获浅色 JSON，再切到 HTML、深色主题，打开查找替换并输入 `feature` 后截图。
需确认实际 Integrated 已启用；Linux 无合成器时的原生回退不能标注为 Integrated。
完成后关闭示例，图片作为文档资源保存在 `doc/images/`。

## Markdown customization / 自定义 Markdown

`markdown` is a small code editor built entirely with public APIs. Its adjacent
`markdown` package supplies an application-owned tokenizer, custom color keys, word
rules, backtick pairs, list continuation and a cancellable completion provider.
It demonstrates links with a custom range/caret, references with an additional
definition edit, and Bold using a single undo transaction. It is an illustrative
Markdown subset, without a preview or embedded Go highlighting.

`markdown` 示例完全使用公开接口。相邻子包提供应用自己的扫描器、样式键、词边界、
反引号配对、列表续行和可取消补全。输入 `[` 选择链接后光标进入 URL，输入 `@`
插入引用并在文末添加定义，F2 提供模板；Bold/Comment、查找和主题按钮可交互验证。
语法是演示子集，没有预览或围栏内 Go 高亮。所有修改只在内存，重启复位，关闭窗口退出。

## Folding and navigation / 折叠与导航

Use the gutter arrows to fold structured markup, headings or code fences.
Control+G opens line[:column] navigation; Escape returns focus to the editor.
SourceView keeps its go-to and find/replace interfaces mutually exclusive.

点击侧栏箭头折叠结构、标题或围栏；Control+G 打开行[:列]跳转，Escape 返回编辑器。
SourceView 自动让跳转栏与查找栏互斥。XML/HTML 的确认名称对支持输入/删除联动，撤销只需一次。

The find panel preserves multiline query/replacement text. Alt+Enter inserts
newlines; Enter/Shift+Enter navigates. Toggle .* for Go regular expressions,
use $1/${name} in replacements, or capture the current editor selection as scope.

查找栏支持多行查询及替换；Alt+Enter 输入换行，Enter/Shift+Enter 导航。
.* 启用 Go 正则，替换使用 $1/${name}；选区按钮捕获当前编辑器范围，结果导航不改变查询范围。
