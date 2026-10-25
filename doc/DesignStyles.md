# Basic / Modern 样式接入

## 职责与使用

SourceView 根包在 init 中分别调用 `basic.Register`、`modern.Register`，登记名为
`github.com/golang-gui/goui-sourceview`。应用只需正常导入 SourceView 并启用 Modern：

```go
app.SetStyleSheet(modern.FromSettings(app.Settings()))
// 声明式 build：
return ui.Root().StyleSheet(modern.FromSettings(app.Settings())).Windows(/* ... */)
```

不需要额外导入适配包。GOUI 默认 nil stylesheet 使用包含 SourceView 的 Basic。
所有第三方控件必须实现 Basic，可选实现 Modern；两个注册入口分别由各主题拥有。
GOUI 的 [注册契约](../../goui/doc/DesignThirdPartyStyles.md) 定义初始化冻结、规则排序和快照。
本模块不增加主题标识、中心匹配、动态适配器或第三方主题降级协议。

Basic 提供完整默认外观；Modern 根据本次 Options 和官方控件的只读 base 生成规则。
Dark、主题色、UI 字体均来自当前 Modern 参数，不能从系统设置猜测应用主题。
注册函数只生成值，不创建 Widget、读取应用、申请原生资源或调用完整主题构造。
应用可在主题 Rules 末尾追加同 selector 覆盖。StyleSheet 是不可变快照，切换时替换整张表。

## 代码区

默认代码区字体是 monospace / 12 pt，与 UI 字体分离，可用 `source-view` 的字体规则覆盖。
Basic 采用内置 Light 的代码颜色；Modern 根据 Dark 选择语法颜色，并从官方 `text-view`
取得正文背景、前景；选区以官方输入框焦点色作为 tint，活动 alpha=64、非活动 alpha=32。保留独立的代码侧栏、当前行、搜索、括号等领域配色。

`SetScheme(s)` 仅覆盖代码区域的完整配色；`SetScheme(nil)` 恢复所属 stylesheet。
`scheme.Definition` 只维护代码颜色，不再包含 Find / Completion UI 调色板。
UI 的界面字体、焦点、圆角和阴影不由代码 Scheme 控制。

## 滚动编辑区域外框

SourceView 仍是正文叶子控件，外部 ScrollView 拥有包含滚动条的统一外框：

```go
scroll := gui.NewScrollView()
scroll.SetStyleName("source-view")
scroll.SetChild(sourceview.NewSourceView())
// 声明式：ui.ScrollView(sourceui.SourceView()).Style("source-view")
```

Basic / Modern 注册分别从官方 `text-view/frame` 复制 frame 的各状态，
从 `scroll-view/trough`、`scroll-view/thumb` 复制滚动条外观。代码正文仍无
外框，原有 `border` part 只控制侧栏分隔等代码装饰，不替代 `frame`。
FindBar 和补全弹窗保留独立容器及样式。显式 Scheme 只改变正文，外框保持
当前 GUI 主题；需要共同自定义时，调用方也为宿主的 `frame` part 提供规则。

ScrollView 在所有焦点/禁用状态间预留稳定圆角安全内区，聚焦只改变外观，
不改变正文宽度或换行。主题变化沿既有 StyleChanged 重新计算；框架机制见
[GOUI 外框设计](../../goui/doc/DesignScrollFrame.md)。该机制需要 GOUI 本次
尚未发布的变更，单独依赖 v0.2.0 不具备此组合效果；本地使用双仓工作区。

选区默认保留 token 前景；SelectionForeground.A==0 表示不覆盖，255 表示活动选区
统一前景，其他 alpha 返回错误。Basic/Modern 的 selection part 显式写入零值前景，
阻断根前景的 part 回退。应用追加 `.Part("selection").ForegroundColor(...)` 可开启覆盖，
追加 `.ForegroundColor(color.NRGBA{})` 可恢复保留语法；完整自定义规则也遵守该契约。
背景 alpha 通过真正的 source-over 合成，不能对预混表面色再次降低 alpha。

代码样式名默认为 `source-view`，以下 part 具有公开语义：

| part | 字段 |
| --- | --- |
| 无 | 背景、前景、字体 |
| gutter | 行号区背景、前景 |
| current-line / search / current-search | 对应叠加背景 |
| selection | 活动选区背景、前景 |
| inactive-selection | 非活动选区背景 |
| bracket | 匹配背景与边框色 |
| caret / preedit | 插入点、预编辑下划线前景 |
| border | 编辑区边框色 |
| whitespace / selected-whitespace | 普通/被选中空白及换行标记的前景 |
| token-plain/property/string/escape/number/keyword/delimiter/tag/attribute/entity/comment/meta/invalid | 对应 token 前景 |

空白标记的两个 part 默认采用相同的低对比中性色：代码背景较亮时为
`#33333333`，较暗时为 `#E3E4E229`（RGBA，末两位为 alpha）。选择变化只改变
使用的 part，不自动提高到正文前景强度。应用可分别覆盖普通和选中色，包括透明值。
显式代码 Scheme 沿现有 partColor 契约使用由代码背景决定的相同默认色。

解析使用 `gui.ResolveWidgetStyle`，因此支持应用主题和显式 `StyleScope`。
颜色转为普通 NRGBA 后按现有 Scheme 契约验证；无效的代码基础色经 ConnectError 上报。
`StyleChanged` 即使字体未改变也更新配色。等价颜色沿用同一不可变 Scheme；颜色变化只让
paragraphStyle 的着色身份失效，不重建文本几何、HeightIndex 或原生布局。
字体变化按原有布局失效路径释放原生资源。关闭补全等外部回调后检查生命周期与状态。
控件不再维护用于自动配色的应用设置订阅；系统跟随由应用/声明式层更新样式表。

