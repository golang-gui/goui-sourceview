# 段落测量减负与独立换行索引

2026-10-22。SourceView 代码基线 `86df709`，联调 GOUI 从 `68cd392` 到 `5545262`。
GOUI 已按六个功能提交，详细设计及全部对照见
[GOUI 片段布局](../../goui/doc/DesignIncrementalTextLayout.md#14-度量索引与换行分离2026-10-22)、
[验证记录](../../goui/doc/PlanMeasuredTextLayout.md)。GOUI 后续文档提交不改变运行代码。

## 已实现与边界

- 实际 TextLayout 路径批量导出 Pango／DirectWrite 度量；Pango 根据 region 绘制
  相交的原生行，保留完整段落上下文。后端没有新增视口、预算或 Reflow 策略。
- GOUI Geometry 修复同 byte 的浮点舍入边缘导致的重复光标步进；本控件复用公开
  设施受益，不复制私有实现。真实间隙、不同 byte／bidi 和软折行位置仍保留。
- GOUI 提供普通值 WrapMeasurements／WrapIterator，本控件**尚未将其接入正式
  换行**。Pango／CoreText 的上下文范围塑形验证仍为前置实验，不是已发布的三后端 API。

**正式的有限行组、按事件间隔推进、换宽复用 advance、局部编辑后缀收敛仍未完成。**
两个控件当前仍可能在完整段落的原生折行中阻塞窗口；没有把整段工作挪到后台或下一帧
冒充解决。本模块未增加生产代码或新依赖，依靠原有公开契约受益。

## 真实操作验证

用户 `1.json` 为 157,862 byte、一个逻辑段落，SHA-256 为
`7baa22a894b415fdc3425c94f8a541d0ded8f77965541c46af3c95a3364c798b`。
文件只读，正常键鼠编辑内存，关闭时完整比较恢复一致。

使用既有 `tests/window/editor/interaction`，断言和输入路径未修改；Modern、
WindowChromeIntegrated、自动换行，SourceView 保留行号／折叠，关闭自动补全。
Linux X11／Xfwm4、Pango 1.50.6／Cairo 1.16.0，Windows 10 19045／DirectWrite／RDP，
macOS 15.7.7 M1／CoreText／VNC，Go 1.25.8、software painter、缩放 1。

Windows 两轮最终代码的中部单字符 input→Widget Paint：TextView 207–210 ms，
基线 261–270 ms；SourceView 375–377 ms，基线 436–447 ms。这是 GUI 捕获至控件
绘制，不含桌面传输；没有声称所有输入已流畅。SourceView 的 Linux 短词粘贴约
855–860 ms → 783–787 ms，仍有较长原生工作和后续着色重排。

通过：中部点击、插入、Backspace、选择、替换、Undo／Redo、Left／Delete、全文恢复；
Linux／Windows 各控件各两轮，macOS 各一轮。Windows 六项缩放断言通过，Linux
SourceView 六项通过，但这不等于每帧预算已经达标。

未通过：Linux TextView 宽／角落拖动只观测到 1 个中间 viewport，要求至少 2 个；
macOS VNC 的六项缩放均未观测到中间 viewport。最终尺寸和全文验证正确不能替代
响应性断言。部分缩放收尾等待反而增加，保留在 GOUI 的完整对照中。

通过 `go test ./...` 的 workspace 联调，GOUI 全包及其 `GOWORK=off` 全包、相关 race
检查；Windows 原生完整 DirectWrite／textedit、macOS 原生完整 CoreText／textedit
测试均通过。新增的原生字体回归覆盖窄宽和不换行大坐标，未放宽旧测试。

SourceView `GOWORK=off go test ./...` 仍返回 `updates to go.mod needed`；未执行 tidy，
未改动用户已有的 `go.mod` replace，也未把 workspace 通过写成独立发布通过。
新 GOUI 能力尚未发布，本轮继续使用不提交的相邻工作区。

窗口、焦点、指针和本机 UTF-8 剪贴板曾随测试变化；所有测试窗口已退出，输入文件
未改变，Windows 自建启动任务已删除，用户既有远端连接保留。产物在仓库外临时目录；
`example/temp` 和其他用户未跟踪工作未纳入提交。
