# 补全设计

本专题描述 `completion.go`、`completion_list.go` 与 `ui` 的当前实现。
补全是 SourceView 的语言和交互能力；GOUI 提供普通 Popover、事件、排版和绘制，
不反向依赖本模块，也不在 TextView 中添加源码特例。

## 候选与公共接口

默认开启 AutomaticCompletion。普通非组合文本输入后按前缀请求候选；XML/HTML
输入 `<` 后也可显示标签候选。Ctrl+Space 显式唤出，应用也可调用 ShowCompletion。
只读、禁用、未挂载和 IME 预编辑期间不开始补全。

内建候选包括 JSON 的 true/false/null、HTML 常见标签和属性、XML 命名空间属性，
以及 caret 附近的文档词。标签名与属性上下文区分；JSON 字符串里不主动插入值关键字。
XML/HTML 名称允许连字符、冒号和点，普通标识符按 Unicode 字母、数字、Mark 与下划线取词。
这是一层词法补全，不承诺 JSON Schema、HTML DOM、XML XSD 或语义类型推断。

应用通过 `SetCompletionProvider(CompletionProvider)` 提供额外候选。
提供者是同步 GUI 线程函数，输入 CompletionContext，返回 `[]Completion, error`：

- Context 带文档、修订、语言、caret、完整词 Range、caret 前的 Prefix 和 Explicit。
- Completion 带 Label、Text、Detail；空 Text 使用 Label，Text 为字面替换文本。
- 自定义候选优先参与去重和数量截断，再补内建候选。匹配按 Label 的大小写无关前缀，
  排序按 Label，不伪装为模糊排序、项目符号索引或完整语义补全。
- 替换覆盖整个词，避免在词中间确认时遗留后缀；确认是一个带选区的 ApplyBatch 历史项。
- 没有候选是正常结果。ShowCompletion 直接返回提供者或创建失败；事件路径通过
  ConnectError 报告一次。提供者重入改变文档、修订、选区、语言或配置时返回 ErrOperationChanged。

提供者应及时返回；当前没有异步 provider、网络请求或额外编辑协议。
未来异步接口必须增加取消、请求身份与投递语义，不能把 goroutine 操作 GUI 当作扩展方案。

## 弹窗与输入

使用不获取原生焦点的普通非模态 Popover；键盘和 IME 保留在编辑器，继续经过
Window.DispatchEvent 和正常 IMClient。选择列表不是菜单：打开后仍需要正常键入前缀。

- 上下键循环切换，Enter/Tab 确认当前候选；鼠标移动选择、点击确认、滚轮浏览。
- Escape 只关闭本次补全，不改正文；下一次正常 Tab 的规则不因此变更。
- Backspace 更新候选；其他导航、指针定位、选区变化和外部修改关闭旧会话。
- 切文档/语言、只读、禁用、预编辑、失焦、卸载、字体/配色改变和视口滚动使会话失效。
- 记录 document/revision/mount/selection/语言及请求代次；提供者返回、弹窗创建及关闭后
  都重新核验。先清除会话身份再释放弹窗，关闭回调不能递归创建新会话或提交旧范围。

原生窗口或系统输入法可能先占用 Ctrl+Space；应用可按自己的快捷键方案调用 ShowCompletion。
模拟事件验证应用内分发，并不证明每种桌面的物理快捷键配置。

## 列表呈现

列表拥有自己的不透明面板、1 DIP 内缩边框和配色，不借用正文选区的背景/前景。
名称与说明分别建立 native layout：名称沿用代码字体，说明使用平台默认字体、代码字号的
90%，以较弱的前景色右对齐在独立列中。两列共用实际字体基线，行高至少 26 DIP，
并为本次候选集中最大的 ascent/descent 留出合计 8 DIP 的上下余量。

- 面板上下/行背景边距 6 DIP，名称左边距 12 DIP，两列至少间隔 16 DIP。
- 同一会话先测量至多 64 个候选，仅保留列宽和字体度量。自然宽度限制在 240..520 DIP，
  名称/说明的自然列宽分别最多 260/220 DIP；最终宽度还受 Popover 的工作区约束。
  滚动只更换可见布局，不重新计算自然宽度或列宽。
- 两列不折行；超过列宽时，从原生 shaping Cluster 的逻辑边界选择前缀，加省略号后
  重新排版并测量，确认所选结果能放入列宽。重叠 Cluster 不从内部截断，不按 byte 数猜宽度。
  名称列不足以容纳省略号时不绘文字；不会用半个 glyph 填满剩余空间。
- 大小写无关前缀在名称中使用专用强调色；选中项使用另一组文字、说明和匹配色。
  Unicode 大小写转换可能改变 UTF-8 byte 长度，着色范围映射回原始名称；省略号不算匹配内容。
- 候选名称尽量与正在补全的词首对齐，面板顶部距离 caret 底部 2 DIP。
  词首在另一折行或已横向隐藏时使用 caret；最终滑移/约束仍由普通 Popover 处理。
- 默认至多八行。受工作区高度限制时仅暴露完整行；可容纳完整行时，保证选中行可见。
  超出可见数量时，右侧预留 8 DIP 区域绘制 3 DIP 的滚动位置提示。
  提示只作装饰，浏览仍由上下键/滚轮完成，不发布拖动动作。
