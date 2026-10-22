# SourceView 布局、滚动与呈现设计

> 状态：首版实现规格。独立 SourceView 叶子控件复用 typography 与文本几何，
> 不使用 TextView 的布局或私有缓存。原生 Tab 配置依赖 [G2](DesignGOUIIntegration.md)。

## 长段落片段布局目标（2026-10-20 修订）

Reflow 及其接入已经通过 Git 回退。TextLayout 保持文字、格式、约束输入以及
测量／渲染输出；片段范围、测量值缓存、可视区布局、调度和失效由上层负责。
同一输入的度量副本可以脱离原生布局保存，不要求 typography 参与缓存运算。
共用设计见 GOUI 的
[文本片段测量与布局](../../goui/doc/DesignIncrementalTextLayout.md)，本仓接入见第 11 节。
这是相邻检出下的开发设计，不是已发布版本能力。

先有限样本估计范围，再测当前几屏，保存范围及只读度量，按事件循环分步补齐。
SourceView 继续独立管理语法、折叠投影和装饰；不等待后端增加可恢复分析接口
才能验证片段缓存。边界上下文正确性单独验证，未确认片段不能伪装成准确几何，
也不退回同步整段。允许文字和着色延迟，Paint、命中与快照不强制补齐整段。
同步 SetSelection 的 cluster 校正需明确处理，不能隐藏阻塞回退。

当前已接入 `textedit.Geometry.LineAtY`／`SelectionLines`，选择、搜索、词出现
及预编辑装饰只为可视行及相邻边界候选生成矩形。保留完整矩形高度，避免因
提前裁剪而改变预编辑下划线位置；最终像素裁剪仍由 Painter 完成。
短范围的配对装饰也受益于 Geometry 的逻辑行索引。

2026-10-21 已接入普通值 Fragment：保存独立度量快照，超过 32 个有序显示行时
按需创建详细几何，每个 Geometry 最多缓存 4 页。原生布局释放后快照仍可查询。
这仍只优化已取得的度量。2026-10-21 的视口调度进一步把已挂载视图的缺失段落
交给帧回调逐个准备，见第 12 节；每次 native 调用仍测完整逻辑段落，单条极长
段落的调用无法中断，不能宣称已解决其卡顿。最近两个有效宽度的完整布局可直接恢复。
原型、依赖与验证见
[长段落实施记录](PlanIncrementalTextLayout.md)。

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

当前实现一个逻辑行（不含 LF）通常创建一个 TextLayout，生成 1 个或多个显示行；
目标片段方案将一次测量范围与逻辑行解耦，见第 11 节。
模型 LF 保持逻辑段落边界，软折行只改变视图，不能写回文档。

每个视图持有 HeightIndex，未度量行用基准行高估计；
实际字体 fallback 或 Wrap 可以使行高不同，不能强制与代码字体常量相同。
LayoutVisible 查询覆盖视口的逻辑行及其后一个逻辑行；已挂载视图把缺失结果交给
帧回调测量，未挂载的离屏调用保留同步行为。准备完成后更新实际高度并请求正常布局。
可见范围重新查询并迭代一次；仍有修正时请求下一次正常布局，不无限追算全文。

HeightIndex 与 Document lineStore 的 splice 对齐；二者不共享同一个可变索引。
重估后保持顶部锚点（LineID、行内 byte、相对 Y），借当前宿主 offset 和锚点的新位置
计算目标视口，再通过 ScrollIntoView 的视口尺寸请求实现恢复。
程序导航有未完成 caret reveal 时优先 caret，避免与顶部锚点请求来回竞争。
若行被删除，锚定到相邻存活逻辑位置；禁止复用已失效行号。

