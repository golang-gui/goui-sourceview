# 应用接入

以下源码路径相对于应用实际选中的 SourceView 模块目录，不是相对于安装后的技能。
在线 main 和本技能的示例可能领先应用锁定的版本，使用前先查 go doc 和实际源码。

## 选择入口

| 需求 | 公开入口与同版本示例 |
| --- | --- |
| 命令式控件、属性和信号 | 根包 SourceView、Document；example/editor |
| 声明式接入 | ui.SourceView、ui.FindBar、ui.GoToBar；example/declarative |
| 文档修改及选区事务 | Document.Model、SourceView.ApplyEdits；commands.go |
| 查找、替换和浮层 | SourceView.SetSearch、FindNext、ReplaceCurrent、ReplaceAll；FindBar |
| 代码配色 | scheme、SourceView.SetScheme，以及 styles.go 的公开样式部件 |
| 应用定义语言和补全 | language、CompletionProvider；example/markdown |

普通界面由 GOUI gui / ui 提供。SourceView 只为需要源码编辑能力的区域增加依赖。

## 声明式最小接入

这是一个完整 main.go。文档在 build 外创建，FindBar 通过同一 Root 内的稳定控件 ID 绑定。
若项目已经使用命令式结构，沿用原结构，参考下一节即可。

```go
package main

import (
    "log"

    source "github.com/golang-gui/goui-sourceview"
    "github.com/golang-gui/goui-sourceview/language"
    sourceui "github.com/golang-gui/goui-sourceview/ui"
    "github.com/golang-gui/goui/core/geometry"
    "github.com/golang-gui/goui/layout"
    "github.com/golang-gui/goui/theme/modern"
    base "github.com/golang-gui/goui/ui"
)

func main() {
    document := source.NewDocument("{\n  \"name\": \"example\"\n}\n")
    if err := document.SetLanguage(language.JSON); err != nil {
        log.Fatal(err)
    }
    err := base.Run("com.example.SourceEditor", func(app base.App) base.RootView {
        return base.Root().StyleSheet(modern.FromSettings(app.Settings())).Windows(
            base.Window("main").Title("Source editor").
                Chrome(base.WindowChromeIntegrated).Size(900, 600).
                Content(base.VBox(
                    base.HeaderBar(base.Label("Source editor")).Padding(8),
                    base.Overlay(
                        base.ScrollView(sourceui.SourceView().ID("editor").
                            Document(document).AcceptsTab(true)).Style("source-view"),
                        base.OverlayItem(sourceui.FindBar("editor")).
                            OnPosition(func(available, size geometry.Size, p *geometry.Point) {
                                p.X = max(0, available.Width-size.Width-8)
                            }),
                    ).MainWeight(1),
                ).CrossAlign(layout.CrossStretch)),
        )
    })
    if err != nil {
        log.Fatal(err)
    }
}
```

Integrated 采用 GOUI 的平台支持与降级规则。自定义窗口装饰不是接入编辑器的前置条件，
已有应用可以保留自己的窗口模式。接受 Tab 插入由 AcceptsTab 显式选择。

## 命令式组合

创建 `source.NewSourceView()` 并 SetDocument；将它放入 `gui.NewScrollView()`，
宿主显式 `SetStyleName("source-view")`，让正文与滚动条位于同一外框内。
需要查找栏时，创建 `source.NewFindBar()`、SetTarget(editor)，将滚动区域作为 Overlay 的
普通内容，查找栏通过 AddOverlay 放在其上。按需设置 ConnectOverlayPosition。
两者挂载到同一 Root 后才能调用 `bar.Open(false)` / `bar.Open(true)` 并处理返回错误。

没有 FindBar 的应用仍能使用编辑器的搜索与替换 API；不要给每个按键重复创建查找栏。
手工 ConnectChange / ConnectSelection / ConnectError 等长期连接由所属应用状态释放；
声明式 OnChange / OnSelection / OnError 的连接由协调器管理。

## 查询、跳转和折叠

- SearchQuery.Regex 启用 Go regexp；SearchError() 保留非法表达式的反馈。捕获组替换使用
  $1/${name}/$$，字面模式的替换保持字面值，不套用 JavaScript/PCRE 语义。
- SetSearchRange(&scope) 复制独立查询范围，nil 清除；不能每次根据 FindNext 的结果重新设置。
  换文档清除范围。FindBar 的查询/替换保留 LF，Alt+Enter 换行，Enter 仍执行查找/替换。
- 可选 NewGoToBar/SetTarget 与 FindBar 一样放在同 Root 的 Overlay，通过 Control+G 打开，
  输入 1 起始 line[:column]；这里的列是 Unicode 码点，最终吸附到合法停靠点。两种浮层同时
  存在时由应用显式安排显示关系，参考 example/editor，不增加中心协调器。
- 折叠状态属于视图，不写入文档/历史。应用用 Fold/Unfold/FoldAll/UnfoldAll 或对应声明式
  开关/信号；语言的 FoldingFunc 提供 0 起始、保留头行的范围，处理取消和不可变快照。

## 文档、文件与编辑

- 一个打开的文件通常对应一份持久 Document。共享 Document 的视图共享文本、历史及语言，
  其选区和滚动独立。SetDocument 切换会重置当前视图的选择等状态；保留各文件的光标/滚动
  需要应用自己的标签状态，不能仅凭共享文档推断会自动恢复。
- NewDocument / Model.SetText 会归一化 UTF-8 和 LF。文件编码、换行形式、路径、脏标记、
  文件冲突和保存失败由应用处理。Model.SetText 是重载，会清历史，不应在普通编辑回调中使用。
- 内置语言通过 SetLanguage 设置；文件识别可使用 language.Guess(filename, sample)，
  再显式绑定结果。不要把文件扩展名或任意字符串直接当作已注册的语言。
- 应用格式化、模板和批量替换可用 `view.ApplyEdits(document, revision, edits, afterSelection)`。
  edits 使用同一原始文档坐标，afterSelection 使用应用结果的坐标；它们是字节位置。
  该入口校验文档身份和修订，并把修改及前后选区作为一个撤销事务。
  后台结果通过 GUI 投递时仍需核对身份/修订；过期结果应丢弃或重新计算。
- ReadOnly 限制视图的用户编辑；应用直接持有模型时仍能同步内容。不要把只读视图当作
  文档不可修改的访问控制。同步检查所调用 API 的错误、预算及生命周期约定。

## 主题与覆盖

应用通常设置 `modern.FromSettings(app.Settings())` 或明确的 Modern Sheet；直接导入
SourceView 已登记其 Basic / Modern 规则，不需要反复 Register。局部亮暗界面使用
GOUI 的显式 StyleScope。自定义 StyleName 时同时提供相应规则。

代码区域的 Scheme 可由应用显式设置，nil 恢复控件所在样式表；FindBar 界面不由 Scheme 控制。
Scheme.Definition.Tokens 是自定义 token 前景的另一入口。SelectionForeground 的 alpha=0
保留语法色，alpha=255 使用统一选中前景；其他 alpha 不合法。

需要定制某个部件时查看所选版本的公开规则，如 source-view 的 token-*、gutter、caret，
以及 sourceview.find.input 的 placeholder。不要把整份代码配色套给查找输入框或重新绘制选区。
