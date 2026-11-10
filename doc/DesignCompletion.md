# 补全设计

本专题主体描述 `completion.go`、`completion_list.go` 与 `ui` 的当前实现。
末尾的 P1 补全呈现为尚未实现的提案，不属于当前 API 契约。
会话及提供器队列集中在 `completion.go`，列表、面板及 ScrollView 组合集中在 `completion_list.go`。
补全是 SourceView 的语言和交互能力；GOUI 提供普通 Popover、事件、排版和绘制，
不反向依赖本模块，也不在 TextView 中添加源码特例。

## 候选与公共接口

默认开启 AutomaticCompletion。普通非组合文本输入后按前缀请求候选；XML/HTML
输入 `<` 后也可显示标签候选。手动触发默认没有按键；应用通过 KeyBindings 绑定
CommandComplete，或调用 ShowCompletion。示例显式选择 F2，见 [命令快捷键](DesignKeyBindings.md)。
只读、禁用、未挂载和 IME 预编辑期间不开始补全。

内建候选包括 JSON 的 true/false/null、HTML 常见标签和属性、XML 命名空间属性，
以及 caret 附近的文档词。标签名与属性上下文区分；JSON 字符串里不主动插入值关键字。
XML/HTML 名称允许连字符、冒号和点，普通标识符按 Unicode 字母、数字、Mark 与下划线取词。
这是一层词法补全，不承诺 JSON Schema、HTML DOM、XML XSD 或语义类型推断。

应用通过 `SetCompletionProvider(*CompletionProvider)` **替换**候选来源。nil 使用默认，
非 nil 不暗中追加内建建议。配置对象不可变，声明式重建复用同一指针：

```go
provider := &sourceview.CompletionProvider{
    TriggerCharacters: "[.@",
    Complete: func(ctx context.Context, request sourceview.CompletionContext) ([]sourceview.Completion, error) {
        // Read copied strings / a thread-safe project index; honor ctx.Done().
        return myCandidates(ctx, request)
    },
}
view.SetCompletionProvider(provider)
```

- Complete 在后台执行。请求没有活 Document、Model、Widget，只带 DocumentID、修订、
  不可变 Definition、语言、完整词 Range、caret、Prefix、Explicit、Trigger、词法上下文及
  有界 Before/After/NearbyText。偏移都是 UTF-8 byte；不能假定 NearbyText 是全文。
- 自定义提供者决定过滤、排序、去重和是否返回空集。控件仅校验展示数据、复制可变切片、
  限制数量；不再进行隐含前缀过滤/字母排序。可显式调用 LanguageCompletions、
  DocumentCompletions 或 DefaultCompletions 来组合静态语言建议和附近词。
  默认函数保留原大小写无关前缀、去重和按名称排序。
  Definition.AllowWordSuggestions 配置默认文档词的上下文策略，不从 ID 猜测应用语言行为。
- TriggerCharacters 是允许空前缀请求的字符集合；默认取 Definition.CompletionTriggers，
  提供者配置可覆盖。不以 ID 推断触发字符。显式补全请求不要求前缀。
  普通输入前缀由语言 Word 规则决定。输入字符经正常 CommitText/IME 路径，
  组合 IME 文本、粘贴和 ReplaceSelection 不额外触发。
- Completion 提供 Label/Text/Detail。空 Text 用 Label；Range 为 nil 时替换请求完整词，
  非 nil 使用项自己的范围。Caret 为 nil 时在插入末尾，否则是规范化 Text 内的 byte 位置。
  AdditionalEdits 与主编辑使用同一旧修订坐标，不得互相重叠，不能与主编辑共用起点。
  确认时排序、校验，按前置附加编辑映射 caret，再复用 ApplyEdits 一次提交一个撤销项。
- ShowCompletion 返回请求建立或同步弹窗失败；异步提供者/弹窗失败经 ConnectError 报告一次。
  无候选、context.Canceled 和正常过期是无操作。坏范围/光标在确认时返回错误，不修改正文。

