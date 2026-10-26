# GOUI SourceView v0.2.0

## 中文

本版本正在准备，尚未发布。需要 Go 1.24 或更新版本，直接依赖 GOUI v0.4.0，
无需相邻 GOUI 检出、replace 或本地 go.work。继续提供 Go 库与源码示例，
采用与 GOUI 一致的 MIT 许可证。

### 主要变化

- 查找、替换与行列定位由 SourceView 内置管理，按需创建并保留输入状态；
  默认快捷键直接可用，可通过 KeyBindings 取消或替换。
- 声明式 ui 包补齐 Document、选区、缩进、搜索、补全、命令及快捷键等类型别名、
  常量和常用构造函数，配置与回调无需混用命令式根包。
- 文档识别 LF、CRLF、CR，编辑模型继续使用 UTF-8/LF；WriteTo 按所选换行方式
  分块导出，切换输出格式不改编辑坐标或文本历史。
- 空白显示增加 Boundary 模式，普通空格使用随字号缩放的圆点，Tab 使用短箭头；
  逻辑换行标记单独控制。Basic / Modern 的亮暗主题提供低对比度标记颜色。
- 超长逻辑行复用 GOUI 的有界原生排版与局部几何，改进编辑、滚动、调整视口、
  大坐标光标显露和拖选；目标度量未就绪时按序等待处理输入。
- 修正行号与折叠侧栏的鼠标指针，以及补全期间左右移动光标时的候选更新。
  示例同步 GOUI v0.4.0 的声明式与 CheckButtonGroup 契约。
- 修正长段落度量尚未就绪时读取语义快照可能触发的空几何崩溃。

### 从 v0.1.0 迁移

公开的 FindBar、GoToBar、NewFindBar、NewGoToBar 及对应 ui 构造入口已移除。
删除应用为这些配套栏创建的 Overlay、Target 和生命周期连接；SourceView 在
自己的视口内管理界面。命令式调用 ShowFind、ShowReplace、ShowGoTo 和 HidePanel；
声明式只需配置 SourceView，使用默认快捷键或应用自己的命令处理。

需要取消默认查找、替换或定位按键时，从 DefaultKeyBindings 返回的表中删除
CommandFind、CommandReplace 或 CommandGoTo，再显式设置 KeyBindings。
底层搜索、替换、选区及滚动方法仍可用于应用自建界面。

