# 查询与 reader 折叠第七批验收（2026-11-08）

## 范围与实现

基线 `60883a0`，功能提交 `2e70ba9`，依赖 GOUI v0.7.0。
新增 Nix、jq、PromQL、Clojure、EDN 的内置折叠，
具体设计和官方语法参照见 [代码折叠](DesignFolding.md)。
继续使用 structuralFolding，不增加公共 API、依赖或 GOUI 修改。

- Nix：属性集合、列表、代码／插值括号、闭合普通／缩进字符串和块注释。
  let/in 按分组深度匹配，仅折叠绑定段，in 与结果表达式所在行保持可见。
  旧式 let 集合不会占用外层 in，属性访问中的同名词不当作控制头。
- jq：确认 then 的条件块、elif／else 分支及 end，确认名称／冒号的 def 函数与
  同深度结束分号。参数和调用内的分号不会提前关闭函数，成员／变量／对象键不形成伪块。
  实际插值分别进入独立控制状态，内层孤立 end/in 不影响外层；字面量保留外层观察。
- PromQL：实际函数／聚合／选择器／范围括号，闭合反引号多行字符串；普通引号
  错误行末恢复时撤销观察。字符串／标签中的括号、注释标记不作为代码。
- Clojure/EDN：集合括号、闭合多行字符串、Clojure 正则／reader 条件。
  字符字面量和字符串中的伪括号不配对。#_ 复用 fnDiscard 的实际完整形式结束，
  按注释整体折叠，支持嵌套、标签数据和 Clojure 元数据前缀；内部不制造正文区域。

观察状态由单次 worker 所有，不进入不可变 State 或 GUI 高亮缓存。
公开 Scan 仍使用 nil 观察器，保持 token／Consumed／ReplaceFrom／State 契约。
候选归并、同起点外层优先、交叉过滤、关闭行尾保留和 Document 修订／取消机制沿用既有设计。

## 逻辑验证与失败记录

`language/folding_query_test.go` 使用固定源文本和独立精确范围：

- 注册前五种公开定义均确定失败于 `builtin has no query/reader folding provider`，
  记录于 `registration-before.log`；注册后通过。
- Nix 嵌套／旧式绑定、属性名称、集合／列表、普通／缩进／相邻字符串、转义与嵌套
  插值、插值状态隔离、路径、块注释、未完成绑定。
- jq 条件／分支、嵌套函数、参数分号、函数体内条件、成员和对象键、缺少 then、
  字符串／相邻字符串、嵌套插值函数和控制隔离、缺少名称／冒号／结束分号。
- PromQL 聚合／选择器及函数参数、反引号与反斜线正文、普通引号错误恢复。
- Clojure 集合、正则、字符字面量、reader 条件、相邻字符串、丢弃形式及其后正文、
  嵌套丢弃、元数据、被丢弃字符串和标签数据；EDN 数据／标签丢弃、独占行 marker、
  多行字符串及未完成形式。
- 五种语言的未闭合／错配／关闭行后正文；八路并发复用同一 provider，
  打开／关闭边界位于 4090..4097 字节、正文长 9000 字节，检查隔离及长行途中取消。
- 以 1..17 字节预算逐步比较公开 Scan 与观察路径的 token、Consumed、ReplaceFrom
  和 State.Equal，包含 Unicode、插值和丢弃形式；覆盖五种新增语言，并补充共享扫描器的
  F#、Elixir、Julia，确认新增观察不改变这些既有分词路径。

首次范围回归有两项预期错误：Clojure 嵌套集合及 reader 条件的内层关闭行还承载
外层闭合符。既有规则保持该行可见，正确内层范围分别是 `{1 3}`、`{3 4}`，
而不是 `{1 4}`、`{3 5}`。保留这两个样本的正确断言，同时新增独立关闭行样本，
明确检查较长的 `{1 5}`、`{3 5}` 区域。初次失败保留在 `query-test.log`／
`query-initial.log`，最终全部语言通过见 `language-test.log`；没有为通过用例改变生产结束行规则。

本轮未增加 skip 或删弱既有断言；没有执行 reader／宏、Nix 求值、jq 或 Prometheus 查询，
不将词法测试等同完整语言语法验证。

## 本机独立桌面

Linux amd64 / Go 1.25.8 / Pango；TigerVNC 1.12.0，24-bit，2560×1800，
Xfwm4 4.16.1，专用 Xauthority、D-Bus 和临时 XDG config/cache。
所有程序使用 `GOWORK=off` 构建，窗口探针记录实际 painter 与 PixelScale。