Caret reveal 保留当前文档、修订、选区代次和挂载代次对应的逻辑请求，而不是只发送一次
基于估计高度的矩形。每次可见区布局先测量覆盖区域，再查询一次覆盖范围；仍有变化时
交给下一次正常布局，不预排全文。用更新后的 HeightIndex 和原生 caret 几何重新请求最小滚动。
发送请求前，额外测量从目标前约一个估计视口至目标后一行的段落，避免先滚动再补测目标上方行
而把光标再次推到视口外；基准行高是实际段落高度的下界，因此该范围能覆盖目标下边缘定位需要的行。
只有收到宿主 `LayoutVisible` 的当前偏移、覆盖区域已经测量且目标可见时才完成请求，
然后重新捕获顶部锚点。`Arrange` 使用的保留偏移仅供临时布局，不能确认请求完成：
重挂载时宿主偏移可能已归零，而控件仍保存上次偏移。

零视口或尚无 typography 时等待首次有效分配；首次挂载可接续未失效的请求。
新选区（包括指针选择）、文档修改/替换及卸载取消旧请求；原生调用和滚动信号返回后
核对请求身份、修订和生命周期，不能覆盖回调创建的新请求。请求完成后的普通滚动不追随 caret，
没有未完成请求的折行、字体和主题操作仍保持顶部锚点。目标大于视口时按 ScrollView
优先 leading edge 的契约完成，不为无法容纳的完整矩形持续请求布局。

参考与取舍（2026-10-18 核对）：