- 鼠标移入候选行更新选中态；边框、留白和滚动提示区域不接受候选。
  快照的行边界与点击计算使用同一几何，发布 VisibleStart/VisibleEnd，以及完整
  `sourceview.completion.detail` 字符串属性；视觉省略不截断语义内容或插入文本。

`scheme.Definition.Completion` 是独立的 `CompletionColors` 值，分别指定面板、名称、说明、
边框、选中背景/名称/说明、普通/选中匹配前景及滚动提示。背景及所有文字颜色必须不透明；
边框和滚动提示允许 alpha，包括用零 alpha 关闭。整组为零的自定义方案由普通前景/背景
派生一组完整配色，避免既有自定义方案出现透明文字；部分自定义应从现有 Definition 修改。

| 内置主题 | 面板 | 边框 | 选中背景 | 名称 | 次要说明 | 普通匹配 |
| --- | --- | --- | --- | --- | --- | --- |
| Light | #f8fafc | #b9c3cf | #e3edf9 | #24292f | #626d7a | #005fb8 |
| Dark | #25292f | #505b69 | #35465e | #dce2e9 | #a0aab7 | #79bbff |

## 资源与边界

词范围各向 caret 两侧最多读取 4096 byte，截到中途的词不补全。
文档词扫描两侧各 32 KiB；语法上下文沿用 [括号设计](DesignPairs.md) 的可靠状态与 16 KiB 前缀上限。
不在每次输入时遍历全文，也不把不完整词窗边界当成新词。

最多保留 64 个去重候选，一次显示至多八行。Label 最长 512 byte、Detail 最长 1024 byte、
替换文本最长 64 KiB，输入按 UTF-8/LF 契约规范化；列表说明压为单行。
名称拒绝换行、Tab、VT/FF、NEL 和 Unicode 行/段分隔符，不让原生硬换行破坏候选行几何。
提供者自己产生的数据和运行时间由应用负责，提供者必须避免阻塞事件循环。

列表静置时至多拥有十六个原生文本布局，按可见片段释放并重建；单个测量/省略过程逐一
释放探针，构建期间最多十七个布局。构建事务暂持未发布资源，防止原生回调取消或卸载时
释放正在使用的探针；创建、度量、着色后均检查会话和代次。递归测量/绘制不会再次构建。
失败回滚本次资源，已显示会话关闭并报告一次；关闭、创建失败和卸载都释放。
同一选中状态的重复绘制不重复设置文字颜色，选中变化只更新相关可见行，不创建布局。
这些快照是正常指针输入规划信息，不提供执行候选的测试后门。

`ui` 提供 AutoPairs、AutomaticCompletion、CompletionProvider 薄绑定，未声明的布尔配置恢复默认。
提供者经稳定代理调用最新回调，避免普通声明式重建反复关闭补全；回调替换影响后续请求，
已显示候选仍按原请求身份和上下文确认。卸载清除代理引用。

## 参考与取舍

2026-10-16 核对以下文档与源码；没有复制实现或新增第三方依赖：

- [GtkSourceView 5 CompletionContext](https://gnome.pages.gitlab.gnome.org/gtksourceview/gtksourceview5/class.CompletionContext.html)
  把请求信息与 provider 分开；[5.18.0 gtksourcecompletion.c](https://github.com/GNOME/gtksourceview/blob/5.18.0/gtksourceview/gtksourcecompletion.c#L1192)
  监听视图移动以隐藏失效弹窗。采用显式上下文和生命周期，当前不照搬其异步对象体系。
- [Scintilla 5 Autocompletion](https://www.scintilla.org/ScintillaDoc.html#Autocompletion)
  由宿主触发候选，候选选择与实际插入有独立步骤。采用可替换的候选来源，
  通过 GOUI 模型提交命令，省略消息和通知协议。
- [VS Code 1.105.0 SuggestController](https://github.com/microsoft/vscode/blob/1.105.0/src/vs/editor/contrib/suggest/browser/suggestController.ts#L247)
  防护选择过程中的取消，并在失焦时取消模型。采用重入后核验；本轮不建设它的
  snippet、additionalTextEdits、异步 resolve 和语言服务基础。
- [GtkSourceView 5.22.1 CompletionCell](https://gnome.pages.gitlab.gnome.org/gtksourceview/gtksourceview5/class.CompletionCell.html)
  文档允许一个候选由名称、参数等多个单元呈现；采用分列和信息层级，保持现有 Label/Text/Detail
  公共接口。没有可靠的候选种类数据，不根据说明字符串猜图标。
- [VS Code 官方主题参考](https://code.visualstudio.com/api/references/theme-color#editor-widget-colors)
  在线文档提供独立的 editorSuggestWidget 背景、边框、选中和两种匹配前景；采用独立配色语义，
  不照搬 CSS 或依赖原生 Monaco。这里记录 2026-10-16 读取的文档承诺，不宣称其内部绘制算法。
- 上述 Scintilla 在线文档的 SCI_AUTOCSETMAXWIDTH/SCI_AUTOCSETMAXHEIGHT 说明宽度限制、
  省略号与超过可见行数时的滚动条；采用有界尺寸和滚动提示，实际 DIP 度量与资源管理使用
  GOUI 公开契约。当前提示不是可拖动滚动条；三端具体像素效果由本仓窗口验收另行记录。
