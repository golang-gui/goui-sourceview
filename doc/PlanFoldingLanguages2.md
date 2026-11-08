# 编程语言折叠第二批验收（2026-11-08）

## 范围

基线 `4290ea4`，功能提交 `3079d83`，依赖 GOUI v0.7.0。
本轮新增 Kotlin、Swift、Dart、Scala、Groovy、PowerShell 的内置 FoldingFunc。
设计和边界见 [代码折叠](DesignFolding.md)，上一批五种语言的历史记录见
[第一批](PlanFoldingLanguages.md)。本轮没有修改 GOUI、依赖版本或公开 API。

复用六个现有扫描器和第一批的 worker 私有观察器，记录真正的打开／关闭／恢复位置。
进入插值表达式保留外层字面量，内层字面量独立入栈。Kotlin／Scala 延迟确认三引号
关闭时仍保留准确位置；PowerShell here-string 按扫描器在行首确认终止符。
按各语言规则识别普通／原始／三引号／slashy 等字面量、块注释和显式 region。
公开 Scan 传入 nil 观察器，继续使用原分词规则和不可变 State。

## 逻辑回归

`language/folding_interpolated_test.go` 使用固定源文本与独立的逐行范围预期，覆盖：

- 六种公开内置定义具备折叠；普通括号块、单行结构、关闭行后的其他语句、
  不匹配／未闭合结构和嵌套／交叉／未闭合 region。
- Kotlin 多 dollar 模板、Swift 扩展分隔符与多行正则、Dart raw 三引号、
  Scala raw／插值三引号、Groovy slashy／dollar-slashy／转义续行、
  PowerShell 普通多行引号／here-string／smart quotes。
- 六种语言的外层字面量、插值表达式与内层多行字面量精确嵌套；
  行内关闭一个又打开另一个字面量，以及错误内层引号的恢复。
- Kotlin／Swift／Dart／Scala 嵌套块注释；PowerShell 首个 `#>` 关闭块注释的非嵌套规则。
- 字符串／块注释中的伪 region；打开／关闭跨 4096 字节扫描边界、
  PowerShell 缩进伪关闭符／错误 here-string 打开头和多字节源位置。
- 每种语言并发复用同一个 FoldingFunc，分别扫描不同起始行、跨多个分片的长字面量；
  各自范围互不污染。扫描长行途中取消返回 context.Canceled 和空结果。
- Scala 3 纯缩进结构本轮不产生范围，明确守住本批边界。

将 `builtin.go` 用仓库外 Go overlay 恢复为基线注册，保留新回归：
`TestBuiltinInterpolatedFolding` 六种语言均确定失败于 `builtin has no folding provider`。
当前注册通过；该对照没有改写工作区文件。第一批及其余既有语言测试全部通过。

## 本机独立会话

Linux amd64 / Go 1.25.8 / Pango；独立 TigerVNC 1.12.0 24-bit + Xfwm4 4.16.1 compositor，
专用 Xauthority、D-Bus 和临时 XDG config/cache。全部关闭 Go 工作区构建。
实际 painter 和 PixelScale 由窗口探针核对。

`tests/window/editor/folding_languages` 保留第一批五种语言，新增六种语言的固定样本。
每个新增样本同时包含外层代码、内层代码块、多行字面量和其中的插值表达式；
精确断言各自候选，再经正常 Window.DispatchEvent 键盘／指针路径检查折叠、可见快照、
边界 Delete、Undo、搜索展开、禁用／恢复、关闭括号编辑后的重分析及共享文档双视图隔离。
只读镜像仍可折叠，单视图操作不改变另一视图的折叠意图；切换文档覆盖旧文档卸载路径。

| 场景 | 实际后端／缩放 | 结果 |
| --- | --- | --- |
| 十一种语言，Basic | Software 1× | 通过 |
| 十一种语言，Modern Light | Software 1.5× | 通过 |
| 十一种语言，Modern Dark | Software 2× | 通过 |
| 十一种语言，Modern Light | OpenGL 1× | 通过 |
| 十一种语言，Modern Dark | OpenGL 1.5× | 通过 |
| 既有 JSON/HTML folding，Modern Light | Software 1× | 通过 |
| 既有 JSON/HTML folding，Modern Dark | OpenGL 2× | 通过 |

完整 vet 检出窗口样本的 40 处跨包 FoldRange 未写字段名，其中 16 处来自既有五种语言。
补齐字段名并重新构建后，完整 vet 通过；最终源码又复跑 Basic Software 1× 和
Modern Dark OpenGL 1.5×，十一种语言均通过。原有范围值、断言与输入步骤保持一致。
合计 7 组主验证与 2 组最终复跑通过。

日志和二进制位于 `/tmp/sourceview-folding2-RHiFTFpL/`；主矩阵明细为
`desktop-8l36emdr/results.json`，最终复跑为 `desktop-nxx2uix4/results.json`。
窗口、专用 X11/WM/D-Bus 会话已退出，没有操作用户桌面、剪贴板、用户文件或系统设置。
OpenGL 通过不等于已验证硬件 GPU。

## 命令与限制

全部命令使用 `GOWORK=off`：

- `go test ./... -count=1`：通过；两次提交各自的独立索引树导出同样通过。
- `go test -race . ./language ./internal/highlight ./internal/editor ./ui`：通过；
  加强长字面量并发样本后，`go test -race ./language -count=1` 再次通过。
  补充 PowerShell 非嵌套注释样本后，`go test ./language -count=1` 通过。
- `go vet ./...`：字段名修正后通过，两次提交独立索引树也通过；没有关闭检查器。
- `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./...`、
  `CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build ./...`：最终源码交叉构建通过。

初次 vet 失败记录在 `vet.log`，最终通过记录在 `vet-final.log`。
`registration-before.log` 是预期的基线回归失败；`go-test.log`、`race.log`、
`language-race-final.log`、`core-tree-test.log`、`core-tree-vet.log`、
`final-tree-test.log` 与 `final-tree-vet.log` 记录通过的验证。
本轮全仓测试未重现第一批记录的 semantic 测试进程启动段错误，不据此宣称其原因已确认。

Windows/macOS 仅交叉构建，未运行原生桌面；物理 IME、混合 DPI、性能对照未执行。
Scala 3 缩进块、JSX/TSX/PHP 混合文档，以及 Lua/Ruby/Shell/SQL/CMake 等语言的后续
折叠规则不在本批范围，不执行条件编译或运行时语义。没有发现需要修改或绕过 GOUI 的功能缺口。
本轮按功能与窗口验收拆分提交；没有发布、推送或修改现有草稿和临时示例。
