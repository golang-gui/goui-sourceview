# 可选查找替换浮层

SourceView 自身提供查询、匹配高亮、结果导航与替换 API。FindBar 是同仓的可选命令式
配套控件，应用选择是否创建。推荐通过 GOUI Overlay 放在编辑器滚动视口的右上角，
与正文处在同一窗口，不挤占编辑区。宿主仍可选择自己的容器。
它不是 SourceView 的内置子控件，不拥有编辑器、Document 或第二份历史。
交互与生命周期见 [初次实现记录](PlanFindBar.md)，新外观验收见 [浮层记录](PlanFindBarAppearance.md)。

## 参考与取舍

- Monaco 共用的 VS Code `1.105.0` 编辑器实现提供
  [FindWidget](https://github.com/microsoft/vscode/blob/1.105.0/src/vs/editor/contrib/find/browser/findWidget.ts)
  与 [FindController](https://github.com/microsoft/vscode/blob/1.105.0/src/vs/editor/contrib/find/browser/findController.ts)：
  输入、查找选项、匹配数、上下项、替换和关闭作为编辑器贡献实现。采用这些局部交互，
  采用其编辑器内浮层及紧凑操作区，通过 GOUI 的普通控件树实现；不引入浏览器、
  服务容器、全局搜索剪贴板或工作台。另核对同版本的
  [FindWidget CSS](https://github.com/microsoft/vscode/blob/1.105.0/src/vs/editor/contrib/find/browser/findWidget.css)：
  单行查找、折叠替换、输入内选项、按宽度隐藏次要项、边框与阴影。
  本项目以 DIP 和原生排版尺寸约束布局，不直接把 CSS 像素当成字体 point。
- GtkSourceView 5 的 [SearchContext](https://gnome.pages.gitlab.gnome.org/gtksourceview/gtksourceview5/class.SearchContext.html)
  文档提供搜索、计数、替换、高亮，并将基础界面放在示例中。采用能力与界面可分离的边界；
  本项目计数按 GUI 帧分块，不宣称实现了其异步扫描/索引体系。
- Scintilla 5 的 [搜索替换接口](https://www.scintilla.org/ScintillaDoc.html#Searching)
  由宿主组织成编辑器交互。本项目继续暴露可单独使用的 API，应用不必使用 FindBar。

以上分别是参考源码与文档的证据。下面是本项目自己的契约，不表示与参考编辑器完全一致。

## API 与组合

```go
find := sourceview.NewFindBar()
find.SetTarget(editor)
viewport := gui.NewOverlay()
viewport.SetChild(scroll) // scroll.Child() == editor
viewport.AddOverlay(find)
viewport.ConnectOverlayPosition(find, func(available, size geometry.Size, p *geometry.Point) {
    p.X = max(0, available.Width-size.Width-8)
})
// 挂载后由应用调用，或通过编辑器中的快捷键打开：
err := find.Open(true) // false 只查找；true 展开替换
```

- `Target` / `SetTarget(*SourceView)` 借用目标。nil 解除绑定；目标与查找栏须挂载在同一 Root。
- `Open(bool) error` 打开并聚焦查询输入；未挂载、无目标或跨 Root 返回 ErrNotMounted。
  若取消编辑器预编辑的回调使操作过期，返回 ErrOperationChanged。
- `Opened`、`Close`、`ReplaceMode` / `SetReplaceMode`、`Replacement` / `SetReplacement`
  分别管理打开状态、替换行和替换文字，不触发模型修改。
- `ConnectOpen(func(bool))` 通知实际打开/关闭；`ConnectError(func(error))` 报告用户操作失败。
  API 自己返回错误；按钮/按键入口消费并通知一次，无默认错误弹窗。
- 一个编辑器由一个配套栏处理打开快捷键。应用自己的栏不需要创建 FindBar，直接使用 SourceView API。

FindBar 的外层保持普通 Widget 的 Visible；内部内容初始关闭。关闭时自然测量高度为零，
不会因为声明式通用 Visible 默认值恢复而重新打开/关闭。普通 GOUI TextInput、Button 和
`widgets.CheckButton` 拥有各自的输入、样式、原生资源和语义，选项有真实选中指示。
上下项、关闭和替换使用原始矢量线条图标，并提供本地化名称；Aa 和 ab 是实际的
ToggleButton，选中状态通过 GOUI CheckInfo 暴露，保留 Space/Tab/焦点和禁用语义。
提示、输入/按钮名称和计数状态支持 GOUI 的八种界面语言，见 [界面语言](DesignLocalization.md)。

查询和替换的占位提示使用 GOUI TextInput.SetPlaceholder，由输入框统一管理字体、基线、
裁剪、空内容/IME 显隐及语义快照；FindBar 在创建和语言变化时设置翻译后的普通字符串。
提示使用 sourceview.find.input 的 placeholder part。FindBar 不创建提示 Label，
不维护第二套提示定位或显隐监听；修改提示不改变查询、替换文本、选区或历史。

## 浮层布局与配色

- 自然宽度 448 DIP，阴影留白由当前样式计算；表面、圆角、边框和阴影来自注册规则。
  Basic 保留 4 DIP 面板圆角与紧凑字体；Modern 跟随官方 menu / text-input 及 UI 字号。
- 一行查找：展开按钮、查询字段及右侧 Aa/整词开关、匹配计数、上下项和关闭。
  展开时第二行替换字段与查询字段左右对齐，后接当前/全部替换图标。
- 行高至少 26 DIP，接受原生 TextInput 的较大测量结果；行间距 4 DIP。
- 小于 400 DIP 隐藏计数；小于 285 DIP 隐藏输入选项、上下项和替换动作。
  输入、展开/关闭及快捷键保留；恢复宽度时恢复控制项，不清除查询或替换文字。
- FindBar 不再拥有内部固定 StyleScope，也不从代码 Scheme 推导界面配色。
  使用自身所在子树的样式，TextInput/Button/CheckButton/Label/Icon 正常响应 StyleChanged。
  两个编辑区需要独立主题时，由应用显式包裹完整编辑区和查找浮层。
- 选区来自注册的原生 input selection 规则；字段、焦点、IME、命中和键盘分发仍在主窗口。
  完整样式名和覆盖方式见 [样式接入](DesignStyles.md#查找浮层)。

## 查询与交互

SourceView.Search 是查询的唯一来源。输入变更立即 SetSearch；外部 API 更改查询时同步字段与选项，
不重复建立搜索模型。FindBar 不控制 SourceView 的 Selection。

- 从编辑器打开时，以非空、最多 4096 字节且不含 LF 的选区作为查询；保留大小写/整词选项。
  多行/超长选区保留既有查询。栏内再次打开不拿当前匹配覆盖用户正在输入的查询。
- 查询字段采用 GOUI 的单行输入契约。程序化多行查询继续在核心中有效；栏中显示其单行规范化文本，
  同步显示不反写查询。用户显式修改字段后成为新的单行查询。不承诺多行查询输入界面。
- Enter / Shift+Enter 查找下一项/上一项；替换输入中的未修改 Enter 执行当前替换并推进。
  普通按钮/选项仍拥有自己的 Enter 和 Space，不被栏的导航抢占。
- 当前替换只修改已核验的当前源范围；无当前匹配时只定位下一项。替换成功后只有文档、修订、
  选区代次和栏目标仍属于这次操作时才继续查找，不覆盖回调切换的新文档或新选区。
- 全部替换复用 ReplaceAll 的预算与单次撤销。只读/禁用时替换输入和按钮不可用，仍能查找和复制。
- Escape/关闭按钮关闭栏；只有焦点原本在栏内时才把焦点归还目标。关闭保留查询及高亮，F3 可继续导航。
  隐藏或卸载的栏不拦截编辑器输入；卸载后应用的相同快捷键恢复可达。

| 绑定 | Windows / Linux | macOS |
| --- | --- | --- |
| 打开查找 | Ctrl+F | Cmd+F |
| 打开替换 | Ctrl+H | Cmd+Option+F |
| 下一项/上一项 | F3 / Shift+F3 | Cmd+G / Cmd+Shift+G，同时接受 F3 / Shift+F3 |

前两项只在配套栏挂载、目标同 Root 且栏可见可用时注册处理；裸 SourceView 保留给应用。
末项属于 SourceView 的核心导航，栏内输入聚焦时由栏转发到同一目标。所有绑定精确匹配修饰键，
仍经过正常 Window/Popover 分发；不添加语义执行协议。

## 计数、失效与生命周期

计数是栏的展示缓存，键为目标 Document 身份、模型修订和 SearchQuery。每个 GUI 帧最多推进
64 KiB 源区间，再读取一个查询长度的后缀上下文，跨块命中只按其起点计数一次。
只保存扫描位置、上次匹配末端和总数，不保存全文件命中数组。计数、Find 和 ReplaceAll
均使用从文首确定的不重叠序列，例如在 `aaaaa` 查找 `aa` 为两个匹配，
不因从第二个字符发起查找而改成另一套重叠范围。整词边界沿用搜索核心的 Word/NonWord 变化定义。

尚未完成显示“计数中”，没有完成前不把局部数字冒充总数。查询超过 4096 字节时不执行界面计数，
显示查询过长提示，显式导航仍可用。空查询不计数。关闭/不可见时暂停计数，文档、修订或查询变化
立即清零旧结果；打开后继续请求帧。显式 Find/Replace 仍遵守既有同步操作契约，
分块计数不代表全部搜索操作已经异步化，也不是对超大文件时延的性能承诺。

挂载期连接目标生命周期、私有搜索状态通知、Root 帧与语言通知。目标卸载时撤掉按键控制器和
搜索订阅，重新挂载且 Root 相同时再连接；栏卸载时全部释放。构造及脱离 Root 时不订阅目标。
更换目标先释放旧连接；两次相同 SetTarget 不重复注册。控件由宿主销毁，不删除或销毁借用的编辑器。

语义快照使用 `sourceview-find-bar` 角色；`sourceview.find` 属性给出打开/替换状态、计数及完整性。
输入、选项、按钮保留 GOUI 的角色、有效 Enabled、名称、边界和动作，装饰不产生伪交互动作。

## 声明式绑定

```go
base.Overlay(
    base.ScrollView(sourceui.SourceView().ID("editor").Document(document)),
    base.OverlayItem(sourceui.FindBar("editor")).OnPosition(
        func(available, size geometry.Size, p *geometry.Point) {
            p.X = max(0, available.Width-size.Width-8)
        }),
)
```

`ui.FindBar(targetID)` 在既有 BuildContext.AfterUpdate 中，按同 Root 的显式 ID 解析并 SetTarget。
每次协调后重新解析，不引入隐式依赖追踪、窗口特例或新的调度器。缺失/错误类型目标解除旧绑定。
Replacement/Open/ReplaceMode 仅在显式指定时受声明控制；OnOpen/OnError 的连接随协调更新和释放。
SourceView.Search 同样仅在显式指定时受声明控制，普通重建保留交互查询；传入空 SearchQuery 可清除。
