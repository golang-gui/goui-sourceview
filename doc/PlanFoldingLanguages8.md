# 函数式语言折叠第八批验收（2026-11-08）

## 范围与实现

基线 `433b1e1`，功能提交 `e58ae58`，依赖 GOUI v0.7.0。
新增 Elixir、Erlang、Julia、OCaml 的内置 Folding provider，具体规则与官方语法参照见
[代码折叠](DesignFolding.md)。继续使用 structuralFolding、现有扫描器和范围归并，
不增加公共 API、第三方依赖或 GOUI 修改。

- Elixir：已确认控制头到 do/end、fn/end、代码／位串括号、闭合引号／heredoc／sigil。
  do 紧邻冒号的简写不形成块，下一行原子表达式保留真实 do 块；成员／原子不作控制头。
- Erlang：begin／case／if／receive／try／maybe 的 end 块，case 须确认 of。
  确认参数及箭头的匿名／命名递归 fun 支持多 clause；本地、远程和变量函数引用不打开块。
  binary 双尖括号可嵌套，代码括号与多行引号按实际边界处理。
- Julia：显式 end 的声明／类型／循环／条件／do 等块，区分 generator／comprehension，
  索引内 begin/end 不抢占外层终止符。闭合多行字符串／字符串宏、插值代码与嵌套注释。
- OCaml：begin／struct／sig／object 到 end，确认 do 的 for／while 到 done；
  普通括号、闭合引号／quoted string 与嵌套注释。注释内字符串保护伪关闭符。

实际插值分别进入 worker 私有控制栈，外层字面量观察跨插值保留；不向不可变 State、
注册 tokenizer 或 GUI 缓存存入可变观察状态。Elixir sigil／Julia 字符串宏的相邻字母
修饰符按片段延长实际结束位置，不改变公开扫描的 token／Consumed／ReplaceFrom 契约。
候选归并、关闭行尾保留、修订／取消与视图隔离沿用既有机制。

## 逻辑验证及失败记录

`language/folding_functional_test.go` 使用公开定义、固定源文本与独立精确范围：

- 注册前四种语言均确定失败于 `builtin has no functional folding provider`，
  记录于 `registration-before.log`；注册后通过。
- Elixir 嵌套模块／函数／条件、跨行函数头、do:／fn:、原子和成员、分支、
  heredoc 嵌套插值、raw sigil、修饰符和移位操作符；插值孤立 end 不影响外层。
- Erlang 条件／receive／after／maybe、命名递归 fun／guard／多 clause、
  本地／远程／变量引用、嵌套 binary 与连续关闭符、quoted atom、字符串及缺少 of。
- Julia 声明／类型／循环／do、推导式／generator、索引 begin/end、推导式中的实际 if、
  数组 begin 中的循环、短函数、symbol／成员、嵌套字符串插值、字符串宏修饰符和嵌套注释。
- OCaml 模块／signature／object、跨行循环、done/end 错配、quoted string 标签、
  嵌套注释和注释内字符串／quoted string、普通 let/match 不推测整体范围。
- 四种语言的未完成结构、关闭行后正文；八路并发复用同一 provider，打开／关闭位于
  4090..4097 字节、正文长 9000 字节，检查隔离和长行途中取消。
- Elixir／Julia 修饰符跨 4096 字节片段，OCaml 5000 字节标签跨片段确认与关闭。
  1..17 字节预算逐步比较公开 Scan 与观察路径的 token、Consumed、ReplaceFrom 和
  State.Equal，包含 Unicode 和嵌套边界；同时覆盖共享扫描器的 F#、Haskell、Clojure、EDN。

首轮范围验证有三类实际失败（`functional-test.log`）：

1. Elixir 的 pending do 把下一行 `:ok` 误当作 do: 简写，导致嵌套范围错位。
   现仅在同一物理行、紧邻 do 的冒号取消 pending 块，保留原精确范围断言。
2. Julia 的 #= 被原有较短的 # 行注释分支拦截，嵌套注释和并发长行注释无范围。
   块注释前缀现优先识别，原有嵌套深度与实际关闭观察继续复用。
3. OCaml 将 `|ta`、LF、`g}` 拼为 quoted string 结束标记，得到错误 `{0 2}`，
   正确范围为 `{0 4}`。现只在真实行末清除部分匹配，扫描片段边界仍保留匹配进度。

后两项通过公开 Scan 的 Context 断言单独复现：仓库外 overlay 仅使用基线
`HEAD:language/scan_functional.go`，新断言分别在 Julia 首行、OCaml 正文和注释内
quoted string 的假关闭行失败（`lexical-baseline.log`）。较早的 `lexical-before.log`
另含旧扫描器缺少折叠观察导致的长标签空范围；未将该项解释为额外的分词缺陷。
最终全部语言回归通过（`language-test.log`），独立提交树也运行全部新增断言。
未增加 skip、删弱既有断言或将错误输入视为完整编译语法验收。

