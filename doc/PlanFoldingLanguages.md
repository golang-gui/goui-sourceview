# 编程语言折叠第一批验收（2026-11-08）

## 范围与实现

基线 `27ecf0c`，依赖 GOUI v0.7.0；功能提交 `6bf8ada`。
新增 C、C++、Java、C#、Rust 内置 FoldingFunc；不增加公开 API、依赖或 GOUI 修改。
设计与支持矩阵见 [代码折叠](DesignFolding.md)。

复用现有词法器确认括号与字面量。私有、单次 worker 所有的观察器记录真实打开／关闭，
保留 C# 插值外层字符串、嵌套字面量及 Rust 嵌套注释，不依赖单纯的行首／行尾
Context 转换。Java 转义括号保留源文本字节坐标与物理行号；C/C++ 宏指令及续行
不参与代码括号配对。沿用 Document 的任务取消、结果修订核验和视图独立折叠状态。

## 逻辑回归

- `language/folding_code_test.go`：固定源文本和逐项独立范围预期，覆盖普通块、宏续行、
  C 续行字符串、C++ 原始字符串、同一行关闭／打开不同字面量或注释、Java 文本块／
  Unicode 转义括号／注释／引号、C# verbatim/raw/插值及嵌套字符串恢复、Rust raw
  字符串／嵌套注释／生命周期／标签／宏括号、未闭合／不匹配和关闭行后其他语句。
- `language/folding_builtin_test.go`：五种公开内置定义具备折叠；嵌套 region、语言专属
  指令、宏续行及字符串／注释中的伪 marker、交叉／未闭合 marker；打开／关闭位置
  跨越 4096 字节扫描边界；扫描长行途中取消。
- 并发复用同一个 FoldingFunc，在不同文本和起始行中生成各自结果；race 检查通过。
- 将 `builtin.go` 用仓库外 Go overlay 恢复为基线并保留新回归，
  `TestBuiltinCodeFolding` 的五种语言均确定失败于 `builtin has no folding provider`。
  没有改动工作区来制造失败；恢复当前代码后通过。

## 本机独立桌面

Linux amd64 / Go 1.25.8 / Pango；TigerVNC 1.12.0、Xfwm4 4.16.1 compositor，
专用 Xauthority、XDG config/cache 和 D-Bus 会话。程序均关闭 Go 工作区构建。
实际 painter 与 PixelScale 由窗口内探针记录，未以环境变量代替实际后端核对。

| 场景 | 实际后端与缩放 | 结果 |
| --- | --- | --- |
| 新代码语言程序，Basic | Software 1× | 通过 |
| 新代码语言程序，Modern Light | Software 1.5× | 通过 |
| 新代码语言程序，Modern Dark | Software 2× | 通过 |
| 新代码语言程序，Modern Light | OpenGL 1× | 通过 |
| 新代码语言程序，Modern Dark | OpenGL 1.5× | 通过 |
| 既有 JSON/HTML folding，Modern Light | Software 1× | 通过 |
| 既有 JSON/HTML folding，Modern Dark | OpenGL 2× | 通过 |

新程序位于 `tests/window/editor/folding_languages`。每组分别运行五种语言，
通过正常 Window.DispatchEvent 键盘／指针路径，逐项断言：

- 两视图等待精确的异步候选；折叠代码与字面量／注释，检查可见快照及指针。
- 一个视图折叠时，另一个只读视图、原文和撤销历史保持各自语义；只读也可以折叠。
- 隐藏边界首次 Delete 仅展开，第二次编辑，Undo 恢复源文本及候选。
- 搜索展开外层区域，仍选中精确的真实源文本字节。
- 单视图禁用／恢复折叠不影响另一视图；删除关闭括号后，两个视图等待新的范围，
  Undo 后重新获得原候选。切换 Document 覆盖旧文档卸载和任务取消路径。

7 组程序全部通过自动断言。窗口、专用 X11 服务和 WM 已退出；未操作用户桌面、
剪贴板或文件，也未修改系统设置。OpenGL 结果不宣称硬件 GPU 验收。

## 命令结果与限制

命令均使用 `GOWORK=off`：

- `go test ./... -count=1`：复查及两次提交各自索引树的独立导出均通过。
- `go test -race . ./language ./internal/highlight ./internal/editor ./ui`：通过。
  补充取消测试后再次运行 `go test ./language -count=1`、`go test -race ./language -count=1` 通过。
- `go vet ./...`：通过；没有增加跳过项或关闭检查器。
- `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./...`：交叉构建通过。
- `CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build ./...`：交叉构建通过。

首次全仓测试的 `example/semantic/analysis` 进程在约 0.008 秒处报告
`signal: segmentation fault (core dumped)`，没有 Go 堆栈；该次全仓运行记为失败。
随后该包完整单独测试、全仓复查均通过。仓库外导出的原始基线构建后，20 次独立
进程启动也均通过。没有足够证据确认原因或归属，保留首次失败日志，不将其宣称为
已确认的既有问题或已修复问题；本轮未修改 semantic 示例。

日志、独立导出和二进制保存在 `/tmp/sourceview-folding-kxteTEVv/`，桌面明细在
`desktop-8v6l83dd/results.json`。`registration-before.log` 为预期的基线回归失败；
`go-test.log` 为首次全仓失败，`go-test-recheck.log`、`core-tree-test.log` 和
`final-tree-test.log` 为通过记录。

未执行 Windows/macOS 原生运行、物理 IME、混合 DPI 或性能对照，不将交叉构建等同
原生验收。未发现本轮需要绕过或修改 GOUI 的功能缺口。
Kotlin/Swift/Dart/Scala/Groovy/PowerShell/JSX/TSX/PHP，以及 Lua/Ruby/Shell/SQL/CMake
等其余语言的折叠仍属后续批次。本轮不评估预处理器条件、不展开宏、不提供完整 AST 折叠。
没有更新版本号、推送或重新发布 v0.4.0；原有草稿与临时示例未纳入提交。
