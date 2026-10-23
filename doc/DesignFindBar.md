# 内置查找、替换与定位界面

SourceView 默认具备查找、替换和行列定位界面。配套栏是控件自己的私有子树，
首次打开时创建，隐藏后保留输入状态；不再公开 FindBar、GoToBar 及其构造函数。
本轮契约取代早期“由应用挂载可选栏”的设计，旧 Plan 文档保留当时的实现和验证记录。

用户只需将 SourceView 放进 GOUI ScrollView。查询、结果导航和替换仍有独立的值／操作 API，
应用可以实现自己的工具栏或查找界面。关闭默认按键使用已有 KeyBindings，
不增加面板开关、控制器或 VisiblePanel 声明式状态。

## 参考与取舍

- Monaco 共用的 VS Code `1.105.0` 编辑器实现提供
  [FindWidget](https://github.com/microsoft/vscode/blob/1.105.0/src/vs/editor/contrib/find/browser/findWidget.ts)
  与 [FindController](https://github.com/microsoft/vscode/blob/1.105.0/src/vs/editor/contrib/find/browser/findController.ts)：
  输入、查找选项、匹配数、上下项、替换和关闭作为编辑器贡献实现。采用这些局部交互，
  采用其编辑器内浮层及紧凑操作区，通过 GOUI 的普通控件树实现；不引入浏览器、
  服务容器、全局搜索剪贴板或工作台。另核对同版本的
  [FindWidget CSS](https://github.com/microsoft/vscode/blob/1.105.0/src/vs/editor/contrib/find/browser/findWidget.css)：
  单行查找、折叠替换、输入内选项、按宽度隐藏次要项、边框与阴影。
  同版本 [FindInput CSS](https://github.com/microsoft/vscode/blob/1.105.0/src/vs/base/browser/ui/findinput/findInput.css#L14)
  将选项固定在输入顶部附近，FindWidget 的查找/替换操作区也使用独立的紧凑高度。
  采用操作区位置与多行输入高度分离的规则，不让选项随整个输入框垂直居中。
  本项目以 DIP 和原生排版尺寸约束布局，不直接把 CSS 像素当成字体 point。
- GtkSourceView 5 的 [SearchContext](https://gnome.pages.gitlab.gnome.org/gtksourceview/gtksourceview5/class.SearchContext.html)
  文档提供搜索、计数、替换、高亮，并将基础界面放在示例中。采用能力与界面可分离的边界；
  本项目计数按 GUI 帧分块，不宣称实现了其异步扫描/索引体系。
- Scintilla 5 的 [搜索替换接口](https://www.scintilla.org/ScintillaDoc.html#Searching)
  由宿主组织成编辑器交互。本项目继续暴露可单独使用的 API，应用不必使用 FindBar。

以上分别是参考源码与文档的证据。下面是本项目自己的契约，不表示与参考编辑器完全一致。

## 公开操作与内部组合

```go
editor := sourceview.NewSourceView()
scroll := gui.NewScrollView()
scroll.SetStyleName("source-view")
scroll.SetChild(editor)
// 挂载后由菜单调用；正常键盘输入使用同一操作路径。
err := editor.ShowFind()
err = editor.ShowReplace()
err = editor.ShowGoTo()
editor.HidePanel()
```

- ShowFind 聚焦查询，按既有规则从正文选区初始化查询；ShowReplace 同时展开替换。
- ShowGoTo 以当前光标的 1 起始行／码点列初始化定位输入。
- 三项要求已挂载且未销毁的编辑器；未挂载返回 ErrNotMounted，禁用返回 ErrDisabled。
  焦点、预编辑和挂载回调改变操作上下文时取消旧操作，返回 ErrOperationChanged。
- 同一视图的查找／替换与定位互斥；不同视图的输入、打开状态、订阅及配色相互独立。
- HidePanel 保留查询、替换文字和正文选区；关闭正在显示的查找栏时隐藏搜索背景并释放选区查找范围。
  只有焦点原本在栏内时才归还正文。外部回调产生新的操作时，不继续关闭新的界面。
- 同步调用通过返回值反馈错误；按键与栏内按钮的失败通过 SourceView.ConnectError 报告一次。
- 不提供公开栏对象、Target、外部定位或面板可见状态绑定。

内部栏是 SourceView 的普通 GUI 子控件，锚定当前视口右上，距边缘 8 DIP。
不参与正文 ContentSize、HeightIndex、折行宽度或滚动偏移。可用宽高减去边距后，
用公开约束测量栏并安排真实子树。关闭时整个栏隐藏，不留下拦截正文的空矩形。
ScrollView 仍拥有滚动状态和滚动条；SourceView 无需另建滚动系统或公共 Editor 包装类型。

正文拖选和右键菜单使用 Target 事件阶段；正文的文本鼠标指针不向内部工具栏继承。
栏内输入保持自己的命中、焦点、IME、
选择和滚动。编辑器的 Capture 控制器仅统一处理 Find/Replace/GoTo 命令；普通编辑
仍要求正文获得焦点，Enter、Shift+Enter、Escape 由各输入界面处理。
普通 TextView/ScrollView、Button 和 widgets.CheckButton 保留原生资源和语义。
上下项、关闭和替换使用原始矢量线条图标，并提供本地化名称；Aa 和 ab 是实际的
ToggleButton，选中状态通过 GOUI CheckInfo 暴露，保留 Space/Tab/焦点和禁用语义。
选区选项采用四角选区边界和文本行的 16 DIP 矢量图形，名称为本地化的“在选区中查找”，
使用同一按钮样式的选中、悬停、禁用状态，不依赖单独颜色表达功能。
提示、输入/按钮名称和计数状态支持 GOUI 的八种界面语言，见 [界面语言](DesignLocalization.md)。

查询和替换的占位提示使用 GOUI TextView.SetPlaceholder，由输入框统一管理字体、基线、
裁剪、空内容/IME 显隐及语义快照；FindBar 在创建和语言变化时设置翻译后的普通字符串。
提示使用 sourceview.find.input 的 placeholder part。FindBar 不创建提示 Label，
不维护第二套提示定位或显隐监听；修改提示不改变查询、替换文本、选区或历史。
版本条件和平台结果见 [占位提示接入验证](PlanTextPlaceholder.md)。

## 浮层布局与配色

- 自然宽度 512 DIP，阴影留白由当前样式计算；表面、圆角、边框和阴影来自注册规则。
  Basic 保留 4 DIP 面板圆角与紧凑字体；Modern 跟随官方 menu / text-input 及 UI 字号。
- 一行查找：展开按钮、查询字段及右侧 Aa/整词/正则/选区开关、匹配计数、上下项和关闭。
  展开时第二行替换字段与查询字段左右对齐，后接当前/全部替换图标。
- 两行独立测量，常规行高至少 28 DIP，行间距 4 DIP。先预留按钮、计数及选项宽度，
  再以实际输入宽度调用 ScrollView.Measure；高度上限通过既有 MaxSize 和父约束表达，
  每个输入视口至多 200 DIP。软换行参与高度；不从逻辑行数或 ContentSize 推算字段高度。
- 每个输入框顶部设紧凑操作行：查找行包含展开、选项、计数、上下项和关闭；替换行包含替换动作。
  操作行至少 28 DIP，并容纳原生测量的选项/状态高度及上下各 3 DIP 留白；输入行高度同时容纳
  操作行和输入内容。按钮在该操作行内居中，字段增高仅向下扩展，不改变按钮相对字段顶部的位置。
  输入滚动不移动操作行。回归与平台记录见 [顶部对齐验证](PlanFindBarAlignment.md)。
- 紧凑字段使用 ScrollVertical / MainWeight(0)。外框由 FindBar 绘制，TextView 维持无边框内容样式；
  文字、Placeholder、光标和输入几何由 GOUI 共用的排版规则负责。
  完整自动增高体验需要 GOUI 的内容驱动 Measure 修正；v0.2.0 尚不含此行为，
  当前 GOUI v0.3.0 已具备该能力，依赖升级与验收见 PlanTextViewSizing.md。
- 小于 400 DIP 隐藏计数；小于 285 DIP 隐藏输入选项、上下项和替换动作。
  输入、展开/关闭及快捷键保留；恢复宽度时恢复控制项，不清除查询或替换文字。
- FindBar 不再拥有内部固定 StyleScope，也不从代码 Scheme 推导界面配色。
  使用自身所在子树的样式，TextView/ScrollView/Button/CheckButton/Label/Icon 正常响应 StyleChanged。
  两个编辑区需要独立主题时，由应用显式包裹完整编辑区和查找浮层。
- 选区来自注册的原生 input selection 规则；字段、焦点、IME、命中和键盘分发仍在主窗口。
  完整样式名和覆盖方式见 [样式接入](DesignStyles.md#查找浮层)。

## 查询与交互

SourceView.Search 是查询的唯一来源。输入变更立即 SetSearch；外部 API 更改查询时同步字段与选项，
不重复建立搜索模型。结果导航和范围切换通过同一搜索核心更新 SourceView 的 Selection。

- 从编辑器打开时，以非空、最多 4096 字节且不含 LF 的选区作为查询；保留大小写/整词选项。
  多行/超长选区保留既有查询。栏内再次打开不拿当前匹配覆盖用户正在输入的查询。
- 查询和替换使用多行输入，LF 原样保留；Alt+Enter 在当前字段插入换行。
  不把多行查询规范化为空格，更多内容在 ScrollView 内滚动。
- Enter / Shift+Enter 查找下一项/上一项；替换输入中的未修改 Enter 执行当前替换并推进。
  普通按钮/选项仍拥有自己的 Enter 和 Space，不被栏的导航抢占。
- 当前替换只修改已核验的当前源范围；无当前匹配时只定位下一项。替换成功后只有文档、修订、
  选区代次和栏目标仍属于这次操作时才继续查找，不覆盖回调切换的新文档或新选区。
- 全部替换复用 ReplaceAll 的预算与单次撤销。只读/禁用时替换输入和按钮不可用，仍能查找和复制。
- “在选区中查找”未启用且当前选区为空时禁用；启用后即使当前选区变为空仍可关闭。
  开启时复制非空源范围，结果导航不重新捕获范围；低层 SetSearchRange 仍允许用于零长度正则的空范围。
  切换后立即使旧计数失效，从原选区起点（包含该点）重新定位新范围内的下一匹配并环绕。
  当前选区已对应合法匹配时保留位置；无匹配保留普通选区并清除当前搜索匹配状态。
  第一个 64 KiB 计数分块在操作中推进，后续继续按帧计数，未完成时显示“计数中”。
- Escape/关闭按钮关闭栏；只有焦点原本在栏内时才把焦点归还目标。关闭保留查询、替换文字和
  大小写/整词/正则选项，清除独立查找范围并关闭普通/当前搜索背景；保留普通文本选区。
  F3 可继续按保留的查询在全文件导航，保持搜索背景关闭；重开时恢复搜索背景。
  隐藏或卸载的栏不拦截编辑器输入；卸载后应用的相同快捷键恢复可达。

搜索背景显隐由 SourceView.HighlightSearchMatches / SetHighlightSearchMatches(bool) 管理，
新建视图默认 true，裸 SourceView 的程序化搜索行为不变；该属性只参与绘制，不改变查询、
计数、替换或普通选区。FindBar 打开时启用，关闭或释放已打开的目标时停用；关闭范围通过
SetSearchRange(nil) 实现。编辑器卸载时关闭栏并清理会话，重新挂载后由用户再次打开。
同一 Document 的其他视图不受影响；不引入全局搜索会话或通用装饰注册体系。
语义上的查询、普通选区与导航仍可用，不因背景隐藏伪装为空文本。
范围/选区通知可重入，操作继续前核对生命周期、目标、Document、修订、查询、范围及选区代次。

以上修正参照 VS Code / Monaco 1.105.0
[选区按钮条件](https://github.com/microsoft/vscode/blob/1.105.0/src/vs/editor/contrib/find/browser/findWidget.ts#L415)、
[范围切换及关闭](https://github.com/microsoft/vscode/blob/1.105.0/src/vs/editor/contrib/find/browser/findController.ts#L159)，
并采用 GtkSourceView 5 [独立搜索高亮开关](https://gnome.pages.gitlab.gnome.org/gtksourceview/gtksourceview5/method.SearchContext.set_highlight.html)
的能力边界；本项目关闭后 F3 只导航的行为以上述契约为准，不宣称完全复制参考实现。

| 绑定 | Windows / Linux | macOS |
| --- | --- | --- |
| 打开查找 | Ctrl+F | Cmd+F |
| 打开替换 | Ctrl+H | Cmd+Option+F |
| 下一项/上一项 | F3 / Shift+F3 | Cmd+G / Cmd+Shift+G，同时接受 F3 / Shift+F3 |

默认绑定由 SourceView 自身处理，无需先创建或挂载配套栏。焦点在正文或其内部配套输入时生效。
表中为默认绑定。打开/替换及结果导航读取目标 SourceView 当前 KeyBindings，解除后旧键不再消费；
字段本地的 Enter/Shift+Enter/Escape 保持不变，见 [命令快捷键](DesignKeyBindings.md)。
末项属于 SourceView 的核心导航，栏内输入聚焦时由栏转发到同一目标。所有绑定精确匹配修饰键，
仍经过正常 Window/Popover 分发；不添加语义执行协议。

## 计数、失效与生命周期

计数是栏的展示缓存，键为目标 Document 身份、模型修订、SearchQuery 和独立范围。
单行字面量每帧最多推进 64 KiB 源区间及查询后缀，保留扫描位置和总数；
正则/多行查询复用搜索核心当前修订的规范匹配数组，首次构建是同步全量分析。
三类查询的计数、Find 和 ReplaceAll 使用规范不重叠序列，整词保留既有边界策略。
缓存所有权、捕获组、零长度匹配及预算详见 [查询设计](DesignSearch.md)。

尚未完成显示“计数中”，没有完成前不把局部数字冒充总数。查询超过 4096 字节时不执行界面计数，
显示查询过长提示，显式导航仍可用。空查询不计数。关闭/不可见时暂停计数，文档、修订或查询变化
立即清零旧结果；打开后继续请求帧。显式 Find/Replace 仍遵守既有同步操作契约，
分块计数不代表全部搜索操作已经异步化，也不是对超大文件时延的性能承诺。

SourceView 拥有内部栏及其生命周期。栏挂载期间连接私有搜索状态、Root 帧和语言通知，
卸载时释放。编辑器卸载先关闭栏，再释放正文资源；允许重挂载，旧的原生资源和计数任务不能继续运行。
懒创建不向正文重复安装按键控制器，也不按重建次数增加订阅。

语义快照使用 `sourceview-find-bar` 角色；`sourceview.find` 属性给出打开/替换状态、计数及完整性。
输入、选项、按钮保留 GOUI 的角色、有效 Enabled、名称、边界和动作，装饰不产生伪交互动作。

## 声明式接入与取消快捷键

```go
base.ScrollView(
    sourceui.SourceView().Document(document).AcceptsTab(true),
).Style("source-view")
```

不声明配套控件或显示状态；重建不会因默认值恢复而关闭、打开或重新初始化内部栏。
已有 ui.FindBar、ui.GoToBar 及根包对应公开类型删除，调用方迁移为 SourceView 上的操作。

```go
bindings := sourceview.DefaultKeyBindings()
delete(bindings, sourceview.CommandFind)
delete(bindings, sourceview.CommandGoTo)
sourceui.SourceView().Document(document).KeyBindings(bindings)
```

解除绑定后不消费对应按键，应用快捷键可正常收到。查找、替换、定位的直接方法仍可用。
若也要解除替换键，删除 CommandReplace；不为某个具体组合键另建布尔开关。

## 验证边界

包内验证未挂载调用、资源隔离与清理；窗口用例验证默认打开、重新绑定、取消绑定、
重复重建、视图移除与重挂载、互斥、栏内拖选及正文事件隔离、焦点恢复、计数、替换、
亮暗主题和缩放。测试输入经过正常 Window.DispatchEvent/CommitText，
桌面观察另在独立 X11 会话进行。实际结果及未执行的平台组合见 Plan。
