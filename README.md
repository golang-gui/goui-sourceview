# GOUI SourceView

GOUI 的可选代码编辑器扩展，模块路径为 github.com/golang-gui/goui-sourceview。

当前已提供独立的 SourceView 控件、共享 Document、JSON / XML / HTML 增量高亮、
选择与复制、编辑与撤销、行号、缩进、字面量搜索替换、亮暗配色和薄声明式绑定。
支持按词拖选、编辑菜单、语言/文档词补全、括号自动闭合/包裹/成对删除和匹配装饰。
常用编辑快捷键与同仓可选 FindBar 已接入；查找栏的创建、放置由应用选择。
控件基于 WidgetBase 与公开的 textedit、typography 设施实现，滚动条由 GOUI ScrollView 提供。

当前是开发中的初始实现，尚未完成所有平台的原生验收与独立发布。
具体结果和限制见 [实现记录](doc/Implementation.md)，开发顺序见 [Plan](doc/Plan.md)。

## 本地运行

当前开发版本使用 GOUI 新增的通用批量编辑、Tab 排版和剪贴板结果接口。
go.mod 保留可获取的 v0.1.1 作为依赖基线；该发行版尚不具备这些新增接口，
当前需要 GOUI 提交 `2f782897684f1bfc892a11b0254e5a2162980ed8` 或之后包含这些接口的版本，
本地通过 go.work 联调。Windows 的反复重着色还需要 GOUI `227220a` 的全范围颜色覆盖回收修正，
详见 [着色缓存验收](doc/PlanColorCaching.md)。新增依赖尚未发布；没有提交机器专属 replace。
菜单键盘导航使用 GOUI `ffc59f3` 的实现；浮层还需要 `00521cc` 的局部阴影接口与
`210732a` 的显式子树样式作用域。样式注册还需要 GOUI `903daf8`。本轮联调 GOUI HEAD 为 `903daf8`。

在本仓库运行：

~~~bash
# 尚无 go.work 时：
go work init . ../goui

go test ./...
go run ./examples/editor
go run ./examples/declarative
~~~

[命令式示例](examples/editor/main.go) 可切换三种语言、只读、折行、行号、主题及查找替换。
也可以通过 go run ./examples/editor path/to/file.json 加载一个本地文件；示例只修改内存，不保存文件。
[声明式示例](examples/declarative/main.go) 展示稳定 Document 与 ui.ScrollView 的组合。
两个示例都用 Overlay 把可选 FindBar 浮在编辑区右上角，支持紧凑查找、展开替换和亮暗配色；Ctrl/Cmd+F 打开查找，Windows/Linux Ctrl+H 或 macOS Cmd+Option+F 展开替换。

## 样式

SourceView 根包已注册 Basic 和 Modern。默认使用 Basic；应用启用 Modern 即可同时应用到
编辑区、查找栏与补全，无需额外导入适配包：

```go
app.SetStyleSheet(modern.FromSettings(app.Settings()))
// 声明式 build：
return ui.Root().StyleSheet(modern.FromSettings(app.Settings())).Windows(/* windows */)
```

命令式示例启动时读取一次系统外观，按钮替换整张 Modern 样式表；声明式示例随设置重建。
`SetScheme` 仅覆盖代码配色，传 nil 恢复当前样式；查找栏和补全的界面仍跟随 Basic/Modern。
需要并列不同主题时，用 `gui.StyleScope` 显式包裹各编辑区及其查找浮层。
应用覆盖、所有命名 part 与失效规则见 [样式设计](doc/DesignStyles.md)，
验证范围见 [接入验收](doc/PlanStyles.md)。

## 接入

~~~go
document := sourceview.NewDocument("{\n    \"ready\": true\n}\n")
if err := document.SetLanguage(language.JSON); err != nil {
    return err
}

editor := sourceview.NewSourceView()
editor.SetDocument(document)
editor.SetAcceptsTab(true)

scroll := gui.NewScrollView()
scroll.SetChild(editor)
window.SetWidget(scroll)
~~~

Document 可由多个视图共享；光标、选区、滚动与原生排版属于各自视图。
Document 及已挂载控件在 GUI 线程顺序访问，后台加载结果通过应用的 Post 投递。
文档位置和选择端点使用规范化 UTF-8 文本的字节偏移，换行使用 LF。

