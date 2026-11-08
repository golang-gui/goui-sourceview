# 结构化语言折叠第五批验收（2026-11-08）

## 范围与实现

基线 `6d8a6d3`，功能提交 `f6e6229`，依赖 GOUI v0.7.0。
新增 Zig、R、GraphQL、HCL/Terraform、Protobuf、Starlark 的内置折叠，
具体边界和官方语法参照见 [代码折叠](DesignFolding.md)。
复用 structuralFolding，不增加公开 API、依赖或 GOUI 修改。

- Zig：代码括号与扫描器实际确认的连续 `\\` 多行字符串。
  空行、其他代码、新的赋值切断连续区域；EOF 可结束行串，单独一行不折叠。
- R：代码括号、已闭合普通／raw 多行字符串；raw 括号种类和 dash 数量沿用扫描器。
  引号、引用名称、注释、自定义运算符里的伪括号不参与代码配对。
- GraphQL：查询／类型／参数／输入值括号和实际闭合的 block string；
  转义三引号不闭合，普通字符串的错误行末恢复不产生多行字面量。
- HCL：块／对象／列表／插值括号、字面量／heredoc／块注释与模板 if/for 指令。
  `%{...}` 头闭合后才处理控制结构；跳过注释、strip marker 和正文转义。
  模板块按所属字符串隔离，内层字符串中的 endif 不能关闭外层 if。
- Protobuf：message/enum/service/oneof 等代码括号、实际闭合的块注释／字面量。
  不执行 protoc、读取 import 或分析 option 类型。
- Starlark：仅 def/if/elif/else/for 缩进块、跨行容器／头和闭合多行字符串。
  支持紧凑条件与反斜线续行，只接受空格缩进；Python 其余保留字不成为本语言块。

观察器由单次 worker 所有，公开 Scan 仍传 nil；不可变 State、GUI 缓存和公共分词规则不变。
本批按起点归并、同起点外层优先并过滤行范围交叉；前四批原有候选生成策略保持原样。
继续使用 Document 的取消、修订核验和视图状态隔离。

## 逻辑验证

`language/folding_data_test.go` 使用固定源文本和独立的精确范围预期：

- 注册前运行六种公开定义的回归，均确定失败于 `builtin has no data folding provider`；
  注册后通过，失败证据保存在 `registration-before.log`。
- Zig 嵌套括号、连续行串、EOF、空行／新赋值断开、引用名称与字符／注释伪括号。
- R 嵌套代码、raw 分隔符、普通多行／相邻字符串、引用名称和自定义运算符。
- GraphQL 跨行参数与查询嵌套、block string 转义／相邻字面量、普通字符串错误恢复。
- HCL 对象／列表、字符串跨插值、heredoc、相邻注释／字符串、嵌套模板 if/for/else、
  指令所属模板隔离、正文转义、注释与 strip marker、空条件／未知／错配指令、
  多行控制头、关闭行仍有正文及未闭合 heredoc。
- Protobuf 嵌套声明／RPC、相邻块注释及引号内伪结构；Starlark suite、跨行头、
  紧凑条件、反斜线续行、load／推导式、字典冒号、三引号内的伪缩进、相邻字面量、
  不支持的 Python 块、无缩进与 Tab 缩进。
- 六种语言的未闭合、错配及关闭行后其他正文；八路并发复用同一 provider，
  打开／关闭边界位于 4090..4097 字节、正文 9000 字节，检查范围与状态隔离和途中取消。
- HCL 的 heredoc 标签长达 5000 字节，模板头／尾跨扫描分片边界；
  另外对六种语言以 1..17 字节扫描预算逐步比较公开 Scan 与带观察器的 Scan，
  明确断言 token、Consumed、ReplaceFrom 和 State.Equal 均一致，包含 Unicode 文本。

没有执行这些语言的编译器、解释器、数据库或配置工具，不将词法折叠等同完整 AST 验证。
首次注册失败后没有删弱断言或增加 skip；全部新用例及既有语言测试通过。

## 本机独立桌面