每视图最多一个运行中的后台调用与一个最新待处理请求。新输入取消旧 context，替换待处理项；
即便提供者忽略取消也不无限创建 goroutine，但会推迟该视图后续自定义请求，故提供者必须配合取消。
请求完成经 Application.Post 投递，后台不访问 GUI；投递后检查文档身份、修订、选区、挂载、
语言定义代次、请求代次、焦点和可编辑性。取消清除 operationState 引用；卸载还断开队列的
视图引用。重新挂载仍复用同一队列，避免不合作任务导致并行数增长。

## 弹窗与输入

使用不获取原生焦点的普通非模态 Popover；键盘和 IME 保留在编辑器，继续经过
Window.DispatchEvent 和正常 IMClient。选择列表不是菜单：打开后仍需要正常键入前缀。

- 上下键循环切换并请求选中项滚入视口，Enter/Tab 确认当前候选；鼠标移动选择、点击确认。
  滚轮和滚动条仅浏览视口，不改变选中项或提交文本；键盘随后从原选中项继续导航。
- Escape 只关闭本次补全，不改正文；下一次正常 Tab 的规则不因此变更。
- Backspace 更新候选。普通左右键保留正文的原生视觉／Cluster 导航；移动后仍在同一
  完整词范围内、没有选区且有效语言定义不变时，更新前缀与候选，复用原 Popover。
  词首空前缀可以继续显示本次候选；移出词范围、Shift 扩选、其他导航、指针定位、
  普通 SetSelection 和外部修改关闭旧会话，不因本轮改动让任意选区变更自动触发补全。
- 默认候选按新上下文重新计算，左移缩短前缀时恢复此前过滤掉的词；不能仅过滤旧的
  已展示数组。自定义 Provider 按新 Caret/Prefix/Range 请求，保留原请求的 Explicit，
  Trigger 为空，因为没有新输入触发字符；不追加内建候选或代替 Provider 过滤、排序。
- 每次移动仅显式保留当前会话，更新选区身份后立即把旧候选标为 pending。
  异步刷新期间保留旧列表预览，禁止 Enter/Tab、点击及 hover 导航旧候选；上下键也不
  改变旧选中项。语义发布 `sourceview.completion.pending`，旧行禁用且不提供动作。
  新结果按文档、修订、挂载、选区、请求代次核验后替换；选中项按 Label、实际 Text、
  Range、Caret 和附加编辑的值保留，不按旧索引或指针地址误认。没有匹配项则关闭。
- 首次请求尚未显示时，左右移动也取消旧请求并为同一词生成新快照。仍只有一个运行
  请求与一个最新待处理项；不合作的 Provider 仍可能拖延新结果，但不能发布过期数据。
  当前请求失败关闭预览并按既有错误契约报告一次；正常取消不报告错误。
- 更新复用 Popover、ScrollView 与列表控件，释放旧候选的布局并重新测量新的有界候选集，
  更新尺寸、位置及选中项滚入视口。每次移动复用原生窗口，候选刷新本身不额外移动正文光标或产生历史。
  会话只保留词范围、有效 Definition 和显式触发标志，不长期保留完整请求文本快照。
- 切文档/语言、只读、禁用、预编辑、失焦、卸载、字体/配色改变和视口滚动使会话失效。
- 嵌入语言的会话有效性比较光标处的实际 Definition，而非要求子语言 ID 等于文档外层 ID。
- 记录 document/revision/mount/selection/语言及请求代次；提供者返回、弹窗创建及关闭后
  都重新核验。先清除会话身份再释放弹窗，关闭回调不能递归创建新会话或提交旧范围。

Ctrl+Space 不再由控件默认处理，以便系统输入法正常使用；应用可按自己的环境显式配置组合键。
模拟事件验证应用内分发，并不证明每种桌面的物理快捷键配置。

## 列表呈现

