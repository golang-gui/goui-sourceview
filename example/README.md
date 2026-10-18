# SourceView examples / 示例

Both examples use Modern and request
`WindowChromeIntegrated`; Linux needs a compositor for Integrated decoration.

两个示例均采用 Modern 与 Integrated 窗口，Linux 的 Integrated 需要桌面合成器。
示例工具栏为英文；编辑菜单及查找栏使用 GOUI 界面语言资源。

## Setup / 环境准备

Requires Go 1.24 or newer and GOUI's [desktop runtime dependencies](https://github.com/golang-gui/goui#platforms-and-dependencies).
For this development snapshot, place a GOUI checkout containing `903daf8` or a later
version with the same APIs at `../goui`. The declared `goui v0.1.1` does not include
the required batch editing, styling and registration APIs yet. From this repository
root, create a local workspace if one does not already exist:

需要 Go 1.24 或更新版本及 GOUI 的桌面运行依赖。当前开发快照需将包含 `903daf8`
或之后相同接口的 GOUI 检出放在相邻 `../goui`；`go.mod` 中的 `goui v0.1.1`
尚不具备所需批量编辑、样式和注册接口。在本仓库根目录为首次使用创建本地工作区：

```sh
go work init . ../goui
```

For an existing workspace, use `go work use . ../goui` instead. Keep the workspace
local; a machine-specific `replace` is unnecessary.

已有工作区时改用 `go work use . ../goui`。工作区仅供本地使用，无需机器专属 `replace`。

## Run / 启动

```sh
go run ./example/editor
go run ./example/editor -theme light
go run ./example/editor -theme dark path/to/file.html
go run ./example/declarative
```

The examples also build with `CGO_ENABLED=0`. In Bash, prefix a command with
`CGO_ENABLED=0`; in Windows PowerShell, set `$env:CGO_ENABLED='0'` first.
If Linux cannot enable Integrated decoration, GOUI may fall back to native decoration.

示例支持 `CGO_ENABLED=0` 构建。Bash 可在命令前加 `CGO_ENABLED=0`；
Windows PowerShell 先设置 `$env:CGO_ENABLED='0'`。
Linux 不满足 Integrated 所需能力时，GOUI 可回退到原生装饰。

## Editor

`editor` embeds original JSON/XML/HTML samples, so the binary can run from any
directory. Each sample has a separate Document. Switching preserves content and
undo history; selection and scroll position belong to the current view. An optional
UTF-8 file is read once and gets its own file selector. All edits stay in memory.
Restart resets the samples. Closing the main window exits. No DevServer is started.

`editor` 内嵌本项目原创 JSON/XML/HTML 样本，可从任意目录运行构建后的程序。
每份样本拥有独立 Document，切换保留内容与撤销历史；选区和滚动属于当前视图。
可选参数读取一个 UTF-8 文件并增加对应选择按钮，修改仅保存在内存中。
重启恢复样本，关闭主窗口退出，不启动开发服务器。

- Click filenames to switch; edit one, switch away and back, then Undo.
  点击文件名切换，编辑后切换回来，确认内容和撤销历史保留。
- Toggle Word wrap, Line numbers and Read only; the footer shows language, line,
  Unicode character column, line count, byte count and editing mode. The displayed
  column counts Unicode code points, rather than bytes or visual tab columns.
  切换折行、行号及只读；底部显示语言、行列、行数、字节数和模式。
  显示列按 Unicode 码点计数，不是模型字节偏移或 Tab 展开后的视觉列。
- Use Find / Replace or platform shortcuts; try `editor` in JSON or `feature`
  in HTML. Replace one/all and Undo. Escape closes the floating panel.
  打开查找替换，在 JSON 搜索 `editor`、HTML 搜索 `feature`，尝试替换及撤销，Escape 关闭浮层。
- Right-click for the editor menu. Ctrl+Space opens completion; type brackets,
  quotes, or select text and type an opening bracket. Try line movement shortcuts.
  右键打开菜单；Ctrl+Space 请求补全，输入括号/引号或包裹选区，尝试按快捷键移动整行。
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
ScrollView, SourceView and optional FindBar. Its stylesheet follows system settings.
Use Ctrl/Cmd+F to open FindBar; editing uses the same widget as the imperative example.

`declarative` 展示最小声明式组合及稳定 Document，样式随系统设置重建。
Ctrl/Cmd+F 打开查找栏；编辑行为由同一个命令式控件提供。

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