## 本机独立桌面

Linux amd64 / Go 1.25.8 / Pango；TigerVNC 1.12.0、24-bit、2560×1800，
Xfwm4 4.16.1，专用 Xauthority、D-Bus 与临时 XDG config/cache。
所有程序使用 GOWORK=off 构建，窗口探针记录实际 painter 与 PixelScale。

新增 `tests/window/editor/folding_functional`，每组八份文档：Elixir 普通块／插值、
Erlang case+binary／fun+字符串、Julia 推导式与索引／嵌套注释、OCaml 循环+quoted string／
嵌套注释。每份两个视图共享 Document，其中一个只读。
正常 Window.DispatchEvent 模拟键盘和指针，断言精确异步范围、折叠／展开、隐藏文本快照、
搜索展开及真实源字节选区；折叠不修改正文或历史，各视图状态独立。
边界第一次 Delete 仅展开，第二次编辑，Undo 恢复；等待候选后检查只读折叠。
删除外层关闭 token 后只剩预期内层区域，两视图候选同步更新；Undo 恢复原候选，
切换文档覆盖任务取消／脱离。

| 场景 | 实际后端／缩放 | 结果 |
| --- | --- | --- |
| 第八批八份文档，Basic | Software 1× | 通过 |
| 第八批八份文档，Modern Light | Software 1.5× | 通过 |
| 第八批八份文档，Modern Dark | Software 2× | 通过 |
| 第八批八份文档，Modern Light | OpenGL 1× | 通过 |
| 第八批八份文档，Modern Dark | OpenGL 1.5× | 通过 |
| 第七批九份文档，Basic | Software 1× | 通过 |
| 第七批九份文档，Modern Dark | OpenGL 1.5× | 通过 |
| 第六批八份文档，Basic | Software 1× | 通过 |
| 第六批八份文档，Modern Dark | OpenGL 1.5× | 通过 |
| 第五批七份文档，Basic | Software 1× | 通过 |
| 第五批七份文档，Modern Dark | OpenGL 1.5× | 通过 |
| 第四批五种语言，Basic | Software 1× | 通过 |
| 第四批五种语言，Modern Dark | OpenGL 1.5× | 通过 |
| 第三批混合文档，Basic | Software 1× | 通过 |
| 第三批混合文档，Modern Dark | OpenGL 1.5× | 通过 |
| 前两批十一种语言，Basic | Software 1× | 通过 |
| 前两批十一种语言，Modern Dark | OpenGL 1.5× | 通过 |
| 原有 JSON/HTML folding，Modern Dark | OpenGL 2× | 通过 |

完整 18 组通过，无窗口断言失败；另有新增场景的 Basic 初验通过。
结果为 `desktop-lc7dmsid/results.json`，汇总 `desktop.log`；初验为
`desktop-9nrjfcm3/` 与 `desktop-initial.log`。后端通过不宣称硬件 GPU 验收，
模拟输入不等同物理键盘／IME 验收。
全部本次应用及专用 X11/WM/D-Bus 会话已退出，认证文件清理；
没有操作用户桌面、用户文件、剪贴板或系统设置。

## 命令、提交与限制

日志及仓库外导出位于 `/tmp/sourceview-folding8-V2S4tklX/`，命令均使用 GOWORK=off：

- `go test ./... -count=1`：通过（`go-test.log`）；核心提交独立索引树导出也通过
  （`core-tree-test.log`）；窗口／记录提交的独立树也通过（`final-tree-test.log`）。
- `go test -race . ./language ./internal/highlight ./internal/editor ./ui`：通过（`race.log`）；
  核心树包含最终新增用例的 `go test -race ./language -count=1` 也通过（`language-race-final.log`）。
- `go vet ./...`：通过（`vet.log`），两个提交独立导出也通过
  （`core-tree-vet.log`、`final-tree-vet.log`）。
- `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./...`、
  `CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build ./...`：交叉构建通过，
  分别见 `windows-build.log`、`darwin-build.log`，未在这两个平台运行原生桌面。

基础与窗口构建分别见 `base-test.log`、`native-build.log`、`native-regression-build.log`。
两个独立导出为 `commit-core`、`commit-final`，均不含原有草稿或临时示例；
最后补写验收结果只修改本记录，没有改变已验证源码。
本轮全仓及两个独立导出未重现前几批记录的 semantic 测试进程启动段错误，
不据此声称原因已确认或问题已修复。

物理 IME、混合 DPI、性能对照及其他平台原生后端未执行。
F#、Haskell、Scala 3 的缩进规则，以及 Erlang 顶层函数 clause 和 OCaml 普通 let/match
的完整范围仍需后续专用规则；本批不提供宏执行、编译器 AST 或完整错误语法解析。
未修改 GOUI、模块版本、既有草稿或临时示例；未发布或推送。
