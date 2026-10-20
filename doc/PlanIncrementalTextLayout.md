# 长段落排版优化记录

## 范围与状态

2026-10-20，用户明确选择保持完整正确性并继续通用方案。GOUI 提供通用排版
基础，本仓只接入公共契约，不基于 TextView 或复制其私有 editor。

- [x] 核对 Scintilla／Monaco、三个原生后端的相关能力，记录独立窗口原型失败。
- [x] GOUI 的几何索引和可视行选区查询；本仓选择／搜索／出现词／预编辑绘制接入。
- [x] 长段落全选仅产生可视行矩形的包内回归。
- [ ] 保留完整上下文的可恢复分析和 shaping。
- [ ] 按事件循环间隔分步推进、测量缓存复用、增量折行、取消与生命周期。
- [ ] 未完成排版期间仍可拖滚动条、操作其他控件的真实窗口验收。

**当前仍会在首次原生长段落排版时阻塞。** 不能以查询优化宣布整个需求完成。
带 bidi 缺陷的独立窗口原型没有接入正式控件，未使用后台测量。
方案与原生能力门槛见 [布局设计](DesignLayout.md)，后端诊断和同环境查询对照见
GOUI [实施记录](../../goui/doc/PlanIncrementalTextLayout.md)。

## 依赖

本轮使用相邻 GOUI `554dbdb` 之后的改动，需要尚未发布的 `21444f9` 中的
`gui/textedit.Geometry.LineAtY` 和 `SelectionLines`；GUI 使用方在 `25e52c1`，
诊断在 `33113b9`，设计与记录在 `b022304`。本仓代码提交为 `6224e5e`。
现有 go.mod 的本地 replace
为用户原有改动，本轮保留；没有修改全局 Go 环境或提交机器路径。

使用本地 go.work 联调。`GOWORK=off` 仍会读取现有 replace，因此该命令成功
也不能算独立依赖验证；需另外使用仓库外临时 modfile 去除 replace 检查实际
v0.2.0 依赖。已有 Rasterize 接口变化也尚未由该版本提供。

## 验证

Linux amd64／Go 1.25.8／Pango 1.50.6，通过：

- `go test ./...`、`go test -race ./...`、`go vet ./...`。
- GOUI `go test ./...` 及 `go test -race ./gui/textedit ./gui`。
- Windows amd64、macOS arm64 的 `CGO_ENABLED=0 go build ./...`。
- `XMODIFIERS=@im=none GOUI_PLAT_PAINTER=software GOUI_PLAT_SCALE=1 go run ./tests/window/editor/selection`：
  X11／XFCE，Basic／Modern Light／Modern Dark 的 JSON／XML／HTML 九组通过，
  包括正常事件分发、语法前景、选区背景像素、焦点与菜单。窗口自动关闭，
  无剪贴板或系统设置修改。

失败单独记录：

- `GOWORK=off go test ./...`：`go: updates to go.mod needed`，没有执行 tidy 改动用户文件。
- 仓库外临时 modfile 删除 replace 后，`GOWORK=off go test -modfile ... ./...`
  在新增 `LineAtY`／`SelectionLines` 编译失败；不能把工作区通过当成独立发布通过。
- GOUI `go vet ./gui/textedit ./gui` 有历史未命名字段诊断；与 HEAD 对照相同，
  详见其实施记录。

首次长段落排版仍同步，窗口响应目标未通过验收。Windows／macOS 原生运行、
真实 IME／物理键鼠和混合 DPI 未执行。上述选区窗口用例不能替代长段落性能验证。

## 点击定位回归联验（2026-10-20）

GOUI `71b25c8` 修复 TextView／TextInput 的光标移动无条件请求尺寸布局：
有效段落直接复用几何，通过既有滚动机制显示目标，缺失／失效时仍请求布局。
本仓源码不需要适配新接口；保留现有 go.mod 和用户的 example/temp。
本次 SourceView 代码基线为 `9827920`。

- 两仓 `go test ./...`，GOUI `go test -race ./gui ./gui/textedit`、
  SourceView `go test -race ./...` 通过；新增 GOUI 回归在修复前代码明确失败。
- GOUI 原生窗口 `tests/window/gui/text_caret` 在 Linux amd64／X11／XFCE／
  Pango 1.50.6／Software、Modern 1×／2× 下通过点击、导航和排版复用断言。
- 本仓 `tests/window/editor/selection` 的 JSON／XML／HTML × Basic／Modern
  Light／Modern Dark 九组正常事件与像素检查通过（Software 1×）。
- 两仓受影响包及本仓全模块的 Windows amd64／macOS arm64 无 CGO 交叉构建
  通过；不代表这两端原生运行验收。
- `GOWORK=off go test ./...` 仍报 `updates to go.mod needed`；没有 tidy、
  更改用户 replace 或把本地工作区通过写成独立发布通过。

