# 混合文档折叠第三批验收（2026-11-08）

## 范围与实现

基线 `2ff12c9`，功能提交 `d5b674b`，依赖 GOUI v0.7.0。
新增 JSX、TSX、PHP 的内置 mixedFolding，设计见 [代码折叠](DesignFolding.md)。
不增加公开 API、依赖，不修改 GOUI 或其他语言的公开分词规则。

JSX/TSX 通过现有扫描器确认标签进入／完成／泛型回退，保存完整名称，区分大小写
配对元素与 Fragment。独立保存尚未完成的属性外层标签；属性表达式可再含 JSX。
JS/TS 扫描路径观察实际字符串和注释，模板插值与 JSX 切换保持外层字面量。
不把 JSX 正文的 String Context 当成跨行字面量。

PHP 记录真实代码区间，只将区间内 token 交给 PHP 结构分析；括号和六种替代控制
结构可跨 PHP/HTML 切换。字面量观察覆盖普通多行字符串、插值、heredoc/nowdoc。
HTML 使用保持字节位置和 LF 的 PHP 屏蔽投影；动态标签名不制造静态元素，
PHP 字符串里的伪标签不进入 HTML 匹配。script/style 仍按 HTML raw 元素折叠，
未新增其内 JS/CSS 独立块折叠，也不启用 PHP 内部 HTML profile 的自动标签编辑。

跨语言范围若在物理行上交叉，按起点顺序保留较早的已确认候选，省略交叉候选；
同起点保留外层。观察器、名称、投影和控制状态由单次 worker 所有，不写入 Tokenizer State。
取消检查覆盖扫描分片、投影屏蔽、控制 token 遍历与归并；结果沿用原 Document 修订核验。

## 逻辑回归

`language/folding_mixed_test.go` 使用固定文本和独立的精确范围预期：

- 三种公开定义具备折叠；此前的注册运行新回归，三者均确定失败于
  `builtin has no mixed folding provider`，其余新边界测试已经通过。
  当前注册后全部通过，首次预期失败保留在 `mixed-red.log`。
- JSX/TSX 配对标签、命名空间／成员名称、Fragment、自闭合、多行属性引号、
  嵌套属性 JSX、代码中的伪标签、大小写／嵌套错配、未闭合、关闭行后的其他语句、
  JSX 正文中的伪 region 及代码 region。
- 模板字符串内嵌 JSX、JSX 表达式内的嵌套模板与插值；TSX 泛型箭头函数和比较运算。
- PHP 与静态 HTML 混排、跨片段代码括号、六种替代控制结构、嵌套条件／分支、
  错误结束词、方法／静态成员伪控制头、PHP 关闭标记隐含分号、交叉范围过滤。
- PHP heredoc/nowdoc、嵌套／相邻字符串、错误 heredoc 打开头、注释、HTML 注释、
  属性中 PHP、动态标签名、raw script 及行注释中关闭 PHP 的切换边界。
- 完整长标签名和 heredoc 标签跨越 4096 字节分片；PHP 打开／关闭、JSX 字面量
  位于分片边界；并发复用同一 provider 时的标签、PHP 字面量／代码区间隔离；长行途中取消。

没有执行 JS/TS/PHP 编译器或运行动态输出，不将这些词法结构样本等同完整语法验证。

## 本机独立桌面

Linux amd64 / Go 1.25.8 / Pango；TigerVNC 1.12.0 24-bit + Xfwm4 4.16.1 compositor，
专用 Xauthority、D-Bus 和临时 XDG config/cache。全部关闭 Go 工作区构建，
实际 painter 与 PixelScale 由窗口内探针记录。

新程序为 `tests/window/editor/folding_mixed`。每组运行 JSX、TSX、PHP 替代控制模板、
PHP nowdoc 四个固定场景。两视图共享文档，其中一个只读；通过正常 Window.DispatchEvent
键盘／指针输入验证精确候选、折叠可见快照、历史不变、搜索展开和真实源字节选区。
边界首次 Delete 仅展开，第二次编辑，Undo 恢复；删除 JSX 关闭标签、PHP endif、
nowdoc 终止符后核对失效范围，Undo 后等待原始候选恢复。只读折叠及视图隔离均有断言。

| 场景 | 实际后端／缩放 | 结果 |
| --- | --- | --- |
| 混合文档四场景，Basic | Software 1× | 通过 |
| 混合文档四场景，Modern Light | Software 1.5× | 通过 |
| 混合文档四场景，Modern Dark | Software 2× | 通过 |
| 混合文档四场景，Modern Light | OpenGL 1× | 通过 |
| 混合文档四场景，Modern Dark | OpenGL 1.5× | 通过 |
| 前两批十一种语言，Basic | Software 1× | 通过 |
| 前两批十一种语言，Modern Dark | OpenGL 1.5× | 通过 |
| 既有 JSON/HTML folding，Modern Light | Software 1× | 通过 |
| 既有 JSON/HTML folding，Modern Dark | OpenGL 2× | 通过 |

首次 Basic 运行因用例在边界 Undo 后、异步候选恢复前检查只读视图而失败于
`missing fold header`。将该检查移到两视图精确范围的就绪条件之后，保留原有所有断言；
随后完整 9 组均通过。补充取消检查的最终源码重新构建后，又复跑 Modern Dark
OpenGL 1.5×，四种场景全部通过。

日志／二进制位于 `/tmp/sourceview-folding3-PH8Bzy4K/`。初次窗口失败为
`desktop-zg_aooez`，完整通过矩阵为 `desktop-s9ha6z1h/results.json`，最终复跑为
`desktop-6hx3ca41/results.json`。窗口与专用 X11/WM/D-Bus 会话已退出，没有操作
用户桌面、剪贴板、用户文件或系统设置。OpenGL 通过不宣称硬件 GPU 验收。

## 命令与限制

全部命令使用 `GOWORK=off`：

- `go test ./... -count=1`：通过；两次提交各自的独立索引树导出同样通过。
- `go test -race . ./language ./internal/highlight ./internal/editor ./ui`：通过。
  加强取消、长标签／字面量及并发 PHP 样本后，`go test -race ./language -count=1` 通过；
  补充模板内 JSX 样本后的 `go test ./language -count=1` 通过。
- `go vet ./...`：通过，两次提交的独立索引树也通过，没有关闭检查器。
- `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./...`、
  `CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build ./...`：最终源码交叉构建通过。

全仓／race／vet／功能提交导出日志分别为 `go-test.log`、`race.log`、`vet-final.log`、
`core-tree-test.log`、`core-tree-vet.log`、`final-tree-test.log` 与 `final-tree-vet.log`。
本轮未重现第一批记录的 semantic 测试进程
启动段错误，不据此宣称该问题原因已经确认。

Windows/macOS 仅交叉构建，未运行原生桌面；物理 IME、混合 DPI、性能对照未执行。
Scala 3 缩进块及 Lua/Ruby/Shell/SQL/CMake 等语言折叠仍属后续批次。
动态 HTML 生成、完整编译器语法与运行时语义不在本轮范围；没有发现需要修改或绕过 GOUI 的功能缺口。
按功能与窗口验收拆分提交，没有发布、推送或纳入现有草稿、临时示例。