Linux amd64 / Go 1.25.8 / Pango；TigerVNC 1.12.0，24-bit，2560×1800，
Xfwm4 4.16.1，专用 Xauthority、D-Bus 和临时 XDG config/cache。
所有程序使用 `GOWORK=off` 构建，窗口探针记录实际 painter 与 PixelScale。

新程序为 `tests/window/editor/folding_data`，每组七个固定文档：六种语言各一份，
另加 HCL heredoc 中嵌套 if/for 模板。每份两视图共享 Document，其中一个只读。
通过正常 Window.DispatchEvent 模拟键盘／指针，断言精确异步候选、折叠／展开、
隐藏源文本快照、搜索展开及真实字节选区。折叠不修改正文／撤销历史，各视图状态独立。
边界第一次 Delete 仅展开、第二次编辑、Undo 恢复；等待两视图候选就绪后验证只读折叠。
删除外层关闭 token 后只剩独立预期的内层范围，Undo 恢复原候选；Starlark 改为删除
外层 def 头，HCL 模板文档删除 heredoc 终止符。切换文档覆盖旧任务取消与脱离。

| 场景 | 实际后端／缩放 | 结果 |
| --- | --- | --- |
| 第五批七个文档，Basic | Software 1× | 通过 |
| 第五批七个文档，Modern Light | Software 1.5× | 通过 |
| 第五批七个文档，Modern Dark | Software 2× | 通过 |
| 第五批七个文档，Modern Light | OpenGL 1× | 通过 |
| 第五批七个文档，Modern Dark | OpenGL 1.5× | 通过 |
| 第四批五种语言，Basic | Software 1× | 通过 |
| 第四批五种语言，Modern Dark | OpenGL 1.5× | 通过 |
| 第三批混合文档，Basic | Software 1× | 通过 |
| 第三批混合文档，Modern Dark | OpenGL 1.5× | 通过 |
| 前两批十一种语言，Basic | Software 1× | 通过 |
| 前两批十一种语言，Modern Dark | OpenGL 1.5× | 通过 |
| 原有 JSON/HTML folding，Modern Dark | OpenGL 2× | 通过 |

完整 12 组均通过，无窗口断言失败；结果为 `desktop-z_9gywvv/results.json`，
汇总日志为 `desktop.log`，各场景完整输出在同一桌面子目录。
没有把 OpenGL 通过表述为硬件 GPU 验收；窗口测试不是物理键盘或物理 IME 测试。
所有应用与本次专用 X11/WM/D-Bus 会话已退出，认证文件已清理；未操作用户桌面、
用户文件、剪贴板或系统设置。

## 命令、提交与限制

日志及仓库外导出位于 `/tmp/sourceview-folding5-avDf5KaZ/`，全部命令使用 `GOWORK=off`：

- `go test ./... -count=1`：通过（`go-test.log`）。
  两个提交各自的独立索引树导出同样通过（`core-tree-test.log`、`final-tree-test.log`）。
- `go test -race . ./language ./internal/highlight ./internal/editor ./ui`：通过（`race.log`）。
- `go vet ./...`：通过（`vet.log`），两个提交独立导出也通过
  （`core-tree-vet.log`、`final-tree-vet.log`）。
- `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./...`、
  `CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build ./...`：交叉构建通过，
  分别见 `windows-build.log`、`darwin-build.log`；未在这两个平台执行原生桌面。

窗口与基础测试日志另有 `native-build.log`、`base-test.log`、`data-test.log` 和
`language-test.log`。独立导出为 `commit-core`、`commit-final`，均未包含原有草稿／临时示例；
最后补写验收结果只修改本记录，没有改变已验证源码。本轮全仓及两个独立导出
未重现前几批记录的 semantic 进程启动段错误，
不据此宣称其原因已确认或问题已修复。

物理 IME、混合 DPI、性能对照与其他平台原生后端未执行。Scala 3 缩进块、其余尚未具备
专用规则的语言仍属后续工作；本批不执行动态模板或提供完整编译器语法分析。
没有发现本轮需要修改或绕过 GOUI 的功能缺口；没有推送、发布或变更版本号。
按功能实现与窗口验证拆分英文提交，原有草稿与未跟踪临时示例均未纳入。