面板使用 `sourceview.completion` 配色，不借用正文选区的背景/前景。
非模态 Popover 设置该 StyleName，独占背景、圆角、1 DIP 细边框、阴影及阴影占位的绘制；
候选内容不重复绘制外壳。Modern 的圆角和阴影取自官方 menu，Basic 使用 4 DIP 圆角和官方 menu 阴影。
正常请求透明表面；若公开平台错误为 ErrUnsupported/ErrUnavailable、没有内容错误且会话仍有效，
释放失败宿主并仅重试一次不透明表面。其他错误直接返回，重试也失败时保留两次错误。
不透明外壳由 GOUI 使用矩形、无外部阴影规则；不把圆角画在另一块不透明矩形背景上。
重试前后核验文档、请求代次、挂载、焦点和可编辑性，过期会话不会重新显示。
名称与说明分别建立 native layout，使用补全面板的 UI 字体；说明字号为主体的
90%，以较弱的前景色右对齐在独立列中。两列共用实际字体基线，行高至少 26 DIP，
并为本次候选集中最大的 ascent/descent 留出合计 8 DIP 的上下余量。

- 面板内边距至少 6 DIP，按圆角和边框保守增加安全留白；内容由 ScrollView 结构性裁剪，
  选中背景与滚动条不覆盖外壳圆角。名称在视口内再留 6 DIP，两列至少间隔 16 DIP。
  较大自定义圆角增加留白时同步调整锚点，使名称仍尽量对齐词首。
- 同一会话先测量至多 64 个候选，仅保留列宽和字体度量。自然宽度限制在 240..520 DIP，
  名称/说明的自然列宽分别最多 260/220 DIP；最终宽度还受 Popover 的工作区约束。
  滚动只更换可见布局，不重新计算自然宽度或列宽。
- 两列不折行；超过列宽时，从原生 shaping Cluster 的逻辑边界选择前缀，加省略号后
  重新排版并测量，确认所选结果能放入列宽。重叠 Cluster 不从内部截断，不按 byte 数猜宽度。
  名称列不足以容纳省略号时不绘文字；不会用半个 glyph 填满剩余空间。
- 大小写无关前缀在名称中使用专用强调色；选中项使用另一组文字、说明和匹配色。
  Unicode 大小写转换可能改变 UTF-8 byte 长度，着色范围映射回原始名称；省略号不算匹配内容。
- 选中背景在完整行矩形内左右留 2 DIP、上下留 1 DIP，使用 selected part 的独立 Radius：
  Basic 默认为 2 DIP，Modern 取官方 menu-item 的圆角（当前 4 DIP），应用可显式覆盖为 0。
  负值及非有限值按 0 处理，实际半径不超过背景短边的一半，过窄区域不绘制负尺寸。
  先绘制完整圆角行，再交由 ScrollView 裁剪；部分行不会在视口边缘重新生成圆角。
  文字、行高、整行命中和语义边界不随装饰留白缩小；仅圆角变化也使旧会话按正常样式流程失效。
- 候选名称尽量与正在补全的词首对齐，面板顶部距离 caret 底部 2 DIP。
  词首在另一折行或已横向隐藏时使用 caret；最终滑移/约束仍由普通 Popover 处理。
- 面板拥有有限视口和留白，内部使用 GOUI ScrollView 承载自绘的 Scrollable 候选内容。
  ScrollView 负责像素/行滚动量、偏移钳制、裁剪、滚动条拖动及轨道点击；行滚动单位使用实际候选行高。
  不截取 DeltaY 的正负号模拟上下键，也不自行绘制装饰滚动条。
- 默认视口至多八行高，工作区约束可以进一步缩小。像素滚动允许首尾部分行露出，
  可见片段按偏移直接计算，绘制和命中包括滚动偏移；语义行边界裁到实际可见部分。
  上下键通过 ScrollIntoViewRequester 请求行的内容坐标，滚动状态只有 ScrollView 一份。
- 鼠标移入候选行更新选中态；边框、面板留白和滚动条区域不接受候选。
  快照的行边界与点击计算使用同一几何，发布 VisibleStart/VisibleEnd，以及完整
  `sourceview.completion.detail` 字符串属性；视觉省略不截断语义内容或插入文本。

