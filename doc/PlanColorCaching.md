# 着色缓存切片与验收

日期：2026-10-16。GOUI 基线为 `2f782897684f1bfc892a11b0254e5a2162980ed8`，
叠加本轮未提交的 DirectWrite 全范围前景覆盖回收修正。SourceView 本轮也未提交。
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
- 两仓工作区依赖仍未发布；DirectWrite 修正需要随 GOUI 后续独立提交/版本提供。
