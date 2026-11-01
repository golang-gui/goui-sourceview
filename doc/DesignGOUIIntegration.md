# GOUI 接入与最小扩展设计

> 当前源码接入基线：GOUI v0.5.0（`14b394d94227cc0a9d5a9bc9a95e8a711330b333`）；
> 四边 Padding、局部选区绑定和文件对话框适配见第 9–10 节。
> 以下较早的版本、提案及验收记录按各自日期保留，不代表当前最低依赖。

> 实施状态：G1/G2/G3 及后续所需扩展已随 GOUI v0.2.0 发布，本模块直接声明该版本；下文保留设计与验收门槛，实际签名、范围和验证见 [Implementation](Implementation.md)。
> 下文第 1–5 节保留初始提案的事实与拟新增契约；实施结果以当前公开接口及 Implementation 为准，样式注册见 [本轮验收](PlanStyles.md)。
> 初始设计的历史基线：GOUI HEAD `4167bb79823efffbea43e00558260448959536b0`，2026-10-16；
> 检查时工作树的 Plan / Roadmap 已有修改，源码结论来自所读取文件，不代替已发布版本承诺。

## 1. 已有公开能力

| 源码 | 已核对事实 | 使用方式 |
| --- | --- | --- |
| [textedit/doc.go](../../goui/gui/textedit/doc.go)、[model.go](../../goui/gui/textedit/model.go) | Model、Range、Edit、Change、修订、单范围 Apply、EditSource、历史选区；顺序访问与同步可重入通知 | 复用存储/历史；按每项 Edit 的实际顺序解释变更 |
| [projection.go](../../goui/gui/textedit/projection.go) | 预编辑投影依赖同一未变模型，不自动订阅 | 任何模型变化都丢弃当前投影 |
| [geometry.go](../../goui/gui/textedit/geometry.go)、[heights.go](../../goui/gui/textedit/heights.go) | Geometry 消费行/Cluster 度量，HeightIndex 支持段落高度与累计查询 | 视图持有，不共享原生排版 |
| [typography/layout.go](../../goui/platform/typography/layout.go) | 范围颜色、字体及度量已公开；Text 文本固定；没有 Tab 间隔设置 | 可独立完成源码着色；可配置 Tab 是真实缺口 |
| [im_context.go](../../goui/gui/im_context.go) | IMClient、提交/预编辑信号、控件局部候选框矩形 | 独立控件直接接入输入法 |
| [scroll_view.go](../../goui/gui/scroll_view.go) | Scrollable 获得视口和 offset；虚拟内容本身安排为视口大小，自己应用偏移；支持可见请求 | SourceView 自绘行号与正文，外部组合 ScrollView |
| [info.go](../../goui/gui/info.go) | 开放 Role、Attributes 和带范围的 TextEditingInfo | 无需新增源码控件的内核角色枚举 |
| [widget.go](../../goui/gui/widget.go) | mount/unmount、Destroyed、失效与树清理；脱离树后销毁不保证再发 unmount | 只在挂载期持有外部订阅和原生资源，不等待不存在的公开 Destroy hook |
| [clipboard.go](../../goui/gui/clipboard.go) | SetText 不返回错误；RequestText 可同步回调，bool 不能区分无文本与失败 | 不能据当前 API 保证“剪切先复制成功再删除” |
| [platform common Clipboard](../../goui/platform/common/clipboard.go) | 原生 SetText 已返回 error，但读取回调仍为 bool | 写错误可向 GUI 透传；读错误区分需要同步演进平台契约与各后端 |
| [root.go](../../goui/gui/root.go)、[settings.go](../../goui/gui/settings.go) | ConnectFrame / RequestPaint；GUI 设置快照与变化信号 | 分帧驱动；界面设置由应用主题统一消费，无控件原生轮询 |

## 2. G1：原子批量编辑与历史选区

### 2.1 需求与最小范围

选中多行缩进、反缩进和 ReplaceAll 需要多个不相邻改动成为一个撤销单元，
并恢复命令指定的编辑后选区。现有 Apply 只有单个 Edit，历史 after-selection 固定为
插入文本末尾。循环 Apply 或给每次修改加 Atomic 都不满足“一次命令一次撤销”；
用覆盖全部中间文本的单个 Replace 会扩大复制与分析失效范围。

拟在 `gui/textedit` 新增以下通用能力；这是提案，不能直接当作现有 API 调用：

```go
package textedit

type BatchOptions struct {
    Source *EditSource
    BeforeSelection *Selection
    AfterSelection *Selection
}

func (m *Model) ApplyBatch(edits []Edit, options BatchOptions) error
```

