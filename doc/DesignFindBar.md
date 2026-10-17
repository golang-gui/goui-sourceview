# 可选查找替换栏

SourceView 自身提供查询、匹配高亮、结果导航与替换 API。FindBar 是同仓的可选命令式
配套控件，应用选择是否创建，以及放在工具栏、编辑区上方或自己的容器中。
它不是 SourceView 的内置子控件，不拥有编辑器、Document 或第二份历史。
具体实现及平台验证边界见 [本轮记录](PlanFindBar.md)。

## 参考与取舍

- Monaco 共用的 VS Code `1.105.0` 编辑器实现提供
  [FindWidget](https://github.com/microsoft/vscode/blob/1.105.0/src/vs/editor/contrib/find/browser/findWidget.ts)
  与 [FindController](https://github.com/microsoft/vscode/blob/1.105.0/src/vs/editor/contrib/find/browser/findController.ts)：
  输入、查找选项、匹配数、上下项、替换和关闭作为编辑器贡献实现。采用这些局部交互，
  不引入其浏览器浮层、服务容器、全局搜索剪贴板或工作台。
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
column.AddChild(find)
column.AddChild(scroll) // scroll.Child() == editor
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
上下项/关闭使用紧凑箭头及关闭标记并提供自然名称；选项行使用 WrapBox，在窄宽度下换行。
不修改应用样式表，也不在 SourceView 中绘制查找栏。

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
base.VBox(
    sourceui.FindBar("editor"),
    base.ScrollView(sourceui.SourceView().ID("editor").Document(document)).MainWeight(1),
).CrossAlign(layout.CrossStretch)
```

`ui.FindBar(targetID)` 在既有 BuildContext.AfterUpdate 中，按同 Root 的显式 ID 解析并 SetTarget。
每次协调后重新解析，不引入隐式依赖追踪、窗口特例或新的调度器。缺失/错误类型目标解除旧绑定。
Replacement/Open/ReplaceMode 仅在显式指定时受声明控制；OnOpen/OnError 的连接随协调更新和释放。
SourceView.Search 同样仅在显式指定时受声明控制，普通重建保留交互查询；传入空 SearchQuery 可清除。