## 查找浮层

FindBar 使用自己在控件树中的样式环境，不在内部安装固定 Basic StyleScope。
需要两个亮暗编辑区时，将各自的 SourceView 放入明确的 StyleScope；内部栏自然继承同一子树的样式。
SetTarget 只建立编辑行为关联，不把另一个子树的外观隐式传进来。

| 名称 / part | 用途 |
| --- | --- |
| sourceview.find | 面板背景、前景、边框、圆角、阴影 |
| sourceview.find.input | 输入 UI 字体、前景与透明内容区 |
| sourceview.find.input / frame | 输入框表面；Normal / Focused / Disabled 的边框 |
| sourceview.find.input / selection、caret、placeholder | 原生文本选区、插入点与占位提示前景 |
| sourceview.find.action / focus | 操作按钮及独立键盘焦点 |
| sourceview.find.option、sourceview.find.expand / button、button-checked、focus | 开关与展开按钮 |
| sourceview.find.icon、option-text、status、error | 显式命名的内容样式 |

Modern 的面板/输入/焦点分别取官方 menu / text-input / Focused；选项、错误与次要文字
取当前主题值。UI 字号使用 Options.FontSize，圆角取官方 menu / text-input。
Basic 保留紧凑的 9.75 pt 输入与 4/2 DIP 圆角。操作图标使用专名，不覆盖全局 icon。

布局继续使用 448 DIP 自然宽度、原生输入测量、4 DIP 行距与既有窄宽度策略。
面板阴影空间从 shadow 的公开半径/偏移计算并保留在普通控件分配内；关闭阴影后不保留
原有大留白。颜色、边框、圆角与阴影通过正常 StyleChanged 刷新，无独立配色帧轮询。
原生输入、焦点、IME、命中和语义仍由现有 GOUI 控件负责。

## 补全窗口

独立原生 Popover 使用 `sourceview.completion` StyleName：根规则给出背景、边框、圆角、阴影和字体，
由宿主统一绘制外壳与预留阴影空间。Modern 跟随 menu 圆角和阴影，Basic 圆角为 4 DIP。
detail / selected / selected-detail / match / selected-match 部件分别控制说明、选中和匹配。
selected 同时控制选中背景的独立 Radius：Basic 为 2 DIP，Modern 跟随官方 menu-item（当前 4 DIP）；
追加 `style.Name("sourceview.completion").Part("selected").Radius(...)` 可覆盖，0 显式使用直角。
圆角不继承外壳的大小；背景左右留 2 DIP、上下留 1 DIP，完整行命中区域和文字位置保持不变。
滚动时裁剪完整圆角背景，半径按实际背景尺寸钳制；负值及非有限值按 0 处理。
滚动条复用官方 scroll-view 的 trough/thumb 和 Normal/Hovered/Pressed 状态；
旧的 sourceview.completion / scroll-thumb 装饰部件已移除，应用覆盖请改用 scroll-view / thumb。
普通/选中匹配前景均来自 Modern 的对比度调整主题色。
文字使用 UI 字体，说明字号为主体的 90%；名称列、原生 cluster 省略和语义完整文本不变。

补全窗口通过所属 SourceView 显式解析样式，不能因为进入另一个原生 Root 而丢掉
编辑器的局部 StyleScope。快照只保存普通颜色、字体和表面参数；原生布局仍由列表独占，
外观变化、会话取消或卸载释放。运行中实际外观变化关闭旧会话，下次打开采用最新样式；悬停等未改变外观的样式通知保留会话。
边框及阴影的显式 alpha 原样保留，不由缺省配色推导覆盖。候选内容和 ScrollView 安排在圆角安全区域。
透明表面不可用时重试不透明宿主，由 GOUI 应用矩形无外部阴影外观；字体、内容和输入契约一致。

## 验证与参照

包测试覆盖默认登记、稳定规则、所有 token part、同字体换色、显式代码方案、UI 隔离、
主题字号/圆角/焦点、应用覆盖、透明装饰及 Popover 所有者样式。原先 scheme 的 UI
调色板测试迁移到这些规则测试，不再要求代码 Scheme 验证界面配色。
现有补全几何、逐字颜色、资源数量、取消与原生省略断言继续保留。
窗口测试验证亮暗并列、单侧换主题、原生输入颜色、缩窄恢复和正常事件路径。

[Monaco 主题颜色](https://code.visualstudio.com/api/references/theme-color#suggest-widget)
将 suggest 的表面、说明、匹配、选中分别命名，采用这些语义，不照搬其注册中心。
[GtkSourceView 5 StyleScheme](https://gnome.pages.gitlab.gnome.org/gtksourceview/gtksourceview5/class.StyleScheme.html)
将源码配色作为编辑器资源；本实现保留代码 Scheme，与 GOUI 界面主题职责分离。


Shell 变量使用 `language.KindVariable`，样式 part 为 `token-variable`，basic、modern
及内置亮暗 Scheme 均提供默认前景；自定义 Scheme 未配置该分类时沿用普通前景。

## 应用语言的样式键

Kind 已扩展为字符串键。Definition.Styles 声明自定义键，按 `token-<key>` 解析前景，
同样可以放入显式 Scheme.Tokens。未知色使用普通前景；词法语义由 Token.Class/Context
给出，不根据颜色推断字符串/注释/括号。绑定新定义或换文档时重新解析样式，
Basic/Modern 注册、局部 StyleScope 和选区混色机制保持统一，见 [语言扩展](DesignLanguageExtensions.md)。