### 2.2 位置、原子性与通知

- 所有输入范围对应同一个调用前修订，按 Start 升序、不重叠。相同位置的多个空插入拒绝，调用方先合并；相邻替换允许。
- 插入文本先规范化；BeforeSelection 属于旧文档，AfterSelection 属于全部规范化编辑完成后的文档，两者均校验范围及 rune 边界。
- 完整校验后才修改。非法任一项时，内容、历史、修订及信号全部不变。可复制输入计划，不保留调用方切片或指针。
- 实际从右向左应用，输入中的未处理范围仍有效；一个批次只增加一次 Revision、只发一个 Change。
- Change.Edits 按实际右向左执行顺序记录，每项范围相对于前一项完成后的内容，保持现有 Change 契约。
- 批次是独立撤销单元，不与前后打字合并。内部保存差量/piece 引用，不构造整个文档的历史副本。
- 空列表或仅空范围插入空串为无操作；相同非空内容的替换先沿用现有 Replace 的提交语义，不偷偷改变旧契约。
- Undo / Redo 每次恢复整个批次并发一个修订通知；记录变更仍按实际顺序解释。返回结果的修订属于该次操作，不能冒充重入后的最新状态。
- 两个选区元数据分别复制；nil 表示不提供该方向的历史选区，不擅自读取 Widget。单范围旧 API 及其历史行为保持兼容。

例：`ab\ncd` 中在 0 和 3 插入空格，输入范围为 [0,0)、[3,3)；
实际先改 3 再改 0，得到 ` ab\n cd`，只增加一次修订。一次撤销恢复原文及 BeforeSelection。

### 2.3 验收门槛

断言批次校验全有或全无、同点歧义、相邻范围、UTF-8 与 CRLF 规范化、
修订和通知次数、Undo/Redo 的内容与双向选区、共享模型与来源切换、
重入编辑及回调销毁。确认局部批量编辑不复制无关中间文本，不削弱原 Apply 测试。
G1 阻塞多行缩进/反缩进和 ReplaceAll 的首版验收，不阻塞基本输入或词法开发。

## 3. G2：原生排版中的 Tab 间隔

### 3.1 拟新增契约

在 typography 的 TextFormat 增加 `TabWidth float32`，在 TextLayout 增加
`SetTabWidth(width float32)`。单位为布局 DIP；0 保留各后端既有默认值；
非零必须有限且大于 0，且不超过 1048575 DIP（保留 Pango 两个重复站点的 int32 表示范围）。创建时非法值返回错误，setter 非法值 panic，遵循既有排版参数规则。
Format 返回当前 TabWidth，改变有效值发 Changed 并使度量与栅格缓存失效。

显式宽度表示从段落起始内联边缘重复的 Tab 间隔，遇到 U+0009 推进到下一站；
停在站点上也推进。保留原始文本和 UTF-8 偏移，Tab 在 Cluster/命中中作为单个字符，
不得将若干空格的内部位置暴露为模型光标。

SourceView 的列数选项乘以同一基准字体 U+0020 的实测 advance 得到 DIP。
这里只将“TabWidth 个空格”转成排版量；普通文本仍使用真实 shaping 度量。

### 3.2 后端核对方向

