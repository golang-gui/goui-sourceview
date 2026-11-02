# 着色缓存切片与验收

日期：2026-10-16。GOUI 基线为 `2f782897684f1bfc892a11b0254e5a2162980ed8`，
叠加当时未提交的 DirectWrite 全范围前景覆盖回收修正。SourceView 当时也未提交。
后续该修正已独立提交为 GOUI `227220a`；本篇保留原测量范围，当前提交和验证见
[编辑体验验收](PlanEditorExperience.md)。
设计契约见 [布局设计](DesignLayout.md#10-着色失效与排版失效分离)。

## 1. 问题与修改

- 旧代码在高亮版本、选区或焦点改变时销毁正文/行号布局，方案切换也清空布局和宽度估计。
  `TestColorChangesReuseParagraph` 在旧代码复现布局创建数 3→5。
- 语言切换此前重建全部行元数据，未修改文本的 LineID 变化，令排版缓存失效。
  `TestLanguageChangePreservesTextLayoutAndClearsTokens` 在旧代码复现身份变化。
- 现在把着色状态与排版身份分开：完整恢复普通前景，再加当前 token 和活动选区前景；
  行号独立更新颜色。保持 TextLayout、Geometry、HeightIndex 和正文宽度；字体等真实排版变化仍失效。
- 初始元数据完成时把同修订/行号的临时布局转接到 LineID；语言切换保留已完成的文本索引，
  重新扫描前隐藏旧语言 token。Plain 不提供语法 token，旧载荷仍受既有 LRU 上限约束，后续重扫/卸载回收。
- 缓存先移除所有权再销毁；颜色调用后检查文档/修订/选区/挂载/布局/缓存所有者和方案状态。
  回调换文档时停止使用旧布局，未发布到缓存的对象仍由当前调用释放。
- GOUI DirectWrite 原先每次 SetTextColor 都追加记录，整段覆盖后仍保留已无效的旧范围。
  回归在 Windows 修复前第二轮复现 4 条记录，修复后 1000 轮维持完整前景+一个局部覆盖的 2 条，
  同时核对 UTF-8→UTF-16 范围。此次不修改 API，也不声称优化了任意局部范围更新序列。

## 2. 验证与环境

| 项目 | 结果与边界 |
| --- | --- |
| Linux amd64 / Ubuntu 22.04.5 / Go 1.25.8 / Pango 1.50.6 | `go test ./...`、`go test -race ./...`、`go vet ./...` 通过 |
| 三端原生前景度量 | `TestNativeForegroundMetrics` 分别在 Linux、Windows 10 19045 amd64 / Go 1.24.0、macOS 15.7.7 arm64 / Go 1.24.9 通过 |
| 三端原生着色位图 | `TestNativeRecolorMatchesFreshLayout` 通过：复用布局切换方案、移除语法色后，逐字节等于独立基础前景构造的布局 |
| Windows DirectWrite | 新回归修复前失败、修复后通过；`go test ./platform/typography/directwrite -count=1` 通过 |
| Windows/macOS SourceView | 两台远端均执行 `go test ./... -count=1` 通过；没有启动 SourceView 交互窗口 |
| Windows/macOS 测试目标编译 | `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -exec /bin/true ./...` 及 darwin/arm64 对应命令通过；这是编译检查 |
| Linux 独立 Xtigervnc / Xfwm4 | Software 1x/2x、OpenGL 1x 的 `go run ./tests/window/editor -auto` 通过，日志确认实际后端/PixelScale |
| GOUI 本机完整测试 | 在隔离 X11 中执行 `GOWORK=off go test ./...` 通过，包含工作副本既有测试 |
| SourceView 独立依赖 | `GOWORK=off go test ./...` 仍失败：已发布 GOUI v0.1.1 缺少 ApplyBatch/BatchOptions；没有用本地 workspace 成功替代发布验证 |

原生度量样本为请求 `monospace`/`serif`、12 pt、不折行/WordChar、64 DIP Tab，包含
`ffi`、组合字符、中文、emoji、希伯来文。对初始度量做独立切片拷贝，依次覆盖全文、
连字内局部、组合字符局部及再次全文，逐字段比较行/Cluster/整体尺寸。
这些是代表性字体请求与文本样本，不是所有字体、语言和显示 DPI 的保证。

位图比较使用原生 `TextBitmap` 的预乘 RGBA、同一后端/字体/比例 1、完整像素缓冲，容差为零，
不跨平台使用共享金图。样本 `true` 的 JSON 语法色独立预期为 Keyword 前景；移除高亮后为普通前景。
真实窗口另验证活动焦点下的整行选区，用 SelectionForeground 构造独立参考布局，
并观察 20 次配色/选区更新均未创建新的正文布局。窗口在 1x/2x 运行，参考位图比较自身使用 1x。

逻辑回归还覆盖：未变状态无重复颜色 setter、冷索引布局转接、语言来回切换、旧 token 隐藏、
行号前景、重入释放、旧有搜索原始匹配与 Cluster 选区分离和共享视图资源释放。

## 3. 前后对照

命令：`go test -run '^$' -bench '^BenchmarkParagraphSelection$' -benchmem -benchtime=200ms -count=3`。
环境为上述 Linux、Intel Xeon Gold 5318Y；固定模拟排版 Context，生成 80/4096 字符文本，
正文布局建立后计时，只包括选区更新和段落访问。初始化、绘制、栅格化、桌面合成不计入。

| 文本 | 修改前（3 次中位数） | 修改后（3 次中位数） | 布局创建数 |
| --- | --- | --- | --- |
| 80 字符 | 31.061 µs/op，41825 B/op | 1.239 µs/op，4 B/op | 约 1 → 0 layouts/op |
| 4096 字符 | 1.040 ms/op，2746350 B/op | 29.887 µs/op，4 B/op | 约 1 → 0 layouts/op |

这是控制器/Go 几何与模拟布局对象的对照，不是原生 shaping 耗时或编辑器 P95 帧率。
零新布局对象不表示后端内部不重算 runs/shaping/raster，也不表示没有 GPU 或原生内存分配。
常规 1/10 MiB 文件、超长行、百万行和长期资源压力仍需按测试规格另测。

## 4. 副作用与保留项

- 新增测试不写源码目录，不改剪贴板。窗口仅在独立 X server/WM 执行，结束时清理该会话。
- 远端通过 remote-shell 在临时目录解包验证；Windows 为 SSH Session 0，macOS 为原生离屏排版，未改已有远端检出。
- GOUI 工作副本全仓测试仍有既有写图副作用；本轮未迁移这些测试。
- 真实 IME、跨屏/真实混合 DPI、所有 GPU 组合和完整规模矩阵未执行，不计作通过。
- 两仓工作区依赖仍未发布；DirectWrite 修正已在 GOUI `227220a` 独立提交，仍待发行版本提供。

## 5. 输入期间的着色快照（2026-11-02）

修正当前行及后续行因词法状态待确认而先清色、随后恢复的闪烁。
引擎保留一份已映射到当前文本坐标的显示快照，另用一个工作缓冲分析新 token；
完成整行后发布，相同结果保持显示版本。括号、配对、补全继续要求有效状态，
不会将显示提示用于编辑判断。新旧 token 一起计入 16 MiB payload 目标，淘汰时释放定义引用。
更早行的缓存重载取消后续工作时，该未完成行仍保留在 dirty 待办中。
完整规则及参考源码见 [稳定着色](DesignHighlighting.md#71-编辑期间的稳定着色)。

旧代码 `a5da6e5` 上，新增
`GOWORK=off go test . -run '^TestEditingKeepsParagraphColorsWhileAnalysisPending$' -count=1`
失败：编辑字符串后，整行变为普通前景 `{36 41 47 255}`。
关闭工作区，在该提交的仓库外归档中仅加入同一窗口验证程序，首次输入即在 `(10,9)` 复现像素变化。
修正后纯测试覆盖 Unicode、插入/删除、分行/合行、替换、撤销/重做、跨行注释变化、
同 ID 的语言定义切换、长行片段被连续编辑打断、LRU 重载前移和共享预算；
最终 token 与重新完整分析一致，未修改段落复用原生布局且不调用颜色 setter。

Ubuntu 22.04.5 amd64 / Go 1.25.8 下通过 `go test ./...`（工作区 GOUI b49049d）、
`GOWORK=off go test ./...`（已发布 GOUI v0.5.0）、`GOWORK=off go test -race ./...`、
`GOWORK=off go vet ./...`，以及 Windows amd64 / macOS arm64 的 CGO_ENABLED=0 `go build ./...`。
新增同 ID 显示隔离断言后，另执行针对性 race 检查通过。本次没有 GOUI、依赖或公开 SourceView API 改动。

专用 TigerVNC :87 / Xfwm4 compositor on / Pango / Software：
`tests/window/editor/highlight` 浅色 1×、深色 1.5× 均通过 127 帧检查，包含 30 次输入、
30 次退格、撤销/重做和结束后的五帧。除自然帧外，每次输入后都经正常 PaintEvent 分发，
确定性覆盖分析 Timer 前的一帧；不调用内部分析动作或改变私有调度状态。
RenderWidget 在 Paint 外采样，首行固定前缀和后续行与同一字体/后端基线的预乘 sRGB RGBA 零误差相等，
裁剪排除光标及被编辑区域；基线须含实际着色，不能用空白基线通过。
探针初版在首帧尚未分配尺寸时返回 `gui: invalid widget render size or scale`；
修正为仅在基线建立前等待有效 allocation，此后的帧仍逐项完整断言，不放宽颜色容差。

语言窗口的 36 种语言建议、接受/撤销及引用、括号、只读和语义断言全部通过。
editor 浅色 1× 经 XTest 实际键入/退格，12 次桌面采样中未修改的后续正文区域像素不变；
另外观察插入文字和退格恢复，未见整行清色。桌面采样不替代确定性的中间帧与最终状态断言。
日志、归档、二进制和截图在 `/tmp/sourceview-highlight-OIZHr4/`，不写源码目录。
未验证真实 IME、Windows/macOS 原生窗口或作出性能量化结论；预算触发时仍允许按设计降级显示。
结束后关闭本次示例、专用窗口管理器和 X11；没有剪贴板或系统设置改动，保留已有用户会话。
