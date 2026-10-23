# 初始实现与验证记录

当前模块直接依赖已发布的 GOUI v0.2.0，无需相邻 GOUI 检出或本地工作区。
本文件保留各阶段的历史依赖和失败记录；发布版接入与独立验证见
[当前计划](Plan.md#14-goui-v020-发布版依赖2026-10-19)。

当前 Basic / Modern 接入与验证见 [样式接入验收](PlanStyles.md)，
默认选区保留语法色的后续修正见 [代码选区验收](PlanSelection.md)。
菜单和查找栏的八语言补齐见 [多语言验收](PlanLocalization.md)。
中大文件功能检查、冷定位修正及回归结果见 [中大文件验证](PlanLargeFiles.md)。

当前公开语言/编辑/补全扩展和 Markdown 示例见 [扩展验证](PlanLanguageExtensions.md)。

日期：2026-10-16。本记录描述实际代码与本轮验证；专题设计中的完整矩阵和性能目标仍需逐项验收。

## 1. 已有实现

| 位置 | 已实现内容 |
| --- | --- |
| 根包 Document | 组合 textedit.Model；语言配置；共享增量引擎；只在视图挂载期间订阅模型，最后卸载释放分析状态 |
| 根包 SourceView | 独立叶子控件；Scrollable / IMClient / Snapshot；视图各自拥有 native layout、Geometry、HeightIndex、选区及滚动 |
| 编辑与输入 | 单选区；正常键盘/手势路径；Cluster 导航；只读；撤销重做；Tab 与 Enter 缩进；剪贴板异步请求身份校验，作废请求释放视图和文档引用 |
| IME | textedit.Projection 临时呈现；原文与 preedit 分离；提交/取消及局部候选框坐标；真实组合输入尚未验收 |
| 搜索 | 流式字面量搜索、Unicode simple fold、整词；原始匹配区间与 Cluster 选区分离；受预算约束的原子全部替换 |
| language | Plain / JSON / XML / HTML / TOML / YAML / Shell 元数据、扩展名和有界样本识别、token Kind |
| internal/highlight | 分片扫描、跨行状态、旧坐标行索引、稳定行身份、向后失效与状态收敛、token LRU |
| scheme | 不可变输入拷贝、亮暗代码配色；自动配色按应用共享一个 Settings 监听 |
| 调度与布局 | Root 共享分析队列；4 ms 总预算、文档 2 ms；16 KiB 扫描片段；可见段落布局；固定侧栏、reveal 和字节位置视口锚点 |
| ui | 薄 WidgetView；恢复可选配置默认值；Document 在 Build 外创建；更新抑制信号写回，卸载断开绑定 |
| examples / tests/window | 两种接入示例、三语言演示、带正常事件路径断言的独立窗口程序 |

分析预算约束扫描工作，不约束单次原生 shaping。原生缓存限制为 128 个段落和约 8 MiB
文本载荷；单个超大段落仍需要整体布局。该数字不是 glyph、原生 raster 或全部进程内存的硬上限。
token 载荷目标为 16 MiB，另有行状态和索引开销；超大行达到 token 数限制时后续部分使用普通前景。

HTML 有独立 raw text / RCDATA / script 状态；首版不递归高亮内嵌 JavaScript / CSS。
高亮用于容错呈现，不是完整 JSON / XML / HTML / TOML / YAML / Shell 语法验证器。

## 2. GOUI 依赖与跨仓变更

GOUI 通用扩展已在分支 `feature/for-source-view` 按原生绑定、功能与文档拆成 8 笔提交，系列 HEAD 为 `2f782897684f1bfc892a11b0254e5a2162980ed8`，
系列基线为 `4167bb79823efffbea43e00558260448959536b0`；未推送或发布。
GOUI 原有的 Plan / Roadmap 修改与其他未跟踪文件未纳入该提交。

实际接口：

- G1：textedit.Model.ApplyBatch 与 BatchOptions。原始坐标、排序非重叠 edit；
  完整预检、一次 revision / Change / undo；明确历史前后选区，Change 按从右到左的实际执行顺序报告。
- G2：typography.TextFormat.TabWidth / TextLayout.SetTabWidth。
  DIP 单位，0 恢复后端默认；有效区间为 [0, 1048575]，创建返回错误、非法 setter panic。
  Pango 的 TabArray、DirectWrite 的 incremental tab stop、CoreText 的 TabStops 与 default interval 分别接入。
  SourceView 使用基准字体 U+0020 的原生 Cluster advance 乘以列数。
- G3：gui / platform/common 的 SetText 返回 error，RequestText 回调使用 (string, error)，
  没有文本/空文本返回 ("", nil)，错误仅用于失败；Paste 空文本无操作；同步/异步回调都支持。UI 包保留结果并负责线程调度。
  三后端同步迁移；X11 校验回复身份、协商 UTF-8/Latin-1、限制属性读取，五秒期限与退出取消均完成待处理回调。
  macOS 检查原生写入结果；Windows 使用平台隐藏 HWND 作为写入所有者、释放失败内存，
  并按分配大小限制 UTF-16 终止符读取。UI 的 RequestText 另返回调度受理结果，拒绝受理不回调。
  GOUI 既有 TextInput / TextView 与示例调用点随契约迁移。

初始实现时 go.mod 声明可获取的 v0.1.1，本地 go.work 同时使用两个仓库且被忽略。
当时尚无包含 G1/G2/G3 的发布版本，因此该阶段整模块不能脱离工作区独立构建或发布。
没有伪造提交、版本或可用的远程依赖，也没有提交机器专属 replace。

## 3. 实际验证

环境：Go 1.25.8，Ubuntu 22.04.5 x86_64，X11 :3.0 / Xfwm4，Pango 1.50.6。

| 验证 | 结果与边界 |
| --- | --- |
| 本仓 go test ./... | 通过；含随机 500 次编辑、手写三语言词法样本、分片不变性、增量收敛、重入、共享视图、历史、原始搜索区间、native Reset 重入后的旧操作取消和缓存所有权 |
| 本仓 go test -race ./... | 通过；没有将 GUI / native 对象放到并发 goroutine |
| 本仓 go vet ./... | 通过 |
| GOUI go test ./... | Linux 工作副本和拟提交干净副本通过；隔离 X11，沿用既有测试分类及工作副本写图副作用 |
| GOUI 相关 race | gui/textedit、gui、ui、Pango、X11 通过 |
| Pango TestTabWidth | 通过；连续 Tab、重复站点、格式/文本保留、setter 通知、恢复默认、颜色后的坐标及非法创建参数 |
| Linux Software 1x | 窗口断言通过；实际 imageResource / PixelScale 日志确认后端和比例 |
| Linux OpenGL 1x | 同一窗口断言通过；实际 OpenGL imageResource / PixelScale=1 |
| Linux Software 2x | 同一窗口断言通过；实际 Software imageResource / PixelScale=2 |
| 原生剪贴板 | 独立窗口内 cut / paste 往返通过；不是跨应用 / INCR 验收，默认自动流程已不改写剪贴板 |
| 三语言与亮暗 | 桌面人工检查示例的 JSON、XML、HTML、固定行号与亮暗显示；截图仅是观察证据，不是像素断言 |
| windows/amd64 原生 | Windows 10 19045 / Go 1.24.0 / Session 0：GOUI gui/ui/textedit/DirectWrite/demo 测试、全仓 build、本仓全仓测试、剪贴板往返和无格式读取通过；完整 GOUI 测试有基线可复现的既有失败 |
| darwin/arm64 原生 | macOS 15.7.7 / Go 1.24.9：GOUI gui/ui/textedit/CoreText/cocoa/demo 测试、全仓 build、本仓全仓测试、剪贴板往返和无文本读取通过；完整 GOUI 测试有基线可复现的既有失败 |
| GOWORK=off go test ./... | 未通过：v0.1.1 没有 ApplyBatch / BatchOptions；这是尚未发布新增依赖的已知阻塞 |
| 关闭工作区的纯逻辑子集 | internal/highlight、language、scheme 测试通过 |

早期交叉编译使用 GOOS / GOARCH / CGO_ENABLED=0 go test -exec /bin/true，仅构建测试目标。
本轮复查另在 Windows/macOS 执行上述原生用例；不将 Session 0 或离屏排版计为交互桌面验收。
GOUI `go vet ./...` 仍有与纯基线逐项相同的 193 条诊断，没有新增；本仓 go vet 通过。
完整命令、原生环境、远端基线失败和副作用见 [GOUI 验收记录](../../goui/doc/PlanSourceViewExtensions.md)。

窗口断言覆盖 CommitText、KeyDown/KeyUp、PointerDown/Up 的正常 Window 入口：
普通输入、只读限制、撤销重做、选区缩进、Escape/Tab 焦点遍历、查找替换、
有界快照、共享文档、卸载重挂、末尾 caret reveal、鼠标选择，以及顶部插行后的视口锚点。

剪贴板测试的已发生副作用：早期自动用例虽然在退出前写回了原文本，
但 X11 所有者随后随进程退出，原剪贴板文本未能保留。
已取消默认自动流程中的剪贴板写入；现需显式 -clipboard 且使用可丢弃内容。
该限制也写在窗口程序入口和 README，不能把“退出前写回”解释为退出后保留。
复查新增 X11 跨进程测试均在独立 Xtigervnc/Xfwm4，验证了无文本、编码回退、
格式/超限失败、无应答五秒期限和退出取消，没有改写日常桌面的剪贴板。
macOS 原生验证通过外部包装快照并恢复 pasteboard，本次原始/恢复状态均为 0 items。

## 4. 尚未完成的验收

- Windows / macOS 的 SourceView 交互窗口绘制、键盘布局、真实 IME、跨应用剪贴板与缩放；
  已通过的原生 Tab/剪贴板基础接口测试不代替这些场景。
- Linux 真实中文/日文等 IME 的内联组合、取消、重入及候选框定位；
  CommitText 断言不能代替这些验收。
- 多窗口最小化、跨屏、长时间交互，以及宽度/font/theme 重排的完整锚点矩阵。
- 10 MiB 常规文档、极长单行和百万行的延迟、内存峰值与长期资源压力测量。
  当前采用整段 native shaping，不承诺极长单行的帧预算。
- 完整语义/像素自动断言、交互应用间剪贴板和 X11 INCR；
  GOUI X11 的五秒期限、退出清理和迟到回复隔离已实现并验证。
- GOUI 新接口发布、更新依赖版本以及关闭工作区后的完整验证：后续状态见 Plan 的发布版依赖记录。

## 5. 着色缓存后续切片（2026-10-16）

高亮、选区、焦点与方案变化现在复用原生正文和行号布局，先清除旧前景再叠加当前颜色；
保持 Geometry、行高和已知正文宽度。初始索引完成时转接临时缓存，语言切换保留文本 LineID。
字体、Tab、折行、文本与挂载 Context 的正常失效不变；回调重入后核对缓存所有权与状态。

本轮还在 GOUI 的 DirectWrite 中修正全段前景覆盖时残留旧范围记录的问题；
SourceView 的 Windows 长寿命布局重着色依赖该修正，后续已单独提交到 GOUI `227220a`。
依赖版本仍未发布，未修改 go.mod 基线；后续新增菜单键盘导航为 GOUI `ffc59f3`。

三端原生度量/位图、本机窗口与基准的环境、前后差异和验证边界见
[着色缓存验收](PlanColorCaching.md)。这份更新补充前面的初始实现记录，
未完成的真实 IME、规模和发布项仍有效。

## 6. 编辑体验后续切片（2026-10-16）

已经实现按词拖选与双击/三击修正、完整编辑菜单、同步语言/文档词补全、
自动配对与括号匹配，命令复用已有历史、选区、原生排版和失效流程；ui 增加薄配置。
实现、验证、各平台环境与限制见 [编辑体验验收](PlanEditorExperience.md)，
公共行为见 [括号设计](DesignPairs.md) 与 [补全设计](DesignCompletion.md)。
本切片按功能分别提交，未推送；不改变前述独立发布及真实 IME 验收状态。

## 7. 编辑快捷键与可选查找栏（2026-10-17）

> 以下保留本切片的历史记录。当前查找与定位已内置 SourceView，旧公开配套控件
> 及声明式绑定已删除；现行接入见 [内置界面](DesignFindBar.md)。

已补充上下插入行、词导航/删除、整行选择、缩进、括号跳转及搜索导航的常用平台快捷键。
同仓 FindBar 为应用组合的可选控件，复用 SourceView 的查询/替换/高亮和模型历史；
ui 仅协调目标、显式值及信号。未控制 Search 的声明式重建保留交互查询。
接口、生命周期和计数边界见 [查找栏设计](DesignFindBar.md)，本机/远端验证及剩余项见
[验收记录](PlanFindBar.md)。GOUI 未修改，独立依赖发布、真实 IME 和远端桌面验收缺口仍有效。


## TOML、YAML、Shell（2026-10-19）

新增三种内置语言、扩展名/启动文件/shebang 识别，复用分片引擎并支持当前行的延迟分类。
对应词法与引用状态同步参与基础补全及自动配对；basic/modern 新增 `token-variable`。
命令式示例内嵌六份可切换文档，文件选择行可换行；README 仅更新对外语言列表。
详细规则及边界见 [语言设计](DesignHighlighting.md#9-tomlyaml-与-shell)，
结果见 [本切片验收](PlanLanguages.md)。未修改 GOUI，也未增加第三方依赖。