新增 `tests/window/editor/folding_query`，每组九个文档：Nix 集合／绑定、jq 函数／
嵌套字符串插值、PromQL 函数与多行字符串、Clojure 普通形式／丢弃形式、EDN 普通／
整段丢弃数据。每份两视图共享 Document，其中一个只读。
正常 Window.DispatchEvent 模拟键盘／指针，断言精确异步范围、折叠／展开、隐藏文本快照、
搜索展开及真实源字节选区。折叠不修改正文／历史，各视图状态独立。
边界第一次 Delete 仅展开，第二次编辑，Undo 恢复；等待候选就绪后检查只读折叠。
删除外层关闭 token 后只剩预期内层区域，EDN 整段丢弃样本变成空结果；Nix 绑定删除
可见的 in 所在行内容后绑定区域失效。Undo 恢复原候选；换文档覆盖任务取消／脱离。

| 场景 | 实际后端／缩放 | 结果 |
| --- | --- | --- |
| 第七批九个文档，Basic | Software 1× | 通过 |
| 第七批九个文档，Modern Light | Software 1.5× | 通过 |
| 第七批九个文档，Modern Dark | Software 2× | 通过 |
| 第七批九个文档，Modern Light | OpenGL 1× | 通过 |
| 第七批九个文档，Modern Dark | OpenGL 1.5× | 通过 |
| 第六批八个文档，Basic | Software 1× | 通过 |
| 第六批八个文档，Modern Dark | OpenGL 1.5× | 通过 |
| 第五批七个文档，Basic | Software 1× | 通过 |
| 第五批七个文档，Modern Dark | OpenGL 1.5× | 通过 |
| 第四批五种语言，Basic | Software 1× | 通过 |
| 第四批五种语言，Modern Dark | OpenGL 1.5× | 通过 |
| 第三批混合文档，Basic | Software 1× | 通过 |
| 第三批混合文档，Modern Dark | OpenGL 1.5× | 通过 |
| 前两批十一种语言，Basic | Software 1× | 通过 |
| 前两批十一种语言，Modern Dark | OpenGL 1.5× | 通过 |
| 原有 JSON/HTML folding，Modern Dark | OpenGL 2× | 通过 |

完整 16 组均通过，无窗口断言失败；结果为 `desktop-cwsmkueh/results.json`，
汇总为 `desktop.log`，完整场景输出位于同一桌面子目录。
OpenGL 通过不宣称硬件 GPU 验收；模拟输入不等同物理键盘／IME 验收。
所有应用及本次专用 X11/WM/D-Bus 会话已退出，认证文件已清理；
没有操作用户桌面、用户文件、剪贴板或系统设置。

## 命令、提交与限制

日志与仓库外导出位于 `/tmp/sourceview-folding7-IHndJYib/`，全部命令使用 `GOWORK=off`：

- `go test ./... -count=1`：通过（`go-test.log`）；两个提交各自的独立索引树导出
  同样通过（`core-tree-test.log`、`final-tree-test.log`）。
- `go test -race . ./language ./internal/highlight ./internal/editor ./ui`：通过（`race.log`）。
- `go vet ./...`：通过（`vet.log`）；两个提交独立导出也通过
  （`core-tree-vet.log`、`final-tree-vet.log`）。
- `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./...`、
  `CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build ./...`：交叉构建通过，
  分别见 `windows-build.log`、`darwin-build.log`，未在这两个平台运行原生桌面。

基础与窗口构建日志另有 `base-test.log`、`native-build.log`。
两个独立导出为 `commit-core`、`commit-final`，均未包含原有草稿／临时示例；
最后补写验收结果只修改本记录，没有改变已验证源码。
本轮全仓及两个独立导出未重现前几批记录的 semantic 测试进程启动段错误，
不据此声称原因已确认或问题已修复。

物理 IME、混合 DPI、性能对照与其他平台原生后端未执行。
Scala 3 缩进块、其余尚未具备专用规则的语言仍是后续工作；本批不提供通用缩进回退、
完整编译器／reader 语法验证或运行语义。没有发现需要修改或绕过 GOUI 的功能缺口。
按功能与窗口验证拆分英文提交，没有推送、发布或变更版本号，原有草稿／临时示例未纳入。
