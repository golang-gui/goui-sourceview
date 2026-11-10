# 内联建议（影子文字）

状态：第一版已实现，依赖已发布 GOUI v0.7.0；验收见 [内联建议验收](PlanInlineSuggestion.md)。

## 定位与范围

SourceView 提供可选的视图内建议显示与接受能力。宿主负责模型、网络、上下文、请求取消和同一上下文内的请求代次，
组件负责文档/修订/光标核对、预览、Tab 接受、Esc 清除和生命周期。没有内置 AI 客户端、模型依赖或语义解析。

第一版默认关闭，每个视图同时保存一个逻辑行末尾的追加型单行字面量，最多 4096 个 UTF-8 字节。
空/全空白、非法 UTF-8、控制字符（含 Tab/换行）及 Unicode 行分隔符为无效输入；首次不做行中替换、多行建议、
流式增量、候选轮播或跨位置改写。正文发生任何修订后清除旧建议，由宿主重新发布；不猜测用户输入是否可复用旧结果。

预览使用光标当前显示行的剩余空间；过长时在原生字形 cluster 边界截断，并画省略提示，接受时仍插入完整建议。
预览不额外换行、不改变正文内容高度、行号、滚动范围或模型几何。多行投影与重排留待单独设计。
逻辑行末尾若处于 RTL 行的视觉内部，第一版隐藏该预览，避免独立文字覆盖真实正文；没有将其宣称为完整 bidi 注入支持。

## API 与线程

- `InlineSuggestionsEnabled()` / `SetInlineSuggestionsEnabled(bool)`：默认 false；关闭时清除现有建议。
- `InlineSuggestion{Position int, Text string}`：真实模型的 UTF-8 字节位置与待追加字面量，值对象。
- `SetInlineSuggestion(document *Document, revision uint64, suggestion InlineSuggestion) (bool, error)`：在 GUI 所属序列发布结果。
  文档身份/修订不匹配、未挂载/未聚焦、只读/禁用、功能关闭、选区非空、位置不在当前光标及逻辑行末尾，返回 false, nil。
  输入格式或有效文档中的位置不合法时返回错误，原建议不被错误请求替换。成功保存返回 true，实际可见性另外报告。
- `InlineSuggestion() (InlineSuggestion, bool)`：返回当前有效请求的独立值；位置在视口外或无足够预览空间时也可返回 true。
- `ClearInlineSuggestion()`：清除当前视图的请求与原生资源，不修改文档。
- `AcceptInlineSuggestion() error`：仅接受实际可见且仍有效的建议，没有可见建议为正常无操作。

宿主先在 GUI 序列读取需要的不可变上下文；后台请求只访问副本。结果经应用的 Post 回到 GUI 序列后再调用 setter，例如：

```go
view.SetInlineSuggestionsEnabled(true)
// 在 GUI 序列取得 doc、revision、position；宿主异步取得单行 suffix 后：
app.Post(func() {
    installed, err := view.SetInlineSuggestion(doc, revision,
        sourceview.InlineSuggestion{Position: position, Text: suffix})
    // installed == false 表示结果已不适用；按应用语义处理 err。
    _, _ = installed, err
})
```

组件保存 operationState，后续每次查询/绘制/接受核对文档、修订、挂载代次、选区代次与语言代次。
同文档/修订/位置上多个宿主请求的先后顺序仍由宿主负责，不把 setter 当作异步调度器。

## 输入、撤销与语义

普通补全请求开始就清除影子建议；候选列表或其请求活动期间不接受新影子建议。输入法开始前清除，保留真实预编辑逻辑。
Tab 的顺序是：普通候选列表、可见影子建议、原有缩进/焦点遍历。无修饰 Esc 在可见建议上只负责清除，不开启普通 Esc 的 Tab 焦点退出模式。
没有影子建议或其被隐藏时，键盘行为保持原有规则。鼠标点选、显式选区/导航、编辑、重置、换文档、失焦、禁用、只读和卸载清除旧状态。
滚动和调整宽度可以保留有效请求；预览只在真实行末光标完整可见且右侧有空间时显示，隐藏建议不得截获 Tab 或由接受方法写入。

