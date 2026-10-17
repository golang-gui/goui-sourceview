# SourceView 布局、滚动与呈现设计

> 状态：首版实现规格。独立 SourceView 叶子控件复用 typography 与文本几何，
> 不使用 TextView 的布局或私有缓存。原生 Tab 配置依赖 [G2](DesignGOUIIntegration.md)。

## 1. Widget 与滚动组合

SourceView 嵌入 WidgetBase，实现 Widget、IMClient、Scrollable、
ScrollIntoViewRequester。焦点、IME 和语义属于同一个 Widget；
行号是该 Widget 的源码侧栏，不为每一行创建子 Widget。

使用方式为 SourceView 放入 GOUI ScrollView。Scrollable 路径把 SourceView 安排为
实际视口大小，通过 LayoutVisible(viewport, offset) 交付偏移；
SourceView 自己应用正文偏移。不能再把 Arrange 的原点减一次 offset。

没有滚动宿主时，Arrange 使用本身 Size 和零偏移安排可见内容；
不会隐式创建第二套滚动状态或全局滚轮处理器。
滚动条、滚轮物理行为及宿主范围钳制仍属于 GOUI。

Measure 遵循 GOUI 的 Constraint/MinSize/MaxSize 契约；未知视口不测量整个文档。
Arrange 先调用 WidgetBase.Arrange 保存矩形。正文估计范围改变时请求正常布局，
不在 Paint 中递归 Arrange，也不在单帧中为收敛滚动条无限循环。

## 2. 坐标与固定侧栏

设 `g` 为侧栏宽度，`p` 为正文内边距，`sx/sy` 为宿主偏移。
段落原点 `(x,y)` 位于未滚动正文坐标，包含正文 padding：

- 正文绘制点：`(g + x - sx, y - sy)`。
- 正文命中点：`(localX - g + sx, localY + sy)`。
- 行号 X 固定在侧栏内，Y 使用同一段落/显示行的 `y - sy`。
- 正文 clip 为 `[g, viewportWidth)`，侧栏 clip 为 `[0,g)`，两者都限制在控件局部边界。
- 内容宽度为 `g + p + 已知最大正文宽度 + p`，内容高度为段落累计高度加上下 padding。

因此侧栏不水平滚动，正文的可用宽度为 `viewportWidth-g`。
Wrap 开启时排版宽度为 `max(0,viewportWidth-g-2p)`；
宽度为 0 时不发非法排版约束，保留空显示及可靠范围。

为让 caret 不藏到固定侧栏后，正文矩形 `(x,y,w,h)` 的 reveal 请求为
`(x,y,g+w,h)`，交给宿主的未滚动内容坐标协议。
这是把 caret 的可见区从 [0,V] 缩成 [g,V] 的左右不等式推导，不能再额外加 g 到起点。
超宽目标按宿主“优先 leading edge”契约处理。需要真实横向滚动集成用例验证此公式。

## 3. 逻辑行、显示行和高度

一个逻辑行（不含 LF）通常创建一个 TextLayout；由它生成 1 个或多个显示行。
模型 LF 保持逻辑段落边界，软折行只改变视图，不能写回文档。

每个视图持有 HeightIndex，未度量行用基准行高估计；
实际字体 fallback 或 Wrap 可以使行高不同，不能强制与代码字体常量相同。
LayoutVisible 找到覆盖视口的逻辑行，测量可见行和约一页 overscan，更新实际高度。
可见范围重新查询并迭代一次；仍有修正时请求下一次正常布局，不无限追算全文。

HeightIndex 与 Document lineStore 的 splice 对齐；二者不共享同一个可变索引。
重估后保持顶部锚点（LineID、行内 byte、相对 Y），借当前宿主 offset 和锚点的新位置
计算目标视口，再通过 ScrollIntoView 的视口尺寸请求实现恢复。
程序导航有未完成 caret reveal 时优先 caret，避免与顶部锚点请求来回竞争。
若行被删除，锚定到相邻存活逻辑位置；禁止复用已失效行号。

换字体、Wrap 或宽度会使高度估计重建；配色不重建高度。
不折行时水平范围来自已度量行，维护带行身份的已测宽度索引及其有效性：
不能在最长已测行被删除后永久保留旧横向范围，也不为求精确最大宽度每帧排全文。
尚未度量的行可能更宽，因此这是已知下界，不能称为全文件宽度上界；
进入长行后扩大范围，后台分帧工作可以补齐估计，诊断记录中明确范围的可信度。

## 4. 段落缓存与 Geometry