用户 157,862 字节单行 JSON 的本机只读诊断中，TextView 与 SourceView
（JSON 高亮）的三次点击均无新增完整度量。TextView 的 32 KiB 单段回归从
每次点击两次整段排版，改为不新增排版调用。采样边界、耗时和限制详见
GOUI [光标回归记录](../../goui/doc/PlanIncrementalTextLayout.md#光标定位回归修复2026-10-20)。

首次整段排版仍同步；SourceView 着色更新后观察到一次约 293 ms 的原生
MeasureSize，尚需确认并修正颜色失效路径。2× 跳转绘制也仍有延迟。
本次修复不改变上面“通用分时排版未完成”的状态。窗口已关闭，
未写剪贴板、系统设置或用户 JSON。

## 已测段落换宽接入（2026-10-20）

GOUI 通用实现依赖 `6676636` / `57163fd`，及其原生绑定、RTL 修正前置。
SourceView 基线 `e0b1a6c`。设计见 [布局](DesignLayout.md)，后端与预算设计见
相邻 GOUI [可视区排版](../../goui/doc/DesignViewportTextLayout.md)。

已测 8 KiB 以上段落在窗口换宽时保留 TextLayout/Geometry，通过公开
DeferTextLayout 分帧生成新宽索引。等待时沿用旧几何绘制、命中和选择，完成后
更新 Geometry/高度与锚点。宽度请求取消旧任务，清理缓存前取消；文档 splice
与初次词法稳定 ID 迁移取消绑定旧行号/旧键的回调。颜色不会丢弃保留的测量。

本次验证也修正了接入阶段发现的重复同宽任务：完成回调捕获的 box 宽度不能
被正文占用宽度覆盖。用户 JSON 在修正前初始化循环产生重复请求，未作为通过；
修正后初始化收敛、连续换宽只发布最新结果。窗口用例对照完整原生几何并断言
点击位置，不以日志或启动窗口代替验收。

新增 `tests/window/editor/reflow`：默认 SourceView，`-textview` 为 TextView，
`-file` 只读现有单段 JSON。Modern 集成窗口，真实 X11 窗口由 xdotool 依次
调整 680/820/620/900 DIP，2× 使用对应物理尺寸；按钮与文本点击均正常派发
Window.DispatchEvent。为显式固定文本原点，此用例关闭行号/折叠。

Linux amd64／Go 1.25.8／X11／XFCE(xfwm4)／Pango 1.50.6／Cairo 1.16.0，
32 KiB 原创长词的最终复测：

| 控件 | 后端/缩放 | 请求/发布 | 待排版定时器回调数 | 最长 Advance | 最长 20 ms 定时器间隔 |
| --- | --- | --- | --- | --- | --- |
| TextView | Software 1× | 17/1 | 42 | 2.07 ms | 69.7 ms |
| SourceView | Software 1× | 4/1 | 43 | 0.68 ms | 64.3 ms |
| TextView | Software 2× | 17/1 | 40 | 0.81 ms | 127.0 ms |
| SourceView | Software 2× | 4/1 | 40 | 1.02 ms | 144.7 ms |
| TextView | OpenGL 1× | 17/1 | 45 | 0.78 ms | 36.9 ms |
| SourceView | OpenGL 1× | 4/1 | 41 | 1.25 ms | 31.2 ms |

全部通过待排版按钮响应、无同步 SetSize、过期宽度取消、最终原生行/cluster、
可见墨迹、点击与编辑/销毁检查。Advance 不包含完成回调里的 Geometry 构造、
布局及绘制，定时器间隔也不是 compositor 延迟或硬实时上限。

用户 `/data/xuyang8/1.json`（157,862 字节）只读验证也通过两控件 Software 1×
换宽、几何、墨迹、点击和编辑检查。SourceView 启用 JSON 高亮。最终记录：
TextView 30 次请求/1 次发布、292 个待排版定时器回调，SourceView 为 4/1、
107 个回调；最长推进约 4.15/4.18 ms，最长定时器间隔约 104/99 ms。该轮与
全量测试并行，作为功能/响应性观察，不作为性能前后对照。未写回 JSON。

严格的排版执行耗时对照在 GOUI `BenchmarkDeferredResize`：同环境、同字体、
32 KiB、520→480 DIP、包含完整度量导出。原生同步约 1.848～1.881 s；保留测量
约 78.3～79.1 ms（无事件循环等待），最长推进约 2.56～2.76 ms。不含首次分析、
绘制或 GUI Geometry；不能将累计执行耗时当作最终文本呈现延迟。

通过两仓全量 `go test ./...`、GOUI 目标 race、本仓 `go test -race ./...`；
GOUI `./gui` 与本仓 `./...` 的 Windows amd64 / macOS arm64 无 CGO 交叉构建
通过。这两端未原生运行，也尚无换宽分帧后端。`GOWORK=off go test ./...` 仍报
`updates to go.mod needed`，保持用户 go.mod/replace 和 example/temp 原样。
全量测试产生的历史图片不提交，新窗口均已关闭，无剪贴板/系统设置修改。

首次完整分析、巨大单行/cluster、完整 Geometry 构造与高 DPI 绘制仍可能较慢。
详细几何分页、新首屏提前发布、检查点/后缀复用、冷分析 continuation 和其他
原生后端还未实现，不能称通用长段落排版或任意输入不阻塞已完成。