补全界面配色与 UI 字体来自 `sourceview.completion` 及其命名 part，见
[样式接入](DesignStyles.md#补全窗口)。Basic 与 Modern 分别注册完整规则，
应用可追加覆盖；显式代码 Scheme 不改变列表外观。面板与文字的内置颜色均不透明，
边框和阴影保留显式 alpha。原生 Popover 从所属编辑器解析样式，保持局部作用域。
滚动条直接使用官方 scroll-view 的 trough/thumb 及状态规则；不再使用旧的装饰 scroll-thumb part。

## 资源与边界

词范围各向 caret 两侧最多读取 4096 byte，截到中途的词不补全。
文档词扫描两侧各 32 KiB；语法上下文沿用 [括号设计](DesignPairs.md) 的可靠状态与 16 KiB 前缀上限。
不在每次输入时遍历全文，也不把不完整词窗边界当成新词。

默认先按前缀筛选、去重，再最多保留 64 个候选，避免 SQL 等长词表的后部词汇被提前丢弃；
自定义提供者的前 64 个有效候选保持顺序，一次显示至多八行。Label 最长 512 byte、Detail 最长 1024 byte、
替换文本最长 64 KiB，输入按 UTF-8/LF 契约规范化；列表说明压为单行。
名称拒绝换行、Tab、VT/FF、NEL 和 Unicode 行/段分隔符，不让原生硬换行破坏候选行几何。
附加编辑至多 128 项，每项文本最多 64 KiB，与主文本合计最多 1 MiB。
提供者自己产生的数据和运行时间由应用负责；后台函数必须响应取消，不能访问 GUI。

八行高视口在像素偏移下至多与九行相交，静置时至多拥有十八个原生文本布局。
离开视口的行释放，仍可见且列宽未变的行保留布局；改变选择只更新相关文字颜色。
单个测量/省略过程逐一释放探针，构建期间最多十九个布局。构建事务暂持未发布资源，防止原生回调取消或卸载时
释放正在使用的探针；创建、度量、着色后均检查会话和代次。递归测量/绘制不会再次构建。
失败回滚本次资源，已显示会话关闭并报告一次；关闭、创建失败和卸载都释放。
同一选中状态的重复绘制不重复设置文字颜色，选中变化只更新相关可见行，不创建布局。
这些快照是正常指针输入规划信息，不提供执行候选的测试后门。

`ui` 提供 AutoPairs、AutomaticCompletion、CompletionProvider 薄绑定，未声明的布尔配置恢复默认。
相同配置指针不取消现有请求，替换配置会取消旧请求并关闭旧列表；不建立读取可变 UI 状态的
后台代理。卸载断开 signal 并清除提供者引用。语言与事务契约见 [语言扩展](DesignLanguageExtensions.md)。

## 参考与取舍

2026-10-23 核对左右导航的边界：

- [VS Code main SuggestModel._onCursorChange / _onNewContext](https://github.com/microsoft/vscode/blob/main/src/vs/editor/contrib/suggest/browser/suggestModel.ts)
  的当日源码对有效键盘光标变化重新过滤，左移时按词上下文重新请求；采用继续会话与取消的
  区分，不照搬贡献系统或服务容器。这里记录 main 源码观察，不宣称其所有版本行为相同。
- Scintilla 5 [SCI_AUTOCSETCANCELATSTART / SCI_AUTOCSELECT](https://www.scintilla.org/ScintillaDoc.html#SCI_AUTOCSETCANCELATSTART)
  的在线文档描述词边界取消和匹配项选择；本项目明确允许本次会话在词首空前缀继续显示，
  不照搬其默认取消位置或字符串消息协议。
- GtkSourceView 5.22.1 [CompletionProvider.refilter](https://gnome.pages.gitlab.gnome.org/gtksourceview/gtksourceview5/method.CompletionProvider.refilter.html)
  提供重过滤既有结果的职责边界。当前 SourceView 的 Provider 完全拥有过滤和排序，
  因此继续使用已有 Complete 回调和取消队列，不新增 Refilter 公共回调或缓存能力声明。

2026-10-16 核对以下文档与源码；没有复制实现或新增第三方依赖：

- [GtkSourceView 5 CompletionContext](https://gnome.pages.gitlab.gnome.org/gtksourceview/gtksourceview5/class.CompletionContext.html)
  把请求信息与 provider 分开；[5.18.0 gtksourcecompletion.c](https://github.com/GNOME/gtksourceview/blob/5.18.0/gtksourceview/gtksourcecompletion.c#L1192)
  监听视图移动以隐藏失效弹窗。采用显式上下文和生命周期，采用 context.Context 与普通 Go 值，不照搬 GObject 异步对象体系。
- [Scintilla 5 Autocompletion](https://www.scintilla.org/ScintillaDoc.html#Autocompletion)
  由宿主触发候选，候选选择与实际插入有独立步骤。采用可替换的候选来源，
  通过 GOUI 模型提交命令，省略消息和通知协议。
- 2026-10-20 核对 [VS Code 1.105.0 SuggestWidget](https://github.com/microsoft/vscode/blob/1.105.0/src/vs/editor/contrib/suggest/browser/suggestWidget.ts#L207)
  使用通用列表处理滚动。采用滚动与候选选择分离的职责；本项目的统一行高、有界候选和原生分段文本
  继续自绘，通过公开 Scrollable 契约复用 GOUI ScrollView，不同时迁移到通用 ListView。
  这是维护范围的取舍，不承诺比 ListView 更快或更省内存。验证见 [滚动改进记录](PlanCompletionScrolling.md)。
- [VS Code 1.105.0 SuggestController](https://github.com/microsoft/vscode/blob/1.105.0/src/vs/editor/contrib/suggest/browser/suggestController.ts#L247)
  防护选择过程中的取消，并在失焦时取消模型。采用重入后核验；采用独立的 Range/Caret/AdditionalEdits 与取消，尚不建设 snippet、异步 resolve 和语言服务基础。
- [GtkSourceView 5.22.1 CompletionCell](https://gnome.pages.gitlab.gnome.org/gtksourceview/gtksourceview5/class.CompletionCell.html)
  文档允许一个候选由名称、参数等多个单元呈现；采用分列和信息层级，保持现有 Label/Text/Detail
  公共接口。没有可靠的候选种类数据，不根据说明字符串猜图标。
- [VS Code 官方主题参考](https://code.visualstudio.com/api/references/theme-color#editor-widget-colors)
  在线文档提供独立的 editorSuggestWidget 背景、边框、选中和两种匹配前景；采用独立配色语义，
  不照搬 CSS 或依赖原生 Monaco。这里记录 2026-10-16 读取的文档承诺，不宣称其内部绘制算法。
- 上述 Scintilla 在线文档的 SCI_AUTOCSETMAXWIDTH/SCI_AUTOCSETMAXHEIGHT 说明宽度限制、
  省略号与超过可见行数时的滚动条；采用有界尺寸和滚动提示，实际 DIP 度量与资源管理使用
  GOUI ScrollView 公开契约。三端具体像素效果由本仓窗口验收另行记录。


## TOML、YAML、Shell 的基础建议

TOML 值上下文提供 `true/false/inf/nan`，YAML 提供 Core 的 `true/false/null`，
Shell 提供保留字及常见内建命令的字面量建议；TOML/YAML 文档词把连字符视为词的一部分。
字符串、注释、块文本与 heredoc 内不提供这三种语言的内置或附近文档词建议。
未知前置词法状态保持无建议；自定义 CompletionProvider 仍由应用决定。
不为配置键推断 Schema，不查询文件系统或 PATH，不执行 Shell。

2026-10-19 扩展补充核对官方 [Monaco CompletionItemProvider](https://microsoft.github.io/monaco-editor/typedoc/interfaces/editor_editor_api.languages.CompletionItemProvider.html)、
[CompletionItem](https://microsoft.github.io/monaco-editor/typedoc/interfaces/editor_editor_api.languages.CompletionItem.html)
的 trigger、取消、每项 range 和非重叠 additionalTextEdits；
[GtkSourceView 5.22.1 CompletionProvider](https://gnome.pages.gitlab.gnome.org/gtksourceview/gtksourceview5/iface.CompletionProvider.html)
推荐异步 populate 避免阻塞主循环。采用这些能力边界，以有界复制输入和单工作队列实现，
不把 Monaco 的可变模型传到 Go 后台，也不复制其完整服务框架。

## 外部索引更新后的显式刷新

`SourceView.RefreshCompletion() error` 只重查已经显示或正在请求的会话；没有会话时
正常返回 nil，不突然弹出列表。应用先发布自己的新索引，再在 GUI 序列调用此方法。
它不是 Provider 变更信号，也不是新的命令或声明式刷新计数器。

刷新从当前位置生成 CompletionContext，保留本次会话的 Explicit，Trigger 为空。
复用既有一个运行请求、一个最新待处理项的队列，取消过期任务；等待时保留旧弹窗预览，
标记 pending 并禁止接受旧候选。重复刷新合并为最新请求。结果依旧按文档、修订、
挂载、选区和请求代次校验。新候选替换原列表，值相同的选中编辑得以保留，否则选择首项。
空结果关闭；失焦、只读、预编辑或无效生命周期结束会话。

同步呈现错误直接返回；异步错误通过 ConnectError，取消和过期不是错误。
ui 使用既有控件引用调用，不复制会话状态或增加特殊生命周期。外部颜色提交与刷新
互相独立：SetHighlights 不调用刷新，也不关闭会话。

参考 GtkSourceView 的 [CompletionProvider](https://gnome.pages.gitlab.gnome.org/gtksourceview/gtksourceview5/iface.CompletionProvider.html)
异步填充/已有结果过滤，以及 Monaco 的
[CompletionItemProvider](https://microsoft.github.io/monaco-editor/typedoc/interfaces/editor_editor_api.languages.CompletionItemProvider.html)
请求上下文与取消契约。已有结果过滤不同于外部索引更新后的重新查询；本项目显式方法
是针对现有 Provider 和调用方所有权的选择，不声称参考框架提供同名接口。

## P1 补全呈现提案（尚未实现）

状态：2026-11-04 下一阶段设计。用户决定暂缓 [代码片段](DesignSnippets.md)，
本批仅扩充候选类型、完整说明和弃用状态，不改变普通文本插入、过滤排序或历史语义。
2026-11-10 框架对照复核后仍列为近期第一项，见 [能力差距与优先级](DesignCapabilityGaps.md)。
已发布的影子建议继续采用现有互斥规则，不把 AI 请求、片段会话或部分接受混入本批。
以下新增字段、尺寸和预算均是实施目标，不能作为已发布能力使用。

### 公共数据

在 Completion 增加三个字段，已有字段和零值行为不变：

```go
Kind          CompletionKind
Documentation string
Deprecated    bool
```

CompletionKind 使用枚举；首批为 CompletionKindText（零值）、Keyword、Function、Method、
Variable、Constant、Type、Property、Module、Value，各常量均使用 CompletionKind 前缀。
只依据显式 Kind 选择图标；未知值降为 Text，不拒绝可用的文本候选，不分析 Label/Detail 猜种类。
不预建一套 LSP 枚举或插件图标注册服务；片段暂缓时不增加暗示片段能力的常量。

- Detail 仍是列表中单行简短签名/类型说明，沿用现有规范化及 1024 byte 上限。
- Documentation 是独立纯文本说明，保留段落换行，统一 UTF-8/LF；不解释 Markdown/HTML，
  不读取外部资源，不自动打开链接。空说明正常且不产生占位提示。
- Deprecated 仅表示不推荐使用，仍可选择与接受。Provider 决定是否保留及排序，控件不隐藏、
  降序或禁用此类项。通过划线和本地化状态文字表达，不只用颜色。
- 三字段均由 Complete 的现有结果返回，不添加 Resolve 回调、可变候选对象或文档请求队列。
  应用索引变化仍通过 RefreshCompletion 重查；不在鼠标 hover 时调用 Provider。
- ui/types.go 同步类型与所有常量别名。CompletionProvider 绑定保持不变，不为呈现元数据
  添加新的声明式状态或重复配置项。

内建关键词仅在来源确定时标为 Keyword，附近文档词保留 Text；不从语言 ID 猜测其语义类型。
外部分析示例依据已有语义对象映射 Function/Variable/Type 等，可提供自己的说明。

### 列表和说明区

继续使用一个无原生焦点的 Popover。列表上方/下方的定位由原有锚点和工作区约束决定，
内部列表在上、说明区在下，不新增第二个浮窗或对外暴露说明控件。

- 任一候选有非 Text 种类时，为整个列表预留统一图标列；全为 Text 时不占该列。
  图标约 16 DIP，间隔 6 DIP，使用已有矢量设施，不使用 emoji 或平台字体图标猜测。
  词首对齐仍按名称列计算，图标不能推偏正文锚点。
- 保留名称/Detail 共基线、行高和选中圆角规则。名称省略后仍保留完整语义文字。
  弃用名称划线不改变文字排版宽度；前缀强调与选中态仍可辨识。
- 说明区按“完整 Detail → 弃用状态 → Documentation”显示，空项省略。
  短 Detail 已在列表完整呈现且没有其他信息时，不重复建立说明区。
- 本次候选集中存在说明内容时统一保留说明区；切到空内容项显示空区域，不留上一项文字。
  这样上下导航不使列表在光标附近反复跳动。候选集整体刷新后可以重新确定尺寸。
- 初始目标沿用列表 240..520 DIP 自然宽度；说明按列表宽度折行，不按文档最长行扩宽弹窗。
  说明区目标高度 120 DIP，并在本次候选集内固定；工作区不足时先缩小说明区，
  优先保留可交互的候选列表。无法同时展示一行候选与有效说明时隐藏视觉说明区。
  最终尺寸必须由父布局约束确定，不硬编码物理像素。
- 说明内容用自己的 ScrollView；滚轮按指针所在区域滚动，拖动说明滚动条不改变候选。
  候选切换重置说明滚动位置；同一候选且说明内容未变时保留。列表选中项和列表滚动
  不受说明布局或说明滚动影响。
- 鼠标进入说明区不确认候选、不改变选中项；输入继续送到编辑器，说明区不请求焦点。
  上下、Enter、Tab、Escape 和左右移动沿用既有规则，不默认增加冲突快捷键。
  说明暂不提供文本编辑/选择能力；语义树仍公开内容。

### 更新、资源和预算

保持 sameCompletion 目前按 Label 和实际编辑比较的身份规则；Kind、Detail、Documentation、
Deprecated 只属于呈现，不参与编辑身份。因此仅说明变化仍保留选中候选，但必须重绘其内容。
同编辑的多项候选仍按现有首次匹配规则保留，不借此次扩展添加公共 ID。

pending 期间保留旧候选预览并禁用接受，说明同步处于 pending 状态；新结果一次替换对应
候选和说明，不能把新候选配上旧文档。关闭、失焦、过期、销毁时按现有生命周期清理。

预算提案：每项 Documentation 最多保留 16 KiB，整个有效候选集的文档保留量最多 256 KiB。
超过时按 Provider 顺序在 UTF-8 边界截断说明，内部标记截断并在界面/语义中提示；
不丢弃可插入候选，也不改动其 Text。保留原有 64 项、替换文本和附加编辑上限。
按有界前缀规范化文档，不能先复制/规范化任意大的字符串再截断；复制保留字符串，避免
小切片继续引用 Provider 的巨大底层内容。Provider 自身分配与执行时间仍归应用管理。

仅为当前选中项建立说明布局，不为全部候选预排版。说明区拥有布局，键包括内容、字体、
宽度和缩放；颜色变化使用样式失效路径，不跨视图共享原生对象。切换项、宽度/字体改变、
关闭或卸载释放旧资源。16 KiB 是输入上限而非延迟保证，长单行和复杂文字需实测；
若原生测量造成可见停顿，使用已有上层分段设施或事件循环调度，不能改坏 Unicode 排版语义。
图标复用已有资源约定；不向 typography 增加领域接口。

### 样式、语义和本地化

在既有 sourceview.completion 下扩充 kind、documentation、separator、deprecated 命名 part，
Basic/Modern 分别注册；字体、前景和分隔线取主题规则，代码 Scheme 不影响弹窗。
selected 状态下图标和弃用文字均使用可读配色；高 DPI 使用 DIP 和矢量绘制。

候选语义快照继续提供完整 Label/Detail，增加稳定的种类字符串、deprecated 布尔值；
当前说明区发布静态文本及滚动范围，不为图标、分隔线、状态标签伪造点击动作。
说明的语义文本与实际保留内容一致，截断和 pending 可识别；被工作区隐藏的视觉内容
仍可通过当前候选的描述读取。若 GOUI 公共语义设施不能表达某项，记录明确缺口再取舍，
不擅自模拟未经支持的原生辅助功能行为。
“已弃用”“说明已截断”等控件文字进入既有多语言资源；Provider 的内容不自动翻译。

### 实施切片与验收

1. 修改 completion.go、ui/types.go，补充元数据清理、预算、身份保留测试；文档与代码同步。
2. 修改 completion_list.go、样式与本地化，加入图标、弃用和说明区；复用原有资源生命周期。
   说明区实现确有独立职责时才拆文件，不为几十行建立零碎组件。
3. 扩充外部分析示例及已有补全窗口验证主题，提供函数/变量/类型、长说明、弃用和动态刷新。
   不添加片段语法，不改变示例实际编辑效果。各切片具备测试并按功能提交。

逻辑测试覆盖零值、未知种类、UTF-8/CRLF、截断边界、总预算、Provider 顺序、元数据变化
保留选择、pending 禁止接受、过期结果、回调重入和双视图隔离。原有文本与附加编辑回归保留。
布局测试覆盖窄窗口、长单行、中文/组合字符/双向文本、说明滚动、候选切换及资源释放；
原有原生布局数量断言需分别计入列表与新增说明，保留有界性检查，不能仅扩大容差通过。

独立 X11 会话通过正常 DispatchEvent、CommitText 和指针路径验证真实交互，检查
Basic/Modern、亮暗主题、100%/150%、屏幕边缘和小工作区。说明出现不能跳选、提交候选、
改变正文、产生历史或抢焦点。记录输入到显示的延迟及事件循环间隔，不仅测接口返回。
执行 go test ./... 与 GOWORK=off go test ./...，按风险加 race、Windows/macOS 编译；
原生未运行的平台明确记录，不以交叉编译代替桌面验收。
当前提案不要求 GOUI 新接口；相邻 GOUI 工作区的版本差异与本批需求分别报告。

### 参考依据

2026-11-10 复核官方契约；没有执行参考框架的外观或性能对比。

- [Monaco CompletionItem](https://microsoft.github.io/monaco-editor/typedoc/interfaces/editor_editor_api.languages.CompletionItem.html)
  在线 API（页面未固定版本）区分 kind、detail、documentation、Deprecated tags。
  采用信息分工，不照搬 Markdown、提交字符、命令或完整 LSP 数据模型。
- [GtkSourceView 5.22.1 CompletionColumn](https://gnome.pages.gitlab.gnome.org/gtksourceview/gtksourceview5/enum.CompletionColumn.html)
  列出 ICON、TYPED_TEXT、COMMENT、DETAILS 等列。采用图标/正文/说明分离，
  不引入每一列的公共回调或可任意拼装单元系统。
- [Scintilla 自动补全](https://www.scintilla.org/ScintillaDoc.html#Autocompletion)
  文档标注 2026-10-01，支持候选类型图像及有界列表尺寸。采用类型提示和受约束滚动，
  不采用字符串中的类型分隔符；说明区尺寸、预算和选择保持是本项目设计取舍。