缓存至少记录 LineID/ContentVersion、投影代次、字体/字号/Tab、WrapWidth/模式、
typography Context 身份、TextLayout、度量与 Geometry。
不能以全局 Model.Revision 作为唯一 key，让一个字符编辑摧毁所有未改变段落。

- 文本、字体、Tab、Wrap 或 Context 改变：销毁/替换 native layout，重建度量与 Geometry。
- token/代码配色/选区前景变化：更新范围颜色和栅格状态，保持几何及 HeightIndex。
- 根挂载、窗口或 Context 变化：所有相关 native 资源失效，不能跨 Context 复用。
- 原生度量切片在 Geometry 使用期保持只读且存活；布局销毁前不留下悬垂度量引用。
- 为了可见段落而创建的空行布局失败时，用可靠基准行高形成空几何，错误仍按资源操作语义反馈。

颜色路径保持度量是首版要在 M0 验证的后端前提，而非仅凭 setter 名称推断的事实。
首次应用 token 颜色应在取得最终度量之前；主题/选择颜色变化后如实测度量有变化，
必须失效 Geometry 并核对排版后端契约，不能继续使用旧命中数据或让光标与文字分离。

缓存按 LRU 管理：默认保留约两页逻辑行，最少 128 项；
输入文本存储目标 8 MiB，当前可见集可钉住，超出时记录实际峰值并淘汰不可见项。
原生对象的真实内存不能用输入字符串字节数冒充，验收还要观测进程与后端资源。
首版不另造文字位图缓存，优先让 Painter.DrawTextLayout 复用既有后端缓存。

## 5. Tab、字体与 Cluster

默认请求等宽 family、12 pt。当前 GOUI 的 textFormatFromStyle 直接把 Style.FontSize
作为 typography.FontInfo.Size（pt）使用，SourceView 沿用这一契约，不额外乘 96/72。
实际行高、空格 advance、padding 和视口坐标是 DIP，优先取排版实测度量。
字体 fallback、CJK、组合字符及双向文本始终以原生 shaping 为事实。

Tab 间隔来自 `TabWidth * 基准字体空格 advance`，由 G2 在 native layout 中表达；
模型仍保留一个 U+0009，不扩成可逐空格选择的伪文本。
反缩进按源前导空白处理，正文中的 Tab 视觉导航按 Cluster。

首版 token 不改变 font，避免分析完成后悄悄移动 caret 和折行。
以后允许 token 字重/斜体时，要将度量影响纳入同一失效契约再开放 Scheme 字段。
代码字体的连字行为沿用原生结果；没有 OpenType feature API 就不承诺可关闭连字。

Geometry 用真实 TextLine / TextCluster 重建，使用 Position.Upstream 表示软折行和 bidi
边界的视觉 affinity。上下导航按 preferredX 选最近合法视觉停靠点，
左右导航用 Geometry.Visual，删除用逻辑 Adjacent；两者不能混为按 byte 加减。

## 6. 行号和选区几何

行号以 1 起始，空文档显示 1；末尾 LF 后空行有下一行号。
仅在每个逻辑行首个显示行画一次行号，续行侧栏留空。
宽度按当前最大行号实测、左右 padding 及分隔线计算，位数跨阈值只触发布局一次；
不能依赖“所有数字必等宽”的假设。

跨行选择包含 LF 时，在行尾显示最小换行选择标记；
最后一行不存在 LF 时不额外涂出无限宽背景。
RTL 的同一逻辑选区可能分裂成多个视觉矩形，使用 Geometry.Selection，
不能将 Anchor/Caret 的两个 X 直接连成矩形。

读者隐藏行号时 g=0；改变字号、侧栏宽度或折行后，命中、候选框和 reveal 使用新几何，
不得各自补偿像素差。

## 7. 配色、绘制顺序与主题

`scheme` 提供经过验证的不可变颜色表、内置 Light/Dark 和 token fallback。
颜色以标准非预乘 color.NRGBA 存储，进入 graphics 通过既有转换；
不重复预乘。Token.Kind 未在方案中指定时使用普通前景。

当前配色接口：

```go
package scheme

import (
    "image/color"
    "github.com/golang-gui/goui-sourceview/language"
)

type Definition struct {
    Name string
    Background color.NRGBA
    Foreground color.NRGBA
    GutterBackground color.NRGBA
    GutterForeground color.NRGBA
    CurrentLine color.NRGBA
    SelectionBackground color.NRGBA
    SelectionForeground color.NRGBA
    InactiveSelectionBackground color.NRGBA
    SearchBackground color.NRGBA
    CurrentSearchBackground color.NRGBA
    BracketMatchBackground color.NRGBA
    BracketMatchBorder color.NRGBA
    Caret color.NRGBA
    PreeditUnderline color.NRGBA
    Border color.NRGBA
    Tokens map[language.Kind]color.NRGBA
}
type Scheme struct { /* private */ }
func New(definition Definition) (*Scheme, error)
func Light() *Scheme
func Dark() *Scheme
func (s *Scheme) Definition() Definition
func (s *Scheme) Color(kind language.Kind) color.NRGBA
```