- Linux：[Pango Layout.set_tabs](https://docs.gtk.org/Pango/method.Layout.set_tabs.html) 支持自定义 TabArray，0 恢复原默认；注意 Pango 单位与布局 DIP、RTL、折行及后续重复站点。
- Windows：[IDWriteTextFormat::SetIncrementalTabStop](https://learn.microsoft.com/en-us/windows/win32/api/dwrite/nf-dwrite-idwritetextformat-setincrementaltabstop) 是核对入口，验证格式/布局对象实际应用位置与单位。
- macOS：[CoreText defaultTabInterval](https://developer.apple.com/documentation/coretext/ctparagraphstylespecifier/defaulttabinterval) 与显式 TabStops 共同核对，不能只设置间隔就假定覆盖默认列表。

这些是官方接入方向，不是三端实现已验证的结论。需要先检查既有绑定再补最小入口；
不在 SourceView 重排原生 glyph，不以文本替换/经验偏移补偿后端 Tab 行为。

### 3.3 验收门槛

测试 leading/middle/consecutive Tab、恰好站点、宽度变化、空行、Unicode/fallback、
RTL、折行、字体与 DPI 变化；颜色更新不改变 Tab 度量。
三端分别验证 Text/Range/Cluster、命中和绘制，0 的旧默认行为不得改变。
G2 阻塞可配置 Tab 的首版验收；默认原生 Tab 可用于早期原型，但不能称配置已生效。

## 4. G3：剪贴板错误反馈

拟将 GOUI GUI 剪贴板收敛为可表达结果的接口，而非添加 SourceView 专属平台通道：

```go
package gui

type Clipboard interface {
    SetText(text string) error
    RequestText(callback func(text string, err error))
}
```

- SetText 返回原生写入失败；成功后 SourceView 才允许剪切删除。无需调用方弹窗。
- RequestText 回调恰好一次，可同步；没有文本格式或文本为空均回调 ("", nil)，仅系统/转换失败使用 error；Paste 遇到空文本无操作。
- 读取结果要从 platform/common 接口及各原生后端一直保留到 gui；不能仅改 GUI 签名便假装 bool 已包含错误原因。
- nil 服务、超时、退出取消、非法/不支持的转换按 GUI 服务契约报错，不能用空字符串掩盖失败。UI 包另返回调度受理结果，拒绝受理时不触发回调。
- 这是接口契约变更，需在 GOUI 内更新调用点、替身及所有平台适配后发布依赖版本。具体错误标识沿用 GOUI 同专题最终命名。
- SourceView 的 public Copy/Cut 返回同步错误；Paste 的后续错误经信号报告。原生错误不在 GUI 与控件重复上报。
- 已有 GUI Clipboard 界面足以开发早期复制/粘贴流程；G3 未完成时，失败安全的剪切验收保持未完成，不通过直接调用 platform 绕行。

G3 验收覆盖空文本、无文本、无服务、读写失败、同步/异步回调、回调中编辑或卸载，
以及写入失败时原文/历史/选区完全不变。

## 5. 其他边界

- 现有文本范围颜色、IME、滚动、快照、公开 Widget helper 先直接使用，不预设需要新接口。
- 固定侧栏的 reveal 和滚动锚点先用现有 ScrollIntoView 请求实现，必要验证见布局专题。
- 若实际源代码或公开契约与本文基线不同，先复核并更新证据，不制造重复入口。
- 初始设计阶段只记录提案。实施 GOUI 改动遵循其 AGENTS、测试与跨平台规范；两仓分别交付，实施和发布状态单独记录。

## 6. 查找浮层使用的通用扩展（2026-10-17）

内部栏使用视口尺寸安排固定子控件；查找栏使用所属样式树。应用可用显式
StyleScope 包裹整个编辑区。Basic / Modern 各自 Register 第三方规则，GOUI 不导入
SourceView。具体接口与代码/UI 边界见 [样式接入](DesignStyles.md)。

StyleScope 与 GUI/widgets 使用者迁移在 GOUI `210732a`；Basic / Modern 注册及默认缓存修正在 `903daf8`，没有反向依赖 SourceView。

`gui.Painter.DrawBoxShadow` 在 GOUI `00521cc` 中转发既有 graphics.BoxShadow，继续使用局部 DIP、
普通 transform 和结构裁剪。后端算法、资源归属不变；控件须在自身分配内预留阴影空间。
GOUI 的记录见相邻仓库 doc/PlanComponentStyles.md，本仓验收见 [浮层记录](PlanFindBarAppearance.md)。
框架通用扩展先验证提交，再由本仓依赖；go.mod 仍不能用尚无这些 API 的发行版本完成独立构建。


## 7. 区域光栅化依赖（2026-10-20）

GOUI `ac0efb3` 及其前置提交提供本轮实现；已发布 v0.2.0 不含本轮 `TextLayout.Rasterize(scale, region, buf)` 契约及完整位图
失败恢复修正。通用实现、三后端与调用迁移在 GOUI；本仓复用 DrawTextLayout，
更新直接 Rasterize 的测试/窗口使用者，并修正长行可见摘录。发布依赖仍待 GOUI
实际可获取的新版本，详见 [本轮验收](PlanTextRasterization.md)。

## 8. 文本片段缓存与布局边界（2026-10-20）

Reflow、后端增量重排及 SourceView 接入已撤回，不再将它们作为依赖条件。
TextLayout 继续提供同步排版／度量／渲染，上层复制普通度量值，组合文本范围
形成片段，自行负责可视区、估计、缓存、失效和事件循环调度。

2026-10-21 已接入 GOUI `42c9b42` 中 gui/textedit 的 NewFragment、Fragment.Geometry：复制
普通值快照，按需建立详细几何页。该 API 尚未发布；范围测量和 GUI 帧调度未接入。
原生资源继续归各控件持有，共享 GUI 预算确需
helper 时放 gui，不放 typography，也不加入 SourceView 特例。
设计见 [布局第 11 节](DesignLayout.md#11-片段缓存接入规格实施中)及相邻 GOUI
[通用片段设计](../../goui/doc/DesignIncrementalTextLayout.md)。

## 9. GOUI 当前接口适配（2026-10-31）

本轮只修改 SourceView，复用 GOUI 已有公开接口，不增加通用框架能力。
该轮 `go.mod` 使用公开代理可解析的 `v0.4.1-0.20261031020841-b5d8421e9a94`；当时 v0.5.0 尚未发布。
本地 `go.work` 继续只用于联调，关闭工作区的测试是独立依赖验收。
随 GOUI 一并采用其 nanovgo v0.0.10、oksvg/rasterx/scanFT v0.1.0 依赖。

### 四边 Padding

命令式控件和 `layout.LinearLayout.Padding` 接收 `geometry.Insets`。
查找／替换／定位输入、按钮和示例容器用 `geometry.UniformInsets` 保留已有等边尺寸，
不改变 SourceView 正文自己的布局模型。窗口测试计算 TextView 内容行高时减去
`Padding().Vertical()`，计算首行位置时使用 `Padding().Top`，避免把总留白当成单边。
声明式已有的 `Padding(float32)` 仍为等边简写，无需迁移。

### 选区属性绑定

`ui.SourceView().BindSelection(source)` 保存稳定的 `ui.BindState[TextSelection]` 指针，
声明时不调用 Get，不让父 ComposeView 意外收集整棵编辑器子树的依赖。
Update 中通过 GOUI `ui.BindValue(ctx, "selection", source, apply)` 注册属性绑定。
后续源通知仅应用选区，不重放 Document、ReadOnly、Scheme、Search 或其他声明，
也不重建父视图。属性应用期间阻断声明式信号连接，避免回写或通知业务回调。
订阅的切换、失效、转移和销毁均交给 GOUI 协调器管理。

修饰方法按 GOUI TextView 的规则处理：

- BindSelection：设置读绑定和生成的 OnSelection 回写；nil 为无操作。
- OnSelection：替换回写，保留读绑定。
- Selection：取消读绑定及生成的回写，保留用户显式设置的 OnSelection。
- 重建时未声明选区：交给控件自身维护；旧读绑定由协调器移除。

无需创建独立的状态订阅系统。SourceView 的语法、搜索、完成项样式仍显式解析
已命名部件；已有 Basic／Modern 注册和 StyleChanged 继续复用。
本轮不改变代码配色、缓存键、文本模型或渲染后端。

回归覆盖放在 `controls_test.go`、`ui/sourceview_test.go`，真实协调和事件循环验证位于
`tests/window/editor/binding`。实际执行结果和平台缺口见 [Plan](Plan.md)。

## 10. GOUI v0.5.0 正式版（2026-11-01）

依赖从上述伪版本切换到可直接获取的 v0.5.0，保持 Go 1.24 要求。
SourceView 的四边 Padding、`ui.BindValue` 局部属性绑定、文本排版与 Basic/Modern
注册继续使用现有公开契约，不增加兼容层或要求本地 GOUI 检出。

声明式文件对话框改为 `OpenFile(windowID, options, callback) error`，
OpenDirectory/SaveFile 同样显式指定窗口 ID。使用者通过已声明的窗口 ID
关联所属窗口，不在缺少所属窗口时改用无主对话框。返回错误表示请求未被派发；
回调报告已接受请求的完成错误，空结果且无错误表示取消。
使用者在本地文件读取结束后关闭所有返回的 FileAccess，包括错误和提前返回分支。
文件筛选器显式提供非空显示名称和匹配模式，以符合 Linux Portal 的筛选器约定。
所属窗口的模态输入、焦点和销毁处理交给 GOUI，不在 SourceView 重复维护模态状态。

v0.5.0 的窗口最小尺寸由应用显式声明，不再从内容测量自动生成。
现有 editor/frame/interaction 的显式最小尺寸保留；控件测量仍只表达控件布局约束。
ScrollView 的已分配视口和滚动几何由其 Arrange/LayoutVisible 流程维护，
SourceView 继续报告内容范围与延迟布局结果，不在 Measure 阶段另行提交滚动几何。

实际接入以独立模块测试和正常窗口事件验证为准，记录见 [Plan](Plan.md)。