- [GTK 4.20.2 gtktextview.c](https://github.com/GNOME/gtk/blob/4.20.2/gtk/gtktextview.c#L2760)
  的 queue/flush scroll 保留 mark，flush 时先验证目标附近的布局并更新 adjustments；
  源码明确说明后续验证可能把目标推到屏幕底部之外。借鉴逻辑请求和先验证布局的顺序，
  复用 GOUI 的事件循环与 ScrollIntoView，不引入 GTK mark/adjustment 对象。
- [VS Code 1.105.0 viewLines.ts](https://github.com/microsoft/vscode/blob/1.105.0/src/vs/editor/browser/viewParts/viewLines/viewLines.ts#L566)
  保留横向 reveal 请求，在目标行渲染后计算水平位置；这是该版本源码的横向行为，
  不能据此声称 Monaco 使用本项目的可变行高策略。采用“几何可用后完成”的原则，不引入 DOM 或动画调度。
- [Scintilla SCI_SCROLLCARET](https://www.scintilla.org/ScintillaDoc.html#SCI_SCROLLCARET)
  文档承诺需要时滚动至当前光标；采用最小可见性目标，不增加 caret policy 配置矩阵。

以上是参考源码/文档；本项目的旧版复现和本轮实测结果另见 [中大文件验证](PlanLargeFiles.md)。

换字体或 Wrap 会使高度估计重建；配色不重建高度。仅宽度／侧栏变化保留旧高度作为
估计，当前宽度的结果逐步替换，不能用旧高度宣称准确折行。避免把全部高度立即压成
单行、隐藏滚动条、改变有效宽度，再显示滚动条而触发第二次排版。
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
- 仅宽度变化：以有效正文宽度区分完整布局，恢复仍在缓存中的宽度时复用其原生对象、
  度量快照和几何。新宽度不修改旧对象，不把旧宽度坐标作为新结果。
- 根挂载、窗口或 Context 变化：所有相关 native 资源失效，不能跨 Context 复用。
- 上层复制度量切片及其嵌套 Clusters，副本在 Geometry 使用期保持只读且存活；
  副本不依赖原生布局寿命，淘汰布局不必丢弃测量值。
- 为了可见段落而创建的空行布局失败时，用可靠基准行高形成空几何，错误仍按资源操作语义反馈。

颜色路径保持度量是首版要在 M0 验证的后端前提，而非仅凭 setter 名称推断的事实。
首次应用 token 颜色应在取得最终度量之前；主题/选择颜色变化后如实测度量有变化，
必须失效 Geometry 并核对排版后端契约，不能继续使用旧命中数据或让光标与文字分离。

当前缓存按 LRU 管理：每个文本身份最多保留两个有效宽度；全视图以 128 项及各项
输入文本字节之和 8 MiB 为软界限。当前访问项、当前宽度的可见区域、未完成 reveal
邻域和顶部锚点结果保留，允许工作集超限；否则高视口超过 128 行时会反复淘汰尚未
展示的结果。缩小／离开后恢复预算淘汰，已编辑旧身份不能因行号相同而继续保留。
不是为每个宽度分别分配 128 项或 8 MiB。持续拖宽淘汰同文本的
最旧宽度，再应用全视图界限。字体、Tab、Wrap 模式、文档重置、预编辑投影、挂载及
Context 变化仍释放全部旧输入资源；局部编辑继续借助稳定 LineID 保留未变后缀。
宽度使用实际送测的 float32 值，不取整合并不同约束；不换行固定为原有测量约束。
侧栏变化按扣除后的宽度区分。Snapshot 只读当前宽度，不能命中另一宽度的坐标。
返回旧宽度时仍核对 token／主题／选区前景指纹，按需更新颜色，不恢复过期着色。

淘汰先移除缓存所有权，再释放原生对象。销毁回调切换文档或销毁控件时，终止旧调用，
不发布已释放的段落。不同视图仍拥有独立缓存，不共享可变 TextLayout。
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

New 拷贝 Tokens；Definition getter 也返回独立 map。基础前景、背景必须不透明，
未知 Kind 或不满足基础字段契约返回错误；普通叠加背景允许 alpha。
SelectionForeground 的 alpha 为 0 表示保留语法前景，并规范化为零值；255 表示活动选区的
显式统一前景。1–254 返回错误；不会把透明文字传给原生排版。
Light/Dark 返回可安全共享的不可变实例；修改以取得 Definition、修改副本、New 的方式完成。
应用不能改变共享 map 后依赖隐式刷新，也不以 Name 是否相同判断两个方案身份。

内置方案颜色如下（六位色值不透明，八位色值为 RRGGBBAA）：

| 字段 / Kind | Light | Dark |
| --- | --- | --- |
| Background / Foreground | #ffffff / #24292f | #1e1e1e / #d4d4d4 |
| GutterBackground / GutterForeground | #f6f8fa / #6e7781 | #252526 / #858585 |
| CurrentLine | #f3f6fa | #2a2d2e |
| SelectionBackground / SelectionForeground | #4682dc40 / 保留语法 | #79bbff40 / 保留语法 |
| InactiveSelectionBackground | #4682dc20 | #79bbff20 |
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
Options.Dark 选择语法色并适配官方正文，以主题焦点色生成真实半透明选区。SetScheme 只覆盖代码色，nil 恢复样式。
控件不订阅系统设置；应用/声明式层更新完整 stylesheet，StyleChanged 同步颜色和字体。
具体注册、样式名、UI 边界与失效契约见 [样式接入](DesignStyles.md)。

绘制顺序：

1. 编辑区和侧栏背景、边框。
2. 当前行背景。
3. 已确认括号配对的淡背景，再画普通搜索命中、当前搜索命中背景。
4. 选区背景（活动 alpha=64、非活动 alpha=32），按既有预乘颜色契约叠加到下层。
5. 括号细边框，再画语法前景；只有显式配置不透明 SelectionForeground 时才覆盖活动选区。
6. 预编辑下划线、行号/分隔线。
7. 活动 caret 与焦点装饰。

括号独立主题色、内缩边框及确认条件见 [括号设计](DesignPairs.md)，不修改语法颜色或排版度量。

段落保留已应用的基础前景及规范化的有效颜色范围：按 token 和显式选区前景的
叠加结果计算，合并相邻同色范围，省略等于基础前景的范围。分析版本、语言标识或
token 分段变化而最终颜色相同时，只更新样式身份，不调用 native SetTextColor。
新布局已从 TextFormat 取得基础色，不再重复覆盖整段。实际颜色改变时，必要的
全范围基础色覆盖清除旧 token/选区，再应用新范围，防止遗留颜色并约束后端属性
数量。范围缓存归属该段落，与 native 资源一同淘汰；各视图和近期宽度不共享可变
状态。颜色更新仍检查生命周期重入，不改变测量几何。
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

编辑通知按逻辑行 splice 标记被替换的旧段落，并映射未变段落的当前行号。
新行身份缺失测量时，可在现有、有界的视图缓存中寻找“已被替换、完整文本逐字节
相同、测量宽度相同”的段落，转移其唯一资源所有权到新身份，复用尺寸、Fragment
和 Geometry。退格恢复原文、撤销及重做因而可避免重复原生测量；不限定字符集。
字体、Tab、WrapMode、Context 或文档变化仍清空这些资源。词法状态可能不同，
复用时必须重新确认当前有效颜色，不能把旧 token 版本相等当成旧颜色有效。

不从仍有效的另一段落借用 native layout，不在两个当前身份下保留同一个可变对象。
旧/新文本和各宽度共同受现有 128 项、8 MiB 源字节的 LRU 约束，可见工作集依既有
规则保护；这是近期值复用，不承诺整个撤销历史常驻，也不缓存共享文档级原生对象。
缓存命中仍检查重入和销毁状态；初次遇到新文本/新宽度的原生排版没有因此变成有界。

文字位图由 GOUI Painter 按可见像素区域生成，不能为完整超长段落分配原生 surface，
也不能先生成全文位图再裁剪。`Rasterize(scale, region, buf)` 的 region 位于原布局的
光栅像素平面，返回尺寸和位置严格由 region 确定，不增加 TextBitmap.Origin。
Painter 对整个布局原点只取整一次，分块光栅化后拼接为单张可见区域图片，避免块边缘
合成接缝；SourceView 继续只调用正常 DrawTextLayout，不复制后端裁剪或字形处理。
这些能力需要 GOUI v0.2.0 之后的区域光栅化接口；已发布 v0.2.0 不能编译本轮更新的
直接 Rasterize 使用者。暂以未提交的双仓工作区联调，不修改发行版本号冒充已发布。
通用契约及限制见相邻 GOUI [区域光栅化设计](../../goui/doc/DesignTextRasterization.md)，
本仓场景及结果见 [长行验收](PlanTextRasterization.md)。区域光栅化不保证 shaping、
语法着色或 Cluster 几何查询的耗时有界。

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
超长行的可见摘录使用已缓存段落几何定位视口左上文字，在其前方最多保留 1024 byte
上下文，截取一个不超过 64 KiB 的连续区间，起止向合法 UTF-8 边界收敛。只读取缓存，
不为 Snapshot 创建原生布局或改变选区、视口；缓存尚未就绪时保留行首降级。
不能拼接不连续片段冒充一个 Range。

Preedit 另外报告，不能混进 committed range。
若在模型重入通知中尚未追平修订，按编辑专题暂不发布 TextEditing/Text，避免新旧状态混用。
Attributes 使用 `sourceview.language`、`sourceview.revision`、`sourceview.analysisPending`
等带命名空间的 JSON 值；不暴露 native/Model 对象。
首版只有真实焦点和输入动作，不为行号 token 伪造语义执行入口。

## 10. 着色失效与排版失效分离

段落按文本身份、字体、Tab、折行宽度和 Context 管理原生布局。
独立记录方案身份、语言 ID/定义代次和 token 版本/有效性。仅显式前景覆盖实际活动时，
将段内选区加入原生着色身份；默认选区移动和焦点变化只重绘背景，不重复设置文字颜色。
状态未变不调用颜色 setter；纯颜色变化不重建 TextLayout、Geometry、HeightIndex 或正文宽度。
StyleChanged 只在实际字体配置变化时失效度量；禁用引起的输入/预编辑取消仍按原有行为执行。

重着色先用普通前景覆盖全文，再应用当前有效 token，最后按需覆盖显式活动选区前景。
行号布局独立更新前景；旧 token、撤销的选区和旧方案不能残留。
全文颜色覆盖必须真正替换旧范围记录，不能在长寿命布局中永久累积颜色操作历史。
GOUI DirectWrite 的完整覆盖回收修正是本实现保持范围记录有界的前置条件。

初始高亮元数据完成后，同一修订/行号的临时段落缓存转接到稳定 LineID，不重新 shaping。
语言切换保留已完成的文本元数据，取消旧扫描工作并使全部词法结果失效；不重建文本布局。
同 ID 更换定义也更新颜色身份；尤其切为无扫描器定义时，即使 token 版本未变，仍清除旧色。
若索引尚未就绪仍可重新安排元数据构建，正文先保持普通前景。
模型 Reset、文本变化、字体/Tab/折行/Context 变化仍走正常排版失效。

真实活动焦点独立于规范化后的着色身份捕获；原生布局创建与颜色调用返回后仍核对
实际焦点、文档、修订、选区、挂载代次、布局代次、缓存所有者和着色状态；
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

## 11. 片段缓存接入规格（实施中）

### 11.1 身份和映射

每个片段记录稳定 LineID／内容版本、模型范围、投影范围、送测上下文及其映射，
再加字体／Tab／宽度等度量键。跨行编辑按实际 Edit 顺序更新，不能用全局 revision
清空所有片段。revision 用于拒绝旧任务；邻近上下文改变时需使受影响片段失效。
不能只以 LineID 不变判定边界状态不变。

折叠只改变可见投影。隐藏片段不主动测量；展开恢复仍有效的测量，未就绪部分
加入正常需求队列。IME 片段增加预编辑投影代次，取消或提交后释放该投影资源。

### 11.2 度量、布局和绘制分开

当前复用 GOUI 公开的 NewFragment／Fragment.Geometry，不复用 TextView 私有缓存。
完整度量复制后，详细几何按需分页；范围测量、视口片段调度及增量后缀仍待实现。
本仓只持有属于本视图的测量副本、布局索引、原生对象和 GUI 需求状态。
一屏由多个有限片段组成，不为一个超长逻辑行构造完整 Geometry。

已测片段在相同有效宽度下直接复用。测量值保留时间可长于原生对象，滚回时
仅为可见部分重建小 TextLayout。详细几何和原生资源先覆盖当前屏、上下各一屏，
加 caret／IME 目标；上层几何缓存按值预算管理，原生资源按覆盖及对象数管理。

token、主题和选区只影响颜色时，不清空度量／折行。颜色范围与片段模型范围求交
后转为布局局部 byte offset，再调用现有范围颜色接口。不能为着色拿整条长行
重新创建布局；实际后端发生的可见重绘成本需单独测量。

### 11.3 可视区、侧栏和装饰

LayoutVisible 只登记最新视口／偏移及工作范围，帧回调逐个准备有限片段。高度
估计先响应滚动，准确行组完成后连同几何发布，保持 LineID／文本位置锚点。
换宽复用值缓存和旧折点提示，优先当前区域，不等待整个逻辑行完成。

模型逻辑行首对应的显示行才绘制行号；片段起点不产生重复行号。折叠标记及固定
侧栏沿用现有坐标。选择、搜索、出现词、配对及预编辑装饰先与已发布片段求交，
再用片段 Geometry 生成局部矩形；不能为一个小装饰建立整段几何。

点击已显示文字只查询片段索引及局部 Geometry。未显示区域的交互请求保留
修订／布局代次，后续输入或滚动取消旧请求；不能显示估计 caret 或迟到地覆盖
新选择。复制与搜索使用模型文本，不为了取得内容而测量。

### 11.4 验证和依赖

先验证有限样本估计、值缓存、同宽滚回、换宽行组发布、局部编辑和后缀复用；
再验证复杂文字边界。完整正确性包含 bidi／isolate、上下文形式、连字、Tab、
组合字和 emoji；固定重叠不视为证明。已知 RLO 失败样本保持验收门槛。

第一阶段不新增 typography 接口。NewFragment 的本地接入依赖未发布 GOUI 提交，
不修改用户已有依赖配置。本节其余条款仍为目标，没有把提案当作已发布能力，也不要求 GOUI
反向依赖本模块。原生／窗口测试在两仓各自组织，未完成排版时必须仍可操作窗口。

选区策略参考：

- [Scintilla Selection styles](https://www.scintilla.org/ScintillaDoc.html#SCI_SETSELFORE)
  （2026-10-17 查阅）明确默认保留原前景，并支持文字下方透明背景；采用该默认，复用 GOUI 绘制顺序。
- [Monaco/VS Code 1.105.0 编辑器颜色](https://github.com/microsoft/vscode/blob/1.105.0/src/vs/platform/theme/common/colors/editorColors.ts)
  将普通主题的 selectionForeground 设为 null，高对比度主题可给出具体色；本项目用现有 NRGBA alpha 表达可选覆盖。
- [GtkSourceView 5 Style Scheme](https://gnome.pages.gitlab.gnome.org/gtksourceview/gtksourceview5/style-reference.html)
  分别定义 selection 与 selection-unfocused；沿用活动/非活动背景区分，未增加新的模式体系。

### 11.5 上下文携带实验的边界

测试原型在每个有限候选前注入零宽方向标记和仍活动的显式 embedding／override／
isolate 控制符；导出时显式移除注入前缀的 byte offset，不删除模型中的控制字符。
注入前缀也计入 512／2048 字节送测上限。前缀占宽、与正文合成 cluster 或单独成行时
拒绝映射，不强行裁剪。FSI 和深度溢出尚未实现，只在实验中明确拒绝，不用于控件。

Pango 1.50.6 下，长 RLO／LRO／RLI 和混合 Unicode 样本的完整行及 cluster 可与整段
一致，但长中性序列、数字和跨窗口括号仍产生不同坐标或方向。反例已成为独立回归，
不能据前几类成功样本将该实验接入正式路径。原有无上下文 RLO 反例仍保留。
方向属性来自已有 x/text v0.22.0（Unicode 15.0 表）；不据此声称与三个原生后端的
Unicode 版本或所有行为一致。扫描段落基础方向的实验也未实现 GUI 分时扫描。

依据 [Unicode UAX #9 rev.52 的 W／N／N0 规则](https://www.unicode.org/reports/tr9/tr9-52.html#Resolving_Neutral_Types)
和[Pango 的段落上下文契约](https://docs.gtk.org/Pango/func.shape_full.html)，活动控制符只是
显式状态的一部分。后续上层方案须处理隐式方向、括号依赖及上下文塑形，准备工作本身
也要分步；不能仅增加固定重叠、反复扩大窗口或同步整段回退来掩盖反例。本节记录
实验限制，不是新增 typography 能力或完整 bidi 支持的承诺。

## 12. 当前视口的段落分帧准备

这是完整段落路径的改善，不把第 11 节的通用上下文片段测量写成已完成能力。
`TextLayout` 仍是同步基础原语，不新增调度接口，不在后台线程测量，也不递归泵送事件。

挂载后，`LayoutVisible`、正文 Paint、caret Paint、IME 位置更新和 Snapshot 不因缓存
缺失而测量完整正文。缺失正文暂不显示；颜色滞后时可暂用同一几何的旧颜色并排队更新。
小字号探针、行号仍沿用既有原生路径。词出现边界只读至多相邻四个 UTF-8 字节，不能
为已滚出屏外的选区调用 cluster 导航；默认关闭的空白装饰不扫描整条段落。

复用每个 Root 的 ConnectFrame：Root 共享约 4 ms，每视图约 2 ms；每次开始下一段
之前检查期限，超时请求下一帧。视图按轮转顺序处理，随后用余量推进共享文档的词法工作。
这只是批次软预算，不能截断一次 native shaping／栅格化，不能承诺每帧不超过 4 ms。

后续大 XML 验证把纯分析从重绘驱动中分离：同一个 frameQueue 的普通 GUI Timer
每 8 ms 推进约 2 ms，只在可见词法结果改变时绘制；帧回调负责正文准备。上述
“帧余量推进”只保留为 Timer 不可用时的后备。具体所有权见 [高亮设计](DesignHighlighting.md)。

只保存“仍有工作”的状态，下一步读取最新宽度、文档和视口；连续缩放／滚动不会累积
已经过时的测量队列。优先顶部锚点和 reveal 目标及其邻域，再准备实际可见区域；这两片
范围分别处理，不能把一次首尾跳转变成扫描整个中间文档，也不能遗漏目标下方可见行。
高度修正最多重查两轮覆盖范围，后续交给下一帧。原生调用返回后核对文档、修订、挂载、
布局代次与销毁状态，失效结果释放后丢弃。

自动创建失败按输入身份保留有界失败缓存，避免同一保留项每帧重报；显式编辑请求可重试，
新输入身份也可重试。卸载、重置及布局资源整体失效时取消工作并释放缓存。
Snapshot 的 `goui.sourceview.layoutPending` 描述尚有视口准备任务；不增加语义动作，
读取快照不测量、不更新颜色。显式 SetSelection、命中及编辑导航仍要求准确 cluster，
冷的极长单段落因此仍可能同步阻塞；后续需要满足上下文正确性的范围测量方案。

参考核对（2026-10-21）：[Scintilla Editor.cxx，提交 c4f1619](https://github.com/mirror/scintilla/blob/c4f161912f4afff81b0697f52f78ad7f0620ac25/src/Editor.cxx#L1508)
分别处理 visible／idle wrapping，Idle 推进尚未完成的折行；
[布局缓存文档](https://www.scintilla.org/ScintillaDoc.html#SCI_SETLAYOUTCACHE)
列出 visible 加 caret 的 page 缓存。采用可见工作集与事件间隔推进原则，复用 GOUI 的
Root 帧事件；不引入其消息 API、全文件预排或多线程测量。与 Monaco 的行身份及测量
复用取舍见第 3、10 节和 GOUI 的 TextEditing 设计。实测与输入来源见
[真实文档验证记录](PlanCorpusLayout.md)。

## 13. 独立 advance 索引与原生工作边界（2026-10-22）

后续宽度缓存分为已塑形测量、折点／行组、详细编辑几何和绘制资源。字体和实际
上下文未变时，换宽保留前者；仅新建换行迭代，逐个确认当前锚点和视口的显示行。
局部编辑从受影响的塑形／方向上下文边界失效，后缀只有在内容及入口状态均收敛后
才能复用。估计及二分只用于选候选，不能成为最终 caret／选择坐标。

GOUI 新增 `textedit.WrapMeasurements`／`WrapIterator` 的普通值索引，支持前缀
advance、Unicode 字素／换行机会、独立迭代与新行 Tab 原点。这是供本控件接入的
基础算法，**当前 SourceView 尚未使用它替代原生整段折行**。完整度量的构建仍是
前置，不能把快速重算折点的算法基准解释成首屏或真实输入耗时。

GOUI 的 Pango／DirectWrite 批量度量导出已直接用于原来的布局路径；Pango 的区域
栅格化只遍历 ink 与 region 相交的原生行。这些改动不改变控件语义、模型或线程；
仍保留原生完整 shaping／bidi，CoreText 当前已有 run 批量导出，无须为统一而改写。
有限范围塑形的 Pango／CoreText 验证仍为实验，尚无三后端公共范围布局契约；不复制该原生
实现到本模块，不引入 GOUI 私有依赖，也不重新引入 typography Reflow／调度。

详细算法、原生事实与参考版本见
[GOUI 文本片段设计](../../goui/doc/DesignIncrementalTextLayout.md#14-度量索引与换行分离2026-10-22)。
有限工作单元、事件循环推进和完整 Unicode 正确性继续是验收目标；当前冷的超长单行
仍可能阻塞，不能将这一批度量／渲染改进宣称为通用分帧排版已经完成。


普通边界的视觉导航按 GOUI Geometry 的 float32 表示误差处理，不复制其内部
停靠点算法。本轮原生回归覆盖三后端和大坐标不换行文本，真实 Shift+Left／删除
仍走正常事件。与同 byte 的舍入边缘不同，bidi／软折行 affinity 必须保留。
本轮结果与剩余缺口见 [测量优化验证](PlanMeasuredTextLayout.md)。

## 14. 初始高度估计与局部编辑的滚动范围（2026-10-22）

对超过 4096 byte 的单逻辑段落，首次自然尺寸查询用至多 512 byte 的 UTF-8 前缀，
按当前字体、Tab、换行模式和候选宽度估计总高度。样本立即释放，每次高度索引重置
后最多一次。目的在于让 ScrollView 在完整段落测量前保留滚动条空间，减少在两个
分配宽度重复折行。显式不换行和 IME 投影不参与此估计。

该结果只进入暂定高度，不能进入 Fragment、Geometry、caret、选择、命中或绘制。
前缀分布可能与后文不同，甚至很多 byte 没有 advance；完整段落的原生度量始终覆盖
估计，不改变 bidi、连字或组合字符的最终结果，也必须能移除不必要的滚动条。
不通过扫描整段来寻找样本边界，不把“可估计”扩大为“可独立塑形”。

无折叠映射且一次 splice 以一个逻辑段落替换一个段落时，保留它原有的确切高度作为
新输入的估计；几何和原生布局仍按新文本身份失效。待测量期间若退回一行高，会让
滚动条消失并产生中间宽度。多段增删继续更新索引，真实短文本测量后正常缩小范围。

本模块继续通过公开 GOUI 契约测量、排版和滚动，没有引入新平台接口。GOUI 同期
为 TextView 的长段落复用少量近期完整宽度，本模块已有的每文本两个宽度缓存不扩大。
两者都是消除重复工作，尚未完成有限行组、跨宽度 advance 复用或局部编辑后缀收敛。
参考原则、缓存所有权及限制见
[GOUI 分配设计](../../goui/doc/DesignIncrementalTextLayout.md#15-避免重复分配宽度与保留近期结果2026-10-22)。
本批真实操作和失败项见 [分配验证](PlanTextLayoutAllocation.md)。

## 15. 完整上下文字形的折行投影

本阶段接入 GOUI 的可选字形测量／范围绘制能力：长逻辑段落保留完整来源，上层
复用 advance、原生视觉顺序及普通值折点；按可见显示行准备详细几何并绘制相应字形。
语法颜色留在普通值范围中，避免每次着色重新塑形完整段落。不同宽度共享不可变来源，
不同视图独立拥有；原生来源与图像缓存明确释放。设计和阶段边界见
[GOUI 上下文字形设计](../../goui/doc/DesignShapedWrapping.md)。

至少 32 KiB 的自动换行段落在 Pango 路径接入，其他后端保持既有准确行为。
SourceView 的着色范围在绘制时按原生 cluster 首 byte 取色，不在连字中间拆字形，
不因 token／选区前景变化反复设置原生属性。位图范围还保留小数像素相位和字体的
墨迹外伸，避免颜色边界的错位与裁切。
首次完整塑形、索引构建和编辑后缀
收敛仍需继续限量，不把有限范围绘制解释成所有冷调用已不再阻塞。验证以真实新文本
编辑和陌生宽度拖动为主，复杂 Unicode、行尾空白及完整恢复均需独立预期。