New 拷贝 Tokens；Definition getter 也返回独立 map。基础前景、背景及选择前景必须不透明，
未知 Kind 或不满足基础字段契约返回错误；普通叠加背景允许 alpha。
Light/Dark 返回可安全共享的不可变实例；修改以取得 Definition、修改副本、New 的方式完成。
应用不能改变共享 map 后依赖隐式刷新，也不以 Name 是否相同判断两个方案身份。

内置方案初始颜色如下（均为不透明值，后续视觉调整仍需对应对比与验收）：

| 字段 / Kind | Light | Dark |
| --- | --- | --- |
| Background / Foreground | #ffffff / #24292f | #1e1e1e / #d4d4d4 |
| GutterBackground / GutterForeground | #f6f8fa / #6e7781 | #252526 / #858585 |
| CurrentLine | #f3f6fa | #2a2d2e |
| SelectionBackground / SelectionForeground | #b8d7ff / #111111 | #264f78 / #ffffff |
| InactiveSelectionBackground | #d8dee8 | #3a3d41 |
| SearchBackground / CurrentSearchBackground | #ffe58f / #ffd166 | #5c4b00 / #805e00 |
| BracketMatchBackground / BracketMatchBorder | #e6eef7 / #9aafc4 | #35414d / #667a8d |
| Caret / PreeditUnderline | #24292f | #d4d4d4 |
| Border | #d0d7de | #454545 |
| KindProperty | #005cc5 | #9cdcfe |
| KindString / KindEscape | #0a7f39 / #6f42c1 | #ce9178 / #d7ba7d |
| KindNumber / KindKeyword | #0550ae / #8250df | #b5cea8 / #569cd6 |
| KindDelimiter | #57606a | #d4d4d4 |
| KindTag / KindAttribute | #116329 / #953800 | #569cd6 / #9cdcfe |
| KindEntity / KindMeta | #8250df | #d7ba7d / #c586c0 |
| KindComment / KindInvalid | #6e7781 / #cf222e | #6a9955 / #f44747 |

KindPlain 使用 Foreground。这个颜色表是本项目初始设计，不是对其他编辑器主题文件的复制或兼容声明。

默认 nil Scheme 根据所属 GUI stylesheet 解析代码颜色。Basic 使用 Light；Modern 按
Options.Dark 选择语法色并适配官方正文/选区。SetScheme 只覆盖代码色，nil 恢复样式。
控件不订阅系统设置；应用/声明式层更新完整 stylesheet，StyleChanged 同步颜色和字体。
具体注册、样式名、UI 边界与失效契约见 [样式接入](DesignStyles.md)。

绘制顺序：

1. 编辑区和侧栏背景、边框。
2. 当前行背景。
3. 已确认括号配对的淡背景，再画普通搜索命中、当前搜索命中背景。
4. 选区背景（焦点/非焦点分别有配色）。
5. 括号细边框，再画语法前景；选中范围使用明确的选择前景覆盖，保证可读。
6. 预编辑下划线、行号/分隔线。
7. 活动 caret 与焦点装饰。

括号独立主题色、内缩边框及确认条件见 [括号设计](DesignPairs.md)，不修改语法颜色或排版度量。

每个段落应用颜色前先恢复全范围基础前景，再按 token 与选择覆盖，
否则重新分析后消失的 token 会遗留旧色。只在颜色指纹改变时更新，
不能每帧无条件为所有段落重复设置范围颜色。
未完成高亮区域用普通前景；无高亮也能完整选择和编辑。

Caret 闪烁使用挂载期、焦点期计时器，失焦/隐藏停止；每次编辑/导航重置为可见。
不为闪烁启动独立 goroutine，Root 帧连接只在有实际工作时请求新帧。

## 8. 长文件与超长行边界

常规验收 profile 的规模见 [Testing](Testing.md)。
词法分析可分片，原生 NewTextLayout / MeasureMetrics 仍可能是不可中断的单次调用，
“有 2 ms 高亮预算”不等于“所有 UI 工作受 2 ms 限制”。