普通源码编辑声明仍使用 sourceui.SourceView；自定义组合组件遵循 GOUI v0.4.0
的 ComposeView.Build(ctx)，直接挂载控件的 WidgetView 实现 Mount / Update / Unmount。
自定义控件的其他 GOUI 迁移要求参见
[GOUI v0.4.0 说明](https://github.com/golang-gui/goui/blob/v0.4.0/doc/ReleaseV040.md)。

Document.Model().Text() 返回规范化的 LF 文本；需要保留文档输出换行时使用
Document.WriteTo。InsertSpaces 决定新输入的 Tab 是否转换为空格，不会转换已有缩进。

### 验证与边界

关闭工作区的干净源码验证：Linux amd64／Go 1.25.8／Pango、Windows 10 amd64／
Go 1.25.8／DirectWrite、macOS 15.7.7 arm64／Go 1.24.9／CoreText，
原生全仓各 520 项测试及子项通过，零失败、零跳过。三端模块校验、tidy、vet、
无 cgo 构建，以及 Linux 全仓 race 通过。Linux 独立 X11／Software 窗口验证覆盖
内置查找、补全、空白显示、外框、宽度缓存，以及 176 MiB 单行编辑与拖选。

macOS／CoreText／OpenGL 和 Windows／DirectWrite／Direct2D 已运行原生窗口矩阵，
覆盖查找、外框、补全、宽度缓存、编辑和长行场景，应用缩放按场景为 100%／150%／200%。
已提交候选的 Basic 100% 空白圆点检查在两端失败；Windows 150% 的超长单行末字符
指针选取和前向拖选检查也失败。单独复验工作区测试调整后，Windows 空白圆点和两端
长行指针检查通过；macOS 的不足一物理像素直径圆点仍被最小 2×2 像素断言拒绝。
Windows 编辑体验用例还出现菜单键盘选中状态的间歇性失败。测试调整尚未提交，
这些结果不能作为已提交候选全部发布门槛通过的证明。

两端 150% 示例的实际桌面操作确认输入与撤销、查找计数、补全滚轮和菜单 End 导航；
示例均正常退出。人工操作通过不替代仍然失败的自动回归。

候选源码、环境和检查项记录在 [发布准备](Plan.md)。真实 IME、混合 DPI 及全部
Painter 组合尚未验收；正式发布前还需确认上述窗口失败、冻结候选并核对标签与
Release。README 安装指引在版本公开并通过消费方验证后更新。

未访问的大文本区域仍可使用估计几何；有界排版不承诺任意文件即时精确布局或
所有 Unicode 上下文与完整原生段落完全等价。首次读取、模型建立和单次原生排版
仍可能产生延迟。默认补全为词法建议，应用可提供自己的补全服务。
多光标与完整 IDE 语言服务不在本次范围，真实 IME、混合 DPI 和所有后端组合
仍需分别验收。0.x 阶段公共 API 可能调整。

## English

This version is in preparation and has not been published. It requires Go 1.24 or newer
and GOUI v0.4.0, without a separate GOUI checkout, replace directive or local workspace.
It distributes the Go library and source examples under the same MIT license as GOUI.

### Changes

- SourceView owns its find, replace and line/column interfaces, creates them on demand,
  and retains their input. Default shortcuts work without companion widgets and can be rebound.
- The ui package adds shared document, selection, indentation, search, completion,
  command and key-binding types, constants and helpers for declarative applications.
- Documents detect LF, CRLF and CR. Editing coordinates remain normalized UTF-8/LF;
  WriteTo exports the chosen line ending in chunks without changing text or undo history.
- Boundary whitespace mode, font-scaled space dots and short Tab arrows improve markers.
  Logical line endings remain independently controlled, with subdued Basic / Modern colors.
- Bounded native layout and local geometry improve long-line editing, scrolling, resizing,
  caret visibility and drag selection. Input waits in order when target geometry is pending.
- Gutter pointers and completion updates during horizontal caret movement are corrected.
  Examples follow GOUI v0.4.0's View and CheckButtonGroup contracts.
- Semantic snapshots handle pending long-paragraph geometry without a panic or new measurement.

### Migration from v0.1.0

FindBar, GoToBar, NewFindBar, NewGoToBar and their ui constructors are removed. Remove
application-owned overlays, target wiring and lifecycle connections for those companions.
Call ShowFind, ShowReplace, ShowGoTo and HidePanel on a mounted SourceView, or use the
default shortcuts. Declarative code only needs sourceui.SourceView.

To disable a default shortcut, remove CommandFind, CommandReplace or CommandGoTo from
a fresh DefaultKeyBindings table and explicitly apply it. Search, replacement,
selection and scrolling methods remain available for application-owned interfaces.

Custom compositions use GOUI's ComposeView.Build(ctx); WidgetView implements
Mount / Update / Unmount. See the
[GOUI v0.4.0 migration notes](https://github.com/golang-gui/goui/blob/v0.4.0/doc/ReleaseV040.md)
for other framework changes.

Document.Model().Text() returns normalized LF text; use Document.WriteTo to preserve
the document's selected output line ending. InsertSpaces controls newly entered Tabs
without converting existing indentation.

### Verification and limits

Clean-source tests with the workspace disabled pass on Linux amd64 / Go 1.25.8 /
Pango, Windows 10 amd64 / Go 1.25.8 / DirectWrite, and macOS 15.7.7 arm64 /
Go 1.24.9 / CoreText: 520 tests and subtests per platform, zero failures or skips.
Module verification, tidy, vet and no-cgo builds pass on all three; full race passes
on Linux. Isolated Linux X11 / Software window checks cover find, completion,
whitespace, frames, width caching, and editing/dragging in a 176 MiB single line.

Native window matrices ran on macOS / CoreText / OpenGL and Windows / DirectWrite /
Direct2D, covering find, frames, completion, width caching, editing and long lines at
application scales of 100%, 150% or 200%, depending on the case. The committed candidate
fails Basic whitespace-dot checks at 100% on both platforms, and final-character pointer
selection and forward dragging in a very long line on Windows at 150%. Separately tested
working-tree adjustments pass the Windows whitespace check and both platforms' long-line
pointer checks. A subpixel-diameter dot on macOS still fails a minimum 2-by-2-pixel
assertion. The Windows editor-experience check also intermittently fails menu keyboard
selection. These test adjustments are uncommitted; the committed candidate's release
gates are not all satisfied.

Actual desktop input at 150% confirmed editing and undo, find counts, completion-wheel
behavior and menu End navigation on both platforms. Both examples exited normally.
These manual observations do not replace the failing automated regressions.

Candidate source, environments and outstanding checks are recorded in the
[release preparation log](Plan.md). Physical IME, mixed DPI and every painter
combination remain unverified. The window failures, final candidate and tag/Release
must be checked before publication. README installation instructions change after
the public version passes consumer verification.

Unvisited regions may use estimated geometry. Bounded layout does not promise immediate,
exact layout of every file or equivalence to a complete native paragraph for all Unicode
contexts. Initial I/O, model construction and individual native calls can still pause.
Default completion is lexical; applications can supply their own service. Multi-cursor
editing and full IDE language services are outside this release. Physical IME, mixed DPI
and every backend combination require separate acceptance. APIs may change during 0.x.