接受时先清除预览，再校验操作状态，复用 `ApplyEdits` 一次提交精确字面量和末尾选区；不走自动括号/标签推测，单次撤销恢复接受前文本。
展示期间不进入 Model，复制、保存、搜索、文档修订、撤销历史和共享文档的其他视图均不包含该建议。

快照的 TextEditing / VisibleText / Preedit 保持原语义。`goui.sourceview` 附加 `inlineSuggestionsEnabled` 与当前 `inlineSuggestion`：
position、text、visible、bounds、truncated。它是键盘接受的文本预览，不伪造可点击按钮或 committed 文本，也不混入输入法预编辑。
`ui` 仅提供类型别名和 `InlineSuggestionsEnabled(bool)` 配置，省略恢复默认关闭；请求仍由宿主拥有。

## 原生排版、资源与外观

每个请求至多拥有一个当前字体/主题下的原生 TextLayout 和其 Geometry；完整段落原生排版仍由正文拥有。
setter、布局/帧准备与样式通知负责创建或替换建议排版，Paint、命中与 Snapshot 不创建原生布局，不扫描长行。
宽度/滚动变化复用单行建议排版，按缓存正文几何重算起点；分页正文只读已缓存的行末编辑窗口。
基线按两份原生行指标对齐，裁剪限制为当前显示行和正文右边界，截断对齐实际 cluster/可视停止点。
原生创建/度量后的回调重入必须再次核对请求身份；失败清理请求，由直接 setter 返回或后续准备阶段按 ConnectError 报告一次。
失效、替换、失焦、只读、禁用和卸载销毁所拥有的排版，资源不跨视图共享。

Basic / Modern 通过 `source-view` 的 `inline-suggestion` part 提供 GutterForeground 淡色文字；显式 Scheme 使用该方案的对应颜色。
可覆盖该 part，不引入中心主题切换或特殊 AI 样式状态。

## GOUI 能力缺口与绕过

GOUI v0.7.0 有 `textedit.Projection` 的范围投影基础，但当前 SourceView 的该状态服务于输入法，并发布 Preedit 语义；
公开接口未提供对应 Monaco 注入文字与 ViewZone 的成套能力。不能直接占用输入法状态实现 AI 预览。
本版在逻辑行末使用独立、有限的原生文字布局绕过，不修改 GOUI、不复制其私有实现。需要行中或多行重排时，再验证独立投影的适用性；
当前不借估算字符宽度或临时改写 Model 模拟完整功能。

## 参考与取舍

2026-11-10 核对：

- [VS Code InlineCompletionItemProvider](https://code.visualstudio.com/api/references/vscode-api#InlineCompletionItemProvider) 与
  [Monaco InlineCompletionsProvider](https://microsoft.github.io/monaco-editor/typedoc/interfaces/editor_editor_api.languages.InlineCompletionsProvider.html)：
  参考建议来源与显示/接受分离，第一版保留更小的宿主发布接口，没有照搬其完整 provider 生命周期和编辑范围。
- [VS Code 1.105.0 GhostTextView](https://github.com/microsoft/vscode/blob/1.105.0/src/vs/editor/contrib/inlineCompletions/browser/view/ghostText/ghostTextView.ts)：
  源码采用注入文字与额外行显示区域。确认多行不只是一次 Overlay 绘制；本版明确限制为单显示行，不声称具备同等重排能力。
- [GtkSourceView 5.23 CompletionProvider](https://gnome.pages.gitlab.gnome.org/gtksourceview/gtksourceview5/iface.CompletionProvider.html)：
  参考异步来源与激活分工，现有 SourceView 列表机制继续保留；未将列表 cell API 视作内联文字接口。
- [Scintilla EOL Annotations](https://www.scintilla.org/ScintillaDoc.html#EndOfLineAnnotations)：只读行末注释和建议预览用途不同，
  没有把注释直接映射为正文或自动接受的补全。

上述是公开契约与固定版本源码核对；本仓实际通过的环境与限制见验收记录。