首版对可见逻辑行仍采用完整 native paragraph shaping，不在未知上下文处分割
复杂文字或只画前 N 字节。超长行保留完整文本、选择和编辑，但单次排版延迟需要实测；
极端单行不承诺与常规文件相同的延迟。

先在第一轮布局原型验证 16 KiB、64 KiB、1 MiB 单行的输入/导航及原生内存。
若首版常规 profile 也不达标，实施计划必须暂停相关交付并设计窗口化 shaping 等
独立能力，不能通过静默截断内容、丢弃 Cluster 或随意放宽阈值掩盖问题。

## 9. 资源与语义快照

所有 native layouts 在 GUI 线程创建/销毁，Context 由 Application 持有；
视图卸载先释放 layouts，再断开最后的根引用，不销毁共享 Context。
Paint 只借用 Painter；不缓存它，不手工绘制其他 Widget 的子树。

Role 使用可扩展的 `gui.Role("sourceview")`，TextEditingInfo 报告完整 Length、
当前选区、只读状态和有界 VisibleRange/VisibleText；整份文档不重复写入 WidgetInfo.Text。
可见原文摘录上限 64 KiB，向 rune 边界截断；其范围必须与文本逐 byte 对齐。
超长行的可见摘录可围绕当前可见锚点取一个连续区间，不能拼接不连续片段冒充一个 Range。

Preedit 另外报告，不能混进 committed range。
若在模型重入通知中尚未追平修订，按编辑专题暂不发布 TextEditing/Text，避免新旧状态混用。
Attributes 使用 `sourceview.language`、`sourceview.revision`、`sourceview.analysisPending`
等带命名空间的 JSON 值；不暴露 native/Model 对象。
首版只有真实焦点和输入动作，不为行号 token 伪造语义执行入口。

## 10. 着色失效与排版失效分离

段落按文本身份、字体、Tab、折行宽度和 Context 管理原生布局。
独立记录方案身份、语言、token 版本/有效性、段内选区和活动焦点组成的着色状态。
状态未变不调用颜色 setter；纯颜色变化不重建 TextLayout、Geometry、HeightIndex 或正文宽度。
StyleChanged 只在实际字体配置变化时失效度量；禁用引起的输入/预编辑取消仍按原有行为执行。

重着色先用普通前景覆盖全文，再应用当前有效 token，最后覆盖活动选区前景。
行号布局独立更新前景；旧 token、撤销的选区和旧方案不能残留。
全文颜色覆盖必须真正替换旧范围记录，不能在长寿命布局中永久累积颜色操作历史。
GOUI DirectWrite 的完整覆盖回收修正是本实现保持范围记录有界的前置条件。

初始高亮元数据完成后，同一修订/行号的临时段落缓存转接到稳定 LineID，不重新 shaping。
语言切换保留已完成的文本元数据，取消旧扫描工作并使全部词法结果失效；不重建文本布局。
若索引尚未就绪仍可重新安排元数据构建，正文先保持普通前景。
模型 Reset、文本变化、字体/Tab/折行/Context 变化仍走正常排版失效。

每次原生颜色调用返回后核对文档、修订、选区、挂载代次、布局代次、缓存所有者和着色状态；
重入销毁/换文档/换方案时不继续使用旧对象。缓存先移除所有权，再销毁原生资源。
原生范围样式只改变前景是本切片经实测验证的条件，不推广为任意字体属性变化都能复用几何。

参考与取舍：

- [GtkSource Style](https://gnome.pages.gitlab.gnome.org/gtksourceview/gtksourceview5/class.Style.html)
  （官方文档 API 5，页面 Library 5.21.1）分别描述 foreground、bold、italic、scale 等属性；
  SourceView 当前仅开放 token 前景，不照搬 GtkTextTag 或 token 字体机制。
- [Scintilla 样式 API](https://www.scintilla.org/ScintillaDoc.html#SCI_STYLESETFORE)
  （页面编辑日期 2026-07-22）将前景与字体样式分开设置；借鉴其职责区分，不引入消息 API 或固定行高假设。
- [VSCode 1.105.0 viewModelImpl](https://github.com/microsoft/vscode/blob/1.105.0/src/vs/editor/common/viewModel/viewModelImpl.ts#L145)
  中 token color 与 theme 使用专门的 view 事件，主题处理还会失效装饰颜色缓存。
  这说明该实现区分颜色事件；不据此声称 Monaco 所有颜色更新均不布局。
- 本模块的原生几何复用结论来自 GOUI 三后端源码核对与测试，具体样本和边界见
  [着色缓存验收](PlanColorCaching.md)，不从上述参考框架推断 GOUI 的线程、资源或度量行为。