AcceptsTab 默认关闭。打开后 Tab 插入缩进，非空选区的 Tab / Shift+Tab 对选中行缩进或反缩进；
Escape 后的下一次 Tab / Shift+Tab 交给 GOUI 焦点遍历。
Ctrl/Cmd+A/C/X/V/Z、Ctrl/Cmd+Shift+Z、方向键、Home/End、PageUp/PageDown 可直接使用。

双击选择词，三击选择逻辑整行，双击后拖动按词扩展。右键或 Shift+F10 打开编辑菜单。
Ctrl+Space 请求补全，上下选择、Enter/Tab 确认、Escape 取消，也可点击候选。
Alt/Option+上下移动逻辑行；加 Shift 复制行（Linux 还需加 Control）；Primary+Shift+K 删除整行。
右键菜单也提供这些命令；移动和复制保留选区方向，每次操作可一次撤销，详见 [行编辑规则](doc/DesignEditing.md#12-逻辑行编辑)。
自动补全和括号自动插入默认开启，可通过 SetAutomaticCompletion/SetAutoPairs 关闭；
SetCompletionProvider 可添加应用候选，ui 提供同名薄绑定配置。
补全列表支持独立亮暗配色、名称/说明分列、前缀强调、长文本省略和滚动位置提示；
界面通过 `sourceview.completion` 及命名 part 定制，具体契约见 [样式接入](doc/DesignStyles.md)。
配对提示使用淡底和细边框；主题的 BracketMatchBackground/BracketMatchBorder 可分别配色或设为透明关闭。
当前补全提供语言关键词/名称及附近文档词，不包含 Schema、LSP 或语义类型推断。

ReplaceSelection、Cut、Copy 等同步操作返回错误；Paste 返回请求是否接受，
延迟的读取/插入失败经 ConnectError 通知；取消输入法时，若回调改变了文档或选区，旧命令返回 ErrOperationChanged。复制为空选区、无文本格式以及过期粘贴请求属于正常情况。
搜索为 Unicode 字面量搜索，支持忽略大小写和整词；全部替换和多行缩进分别只占一次撤销。
Ctrl/Cmd+Enter 在下方插入行，加 Shift 在上方；Ctrl/Cmd+L 选择整行并可重复扩展；
Ctrl/Cmd+] / [ 整行缩进/反缩进。按词移动/删除使用 Windows/Linux 的 Ctrl 或 macOS 的 Option。
Ctrl/Cmd+Shift+Backslash 在已有相邻匹配括号之间跳转，F3 / Shift+F3 查找下一项/上一项。
具体边界和平台差异见 [快捷键设计](doc/DesignEditing.md#13-常用编辑快捷键)。

创建 `NewFindBar()` 并 `SetTarget(editor)` 组合可选查找浮层，`Open(false)` / `Open(true)` 打开查找/替换。
关闭保留查询和高亮，裸 SourceView 不拦截打开界面的快捷键。声明式使用
`sourceui.FindBar("editor")` 放入 `base.OverlayItem(...)`，配合 `sourceui.SourceView().ID("editor")`；未显式设置 Search 时，重建保留交互查询。
计数按帧推进，不保存全文件命中列表；导航/替换仍为显式同步操作。见 [查找栏设计](doc/DesignFindBar.md)。

## 验证与文档

~~~bash
go test ./...
go test -race ./...
go vet ./...

# 真实桌面，默认不改写剪贴板：
XMODIFIERS=@im=none GOUI_PLAT_PAINTER=software go run ./tests/window/editor -auto
XMODIFIERS=@im=none GOUI_PLAT_PAINTER=software go run ./tests/window/experience
~~~

Linux 有真实输入法时应保留其 XMODIFIERS，再按窗口程序注释手动验收组合输入。
-clipboard 仅用于可丢弃内容的独立剪贴板测试，会改写系统剪贴板；
X11 下内容可能随测试进程退出而消失。

- [总设计](doc/DesignSourceView.md)
- [编辑与输入](doc/DesignEditing.md)
- [可选查找替换栏](doc/DesignFindBar.md)
- [括号编辑与匹配](doc/DesignPairs.md)
- [补全](doc/DesignCompletion.md)
- [语言与增量高亮](doc/DesignHighlighting.md)
- [排版与呈现](doc/DesignLayout.md)
- [GOUI 接入与扩展](doc/DesignGOUIIntegration.md)
- [测试矩阵](doc/Testing.md)
- [实现和验收记录](doc/Implementation.md)
- [编辑体验验收](doc/PlanEditorExperience.md)
